package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/model-distribution/builder"
	"github.com/docker/model-distribution/distribution"
	"github.com/docker/model-distribution/registry"
)

// stringSliceFlag is a flag that can be specified multiple times to collect multiple string values
type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, ", ")
}

func (s *stringSliceFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

const (
	defaultStorePath = "./model-store"
	version          = "0.1.0"
)

var (
	storePath string
	showHelp  bool
	showVer   bool
)

func init() {
	flag.StringVar(&storePath, "store-path", defaultStorePath, "Path to the model store")
	flag.BoolVar(&showHelp, "help", false, "Show help")
	flag.BoolVar(&showVer, "version", false, "Show version")
}

func main() {
	flag.Parse()

	if showVer {
		fmt.Printf("model-distribution-tool version %s\n", version)
		return
	}

	if showHelp || flag.NArg() == 0 {
		printUsage()
		return
	}

	// Create absolute path for store
	absStorePath, err := filepath.Abs(storePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving store path: %v\n", err)
		os.Exit(1)
	}

	// Create the client with auth if environment variables are set
	clientOpts := []distribution.Option{
		distribution.WithStoreRootPath(absStorePath),
		distribution.WithUserAgent("model-distribution-tool/" + version),
	}

	if username := os.Getenv("DOCKER_USERNAME"); username != "" {
		if password := os.Getenv("DOCKER_PASSWORD"); password != "" {
			clientOpts = append(clientOpts, distribution.WithRegistryAuth(username, password))
		}
	}

	client, err := distribution.NewClient(clientOpts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating client: %v\n", err)
		os.Exit(1)
	}

	// Get the command and arguments
	command := flag.Arg(0)
	args := flag.Args()[1:]

	// Execute the command
	exitCode := 0
	switch command {
	case "pull":
		exitCode = cmdPull(client, args)
	case "package":
		exitCode = cmdPackage(args)
	case "push":
		exitCode = cmdPush(client, args)
	case "list":
		exitCode = cmdList(client, args)
	case "get":
		exitCode = cmdGet(client, args)
	case "get-path":
		exitCode = cmdGetPath(client, args)
	case "rm":
		exitCode = cmdRm(client, args)
	case "tag":
		exitCode = cmdTag(client, args)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printUsage()
		exitCode = 1
	}

	os.Exit(exitCode)
}

func printUsage() {
	fmt.Println("Usage: model-distribution-tool [options] <command> [arguments]")
	fmt.Println("\nOptions:")
	flag.PrintDefaults()
	fmt.Println("\nCommands:")
	fmt.Println("  pull <reference>                Pull a model from a registry")
	fmt.Println("  package <source> <reference>    Package a model file as an OCI artifact and push it to a registry (use --licenses to add license files)")
	fmt.Println("  push <tag>                      Push a model from the content store to the registry")
	fmt.Println("  list                            List all models")
	fmt.Println("  get <reference>                 Get a model by reference")
	fmt.Println("  get-path <reference>            Get the local file path for a model")
	fmt.Println("  rm <reference>                  Remove a model by reference")
	fmt.Println("\nExamples:")
	fmt.Println("  model-distribution-tool --store-path ./models pull registry.example.com/models/llama:v1.0")
	fmt.Println("  model-distribution-tool package ./model.gguf registry.example.com/models/llama:v1.0 --licenses ./license1.txt --licenses ./license2.txt")
	fmt.Println("  model-distribution-tool push registry.example.com/models/llama:v1.0")
	fmt.Println("  model-distribution-tool list")
	fmt.Println("  model-distribution-tool rm registry.example.com/models/llama:v1.0")
}

func cmdPull(client *distribution.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: missing reference argument\n")
		fmt.Fprintf(os.Stderr, "Usage: model-distribution-tool pull <reference>\n")
		return 1
	}

	reference := args[0]
	ctx := context.Background()

	if err := client.PullModel(ctx, reference, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Error pulling model: %v\n", err)
		return 1
	}

	fmt.Printf("Successfully pulled model: %s\n", reference)
	return 0
}

func cmdPackage(args []string) int {
	fs := flag.NewFlagSet("push", flag.ExitOnError)
	var licensePaths stringSliceFlag
	fs.Var(&licensePaths, "licenses", "Paths to license files (can be specified multiple times)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		return 1
	}
	args = fs.Args()

	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "Error: missing arguments\n")
		fmt.Fprintf(os.Stderr, "Usage: model-distribution-tool push <source> <reference> [--licenses <path-to-license-file1> --licenses <path-to-license-file2> ...]\n")
		return 1
	}

	source := args[0]
	reference := args[1]
	ctx := context.Background()

	// Check if source file exists
	if _, err := os.Stat(source); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: source file does not exist: %s\n", source)
		return 1
	}

	// Check if source file is a GGUF file
	if !strings.HasSuffix(strings.ToLower(source), ".gguf") {
		fmt.Fprintf(os.Stderr, "Warning: source file does not have .gguf extension: %s\n", source)
		fmt.Fprintf(os.Stderr, "Continuing anyway, but this may cause issues.\n")
	}

	// Prepare registry client options
	registryClientOpts := []registry.ClientOption{
		registry.WithUserAgent("model-distribution-tool/" + version),
	}

	// Add auth if available
	if username := os.Getenv("DOCKER_USERNAME"); username != "" {
		if password := os.Getenv("DOCKER_PASSWORD"); password != "" {
			registryClientOpts = append(registryClientOpts, registry.WithAuthConfig(username, password))
		}
	}

	// Create registry client once with all options
	registryClient := registry.NewClient(registryClientOpts...)

	// Parse the reference
	target, err := registryClient.NewTarget(reference)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing reference: %v\n", err)
		return 1
	}

	// Create image with layer
	builder, err := builder.FromGGUF(source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating model from gguf: %v\n", err)
		return 1
	}

	// Add all license files as layers
	for _, path := range licensePaths {
		fmt.Println("Adding license file:", path)
		builder, err = builder.WithLicense(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error adding license layer for %s: %v\n", path, err)
			return 1
		}
	}

	// Push the image
	if err := builder.Build(ctx, target, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing model %q to registry: %v\n", reference, err)
		return 1
	}
	return 0
}

func cmdPush(client *distribution.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: missing tag argument\n")
		fmt.Fprintf(os.Stderr, "Usage: model-distribution-tool push <tag>\n")
		return 1
	}

	tag := args[0]
	ctx := context.Background()

	if err := client.PushModel(ctx, tag, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Error pushing model: %v\n", err)
		return 1
	}

	fmt.Printf("Successfully pushed model: %s\n", tag)
	return 0
}

func cmdList(client *distribution.Client, args []string) int {
	models, err := client.ListModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing models: %v\n", err)
		return 1
	}

	if len(models) == 0 {
		fmt.Println("No models found")
		return 0
	}

	fmt.Println("Models:")
	for i, model := range models {
		id, err := model.ID()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting model ID: %v\n", err)
			continue
		}
		fmt.Printf("%d. ID: %s\n", i+1, id)
		fmt.Printf("   Tags: %s\n", strings.Join(model.Tags(), ", "))

		ggufPath, err := model.GGUFPath()
		if err == nil {
			fmt.Printf("   GGUF Path: %s\n", ggufPath)
		}
	}
	return 0
}

func cmdGet(client *distribution.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: missing reference argument\n")
		fmt.Fprintf(os.Stderr, "Usage: model-distribution-tool get <reference>\n")
		return 1
	}

	reference := args[0]

	model, err := client.GetModel(reference)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting model: %v\n", err)
		return 1
	}

	fmt.Printf("Model: %s\n", reference)

	id, err := model.ID()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting model ID %v\n", err)
		return 1
	}
	fmt.Printf("ID: %s\n", id)

	ggufPath, err := model.GGUFPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting gguf path %v\n", err)
		return 1
	}
	fmt.Printf("GGUF Path: %s\n", ggufPath)

	cfg, err := model.Config()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading model config: %v\n", err)
		return 1
	}
	fmt.Printf("Format: %s\n", cfg.Format)
	fmt.Printf("Architecture: %s\n", cfg.Architecture)
	fmt.Printf("Parameters: %s\n", cfg.Parameters)
	fmt.Printf("Quantization: %s\n", cfg.Quantization)
	return 0
}

func cmdGetPath(client *distribution.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: missing reference argument\n")
		fmt.Fprintf(os.Stderr, "Usage: model-distribution-tool get-path <reference>\n")
		return 1
	}

	reference := args[0]

	model, err := client.GetModel(reference)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get model: %v\n", err)
		return 1
	}

	modelPath, err := model.GGUFPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting model path: %v\n", err)
		return 1
	}

	fmt.Println(modelPath)
	return 0
}

func cmdRm(client *distribution.Client, args []string) int {
	var force bool
	fs := flag.NewFlagSet("rm", flag.ExitOnError)
	fs.BoolVar(&force, "force", false, "Force remove the model")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		return 1
	}
	args = fs.Args()

	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: missing reference argument\n")
		fmt.Fprintf(os.Stderr, "Usage: model-distribution-tool rm [--force] <reference>\n")
		return 1
	}

	reference := args[0]

	if err := client.DeleteModel(reference, force); err != nil {
		fmt.Fprintf(os.Stderr, "Error removing model: %v\n", err)
		return 1
	}

	fmt.Printf("Successfully removed model: %s\n", reference)
	return 0
}

func cmdTag(client *distribution.Client, args []string) int {
	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: model-distribution-tool tag <reference> <tag>\n")
		return 1
	}

	source := args[0]
	target := args[1]

	if err := client.Tag(source, target); err != nil {
		fmt.Fprintf(os.Stderr, "Error tagging model: %v\n", err)
		return 1
	}

	fmt.Printf("Successfully applied tag %s to model: %s\n", target, source)
	return 0
}
