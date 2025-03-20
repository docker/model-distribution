# Model Distribution

A library and CLI tool for distributing ML models using container registries.

## Overview

Model Distribution is a Go library and CLI tool that allows you to push, pull, and manage ML models using container registries. It provides a simple API and command-line interface for working with models in GGUF format.

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

# Push a model to a registry
./bin/model-distribution-tool push --license license.txt ./model.gguf registry.example.com/models/llama:v1.0

# List all models in the local store
./bin/model-distribution-tool list

# Get information about a model
./bin/model-distribution-tool get registry.example.com/models/llama:v1.0

# Get the local file path for a model
./bin/model-distribution-tool get-path registry.example.com/models/llama:v1.0
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
modelPath, err := client.PullModel(context.Background(), "registry.example.com/models/llama:v1.0")
if err != nil {
    // Handle error
}

// Use the model path - this now returns the direct path to the blob file
// without creating a temporary copy
fmt.Println("Model path:", modelPath)
```
