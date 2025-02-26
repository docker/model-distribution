package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/docker/model-distribution/pkg/layer"
	"github.com/docker/model-distribution/pkg/utils"
)

// getAuthenticator returns the appropriate authenticator based on the registry and available credentials
func getAuthenticator(ref name.Reference) authn.Authenticator {
	registry := ref.Context().Registry.Name()

	// First try to use the Docker config authentication
	// This will use credentials from ~/.docker/config.json which is set up by the workflow
	dockerAuth, err := authn.DefaultKeychain.Resolve(ref.Context())
	if err == nil && dockerAuth != authn.Anonymous {
		fmt.Println("Using Docker config authentication")
		return dockerAuth
	}

	// Default to anonymous authentication
	auth := authn.Anonymous

	// Check for Google Application Credentials file (for GAR)
	// This is set by google-github-actions/auth@v2 with create_credentials_file: true
	if credFile := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); credFile != "" && strings.Contains(registry, "pkg.dev") {
		fmt.Println("Using Google Application Credentials file for authentication")
		// Resolve an authenticator using the credentials file
		googleAuth, err := authn.DefaultKeychain.Resolve(ref.Context())
		if err == nil && googleAuth != authn.Anonymous {
			return googleAuth
		}
		// If resolution fails, continue to other auth methods
	}

	// Check for Google OAuth token (for GAR)
	if googleToken := os.Getenv("GOOGLE_OAUTH_ACCESS_TOKEN"); googleToken != "" && strings.Contains(registry, "pkg.dev") {
		fmt.Println("Using Google OAuth token authentication")
		return &authn.Bearer{
			Token: googleToken,
		}
	}

	// Check for AWS credentials (for ECR)
	// First check if we're dealing with an ECR registry
	if strings.Contains(registry, "amazonaws.com") {
		// Try to use AWS SDK credential chain first (handles OIDC, role assumption, etc.)
		// This will work when credentials are set up by aws-actions/configure-aws-credentials
		fmt.Println("Attempting to use AWS SDK credential chain for ECR authentication")
		awsAuth, err := authn.DefaultKeychain.Resolve(ref.Context())
		if err == nil && awsAuth != authn.Anonymous {
			fmt.Println("Successfully authenticated with AWS SDK credential chain")
			return awsAuth
		}

		// Fall back to explicit environment variables if available
		if os.Getenv("AWS_ACCESS_KEY_ID") != "" && os.Getenv("AWS_SECRET_ACCESS_KEY") != "" {
			fmt.Println("Using AWS credentials from environment variables")
			return authn.FromConfig(authn.AuthConfig{
				Username: os.Getenv("AWS_ACCESS_KEY_ID"),
				Password: os.Getenv("AWS_SECRET_ACCESS_KEY"),
			})
		}

		fmt.Println("Warning: Could not authenticate with AWS for ECR")
	}

	// Fall back to Docker credentials if available
	if username, password := os.Getenv("DOCKER_USERNAME"), os.Getenv("DOCKER_PASSWORD"); username != "" && password != "" {
		fmt.Println("Using Docker username/password authentication")
		return &authn.Basic{
			Username: username,
			Password: password,
		}
	}

	fmt.Println("Using anonymous authentication")
	return auth
}

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
	l := layer.New(fileContent)
	layerSize, _ := l.Size()
	fmt.Printf("   Layer size: %s\n", utils.FormatBytes(int(layerSize)))

	fmt.Println("4. Creating empty image with artifact configuration...")
	img := empty.Image

	configFile := &v1.ConfigFile{
		Architecture: "unknown",
		OS:           "unknown",
		Config:       v1.Config{},
	}

	img, err = mutate.ConfigFile(img, configFile)
	if err != nil {
		return nil, err
	}

	// Set up artifact manifest according to OCI spec
	img = mutate.MediaType(img, types.OCIManifestSchema1)
	img = mutate.ConfigMediaType(img, "application/vnd.docker.ai.model.config.v1+json")

	fmt.Println("5. Appending imgLayer to image...")
	img, err = mutate.AppendLayers(img, l)
	if err != nil {
		return nil, err
	}

	fmt.Println("6. Getting manifest details...")
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
	go utils.ShowProgress("Uploading", progressChan64, -1) // -1 since total size might not be known

	// Get the appropriate authenticator for this registry
	auth := getAuthenticator(ref)

	// Push the image with progress and auth config
	if err := remote.Write(ref, img,
		remote.WithAuth(auth),
		remote.WithProgress(progressChan),
	); err != nil {
		return nil, fmt.Errorf("writing image: %v", err)
	}

	fmt.Printf("Successfully pushed %s\n", ref.String())
	return ref, nil
}

func PullModel(tag string) (v1.Image, error) {
	ref, err := name.ParseReference(tag)
	if err != nil {
		return nil, fmt.Errorf("parsing reference: %v", err)
	}

	// Get the appropriate authenticator for this registry
	auth := getAuthenticator(ref)

	return remote.Image(ref, remote.WithAuth(auth))
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
