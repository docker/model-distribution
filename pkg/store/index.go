package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/model-distribution/pkg/types"
)

// Index represents the index of all models in the store
type Index struct {
	Models []types.ModelInfo `json:"models"`
}

// indexPath returns the path to the index file
func (s *LocalStore) indexPath() string {
	return filepath.Join(s.rootPath, "models.json")
}

// writeIndex writes the index to the index file
func (s *LocalStore) writeIndex(index Index) error {
	// Marshal the models index
	modelsData, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling models: %w", err)
	}

	// Write the models index
	if err := os.WriteFile(s.indexPath(), modelsData, 0644); err != nil {
		return fmt.Errorf("writing models file: %w", err)
	}

	return nil
}

// readIndex reads the index from the index file
func (s *LocalStore) readIndex() ([]types.ModelInfo, error) {
	// Read the models index
	modelsData, err := os.ReadFile(s.indexPath())
	if err != nil {
		return nil, fmt.Errorf("reading models file: %w", err)
	}

	// Unmarshal the models index
	var index Index
	if err := json.Unmarshal(modelsData, &index); err != nil {
		return nil, fmt.Errorf("unmarshaling models: %w", err)
	}

	return index.Models, nil
}
