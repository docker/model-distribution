package distribution

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/docker/model-distribution/pkg/gguf"
	"github.com/docker/model-distribution/pkg/store"
	"github.com/docker/model-distribution/pkg/types"
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

	s, err := store.New(store.Options{
		RootPath: options.storeRootPath,
	})
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
func (c *Client) PullModel(ctx context.Context, reference string, progressWriter io.Writer) error {
	c.log.Infoln("Starting model pull:", reference)

	// Clean up any existing incomplete files before starting
	c.store.CleanupIncompleteFiles()

	// Check if model exists in local store
	mdl, err := c.store.Read(reference)
	if err == nil {
		c.log.Infoln("Model found in local store:", reference)
		// Model exists in local store, get its path
		ggufPath, err := mdl.GGUFPath()
		if err != nil {
			return fmt.Errorf("getting gguf path: %w", err)
		}

		// Check if there are any incomplete files for this model
		incompleteFiles := false

		// Check if the GGUF file has an incomplete version
		if _, err := os.Stat(ggufPath + ".incomplete"); err == nil {
			c.log.Infoln("Found incomplete GGUF file for model:", reference)
			incompleteFiles = true
		}

		// If no incomplete files, use the cached model
		if !incompleteFiles {
			// Get file size for progress reporting
			fileInfo, err := os.Stat(ggufPath)
			if err != nil {
				return fmt.Errorf("getting file info: %w", err)
			}

			// Report progress for local model
			if progressWriter != nil {
				size := fileInfo.Size()
				fmt.Fprintf(progressWriter, "Using cached model: %.2f MB\n", float64(size)/1024/1024)
			}

			return nil
		}

		// If we found incomplete files, we'll pull the model again
		c.log.Infoln("Found incomplete files for model, will pull again:", reference)
	} else {
		c.log.Infoln("Model not found in local store, pulling from remote:", reference)
	}

	// Model doesn't exist in local store, pull from remote
	ref, err := name.ParseReference(reference)
	if err != nil {
		return NewReferenceError(reference, err)
	}

	// Create a buffered channel for progress updates
	progress := make(chan v1.Update, 100)
	defer close(progress)

	// Start a goroutine to handle progress updates
	go func() {
		var lastComplete int64
		var lastUpdate time.Time
		const updateInterval = 500 * time.Millisecond // Update every 500ms
		const minBytesForUpdate = 1024 * 1024         // At least 1MB difference

		for p := range progress {
			if progressWriter != nil {
				now := time.Now()
				bytesDownloaded := p.Complete - lastComplete

				// Only update if enough time has passed or enough bytes downloaded
				if now.Sub(lastUpdate) >= updateInterval || bytesDownloaded >= minBytesForUpdate {
					fmt.Fprintf(progressWriter, "Downloaded: %.2f MB\n", float64(p.Complete)/1024/1024)
					lastUpdate = now
					lastComplete = p.Complete
				}
			}
		}
	}()

	// Configure remote options with progress tracking
	remoteOpts := []remote.Option{
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithContext(ctx),
	}

	// Pull the image with progress tracking
	img, err := remote.Image(ref, remoteOpts...)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "UNAUTHORIZED") {
			return NewPullError(reference, "UNAUTHORIZED", "Authentication required for this model", err)
		}
		if strings.Contains(errStr, "MANIFEST_UNKNOWN") {
			return NewPullError(reference, "MANIFEST_UNKNOWN", "Model not found", err)
		}
		c.log.Errorln("Failed to pull image:", err, "reference:", reference)
		return NewPullError(reference, "UNKNOWN", err.Error(), err)
	}

	if err = c.store.Write(ctx, img, []string{reference}, progress); err != nil {
		return fmt.Errorf("writing image to store: %w", err)
	}

	return nil
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

// ListModels returns all available models
func (c *Client) ListModels() ([]types.Model, error) {
	c.log.Infoln("Listing available models")
	modelInfos, err := c.store.List()
	if err != nil {
		c.log.Errorln("Failed to list models:", err)
		return nil, fmt.Errorf("listing models: %w", err)
	}

	result := make([]types.Model, 0, len(modelInfos))
	for _, modelInfo := range modelInfos {
		// For each model info, find a tag to use for reading the model
		if len(modelInfo.Tags) > 0 {
			// Use the first tag to read the model
			model, err := c.store.Read(modelInfo.Tags[0])
			if err != nil {
				c.log.Warnf("Failed to read model with tag %s: %v", modelInfo.Tags[0], err)
				continue
			}
			result = append(result, model)
		}
	}

	c.log.Infoln("Successfully listed models, count:", len(result))
	return result, nil
}

// GetModel returns a model by reference
func (c *Client) GetModel(reference string) (types.Model, error) {
	c.log.Infoln("Getting model by reference:", reference)
	model, err := c.store.Read(reference)
	if err != nil {
		c.log.Errorln("Model not found:", err, "reference:", reference)
		return nil, ErrModelNotFound
	}

	return model, nil
}

// PushModel pushes a model to a registry
func (c *Client) PushModel(ctx context.Context, source, reference string) error {
	c.log.Infoln("Starting model push, source:", source, "reference:", reference)

	// Parse the reference
	ref, err := name.ParseReference(reference)
	if err != nil {
		c.log.Errorln("Failed to parse reference:", err, "reference:", reference)
		return fmt.Errorf("parsing reference: %w", err)
	}

	// Create image with layer
	mdl, err := gguf.NewModel(source)
	if err != nil {
		c.log.Errorln("Failed to create image:", err)
		return fmt.Errorf("creating image: %w", err)
	}

	// Push the image
	if err := remote.Write(ref, mdl,
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithContext(ctx),
	); err != nil {
		c.log.Errorln("Failed to push image:", err, "reference:", reference)
		return fmt.Errorf("pushing image: %w", err)
	}

	c.log.Infoln("Successfully pushed model:", reference)
	return nil
}

// DeleteModel deletes a model by tag
func (c *Client) DeleteModel(tag string) error {
	c.log.Infoln("Deleting model:", tag)
	if err := c.store.Delete(tag); err != nil {
		c.log.Errorln("Failed to delete model:", err, "tag:", tag)
		return fmt.Errorf("deleting model: %w", err)
	}
	c.log.Infoln("Successfully deleted model:", tag)
	return nil
}
