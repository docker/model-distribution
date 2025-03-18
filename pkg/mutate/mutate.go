package mutate

import (
	"encoding/json"
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	ggcrpartial "github.com/google/go-containerregistry/pkg/v1/partial"
	ggcr "github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/docker/model-distribution/pkg/partial"
	"github.com/docker/model-distribution/pkg/types"
)

func AppendLayers(mdl types.ModelArtifact, layers ...v1.Layer) types.ModelArtifact {
	return &model{
		base:     mdl,
		appended: layers,
	}
}

type model struct {
	base     types.ModelArtifact
	appended []v1.Layer
}

func (m *model) Descriptor() (types.Descriptor, error) {
	return partial.Descriptor(m.base)
}

func (m *model) ID() (string, error) {
	return partial.ID(m)
}

func (m *model) GGUFPath() (string, error) {
	return partial.GGUFPath(m)
}

func (m *model) Config() (types.Config, error) {
	return partial.Config(m)
}

func (m *model) Tags() []string {
	return m.base.Tags()
}

func (m *model) MediaType() (ggcr.MediaType, error) {
	manifest, err := m.Manifest()
	if err != nil {
		return "", fmt.Errorf("compute maniest: %w", err)
	}
	return manifest.MediaType, nil
}

func (m *model) Size() (int64, error) {
	return ggcrpartial.Size(m)
}

func (m *model) ConfigName() (v1.Hash, error) {
	return ggcrpartial.ConfigName(m)
}

func (m *model) ConfigFile() (*v1.ConfigFile, error) {
	return nil, fmt.Errorf("invalid for model")
}

func (m *model) Digest() (v1.Hash, error) {
	return ggcrpartial.Digest(m)
}

func (m *model) RawManifest() ([]byte, error) {
	return ggcrpartial.RawManifest(m)
}

func (m *model) LayerByDigest(hash v1.Hash) (v1.Layer, error) {
	ls, err := m.Layers()
	if err != nil {
		return nil, err
	}
	for _, l := range ls {
		d, err := l.Digest()
		if err != nil {
			return nil, fmt.Errorf("get layer digest: %w", err)
		}
		if d == hash {
			return l, nil
		}
	}
	return nil, fmt.Errorf("layer not found")
}

func (m *model) LayerByDiffID(hash v1.Hash) (v1.Layer, error) {
	ls, err := m.Layers()
	if err != nil {
		return nil, err
	}
	for _, l := range ls {
		d, err := l.Digest()
		if err != nil {
			return nil, fmt.Errorf("get layer digest: %w", err)
		}
		if d == hash {
			return l, nil
		}
	}
	return nil, fmt.Errorf("layer not found")
}

func (m *model) Layers() ([]v1.Layer, error) {
	ls, err := m.base.Layers()
	if err != nil {
		return nil, err
	}
	return append(ls, m.appended...), nil
}

func (m *model) Manifest() (*v1.Manifest, error) {
	return partial.ManifestForLayers(m)
}

func (m *model) RawConfigFile() ([]byte, error) {
	cf, err := partial.ConfigFile(m.base)
	if err != nil {
		return nil, err
	}
	for _, l := range m.appended {
		diffID, err := l.DiffID()
		if err != nil {
			return nil, err
		}
		cf.RootFS.DiffIDs = append(cf.RootFS.DiffIDs, diffID)
	}
	raw, err := json.Marshal(cf)
	if err != nil {
		return nil, err
	}
	return raw, err
}
