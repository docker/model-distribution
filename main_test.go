package main

import (
	"os"
	"testing"
)

func TestPushModel(t *testing.T) {
	// Check for required environment variables
	registry := os.Getenv("DOCKER_REGISTRY")
	username := os.Getenv("DOCKER_USERNAME")
	password := os.Getenv("DOCKER_PASSWORD")

	if registry == "" || username == "" || password == "" {
		t.Skip("Skipping test: DOCKER_REGISTRY, DOCKER_USERNAME, or DOCKER_PASSWORD not set")
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
			tag:     registry + "/" + username + "/test-model:latest",
			wantErr: false,
		},
		{
			name:    "Invalid source",
			source:  "nonexistent.gguf",
			tag:     registry + "/" + username + "/test-model:latest",
			wantErr: true,
		},
		{
			name:    "Invalid tag",
			source:  "assets/dummy.gguf",
			tag:     "invalid:tag:format",
			wantErr: true,
		},
	}

	// Create test directory and dummy file
	err := os.MkdirAll("assets", 0755)
	if err != nil {
		t.Fatalf("Failed to create assets directory: %v", err)
	}

	dummyContent := []byte("dummy model content")
	err = os.WriteFile("assets/dummy.gguf", dummyContent, 0644)
	if err != nil {
		t.Fatalf("Failed to create dummy file: %v", err)
	}

	// Clean up after tests
	defer os.RemoveAll("assets")

	// Run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := PushModel(tt.source, tt.tag)
			if (err != nil) != tt.wantErr {
				t.Errorf("PushModel() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
