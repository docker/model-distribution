package distribution

import (
	"reflect"
	"testing"

	"github.com/docker/model-distribution/pkg/types"
)

func TestConvertToManagerModel(t *testing.T) {
	// Create a test model
	model := &types.Model{
		ManifestDigest: "sha256:1234567890abcdef",
		Tags:           []string{"test/model:v1.0.0", "test/model:latest"},
	}

	// Convert to manager model
	ggufPath := "/path/to/model.gguf"
	managerModel := ConvertToManagerModel(model, ggufPath)

	// Verify the conversion
	if managerModel.ID == "" {
		t.Error("Manager model ID is empty")
	}

	if !reflect.DeepEqual(managerModel.Tags, model.Tags) {
		t.Errorf("Manager model tags don't match: got %v, want %v", managerModel.Tags, model.Tags)
	}

	if len(managerModel.Files) != 1 || managerModel.Files[0] != ggufPath {
		t.Errorf("Manager model files don't match: got %v, want [%s]", managerModel.Files, ggufPath)
	}

	if managerModel.Created == 0 {
		t.Error("Manager model created timestamp is zero")
	}
}

func TestConvertToManagerModelList(t *testing.T) {
	// Create test models
	models := []*types.Model{
		{
			ManifestDigest: "sha256:1234567890abcdef",
			Tags:           []string{"test/model1:v1.0.0", "test/model1:latest"},
		},
		{
			ManifestDigest: "sha256:0987654321fedcba",
			Tags:           []string{"test/model2:v1.0.0"},
		},
		{
			ManifestDigest: "sha256:abcdef1234567890",
			Tags:           []string{},
		},
	}

	// Create paths map
	paths := map[string]string{
		"test/model1:v1.0.0": "/path/to/model1.gguf",
		"test/model2:v1.0.0": "/path/to/model2.gguf",
	}

	// Convert to manager model list
	managerModels := ConvertToManagerModelList(models, paths)

	// Verify the conversion
	if len(managerModels) != 2 {
		t.Errorf("Expected 2 manager models, got %d", len(managerModels))
	}

	// Check if all models have the correct properties
	for _, model := range managerModels {
		if model.ID == "" {
			t.Error("Manager model ID is empty")
		}

		if len(model.Tags) == 0 {
			t.Error("Manager model tags are empty")
		}

		if len(model.Files) != 1 {
			t.Errorf("Expected 1 file, got %d", len(model.Files))
		}

		if model.Created == 0 {
			t.Error("Manager model created timestamp is zero")
		}
	}
}

func TestToOpenAI(t *testing.T) {
	// Create a test manager model
	managerModel := &ManagerModel{
		ID:      "1234567890abcdef",
		Tags:    []string{"test/model:v1.0.0", "test/model:latest"},
		Files:   []string{"/path/to/model.gguf"},
		Created: 1625097600,
	}

	// Convert to OpenAI model
	openAIModel := managerModel.ToOpenAI()

	// Verify the conversion
	if openAIModel.ID != managerModel.ID {
		t.Errorf("OpenAI model ID doesn't match: got %s, want %s", openAIModel.ID, managerModel.ID)
	}

	if openAIModel.Object != "model" {
		t.Errorf("OpenAI model object doesn't match: got %s, want model", openAIModel.Object)
	}

	if openAIModel.Created != managerModel.Created {
		t.Errorf("OpenAI model created timestamp doesn't match: got %d, want %d", openAIModel.Created, managerModel.Created)
	}

	if openAIModel.OwnedBy != "organization-owner" {
		t.Errorf("OpenAI model owned_by doesn't match: got %s, want organization-owner", openAIModel.OwnedBy)
	}
}

func TestManagerModelListToOpenAI(t *testing.T) {
	// Create test manager models
	managerModels := ManagerModelList{
		{
			ID:      "1234567890abcdef",
			Tags:    []string{"test/model1:v1.0.0", "test/model1:latest"},
			Files:   []string{"/path/to/model1.gguf"},
			Created: 1625097600,
		},
		{
			ID:      "0987654321fedcba",
			Tags:    []string{"test/model2:v1.0.0"},
			Files:   []string{"/path/to/model2.gguf"},
			Created: 1625184000,
		},
	}

	// Convert to OpenAI model list
	openAIModelList := managerModels.ToOpenAI()

	// Verify the conversion
	if openAIModelList.Object != "list" {
		t.Errorf("OpenAI model list object doesn't match: got %s, want list", openAIModelList.Object)
	}

	if len(openAIModelList.Data) != len(managerModels) {
		t.Errorf("Expected %d OpenAI models, got %d", len(managerModels), len(openAIModelList.Data))
	}

	// Check if all models have the correct properties
	for i, model := range openAIModelList.Data {
		if model.ID != managerModels[i].ID {
			t.Errorf("OpenAI model ID doesn't match: got %s, want %s", model.ID, managerModels[i].ID)
		}

		if model.Object != "model" {
			t.Errorf("OpenAI model object doesn't match: got %s, want model", model.Object)
		}

		if model.Created != managerModels[i].Created {
			t.Errorf("OpenAI model created timestamp doesn't match: got %d, want %d", model.Created, managerModels[i].Created)
		}

		if model.OwnedBy != "organization-owner" {
			t.Errorf("OpenAI model owned_by doesn't match: got %s, want organization-owner", model.OwnedBy)
		}
	}
}
