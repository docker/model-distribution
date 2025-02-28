package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/model-distribution/pkg/distribution"
)

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

	// Create the client
	client, err := distribution.NewClient(absStorePath)
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
	case "push":
		exitCode = cmdPush(client, args)
	case "list":
		exitCode = cmdList(client, args)
	case "get":
		exitCode = cmdGet(client, args)
	case "get-path":
		exitCode = cmdGetPath(client, args)
	case "delete":
		exitCode = cmdDelete(client, args)
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
	fmt.Println("  push <source> <reference>       Push a model to a registry")
	fmt.Println("  list                            List all models")
	fmt.Println("  get <reference>                 Get a model by reference")
	fmt.Println("  get-path <reference>            Get the local file path for a model")
	fmt.Println("  delete <reference>              Delete a model from registry and local store")
	fmt.Println("\nExamples:")
	fmt.Println("  model-distribution-tool --store-path ./models pull registry.example.com/models/llama:v1.0")
	fmt.Println("  model-distribution-tool push ./model.gguf registry.example.com/models/llama:v1.0")
	fmt.Println("  model-distribution-tool list")
}

func cmdPull(client *distribution.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: missing reference argument\n")
		fmt.Fprintf(os.Stderr, "Usage: model-distribution-tool pull <reference>\n")
		return 1
	}

	reference := args[0]
	ctx := context.Background()

	modelPath, err := client.PullModel(ctx, reference)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error pulling model: %v\n", err)
		return 1
	}

	fmt.Printf("Successfully pulled model: %s\n", reference)
	fmt.Printf("Model path: %s\n", modelPath)
	return 0
}

func cmdPush(client *distribution.Client, args []string) int {
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "Error: missing arguments\n")
		fmt.Fprintf(os.Stderr, "Usage: model-distribution-tool push <source> <reference>\n")
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

	err := client.PushModel(ctx, source, reference)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error pushing model: %v\n", err)
		return 1
	}

	fmt.Printf("Successfully pushed model: %s\n", reference)
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
		fmt.Printf("%d. Manifest: %s\n", i+1, model.ManifestDigest)
		fmt.Printf("   Tags: %s\n", strings.Join(model.Tags, ", "))
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
	fmt.Printf("Manifest: %s\n", model.ManifestDigest)
	fmt.Printf("Tags: %s\n", strings.Join(model.Tags, ", "))
	return 0
}

func cmdGetPath(client *distribution.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: missing reference argument\n")
		fmt.Fprintf(os.Stderr, "Usage: model-distribution-tool get-path <reference>\n")
		return 1
	}

	reference := args[0]

	modelPath, err := client.GetModelPath(reference)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting model path: %v\n", err)
		return 1
	}

	fmt.Println(modelPath)
	return 0
}

// cmdDelete deletes a model from the registry and local store
func cmdDelete(client *distribution.Client, args []string) int {
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: model-distribution-tool delete <reference>\n")
		return 1
	}

	reference := args[0]
	fmt.Printf("Deleting model %s...\n", reference)

	if err := client.DeleteModel(context.Background(), reference); err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting model: %v\n", err)
		return 1
	}

	fmt.Printf("Model %s deleted successfully\n", reference)
	return 0
}
