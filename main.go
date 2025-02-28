package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"

	"github.com/docker/model-distribution/pkg/image"
	"github.com/docker/model-distribution/pkg/model"
	"github.com/docker/model-distribution/pkg/store"
	"github.com/docker/model-distribution/pkg/types"
	"github.com/docker/model-distribution/pkg/utils"
)

func PushModel(source, tag string) (name.Reference, error) {
	fmt.Println("1. Creating reference for target image...")
	ref, err := name.ParseReference(tag)
	if err != nil {
		return nil, err
	}
	fmt.Printf("   Reference: %s\n", ref.String())

	fmt.Printf("2. Reading from source: %s\n", source)
	fileContent, err := utils.ReadContent(source)
	if err != nil {
		return nil, err
	}
	fmt.Printf("   Size: %s\n", utils.FormatBytes(len(fileContent)))

	fmt.Println("3. Creating imgLayer from file content...")
	l := static.NewLayer(fileContent, "application/vnd.docker.ai.model.file.v1+gguf")
	layerSize, _ := l.Size()
	fmt.Printf("   Layer size: %s\n", utils.FormatBytes(int(layerSize)))

	fmt.Println("4. Creating image with layer...")
	img, err := model.FromGGUF(l)
	if err != nil {
		return nil, err
	}

	fmt.Println("5. Getting manifest details...")
	manifest, err := img.Manifest()
	if err != nil {
		return nil, err
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

	fmt.Println("6. Pushing image to registry...")
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
	go utils.ShowProgress("Uploading", progressChan64, -1) // -1 since total size might not be known

	// Push the image with progress and auth config
	if err := remote.Push(ref, img,
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithProgress(progressChan),
	); err != nil {
		return nil, fmt.Errorf("writing image: %v", err)
	}

	fmt.Printf("Successfully pushed %s\n", ref.String())
	return ref, nil
}

// getLocalStore initializes and returns a local store instance
func getLocalStore() (*store.LocalStore, error) {
	// Use a default path for the local store (e.g., ~/.model-distribution/store)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting user home directory: %v", err)
	}

	storePath := filepath.Join(homeDir, ".model-distribution", "store")
	return store.New(types.StoreOptions{RootPath: storePath})
}

// getImageFromLocalStore creates an image from a model in the local store
func getImageFromLocalStore(localStore *store.LocalStore, model *types.Model) (v1.Image, error) {
	// Create a temporary file to store the model content
	tempFile, err := os.CreateTemp("", "model-*.gguf")
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Pull the model to the temporary file
	if err := localStore.Pull(model.Tags[0], tempFile.Name()); err != nil {
		return nil, fmt.Errorf("pulling model from local store: %v", err)
	}

	// Read the model content
	modelContent, err := os.ReadFile(tempFile.Name())
	if err != nil {
		return nil, fmt.Errorf("reading model content: %v", err)
	}

	// Create layer from model content
	ggufLayer := static.NewLayer(modelContent, "application/vnd.docker.ai.model.file.v1+gguf")

	// Create image with layer
	return image.CreateImage(ggufLayer)
}

// storeRemoteImage stores a remote image in the local store
func storeRemoteImage(img v1.Image, tag string, localStore *store.LocalStore) error {
	// Create a temporary file to store the model content
	tempFile, err := os.CreateTemp("", "model-*.gguf")
	if err != nil {
		return fmt.Errorf("creating temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Get the model content from the image
	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("getting layers: %v", err)
	}

	if len(layers) == 0 {
		return fmt.Errorf("no layers in image")
	}

	// Use the first layer (assuming there's only one for models)
	layer := layers[0]

	// Get the layer content
	rc, err := layer.Uncompressed()
	if err != nil {
		return fmt.Errorf("getting layer content: %v", err)
	}
	defer rc.Close()

	// Write the layer content to the temporary file
	if _, err := io.Copy(tempFile, rc); err != nil {
		return fmt.Errorf("writing layer content: %v", err)
	}

	// Push the model to the local store
	return localStore.Push(tempFile.Name(), []string{tag})
}

func PullModel(tag string) (v1.Image, error) {
	// Initialize local store
	localStore, err := getLocalStore()
	if err != nil {
		return nil, fmt.Errorf("initializing local store: %v", err)
	}

	// Check if model exists in local store
	model, err := localStore.GetByTag(tag)
	if err == nil {
		fmt.Printf("Model %s found in local store, retrieving...\n", tag)
		// Model exists in local store, create and return image from local store
		return getImageFromLocalStore(localStore, model)
	}

	fmt.Printf("Model %s not found in local store, pulling from hub...\n", tag)
	// Model doesn't exist in local store, pull from remote
	ref, err := name.ParseReference(tag)
	if err != nil {
		return nil, fmt.Errorf("parsing reference: %v", err)
	}

	img, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, err
	}

	fmt.Printf("Storing model %s in local store...\n", tag)
	// Store the pulled model in local store
	if err := storeRemoteImage(img, tag, localStore); err != nil {
		// Log the error but continue since we already have the image
		fmt.Printf("Warning: Failed to store model in local store: %v\n", err)
	}

	return img, nil
}

// DeleteModel deletes a model artifact from a container registry
func DeleteModel(tag string) error {
	ref, err := name.ParseReference(tag)
	if err != nil {
		return fmt.Errorf("parsing reference: %v", err)
	}

	fmt.Printf("Deleting artifact: %s\n", ref.String())
	return remote.Delete(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
}

func main() {
	var (
		source = flag.String("source", "", "Path to local file or URL to download")
		tag    = flag.String("tag", "", "Target registry/repository:tag")
	)
	flag.Parse()

	if *source == "" || *tag == "" {
		flag.Usage()
		os.Exit(1)
	}

	_, err := PushModel(*source, *tag)
	if err != nil {
		log.Fatal(err)
	}
}
