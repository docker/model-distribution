#!/bin/bash
set -e

# Default values
DEFAULT_LICENSE_PATH="$(pwd)/assets/license.txt"
DEFAULT_MODELS_DIR="$(pwd)/models"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Function to display usage information
usage() {
    echo "Usage: $0 [OPTIONS]"
    echo
    echo "Options:"
    echo "  --hf-model HF_NAME/HF_REPO    Hugging Face model name/repository (required)"
    echo "  --target REPOSITORY/TAG       Target repository and tag (required)"
    echo "  --license PATH                Path to license file (default: ${DEFAULT_LICENSE_PATH})"
    echo "  --models-dir PATH             Path to store models (default: ${DEFAULT_MODELS_DIR})"
    echo "  --hf-token TOKEN              Hugging Face token (required)"
    echo "  --help                        Display this help message"
    echo
    echo "Example:"
    echo "  $0 --hf-model meta-llama/Llama-2-7b-chat-hf --target myregistry.com/models/llama:v1.0 --hf-token hf_xxx"
    exit 1
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --hf-model)
            HF_MODEL="$2"
            shift 2
            ;;
        --target)
            TARGET="$2"
            shift 2
            ;;
        --license)
            LICENSE_PATH="$2"
            shift 2
            ;;
        --models-dir)
            MODELS_DIR="$2"
            shift 2
            ;;
        --hf-token)
            HF_TOKEN="$2"
            shift 2
            ;;
        --help)
            usage
            ;;
        *)
            echo "Unknown option: $1"
            usage
            ;;
    esac
done

# Validate required parameters
if [ -z "$HF_MODEL" ]; then
    echo "Error: Hugging Face model (--hf-model) is required"
    usage
fi

if [ -z "$TARGET" ]; then
    echo "Error: Target repository/tag (--target) is required"
    usage
fi

if [ -z "$HF_TOKEN" ]; then
    echo "Error: Hugging Face token (--hf-token) is required"
    usage
fi

# Set default values if not provided
LICENSE_PATH="${LICENSE_PATH:-$DEFAULT_LICENSE_PATH}"
MODELS_DIR="${MODELS_DIR:-$DEFAULT_MODELS_DIR}"

# Create models directory if it doesn't exist
mkdir -p "$MODELS_DIR"

# Check if Docker is installed
if ! command -v docker &> /dev/null; then
    echo "Error: Docker is not installed or not in PATH"
    exit 1
fi

# Check if model-distribution-tool exists
if [ ! -f "${PROJECT_ROOT}/bin/model-distribution-tool" ]; then
    echo "Error: model-distribution-tool not found at ${PROJECT_ROOT}/bin/model-distribution-tool"
    echo "Please build the tool first with 'make build'"
    exit 1
fi

echo "=== Model Push Script ==="
echo "Hugging Face Model: $HF_MODEL"
echo "Target Repository: $TARGET"
echo "License Path: $LICENSE_PATH"
echo "Models Directory: $MODELS_DIR"
echo

# Step 1: Run Docker container to convert the model from Hugging Face
echo "Step 1: Converting model from Hugging Face..."
docker run --rm \
    -e HUGGINGFACE_TOKEN="$HF_TOKEN" \
    -v "$MODELS_DIR:/models" \
    ignaciolopezluna020/llama-converter:latest \
    --from-hf "$HF_MODEL"

# Check if the model was converted successfully
MODEL_FILE="$MODELS_DIR"/"$(echo "$HF_MODEL" | sed 's/.*\///')"/ggml-model-Q4_K_M.gguf

# Check if the model file exists
if [ ! -f "$MODEL_FILE" ]; then
    echo "Error: Model file not found at $MODEL_FILE"
    exit 1
fi

echo "Model file: $MODEL_FILE"

# Step 2: Check for license file
echo "Step 2: Checking for license file..."
LICENSE_FLAG=""
if [ ! -f "$LICENSE_PATH" ]; then
    echo "Warning: License file not found at $LICENSE_PATH"
    echo "Proceeding without license file..."
else
    echo "License file found: $LICENSE_PATH"
    LICENSE_FLAG="--license $LICENSE_PATH"
fi

# Step 3: Push the model to the repository
echo "Step 3: Pushing model to repository..."
if [ -n "$LICENSE_FLAG" ]; then
    "${PROJECT_ROOT}/bin/model-distribution-tool" push $LICENSE_FLAG "$MODEL_FILE" "$TARGET"
else
    "${PROJECT_ROOT}/bin/model-distribution-tool" push "$MODEL_FILE" "$TARGET"
fi

echo "Model successfully pushed to $TARGET"
