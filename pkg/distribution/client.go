package distribution

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/sirupsen/logrus"

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
	log   *logrus.Entry
}

// GetStorePath returns the root path where models are stored
func (c *Client) GetStorePath() string {
	return c.store.RootPath()
}

// ClientOptions represents options for creating a new Client
type ClientOptions struct {
	storeRootPath string
	logger        *logrus.Entry
}

// WithStoreRootPath sets the store root path
func WithStoreRootPath(path string) func(*ClientOptions) {
	return func(o *ClientOptions) {
		o.storeRootPath = path
	}
}

// WithLogger sets the logger
func WithLogger(logger *logrus.Entry) func(*ClientOptions) {
	return func(o *ClientOptions) {
		o.logger = logger
	}
}

// NewClient creates a new distribution client
func NewClient(opts ...func(*ClientOptions)) (*Client, error) {
	options := &ClientOptions{
		logger: logrus.NewEntry(logrus.StandardLogger()),
	}
	for _, opt := range opts {
		opt(options)
	}

	if options.storeRootPath == "" {
		return nil, fmt.Errorf("store root path is required")
	}

	s, err := store.New(types.StoreOptions{RootPath: options.storeRootPath})
	if err != nil {
		return nil, fmt.Errorf("initializing store: %w", err)
	}

	options.logger.Infoln("Successfully initialized store")
	return &Client{
		store: s,
		log:   options.logger,
	}, nil
}

// PullModel pulls a model from a registry and returns the local file path
func (c *Client) PullModel(ctx context.Context, reference string) (string, error) {
	c.log.WithField("reference", reference).Infoln("Starting model pull")

	// Check if model exists in local store
	_, err := c.store.GetByTag(reference)
	if err == nil {
		c.log.WithField("reference", reference).Infoln("Model found in local store")
		// Model exists in local store, get its path
		return c.GetModelPath(reference)
	}

	c.log.WithField("reference", reference).Infoln("Model not found in local store, pulling from remote")
	// Model doesn't exist in local store, pull from remote
	ref, err := name.ParseReference(reference)
	if err != nil {
		c.log.WithError(err).WithField("reference", reference).Error("Failed to parse reference")
		return "", fmt.Errorf("parsing reference: %w", err)
	}

	img, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain), remote.WithContext(ctx))
	if err != nil {
		c.log.WithError(err).WithField("reference", reference).Error("Failed to pull image")
		return "", fmt.Errorf("pulling image: %w", err)
	}

	// Create a temporary file to store the model content
	tempFile, err := os.CreateTemp("", "model-*.gguf")
	if err != nil {
		c.log.WithError(err).Error("Failed to create temporary file")
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Get the model content from the image
	layers, err := img.Layers()
	if err != nil {
		c.log.WithError(err).Error("Failed to get image layers")
		return "", fmt.Errorf("getting layers: %w", err)
	}

	if len(layers) == 0 {
		c.log.Error("No layers found in image")
		return "", fmt.Errorf("no layers in image")
	}

	// Use the first layer (assuming there's only one for models)
	layer := layers[0]

	// Get the layer content
	rc, err := layer.Uncompressed()
	if err != nil {
		c.log.WithError(err).Error("Failed to get layer content")
		return "", fmt.Errorf("getting layer content: %w", err)
	}
	defer rc.Close()

	// Write the layer content to the temporary file
	if _, err := io.Copy(tempFile, rc); err != nil {
		c.log.WithError(err).Error("Failed to write layer content")
		return "", fmt.Errorf("writing layer content: %w", err)
	}

	// Push the model to the local store
	if err := c.store.Push(tempFile.Name(), []string{reference}); err != nil {
		c.log.WithError(err).WithField("reference", reference).Error("Failed to store model in local store")
		return "", fmt.Errorf("storing model in local store: %w", err)
	}

	c.log.WithField("reference", reference).Info("Successfully pulled and stored model")
	// Get the model path
	return c.GetModelPath(reference)
}

// GetModelPath returns the local file path for a model
func (c *Client) GetModelPath(reference string) (string, error) {
	c.log.WithField("reference", reference).Infoln("Getting model path")
	// Get the direct path to the blob file
	blobPath, err := c.store.GetBlobPath(reference)
	if err != nil {
		c.log.WithError(err).WithField("reference", reference).Error("Failed to get blob path")
		return "", fmt.Errorf("getting blob path: %w", err)
	}

	return blobPath, nil
}

// ListModels returns all available models
func (c *Client) ListModels() ([]*types.Model, error) {
	c.log.Infoln("Listing available models")
	models, err := c.store.List()
	if err != nil {
		c.log.WithError(err).Error("Failed to list models")
		return nil, fmt.Errorf("listing models: %w", err)
	}

	result := make([]*types.Model, len(models))
	for i, model := range models {
		modelCopy := model // Create a copy to avoid issues with the loop variable
		result[i] = &modelCopy
	}

	c.log.WithField("count", len(result)).Infoln("Successfully listed models")
	return result, nil
}

// GetModel returns a model by reference
func (c *Client) GetModel(reference string) (*types.Model, error) {
	c.log.WithField("reference", reference).Infoln("Getting model by reference")
	model, err := c.store.GetByTag(reference)
	if err != nil {
		c.log.WithError(err).WithField("reference", reference).Error("Model not found")
		return nil, ErrModelNotFound
	}

	return model, nil
}

// PushModel pushes a model to a registry
func (c *Client) PushModel(ctx context.Context, source, reference string) error {
	c.log.WithFields(logrus.Fields{
		"source":    source,
		"reference": reference,
	}).Infoln("Starting model push")

	// Parse the reference
	ref, err := name.ParseReference(reference)
	if err != nil {
		c.log.WithError(err).WithField("reference", reference).Error("Failed to parse reference")
		return fmt.Errorf("parsing reference: %w", err)
	}

	// Read the model file
	fileContent, err := os.ReadFile(source)
	if err != nil {
		c.log.WithError(err).WithField("source", source).Error("Failed to read model file")
		return fmt.Errorf("reading model file: %w", err)
	}

	// Create layer from model content
	layer := static.NewLayer(fileContent, "application/vnd.docker.ai.model.file.v1+gguf")

	// Create image with layer
	img, err := image.CreateImage(layer)
	if err != nil {
		c.log.WithError(err).Error("Failed to create image")
		return fmt.Errorf("creating image: %w", err)
	}

	// Push the image
	if err := remote.Write(ref, img,
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithContext(ctx),
	); err != nil {
		c.log.WithError(err).WithField("reference", reference).Error("Failed to push image")
		return fmt.Errorf("pushing image: %w", err)
	}

	// Store the model in the local store
	if err := c.store.Push(source, []string{reference}); err != nil {
		c.log.WithError(err).WithField("reference", reference).Error("Failed to store model in local store")
		return fmt.Errorf("storing model in local store: %w", err)
	}

	c.log.WithField("reference", reference).Info("Successfully pushed model")
	return nil
}

// getImageFromLocalStore creates an image from a model in the local store
func (c *Client) getImageFromLocalStore(model *types.Model) (v1.Image, error) {
	c.log.WithField("model", model.Tags[0]).Infoln("Getting image from local store")

	// Get the direct path to the blob file
	blobPath, err := c.store.GetBlobPath(model.Tags[0])
	if err != nil {
		c.log.WithError(err).WithField("model", model.Tags[0]).Error("Failed to get blob path")
		return nil, fmt.Errorf("getting blob path: %w", err)
	}

	// Read the model content directly from the blob file
	modelContent, err := os.ReadFile(blobPath)
	if err != nil {
		c.log.WithError(err).WithField("path", blobPath).Error("Failed to read model content")
		return nil, fmt.Errorf("reading model content: %w", err)
	}

	// Create layer from model content
	layer := static.NewLayer(modelContent, "application/vnd.docker.ai.model.file.v1+gguf")

	// Create image with layer
	img, err := image.CreateImage(layer)
	if err != nil {
		c.log.WithError(err).Error("Failed to create image from layer")
		return nil, err
	}

	return img, nil
}

// DeleteModel deletes a model by tag
func (c *Client) DeleteModel(tag string) error {
	c.log.WithField("tag", tag).Infoln("Deleting model")
	if err := c.store.Delete(tag); err != nil {
		c.log.WithError(err).WithField("tag", tag).Error("Failed to delete model")
		return fmt.Errorf("deleting model: %w", err)
	}
	c.log.WithField("tag", tag).Info("Successfully deleted model")
	return nil
}
