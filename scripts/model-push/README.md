# Model Push Script

This script automates the process of converting models from Hugging Face and pushing them to a container registry using the model-distribution-tool.

## Prerequisites

- Docker installed and running
- Hugging Face account and API token
- model-distribution-tool built (run `make build` in the project root)

## Building the Docker Image

To build the Docker image, you'll need to set the following environment variables:

```bash
export HUGGINGFACE_TOKEN="your_huggingface_token"
export DOCKER_USERNAME="your_docker_username"
export DOCKER_PASSWORD="your_docker_password"
```

Then run the Docker build command:

```bash
docker build \
  --secret id=HUGGINGFACE_TOKEN,env=HUGGINGFACE_TOKEN \
  --secret id=docker_username,env=DOCKER_USERNAME \
  --secret id=docker_password,env=DOCKER_PASSWORD \
  --build-arg HUGGINGFACE_REPOSITORY="HuggingFaceTB/SmolLM2-135M-Instruct" \
  --build-arg QUANTIZATION=F16 \
  --build-arg WEIGHTS=135M \
  --build-arg HUB_REPOSITORY=ignaciolopezluna020/smollm2 \
  --build-arg LICENSE="https://huggingface.co/datasets/choosealicense/licenses/resolve/main/markdown/apache-2.0.md" \
. 
```

The build arguments can be customized:
- `HUGGINGFACE_REPOSITORY`: The Hugging Face model repository to use
- `QUANTIZATION`: The quantization type to apply (see Quantization Types section below)

## Usage

```bash
./push-model.sh [OPTIONS]
```

### Options

- `--hf-model HF_NAME/HF_REPO`: Hugging Face model name/repository (required)
- `--repository USER/REPOSITORY`: Target repository (required)
- `--weights WEIGHTS`: Model weights tag (required)
- `--licenses PATH`: Paths to license files (optional, list, default: ./assets/license.txt)
- `--models-dir PATH`: Path to store models (default: ./models)
- `--hf-token TOKEN`: Hugging Face token (required)
- `--quantization TYPE`: Quantization type to use (default: Q4_K_M)
- `--skip-f16`: Skip pushing the F16 (non-quantized) version
- `--help`: Display help message

### Quantization Types

The following quantization types are supported:

- `Q4_0`, `Q4_1`: 4-bit quantization (different methods)
- `Q5_0`, `Q5_1`: 5-bit quantization (different methods)
- `Q8_0`, `Q8_1`: 8-bit quantization (different methods)
- `Q2_K`, `Q3_K_S`, `Q3_K_M`, `Q3_K_L`: K-quant with 2-3 bits
- `Q4_K_S`, `Q4_K_M`: K-quant with 4 bits (small and medium, Q4_K_M is default)
- `Q5_K_S`, `Q5_K_M`: K-quant with 5 bits (small and medium)
- `Q6_K`: K-quant with 6 bits
- `F16`: 16-bit floating point (no quantization)
- `F32`: 32-bit floating point (no quantization)

### Examples

Basic usage with default quantization (Q4_K_M):
```bash
./push-model.sh \
  --hf-model meta-llama/Llama-2-7b-chat-hf \
  --repository myregistry.com/models/llama \
  --weights 7B \
  --hf-token hf_xxx \
  --licenses ./assets/license.txt
```

Using a specific quantization type:
```bash
./push-model.sh \
  --hf-model meta-llama/Llama-2-7b-chat-hf \
  --repository myregistry.com/models/llama \
  --weights 7B \
  --hf-token hf_xxx \
  --quantization Q8_0 \
  --licenses ./assets/license.txt
```

Skip pushing the F16 version:
```bash
./push-model.sh \
  --hf-model meta-llama/Llama-2-7b-chat-hf \
  --repository myregistry.com/models/llama \
  --weights 7B \
  --hf-token hf_xxx \
  --skip-f16 \
  --licenses ./assets/license.txt
```

Push only the F16 version (no quantization):
```bash
./push-model.sh \
  --hf-model meta-llama/Llama-2-7b-chat-hf \
  --repository myregistry.com/models/llama \
  --weights 7B \
  --hf-token hf_xxx \
  --quantization F16 \
  --licenses ./assets/license.txt
```

Using multiple license files:
```bash
./push-model.sh \
  --hf-model meta-llama/Llama-2-7b-chat-hf \
  --repository myregistry.com/models/llama \
  --weights 7B \
  --hf-token hf_xxx \
  --licenses ./assets/license1.txt,./assets/license2.txt,./assets/license3.txt
```

## Process

The script performs the following steps:

1. Runs a Docker container to convert the model from Hugging Face to GGUF format with the specified quantization
2. Verifies both the quantized model and F16 model files were created successfully
3. Checks for the license file
4. Pushes the quantized model to the specified repository
5. Pushes the F16 model to the same repository with a "-F16" suffix in the tag (unless skipped)

## Notes

- The script creates the models directory if it doesn't exist
- By default, it pushes both the quantized version and the F16 version of the model
- The F16 version is pushed with a "-F16" suffix added to the tag
- If any license file is not found, the script will display an error and exit
- You can specify multiple license files by separating them with commas: `--licenses file1.txt,file2.txt`
- You can skip pushing the F16 version with the `--skip-f16` flag
- If you specify `--quantization F16`, only the F16 version will be pushed
- The script will exit with an error if any critical step fails (Docker not installed, model conversion fails, etc.)
