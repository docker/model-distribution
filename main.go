package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/stream"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/klauspost/compress/gzip"

	"github.com/docker/model-distribution/pkg/layer"
	"github.com/docker/model-distribution/pkg/utils"
)

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

	fmt.Println("Creating reference for target image...")
	ref, err := name.ParseReference(*tag)
	if err != nil {
		log.Fatalf("parsing reference: %v", err)
	}
	fmt.Printf("   Reference: %s\n", ref.String())

	fmt.Printf("Creating layer from source file: %s\n", *source)
	f, err := utils.Open(*source)
	if err != nil {
		log.Fatalf("reading content: %v", err)
	}
	defer f.Close()

	l := stream.NewLayer(f,
		stream.WithMediaType(layer.MediaTypeGGUF),
		stream.WithCompressionLevel(gzip.NoCompression),
	)

	fmt.Println("Creating empty image with artifact configuration...")
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
	img = mutate.MediaType(img, types.OCIManifestSchema1)
	img = mutate.ConfigMediaType(img, "application/vnd.docker.ai.model.config.v1+json")

	fmt.Println("Appending imgLayer to image...")
	img, err = mutate.AppendLayers(img, l)
	if err != nil {
		log.Fatalf("appending imgLayer: %v", err)
	}
	//
	fmt.Println("Pushing image to registry...")
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

	// Push the image with progress
	if err := remote.Write(ref, img,
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithProgress(progressChan),
	); err != nil {
		log.Fatalf("writing image: %v", err)
	}

	fmt.Println("Getting manifest details...")
	manifest, err := img.RawManifest()
	if err != nil {
		log.Fatalf("getting manifest: %v", err)
	}

	fmt.Println("\nManifest:")
	fmt.Println(string(manifest))

	fmt.Printf("Successfully pushed %s\n", ref.String())
}
