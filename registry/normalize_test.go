package registry

import (
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  string
		shouldErr bool
	}{
		{
			name:     "simple model name",
			input:    "llama",
			expected: "docker.io/ai/llama:latest",
		},
		{
			name:     "model name with tag",
			input:    "llama:v1.0",
			expected: "docker.io/ai/llama:v1.0",
		},
		{
			name:     "model name with organization",
			input:    "myorg/llama",
			expected: "docker.io/myorg/llama:latest",
		},
		{
			name:     "model name with organization and tag",
			input:    "myorg/llama:v1.0",
			expected: "docker.io/myorg/llama:v1.0",
		},
		{
			name:     "full reference with different registry",
			input:    "registry.example.com/myorg/llama:v1.0",
			expected: "registry.example.com/myorg/llama:v1.0",
		},
		{
			name:     "registry without tag",
			input:    "registry.example.com/myorg/llama",
			expected: "registry.example.com/myorg/llama:latest",
		},
		{
			name:     "docker.io with explicit namespace",
			input:    "docker.io/myorg/llama",
			expected: "docker.io/myorg/llama:latest",
		},
		{
			name:     "docker.io with explicit namespace and tag",
			input:    "docker.io/myorg/llama:v1.0",
			expected: "docker.io/myorg/llama:v1.0",
		},
		{
			name:     "docker.io with official prefix already present",
			input:    "docker.io/ai/llama:v1.0",
			expected: "docker.io/ai/llama:v1.0",
		},
		{
			name:     "docker.io with simple name",
			input:    "docker.io/llama",
			expected: "docker.io/ai/llama:latest",
		},
		{
			name:     "docker.io with simple name and tag",
			input:    "docker.io/llama:v1.0",
			expected: "docker.io/ai/llama:v1.0",
		},
		{
			name:     "model with digest",
			input:    "llama@sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
			expected: "docker.io/ai/llama@sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
		},
		{
			name:     "full reference with digest",
			input:    "docker.io/myorg/llama@sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
			expected: "docker.io/myorg/llama@sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234",
		},
		{
			name:      "empty reference",
			input:     "",
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Normalize(tt.input)
			
			if tt.shouldErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestNormalizeConstants(t *testing.T) {
	// Test that our constants have the expected values
	if defaultDomain != "docker.io" {
		t.Errorf("expected defaultDomain to be 'docker.io', got %q", defaultDomain)
	}
	
	if officialRepoPrefix != "ai/" {
		t.Errorf("expected officialRepoPrefix to be 'ai/', got %q", officialRepoPrefix)
	}
	
	if defaultTag != "latest" {
		t.Errorf("expected defaultTag to be 'latest', got %q", defaultTag)
	}
}