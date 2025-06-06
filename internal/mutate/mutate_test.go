package mutate_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/static"
	ggcr "github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/docker/model-distribution/internal/gguf"
	"github.com/docker/model-distribution/internal/mutate"
	"github.com/docker/model-distribution/types"
)

func TestAppendLayer(t *testing.T) {
	mdl1, err := gguf.NewModel(filepath.Join("..", "..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}
	manifest1, err := mdl1.Manifest()
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}
	if len(manifest1.Layers) != 1 { // begin with one layer
		t.Fatalf("Expected 1 layer, got %d", len(manifest1.Layers))
	}

	// Append a layer
	mdl2 := mutate.AppendLayers(mdl1,
		static.NewLayer([]byte("some layer content"), "application/vnd.example.some.media.type"),
	)

	if mdl2 == nil {
		t.Fatal("Expected non-nil model")
	}

	// Check the manifest
	manifest2, err := mdl2.Manifest()
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}
	if len(manifest2.Layers) != 2 { // begin with one layer
		t.Fatalf("Expected 2 layers, got %d", len(manifest1.Layers))
	}

	// Check the config file
	rawCfg, err := mdl2.RawConfigFile()
	if err != nil {
		t.Fatalf("Failed to get raw config file: %v", err)
	}
	var cfg types.ConfigFile
	if err := json.Unmarshal(rawCfg, &cfg); err != nil {
		t.Fatalf("Failed to unmarshal config file: %v", err)
	}
	if len(cfg.RootFS.DiffIDs) != 2 {
		t.Fatalf("Expected 2 diff ids in rootfs, got %d", len(cfg.RootFS.DiffIDs))
	}
}

func TestConfigMediaTypes(t *testing.T) {
	mdl1, err := gguf.NewModel(filepath.Join("..", "..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}
	manifest1, err := mdl1.Manifest()
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}
	if manifest1.Config.MediaType != types.MediaTypeModelConfigV01 {
		t.Fatalf("Expected media type %s, got %s", types.MediaTypeModelConfigV01, manifest1.Config.MediaType)
	}

	newMediaType := ggcr.MediaType("application/vnd.example.other.type")
	mdl2 := mutate.ConfigMediaType(mdl1, newMediaType)
	manifest2, err := mdl2.Manifest()
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}
	if manifest2.Config.MediaType != newMediaType {
		t.Fatalf("Expected media type %s, got %s", newMediaType, manifest2.Config.MediaType)
	}
}

func TestWithConfig(t *testing.T) {
	mdl1, err := gguf.NewModel(filepath.Join("..", "..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}

	// Create a new config with some test values
	newConfig := types.Config{
		Format:       types.FormatGGUF,
		Architecture: "test-arch",
		Parameters:   "test-params",
		Quantization: "test-quant",
		Size:         "test-size",
		GGUF: map[string]string{
			"test-key": "test-value",
		},
		Capabilities: &types.Capabilities{
			IO: types.IOTypes{
				Input:  []string{types.IOTypeText, types.IOTypeAudio},
				Output: []string{types.IOTypeText, types.IOTypeAudio},
			},
			ToolUsage: true,
		},
	}

	// Apply the new config
	mdl2 := mutate.WithConfig(mdl1, newConfig)

	// Verify the config was updated
	config, err := mdl2.Config()
	if err != nil {
		t.Fatalf("Failed to get config: %v", err)
	}

	if config.Format != newConfig.Format {
		t.Errorf("Expected format %s, got %s", newConfig.Format, config.Format)
	}
	if config.Architecture != newConfig.Architecture {
		t.Errorf("Expected architecture %s, got %s", newConfig.Architecture, config.Architecture)
	}
	if config.Parameters != newConfig.Parameters {
		t.Errorf("Expected parameters %s, got %s", newConfig.Parameters, config.Parameters)
	}
	if config.Quantization != newConfig.Quantization {
		t.Errorf("Expected quantization %s, got %s", newConfig.Quantization, config.Quantization)
	}
	if config.Size != newConfig.Size {
		t.Errorf("Expected size %s, got %s", newConfig.Size, config.Size)
	}
	if len(config.GGUF) != len(newConfig.GGUF) {
		t.Errorf("Expected %d GGUF entries, got %d", len(newConfig.GGUF), len(config.GGUF))
	}
	if config.GGUF["test-key"] != newConfig.GGUF["test-key"] {
		t.Errorf("Expected GGUF value %s, got %s", newConfig.GGUF["test-key"], config.GGUF["test-key"])
	}
	if config.Capabilities == nil {
		t.Error("Expected non-nil capabilities")
	} else {
		if !config.Capabilities.ToolUsage {
			t.Error("Expected tool usage to be true")
		}
		if len(config.Capabilities.IO.Input) != 2 || config.Capabilities.IO.Input[0] != types.IOTypeText || config.Capabilities.IO.Input[1] != types.IOTypeAudio {
			t.Errorf("Expected input type [%s, %s], got %v", types.IOTypeText, types.IOTypeAudio, config.Capabilities.IO.Input)
		}
		if len(config.Capabilities.IO.Output) != 2 || config.Capabilities.IO.Output[0] != types.IOTypeText || config.Capabilities.IO.Output[1] != types.IOTypeAudio {
			t.Errorf("Expected output type [%s, %s], got %v", types.IOTypeText, types.IOTypeAudio, config.Capabilities.IO.Output)
		}
	}
}
