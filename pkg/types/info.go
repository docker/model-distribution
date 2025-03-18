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

func (m ModelInfo) HasTag(tag string) bool {
	for _, t := range m.Tags {
		if t == tag {
			return true
		}
	}
	return false
}
