package gguf_test

import (
	"path/filepath"
	"testing"

	"github.com/docker/model-distribution/internal/gguf"
	"github.com/docker/model-distribution/types"
)

func TestGGUF(t *testing.T) {
	t.Run("TestGGUFModel", func(t *testing.T) {
		mdl, err := gguf.NewModel(filepath.Join("..", "..", "assets", "dummy.gguf"))
		if err != nil {
			t.Fatalf("Failed to create model: %v", err)
		}

		t.Run("TestConfig", func(t *testing.T) {
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

		t.Run("TestManifest", func(t *testing.T) {
			manifest, err := mdl.Manifest()
			if err != nil {
				t.Fatalf("Failed to get config: %v", err)
			}
			if len(manifest.Layers) != 1 {
				t.Fatalf("Expected 1 layer, got %d", len(manifest.Layers))
			}
			if manifest.Layers[0].MediaType != types.MediaTypeGGUF {
				t.Fatalf("Expected layer with media type %s, got %s", types.MediaTypeGGUF, manifest.Layers[0].MediaType)
			}
		})
	})
}
