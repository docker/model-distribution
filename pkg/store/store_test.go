package store_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/model-distribution/pkg/gguf"
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

	t.Run("Read/Write", func(t *testing.T) {
		mdl1, err := gguf.NewModel(modelPath)
		if err != nil {
			t.Fatalf("Create model failed: %v", err)
		}
		writeDigest, err := mdl1.Digest()
		if err != nil {
			t.Fatalf("Digest failed: %v", err)
		}
		if err := s.Write(mdl1, []string{"api-model:latest"}, nil); err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		mdl2, err := s.Read("api-model:latest")
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
		readDigest, err := mdl2.Digest()
		if err != nil {
			t.Fatalf("Digest failed: %v", err)
		}
		if writeDigest != readDigest {
			t.Fatalf("Digest mismatch %s != %s", writeDigest.Hex, readDigest.Hex)
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

	// Test AddTags
	t.Run("AddTags", func(t *testing.T) {
		err := s.AddTags("api-model:latest", []string{"api-v1.0", "api-stable"})
		if err != nil {
			t.Fatalf("AddTags failed: %v", err)
		}

		// Verify tags were added to model
		model, err := s.Read("api-model:latest")
		if err != nil {
			t.Fatalf("GetByTag failed: %v", err)
		}
		if !containsTag(model.Tags(), "api-v1.0") || !containsTag(model.Tags(), "api-stable") {
			t.Errorf("Expected new tags, got %v", model.Tags())
		}

		// Verify tags were added to list
		models, err := s.List()
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(models) != 1 {
			t.Fatalf("Expected 1 model, got %d", len(models))
		}
		if len(models[0].Tags) != 3 {
			t.Fatalf("Expected 3 tags, got %d", len(models[0].Tags))
		}
	})

	// Test RemoveTags
	t.Run("RemoveTags", func(t *testing.T) {
		err := s.RemoveTags([]string{"api-model:api-v1.0"})
		if err != nil {
			t.Fatalf("RemoveTags failed: %v", err)
		}

		// Verify tag was removed from list
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

		// Verify read by tag fails
		if _, err = s.Read("api-model:api-v1.0"); err == nil {
			t.Errorf("Expected read error after tag removal, got nil")
		}
	})

	// Test Delete
	t.Run("Delete", func(t *testing.T) {
		err := s.Delete("api-model:latest")
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		// Verify model with that tag is gone
		_, err = s.Read("api-model:latest")
		if err == nil {
			t.Errorf("Expected error after deletion, got nil")
		}
	})

	// Test that Delete removes the blob files
	t.Run("DeleteRemovesBlobs", func(t *testing.T) {
		// Create a new model with unique content
		modelContent := []byte("unique content for blob deletion test")
		modelPath := filepath.Join(tempDir, "blob-deletion-test.gguf")
		if err := os.WriteFile(modelPath, modelContent, 0644); err != nil {
			t.Fatalf("Failed to create test model file: %v", err)
		}

		// Calculate the blob hash to find it later
		hash := sha256.Sum256(modelContent)
		blobHash := hex.EncodeToString(hash[:])

		// Add model to store with a unique tag
		mdl, err := gguf.NewModel(modelPath)
		if err != nil {
			t.Fatalf("Create model failed: %v", err)
		}

		if err := s.Write(mdl, []string{"blob-test:latest"}, nil); err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		// Get the blob path
		blobPath := filepath.Join(storePath, "blobs", "sha256", blobHash)

		// Verify the blob exists on disk before deletion
		if _, err := os.Stat(blobPath); os.IsNotExist(err) {
			t.Fatalf("Blob file doesn't exist before deletion: %s", blobPath)
		}

		// Delete the model
		if err := s.Delete("blob-test:latest"); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		// Verify the blob no longer exists on disk after deletion
		if _, err := os.Stat(blobPath); !os.IsNotExist(err) {
			t.Errorf("Blob file still exists after deletion: %s", blobPath)
		}
	})
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
