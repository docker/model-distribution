package model

import (
	"io"
	"os"

	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"

	mdltypes "github.com/docker/model-distribution/pkg/types"
)

var _ v1.Layer = &Layer{}

type Layer struct {
	Path string
	v1.Descriptor
}

func NewLayer(path string) (*Layer, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	hash, size, err := v1.SHA256(f)
	return &Layer{
		Path: path,
		Descriptor: v1.Descriptor{
			Size:      size,
			Digest:    hash,
			MediaType: mdltypes.MediaTypeGGUF,
		},
	}, nil
}

func (l Layer) Digest() (v1.Hash, error) {

	return l.Descriptor.Digest, nil
}

func (l Layer) DiffID() (v1.Hash, error) {
	return l.Descriptor.Digest, nil
}

func (l Layer) Compressed() (io.ReadCloser, error) {
	return os.Open(l.Path)
}

func (l Layer) Uncompressed() (io.ReadCloser, error) {
	return os.Open(l.Path)
}

func (l Layer) Size() (int64, error) {
	return l.Descriptor.Size, nil
}

func (l Layer) MediaType() (types.MediaType, error) {
	return l.Descriptor.MediaType, nil
}
