package mutate

import (
	"encoding/json"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/docker/model-distribution/pkg/partial"
	"github.com/docker/model-distribution/pkg/types"
)

func AppendLayers(mdl types.ModelArtifact, layers ...v1.Layer) (types.ModelArtifact, error) {
	return &model{
		ModelArtifact: mdl,
		appended:      layers,
	}, nil
}

type model struct {
	types.ModelArtifact
	appended []v1.Layer
}

func (m *model) Layers() ([]v1.Layer, error) {
	ls, err := m.ModelArtifact.Layers()
	if err != nil {
		return nil, err
	}
	return append(ls, m.appended...), nil
}

func (m *model) Manifest() (*v1.Manifest, error) {
	return partial.ManifestForLayers(m)
}

func (m *model) RawConfigFile() ([]byte, error) {
	cf, err := partial.ConfigFile(m.ModelArtifact)
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
