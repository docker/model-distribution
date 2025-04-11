package distribution

import (
	"context"
	"os"
	"testing"
)

func TestECRIntegration(t *testing.T) {
	// Skip if ECR integration is not enabled
	if os.Getenv("TEST_ECR_ENABLED") != "true" {
		t.Skip("Skipping ECR integration test")
	}

	// Get ECR tag from environment
	ecrTag := os.Getenv("TEST_ECR_TAG")
	if ecrTag == "" {
		t.Fatal("TEST_ECR_TAG environment variable is required")
	}

	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-ecr-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Read test model file
	modelFile := "../assets/dummy.gguf"
	modelContent, err := os.ReadFile(modelFile)
	if err != nil {
		t.Fatalf("Failed to read test model file: %v", err)
	}

	// Test push to ECR
	t.Run("Push", func(t *testing.T) {
		err := client.PushModel(context.Background(), modelFile, ecrTag)
		if err != nil {
			t.Fatalf("Failed to push model to ECR: %v", err)
		}
	})

	// Test pull from ECR
	t.Run("Pull without progress", func(t *testing.T) {
		err := client.PullModel(context.Background(), ecrTag, nil)
		if err != nil {
			t.Fatalf("Failed to pull model from ECR: %v", err)
		}

		model, err := client.GetModel(ecrTag)
		if err != nil {
			t.Fatalf("Failed to get model: %v", err)
		}

		modelPath, err := model.GGUFPath()
		if err != nil {
			t.Fatalf("Failed to get model path: %v", err)
		}

		defer os.Remove(modelPath)

		// Verify model content
		pulledContent, err := os.ReadFile(modelPath)
		if err != nil {
			t.Fatalf("Failed to read pulled model: %v", err)
		}

		if string(pulledContent) != string(modelContent) {
			t.Errorf("Pulled model content doesn't match original: got %q, want %q", pulledContent, modelContent)
		}
	})

	// Test get model info
	t.Run("GetModel", func(t *testing.T) {
		model, err := client.GetModel(ecrTag)
		if err != nil {
			t.Fatalf("Failed to get model info: %v", err)
		}

		if len(model.Tags()) == 0 || model.Tags()[0] != ecrTag {
			t.Errorf("Model tags don't match: got %v, want [%s]", model.Tags(), ecrTag)
		}
	})
}
