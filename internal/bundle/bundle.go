package bundle

import (
	"path/filepath"

	"github.com/docker/model-distribution/types"
)

// Bundle represents a runtime bundle containing a model and runtime config
type Bundle struct {
	dir           string
	mmprojPath    string
	ggufFile      string // path to GGUF file (first shard when model is split among files)
	runtimeConfig types.Config
	templatePath  string
}

// RootDir return the path to the bundle root directory
func (b *Bundle) RootDir() string {
	return b.dir
}

// GGUFPath return the path to model GGUF file. If the model is sharded this will be the path to the first shard,
// containing metadata headers.
func (b *Bundle) GGUFPath() string {
	if b.ggufFile == "" {
		return ""
	}
	return filepath.Join(b.dir, b.ggufFile)
}

// MMPROJPath returns the path to a multi-modal projector file or "" if none is present.
func (b *Bundle) MMPROJPath() string {
	if b.mmprojPath == "" {
		return ""
	}
	return filepath.Join(b.dir, b.mmprojPath)
}

// ChatTemplatePath return the path to a Jinja chat template file "" if none is present.
func (b *Bundle) ChatTemplatePath() string {
	if b.templatePath == "" {
		return ""
	}
	return filepath.Join(b.dir, b.templatePath)
}

// RuntimeConfig returns config that should be respected by the backend at runtime.
func (b *Bundle) RuntimeConfig() types.Config {
	return b.runtimeConfig
}
