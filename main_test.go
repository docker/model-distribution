package main

import (
	"context"
	"fmt"
	"os"
	"testing"

	tc "github.com/testcontainers/testcontainers-go/modules/registry"
)

func TestPushModel(t *testing.T) {
	registryContainer, err := tc.Run(context.Background(), "registry:2.8.3")
	if err != nil {
		t.Fatalf("Failed to start registry container: %v", err)
	}
	registry, err := registryContainer.HostAddress(context.Background())
	if err != nil {
		t.Fatalf("Failed to get registry address: %v", err)
	}
	username := "testuser"

	// Test cases
	tests := []struct {
		name    string
		source  string
		tag     string
		wantErr bool
	}{
		{
			name:    "Valid push",
			source:  "assets/dummy.gguf",
			tag:     registry + "/" + username + "/myartifact:v1.0.0",
			wantErr: false,
		},
		{
			name:    "Invalid source file",
			source:  "nonexistent/file.gguf",
			tag:     registry + "/" + username + "/myartifact:v1.0.0",
			wantErr: true,
		},
		{
			name:    "Invalid tag format",
			source:  "assets/dummy.gguf",
			tag:     "invalid:tag:format",
			wantErr: true,
		},
		{
			name:    "Empty source",
			source:  "",
			tag:     registry + "/" + username + "/myartifact:v1.0.0",
			wantErr: true,
		},
		{
			name:    "Empty tag",
			source:  "assets/dummy.gguf",
			tag:     "",
			wantErr: true,
		},
	}

	// Run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := PushModel(tt.source, tt.tag)
			if (err != nil) != tt.wantErr {
				t.Errorf("PushModel() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				if ref.String() != tt.tag {
					t.Errorf("PushModel() ref = %v, want %v", ref.String(), tt.tag)
				}
			}
		})
	}
}

func TestPullModel(t *testing.T) {
	// Set up test registry
	registryContainer, err := tc.Run(context.Background(), "registry:2.8.3")
	if err != nil {
		t.Fatalf("Failed to start registry container: %v", err)
	}

	registry, err := registryContainer.HostAddress(context.Background())
	if err != nil {
		t.Fatalf("Failed to get registry address: %v", err)
	}
	username := "testuser"

	// First push a test model
	source := "assets/dummy.gguf"
	tag := registry + "/" + username + "/pulltest:v1.0.0"

	_, err = PushModel(source, tag)
	if err != nil {
		t.Fatalf("Failed to push test model: %v", err)
	}

	// Test cases
	tests := []struct {
		name    string
		tag     string
		wantErr bool
	}{
		{
			name:    "Valid pull",
			tag:     tag,
			wantErr: false,
		},
		{
			name:    "Invalid tag format",
			tag:     "invalid:tag:format",
			wantErr: true,
		},
		{
			name:    "Nonexistent image",
			tag:     registry + "/" + username + "/nonexistent:v1.0.0",
			wantErr: true,
		},
		{
			name:    "Empty tag",
			tag:     "",
			wantErr: true,
		},
	}

	// Run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img, err := PullModel(tt.tag)
			if (err != nil) != tt.wantErr {
				t.Errorf("PullModel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				// Verify the pulled image is valid
				manifest, err := img.Manifest()
				if err != nil {
					t.Errorf("Failed to get manifest from pulled image: %v", err)
				}
				if manifest == nil {
					t.Error("Pulled image manifest is nil")
				}
			}
		})
	}
}

// TestGARIntegration tests pushing and pulling a model to/from Google Artifact Registry
func TestGARIntegration(t *testing.T) {
	// Skip if not running in CI with GAR enabled
	if os.Getenv("TEST_GAR_ENABLED") != "true" {
		t.Skip("Skipping GAR integration test - not enabled")
	}

	// Get environment variables
	garLocation := os.Getenv("TEST_GAR_LOCATION")
	projectID := os.Getenv("TEST_PROJECT_ID")
	garRepo := os.Getenv("TEST_GAR_REPOSITORY")
	modelName := os.Getenv("TEST_MODEL_NAME")
	modelVersion := os.Getenv("TEST_MODEL_VERSION")

	if garLocation == "" || projectID == "" || garRepo == "" || modelName == "" || modelVersion == "" {
		t.Fatal("Missing required environment variables for GAR test")
	}

	// Create tag for GAR
	tag := fmt.Sprintf("%s/%s/%s/%s:%s", garLocation, projectID, garRepo, modelName, modelVersion)
	source := "assets/dummy.gguf"

	// Test push to GAR
	t.Log("Pushing model to GAR:", tag)
	ref, err := PushModel(source, tag)
	if err != nil {
		t.Fatalf("Failed to push model to GAR: %v", err)
	}
	t.Log("Successfully pushed model to GAR:", ref.String())

	// Test pull from GAR
	t.Log("Pulling model from GAR:", tag)
	img, err := PullModel(tag)
	if err != nil {
		t.Fatalf("Failed to pull model from GAR: %v", err)
	}

	// Verify the pulled image
	manifest, err := img.Manifest()
	if err != nil {
		t.Fatalf("Failed to get manifest from pulled GAR image: %v", err)
	}
	if manifest == nil {
		t.Fatal("Pulled GAR image manifest is nil")
	}
	t.Log("Successfully pulled and verified model from GAR")
}

// TestECRIntegration tests pushing and pulling a model to/from Amazon ECR
func TestECRIntegration(t *testing.T) {
	// Skip if not running in CI with ECR enabled
	if os.Getenv("TEST_ECR_ENABLED") != "true" {
		t.Skip("Skipping ECR integration test - not enabled")
	}

	// Get environment variables
	ecrRegistry := os.Getenv("TEST_ECR_REGISTRY")
	ecrRepo := os.Getenv("TEST_ECR_REPOSITORY")
	modelName := os.Getenv("TEST_MODEL_NAME")
	modelVersion := os.Getenv("TEST_MODEL_VERSION")

	if ecrRegistry == "" || ecrRepo == "" || modelName == "" || modelVersion == "" {
		t.Fatal("Missing required environment variables for ECR test")
	}

	// Create tag for ECR
	tag := fmt.Sprintf("%s/%s/%s:%s", ecrRegistry, ecrRepo, modelName, modelVersion)
	source := "assets/dummy.gguf"

	// Test push to ECR
	t.Log("Pushing model to ECR:", tag)
	ref, err := PushModel(source, tag)
	if err != nil {
		t.Fatalf("Failed to push model to ECR: %v", err)
	}
	t.Log("Successfully pushed model to ECR:", ref.String())

	// Test pull from ECR
	t.Log("Pulling model from ECR:", tag)
	img, err := PullModel(tag)
	if err != nil {
		t.Fatalf("Failed to pull model from ECR: %v", err)
	}

	// Verify the pulled image
	manifest, err := img.Manifest()
	if err != nil {
		t.Fatalf("Failed to get manifest from pulled ECR image: %v", err)
	}
	if manifest == nil {
		t.Fatal("Pulled ECR image manifest is nil")
	}
	t.Log("Successfully pulled and verified model from ECR")
}
