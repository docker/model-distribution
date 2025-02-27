package types

// Store interface for model storage operations
type Store interface {
	// Push a model to the store with given tags
	Push(modelPath string, tags []string) error

	// Pull a model by tag
	Pull(tag string, destPath string) error

	// List all models in the store
	List() ([]Model, error)

	// Get model info by tag
	GetByTag(tag string) (*Model, error)

	// Delete a model by tag
	Delete(tag string) error

	// Add tags to an existing model
	AddTags(tag string, newTags []string) error

	// Remove tags from a model
	RemoveTags(tags []string) error

	// Get store version
	Version() string

	// Upgrade store to latest version
	Upgrade() error
}

// Model represents a model with its metadata and tags
type Model struct {
	ManifestDigest string   `json:"manifestDigest"`
	Tags           []string `json:"tags"`
}

// ModelIndex represents the index of all models in the store
type ModelIndex struct {
	Models []Model `json:"models"`
}

// StoreLayout represents the layout information of the store
type StoreLayout struct {
	Version string `json:"version"`
}

// ManifestReference represents a reference to a manifest in the store
type ManifestReference struct {
	Digest    string `json:"digest"`
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
}

// StoreOptions represents options for creating a store
type StoreOptions struct {
	RootPath string
}
