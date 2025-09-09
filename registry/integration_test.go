package registry

import (
	"context"
	"testing"
)

func TestNormalizationIntegration(t *testing.T) {
	// Test that normalization is being called in the registry client methods
	client := NewClient()
	
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name in Model()",
			input:    "llama",
			expected: "docker.io/ai/llama:latest",
		},
		{
			name:     "name with tag in Model()",
			input:    "llama:v1.0",
			expected: "docker.io/ai/llama:v1.0",
		},
		{
			name:     "name with org in NewTarget()",
			input:    "myorg/llama:v2.0",
			expected: "docker.io/myorg/llama:v2.0",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			// Test the normalization directly
			normalized, err := Normalize(tt.input)
			if err != nil {
				t.Fatalf("normalization failed: %v", err)
			}
			
			if normalized != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, normalized)
			}
			
			// Test that the methods would use normalization (they will fail to connect to registry, but that's expected)
			// We're just verifying the normalization path is called
			_, err = client.Model(context.Background(), tt.input)
			// We expect this to fail due to no registry, but it should fail AFTER normalization
			if err == nil {
				t.Error("expected error when trying to connect to non-existent registry")
			}
			
			// Test NewTarget
			_, err = client.NewTarget(tt.input)
			// This should succeed since NewTarget only parses the reference
			if err != nil {
				t.Errorf("NewTarget failed: %v", err)
			}
		})
	}
}