package store_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/model-distribution/internal/gguf"
	"github.com/docker/model-distribution/internal/mutate"
	"github.com/docker/model-distribution/internal/partial"
	"github.com/docker/model-distribution/internal/store"
	"github.com/docker/model-distribution/types"
)

// TestStoreAPI tests the store API directly
func TestStoreAPI(t *testing.T) {
	// Create a temporary directory for the test store
	tempDir, err := os.MkdirTemp("", "store-api-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create store
	storePath := filepath.Join(tempDir, "api-model-store")
	s, err := store.New(store.Options{
		RootPath: storePath,
	})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	// Everything must handle directory deletion
	if err := os.RemoveAll(storePath); err != nil {
		t.Fatalf("Failed to remove store directory: %v", err)
	}

	model := newTestModel(t)
	layers, err := model.Layers()
	if err != nil {
		t.Fatalf("Failed to get layers: %v", err)
	}
	ggufDiffID, err := layers[0].DiffID()
	if err != nil {
		t.Fatalf("Failed to get diff ID: %v", err)
	}
	expectedBlobHash := ggufDiffID.String()

	digest, err := model.Digest()
	if err != nil {
		t.Fatalf("Digest failed: %v", err)
	}
	if err := s.Write(model, []string{"api-model:latest"}, nil); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	t.Run("ReadByTag", func(t *testing.T) {
		mdl2, err := s.Read("api-model:latest")
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
		readDigest, err := mdl2.Digest()
		if err != nil {
			t.Fatalf("Digest failed: %v", err)
		}
		if digest != readDigest {
			t.Fatalf("Digest mismatch %s != %s", digest.Hex, readDigest.Hex)
		}
	})

	t.Run("ReadByID", func(t *testing.T) {
		id, err := model.ID()
		if err != nil {
			t.Fatalf("ID failed: %v", err)
		}
		mdl2, err := s.Read(id)
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
		readDigest, err := mdl2.Digest()
		if err != nil {
			t.Fatalf("Digest failed: %v", err)
		}
		if digest != readDigest {
			t.Fatalf("Digest mismatch %s != %s", digest.Hex, readDigest.Hex)
		}
		if !containsTag(mdl2.Tags(), "api-model:latest") {
			t.Errorf("Expected tag api-model:latest, got %v", mdl2.Tags())
		}

	})

	t.Run("ReadNotFound", func(t *testing.T) {
		if _, err := s.Read("non-existent-model:latest"); !errors.Is(err, store.ErrModelNotFound) {
			t.Fatalf("Expected ErrModelNotFound got: %v", err)
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
		if len(models[0].Files) != 3 {
			t.Fatalf("Expected 3 files (gguf, license, config), got %d", len(models[0].Files))
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

	// Test Delete Non Existent Model
	t.Run("Delete", func(t *testing.T) {
		err := s.Delete("non-existent-model:latest")
		if !errors.Is(err, store.ErrModelNotFound) {
			t.Fatalf("Expected ErrModelNotFound, got %v", err)
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
		if _, err := os.Stat(blobPath); err != nil {
			t.Fatalf("Failed to stat blob at path '%s': %v", blobPath, err)
		}

		// Get the manifest path
		digest, err := mdl.Digest()
		if err != nil {
			t.Fatalf("Failed to get digest: %v", err)
		}

		// Verify the model manifest exists
		manifestPath := filepath.Join(storePath, "manifests", "sha256", digest.Hex)
		if _, err := os.Stat(manifestPath); err != nil {
			t.Fatalf("Failed to stat manifest at path '%s': %v", manifestPath, err)
		}

		// Delete the model
		if err := s.Delete("blob-test:latest"); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		// Verify the blob no longer exists on disk after deletion
		if _, err := os.Stat(blobPath); !os.IsNotExist(err) {
			t.Errorf("Blob file still exists after deletion: %s", blobPath)
		}

		// Verify the manifest no longer exists on disk after deletion
		if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
			t.Errorf("Manifest file still exists after deletion: %s", blobPath)
		}
	})

	// Test that blobs and model are not removed if there is a tag pointing to it
	t.Run("BlobsPreservedWithRemainingTags", func(t *testing.T) {
		// Create a new model with unique content
		modelContent := []byte("unique content for multi-tag test")
		modelPath := filepath.Join(tempDir, "multi-tag-test.gguf")
		if err := os.WriteFile(modelPath, modelContent, 0644); err != nil {
			t.Fatalf("Failed to create test model file: %v", err)
		}

		// Calculate the blob hash to find it later
		hash := sha256.Sum256(modelContent)
		blobHash := hex.EncodeToString(hash[:])
		expectedBlobDigest := fmt.Sprintf("sha256:%s", blobHash)

		// Add model to store with multiple tags
		mdl, err := gguf.NewModel(modelPath)
		if err != nil {
			t.Fatalf("Create model failed: %v", err)
		}

		// Write the model with two tags
		if err := s.Write(mdl, []string{"multi-tag:v1", "multi-tag:latest"}, nil); err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		// Get the blob path
		blobPath := filepath.Join(storePath, "blobs", "sha256", blobHash)

		// Verify the blob exists on disk
		if _, err := os.Stat(blobPath); os.IsNotExist(err) {
			t.Fatalf("Blob file doesn't exist: %s", blobPath)
		}

		// Delete one of the tags
		if err := s.Delete("multi-tag:v1"); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		// Verify the blob still exists on disk after deleting one tag
		if _, err := os.Stat(blobPath); os.IsNotExist(err) {
			t.Errorf("Blob file was incorrectly removed: %s", blobPath)
		}

		// Verify the model is still in the index with the remaining tag
		models, err := s.List()
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}

		var foundModel bool
		for _, model := range models {
			if containsTag(model.Tags, "multi-tag:latest") {
				foundModel = true
				// Verify the blob is still associated with the model
				if len(model.Files) != 2 || model.Files[0] != expectedBlobDigest {
					t.Errorf("Expected blob %s, got %v", expectedBlobDigest, model.Files)
				}
				break
			}
		}

		if !foundModel {
			t.Errorf("Model with tag multi-tag:latest not found after deleting multi-tag:v1")
		}

		// Verify the model can still be read using the remaining tag
		remainingModel, err := s.Read("multi-tag:latest")
		if err != nil {
			t.Fatalf("Read failed for remaining tag: %v", err)
		}

		if remainingModel == nil {
			t.Fatalf("Model is nil despite having a remaining tag")
		}

		// Verify the remaining tag is present in the model
		if !containsTag(remainingModel.Tags(), "multi-tag:latest") {
			t.Errorf("Expected tag multi-tag:latest in model tags, got %v", remainingModel.Tags())
		}
	})

	// Test that shared blobs between different models are not deleted
	t.Run("SharedBlobsPreservation", func(t *testing.T) {
		// Create a model file with content that will be shared
		sharedContent := []byte("shared content for multiple models test")
		sharedModelPath := filepath.Join(tempDir, "shared-model.gguf")
		if err := os.WriteFile(sharedModelPath, sharedContent, 0644); err != nil {
			t.Fatalf("Failed to create shared model file: %v", err)
		}

		// Calculate the blob hash to find it later
		hash := sha256.Sum256(sharedContent)
		blobHash := hex.EncodeToString(hash[:])
		expectedBlobDigest := fmt.Sprintf("sha256:%s", blobHash)

		// Create first model with the shared content
		model1, err := gguf.NewModel(sharedModelPath)
		if err != nil {
			t.Fatalf("Create first model failed: %v", err)
		}

		// Write the first model
		if err := s.Write(model1, []string{"shared-model-1:latest"}, nil); err != nil {
			t.Fatalf("Write first model failed: %v", err)
		}

		// Create second model with the same shared content
		model2, err := gguf.NewModel(sharedModelPath)
		if err != nil {
			t.Fatalf("Create second model failed: %v", err)
		}

		// Write the second model
		if err := s.Write(model2, []string{"shared-model-2:latest"}, nil); err != nil {
			t.Fatalf("Write second model failed: %v", err)
		}

		// Get the blob path
		blobPath := filepath.Join(storePath, "blobs", "sha256", blobHash)

		// Get the config blob paths (not shared)
		name1, err := model1.ConfigName()
		if err != nil {
			t.Fatalf("Failed to get config name: %v", err)
		}
		config1Path := filepath.Join(storePath, "blobs", "sha256", name1.Hex)
		name2, err := model2.ConfigName()
		if err != nil {
			t.Fatalf("Failed to get config name: %v", err)
		}
		config2Path := filepath.Join(storePath, "blobs", "sha256", name2.Hex)

		// Verify the blobs exists on disk
		if _, err := os.Stat(blobPath); os.IsNotExist(err) {
			t.Fatalf("Shared blob file doesn't exist: %s", blobPath)
		}
		if _, err := os.Stat(config1Path); os.IsNotExist(err) {
			t.Fatalf("Model 1 config blob file doesn't exist: %s", config1Path)
		}
		if _, err := os.Stat(config2Path); os.IsNotExist(err) {
			t.Fatalf("Model 2 config blob file doesn't exist: %s", config2Path)
		}

		// Delete the first model
		if err := s.Delete("shared-model-1:latest"); err != nil {
			t.Fatalf("Delete first model failed: %v", err)
		}

		// Verify the shared blob still exists on disk after deleting the first model
		if _, err := os.Stat(blobPath); os.IsNotExist(err) {
			t.Errorf("Shared blob file was incorrectly removed: %s", blobPath)
		}

		// Verify the first model config blob does not exist
		if _, err := os.Stat(config1Path); !os.IsNotExist(err) {
			t.Errorf("Model 1 config blob should have been removed: %s", config1Path)
		}

		// Verify the second model config blob still exists
		if _, err := os.Stat(blobPath); os.IsNotExist(err) {
			t.Errorf("Model 2 config blob file was incorrectly removed: %s", config2Path)
		}

		// Verify the second model is still in the index
		models, err := s.List()
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}

		var foundModel bool
		for _, model := range models {
			if containsTag(model.Tags, "shared-model-2:latest") {
				foundModel = true
				// Verify the blob is still associated with the model
				if len(model.Files) != 2 {
					t.Errorf("Expected 2 blobs, got %d", len(model.Files))
				}
				if model.Files[0] != expectedBlobDigest {
					t.Errorf("Expected blob %s, got %v", expectedBlobDigest, model.Files)
				}
				if model.Files[1] != name2.String() {
					t.Errorf("Expected blob %s, got %v", expectedBlobDigest, model.Files)
				}
				break
			}
		}

		if !foundModel {
			t.Errorf("Second model not found after deleting first model")
		}

		// Delete the second model
		if err := s.Delete("shared-model-2:latest"); err != nil {
			t.Fatalf("Delete second model failed: %v", err)
		}

		// Now the blob should be deleted since no models reference it
		if _, err := os.Stat(blobPath); !os.IsNotExist(err) {
			t.Errorf("Shared blob file still exists after deleting all referencing models: %s", blobPath)
		}
	})
}

// TestIncompleteFileHandling tests that files are created with .incomplete suffix and renamed on success
func TestIncompleteFileHandling(t *testing.T) {
	// Create a temporary directory for the test store
	tempDir, err := os.MkdirTemp("", "incomplete-file-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a temporary model file with known content
	modelContent := []byte("test model content for incomplete file test")
	modelPath := filepath.Join(tempDir, "incomplete-test-model.gguf")
	if err := os.WriteFile(modelPath, modelContent, 0644); err != nil {
		t.Fatalf("Failed to create test model file: %v", err)
	}

	// Calculate expected blob hash
	hash := sha256.Sum256(modelContent)
	blobHash := hex.EncodeToString(hash[:])

	// Create store
	storePath := filepath.Join(tempDir, "incomplete-model-store")
	s, err := store.New(store.Options{
		RootPath: storePath,
	})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Create the blobs directory
	blobsDir := filepath.Join(storePath, "blobs", "sha256")
	if err := os.MkdirAll(blobsDir, 0755); err != nil {
		t.Fatalf("Failed to create blobs directory: %v", err)
	}

	// Create an incomplete file directly
	incompleteFilePath := filepath.Join(blobsDir, blobHash+".incomplete")
	if err := os.WriteFile(incompleteFilePath, modelContent, 0644); err != nil {
		t.Fatalf("Failed to create incomplete file: %v", err)
	}

	// Verify the incomplete file exists
	if _, err := os.Stat(incompleteFilePath); os.IsNotExist(err) {
		t.Fatalf("Failed to create test .incomplete file")
	}

	// Create a model
	mdl, err := gguf.NewModel(modelPath)
	if err != nil {
		t.Fatalf("Create model failed: %v", err)
	}

	// Write the model - this should clean up the incomplete file and create the final file
	if err := s.Write(mdl, []string{"incomplete-test:latest"}, nil); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify that no .incomplete files remain after successful write
	files, err := os.ReadDir(blobsDir)
	if err != nil {
		t.Fatalf("Failed to read blobs directory: %v", err)
	}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".incomplete") {
			t.Errorf("Found .incomplete file after successful write: %s", file.Name())
		}
	}

	// Verify the blob exists with its final name
	blobPath := filepath.Join(blobsDir, blobHash)
	if _, err := os.Stat(blobPath); os.IsNotExist(err) {
		t.Errorf("Blob file doesn't exist at expected path: %s", blobPath)
	}
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

func newTestModel(t *testing.T) types.ModelArtifact {
	var mdl types.ModelArtifact
	var err error

	mdl, err = gguf.NewModel(filepath.Join("testdata", "dummy.gguf"))
	if err != nil {
		t.Fatalf("failed to create model from gguf file: %v", err)
	}
	licenseLayer, err := partial.NewLayer(filepath.Join("testdata", "license.txt"), types.MediaTypeLicense)
	if err != nil {
		t.Fatalf("failed to create license layer: %v", err)
	}
	mdl = mutate.AppendLayers(mdl, licenseLayer)
	return mdl
}
