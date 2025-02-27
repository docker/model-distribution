# Model Distribution

A library for distributing ML models using container registries.

## Overview

Model Distribution is a Go library that allows you to push, pull, and manage ML models using container registries. It provides a simple API for working with models in GGUF format.

## Features

- Push models to container registries
- Pull models from container registries
- Local model storage
- Model metadata management

## Usage

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
