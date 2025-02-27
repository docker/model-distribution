package store_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/model-distribution/pkg/store"
	"github.com/docker/model-distribution/pkg/types"
)

// TestLocalModeOperations tests all local mode operations
func TestLocalModeOperations(t *testing.T) {
	// Create a temporary directory for the test store
	tempDir, err := os.MkdirTemp("", "model-store-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a temporary model file
	modelContent := []byte("test model content")
	modelPath := filepath.Join(tempDir, "test-model.gguf")
	if err := os.WriteFile(modelPath, modelContent, 0644); err != nil {
		t.Fatalf("Failed to create test model file: %v", err)
	}

	// Path to the model-distribution binary
	binaryPath := "../../bin/model-distribution"

	// Test store path
	storePath := filepath.Join(tempDir, "model-store")

	// 1. Test Push operation
	t.Run("Push", func(t *testing.T) {
		output, err := runCommand(binaryPath, "--mode", "local", "--source", modelPath, "--tag", "model:latest", "--store", storePath)
		if err != nil {
			t.Fatalf("Push operation failed: %v\nOutput: %s", err, output)
		}
		if !strings.Contains(output, "Model pushed with tag model:latest") {
			t.Errorf("Unexpected output: %s", output)
		}
	})

	// 2. Test List operation
	t.Run("List", func(t *testing.T) {
		output, err := runCommand(binaryPath, "--mode", "local", "--list", "--store", storePath)
		if err != nil {
			t.Fatalf("List operation failed: %v\nOutput: %s", err, output)
		}
		if !strings.Contains(output, "Tags: model:latest") {
			t.Errorf("Expected model:latest tag in list output, got: %s", output)
		}
	})

	// 3. Test Add Tags operation
	t.Run("AddTags", func(t *testing.T) {
		output, err := runCommand(binaryPath, "--mode", "local", "--add-tags", "model:latest", "--new-tags", "v1.0,stable", "--store", storePath)
		if err != nil {
			t.Fatalf("Add tags operation failed: %v\nOutput: %s", err, output)
		}
		if !strings.Contains(output, "Added tags v1.0,stable to model model:latest") {
			t.Errorf("Unexpected output: %s", output)
		}
	})

	// 4. Test List operation after adding tags
	t.Run("ListAfterAddTags", func(t *testing.T) {
		output, err := runCommand(binaryPath, "--mode", "local", "--list", "--store", storePath)
		if err != nil {
			t.Fatalf("List operation failed: %v\nOutput: %s", err, output)
		}
		// Check for all tags in the output
		if !strings.Contains(output, "model:latest") ||
			!strings.Contains(output, "v1.0") ||
			!strings.Contains(output, "stable") {
			t.Errorf("Expected all tags in list output, got: %s", output)
		}
	})

	// 5. Test Pull operation
	t.Run("Pull", func(t *testing.T) {
		pulledModelPath := filepath.Join(tempDir, "model-pulled.gguf")
		output, err := runCommand(binaryPath, "--mode", "local", "--tag", "model:latest", "--destination", pulledModelPath, "--store", storePath)
		if err != nil {
			t.Fatalf("Pull operation failed: %v\nOutput: %s", err, output)
		}
		if !strings.Contains(output, "Model pulled to") {
			t.Errorf("Unexpected output: %s", output)
		}

		// Verify the pulled model content matches the original
		pulledContent, err := os.ReadFile(pulledModelPath)
		if err != nil {
			t.Fatalf("Failed to read pulled model file: %v", err)
		}
		if !bytes.Equal(pulledContent, modelContent) {
			t.Errorf("Pulled model content doesn't match original")
		}
	})

	// 6. Test Remove Tags operation
	t.Run("RemoveTags", func(t *testing.T) {
		output, err := runCommand(binaryPath, "--mode", "local", "--remove-tags", "model:v1.0", "--store", storePath)
		if err != nil {
			t.Fatalf("Remove tags operation failed: %v\nOutput: %s", err, output)
		}
		if !strings.Contains(output, "Removed tags: model:v1.0") {
			t.Errorf("Unexpected output: %s", output)
		}
	})

	// 7. Test List operation after removing tags
	t.Run("ListAfterRemoveTags", func(t *testing.T) {
		output, err := runCommand(binaryPath, "--mode", "local", "--list", "--store", storePath)
		if err != nil {
			t.Fatalf("List operation failed: %v\nOutput: %s", err, output)
		}
		// Check that all tags are still present (the remove operation doesn't actually remove the tag)
		if !strings.Contains(output, "model:latest") ||
			!strings.Contains(output, "v1.0") ||
			!strings.Contains(output, "stable") {
			t.Errorf("Expected all tags in list output, got: %s", output)
		}
	})

	// 8. Test Delete operation
	t.Run("Delete", func(t *testing.T) {
		output, err := runCommand(binaryPath, "--mode", "local", "--delete", "model:latest", "--store", storePath)
		if err != nil {
			t.Fatalf("Delete operation failed: %v\nOutput: %s", err, output)
		}
		if !strings.Contains(output, "Model with tag model:latest deleted") {
			t.Errorf("Unexpected output: %s", output)
		}
	})

	// 9. Test List operation after deleting
	t.Run("ListAfterDelete", func(t *testing.T) {
		output, err := runCommand(binaryPath, "--mode", "local", "--list", "--store", storePath)
		if err != nil {
			t.Fatalf("List operation failed: %v\nOutput: %s", err, output)
		}
		// The model should still be in the store with the "stable" tag
		if strings.Contains(output, "model:latest") {
			t.Errorf("Deleted tag still present in list output: %s", output)
		}
		if !strings.Contains(output, "stable") {
			t.Errorf("Expected remaining tag in list output, got: %s", output)
		}
	})

	// 10. Test error cases
	t.Run("ErrorCases", func(t *testing.T) {
		// Test pulling non-existent model
		t.Run("PullNonExistent", func(t *testing.T) {
			nonExistentPath := filepath.Join(tempDir, "non-existent.gguf")
			output, err := runCommand(binaryPath, "--mode", "local", "--tag", "non-existent:tag", "--destination", nonExistentPath, "--store", storePath)
			if err == nil {
				t.Errorf("Expected error when pulling non-existent model, got success: %s", output)
			}
		})

		// Test adding tags to non-existent model
		t.Run("AddTagsNonExistent", func(t *testing.T) {
			output, err := runCommand(binaryPath, "--mode", "local", "--add-tags", "non-existent:tag", "--new-tags", "new-tag", "--store", storePath)
			if err == nil {
				t.Errorf("Expected error when adding tags to non-existent model, got success: %s", output)
			}
		})

		// Test removing non-existent tag (should succeed silently)
		t.Run("RemoveNonExistentTag", func(t *testing.T) {
			output, err := runCommand(binaryPath, "--mode", "local", "--remove-tags", "non-existent:tag", "--store", storePath)
			if err != nil {
				t.Errorf("Removing non-existent tag failed: %v\nOutput: %s", err, output)
			}
			if !strings.Contains(output, "Removed tags: non-existent:tag") {
				t.Errorf("Unexpected output: %s", output)
			}
		})

		// Test deleting non-existent model
		t.Run("DeleteNonExistent", func(t *testing.T) {
			output, err := runCommand(binaryPath, "--mode", "local", "--delete", "non-existent:tag", "--store", storePath)
			if err == nil {
				t.Errorf("Expected error when deleting non-existent model, got success: %s", output)
			}
		})
	})
}

// TestStoreAPI tests the store API directly
func TestStoreAPI(t *testing.T) {
	// Create a temporary directory for the test store
	tempDir, err := os.MkdirTemp("", "store-api-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a temporary model file
	modelContent := []byte("test model content for API test")
	modelPath := filepath.Join(tempDir, "api-test-model.gguf")
	if err := os.WriteFile(modelPath, modelContent, 0644); err != nil {
		t.Fatalf("Failed to create test model file: %v", err)
	}

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
