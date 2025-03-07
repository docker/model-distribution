package store

import (
	"os"
	"path/filepath"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

var _ v1.Image = &Model{}

type Model struct {
	rawManfiest []byte
	rawMConfigFile []byte
	layres	  []v1.Layer
}

func (m Model) Layers() ([]v1.Layer, error) {
	//TODO implement me
	panic("implement me")
}

func (m Model) MediaType() (types.MediaType, error) {
	//TODO implement me
	panic("implement me")
}

func (m Model) Size() (int64, error) {
	//TODO implement me
	panic("implement me")
}

func (m Model) ConfigName() (v1.Hash, error) {
	//TODO implement me
	panic("implement me")
}

func (m Model) ConfigFile() (*v1.ConfigFile, error) {
	//TODO implement me
	panic("implement me")
}

func (m Model) RawConfigFile() ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (m Model) Digest() (v1.Hash, error) {
	//TODO implement me
	panic("implement me")
}

func (m Model) Manifest() (*v1.Manifest, error) {
	return partial.Manifest(m)
}

func (m Model) RawManifest() ([]byte, error) {
	return m.rawManfiest, nil
}

func (m Model) LayerByDigest(hash v1.Hash) (v1.Layer, error) {
	//TODO implement me
	panic("implement me")
}

func (m Model) LayerByDiffID(hash v1.Hash) (v1.Layer, error) {
	//TODO implement me
	panic("implement me")
}

func new(rawManfiest []byte) *Model {
	os.RemoveAll(filepath.Join(tempDir, "test-model.gguf")
	return &Model{
		rawManfiest: rawManfiest,
		rawConfigFile
	}
}
