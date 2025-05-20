# Model Distribution

A library and CLI tool for distributing models using container registries.

## Overview

Model Distribution is a Go library and CLI tool that allows you to package, push, pull, and manage models using container registries. It provides a simple API and command-line interface for working with models in GGUF format.

## Features

- Push models to container registries
- Pull models from container registries
- Local model storage
- Model metadata management
- Command-line interface for all operations

## Usage

### As a CLI Tool

```bash
# Build the CLI tool
make build

# Pull a model from a registry
./bin/model-distribution-tool pull registry.example.com/models/llama:v1.0

# Package a model and push to a registry
./bin/model-distribution-tool package ./model.gguf registry.example.com/models/llama:v1.0

# Package a model with license files and push to a registry
./bin/model-distribution-tool package --licenses license1.txt --licenses license2.txt ./model.gguf registry.example.com/models/llama:v1.0

# Push a model from the content store to the registry
./bin/model-distribution-tool push registry.example.com/models/llama:v1.0

# List all models in the local store
./bin/model-distribution-tool list

# Get information about a model
./bin/model-distribution-tool get registry.example.com/models/llama:v1.0

# Get the local file path for a model
./bin/model-distribution-tool get-path registry.example.com/models/llama:v1.0

# Remove a model from the local store (will untag w/o deleting if there are multiple tags)
./bin/model-distribution-tool rm registry.example.com/models/llama:v1.0

# Force Removal of a model from the local store, even when there are multiple referring tags
./bin/model-distribution-tool rm --force sha256:0b329b335467cccf7aa219e8f5e1bd65e59b6dfa81cfa42fba2f8881268fbf82

# Tag a model with an additional reference
./bin/model-distribution-tool tag registry.example.com/models/llama:v1.0 registry.example.com/models/llama:latest
```

For more information about the CLI tool, run:

```bash
./bin/model-distribution-tool --help
```

### As a Library

```go
import (
    "context"
    "github.com/docker/model-distribution/pkg/distribution"
)

// Create a new client
client, err := distribution.NewClient("/path/to/cache")
if err != nil {
    // Handle error
}

// Pull a model
err := client.PullModel(context.Background(), "registry.example.com/models/llama:v1.0", os.Stdout)
if err != nil {
    // Handle error
}

// Get a model
model, err := client.GetModel("registry.example.com/models/llama:v1.0")
if err != nil {
    // Handle error
}

// Get the GGUF file path
modelPath, err := model.GGUFPath()
if err != nil {
    // Handle error
}

fmt.Println("Model path:", modelPath)

// List all models
models, err := client.ListModels()
if err != nil {
    // Handle error
}

// Delete a model
err = client.DeleteModel("registry.example.com/models/llama:v1.0", false)
if err != nil {
    // Handle error
}

// Tag a model
err = client.Tag("registry.example.com/models/llama:v1.0", "registry.example.com/models/llama:latest")
if err != nil {
    // Handle error
}

// Push a model
err = client.PushModel("registry.example.com/models/llama:v1.0")
if err != nil {
    // Handle error
}
```
