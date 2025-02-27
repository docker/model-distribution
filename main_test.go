package main

import (
	"context"
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
			tag:     registry + "/myartifact:v1.0.0",
			wantErr: false,
		},
		{
			name:    "Invalid source file",
			source:  "nonexistent/file.gguf",
			tag:     registry + "/myartifact:v1.0.0",
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
			tag:     registry + "/myartifact:v1.0.0",
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

	// First push a test model
	source := "assets/dummy.gguf"
	tag := registry + "/pulltest:v1.0.0"

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
			tag:     registry + "/nonexistent:v1.0.0",
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

			if tt.wantErr {
				return
			}

			// Verify the pulled image is valid
			manifest, err := img.Manifest()
			if err != nil {
				t.Errorf("Failed to get manifest from pulled image: %v", err)
			}
			if manifest == nil {
				t.Error("Pulled image manifest is nil")
			}
		})
	}
}

// TestLocalStorePullModel tests the local store functionality of PullModel
func TestLocalStorePullModel(t *testing.T) {
	// Set up test registry
	registryContainer, err := tc.Run(context.Background(), "registry:2.8.3")
	if err != nil {
		t.Fatalf("Failed to start registry container: %v", err)
	}

	registry, err := registryContainer.HostAddress(context.Background())
	if err != nil {
		t.Fatalf("Failed to get registry address: %v", err)
	}

	// First push a test model
	source := "assets/dummy.gguf"
	tag := registry + "/localstoretest:v1.0.0"

	_, err = PushModel(source, tag)
	if err != nil {
		t.Fatalf("Failed to push test model: %v", err)
	}

	// First pull - should pull from remote and store locally
	t.Log("First pull - should pull from remote")
	img1, err := PullModel(tag)
	if err != nil {
		t.Fatalf("Failed to pull model from remote: %v", err)
	}

	// Verify the pulled image is valid
	manifest1, err := img1.Manifest()
	if err != nil {
		t.Fatalf("Failed to get manifest from pulled image: %v", err)
	}

	// Terminate the registry container to ensure the next pull is from local store
	err = registryContainer.Terminate(context.Background())
	if err != nil {
		t.Fatalf("Failed to terminate registry container: %v", err)
	}

	// Second pull - should retrieve from local store
	t.Log("Second pull - should retrieve from local store")
	img2, err := PullModel(tag)
	if err != nil {
		t.Fatalf("Failed to pull model from local store: %v", err)
	}

	// Verify the pulled image is valid
	manifest2, err := img2.Manifest()
	if err != nil {
		t.Fatalf("Failed to get manifest from pulled image: %v", err)
	}

	// Verify both manifests are the same
	if manifest1.Config.Digest.String() != manifest2.Config.Digest.String() {
		t.Errorf("Manifests from remote and local store don't match")
	}
}

// TestLocalStoreErrorHandling tests error handling in the local store functionality
func TestLocalStoreErrorHandling(t *testing.T) {
	// Save original home directory
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome) // Restore original home directory after test

	// Test with invalid home directory to simulate local store initialization failure
	t.Run("Local store initialization failure", func(t *testing.T) {
		// Set HOME to a non-existent directory
		os.Setenv("HOME", "/nonexistent/directory")

		// Set up test registry
		registryContainer, err := tc.Run(context.Background(), "registry:2.8.3")
		if err != nil {
			t.Fatalf("Failed to start registry container: %v", err)
		}

		registry, err := registryContainer.HostAddress(context.Background())
		if err != nil {
			t.Fatalf("Failed to get registry address: %v", err)
		}

		// Try to pull a model
		tag := registry + "/errortest:v1.0.0"

		// Should fall back to remote pull even if local store initialization fails
		img, err := PullModel(tag)
		// We expect an error here because the image doesn't exist in the registry
		if err == nil {
			t.Errorf("Expected error when pulling non-existent model with invalid store")

			// If no error, verify the image
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

// TestGARIntegration tests pushing and pulling a model to/from Google Artifact Registry
func TestGARIntegration(t *testing.T) {
	// Skip if not running in CI with GAR enabled
	if os.Getenv("TEST_GAR_ENABLED") != "true" {
		t.Skip("Skipping GAR integration test - not enabled")
	}

	// Get tag from environment variable
	tag := os.Getenv("TEST_GAR_TAG")
	if tag == "" {
		t.Fatal("Missing required environment variable for GAR test: TEST_GAR_TAG")
	}

	// Register cleanup function
	t.Cleanup(func() {
		t.Log("Cleaning up GAR artifact:", tag)
		if err := DeleteModel(tag); err != nil {
			t.Logf("Warning: Failed to cleanup GAR artifact: %v", err)
		} else {
			t.Log("Successfully cleaned up GAR artifact")
		}
	})

	// Log authentication method
	if credFile := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); credFile != "" {
		t.Logf("Using Google Application Credentials file: %s", credFile)
	} else {
		t.Log("GOOGLE_APPLICATION_CREDENTIALS not set, will try other authentication methods")
	}

	source := "assets/dummy.gguf"

	// Test push to GAR
	t.Log("Pushing model to GAR:", tag)
	if _, err := PushModel(source, tag); err != nil {
		t.Fatalf("Failed to push test model: %v", err)
	}
	t.Log("Successfully pushed model to GAR:", tag)

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

	// Get tag from environment variable
	tag := os.Getenv("TEST_ECR_TAG")
	if tag == "" {
		t.Fatal("Missing required environment variable for ECR test: TEST_ECR_TAG")
	}

	// Register cleanup function
	t.Cleanup(func() {
		t.Log("Cleaning up ECR artifact:", tag)
		if err := DeleteModel(tag); err != nil {
			t.Logf("Warning: Failed to cleanup ECR artifact: %v", err)
		} else {
			t.Log("Successfully cleaned up ECR artifact")
		}
	})

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
