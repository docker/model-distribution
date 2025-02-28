package distribution

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/docker/model-distribution/pkg/image"
	"github.com/docker/model-distribution/pkg/store"
	"github.com/docker/model-distribution/pkg/types"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"
)

// Client provides model distribution functionality
type Client struct {
	store *store.LocalStore
}

// NewClient creates a new distribution client
func NewClient(storeRootPath string) (*Client, error) {
	s, err := store.New(types.StoreOptions{RootPath: storeRootPath})
	if err != nil {
		return nil, fmt.Errorf("initializing store: %w", err)
	}

	return &Client{
		store: s,
	}, nil
}

// PullModel pulls a model from a registry and returns the local file path
func (c *Client) PullModel(ctx context.Context, reference string) (string, error) {
	// Check if model exists in local store
	_, err := c.store.GetByTag(reference)
	if err == nil {
		// Model exists in local store, get its path
		return c.GetModelPath(reference)
	}

	// Model doesn't exist in local store, pull from remote
	ref, err := name.ParseReference(reference)
	if err != nil {
		return "", fmt.Errorf("parsing reference: %w", err)
	}

	img, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain), remote.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("pulling image: %w", err)
	}

	// Create a temporary file to store the model content
	tempFile, err := os.CreateTemp("", "model-*.gguf")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Get the model content from the image
	layers, err := img.Layers()
	if err != nil {
		return "", fmt.Errorf("getting layers: %w", err)
	}

	if len(layers) == 0 {
		return "", fmt.Errorf("no layers in image")
	}

	// Use the first layer (assuming there's only one for models)
	layer := layers[0]

	// Get the layer content
	rc, err := layer.Uncompressed()
	if err != nil {
		return "", fmt.Errorf("getting layer content: %w", err)
	}
	defer rc.Close()

	// Write the layer content to the temporary file
	if _, err := io.Copy(tempFile, rc); err != nil {
		return "", fmt.Errorf("writing layer content: %w", err)
	}

	// Push the model to the local store
	if err := c.store.Push(tempFile.Name(), []string{reference}); err != nil {
		return "", fmt.Errorf("storing model in local store: %w", err)
	}

	// Get the model path
	return c.GetModelPath(reference)
}

// GetModelPath returns the local file path for a model
func (c *Client) GetModelPath(reference string) (string, error) {
	// Get the direct path to the blob file
	blobPath, err := c.store.GetBlobPath(reference)
	if err != nil {
		return "", fmt.Errorf("getting blob path: %w", err)
	}

	// Return the path to the blob file
	return blobPath, nil
}

// ListModels returns all available models
func (c *Client) ListModels() ([]*types.Model, error) {
	models, err := c.store.List()
	if err != nil {
		return nil, fmt.Errorf("listing models: %w", err)
	}

	result := make([]*types.Model, len(models))
	for i, model := range models {
		modelCopy := model // Create a copy to avoid issues with the loop variable
		result[i] = &modelCopy
	}

	return result, nil
}

// GetModel returns a model by reference
func (c *Client) GetModel(reference string) (*types.Model, error) {
	model, err := c.store.GetByTag(reference)
	if err != nil {
		return nil, ErrModelNotFound
	}

	return model, nil
}

// PushModel pushes a model to a registry
func (c *Client) PushModel(ctx context.Context, source, reference string) error {
	// Parse the reference
	ref, err := name.ParseReference(reference)
	if err != nil {
		return fmt.Errorf("parsing reference: %w", err)
	}

	// Read the model file
	fileContent, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("reading model file: %w", err)
	}

	// Create layer from model content
	layer := static.NewLayer(fileContent, "application/vnd.docker.ai.model.file.v1+gguf")

	// Create image with layer
	img, err := image.CreateImage(layer)
	if err != nil {
		return fmt.Errorf("creating image: %w", err)
	}

	// Push the image
	if err := remote.Write(ref, img,
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithContext(ctx),
	); err != nil {
		return fmt.Errorf("pushing image: %w", err)
	}

	// Store the model in the local store
	if err := c.store.Push(source, []string{reference}); err != nil {
		return fmt.Errorf("storing model in local store: %w", err)
	}

	return nil
}

// DeleteModel deletes a model from a registry
func (c *Client) DeleteModel(ctx context.Context, reference string) error {
	// Parse the reference to validate it
	_, err := name.ParseReference(reference)
	if err != nil {
		return fmt.Errorf("parsing reference: %w", err)
	}

	// Get the model from the local store to get its digest
	model, err := c.store.GetByTag(reference)
	if err != nil {
		return fmt.Errorf("getting model from local store: %w", err)
	}

	// Create a reference with the digest
	digestRef, err := name.ParseReference(fmt.Sprintf("%s@%s", reference, model.ManifestDigest))
	if err != nil {
		return fmt.Errorf("parsing digest reference: %w", err)
	}

	// Delete the manifest from the registry
	// If this fails, we still want to delete from the local store
	registryErr := remote.Delete(digestRef,
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithContext(ctx),
	)
	if registryErr != nil {
		// Log the error but continue to delete from local store
		fmt.Printf("Warning: Failed to delete model from registry: %v\n", registryErr)
	}

	// Delete the model from the local store
	if err := c.store.Delete(reference); err != nil {
		return fmt.Errorf("deleting model from local store: %w", err)
	}

	// If we failed to delete from registry but succeeded in deleting from local store,
	// return the registry error
	if registryErr != nil {
		return fmt.Errorf("deleting model from registry: %w", registryErr)
	}

	return nil
}

// getImageFromLocalStore creates an image from a model in the local store
func (c *Client) getImageFromLocalStore(model *types.Model) (v1.Image, error) {
	// Get the direct path to the blob file
	blobPath, err := c.store.GetBlobPath(model.Tags[0])
	if err != nil {
		return nil, fmt.Errorf("getting blob path: %w", err)
	}

	// Read the model content directly from the blob file
	modelContent, err := os.ReadFile(blobPath)
	if err != nil {
		return nil, fmt.Errorf("reading model content: %w", err)
	}

	// Create layer from model content
	layer := static.NewLayer(modelContent, "application/vnd.docker.ai.model.file.v1+gguf")

	// Create image with layer
	return image.CreateImage(layer)
}
