package store

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

const (
	blobsDir = "blobs"
)

// blobDir returns the path to the blobs directory
func (s *LocalStore) blobsDir() string {
	return filepath.Join(s.rootPath, blobsDir)
}

// blobPath returns the path to the blob for the given hash.
func (s *LocalStore) blobPath(hash v1.Hash) string {
	return filepath.Join(s.rootPath, blobsDir, hash.Algorithm, hash.Hex)
}

type blob interface {
	DiffID() (v1.Hash, error)
	Uncompressed() (io.ReadCloser, error)
}

// writeBlob write the blob to the store, reporting progress to the given channel.
// If the blob is already in the store, it is a no-op.
func (s *LocalStore) writeBlob(layer blob, progress chan<- v1.Update) error {
	hash, err := layer.DiffID()
	if err != nil {
		return fmt.Errorf("get file hash: %w", err)
	}
	if s.hasBlob(hash) {
		// todo: write something to the progress channel (we probably need to redo progress reporting a little bit)
		return nil
	}

	path := s.blobPath(hash)
	lr, err := layer.Uncompressed()
	if err != nil {
		return fmt.Errorf("get blob contents: %w", err)
	}
	defer lr.Close()
	r := withProgress(lr, progress)

	f, err := createFile(incompletePath(path))
	if err != nil {
		return fmt.Errorf("create blob file: %w", err)
	}
	defer os.Remove(incompletePath(path))
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("copy blob %q to store: %w", hash.String(), err)
	}
	if err := os.Rename(incompletePath(path), path); err != nil {
		return fmt.Errorf("rename blob file: %w", err)
	}
	return nil
}

// removeBlob removes the blob with the given hash from the store.
func (s *LocalStore) removeBlob(hash v1.Hash) error {
	return os.Remove(s.blobPath(hash))
}

func (s *LocalStore) hasBlob(hash v1.Hash) bool {
	if _, err := os.Stat(s.blobPath(hash)); err == nil {
		return true
	}
	return false
}

// createFile is a wrapper around os.Create that creates any parent directories as needed.
func createFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0777); err != nil {
		return nil, fmt.Errorf("create parent directory %q: %w", filepath.Dir(path), err)
	}
	return os.Create(path)
}

// withProgress returns a reader that reports progress to the given channel.
func withProgress(r io.Reader, progress chan<- v1.Update) io.Reader {
	if progress == nil {
		return r
	}
	return &ProgressReader{
		Reader:       r,
		ProgressChan: progress,
	}
}

// incompletePath returns the path to the incomplete file for the given path.
func incompletePath(path string) string {
	return path + ".incomplete"
}

// writeConfigFile writes the model config JSON file to the blob store
func (s *LocalStore) writeConfigFile(mdl v1.Image) error {
	hash, err := mdl.ConfigName()
	if err != nil {
		return fmt.Errorf("get digest: %w", err)
	}
	rcf, err := mdl.RawConfigFile()
	if err != nil {
		return fmt.Errorf("get raw manifest: %w", err)
	}
	return writeFile(s.blobPath(hash), rcf)
}
