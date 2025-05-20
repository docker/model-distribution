package mutate

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
	ggcr "github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/docker/model-distribution/types"
)

func AppendLayers(mdl types.ModelArtifact, layers ...v1.Layer) types.ModelArtifact {
	return &model{
		base:     mdl,
		appended: layers,
	}
}

func ConfigMediaType(mdl types.ModelArtifact, mt ggcr.MediaType) types.ModelArtifact {
	return &model{
		base:            mdl,
		configMediaType: mt,
	}
}

// WithConfig returns a new model with the updated config
func WithConfig(mdl types.ModelArtifact, config types.Config) types.ModelArtifact {
	// Create a new model with the updated config
	return &model{
		base:   mdl,
		config: &config,
	}
}
