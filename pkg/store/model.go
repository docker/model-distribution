package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/types"

	mdpartial "github.com/docker/model-distribution/pkg/partial"
	mdtypes "github.com/docker/model-distribution/pkg/types"
)

var _ v1.Image = &Model{}

type Model struct {
	rawManifest []byte
	blobsDir    string
	tags        []string
}

func (m *Model) Layers() ([]v1.Layer, error) {
	manifest, err := m.Manifest()
	if err != nil {
		return nil, fmt.Errorf("get manifest: %w", err)
	}
	var layers []v1.Layer
	for _, ld := range manifest.Layers {
		layers = append(layers, &mdpartial.Layer{
			Path:       filepath.Join(m.blobsDir, ld.Digest.Hex),
			Descriptor: ld,
		})
	}
	return layers, nil
}

func (m *Model) MediaType() (types.MediaType, error) {
	manifest, err := m.Manifest()
	if err != nil {
		return "", fmt.Errorf("get manifest: %w", err)
	}
	return manifest.MediaType, nil
}

func (m *Model) Size() (int64, error) {
	return partial.Size(m)
}

func (m *Model) ConfigName() (v1.Hash, error) {
	return partial.ConfigName(m)
}

func (m *Model) ConfigFile() (*v1.ConfigFile, error) {
	return nil, errors.New("invalid for model")
}

func (m *Model) RawConfigFile() ([]byte, error) {
	manifest, err := m.Manifest()
	if err != nil {
		return nil, fmt.Errorf("get manifest: %w", err)
	}
	configPath := filepath.Join(m.blobsDir, manifest.Config.Digest.Hex)
	rawConfig, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config from %s: %w", configPath, err)
	}
	return rawConfig, nil
}

func (m *Model) Digest() (v1.Hash, error) {
	return partial.Digest(m)
}

func (m *Model) Manifest() (*v1.Manifest, error) {
	return partial.Manifest(m)
}

func (m *Model) RawManifest() ([]byte, error) {
	return m.rawManifest, nil
}

func (m *Model) LayerByDigest(hash v1.Hash) (v1.Layer, error) {
	layers, err := m.Layers()
	if err != nil {
		return nil, err
	}
	for _, l := range layers {
		d, err := l.Digest()
		if err != nil {
			return nil, fmt.Errorf("get digest: %w", err)
		}
		if d == hash {
			return l, nil
		}
	}
	return nil, fmt.Errorf("layer with digest %s not found", hash)
}

func (m *Model) LayerByDiffID(hash v1.Hash) (v1.Layer, error) {
	return m.LayerByDigest(hash)
}

func (m *Model) GGUFPath() (string, error) {
	return mdpartial.GGUFPath(m)
}

func (m *Model) Tags() []string {
	return m.tags
}

func (m *Model) ID() (string, error) {
	return mdpartial.ID(m)
}

func (m *Model) Config() (mdtypes.Config, error) {
	return mdpartial.Config(m)
}

func (m *Model) Descriptor() (mdtypes.Descriptor, error) {
	return mdpartial.Descriptor(m)
}
