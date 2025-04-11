package distribution

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sirupsen/logrus"
	tc "github.com/testcontainers/testcontainers-go/modules/registry"

	"github.com/docker/model-distribution/internal/gguf"
	"github.com/docker/model-distribution/internal/mutate"
)

var (
	testGGUFFile = filepath.Join("..", "assets", "dummy.gguf")
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

	// Read model content for verification later
	modelContent, err := os.ReadFile(testGGUFFile)
	if err != nil {
		t.Fatalf("Failed to read test model file: %v", err)
	}

	model, err := gguf.NewModel(testGGUFFile)
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}
	tag := registry + "/testmodel:v1.0.0"
	ref, err := name.ParseReference(tag)
	if err != nil {
		t.Fatalf("Failed to parse reference: %v", err)
	}
	if err := remote.Write(ref, model); err != nil {
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
		mdl, err := gguf.NewModel(testGGUFFile)
		if err != nil {
			t.Fatalf("Failed to create model: %v", err)
		}

		// Push model to local store
		tag := registry + "/incomplete-test/model:v1.0.0"
		if err := client.store.Write(mdl, []string{tag}, nil); err != nil {
			t.Fatalf("Failed to push model to store: %v", err)
		}

		// Push model to registry
		if err := client.PushModel(context.Background(), testGGUFFile, tag); err != nil {
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

		// Read model content for verification later
		modelContent, err := os.ReadFile(testGGUFFile)
		if err != nil {
			t.Fatalf("Failed to read test model file: %v", err)
		}

		// Push first version of model to registry
		tag := registry + "/update-test:v1.0.0"
		if err := client.PushModel(context.Background(), testGGUFFile, tag); err != nil {
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

	t.Run("pull unsupported (newer) version", func(t *testing.T) {
		newMdl := mutate.ConfigMediaType(model, "application/vnd.docker.ai.model.config.v0.2+json")
		// Push model to local store
		tag := registry + "/unsupported-test/model:v1.0.0"
		ref, err := name.ParseReference(tag)
		if err != nil {
			t.Fatalf("Failed to parse reference: %v", err)
		}
		if err := remote.Write(ref, newMdl); err != nil {
			t.Fatalf("Failed to push model: %v", err)
		}
		if err := client.PullModel(context.Background(), tag, nil); err == nil || !errors.Is(err, ErrUnsupportedMediaType) {
			t.Fatalf("Expected artifact version error, got %v", err)
		}
	})

	t.Run("pull with JSON progress messages", func(t *testing.T) {
		// Create temp directory for store
		tempDir, err := os.MkdirTemp("", "model-distribution-json-test-*")
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

		// Pull model from registry with progress writer
		if err := client.PullModel(context.Background(), tag, &progressBuffer); err != nil {
			t.Fatalf("Failed to pull model: %v", err)
		}

		// Parse progress output as JSON
		var messages []ProgressMessage
		scanner := bufio.NewScanner(&progressBuffer)
		for scanner.Scan() {
			line := scanner.Text()
			var msg ProgressMessage
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				t.Fatalf("Failed to parse JSON progress message: %v, line: %s", err, line)
			}
			messages = append(messages, msg)
		}

		if err := scanner.Err(); err != nil {
			t.Fatalf("Error reading progress output: %v", err)
		}

		// Verify we got some messages
		if len(messages) == 0 {
			t.Fatal("No progress messages received")
		}

		// Check the last message is a success message
		lastMsg := messages[len(messages)-1]
		if lastMsg.Type != "success" {
			t.Errorf("Expected last message to be success, got type: %s, message: %s", lastMsg.Type, lastMsg.Message)
		}

		// Verify model was pulled correctly
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
			t.Errorf("Pulled model content doesn't match original")
		}
	})

	t.Run("pull with error and JSON progress messages", func(t *testing.T) {
		// Create temp directory for store
		tempDir, err := os.MkdirTemp("", "model-distribution-json-error-test-*")
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

		// Expect an error
		if err == nil {
			t.Fatal("Expected error for non-existent model, got nil")
		}

		// Verify it's a PullError
		var pullErr *PullError
		if !errors.As(err, &pullErr) {
			t.Fatalf("Expected PullError, got %T", err)
		}

		// No JSON messages should be in the buffer for this error case
		// since the error happens before we start streaming progress
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

	// Create model from test GGUF file
	model, err := gguf.NewModel(testGGUFFile)
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
	mdl, err := gguf.NewModel(testGGUFFile)
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

func TestClientDeleteNonexistentModel(t *testing.T) {
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

	// Delete the model
	if err := client.DeleteModel("some/missing:model"); !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("Should return ErrModelNotFound got: %v", err)
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

func TestWithFunctionsNilChecks(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test WithStoreRootPath with empty string
	t.Run("WithStoreRootPath empty string", func(t *testing.T) {
		// Create options with a valid path first
		opts := defaultOptions()
		WithStoreRootPath(tempDir)(opts)

		// Then try to override with empty string
		WithStoreRootPath("")(opts)

		// Verify the path wasn't changed to empty
		if opts.storeRootPath != tempDir {
			t.Errorf("WithStoreRootPath with empty string changed the path: got %q, want %q",
				opts.storeRootPath, tempDir)
		}
	})

	// Test WithLogger with nil
	t.Run("WithLogger nil", func(t *testing.T) {
		// Create options with default logger
		opts := defaultOptions()
		defaultLogger := opts.logger

		// Try to override with nil
		WithLogger(nil)(opts)

		// Verify the logger wasn't changed to nil
		if opts.logger == nil {
			t.Error("WithLogger with nil changed logger to nil")
		}

		// Verify it's still the default logger
		if opts.logger != defaultLogger {
			t.Error("WithLogger with nil changed the logger")
		}
	})

	// Test WithTransport with nil
	t.Run("WithTransport nil", func(t *testing.T) {
		// Create options with default transport
		opts := defaultOptions()
		defaultTransport := opts.transport

		// Try to override with nil
		WithTransport(nil)(opts)

		// Verify the transport wasn't changed to nil
		if opts.transport == nil {
			t.Error("WithTransport with nil changed transport to nil")
		}

		// Verify it's still the default transport
		if opts.transport != defaultTransport {
			t.Error("WithTransport with nil changed the transport")
		}
	})

	// Test WithUserAgent with empty string
	t.Run("WithUserAgent empty string", func(t *testing.T) {
		// Create options with default user agent
		opts := defaultOptions()
		defaultUA := opts.userAgent

		// Try to override with empty string
		WithUserAgent("")(opts)

		// Verify the user agent wasn't changed to empty
		if opts.userAgent == "" {
			t.Error("WithUserAgent with empty string changed user agent to empty")
		}

		// Verify it's still the default user agent
		if opts.userAgent != defaultUA {
			t.Errorf("WithUserAgent with empty string changed the user agent: got %q, want %q",
				opts.userAgent, defaultUA)
		}
	})
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
