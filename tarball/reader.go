package tarball

import (
	"archive/tar"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"io"
	"log"
)

type Reader struct {
	tr tar.Reader
}

type Blob struct {
	diffID v1.Hash
	rc     io.ReadCloser
}

func (r Reader) NextBlob() (*Blob, error) {
	hdr, err := r.tr.Next()
	if err != nil {
		return nil, err
	}
	for {
		hdr, err := r.tr.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			log.Fatalf("Error reading tar entry: %v", err)
		}
	}
}

func NewReader(rc io.ReadCloser) *Reader {
	return &Reader{
		tr: tar.NewReader(rc),
	}
}
