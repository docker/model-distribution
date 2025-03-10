package store

import (
	"io"
	"os"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

var _ v1.Layer = &Layer{}

type Layer struct {
	path string
	v1.Descriptor
}

func (l Layer) Digest() (v1.Hash, error) {
	return l.Descriptor.Digest, nil
}

func (l Layer) DiffID() (v1.Hash, error) {
	return l.Descriptor.Digest, nil
}

func (l Layer) Compressed() (io.ReadCloser, error) {
	return os.Open(l.path)
}

func (l Layer) Uncompressed() (io.ReadCloser, error) {
	return os.Open(l.path)
}

func (l Layer) Size() (int64, error) {
	return l.Descriptor.Size, nil
}

func (l Layer) MediaType() (types.MediaType, error) {
	return l.Descriptor.MediaType, nil
}
