package partial

import (
	"encoding/json"
	"fmt"

	"github.com/google/go-containerregistry/pkg/v1/partial"

	"github.com/docker/model-distribution/pkg/types"
)

type WithRawConfigFile interface {
	// RawConfigFile returns the serialized bytes of this model's config file.
	RawConfigFile() ([]byte, error)
}

func ConfigFile(i WithRawConfigFile) (*types.ConfigFile, error) {
	raw, err := i.RawConfigFile()
	if err != nil {
		return nil, fmt.Errorf("get raw config file: %w", err)
	}
	var cf types.ConfigFile
	if err := json.Unmarshal(raw, &cf); err != nil {
		return nil, fmt.Errorf("unmarshal : %w", err)
	}
	return &cf, nil
}

// Config returns the types.Config for the model.
func Config(i WithRawConfigFile) (types.Config, error) {
	cf, err := ConfigFile(i)
	if err != nil {
		return types.Config{}, fmt.Errorf("config file: %w", err)
	}
	return cf.Config, nil
}

// Descriptor returns the types.Descriptor for the model.
func Descriptor(i WithRawConfigFile) (types.Descriptor, error) {
	cf, err := ConfigFile(i)
	if err != nil {
		return types.Descriptor{}, fmt.Errorf("config file: %w", err)
	}
	return cf.Descriptor, nil
}

// WithRawManifest defines the subset of types.Model used by these helper methods
type WithRawManifest interface {
	// RawManifest returns the serialized bytes of this model's manifest file.
	RawManifest() ([]byte, error)
}

func ID(i WithRawManifest) (string, error) {
	digest, err := partial.Digest(i)
	if err != nil {
		return "", fmt.Errorf("get digest: %w", err)
	}
	return digest.String(), nil
}
