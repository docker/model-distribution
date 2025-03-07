package model

import (
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/docker/model-distribution/pkg/types"
)

func FromGGUF(ggufLayer v1.Layer) (*Model, error) {
	diffID, err := ggufLayer.DiffID()
	if err != nil {
		return nil, fmt.Errorf("get gguf layer diffID: %w", err)
	}

	cfg := types.ConfigFile{
		Config: types.Config{
			Format: "gguf",
		},
		RootFS: v1.RootFS{
			Type: "rootfs",
			DiffIDs: []v1.Hash{
				diffID,
			},
		},
	}
	layers := []v1.Layer{ggufLayer}
	return &Model{
		configFile: cfg,
		//manifest:   manifest(cfg, layers),
		layers: layers,
	}, nil
}

//func manifest(cfg types.ConfigFile, layers []v1.Layer) *v1.Manifest {
//	cfgLayer, err := partial.ConfigLayer(m)
//	if err != nil {
//		return nil, fmt.Errorf("get raw config file: %w", err)
//	}
//	cfgDsc, err := partial.Descriptor(cfgLayer)
//	if err != nil {
//		return nil, fmt.Errorf("get config descriptor: %w", err)
//	}
//	cfgDsc.MediaType = types.MediaTypeModelConfig
//
//	ls, err := m.Layers()
//	if err != nil {
//		return nil, fmt.Errorf("get layers: %w", err)
//	}
//
//	var layers []v1.Descriptor
//	for _, l := range ls {
//		desc, err := partial.Descriptor(l)
//		if err != nil {
//			return nil, fmt.Errorf("get layer descriptor: %w", err)
//		}
//		layers = append(layers, *desc)
//	}
//
//	return &v1.Manifest{
//		SchemaVersion: 2,
//		MediaType:     ggcr.OCIManifestSchema1,
//		Config:        *cfgDsc,
//		Layers:        layers,
//	}, nil
//}
