package store

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

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

// Options represents options for creating a store
type Options struct {
	RootPath string
}

// New creates a new LocalStore
func New(opts Options) (*LocalStore, error) {
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
		layout := Layout{
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
		models := Index{
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

// List lists all models in the store
func (s *LocalStore) List() ([]types.ModelInfo, error) {
	return s.readIndex()
}

// Delete deletes a model by tag
func (s *LocalStore) Delete(tag string) error {
	models, err := s.List()
	if err != nil {
		return fmt.Errorf("reading models file: %w", err)
	}

	// Find the model by tag
	var modelIndex = -1
	var tagIndex = -1
	for i, model := range models {
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
		// Model not found, nothing to delete
		return nil
	}

	// Get the model before removing it
	model := &models[modelIndex]

	// Remove the tag
	model.Tags = append(model.Tags[:tagIndex], model.Tags[tagIndex+1:]...)

	// If no more tags, remove the model and check if its blobs can be deleted
	if len(model.Tags) == 0 {
		// Remove manifest file
		if digest, err := v1.NewHash(model.ID); err != nil {
			fmt.Printf("Warning: failed to parse manifest digest %s: %v\n", digest, err)
		} else if err := os.Remove(filepath.Join(s.rootPath, "manifests", digest.Algorithm, digest.Hex)); err != nil {
			fmt.Printf("Warning: failed to remove manifest file %s: %v\n",
				filepath.Join(s.rootPath, "manifests", digest.Algorithm, digest.Hex), err,
			)
		}
		// Before deleting blobs, check if they are referenced by other models
		blobRefs := make(map[string]int)
		for _, m := range models {
			if m.ID == model.ID {
				continue // Skip the model being deleted
			}
			for _, file := range m.Files {
				blobRefs[file]++
			}
		}
		// Only delete blobs that are not referenced by other models
		for _, blobFile := range model.Files {
			if blobRefs[blobFile] > 0 {
				// Skip deletion if blob is referenced by other models
				continue
			}
			hash, err := v1.NewHash(blobFile)
			if err != nil {
				fmt.Printf("Warning: failed to parse blob hash %s: %v\n", blobFile, err)
				continue
			}
			blobPath := filepath.Join(s.rootPath, "blobs", hash.Algorithm, hash.Hex)
			if err := os.Remove(blobPath); err != nil {
				// Just log the error but don't fail the operation
				fmt.Printf("Warning: failed to remove blob file %s: %v\n", blobPath, err)
			}
		}

		// Remove the model from the list
		models = append(models[:modelIndex], models[modelIndex+1:]...)
	}

	return s.writeIndex(Index{Models: models})
}

// AddTags adds tags to an existing model
func (s *LocalStore) AddTags(tag string, newTags []string) error {
	models, err := s.List()
	if err != nil {
		return fmt.Errorf("reading models file: %w", err)
	}

	// Find the model in the index
	for i, m := range models {
		if m.HasTag(tag) {
			// Add new tags
			existingTags := make(map[string]bool)
			for _, t := range m.Tags {
				existingTags[t] = true
			}

			for _, newTag := range newTags {
				if !existingTags[newTag] {
					models[i].Tags = append(models[i].Tags, newTag)
					existingTags[newTag] = true
				}
			}
			break
		}
	}

	return s.writeIndex(Index{Models: models})
}

// RemoveTags removes tags from models
func (s *LocalStore) RemoveTags(tags []string) error {
	models, err := s.List()
	if err != nil {
		return fmt.Errorf("reading models file: %w", err)
	}
	// Create a map of tags to remove
	tagsToRemove := make(map[string]bool)
	for _, tag := range tags {
		tagsToRemove[tag] = true
	}

	// Remove tags from models
	var modelsToRemove []int
	for i, model := range models {
		var newTags []string
		for _, tag := range model.Tags {
			if !tagsToRemove[tag] {
				newTags = append(newTags, tag)
			}
		}
		models[i].Tags = newTags

		// If no more tags, mark model for removal
		if len(models[i].Tags) == 0 {
			modelsToRemove = append(modelsToRemove, i)
		}
	}

	// Remove models with no tags (in reverse order to avoid index issues)
	for i := len(modelsToRemove) - 1; i >= 0; i-- {
		index := modelsToRemove[i]
		models = append(models[:index], models[index+1:]...)
	}

	return s.writeIndex(Index{Models: models})
}

// Version returns the store version
func (s *LocalStore) Version() string {
	layout, err := s.readLayout()
	if err != nil {
		return "unknown"
	}

	return layout.Version
}

// Write writes a model to the store
func (s *LocalStore) Write(mdl v1.Image, tags []string, progress chan<- v1.Update) error {
	if progress != nil {
		defer close(progress)
	}
	cf, err := mdl.RawConfigFile()
	if err != nil {
		return fmt.Errorf("get raw config file: %w", err)
	}
	name, err := mdl.ConfigName()
	if err != nil {
		return fmt.Errorf("getting config name: %w", err)
	}
	configPath := filepath.Join(s.rootPath, "blobs", "sha256", name.Hex)
	configTempPath := configPath + ".incomplete"

	// Clean up any existing incomplete config file
	os.Remove(configTempPath)

	// Create the temporary config file
	f, err := os.Create(configTempPath)
	if err != nil {
		return fmt.Errorf("opening config blob: %w", err)
	}
	defer f.Close()
	_, err = f.Write(cf)
	if err != nil {
		os.Remove(configTempPath) // Clean up on error
		return fmt.Errorf("writing config blob: %w", err)
	}

	// Rename config file to final path after successful write
	if err := os.Rename(configTempPath, configPath); err != nil {
		os.Remove(configTempPath) // Clean up on error
		return fmt.Errorf("renaming config blob: %w", err)
	}

	// Gets SHA256 digest
	// digest := manifest.Layers[0].Digest
	sz, err := mdl.Size()
	if err != nil {
		return fmt.Errorf("getting model size: %w", err)
	}

	layers, err := mdl.Layers()
	if err != nil {
		return fmt.Errorf("getting layers: %w", err)
	}

	var blobDigest v1.Hash
	var createdTempFiles []string // Track created temp files for cleanup on error

	for _, layer := range layers {
		d, err := layer.Digest()
		if err != nil {
			// Clean up any temp files created so far
			for _, tempFile := range createdTempFiles {
				os.Remove(tempFile)
			}
			return fmt.Errorf("getting layer digest: %w", err)
		}
		blobPath := filepath.Join(s.rootPath, "blobs", "sha256", d.Hex)
		blobTempPath := blobPath + ".incomplete"

		// Clean up any existing incomplete file
		os.Remove(blobTempPath)

		// Create the temporary file
		f, err := os.Create(blobTempPath)
		if err != nil {
			// Clean up any temp files created so far
			for _, tempFile := range createdTempFiles {
				os.Remove(tempFile)
			}
			return fmt.Errorf("opening blob file: %w", err)
		}

		// Add to list of temp files for cleanup in case of error
		createdTempFiles = append(createdTempFiles, blobTempPath)

		defer f.Close()
		lr, err := layer.Uncompressed()
		if err != nil {
			// Clean up any temp files created so far
			for _, tempFile := range createdTempFiles {
				os.Remove(tempFile)
			}
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
			// Clean up any temp files created so far
			for _, tempFile := range createdTempFiles {
				os.Remove(tempFile)
			}
			return fmt.Errorf("writing layer content: %w", err)
		}

		// Close the file before renaming
		f.Close()

		// Rename to final path after successful write
		if err := os.Rename(blobTempPath, blobPath); err != nil {
			// Clean up any temp files created so far
			for _, tempFile := range createdTempFiles {
				os.Remove(tempFile)
			}
			return fmt.Errorf("renaming blob file: %w", err)
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

	models, err := s.List()
	if err != nil {
		return fmt.Errorf("reading models: %w", err)
	}

	// Check if the model already exists
	var model *types.ModelInfo
	for i, m := range models {
		if m.ID == digest.String() {
			model = &models[i]
			break
		}
	}

	if model == nil {
		// Model doesn't exist, add it
		models = append(models, types.ModelInfo{
			ID:    digest.String(),
			Tags:  tags,
			Files: []string{blobDigest.String()},
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

	return s.writeIndex(Index{Models: models})
}

// Read reads a model from the store by tag
func (s *LocalStore) Read(tag string) (*Model, error) {
	models, err := s.List()
	if err != nil {
		return nil, fmt.Errorf("reading models file: %w", err)
	}

	// Find the model by tag
	for i, model := range models {
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
					rawManifest: rawManifest,
					blobsDir:    filepath.Join(s.rootPath, "blobs", "sha256"),
					tags:        models[i].Tags,
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
