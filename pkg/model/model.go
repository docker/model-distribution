package model

import (
	"encoding/json"
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

var _ v1.Image = &Model{}

type Model struct {
	configFile ConfigFile
	layers     []v1.Layer
}

func (m *Model) Layers() ([]v1.Layer, error) {
	return m.layers, nil
}

func (m *Model) Size() (int64, error) {
	return partial.Size(m)
}

func (m *Model) ConfigName() (v1.Hash, error) {
	panic("implement me")
}

func (m *Model) ConfigFile() (*v1.ConfigFile, error) {
	panic("implement me")
}

func (m *Model) Digest() (v1.Hash, error) {
	return partial.Digest(m)
}

func (m *Model) Manifest() (*v1.Manifest, error) {
	cfgLayer, err := partial.ConfigLayer(m)
	if err != nil {
		return nil, fmt.Errorf("get raw config file: %w", err)
	}
	cfgDsc, err := partial.Descriptor(cfgLayer)
	if err != nil {
		return nil, fmt.Errorf("get config descriptor: %w", err)
	}
	cfgDsc.MediaType = MediaTypeModelConfig
	
	ls, err := m.Layers()
	if err != nil {
		return nil, fmt.Errorf("get layers: %w", err)
	}

	var layers []v1.Descriptor
	for _, l := range ls {
		desc, err := partial.Descriptor(l)
		if err != nil {
			return nil, fmt.Errorf("get layer descriptor: %w", err)
		}
		layers = append(layers, *desc)
	}

	return &v1.Manifest{
		SchemaVersion: 2,
		MediaType:     types.OCIManifestSchema1,
		Config:        *cfgDsc,
		Layers:        layers,
	}, nil
}

func (m *Model) LayerByDigest(hash v1.Hash) (v1.Layer, error) {
	panic("implement me")
}

func (m *Model) LayerByDiffID(hash v1.Hash) (v1.Layer, error) {
	panic("implement me")
}

func (m *Model) RawManifest() ([]byte, error) {
	return partial.RawManifest(m)
}

func (m *Model) RawConfigFile() ([]byte, error) {
	return json.Marshal(m.configFile)
}

func (m *Model) MediaType() (types.MediaType, error) {
	manifest, err := m.Manifest()
	if err != nil {
		return "", fmt.Errorf("compute maniest: %w", err)
	}
	return manifest.MediaType, nil
}
