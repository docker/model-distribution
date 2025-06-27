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
  uses: docker/model-distribution/.github/actions/package-model@main
  with:
    gguf-file-url: 'url-to-gguf'
    registry-repository: 'myorg/mymodel'
    tag: '4B-Q4_K_M'
    license-url: 'https://example.com/license.txt'
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
        default: 'https://huggingface.co/unsloth/SmolLM2-135M-Instruct-GGUF/resolve/main/SmolLM2-135M-Instruct-Q2_K.gguf'
      model_name:
        description: 'Model name for the repository'
        required: true
        default: 'smollm2'
      model_tag:
        description: 'Tag for the model'
        required: true
        default: '135M-Q2_K'
      license_url:
        description: 'License URL'
        required: true
        default: 'https://huggingface.co/datasets/choosealicense/licenses/resolve/main/markdown/apache-2.0.md'

jobs:
  package:
    runs-on: ubuntu-latest
    steps:
      - name: Package Model
        id: package
        uses: docker/model-distribution/.github/actions/package-model@main
        with:
          gguf-file-url: ${{ github.event.inputs.model_url }}
          registry-repository: ${{ secrets.DOCKER_USERNAME }}/${{ github.event.inputs.model_name }}
          tag: ${{ github.event.inputs.model_tag }}
          license-url: ${{ github.event.inputs.license_url }}
          docker-username: ${{ secrets.DOCKER_USERNAME }}
          docker-password: ${{ secrets.DOCKER_PASSWORD }}

      - name: Show packaged model reference
        run: |
          echo "✅ Successfully packaged model!"
          echo "📦 Model Reference: ${{ steps.package.outputs.model-reference }}"
          echo "🔗 Model URL: ${{ github.event.inputs.model_url }}"
          echo "📄 License URL: ${{ github.event.inputs.license_url }}"
          echo ""
          echo "You can now pull this model using:"
          echo "docker pull ${{ steps.package.outputs.model-reference }}"
```

## Inputs

| Input                    | Description                                     | Required | Default              |
|--------------------------|-------------------------------------------------|----------|----------------------|
| `gguf-file-url`          | URL to the GGUF file                            | Yes      | -                    |
| `registry-repository`    | OCI Registry repository (e.g., `myorg/mymodel`) | Yes      | -                    |
| `tag`                    | Tag for the model (e.g., `v1.0`, `4B-Q4_K_M`)   | Yes      | -                    |
| `license-url`            | URL to the license file                         | Yes      | -                    |
| `docker-username`        | Docker Hub username                             | Yes      | -                    |
| `docker-password`        | Docker Hub password/token                       | Yes      | -                    |
| `platforms`              | Target platforms for the build                  | No       | `linux/arm64`        |
| `buildx-endpoint`        | Docker Buildx cloud endpoint                    | No       | `{username}/default` |
| `model-distribution-ref` | Git ref of model-distribution repo to use       | No       | `main`               |

## Outputs

| Output            | Description                                                       |
|-------------------|-------------------------------------------------------------------|
| `model-reference` | Full model reference that was pushed (e.g., `myorg/mymodel:v1.0`) |

## Requirements

- Docker Hub credentials stored as repository secrets
- Access to Docker Build Cloud (for improved performance)
- The action automatically checks out the `docker/model-distribution` repository for the build context

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
