package gguf

import (
	"fmt"
	"strings"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	gguf_parser "github.com/gpustack/gguf-parser-go"

	"github.com/docker/model-distribution/internal/partial"
	"github.com/docker/model-distribution/types"
)

func NewModel(path string) (*Model, error) {
	layer, err := partial.NewLayer(path, types.MediaTypeGGUF)
	if err != nil {
		return nil, fmt.Errorf("create gguf layer: %w", err)
	}
	diffID, err := layer.DiffID()
	if err != nil {
		return nil, fmt.Errorf("get gguf layer diffID: %w", err)
	}

	created := time.Now()
	return &Model{
		configFile: types.ConfigFile{
			Config: configFromFile(path),
			Descriptor: types.Descriptor{
				Created: &created,
			},
			RootFS: v1.RootFS{
				Type: "rootfs",
				DiffIDs: []v1.Hash{
					diffID,
				},
			},
		},
		layers: []v1.Layer{layer},
	}, nil
}

func configFromFile(path string) types.Config {
	ggufFile, err := gguf_parser.ParseGGUFFile(path)
	if err != nil {
		return types.Config{} // continue without metadata
	}
	return types.Config{
		Format:       types.FormatGGUF,
		Parameters:   strings.TrimSpace(ggufFile.Metadata().Parameters.String()),
		Architecture: strings.TrimSpace(ggufFile.Metadata().Architecture),
		Quantization: strings.TrimSpace(ggufFile.Metadata().FileType.String()),
		Size:         strings.TrimSpace(ggufFile.Metadata().Size.String()),
	}
}
