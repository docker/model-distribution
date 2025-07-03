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

# Package a model with a default context size and push to a registry
./bin/model-distribution-tool ./model.gguf --context-size 2048 registry.example.com/models/llama:v1.0

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

### GitHub Workflows for Model Packaging and Promotion

This project provides GitHub workflows to automate the process of packaging GGUF models and promoting them from staging to production environments.

#### Overview

The model promotion process follows a two-step workflow:
1. **Package and Push to Staging**: Use `package-gguf-model.yml` to download a GGUF model from HuggingFace and push it to the `aistaging` namespace
2. **Promote to Production**: Use `promote-model-to-production.yml` to copy the model from staging (`aistaging`) to production (`ai`) namespace

#### Prerequisites

The following GitHub secrets must be configured:
- `DOCKER_USER`: DockerHub username for production namespace
- `DOCKER_OAT`: DockerHub access token for production namespace
- `DOCKER_USER_STAGING`: DockerHub username for staging namespace (typically `aistaging`)
- `DOCKER_OAT_STAGING`: DockerHub access token for staging namespace

**Note**: The current secrets are configured to write to the `ai` production namespace. If you need to write to a different namespace, you'll need to update the `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` secrets accordingly.

#### Step 1: Package Model to Staging

Use the **Package GGUF model** workflow to download a model from HuggingFace and push it to the staging environment.

**Single Model Example:**
1. Go to Actions → Package GGUF model → Run workflow
2. Fill in the inputs:
   - **GGUF file URL**: `https://huggingface.co/unsloth/SmolLM2-135M-Instruct-GGUF/resolve/main/SmolLM2-135M-Instruct-Q4_K_M.gguf`
   - **Registry repository**: `smollm2`
   - **Tag**: `135M-Q4_K_M`
   - **License URL**: `https://huggingface.co/datasets/choosealicense/licenses/resolve/main/markdown/apache-2.0.md`

This will create: `aistaging/smollm2:135M-Q4_K_M`

**Multi-Model Example:**
For packaging multiple models at once, use the `models_json` input:
```json
[
  {
    "gguf_url": "https://huggingface.co/unsloth/Qwen3-32B-GGUF/resolve/main/Qwen3-32B-Q4_K_XL.gguf",
    "repository": "qwen3-gguf",
    "tag": "32B-Q4_K_XL",
    "license_url": "https://huggingface.co/datasets/choosealicense/licenses/resolve/main/markdown/apache-2.0.md"
  },
  {
    "gguf_url": "https://huggingface.co/unsloth/Qwen3-32B-GGUF/resolve/main/Qwen3-32B-Q8_0.gguf",
    "repository": "qwen3-gguf", 
    "tag": "32B-Q8_0",
    "license_url": "https://huggingface.co/datasets/choosealicense/licenses/resolve/main/markdown/apache-2.0.md"
  }
]
```

#### Step 2: Promote to Production

Once your model is successfully packaged in staging, use the **Promote Model to Production** workflow to copy it to the production namespace.

1. Go to Actions → Promote Model to Production → Run workflow
2. Fill in the inputs:
   - **Image**: `smollm2:135M-Q4_K_M` (must match the repository:tag from Step 1)
   - **Source namespace**: `aistaging` (default, can be changed if needed)
   - **Target namespace**: `ai` (default, can be changed if needed)

This will copy: `aistaging/smollm2:135M-Q4_K_M` → `ai/smollm2:135M-Q4_K_M`

#### Complete Example Walkthrough

Let's walk through packaging and promoting a Qwen3 model:

1. **Package to Staging**:
   - Workflow: Package GGUF model
   - GGUF file URL: `https://huggingface.co/unsloth/SmolLM2-135M-Instruct-GGUF/resolve/main/SmolLM2-135M-Instruct-Q4_K_M.gguf`
   - Registry repository: `smollm2`
   - Tag: `135M-Q4_K_M`
   - License URL: `https://huggingface.co/datasets/choosealicense/licenses/resolve/main/markdown/apache-2.0.md`
   - Result: `aistaging/smollm2:135M-Q4_K_M`

2. **Promote to Production**:
   - Workflow: Promote Model to Production
   - Image: `smollm2:135M-Q4_K_M`
   - Result: `ai/smollm2:135M-Q4_K_M`

Your model is now available in production and can be pulled using:
```bash
docker pull ai/smollm2:135M-Q4_K_M
```
