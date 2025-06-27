# Package Model Action

A composite GitHub Action that packages GGUF model files as OCI artifacts and pushes them to a registry using Docker Build Cloud.

## Features

- Downloads GGUF model files from any URL (e.g., Hugging Face)
- Downloads license files automatically
- Uses Docker Build Cloud for improved performance and caching
- Packages models as OCI artifacts using existing Dockerfile
- Pushes to Docker Hub or any OCI-compatible registry
- Provides the full model reference as output

## Usage

### Basic Usage

```yaml
- name: Package Model
  uses: ./.github/actions/package-model
  with:
    gguf-file-url: 'url-to-gguf'
    registry-repository: 'myorg/mymodel'
    tag: '4B-Q4_K_M'
    docker-username: ${{ secrets.DOCKER_USERNAME }}
    docker-password: ${{ secrets.DOCKER_PASSWORD }}
```

### Complete Example Workflow

```yaml
name: Package Model Example

on:
  workflow_dispatch:
    inputs:
      model_url:
        description: 'URL to the GGUF model file'
        required: true
      model_name:
        description: 'Model name for the repository'
        required: true
      model_tag:
        description: 'Tag for the model'
        required: true

jobs:
  package:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout repository
        uses: actions/checkout@v4

      - name: Package Model
        id: package
        uses: ./.github/actions/package-model
        with:
          gguf-file-url: ${{ github.event.inputs.model_url }}
          registry-repository: ${{ secrets.DOCKER_USERNAME }}/${{ github.event.inputs.model_name }}
          tag: ${{ github.event.inputs.model_tag }}
          docker-username: ${{ secrets.DOCKER_USERNAME }}
          docker-password: ${{ secrets.DOCKER_PASSWORD }}

      - name: Show packaged model reference
        run: |
          echo "Model packaged as: ${{ steps.package.outputs.model-reference }}"
```

## Inputs

| Input                 | Description                                     | Required | Default              |
|-----------------------|-------------------------------------------------|----------|----------------------|
| `gguf-file-url`       | URL to the GGUF file                            | Yes      | -                    |
| `registry-repository` | OCI Registry repository (e.g., `myorg/mymodel`) | Yes      | -                    |
| `tag`                 | Tag for the model (e.g., `v1.0`, `4B-Q4_K_M`)   | Yes      | -                    |
| `license-url`         | URL to the license file                         | yes      | -                    |
| `docker-username`     | Docker Hub username                             | Yes      | -                    |
| `docker-password`     | Docker Hub password/token                       | Yes      | -                    |
| `buildx-endpoint`     | Docker Buildx cloud endpoint                    | No       | `{username}/default` |

## Outputs

| Output            | Description                                                       |
|-------------------|-------------------------------------------------------------------|
| `model-reference` | Full model reference that was pushed (e.g., `myorg/mymodel:v1.0`) |

## Requirements

- The repository must contain the model-distribution-tool source code and Dockerfile
- Docker Hub credentials stored as repository secrets
- Access to Docker Build Cloud (for improved performance)

## Secrets Required

Store these as repository secrets:

- `DOCKER_USERNAME`: Your Docker Hub username
- `DOCKER_PASSWORD`: Your Docker Hub password or access token

## Error Handling

The action includes automatic cleanup of temporary files and provides detailed logging for troubleshooting. If the action fails:

1. Check that the GGUF file URL is accessible
2. Verify Docker Hub credentials are correct
3. Ensure the repository name follows Docker Hub naming conventions
4. Check the GitHub Actions logs for specific error messages

## Development

To modify this action:

1. Edit `.github/actions/package-model/action.yml`
2. Test with a workflow that uses the action
3. Commit and push changes

The action will use the version from the current commit when referenced locally.
