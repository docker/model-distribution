package gguf

import (
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/docker/model-distribution/pkg/types"
)

func NewModel(path string) (*Model, error) {
	layer, err := NewLayer(path)
	if err != nil {
		return nil, fmt.Errorf("create gguf layer: %w", err)
	}
	diffID, err := layer.DiffID()
	if err != nil {
		return nil, fmt.Errorf("get gguf layer diffID: %w", err)
	}

	return &Model{
		configFile: types.ConfigFile{
			Config: types.Config{
				Format: "gguf",
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
