package types

// ModelInfo represents a model with its metadata and tags
type ModelInfo struct {
	// ID is the globally unique model identifier.
	ID string `json:"id"`
	// Tags are the list of tags associated with the model.
	Tags []string `json:"tags"`
	// Files are the GGUF files associated with the model.
	Files []string `json:"files"`
}

// ModelIndex represents the index of all models in the store
type ModelIndex struct {
	Models []ModelInfo `json:"models"`
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
