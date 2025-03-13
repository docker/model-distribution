package store

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/docker/model-distribution/pkg/types"
)

const (
	// CurrentVersion is the current version of the store layout
	CurrentVersion = "1.0.0"
)

// LocalStore implements the Store interface for local storage
type LocalStore struct {
	rootPath string
}

// RootPath returns the root path of the store
func (s *LocalStore) RootPath() string {
	return s.rootPath
}

// New creates a new LocalStore
func New(opts types.StoreOptions) (*LocalStore, error) {
	store := &LocalStore{
		rootPath: opts.RootPath,
	}

	// Initialize store if it doesn't exist
	if err := store.initialize(); err != nil {
		return nil, fmt.Errorf("initializing store: %w", err)
	}

	return store, nil
}

// initialize creates the store directory structure if it doesn't exist
func (s *LocalStore) initialize() error {
	// Create root directory if it doesn't exist
	if err := os.MkdirAll(s.rootPath, 0755); err != nil {
		return fmt.Errorf("creating root directory: %w", err)
	}

	// Create blobs directory
	blobsDir := filepath.Join(s.rootPath, "blobs", "sha256")
	if err := os.MkdirAll(blobsDir, 0755); err != nil {
		return fmt.Errorf("creating blobs directory: %w", err)
	}

	// Create manifests directory
	manifestsDir := filepath.Join(s.rootPath, "manifests", "sha256")
	if err := os.MkdirAll(manifestsDir, 0755); err != nil {
		return fmt.Errorf("creating manifests directory: %w", err)
	}

	// Check if layout.json exists, create if not
	layoutPath := filepath.Join(s.rootPath, "layout.json")
	if _, err := os.Stat(layoutPath); os.IsNotExist(err) {
		layout := types.StoreLayout{
			Version: CurrentVersion,
		}
		layoutData, err := json.MarshalIndent(layout, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling layout: %w", err)
		}
		if err := os.WriteFile(layoutPath, layoutData, 0644); err != nil {
			return fmt.Errorf("writing layout file: %w", err)
		}
	}

	// Check if models.json exists, create if not
	modelsPath := filepath.Join(s.rootPath, "models.json")
	if _, err := os.Stat(modelsPath); os.IsNotExist(err) {
		models := types.ModelIndex{
			Models: []types.ModelInfo{},
		}
		modelsData, err := json.MarshalIndent(models, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling models: %w", err)
		}
		if err := os.WriteFile(modelsPath, modelsData, 0644); err != nil {
			return fmt.Errorf("writing models file: %w", err)
		}
	}

	return nil
}

// updateModelsIndex updates the models index with a new model
func (s *LocalStore) updateModelsIndex(manifestDigest string, tags []string, blobDigest string) error {
	// Ensure the manifest digest has the correct format (sha256:...)
	if !strings.Contains(manifestDigest, ":") {
		manifestDigest = fmt.Sprintf("sha256:%s", manifestDigest)
	}

	// Read the models index
	modelsPath := filepath.Join(s.rootPath, "models.json")
	modelsData, err := os.ReadFile(modelsPath)
	if err != nil {
		return fmt.Errorf("reading models file: %w", err)
	}

	// Unmarshal the models index
	var models types.ModelIndex
	if err := json.Unmarshal(modelsData, &models); err != nil {
		return fmt.Errorf("unmarshaling models: %w", err)
	}

	// Check if the model already exists
	var model *types.ModelInfo
	for i, m := range models.Models {
		if m.ID == manifestDigest {
			model = &models.Models[i]
			break
		}
	}

	if model == nil {
		// Model doesn't exist, add it
		models.Models = append(models.Models, types.ModelInfo{
			ID:    manifestDigest,
			Tags:  tags,
			Files: []string{fmt.Sprintf("sha256:%s", blobDigest)},
		})
	} else {
		// Model exists, update tags
		existingTags := make(map[string]bool)
		for _, tag := range model.Tags {
			existingTags[tag] = true
		}

		// Add new tags
		for _, tag := range tags {
			if !existingTags[tag] {
				model.Tags = append(model.Tags, tag)
				existingTags[tag] = true
			}
		}
	}

	// Marshal the models index
	modelsData, err = json.MarshalIndent(models, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling models: %w", err)
	}

	// Write the models index
	if err := os.WriteFile(modelsPath, modelsData, 0644); err != nil {
		return fmt.Errorf("writing models file: %w", err)
	}

	return nil
}

// List lists all models in the store
func (s *LocalStore) List() ([]types.ModelInfo, error) {
	// Read the models index
	modelsPath := filepath.Join(s.rootPath, "models.json")
	modelsData, err := os.ReadFile(modelsPath)
	if err != nil {
		return nil, fmt.Errorf("reading models file: %w", err)
	}

	// Unmarshal the models index
	var models types.ModelIndex
	if err := json.Unmarshal(modelsData, &models); err != nil {
		return nil, fmt.Errorf("unmarshaling models: %w", err)
	}

	return models.Models, nil
}

// Delete deletes a model by tag
func (s *LocalStore) Delete(tag string) error {
	// Read the models index
	modelsPath := filepath.Join(s.rootPath, "models.json")
	modelsData, err := os.ReadFile(modelsPath)
	if err != nil {
		return fmt.Errorf("reading models file: %w", err)
	}

	// Unmarshal the models index
	var models types.ModelIndex
	if err := json.Unmarshal(modelsData, &models); err != nil {
		return fmt.Errorf("unmarshaling models: %w", err)
	}

	// Find the model by tag
	var modelIndex = -1
	var tagIndex = -1
	for i, model := range models.Models {
		for j, modelTag := range model.Tags {
			if modelTag == tag {
				modelIndex = i
				tagIndex = j
				break
			}
		}
		if modelIndex != -1 {
			break
		}
	}

	if modelIndex == -1 {
		return fmt.Errorf("model with tag %s not found", tag)
	}

	// Remove the tag
	model := &models.Models[modelIndex]
	model.Tags = append(model.Tags[:tagIndex], model.Tags[tagIndex+1:]...)

	// If no more tags, remove the model
	if len(model.Tags) == 0 {
		models.Models = append(models.Models[:modelIndex], models.Models[modelIndex+1:]...)
	}

	// Marshal the models index
	modelsData, err = json.MarshalIndent(models, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling models: %w", err)
	}

	// Write the models index
	if err := os.WriteFile(modelsPath, modelsData, 0644); err != nil {
		return fmt.Errorf("writing models file: %w", err)
	}

	return nil
}

// AddTags adds tags to an existing model
func (s *LocalStore) AddTags(tag string, newTags []string) error {
	// Read the models index
	modelsPath := filepath.Join(s.rootPath, "models.json")
	modelsData, err := os.ReadFile(modelsPath)
	if err != nil {
		return fmt.Errorf("reading models file: %w", err)
	}

	// Unmarshal the models index
	var models types.ModelIndex
	if err := json.Unmarshal(modelsData, &models); err != nil {
		return fmt.Errorf("unmarshaling models: %w", err)
	}

	// Find the model in the index
	for i, m := range models.Models {
		if m.HasTag(tag) {
			// Add new tags
			existingTags := make(map[string]bool)
			for _, t := range m.Tags {
				existingTags[t] = true
			}

			for _, newTag := range newTags {
				if !existingTags[newTag] {
					models.Models[i].Tags = append(models.Models[i].Tags, newTag)
					existingTags[newTag] = true
				}
			}
			break
		}
	}

	// Marshal the models index
	modelsData, err = json.MarshalIndent(models, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling models: %w", err)
	}

	// Write the models index
	if err := os.WriteFile(modelsPath, modelsData, 0644); err != nil {
		return fmt.Errorf("writing models file: %w", err)
	}

	return nil
}

// RemoveTags removes tags from models
func (s *LocalStore) RemoveTags(tags []string) error {
	// Read the models index
	modelsPath := filepath.Join(s.rootPath, "models.json")
	modelsData, err := os.ReadFile(modelsPath)
	if err != nil {
		return fmt.Errorf("reading models file: %w", err)
	}

	// Unmarshal the models index
	var models types.ModelIndex
	if err := json.Unmarshal(modelsData, &models); err != nil {
		return fmt.Errorf("unmarshaling models: %w", err)
	}

	// Create a map of tags to remove
	tagsToRemove := make(map[string]bool)
	for _, tag := range tags {
		tagsToRemove[tag] = true
	}

	// Remove tags from models
	var modelsToRemove []int
	for i, model := range models.Models {
		var newTags []string
		for _, tag := range model.Tags {
			if !tagsToRemove[tag] {
				newTags = append(newTags, tag)
			}
		}
		models.Models[i].Tags = newTags

		// If no more tags, mark model for removal
		if len(models.Models[i].Tags) == 0 {
			modelsToRemove = append(modelsToRemove, i)
		}
	}

	// Remove models with no tags (in reverse order to avoid index issues)
	for i := len(modelsToRemove) - 1; i >= 0; i-- {
		index := modelsToRemove[i]
		models.Models = append(models.Models[:index], models.Models[index+1:]...)
	}

	// Marshal the models index
	modelsData, err = json.MarshalIndent(models, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling models: %w", err)
	}

	// Write the models index
	if err := os.WriteFile(modelsPath, modelsData, 0644); err != nil {
		return fmt.Errorf("writing models file: %w", err)
	}

	return nil
}

// Version returns the store version
func (s *LocalStore) Version() string {
	// Read the layout file
	layoutPath := filepath.Join(s.rootPath, "layout.json")
	layoutData, err := os.ReadFile(layoutPath)
	if err != nil {
		return "unknown"
	}

	// Unmarshal the layout
	var layout types.StoreLayout
	if err := json.Unmarshal(layoutData, &layout); err != nil {
		return "unknown"
	}

	return layout.Version
}

// Upgrade upgrades the store to the latest version
func (s *LocalStore) Upgrade() error {
	// Read the layout file
	layoutPath := filepath.Join(s.rootPath, "layout.json")
	layoutData, err := os.ReadFile(layoutPath)
	if err != nil {
		return fmt.Errorf("reading layout file: %w", err)
	}

	// Unmarshal the layout
	var layout types.StoreLayout
	if err := json.Unmarshal(layoutData, &layout); err != nil {
		return fmt.Errorf("unmarshaling layout: %w", err)
	}

	// Check if upgrade is needed
	if layout.Version == CurrentVersion {
		return nil
	}

	// Implement upgrade logic here
	// For now, just update the version
	layout.Version = CurrentVersion

	// Marshal the layout
	layoutData, err = json.MarshalIndent(layout, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling layout: %w", err)
	}

	// Write the layout file
	if err := os.WriteFile(layoutPath, layoutData, 0644); err != nil {
		return fmt.Errorf("writing layout file: %w", err)
	}

	return nil
}

// Write writes a model to the store
func (s *LocalStore) Write(mdl v1.Image, tags []string, progress chan<- v1.Update) error {
	cf, err := mdl.RawConfigFile()
	if err != nil {
		return fmt.Errorf("get raw config file: %w", err)
	}
	name, err := mdl.ConfigName()
	if err != nil {
		return fmt.Errorf("getting config name: %w", err)
	}
	configPath := filepath.Join(s.rootPath, "blobs", "sha256", name.Hex)
	f, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("opening config blob: %w", err)
	}
	defer f.Close()
	_, err = f.Write(cf)
	if err != nil {
		return fmt.Errorf("writing config blob: %w", err)
	}

	// Gets SHA256 digest
	//digest := manifest.Layers[0].Digest
	sz, err := mdl.Size()
	if err != nil {
		return fmt.Errorf("getting model size: %w", err)
	}

	layers, err := mdl.Layers()
	if err != nil {
		return fmt.Errorf("getting layers: %w", err)
	}

	var blobDigest v1.Hash
	for _, layer := range layers {
		d, err := layer.Digest()
		if err != nil {
			return fmt.Errorf("getting layer digest: %w", err)
		}
		blobPath := filepath.Join(s.rootPath, "blobs", "sha256", d.Hex)
		f, err := os.Create(blobPath)
		if err != nil {
			return fmt.Errorf("opening blob file: %w", err)
		}
		defer f.Close()
		lr, err := layer.Uncompressed()
		if err != nil {
			return fmt.Errorf("opening layer: %w", err)
		}
		defer lr.Close()

		var r io.Reader
		if progress != nil {
			r = &ProgressReader{
				Reader:       lr,
				ProgressChan: progress,
				Total:        sz,
			}
		} else {
			r = lr
		}
		if _, err = io.Copy(f, r); err != nil {
			return fmt.Errorf("writing layer content: %w", err)
		}

		mt, err := layer.MediaType()
		if err != nil {
			return fmt.Errorf("getting layer media type: %w", err)
		}

		if mt == types.MediaTypeGGUF {
			blobDigest = d
		}
	}

	rawManifest, err := mdl.RawManifest()
	if err != nil {
		return err
	}
	// Calculate manifest digest
	digest, err := mdl.Digest()
	if err != nil {
		return fmt.Errorf("getting model digest: %w", err)
	}

	// Store the manifest
	manifestPath := filepath.Join(s.rootPath, "manifests", digest.Algorithm, digest.Hex)
	if err := os.WriteFile(manifestPath, rawManifest, 0644); err != nil {
		return fmt.Errorf("writing manifest file: %w", err)
	}

	// Update the models index
	if err := s.updateModelsIndex(digest.Hex, tags, blobDigest.Hex); err != nil {
		return fmt.Errorf("updating models index: %w", err)
	}

	return nil
}

// Read reads a model from the store by tag
func (s *LocalStore) Read(tag string) (*Model, error) {
	// Read the models index
	modelsPath := filepath.Join(s.rootPath, "models.json")
	modelsData, err := os.ReadFile(modelsPath)
	if err != nil {
		return nil, fmt.Errorf("reading models file: %w", err)
	}

	// Unmarshal the models index
	var models types.ModelIndex
	if err := json.Unmarshal(modelsData, &models); err != nil {
		return nil, fmt.Errorf("unmarshaling models: %w", err)
	}

	// Find the model by tag
	for i, model := range models.Models {
		for _, modelTag := range model.Tags {
			if modelTag == tag {
				hash, err := v1.NewHash(model.ID)
				if err != nil {
					return nil, fmt.Errorf("parsing hash: %w", err)
				}
				rawManifest, err := os.ReadFile(filepath.Join(s.rootPath, "manifests", hash.Algorithm, hash.Hex))
				if err != nil {
					return nil, fmt.Errorf("reading manifest file: %w", err)
				}
				return &Model{
					rawManfiest: rawManifest,
					blobsDir:    filepath.Join(s.rootPath, "blobs", "sha256"),
					tags:        models.Models[i].Tags,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("model with tag %s not found", tag)
}

// ProgressReader wraps an io.Reader to track reading progress
type ProgressReader struct {
	Reader       io.Reader
	ProgressChan chan<- v1.Update
	Total        int64
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	if n > 0 {
		pr.Total += int64(n)
		pr.ProgressChan <- v1.Update{Complete: pr.Total}
	}
	return n, err
}
