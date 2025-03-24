# Model Push Script

This script automates the process of converting models from Hugging Face and pushing them to a container registry using the model-distribution-tool.

## Prerequisites

- Docker installed and running
- Hugging Face account and API token
- model-distribution-tool built (run `make build` in the project root)

## Usage

```bash
./push-model.sh [OPTIONS]
```

### Options

- `--hf-model HF_NAME/HF_REPO`: Hugging Face model name/repository (required)
- `--target REPOSITORY/TAG`: Target repository and tag (required)
- `--license PATH`: Path to license file (optional, default: ./assets/license.txt)
- `--models-dir PATH`: Path to store models (default: ./models)
- `--hf-token TOKEN`: Hugging Face token (required)
- `--help`: Display help message

### Example

```bash
./push-model.sh \
  --hf-model meta-llama/Llama-2-7b-chat-hf \
  --target myregistry.com/models/llama:v1.0 \
  --hf-token hf_xxx
```

## Process

The script performs the following steps:

1. Runs a Docker container to convert the model from Hugging Face to GGUF format
2. Verifies the model was converted successfully
3. Checks for the license file
4. Uses model-distribution-tool to push the model to the specified repository

## Notes

- The script creates the models directory if it doesn't exist
- It uses the first GGUF file found in the models directory
- If the license file is not found, the script will display a warning and proceed without it
- The script will exit with an error if any critical step fails (Docker not installed, model conversion fails, etc.)
