package types

import (
	"time"

	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

const (
	MediaTypeModelConfig = types.MediaType("application/vnd.docker.ai.model.config.v1+json")
	MediaTypeGGUF        = types.MediaType("application/vnd.docker.ai.model.file.v1+gguf")
	MediaTypeLicense     = types.MediaType("application/vnd.docker.ai.license")

	FormatGGUF = Format("gguf")
)

type Format string

type ConfigFile struct {
	Config     Config     `json:"config"`
	Descriptor Descriptor `json:"descriptor"`
	RootFS     v1.RootFS  `json:"rootfs"`
}

// Config describes the model.
type Config struct {
	Format       Format `json:"format,omitempty"`
	Quantization string `json:"quantization,omitempty"`
	Parameters   string `json:"parameters,omitempty"`
	Architecture string `json:"architecture,omitempty"`
	Size         string `json:"size,omitempty"`
}

// Descriptor provides metadata about the provenance of the model.
type Descriptor struct {
	Created *time.Time `json:"created,omitempty"`
}
