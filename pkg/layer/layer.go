package layer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

const (
	MediaTypeGGUF = "application/vnd.docker.ai.model.file.v1+gguf"
)

// Raw implements v1.Layer for raw content
type Raw struct {
	content []byte
}

// New creates a new layer from raw content
func New(content []byte) v1.Layer {
	return &Raw{content: content}
}

func (l *Raw) Digest() (v1.Hash, error) {
	h := sha256.Sum256(l.content)
	return v1.Hash{
		Algorithm: "sha256",
		Hex:       hex.EncodeToString(h[:]),
	}, nil
}

func (l *Raw) DiffID() (v1.Hash, error) {
	return l.Digest()
}

func (l *Raw) Compressed() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(l.content)), nil
}

func (l *Raw) Uncompressed() (io.ReadCloser, error) {
	return l.Compressed()
}

func (l *Raw) Size() (int64, error) {
	return int64(len(l.content)), nil
}

func (l *Raw) MediaType() (types.MediaType, error) {
	return "application/vnd.docker.ai.model.file.v1+gguf", nil
}
