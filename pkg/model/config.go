package model

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

const (
	MediaTypeModelConfig = types.MediaType("application/vnd.docker.ai.model.config.v1+json")
)

type ConfigFile struct {
	Config Config    `json:"config"`
	RootFS v1.RootFS `json:"rootfs"`
}

type Config struct {
	Format       string `json:"format,omitempty"`
	Quantization string `json:"quantization,omitempty"`
}
