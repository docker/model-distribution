package model

import (
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

func FromGGUF(ggufLayer v1.Layer) (*Model, error) {
	diffID, err := ggufLayer.DiffID()
	if err != nil {
		return nil, fmt.Errorf("get gguf layer diffID: %w", err)
	}

	cfg := ConfigFile{
		Config: Config{
			Format: "gguf",
		},
		RootFS: v1.RootFS{
			Type: "rootfs",
			DiffIDs: []v1.Hash{
				diffID,
			},
		},
	}

	return &Model{
		configFile: cfg,
		layers:     []v1.Layer{ggufLayer},
	}, nil
}
