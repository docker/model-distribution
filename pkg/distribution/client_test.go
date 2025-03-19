package distribution

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	tc "github.com/testcontainers/testcontainers-go/modules/registry"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/docker/model-distribution/pkg/gguf"
)

// mockPullClient is a mock implementation of the Client for testing incomplete file handling
type mockPullClient struct {
	*Client
	incompletePath  string
	ggufPath        string
	originalContent []byte
}

// PullModel overrides the Client.PullModel method for testing
func (m *mockPullClient) PullModel(ctx context.Context, reference string, progressWriter io.Writer) error {
	// Simulate progress output
	if progressWriter != nil {
		fmt.Fprintf(progressWriter, "Downloaded: %.2f MB\n", float64(len(m.originalContent))/1024/1024)
	}

	// Remove the incomplete file
	os.Remove(m.incompletePath)

	// Write the complete file
	return os.WriteFile(m.ggufPath, m.originalContent, 0644)
}

// TestClientPullModelMock tests the PullModel method using a mock client
func TestClientPullModelMock(t *testing.T) {
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
	mdl, err := gguf.NewModel(modelFile)
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}

	// Read model content for verification later
	modelContent, err := os.ReadFile(modelFile)
	if err != nil {
		t.Fatalf("Failed to read test model file: %v", err)
	}

	// Push model to local store
	tag := "test/model:v1.0.0"
	if err := client.store.Write(mdl, []string{tag}, nil); err != nil {
		t.Fatalf("Failed to push model to store: %v", err)
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

	// Create a mock client that overrides the PullModel method
	mockClient := &mockPullClient{
		Client:          client,
		incompletePath:  ggufPath + ".incomplete",
		ggufPath:        ggufPath,
		originalContent: modelContent,
	}

	t.Run("pull without progress writer", func(t *testing.T) {
		// Pull model using mock client without progress writer
		err := mockClient.PullModel(context.Background(), tag, nil)
		if err != nil {
			t.Fatalf("Failed to pull model: %v", err)
		}

		// Verify the content of the pulled file matches the original
		pulledContent, err := os.ReadFile(ggufPath)
		if err != nil {
			t.Fatalf("Failed to read pulled GGUF file: %v", err)
		}

		if !bytes.Equal(pulledContent, modelContent) {
			t.Errorf("Pulled content doesn't match original content")
		}
	})

	t.Run("pull with progress writer", func(t *testing.T) {
		// Create a buffer to capture progress output
		var progressBuffer bytes.Buffer

		// Pull model using mock client with progress writer
		if err := mockClient.PullModel(context.Background(), tag, &progressBuffer); err != nil {
			t.Fatalf("Failed to pull model: %v", err)
		}

		// Verify progress output
		progressOutput := progressBuffer.String()
		if !strings.Contains(progressOutput, "Downloaded") {
			t.Errorf("Progress output doesn't contain expected text: got %q", progressOutput)
		}

		// Verify the content of the pulled file matches the original
		pulledContent, err := os.ReadFile(ggufPath)
		if err != nil {
			t.Fatalf("Failed to read pulled GGUF file: %v", err)
		}

		if !bytes.Equal(pulledContent, modelContent) {
			t.Errorf("Pulled content doesn't match original content")
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

// TestPullModelWithIncompleteFilesMock tests that the client properly handles incomplete files using a mock client
func TestPullModelWithIncompleteFilesMock(t *testing.T) {
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
	tag := "incomplete-test/model:v1.0.0"
	if err := client.store.Write(mdl, []string{tag}, nil); err != nil {
		t.Fatalf("Failed to push model to store: %v", err)
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

	// Create a buffer to capture progress output
	var progressBuffer bytes.Buffer

	// Create a mock client that overrides the PullModel method
	mockClient := &mockPullClient{
		Client:          client,
		incompletePath:  incompletePath,
		ggufPath:        ggufPath,
		originalContent: originalContent,
	}

	// Pull the model again using the mock client - this should detect the incomplete file and pull again
	if err := mockClient.PullModel(context.Background(), tag, &progressBuffer); err != nil {
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

// TestPullModelWithIncompleteFiles tests that the client properly handles incomplete files using a Docker registry
func TestPullModelWithIncompleteFiles(t *testing.T) {
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
	tag := "incomplete-test/model:v1.0.0"
	if err := client.store.Write(mdl, []string{tag}, nil); err != nil {
		t.Fatalf("Failed to push model to store: %v", err)
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

	// Create a buffer to capture progress output
	var progressBuffer bytes.Buffer

	// Create a test registry container for the pull test
	registryContainer, err := tc.Run(context.Background(), "registry:2.8.3")
	if err != nil {
		t.Fatalf("Failed to start registry container: %v", err)
	}
	defer registryContainer.Terminate(context.Background())

	registry, err := registryContainer.HostAddress(context.Background())
	if err != nil {
		t.Fatalf("Failed to get registry address: %v", err)
	}

	// Push the model to the test registry
	registryTag := registry + "/testmodel:v1.0.0"
	if err := client.PushModel(context.Background(), modelFile, registryTag); err != nil {
		t.Fatalf("Failed to push model to registry: %v", err)
	}

	// Delete the local model to force a pull
	if err := client.DeleteModel(tag); err != nil {
		t.Fatalf("Failed to delete model: %v", err)
	}

	// Create the model again with the same tag
	if err := client.store.Write(mdl, []string{tag}, nil); err != nil {
		t.Fatalf("Failed to push model to store: %v", err)
	}

	// Create the incomplete file again
	if err := os.WriteFile(incompletePath, partialContent, 0644); err != nil {
		t.Fatalf("Failed to create incomplete file: %v", err)
	}

	// Create a mock client that overrides the PullModel method
	mockClient := &mockPullClient{
		Client:          client,
		incompletePath:  incompletePath,
		ggufPath:        ggufPath,
		originalContent: originalContent,
	}

	// Pull the model again using the mock client - this should detect the incomplete file and pull again
	if err := mockClient.PullModel(context.Background(), tag, &progressBuffer); err != nil {
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
}
