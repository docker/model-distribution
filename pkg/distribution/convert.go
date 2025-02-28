package distribution

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/docker/model-distribution/pkg/types"
)

// ManagerModel represents the model structure expected by the model manager
type ManagerModel struct {
	ID      string   `json:"id"`
	Tags    []string `json:"tags"`
	Files   []string `json:"files"`
	Created int64    `json:"created"`
}

// ManagerModelList represents a list of models in the model manager format
type ManagerModelList []*ManagerModel

// ConvertToManagerModel converts a model-distribution Model to a model manager Model
func ConvertToManagerModel(model *types.Model, ggufPath string) *ManagerModel {
	// Create a model manager compatible model
	return &ManagerModel{
		ID:      fmt.Sprintf("%x", sha256.Sum256([]byte(model.Tags[0]))),
		Tags:    model.Tags,
		Files:   []string{ggufPath},
		Created: time.Now().Unix(),
	}
}

// ConvertToManagerModelList converts a list of model-distribution Models to a model manager ModelList
func ConvertToManagerModelList(models []*types.Model, ggufPaths map[string]string) ManagerModelList {
	result := make(ManagerModelList, 0, len(models))

	for _, model := range models {
		if len(model.Tags) == 0 {
			continue
		}

		// Use the first tag as the reference
		reference := model.Tags[0]

		// Get the GGUF path for this model
		ggufPath, ok := ggufPaths[reference]
		if !ok {
			// If no path is available, use an empty string
			ggufPath = ""
		}

		result = append(result, ConvertToManagerModel(model, ggufPath))
	}

	return result
}

// OpenAIModel represents a model in the OpenAI format
type OpenAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// OpenAIModelList represents a list of models in the OpenAI format
type OpenAIModelList struct {
	Object string        `json:"object"`
	Data   []OpenAIModel `json:"data"`
}

// ToOpenAI converts a ManagerModel to an OpenAI model
func (m *ManagerModel) ToOpenAI() OpenAIModel {
	return OpenAIModel{
		ID:      m.ID,
		Object:  "model",
		Created: m.Created,
		OwnedBy: "organization-owner",
	}
}

// ToOpenAI converts a ManagerModelList to an OpenAI model list
func (ml ManagerModelList) ToOpenAI() OpenAIModelList {
	result := OpenAIModelList{
		Object: "list",
		Data:   make([]OpenAIModel, 0, len(ml)),
	}

	for _, model := range ml {
		result.Data = append(result.Data, model.ToOpenAI())
	}

	return result
}
