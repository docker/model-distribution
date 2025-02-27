package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/docker/model-distribution/pkg/store"
	"github.com/docker/model-distribution/pkg/types"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	v1types "github.com/google/go-containerregistry/pkg/v1/types"
)

// ReadContent reads content from a file or URL
func ReadContent(path string) ([]byte, error) {
	// For now, just read from a file
	return os.ReadFile(path)
}

// FormatBytes formats bytes to a human-readable string
func FormatBytes(bytes int) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// ShowProgress shows progress for an operation
func ShowProgress(operation string, progressChan <-chan int64, total int64) {
	fmt.Printf("%s: in progress...\n", operation)
	for progress := range progressChan {
		if total > 0 {
			fmt.Printf("\r%s: %s / %s (%.1f%%)", operation, FormatBytes(int(progress)), FormatBytes(int(total)), float64(progress)/float64(total)*100)
		} else {
			fmt.Printf("\r%s: %s", operation, FormatBytes(int(progress)))
		}
	}
	fmt.Println("\nDone!")
}

// Layer represents a layer in an OCI image
type Layer struct {
	content []byte
}

// New creates a new layer from content
func New(content []byte) v1.Layer {
	return &Layer{content: content}
}

// Digest returns the digest of the layer
func (l *Layer) Digest() (v1.Hash, error) {
	return v1.NewHash("sha256:" + fmt.Sprintf("%x", l.content))
}

// DiffID returns the diff ID of the layer
func (l *Layer) DiffID() (v1.Hash, error) {
	return l.Digest()
}

// Compressed returns the compressed layer
func (l *Layer) Compressed() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(l.content)), nil
}

// Uncompressed returns the uncompressed layer
func (l *Layer) Uncompressed() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(l.content)), nil
}

// Size returns the size of the layer
func (l *Layer) Size() (int64, error) {
	return int64(len(l.content)), nil
}

// MediaType returns the media type of the layer
func (l *Layer) MediaType() (v1types.MediaType, error) {
	return v1types.MediaType("application/vnd.oci.image.layer.v1.tar"), nil
}

func main() {
	var (
		source      = flag.String("source", "", "Path to local file or URL to download")
		tag         = flag.String("tag", "", "Target registry/repository:tag")
		mode        = flag.String("mode", "registry", "Mode: 'registry' or 'local'")
		storePath   = flag.String("store", "./model-store", "Path to local model store")
		destination = flag.String("destination", "", "Destination path for pull operation")
		list        = flag.Bool("list", false, "List models in local store")
		addTags     = flag.String("add-tags", "", "Add tags to a model (specify existing tag)")
		newTags     = flag.String("new-tags", "", "New tags to add (comma-separated)")
		removeTags  = flag.String("remove-tags", "", "Remove tags (comma-separated)")
		deleteTag   = flag.String("delete", "", "Delete a model by tag")
	)
	flag.Parse()

	// Handle local store operations
	if *mode == "local" {
		// Create the store
		s, err := store.New(types.StoreOptions{
			RootPath: *storePath,
		})
		if err != nil {
			log.Fatalf("creating store: %v", err)
		}

		// List models
		if *list {
			models, err := s.List()
			if err != nil {
				log.Fatalf("listing models: %v", err)
			}
			fmt.Println("Models in store:")
			for _, model := range models {
				fmt.Printf("  Manifest: %s\n", model.ManifestDigest)
				fmt.Printf("  Tags: %s\n", strings.Join(model.Tags, ", "))
				fmt.Println()
			}
			return
		}

		// Delete a model
		if *deleteTag != "" {
			if err := s.Delete(*deleteTag); err != nil {
				log.Fatalf("deleting model: %v", err)
			}
			fmt.Printf("Model with tag %s deleted\n", *deleteTag)
			return
		}

		// Add tags
		if *addTags != "" && *newTags != "" {
			newTagsList := strings.Split(*newTags, ",")
			if err := s.AddTags(*addTags, newTagsList); err != nil {
				log.Fatalf("adding tags: %v", err)
			}
			fmt.Printf("Added tags %s to model %s\n", *newTags, *addTags)
			return
		}

		// Remove tags
		if *removeTags != "" {
			tagsList := strings.Split(*removeTags, ",")
			if err := s.RemoveTags(tagsList); err != nil {
				log.Fatalf("removing tags: %v", err)
			}
			fmt.Printf("Removed tags: %s\n", *removeTags)
			return
		}

		// Pull operation
		if *tag != "" && *destination != "" {
			fmt.Printf("Pulling model %s from local store...\n", *tag)
			if err := s.Pull(*tag, *destination); err != nil {
				log.Fatalf("pulling model: %v", err)
			}
			fmt.Printf("Model pulled to %s\n", *destination)
			return
		}

		// Push operation
		if *source != "" && *tag != "" {
			fmt.Printf("Pushing model %s to local store with tag %s...\n", *source, *tag)
			if err := s.Push(*source, []string{*tag}); err != nil {
				log.Fatalf("pushing model: %v", err)
			}
			fmt.Printf("Model pushed with tag %s\n", *tag)
			return
		}

		// If we get here, no valid operation was specified
		flag.Usage()
		os.Exit(1)
	}

	// Registry mode
	if *source == "" || *tag == "" {
		flag.Usage()
		os.Exit(1)
	}

	fmt.Println("1. Creating reference for target image...")
	ref, err := name.ParseReference(*tag)
	if err != nil {
		log.Fatalf("parsing reference: %v", err)
	}
	fmt.Printf("   Reference: %s\n", ref.String())

	fmt.Printf("2. Reading from source: %s\n", *source)
	fileContent, err := ReadContent(*source)
	if err != nil {
		log.Fatalf("reading content: %v", err)
	}
	fmt.Printf("   Size: %s\n", FormatBytes(len(fileContent)))

	fmt.Println("3. Creating imgLayer from file content...")
	l := New(fileContent)
	layerSize, _ := l.Size()
	fmt.Printf("   Layer size: %s\n", FormatBytes(int(layerSize)))

	fmt.Println("4. Creating empty image with artifact configuration...")
	img := empty.Image

	configFile := &v1.ConfigFile{
		Architecture: "unknown",
		OS:           "unknown",
		Config:       v1.Config{},
	}

	img, err = mutate.ConfigFile(img, configFile)
	if err != nil {
		log.Fatalf("setting config: %v", err)
	}

	// Set up artifact manifest according to OCI spec
	img = mutate.MediaType(img, v1types.OCIManifestSchema1)
	img = mutate.ConfigMediaType(img, "application/vnd.docker.ai.model.config.v1+json")

	fmt.Println("5. Appending imgLayer to image...")
	img, err = mutate.AppendLayers(img, l)
	if err != nil {
		log.Fatalf("appending imgLayer: %v", err)
	}

	fmt.Println("6. Getting manifest details...")
	manifest, err := img.Manifest()
	if err != nil {
		log.Fatalf("getting manifest: %v", err)
	}

	fmt.Println("\nManifest details:")
	fmt.Printf("  MediaType: %s\n", manifest.MediaType)
	fmt.Printf("  Config:")
	fmt.Printf("    MediaType: %s\n", manifest.Config.MediaType)
	fmt.Printf("    Size: %d bytes\n", manifest.Config.Size)
	fmt.Printf("    Digest: %s\n", manifest.Config.Digest)
	fmt.Printf("  Layers:\n")
	for i, imgLayer := range manifest.Layers {
		fmt.Printf("    Layer %d:\n", i+1)
		fmt.Printf("      MediaType: %s\n", imgLayer.MediaType)
		fmt.Printf("      Size: %d bytes\n", imgLayer.Size)
		fmt.Printf("      Digest: %s\n", imgLayer.Digest)
	}
	fmt.Println()

	fmt.Println("7. Pushing image to registry...")
	// Create progress channel
	progressChan := make(chan v1.Update, 1)

	// Convert v1.Update channel to int64 channel for ShowProgress
	progressChan64 := make(chan int64, 1)
	go func() {
		for update := range progressChan {
			progressChan64 <- update.Complete
		}
		close(progressChan64)
	}()

	// Show progress
	go ShowProgress("Uploading", progressChan64, -1) // -1 since total size might not be known

	// Push the image with progress
	if err := remote.Write(ref, img,
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithProgress(progressChan),
	); err != nil {
		log.Fatalf("writing image: %v", err)
	}

	fmt.Printf("Successfully pushed %s\n", ref.String())
}
