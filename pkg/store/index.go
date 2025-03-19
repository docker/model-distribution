package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// Index represents the index of all models in the store
type Index struct {
	Models []IndexEntry `json:"models"`
}

func (i Index) Tag(reference string, tag string) (Index, error) {
	tagRef, err := name.NewTag(tag)
	if err != nil {
		return Index{}, fmt.Errorf("invalid tag: %w", err)
	}

	result := Index{}
	var tagged bool
	for _, entry := range i.Models {
		if entry.MatchesReference(reference) {
			result.Models = append(result.Models, entry.Tag(tagRef))
			tagged = true
		} else {
			result.Models = append(result.Models, entry.UnTag(tagRef))
		}
	}
	if !tagged {
		return Index{}, fmt.Errorf("model not found")
	}

	return result, nil
}

func (i Index) UnTag(tag string) Index {
	tagRef, err := name.NewTag(tag)
	if err != nil {
		return Index{}
	}

	result := Index{
		Models: make([]IndexEntry, 0, len(i.Models)),
	}
	for _, entry := range i.Models {
		result.Models = append(result.Models, entry.UnTag(tagRef))
	}

	return result
}

func (i Index) Find(reference string) (IndexEntry, int, bool) {
	for n, entry := range i.Models {
		if entry.MatchesReference(reference) {
			return i.Models[n], n, true
		}
	}

	return IndexEntry{}, 0, false
}

func (i Index) Remove(reference string) Index {
	var result Index
	for _, entry := range i.Models {
		if entry.MatchesReference(reference) {
			continue
		}
		result.Models = append(result.Models, entry)
	}

	return result
}

func (i Index) Add(entry IndexEntry) Index {
	_, _, ok := i.Find(entry.ID)
	if ok {
		return i
	}
	return Index{
		Models: append(i.Models, entry),
	}
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
func (s *LocalStore) readIndex() (Index, error) {
	// Read the models index
	modelsData, err := os.ReadFile(s.indexPath())
	if err != nil {
		return Index{}, fmt.Errorf("reading models file: %w", err)
	}

	// Unmarshal the models index
	var index Index
	if err := json.Unmarshal(modelsData, &index); err != nil {
		return Index{}, fmt.Errorf("unmarshaling models: %w", err)
	}

	return index, nil
}

// IndexEntry represents a model with its metadata and tags
type IndexEntry struct {
	// ID is the globally unique model identifier.
	ID string `json:"id"`
	// Tags are the list of tags associated with the model.
	Tags []string `json:"tags"`
	// Files are the GGUF files associated with the model.
	Files []string `json:"files"`
}

func newEntry(image v1.Image) (IndexEntry, error) {
	digest, err := image.Digest()
	if err != nil {
		return IndexEntry{}, fmt.Errorf("getting digest: %w", err)
	}

	layers, err := image.Layers()
	if err != nil {
		return IndexEntry{}, fmt.Errorf("getting layers: %w", err)
	}
	files := make([]string, len(layers)+1)
	for i, layer := range layers {
		diffID, err := layer.DiffID()
		if err != nil {
			return IndexEntry{}, fmt.Errorf("getting diffID: %w", err)
		}
		files[i] = diffID.String()
	}
	cfgName, err := image.ConfigName()
	if err != nil {
		return IndexEntry{}, fmt.Errorf("getting config name: %w", err)
	}
	files[len(layers)] = cfgName.String()

	return IndexEntry{
		ID:    digest.String(),
		Files: files,
	}, nil
}

func (e IndexEntry) HasTag(tag string) bool {
	ref, err := name.NewTag(tag)
	if err != nil {
		return false
	}
	for _, t := range e.Tags {
		tr, err := name.ParseReference(t)
		if err != nil {
			continue
		}
		if tr.Name() == ref.Name() {
			return true
		}
	}
	return false
}

func (e IndexEntry) hasTag(tag name.Tag) bool {
	for _, t := range e.Tags {
		tr, err := name.ParseReference(t)
		if err != nil {
			continue
		}
		if tr.Name() == tag.Name() {
			return true
		}
	}
	return false
}

func (e IndexEntry) MatchesReference(reference string) bool {
	if e.ID == reference {
		return true
	}
	ref, err := name.ParseReference(reference)
	if err != nil {
		return false
	}
	if dgst, ok := ref.(name.Digest); ok {
		if dgst.DigestStr() == e.ID {
			return true
		}
	}
	return e.HasTag(reference)
}

func (e IndexEntry) Tag(tag name.Tag) IndexEntry {
	if e.hasTag(tag) {
		return e
	}
	return IndexEntry{
		ID:    e.ID,
		Tags:  append(e.Tags, tag.String()),
		Files: e.Files,
	}
}

func (e IndexEntry) UnTag(tag name.Tag) IndexEntry {
	var tags []string
	for i, t := range e.Tags {
		tr, err := name.ParseReference(t)
		if err != nil {
			continue
		}
		if tr.Name() == tag.Name() {
			continue
		}
		tags = append(tags, e.Tags[i])
	}
	return IndexEntry{
		ID:    e.ID,
		Tags:  tags,
		Files: e.Files,
	}
}
