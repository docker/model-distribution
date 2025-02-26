# Model Distribution

This repository contains tools for distributing AI models to container registries like Google Artifact Registry (GAR) and Amazon Elastic Container Registry (ECR).

## Features

- Push models to container registries using OCI artifacts
- Pull models from container registries
- Verify model integrity after push/pull operations
- CI/CD workflows for testing with GAR and ECR

## Usage

### Command Line

```bash
# Push a model to a registry
go run main.go --source path/to/model.gguf --tag registry/repository:tag

# Example for GAR
go run main.go --source assets/dummy.gguf --tag us-east4-docker.pkg.dev/project-id/repository/model:v1.0.0

# Example for ECR
go run main.go --source assets/dummy.gguf --tag 123456789012.dkr.ecr.us-east-1.amazonaws.com/repository/model:v1.0.0
```

### As a Library

```go
import "github.com/your-org/model-distribution"

// Push a model
ref, err := PushModel("path/to/model.gguf", "registry/repository:tag")
if err != nil {
    log.Fatal(err)
}

// Pull a model
img, err := PullModel("registry/repository:tag")
if err != nil {
    log.Fatal(err)
}
```

## CI/CD Workflows

This repository includes GitHub Actions workflows for testing model distribution with different container registries. The verify-registry-push-pull.yml workflow tests pushing and pulling models to/from GAR and ECR.

### Environment Variables

For GAR integration tests:
`TEST_GAR_ENABLED`: Set to "true" to enable GAR tests
`TEST_GAR_TAG`: Full GAR tag (e.g., "us-east4-docker.pkg.dev/project-id/repository/model:v1.0.0")

For ECR integration tests:
`TEST_ECR_ENABLED`: Set to "true" to enable ECR tests
`TEST_ECR_TAG`: Full ECR tag (e.g., "123456789012.dkr.ecr.us-east-1.amazonaws.com/repository/model:v1.0.0")

## Development

### Running Tests

```bash
# Run all tests
go test -v ./...

# Run only GAR integration tests (using full tag)
TEST_GAR_ENABLED=true TEST_GAR_TAG=us-east4-docker.pkg.dev/project-id/repository/model:v1.0.0 go test -v -run TestGARIntegration

# Run only ECR integration tests (using full tag)
TEST_ECR_ENABLED=true TEST_ECR_TAG=123456789012.dkr.ecr.us-east-1.amazonaws.com/repository/model:v1.0.0 go test -v -run TestECRIntegration
```
