package gguf_test

import (
	"path/filepath"
	"testing"

	"github.com/docker/model-distribution/pkg/gguf"
	"github.com/docker/model-distribution/pkg/types"
)

func TestGGUF(t *testing.T) {
	t.Run("TestGGUFModel", func(t *testing.T) {
		mdl, err := gguf.NewModel(filepath.Join("..", "..", "assets", "dummy.gguf"))
		if err != nil {
			t.Fatalf("Failed to create model: %v", err)
		}

		t.Run("TestGGUFConfig", func(t *testing.T) {
			cfg, err := mdl.Config()
			if err != nil {
				t.Fatalf("Failed to get config: %v", err)
			}
			if cfg.Format != types.FormatGGUF {
				t.Fatalf("Unexpected format: got %s expected %s", cfg.Format, types.FormatGGUF)
			}
			if cfg.Parameters != "183" {
				t.Fatalf("Unexpected parameters: got %s expected %s", cfg.Parameters, "183")
			}
			if cfg.Architecture != "llama" {
				t.Fatalf("Unexpected architecture: got %s expected %s", cfg.Parameters, "llama")
			}
			if cfg.Quantization != "Unknown" { // todo: testdata with a real value
				t.Fatalf("Unexpected quantization: got %s expected %s", cfg.Quantization, "Unknown")
			}
			if cfg.Size != "864 B" {
				t.Fatalf("Unexpected quantization: got %s expected %s", cfg.Quantization, "Unknown")
			}
		})

		t.Run("TestDescriptor", func(t *testing.T) {
			desc, err := mdl.Descriptor()
			if err != nil {
				t.Fatalf("Failed to get config: %v", err)
			}
			if desc.Created == nil {
				t.Fatal("Expected created time to be set: got ni")
			}
		})
	})
}
