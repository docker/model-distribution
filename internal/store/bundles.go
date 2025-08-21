package store

import (
	"fmt"
	"os"
	"path/filepath"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/docker/model-distribution/internal/bundle"
)

const (
	bundlesDir = "bundles"
)

// manifestPath returns the path to the manifest file for the given hash.
func (s *LocalStore) bundlePath(hash v1.Hash) string {
	return filepath.Join(s.rootPath, bundlesDir, hash.Algorithm, hash.Hex)
}

func (s *LocalStore) BundlePathForModel(ref string) (string, error) {
	mdl, err := s.Read(ref)
	if err != nil {
		return "", fmt.Errorf("find model content: %w", err)
	}
	dgst, err := mdl.Digest()
	if err != nil {
		return "", fmt.Errorf("get model ID: %w", err)
	}
	path := s.bundlePath(dgst)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", fmt.Errorf("create bundle directory: %w", err)
	}
	if err := bundle.Unpack(path, mdl); err != nil {
		return "", fmt.Errorf("unpack bundle: %w", err)
	}
	return path, nil
}

func (s *LocalStore) removeBundle(hash v1.Hash) error {
	return os.RemoveAll(s.bundlePath(hash))
}
