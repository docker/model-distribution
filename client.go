package main

import (
	"fmt"
	"os"

	"github.com/docker/model-distribution/pkg/utils"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/stream"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/klauspost/compress/gzip"
)

type ModelImage interface {
	v1.Image
	GetFormat() string
	GetQuantization() string
}

// Define a struct that implements ModelImage interface
type Model struct {
	v1.Image
	Format       string
	Quantization string
}

func (m *Model) GetFormat() string       { return m.Format }
func (m *Model) GetQuantization() string { return m.Quantization }

// TODO it should allow multiple (and different) sources, adapter, model template, others?
func Push(source, tag string) (name.Reference, error) {
	ref, err := name.ParseReference(tag)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Reference: %s\n", ref.String())

	repo, err := name.NewRepository(ref.Context().RegistryStr() + "/" + ref.Context().RepositoryStr())
	if err != nil {
		return nil, err
	}

	gguf, err := utils.ReadContent(source)
	if err != nil {
		return nil, err
	}

	fmt.Println("Creating layer from file content...")
	layer := stream.NewLayer(gguf,
		stream.WithMediaType("application/vnd.docker.ai.model.file.v1+gguf"),
		stream.WithCompressionLevel(gzip.NoCompression),
	)

	auth := &authn.Basic{
		Username: os.Getenv("DOCKER_USERNAME"),
		Password: os.Getenv("DOCKER_PASSWORD"),
	}

	if err := remote.WriteLayer(repo, layer, remote.WithAuth(auth)); err != nil {
		return nil, err
	}

	digest, err := layer.Digest()
	if err != nil {
		return nil, err
	}
	fmt.Printf("Layer created successfully: %s\n\n", digest)

	image, err := createImage(layer)
	if err != nil {
		return nil, err
	}

	q4km := &Model{
		Image:        image,
		Format:       "gguf",
		Quantization: "Q4_K_M",
	}
	q5km := &Model{
		Image:        image,
		Format:       "gguf",
		Quantization: "Q5_K_M",
	}
	idx, err := createIndex([]ModelImage{q4km, q5km})
	if err != nil {
		return nil, err
	}

	err = pushIndex(ref, idx)
	return ref, err
}

func createImage(l v1.Layer) (v1.Image, error) {
	fmt.Println("Creating image...")
	idxImage := empty.Image
	idxImage = mutate.MediaType(idxImage, types.OCIManifestSchema1)
	idxImage = mutate.ConfigMediaType(idxImage, "application/vnd.docker.ai.model.manifest.v1+json")
	return mutate.AppendLayers(idxImage, l)
}

func createIndex(models []ModelImage) (v1.ImageIndex, error) {
	fmt.Println("Creating image index...")
	var addenda []mutate.IndexAddendum
	for _, model := range models {
		addenda = append(addenda, mutate.IndexAddendum{
			Add: model,
			Descriptor: v1.Descriptor{
				Platform: &v1.Platform{
					Features: []string{"quantization=" + model.GetQuantization(), "format=" + model.GetFormat()},
				},
				ArtifactType: "application/vnd.docker.ai.model.config.v1+json",
			},
		})
	}

	idx := mutate.IndexMediaType(empty.Index, types.OCIImageIndex)
	return mutate.AppendManifests(idx, addenda...), nil
}

func pushIndex(ref name.Reference, idx v1.ImageIndex) error {
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
	return remote.WriteIndex(ref, idx,
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithProgress(progressChan))
}
