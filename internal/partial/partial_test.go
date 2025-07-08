package partial_test

import (
	"path/filepath"
	"testing"

	"github.com/docker/model-distribution/internal/gguf"
	"github.com/docker/model-distribution/internal/mutate"
	"github.com/docker/model-distribution/internal/partial"
	"github.com/docker/model-distribution/types"
)

func TestMMPROJPath(t *testing.T) {
	// Create a model from GGUF file
	mdl, err := gguf.NewModel(filepath.Join("..", "..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create model from GGUF: %v", err)
	}

	// Add multimodal projector layer
	mmprojLayer, err := partial.NewLayer(filepath.Join("..", "..", "assets", "dummy.mmproj"), types.MediaTypeMultimodalProjector)
	if err != nil {
		t.Fatalf("Failed to create multimodal projector layer: %v", err)
	}

	mdlWithMMProj := mutate.AppendLayers(mdl, mmprojLayer)

	// Test MMPROJPath function
	mmprojPath, err := partial.MMPROJPath(mdlWithMMProj)
	if err != nil {
		t.Fatalf("Failed to get multimodal projector path: %v", err)
	}

	expectedPath := filepath.Join("..", "..", "assets", "dummy.mmproj")
	if mmprojPath != expectedPath {
		t.Errorf("Expected multimodal projector path %s, got %s", expectedPath, mmprojPath)
	}
}

func TestMMPROJPathNotFound(t *testing.T) {
	// Create a model from a GGUF file without a Multimodal projector
	mdl, err := gguf.NewModel(filepath.Join("..", "..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create model from GGUF: %v", err)
	}

	// Test MMPROJPath function should return error
	_, err = partial.MMPROJPath(mdl)
	if err == nil {
		t.Error("Expected error when getting multimodal projector path from model without multimodal projector layer")
	}

	expectedErrorMsg := "model does not contain a application/vnd.docker.ai.mmproj layer"
	if err.Error() != expectedErrorMsg {
		t.Errorf("Expected error message %q, got %q", expectedErrorMsg, err.Error())
	}
}

func TestGGUFPath(t *testing.T) {
	// Create a model from GGUF file
	mdl, err := gguf.NewModel(filepath.Join("..", "..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create model from GGUF: %v", err)
	}

	// Test GGUFPath function
	ggufPath, err := partial.GGUFPath(mdl)
	if err != nil {
		t.Fatalf("Failed to get GGUF path: %v", err)
	}

	expectedPath := filepath.Join("..", "..", "assets", "dummy.gguf")
	if ggufPath != expectedPath {
		t.Errorf("Expected GGUF path %s, got %s", expectedPath, ggufPath)
	}
}

func TestLayerPathByMediaType(t *testing.T) {
	// Create a model from GGUF file
	mdl, err := gguf.NewModel(filepath.Join("..", "..", "assets", "dummy.gguf"))
	if err != nil {
		t.Fatalf("Failed to create model from GGUF: %v", err)
	}

	// Add license layer
	licenseLayer, err := partial.NewLayer(filepath.Join("..", "..", "assets", "license.txt"), types.MediaTypeLicense)
	if err != nil {
		t.Fatalf("Failed to create license layer: %v", err)
	}

	// Add a Multimodal projector layer
	mmprojLayer, err := partial.NewLayer(filepath.Join("..", "..", "assets", "dummy.mmproj"), types.MediaTypeMultimodalProjector)
	if err != nil {
		t.Fatalf("Failed to create multimodal projector layer: %v", err)
	}

	mdlWithLayers := mutate.AppendLayers(mdl, licenseLayer, mmprojLayer)

	// Test that we can find each layer type
	ggufPath, err := partial.GGUFPath(mdlWithLayers)
	if err != nil {
		t.Fatalf("Failed to get GGUF path: %v", err)
	}
	if ggufPath != filepath.Join("..", "..", "assets", "dummy.gguf") {
		t.Errorf("Expected GGUF path to be: %s, got: %s", filepath.Join("..", "..", "assets", "dummy.gguf"), ggufPath)
	}

	mmprojPath, err := partial.MMPROJPath(mdlWithLayers)
	if err != nil {
		t.Fatalf("Failed to get multimodal projector path: %v", err)
	}
	if mmprojPath != filepath.Join("..", "..", "assets", "dummy.mmproj") {
		t.Errorf("Expected multimodal projector path to be: %s, got: %s", filepath.Join("..", "..", "assets", "dummy.mmproj"), mmprojPath)
	}

}
