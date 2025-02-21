# Model Distribution Tool

[![CI](https://github.com/docker/model-distribution/actions/workflows/ci.yml/badge.svg)](https://github.com/docker/model-distribution/actions/workflows/ci.yml)

A tool for packaging and distributing AI models as OCI artifacts in container registries.

## Development

### Configuration

Copy the example environment file and update with your values:

```bash
cp .env.example .env
```

Required environment variables for tests:
- `DOCKER_REGISTRY`: Registry URL (default: docker.io)
- `DOCKER_USERNAME`: Your Docker registry username
- `DOCKER_PASSWORD`: Your Docker Personal Access Token (not your password)

### Building

```bash
# Build the binary
make build

# Run tests
make test

# Clean build artifacts
make clean
```

See `make help` for all available commands.

## Usage

The tool requires two parameters:
- `--source`: Path to local file or URL to download
- `--tag`: Target registry/repository:tag where the model will be pushed

### Using make run

The easiest way to run the tool is using the `make run` command:

```bash
# Make sure your .env file is configured with DOCKER_* variables
make run SOURCE=path/to/model.gguf TAG=registry.example.com/my-model:latest
```

### Using the binary directly

Package a local model file:
```
./model-distribution-tool --source "/Users/ilopezluna/Downloads/llama-2-7b-chat.Q2_K.gguf" --tag registry.example.com/my-model:latest
```

Package a remote model file:
```
./model-distribution-tool --source "https://huggingface.co/TheBloke/Llama-2-7B-Chat-GGUF/resolve/191239b/llama-2-7b-chat.Q2_K.gguf" --tag registry.example.com/my-model:latest
```
