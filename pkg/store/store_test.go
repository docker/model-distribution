package store_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/docker/model-distribution/pkg/store"
	"github.com/docker/model-distribution/pkg/types"
)

// TestStoreAPI tests the store API directly
func TestStoreAPI(t *testing.T) {
	// Create a temporary directory for the test store
	tempDir, err := os.MkdirTemp("", "store-api-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a temporary model file with known content
	modelContent := []byte("test model content for API test")
	modelPath := filepath.Join(tempDir, "api-test-model.gguf")
	if err := os.WriteFile(modelPath, modelContent, 0644); err != nil {
		t.Fatalf("Failed to create test model file: %v", err)
	}

	// Calculate expected blob hash
	hash := sha256.Sum256(modelContent)
	expectedBlobHash := fmt.Sprintf("sha256:%s", hex.EncodeToString(hash[:]))

	// Create store
	storePath := filepath.Join(tempDir, "api-model-store")
	s, err := store.New(types.StoreOptions{
		RootPath: storePath,
	})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Test Push
	t.Run("Push", func(t *testing.T) {
		err := s.Push(modelPath, []string{"api-model:latest"})
		if err != nil {
			t.Fatalf("Push failed: %v", err)
		}

		// Verify the model was stored correctly
		models, err := s.List()
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(models) != 1 {
			t.Fatalf("Expected 1 model, got %d", len(models))
		}
		if len(models[0].Files) != 1 {
			t.Fatalf("Expected 1 file, got %d", len(models[0].Files))
		}
		if models[0].Files[0] != expectedBlobHash {
			t.Errorf("Expected blob hash %s, got %s", expectedBlobHash, models[0].Files[0])
		}
	})

	// Test List
	t.Run("List", func(t *testing.T) {
		models, err := s.List()
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(models) != 1 {
			t.Fatalf("Expected 1 model, got %d", len(models))
		}
		if !containsTag(models[0].Tags, "api-model:latest") {
			t.Errorf("Expected tag api-model:latest, got %v", models[0].Tags)
		}
		if models[0].Files[0] != expectedBlobHash {
			t.Errorf("Expected blob hash %s, got %s", expectedBlobHash, models[0].Files[0])
		}
	})

	// Test GetByTag
	t.Run("GetByTag", func(t *testing.T) {
		model, err := s.GetByTag("api-model:latest")
		if err != nil {
			t.Fatalf("GetByTag failed: %v", err)
		}
		if model == nil {
			t.Fatalf("Expected model, got nil")
		}
		if !containsTag(model.Tags, "api-model:latest") {
			t.Errorf("Expected tag api-model:latest, got %v", model.Tags)
		}
		if model.Files[0] != expectedBlobHash {
			t.Errorf("Expected blob hash %s, got %s", expectedBlobHash, model.Files[0])
		}
	})

	// Test AddTags
	t.Run("AddTags", func(t *testing.T) {
		err := s.AddTags("api-model:latest", []string{"api-v1.0", "api-stable"})
		if err != nil {
			t.Fatalf("AddTags failed: %v", err)
		}

		// Verify tags were added
		model, err := s.GetByTag("api-model:latest")
		if err != nil {
			t.Fatalf("GetByTag failed: %v", err)
		}
		if !containsTag(model.Tags, "api-v1.0") || !containsTag(model.Tags, "api-stable") {
			t.Errorf("Expected new tags, got %v", model.Tags)
		}
		if model.Files[0] != expectedBlobHash {
			t.Errorf("Expected blob hash %s, got %s", expectedBlobHash, model.Files[0])
		}
	})

	// Test Pull
	t.Run("Pull", func(t *testing.T) {
		pulledPath := filepath.Join(tempDir, "api-pulled-model.gguf")
		err := s.Pull("api-model:latest", pulledPath)
		if err != nil {
			t.Fatalf("Pull failed: %v", err)
		}

		// Verify pulled content
		pulledContent, err := os.ReadFile(pulledPath)
		if err != nil {
			t.Fatalf("Failed to read pulled file: %v", err)
		}
		if !bytes.Equal(pulledContent, modelContent) {
			t.Errorf("Pulled content doesn't match original")
		}

		// Verify blob path exists
		blobPath := filepath.Join(storePath, "blobs", "sha256", hex.EncodeToString(hash[:]))
		if _, err := os.Stat(blobPath); os.IsNotExist(err) {
			t.Errorf("Blob file does not exist at expected path: %s", blobPath)
		}
	})

	// Test RemoveTags
	t.Run("RemoveTags", func(t *testing.T) {
		err := s.RemoveTags([]string{"api-model:api-v1.0"})
		if err != nil {
			t.Fatalf("RemoveTags failed: %v", err)
		}

		// Verify tag was removed
		models, err := s.List()
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		for _, model := range models {
			if containsTag(model.Tags, "api-model:api-v1.0") {
				t.Errorf("Tag should have been removed, but still present: %v", model.Tags)
			}
			if model.Files[0] != expectedBlobHash {
				t.Errorf("Expected blob hash %s, got %s", expectedBlobHash, model.Files[0])
			}
		}
	})

	// Test Delete
	t.Run("Delete", func(t *testing.T) {
		err := s.Delete("api-model:latest")
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		// Verify model with that tag is gone
		_, err = s.GetByTag("api-model:latest")
		if err == nil {
			t.Errorf("Expected error after deletion, got nil")
		}
	})
}

// Helper function to run the model-distribution command
func runCommand(binaryPath string, args ...string) (string, error) {
	cmd := exec.Command(binaryPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Sprintf("stdout: %s\nstderr: %s", stdout.String(), stderr.String()), err
	}
	return stdout.String(), nil
}

// Helper function to check if a tag is in a slice of tags
func containsTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}
