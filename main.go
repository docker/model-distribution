package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/google/go-containerregistry/pkg/v1/static"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"

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

	layer := createLayer(source)
	armImage := createPlatformImage("arm", "darwin", layer)
	amdImage := createPlatformImage("amd", "darwin", layer)
	idx := createImageIndex([]v1.Image{armImage, amdImage})
	ref := pushImage(tag, idx)

	fmt.Printf("\nSuccessfully pushed multi-arch image %s\n", ref.String())
}

func createLayer(source *string) v1.Layer {
	fmt.Printf("Reading from source: %s\n", *source)
	fileContent, err := utils.ReadContent(*source)
	if err != nil {
		log.Fatalf("reading content: %v", err)
	}
	fmt.Printf("   Size: %s\n", utils.FormatBytes(len(fileContent)))

	fmt.Println("Creating imgLayer from file content...")
	layer := static.NewLayer(fileContent, "application/vnd.docker.ai.model.file.v1+gguf")
	layerSize, _ := layer.Size()
	fmt.Printf("   Layer size: %s\n", utils.FormatBytes(int(layerSize)))
	return layer
}

// createPlatformImage creates a single platform image with the given layer
func createPlatformImage(arch, os string, l v1.Layer) v1.Image {
	platformImg := empty.Image
	configFile := &v1.ConfigFile{
		Architecture: arch,
		OS:           os,
		Config:       v1.Config{},
	}
	platformImg, err := mutate.ConfigFile(platformImg, configFile)
	if err != nil {
		log.Fatalf("setting config for %s/%s: %v", arch, os, err)
	}

	platformImg = mutate.MediaType(platformImg, types.OCIManifestSchema1)
	platformImg = mutate.ConfigMediaType(platformImg, "application/vnd.docker.ai.model.manifest.v1+json")

	fmt.Printf("Appending layer to image for %s/%s...\n", arch, os)
	platformImg, err = mutate.AppendLayers(platformImg, l)
	if err != nil {
		log.Fatalf("appending layer: %v", err)
	}

	return platformImg
}

// createImageIndex creates an OCI image index with given images
func createImageIndex(images []v1.Image) v1.ImageIndex {
	fmt.Println("Creating image index...")

	var addenda []mutate.IndexAddendum
	for _, img := range images {
		cf, err := img.ConfigFile()
		if err != nil {
			log.Fatalf("getting config file: %v", err)
		}
		addenda = append(addenda, mutate.IndexAddendum{
			Add: img,
			Descriptor: v1.Descriptor{
				Platform: &v1.Platform{
					Architecture: cf.Architecture,
					OS:           cf.OS,
					//TODO add features
				},
				ArtifactType: "application/vnd.docker.ai.model.config.v1+json",
			},
		})
	}

	idx := mutate.IndexMediaType(empty.Index, types.OCIImageIndex)
	return mutate.AppendManifests(idx, addenda...)
}

func pushImage(tag *string, idx v1.ImageIndex) name.Reference {
	fmt.Println("Pushing image index to registry...")
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
	go utils.ShowProgress("Uploading", progressChan64, -1)

	// Push the image index with progress
	fmt.Println("Creating reference for target image...")
	ref, err := name.ParseReference(*tag)
	if err != nil {
		log.Fatalf("parsing reference: %v", err)
	}
	fmt.Printf("   Reference: %s\n", ref.String())
	if err := remote.WriteIndex(ref, idx,
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithProgress(progressChan),
	); err != nil {
		log.Fatalf("writing image index: %v", err)
	}
	return ref
}
