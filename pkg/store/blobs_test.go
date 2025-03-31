package store

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/static"
)

func TestBlobs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "blob-test")
	if err != nil {
		t.Fatalf("error creating temp dir: %v", err)
	}
	rootDir := filepath.Join(tmpDir, "store")
	store, err := New(Options{RootPath: rootDir})
	if err != nil {
		t.Fatalf("error creating store: %v", err)
	}

	t.Run("writeBlob with missing dir", func(t *testing.T) {
		// remove blobs directory to ensure it is recreated as needed
		if err := os.RemoveAll(store.blobsDir()); err != nil {
			t.Fatalf("expected blobs directory not be present")
		}

		// create the blob
		expectedContent := "some data"
		blob := static.NewLayer([]byte(expectedContent), "application/vnd.example.some.mt")
		hash, err := blob.DiffID()
		if err != nil {
			t.Fatalf("error getting blob hash: %v", err)
		}

		// write the blob
		if err := store.writeBlob(blob, nil); err != nil {
			t.Fatalf("error writing blob: %v", err)
		}

		// ensure blob file exists
		content, err := os.ReadFile(store.blobPath(hash))
		if err != nil {
			t.Fatalf("error reading blob file: %v", err)
		}

		// ensure correct content
		if string(content) != expectedContent {
			t.Fatalf("unexpected blob content: got %v expected %s", string(content), expectedContent)
		}

		// ensure incomplete blob file does not exist
		tmpFile := incompletePath(store.blobPath(hash))
		if _, err := os.Stat(tmpFile); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected incomplete blob file %s not be present", tmpFile)
		}
	})

	t.Run("writeBlob fails", func(t *testing.T) {
		// simulate lingering incomplete blob file (if program crashed)
		hash := v1.Hash{
			Algorithm: "some-alg",
			Hex:       "some-hash",
		}
		if err := writeFile(incompletePath(store.blobPath(hash)), []byte("incomplete")); err != nil {
			t.Fatalf("error creating incomplete blob file for test: %v", err)
		}

		if err := store.writeBlob(&fakeBlob{
			readCloser: &errorReader{},
			hash:       hash,
		}, nil); err == nil {
			t.Fatalf("expected error writing blob")
		}

		// ensure blob file does not exist
		if _, err := os.ReadFile(store.blobPath(hash)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected blob file not to exist")
		}

		// ensure incomplete file is not left behind
		if _, err := os.ReadFile(incompletePath(store.blobPath(hash))); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected incomplete blob file not to exist")
		}
	})

	t.Run("writeBlob reuses existing blob", func(t *testing.T) {
		// simulate existing blob
		hash := v1.Hash{
			Algorithm: "some-alg",
			Hex:       "some-hash",
		}
		if err := writeFile(store.blobPath(hash), []byte("some-data")); err != nil {
			t.Fatalf("error creating incomplete blob file for test: %v", err)
		}

		if err := store.writeBlob(&fakeBlob{
			readCloser: &errorReader{}, // will error if existing blob is not reused
			hash:       hash,
		}, nil); err != nil {
			t.Fatalf("error writing blob: %v", err)
		}

		// ensure blob file exists
		content, err := os.ReadFile(store.blobPath(hash))
		if err != nil {
			t.Fatalf("error reading blob file: %v", err)
		}

		// ensure correct content
		if string(content) != "some-data" {
			t.Fatalf("unexpected blob content: got %v expected %s", string(content), "some-data")
		}
	})
}

type fakeBlob struct {
	readCloser io.ReadCloser
	hash       v1.Hash
}

func (f fakeBlob) DiffID() (v1.Hash, error) {
	return f.hash, nil
}

func (f fakeBlob) Uncompressed() (io.ReadCloser, error) {
	return f.readCloser, nil
}

var _ io.Reader = &errorReader{}

type errorReader struct {
}

func (e errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("fake error")
}

func (e errorReader) Close() error {
	return nil
}
