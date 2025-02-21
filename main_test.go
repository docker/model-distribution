package main

import (
	"context"
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
