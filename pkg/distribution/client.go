package distribution

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
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

const (
	defaultUserAgent = "model-distribution"
)

// Client provides model distribution functionality
type Client struct {
	store         *store.LocalStore
	log           *logrus.Entry
	remoteOptions []remote.Option
}

// GetStorePath returns the root path where models are stored
func (c *Client) GetStorePath() string {
	return c.store.RootPath()
}

// Option represents an option for creating a new Client
type Option func(*options)

// options holds the configuration for a new Client
type options struct {
	storeRootPath string
	logger        *logrus.Entry
	transport     http.RoundTripper
	userAgent     string
}

// WithStoreRootPath sets the store root path
func WithStoreRootPath(path string) Option {
	return func(o *options) {
		o.storeRootPath = path
	}
}

// WithLogger sets the logger
func WithLogger(logger *logrus.Entry) Option {
	return func(o *options) {
		o.logger = logger
	}
}

// WithTransport sets the HTTP transport to use when pulling and pushing models.
func WithTransport(transport http.RoundTripper) Option {
	return func(o *options) {
		o.transport = transport
	}
}

// WithUserAgent sets the User-Agent header to use when pulling and pushing models.
func WithUserAgent(ua string) Option {
	return func(o *options) {
		o.userAgent = ua
	}
}

func defaultOptions() *options {
	return &options{
		logger:    logrus.NewEntry(logrus.StandardLogger()),
		transport: remote.DefaultTransport,
		userAgent: defaultUserAgent,
	}
}

// NewClient creates a new distribution client
func NewClient(opts ...Option) (*Client, error) {
	options := defaultOptions()
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
		remoteOptions: []remote.Option{
			remote.WithAuthFromKeychain(authn.DefaultKeychain),
			remote.WithTransport(options.transport),
			remote.WithUserAgent(options.userAgent),
		},
	}, nil
}

// PullModel pulls a model from a registry and returns the local file path
func (c *Client) PullModel(ctx context.Context, reference string, progressWriter io.Writer) error {
	c.log.Infoln("Starting model pull:", reference)

	// Parse the reference
	ref, err := name.ParseReference(reference)
	if err != nil {
		return NewReferenceError(reference, err)
	}

	// First, check the remote registry for the model's digest
	c.log.Infoln("Checking remote registry for model:", reference)
	opts := append([]remote.Option{remote.WithContext(ctx)}, c.remoteOptions...)
	remoteImg, err := remote.Image(ref, opts...)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "UNAUTHORIZED") {
			return NewPullError(reference, "UNAUTHORIZED", "Authentication required for this model", err)
		}
		if strings.Contains(errStr, "MANIFEST_UNKNOWN") {
			return NewPullError(reference, "MANIFEST_UNKNOWN", "Model not found", err)
		}
		c.log.Errorln("Failed to check remote image:", err, "reference:", reference)
		return NewPullError(reference, "UNKNOWN", err.Error(), err)
	}

	//Check for supported type
	if err := checkCompat(remoteImg); err != nil {
		return err
	}

	// Get the remote image digest
	remoteDigest, err := remoteImg.Digest()
	if err != nil {
		c.log.Errorln("Failed to get remote image digest:", err)
		return fmt.Errorf("getting remote image digest: %w", err)
	}
	c.log.Infoln("Remote model digest:", remoteDigest.String())

	// Check if model exists in local store
	localModel, err := c.store.Read(remoteDigest.String())
	if err == nil {
		c.log.Infoln("Model found in local store:", reference)
		ggufPath, err := localModel.GGUFPath()
		if err != nil {
			return fmt.Errorf("getting gguf path: %w", err)
		}

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

		// Ensure model has the correct tag
		if err := c.store.AddTags(remoteDigest.String(), []string{reference}); err != nil {
			return fmt.Errorf("tagging modle: %w", err)
		}
		return nil
	} else {
		c.log.Infoln("Model not found in local store, pulling from remote:", reference)
	}

	// Model doesn't exist in local store or digests don't match, pull from remote

	var wg sync.WaitGroup
	var progress chan v1.Update
	if progressWriter != nil {
		// Start a goroutine to handle progress updates
		// Wait for the goroutine to finish or `progressWriter`'s underlying Writer may be closed
		wg.Add(1)
		defer wg.Wait()

		// Create a buffered channel for progress updates
		progress = make(chan v1.Update, 100)
		defer close(progress)
		go func() {
			defer wg.Done()
			var lastComplete int64
			var lastUpdate time.Time
			const updateInterval = 500 * time.Millisecond // Update every 500ms
			const minBytesForUpdate = 1024 * 1024         // At least 1MB difference

			for p := range progress {
				now := time.Now()
				bytesDownloaded := p.Complete - lastComplete

				// Only update if enough time has passed or enough bytes downloaded
				if now.Sub(lastUpdate) >= updateInterval || bytesDownloaded >= minBytesForUpdate {
					fmt.Fprintf(progressWriter, "Downloaded: %.2f MB\n", float64(p.Complete)/1024/1024)
					lastUpdate = now
					lastComplete = p.Complete
				}
			}
		}()
	}

	if err = c.store.Write(remoteImg, []string{reference}, progress); err != nil {
		return fmt.Errorf("writing image to store: %w", err)
	}

	return nil
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
		// Read the models
		model, err := c.store.Read(modelInfo.ID)
		if err != nil {
			c.log.Warnf("Failed to read model with tag %s: %v", modelInfo.Tags[0], err)
			continue
		}
		result = append(result, model)
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
	opts := append([]remote.Option{remote.WithContext(ctx)}, c.remoteOptions...)
	if err := remote.Write(ref, mdl, opts...); err != nil {
		c.log.Errorln("Failed to push image:", err, "reference:", reference)
		return fmt.Errorf("pushing image: %w", err)
	}

	c.log.Infoln("Successfully pushed model:", reference)
	return nil
}

// DeleteModel deletes a model
func (c *Client) DeleteModel(reference string) error {
	c.log.Infoln("Deleting model:", reference)
	if err := c.store.Delete(reference); err != nil {
		c.log.Errorln("Failed to delete model:", err, "tag:", reference)
		return fmt.Errorf("deleting model: %w", err)
	}
	c.log.Infoln("Successfully deleted model:", reference)
	return nil
}

// Tag adds a tag to a model
func (c *Client) Tag(source string, target string) error {
	c.log.Infoln("Tagging model, source:", source, "target:", target)
	return c.store.AddTags(source, []string{target})
}

func checkCompat(image v1.Image) error {
	manifest, err := image.Manifest()
	if err != nil {
		return err
	}
	if manifest.Config.MediaType != types.MediaTypeModelConfigV01 {
		return fmt.Errorf("config type %q is unsupported: %w", manifest.Config.MediaType, ErrUnsupportedMediaType)
	}
	return nil
}
