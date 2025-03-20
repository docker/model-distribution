package distribution

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	tc "github.com/testcontainers/testcontainers-go/modules/registry"

	"github.com/docker/model-distribution/pkg/gguf"
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
	client, err := NewClient(WithStoreRootPath(tempDir))
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

	t.Run("pull without progress writer", func(t *testing.T) {
		// Pull model from registry without progress writer
		err := client.PullModel(context.Background(), tag, nil)
		if err != nil {
			t.Fatalf("Failed to pull model: %v", err)
		}

		model, err := client.GetModel(tag)
		if err != nil {
			t.Fatalf("Failed to get model: %v", err)
		}

		modelPath, err := model.GGUFPath()
		if err != nil {
			t.Fatalf("Failed to get model path: %v", err)
		}
		// Verify model content
		pulledContent, err := os.ReadFile(modelPath)
		if err != nil {
			t.Fatalf("Failed to read pulled model: %v", err)
		}

		if string(pulledContent) != string(modelContent) {
			t.Errorf("Pulled model content doesn't match original: got %q, want %q", pulledContent, modelContent)
		}
	})

	t.Run("pull with progress writer", func(t *testing.T) {
		// Create a buffer to capture progress output
		var progressBuffer bytes.Buffer

		// Pull model from registry with progress writer
		if err := client.PullModel(context.Background(), tag, &progressBuffer); err != nil {
			t.Fatalf("Failed to pull model: %v", err)
		}

		// Verify progress output
		progressOutput := progressBuffer.String()
		if !strings.Contains(progressOutput, "Using cached model") && !strings.Contains(progressOutput, "Downloading") {
			t.Errorf("Progress output doesn't contain expected text: got %q", progressOutput)
		}

		model, err := client.GetModel(tag)
		if err != nil {
			t.Fatalf("Failed to get model: %v", err)
		}

		modelPath, err := model.GGUFPath()
		if err != nil {
			t.Fatalf("Failed to get model path: %v", err)
		}

		// Verify model content
		pulledContent, err := os.ReadFile(modelPath)
		if err != nil {
			t.Fatalf("Failed to read pulled model: %v", err)
		}

		if string(pulledContent) != string(modelContent) {
			t.Errorf("Pulled model content doesn't match original: got %q, want %q", pulledContent, modelContent)
		}
	})

	t.Run("pull non-existent model", func(t *testing.T) {
		// Create temp directory for store
		tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp directory: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Create client
		client, err := NewClient(WithStoreRootPath(tempDir))
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		// Create a buffer to capture progress output
		var progressBuffer bytes.Buffer

		// Test with non-existent model
		nonExistentRef := registry + "/nonexistent/model:v1.0.0"
		err = client.PullModel(context.Background(), nonExistentRef, &progressBuffer)
		if err == nil {
			t.Fatal("Expected error for non-existent model, got nil")
		}

		// Verify it's a PullError
		var pullErr *PullError
		ok := errors.As(err, &pullErr)
		if !ok {
			t.Fatalf("Expected PullError, got %T", err)
		}

		// Verify error fields
		if pullErr.Reference != nonExistentRef {
			t.Errorf("Expected reference %q, got %q", nonExistentRef, pullErr.Reference)
		}
		if pullErr.Code != "MANIFEST_UNKNOWN" {
			t.Errorf("Expected error code MANIFEST_UNKNOWN, got %q", pullErr.Code)
		}
		if pullErr.Message != "Model not found" {
			t.Errorf("Expected message 'Model not found', got %q", pullErr.Message)
		}
		if pullErr.Err == nil {
			t.Error("Expected underlying error to be non-nil")
		}
	})

	t.Run("pull with incomplete files", func(t *testing.T) {
		// Create temp directory for store
		tempDir, err := os.MkdirTemp("", "model-distribution-incomplete-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp directory: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Create client
		client, err := NewClient(WithStoreRootPath(tempDir))
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		// Use the dummy.gguf file from assets directory
		modelFile := filepath.Join("..", "..", "assets", "dummy.gguf")
		mdl, err := gguf.NewModel(modelFile)
		if err != nil {
			t.Fatalf("Failed to create model: %v", err)
		}

		// Push model to local store
		tag := registry + "/incomplete-test/model:v1.0.0"
		if err := client.store.Write(mdl, []string{tag}, nil); err != nil {
			t.Fatalf("Failed to push model to store: %v", err)
		}

		// Push model to registry
		if err := client.PushModel(context.Background(), modelFile, tag); err != nil {
			t.Fatalf("Failed to pull model: %v", err)
		}

		// Get the model to find the GGUF path
		model, err := client.GetModel(tag)
		if err != nil {
			t.Fatalf("Failed to get model: %v", err)
		}

		ggufPath, err := model.GGUFPath()
		if err != nil {
			t.Fatalf("Failed to get GGUF path: %v", err)
		}

		// Create an incomplete file by copying the GGUF file and adding .incomplete suffix
		incompletePath := ggufPath + ".incomplete"
		originalContent, err := os.ReadFile(ggufPath)
		if err != nil {
			t.Fatalf("Failed to read GGUF file: %v", err)
		}

		// Write partial content to simulate an incomplete download
		partialContent := originalContent[:len(originalContent)/2]
		if err := os.WriteFile(incompletePath, partialContent, 0644); err != nil {
			t.Fatalf("Failed to create incomplete file: %v", err)
		}

		// Verify the incomplete file exists
		if _, err := os.Stat(incompletePath); os.IsNotExist(err) {
			t.Fatalf("Failed to create incomplete file: %v", err)
		}

		// Delete the local model to force a pull
		if err := client.DeleteModel(tag); err != nil {
			t.Fatalf("Failed to delete model: %v", err)
		}

		// Create a buffer to capture progress output
		var progressBuffer bytes.Buffer

		// Pull the model again - this should detect the incomplete file and pull again
		if err := client.PullModel(context.Background(), tag, &progressBuffer); err != nil {
			t.Fatalf("Failed to pull model: %v", err)
		}

		// Verify progress output indicates a new download, not using cached model
		progressOutput := progressBuffer.String()
		if strings.Contains(progressOutput, "Using cached model") {
			t.Errorf("Expected to pull model again due to incomplete file, but used cached model")
		}

		// Verify the incomplete file no longer exists
		if _, err := os.Stat(incompletePath); !os.IsNotExist(err) {
			t.Errorf("Incomplete file still exists after successful pull: %s", incompletePath)
		}

		// Verify the complete file exists
		if _, err := os.Stat(ggufPath); os.IsNotExist(err) {
			t.Errorf("GGUF file doesn't exist after pull: %s", ggufPath)
		}

		// Verify the content of the pulled file matches the original
		pulledContent, err := os.ReadFile(ggufPath)
		if err != nil {
			t.Fatalf("Failed to read pulled GGUF file: %v", err)
		}

		if !bytes.Equal(pulledContent, originalContent) {
			t.Errorf("Pulled content doesn't match original content")
		}
	})

	t.Run("pull updated model with same tag", func(t *testing.T) {
		// Create temp directory for store
		tempDir, err := os.MkdirTemp("", "model-distribution-update-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp directory: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Create client
		client, err := NewClient(WithStoreRootPath(tempDir))
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		// Use the dummy.gguf file from assets directory for first version
		modelFile := filepath.Join("..", "..", "assets", "dummy.gguf")

		// Read model content for verification later
		modelContent, err := os.ReadFile(modelFile)
		if err != nil {
			t.Fatalf("Failed to read test model file: %v", err)
		}

		// Push first version of model to registry
		tag := registry + "/update-test:v1.0.0"
		if err := client.PushModel(context.Background(), modelFile, tag); err != nil {
			t.Fatalf("Failed to push first version of model: %v", err)
		}

		// Pull first version of model
		if err := client.PullModel(context.Background(), tag, nil); err != nil {
			t.Fatalf("Failed to pull first version of model: %v", err)
		}

		// Verify first version is in local store
		model, err := client.GetModel(tag)
		if err != nil {
			t.Fatalf("Failed to get first version of model: %v", err)
		}

		modelPath, err := model.GGUFPath()
		if err != nil {
			t.Fatalf("Failed to get model path: %v", err)
		}

		// Verify first version content
		pulledContent, err := os.ReadFile(modelPath)
		if err != nil {
			t.Fatalf("Failed to read pulled model: %v", err)
		}

		if string(pulledContent) != string(modelContent) {
			t.Errorf("Pulled model content doesn't match original: got %q, want %q", pulledContent, modelContent)
		}

		// Create a modified version of the model
		updatedModelFile := filepath.Join(tempDir, "updated-dummy.gguf")
		updatedContent := append(modelContent, []byte("UPDATED CONTENT")...)
		if err := os.WriteFile(updatedModelFile, updatedContent, 0644); err != nil {
			t.Fatalf("Failed to create updated model file: %v", err)
		}

		// Push updated model with same tag
		if err := client.PushModel(context.Background(), updatedModelFile, tag); err != nil {
			t.Fatalf("Failed to push updated model: %v", err)
		}

		// Create a buffer to capture progress output
		var progressBuffer bytes.Buffer

		// Pull model again - should get the updated version
		if err := client.PullModel(context.Background(), tag, &progressBuffer); err != nil {
			t.Fatalf("Failed to pull updated model: %v", err)
		}

		// Verify progress output indicates a new download, not using cached model
		progressOutput := progressBuffer.String()
		if strings.Contains(progressOutput, "Using cached model") {
			t.Errorf("Expected to pull updated model, but used cached model")
		}

		// Get the model again to verify it's the updated version
		updatedModel, err := client.GetModel(tag)
		if err != nil {
			t.Fatalf("Failed to get updated model: %v", err)
		}

		updatedModelPath, err := updatedModel.GGUFPath()
		if err != nil {
			t.Fatalf("Failed to get updated model path: %v", err)
		}

		// Verify updated content
		updatedPulledContent, err := os.ReadFile(updatedModelPath)
		if err != nil {
			t.Fatalf("Failed to read updated pulled model: %v", err)
		}

		if string(updatedPulledContent) != string(updatedContent) {
			t.Errorf("Updated pulled model content doesn't match: got %q, want %q", updatedPulledContent, updatedContent)
		}
	})
}

func TestClientGetModel(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Use the dummy.gguf file from assets directory
	modelFile := filepath.Join("..", "..", "assets", "dummy.gguf")

	model, err := gguf.NewModel(modelFile)
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}

	// Push model to local store
	tag := "test/model:v1.0.0"
	if err := client.store.Write(model, []string{tag}, nil); err != nil {
		t.Fatalf("Failed to push model to store: %v", err)
	}

	// Get model
	mi, err := client.GetModel(tag)
	if err != nil {
		t.Fatalf("Failed to get model: %v", err)
	}

	// Verify model
	if len(mi.Tags()) == 0 || mi.Tags()[0] != tag {
		t.Errorf("Model tags don't match: got %v, want [%s]", mi.Tags(), tag)
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
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Get non-existent model
	_, err = client.GetModel("nonexistent/model:v1.0.0")
	if !errors.Is(err, ErrModelNotFound) {
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
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create test model file
	modelContent := []byte("test model content")
	modelFile := filepath.Join(tempDir, "test-model.gguf")
	if err := os.WriteFile(modelFile, modelContent, 0644); err != nil {
		t.Fatalf("Failed to write test model file: %v", err)
	}

	mdl, err := gguf.NewModel(modelFile)
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}

	// Push models to local store with different manifest digests
	// First model
	tag1 := "test/model1:v1.0.0"
	if err := client.store.Write(mdl, []string{tag1}, nil); err != nil {
		t.Fatalf("Failed to push model to store: %v", err)
	}

	// Create a slightly different model file for the second model
	modelContent2 := []byte("test model content 2")
	modelFile2 := filepath.Join(tempDir, "test-model2.gguf")
	if err := os.WriteFile(modelFile2, modelContent2, 0644); err != nil {
		t.Fatalf("Failed to write test model file: %v", err)
	}
	mdl2, err := gguf.NewModel(modelFile2)
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}

	// Second model
	tag2 := "test/model2:v1.0.0"
	if err := client.store.Write(mdl2, []string{tag2}, nil); err != nil {
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
		for _, tag := range model.Tags() {
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
	client, err := NewClient(WithStoreRootPath(tempDir))
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
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Use the dummy.gguf file from assets directory
	mdl, err := gguf.NewModel(filepath.Join("..", "..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}

	// Push model to local store
	tag := "test/model:v1.0.0"
	if err := client.store.Write(mdl, []string{tag}, nil); err != nil {
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

func TestClientDefaultLogger(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client without specifying logger
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Verify that logger is not nil
	if client.log == nil {
		t.Error("Default logger should not be nil")
	}

	// Create client with custom logger
	customLogger := logrus.NewEntry(logrus.New())
	client, err = NewClient(
		WithStoreRootPath(tempDir),
		WithLogger(customLogger),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Verify that custom logger is used
	if client.log != customLogger {
		t.Error("Custom logger should be used when specified")
	}
}

func TestNewReferenceError(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test with invalid reference
	invalidRef := "invalid:reference:format"
	err = client.PullModel(context.Background(), invalidRef, nil)
	if err == nil {
		t.Fatal("Expected error for invalid reference, got nil")
	}

	// Verify it's a ReferenceError
	refErr, ok := err.(*ReferenceError)
	if !ok {
		t.Fatalf("Expected ReferenceError, got %T", err)
	}

	// Verify error fields
	if refErr.Reference != invalidRef {
		t.Errorf("Expected reference %q, got %q", invalidRef, refErr.Reference)
	}
	if refErr.Err == nil {
		t.Error("Expected underlying error to be non-nil")
	}
}
