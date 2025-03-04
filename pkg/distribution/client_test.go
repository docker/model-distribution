package distribution

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	tc "github.com/testcontainers/testcontainers-go/modules/registry"
)

func TestClientPullModel(t *testing.T) {
	// Set up test registry
	registryContainer, err := tc.Run(context.Background(), "registry:2.8.3")
	if err != nil {
		t.Fatalf("Failed to start registry container: %v", err)
	}
	defer registryContainer.Terminate(context.Background())

	registry, err := registryContainer.HostAddress(context.Background())
	if err != nil {
		t.Fatalf("Failed to get registry address: %v", err)
	}

	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(tempDir)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Use the dummy.gguf file from assets directory
	modelFile := filepath.Join("..", "..", "assets", "dummy.gguf")

	// Read model content for verification later
	modelContent, err := os.ReadFile(modelFile)
	if err != nil {
		t.Fatalf("Failed to read test model file: %v", err)
	}

	// Push model to registry
	tag := registry + "/testmodel:v1.0.0"
	if err := client.PushModel(context.Background(), modelFile, tag); err != nil {
		t.Fatalf("Failed to push model: %v", err)
	}

	// Pull model from registry
	modelPath, err := client.PullModel(context.Background(), tag)
	if err != nil {
		t.Fatalf("Failed to pull model: %v", err)
	}

	// Verify model content
	pulledContent, err := os.ReadFile(modelPath)
	if err != nil {
		t.Fatalf("Failed to read pulled model: %v", err)
	}

	if string(pulledContent) != string(modelContent) {
		t.Errorf("Pulled model content doesn't match original: got %q, want %q", pulledContent, modelContent)
	}
}

func TestClientGetModel(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(tempDir)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Use the dummy.gguf file from assets directory
	modelFile := filepath.Join("..", "..", "assets", "dummy.gguf")

	// Push model to local store
	tag := "test/model:v1.0.0"
	if err := client.store.Push(modelFile, []string{tag}); err != nil {
		t.Fatalf("Failed to push model to store: %v", err)
	}

	// Get model
	model, err := client.GetModel(tag)
	if err != nil {
		t.Fatalf("Failed to get model: %v", err)
	}

	// Verify model
	if len(model.Tags) == 0 || model.Tags[0] != tag {
		t.Errorf("Model tags don't match: got %v, want [%s]", model.Tags, tag)
	}
}

func TestClientGetModelNotFound(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(tempDir)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Get non-existent model
	_, err = client.GetModel("nonexistent/model:v1.0.0")
	if err != ErrModelNotFound {
		t.Errorf("Expected ErrModelNotFound, got %v", err)
	}
}

func TestClientListModels(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(tempDir)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create test model file
	modelContent := []byte("test model content")
	modelFile := filepath.Join(tempDir, "test-model.gguf")
	if err := os.WriteFile(modelFile, modelContent, 0644); err != nil {
		t.Fatalf("Failed to write test model file: %v", err)
	}

	// Push models to local store with different manifest digests
	// First model
	tag1 := "test/model1:v1.0.0"
	if err := client.store.Push(modelFile, []string{tag1}); err != nil {
		t.Fatalf("Failed to push model to store: %v", err)
	}

	// Create a slightly different model file for the second model
	modelContent2 := []byte("test model content 2")
	modelFile2 := filepath.Join(tempDir, "test-model2.gguf")
	if err := os.WriteFile(modelFile2, modelContent2, 0644); err != nil {
		t.Fatalf("Failed to write test model file: %v", err)
	}

	// Second model
	tag2 := "test/model2:v1.0.0"
	if err := client.store.Push(modelFile2, []string{tag2}); err != nil {
		t.Fatalf("Failed to push model to store: %v", err)
	}

	// Tags for verification
	tags := []string{tag1, tag2}

	// List models
	models, err := client.ListModels()
	if err != nil {
		t.Fatalf("Failed to list models: %v", err)
	}

	// Verify models
	if len(models) != len(tags) {
		t.Errorf("Expected %d models, got %d", len(tags), len(models))
	}

	// Check if all tags are present
	tagMap := make(map[string]bool)
	for _, model := range models {
		for _, tag := range model.Tags {
			tagMap[tag] = true
		}
	}

	for _, tag := range tags {
		if !tagMap[tag] {
			t.Errorf("Tag %s not found in models", tag)
		}
	}
}

func TestClientGetStorePath(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(tempDir)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Get store path
	storePath := client.GetStorePath()

	// Verify store path matches the temp directory
	if storePath != tempDir {
		t.Errorf("Store path doesn't match: got %s, want %s", storePath, tempDir)
	}

	// Verify the store directory exists
	if _, err := os.Stat(storePath); os.IsNotExist(err) {
		t.Errorf("Store directory does not exist: %s", storePath)
	}
}

func TestClientDeleteModel(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(tempDir)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Use the dummy.gguf file from assets directory
	modelFile := filepath.Join("..", "..", "assets", "dummy.gguf")

	// Push model to local store
	tag := "test/model:v1.0.0"
	if err := client.store.Push(modelFile, []string{tag}); err != nil {
		t.Fatalf("Failed to push model to store: %v", err)
	}

	// Delete the model
	if err := client.DeleteModel(tag); err != nil {
		t.Fatalf("Failed to delete model: %v", err)
	}

	// Verify model is deleted
	_, err = client.GetModel(tag)
	if !errors.Is(err, ErrModelNotFound) {
		t.Errorf("Expected ErrModelNotFound after deletion, got %v", err)
	}
}
