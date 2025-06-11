package types

import (
	"fmt"
	"strings"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

const (
	// modelConfigPrefix is the prefix for all versioned model config media types.
	modelConfigPrefix = "application/vnd.docker.ai.model.config"

	// MediaTypeModelConfigV01 is the media type for the model config json.
	MediaTypeModelConfigV01 = types.MediaType("application/vnd.docker.ai.model.config.v0.1+json")

	// MediaTypeGGUF indicates a file in GGUF version 3 format, containing a tensor model.
	MediaTypeGGUF = types.MediaType("application/vnd.docker.ai.gguf.v3")

	// MediaTypeLicense indicates a plain text file containing a license
	MediaTypeLicense = types.MediaType("application/vnd.docker.ai.license")

	FormatGGUF = Format("gguf")

	// IOTypeText Valid IO types
	IOTypeText      = "text"
	IOTypeEmbedding = "embedding"
	IOTypeImage     = "image"
	IOTypeAudio     = "audio"
	IOTypeVideo     = "video"
)

func IsModelConfig(mt types.MediaType) bool {
	return strings.HasPrefix(string(mt), string(MediaTypeModelConfigV01))
}

type Format string

type ConfigFile struct {
	Config     Config     `json:"config"`
	Descriptor Descriptor `json:"descriptor"`
	RootFS     v1.RootFS  `json:"rootfs"`
}

// IOTypes represents the input and output types a model can handle
type IOTypes struct {
	Input  []string `json:"input" validate:"dive,oneof=text embedding image audio video"`
	Output []string `json:"output" validate:"dive,oneof=text embedding image audio video"`
}

// Validate validates the IOTypes
func (io *IOTypes) Validate() error {
	validTypes := map[string]bool{
		IOTypeText:      true,
		IOTypeEmbedding: true,
		IOTypeImage:     true,
		IOTypeAudio:     true,
		IOTypeVideo:     true,
	}

	// Check for duplicates and validate input types
	seen := make(map[string]bool)
	for _, t := range io.Input {
		t = strings.TrimSpace(t)
		if !validTypes[t] {
			return fmt.Errorf("invalid input type: %s", t)
		}
		if seen[t] {
			return fmt.Errorf("duplicate input type: %s", t)
		}
		seen[t] = true
	}

	// Check for duplicates and validate output types
	seen = make(map[string]bool)
	for _, t := range io.Output {
		t = strings.TrimSpace(t)
		if !validTypes[t] {
			return fmt.Errorf("invalid output type: %s", t)
		}
		if seen[t] {
			return fmt.Errorf("duplicate output type: %s", t)
		}
		seen[t] = true
	}

	return nil
}

// Capabilities describes what the model can do
type Capabilities struct {
	IO        IOTypes `json:"io"`
	ToolUsage *bool   `json:"tool_usage"`
}

// Validate validates the Capabilities
func (c *Capabilities) Validate() error {
	if c == nil {
		return fmt.Errorf("capabilities cannot be nil")
	}
	return c.IO.Validate()
}

// NewCapabilities creates a new Capabilities with validation
func NewCapabilities(input, output []string, toolUsage bool) (*Capabilities, error) {
	capabilities := &Capabilities{
		IO: IOTypes{
			Input:  input,
			Output: output,
		},
		ToolUsage: &toolUsage,
	}
	if err := capabilities.Validate(); err != nil {
		return nil, err
	}
	return capabilities, nil
}

// Config describes the model.
type Config struct {
	Format       Format            `json:"format,omitempty"`
	Quantization string            `json:"quantization,omitempty"`
	Parameters   string            `json:"parameters,omitempty"`
	Architecture string            `json:"architecture,omitempty"`
	Size         string            `json:"size,omitempty"`
	GGUF         map[string]string `json:"gguf,omitempty"`
	Capabilities *Capabilities     `json:"capabilities,omitempty"`
}

// Descriptor provides metadata about the provenance of the model.
type Descriptor struct {
	Created *time.Time `json:"created,omitempty"`
}
