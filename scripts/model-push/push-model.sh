#!/bin/bash
set -e

# Default values
DEFAULT_LICENSE_PATH="$(pwd)/assets/license.txt"
DEFAULT_MODELS_DIR="$(pwd)/models"
DEFAULT_QUANTIZATION="Q4_K_M"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Function to display usage information
usage() {
    echo "Usage: $0 [OPTIONS]"
    echo
    echo "Options:"
    echo "  --hf-model HF_NAME/HF_REPO    Hugging Face model name/repository (required)"
    echo "  --target REPOSITORY/TAG       Target repository and tag (required)"
    echo "  --license PATH                Path to license file (optional, default: ${DEFAULT_LICENSE_PATH})"
    echo "  --models-dir PATH             Path to store models (default: ${DEFAULT_MODELS_DIR})"
    echo "  --hf-token TOKEN              Hugging Face token (required)"
    echo "  --quantization TYPE           Quantization type to use (default: ${DEFAULT_QUANTIZATION})"
    echo "  --skip-f16                    Skip pushing the F16 (non-quantized) version"
    echo "  --help                        Display this help message"
    echo
    echo "Available quantization types:"
    echo "  Q4_0, Q4_1, Q5_0, Q5_1, Q8_0, Q8_1, Q2_K, Q3_K_S, Q3_K_M, Q3_K_L,"
    echo "  Q4_K_S, Q4_K_M (default), Q5_K_S, Q5_K_M, Q6_K, F16, F32"
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
        --quantization)
            QUANTIZATION="$2"
            shift 2
            ;;
        --skip-f16)
            SKIP_F16=true
            shift
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
QUANTIZATION="${QUANTIZATION:-$DEFAULT_QUANTIZATION}"
SKIP_F16="${SKIP_F16:-false}"

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
echo "Quantization: $QUANTIZATION"
echo "Skip F16 Version: $SKIP_F16"
echo

# Step 1: Run Docker container to convert the model from Hugging Face
echo "Step 1: Converting model from Hugging Face..."
docker run --rm \
    -e HUGGINGFACE_TOKEN="$HF_TOKEN" \
    -v "$MODELS_DIR:/models" \
    ignaciolopezluna020/llama-converter:latest \
    --from-hf "$HF_MODEL" --quantization "$QUANTIZATION"

# Get the model name from the HF_MODEL
MODEL_NAME="$(echo "$HF_MODEL" | sed 's/.*\///')"
MODEL_DIR="$MODELS_DIR/$MODEL_NAME"

# Define paths for both model versions
if [[ "$QUANTIZATION" == "F16" ]]; then
    # If F16 is requested, there's only one model file
    QUANTIZED_MODEL_FILE="$MODEL_DIR"/"$MODEL_NAME"-F16.gguf
    F16_MODEL_FILE="$QUANTIZED_MODEL_FILE"
else
    # For other quantization types, we have both quantized and F16 versions
    QUANTIZED_MODEL_FILE="$MODEL_DIR/ggml-model-$QUANTIZATION.gguf"
    F16_MODEL_FILE="$MODEL_DIR"/"$MODEL_NAME"-F16.gguf
fi

# Check if the quantized model file exists
if [ ! -f "$QUANTIZED_MODEL_FILE" ]; then
    echo "Error: Quantized model file not found at $QUANTIZED_MODEL_FILE"
    exit 1
fi

echo "Quantized model file: $QUANTIZED_MODEL_FILE"

# Check if the F16 model file exists (if we're not skipping it)
if [ "$SKIP_F16" != "true" ] && [ "$QUANTIZATION" != "F16" ]; then
    if [ ! -f "$F16_MODEL_FILE" ]; then
        echo "Warning: F16 model file not found. Skipping F16 model push."
        SKIP_F16=true
    else
        echo "F16 model file: $F16_MODEL_FILE"
    fi
fi

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

# Step 3: Push the model(s) to the repository
echo "Step 3: Pushing model(s) to the repository..."

# Push the quantized model
echo "Pushing quantized model ($QUANTIZATION) to $TARGET..."
if [ -n "$LICENSE_FLAG" ]; then
    "${PROJECT_ROOT}/bin/model-distribution-tool" push $LICENSE_FLAG "$QUANTIZED_MODEL_FILE" "$TARGET"
else
    "${PROJECT_ROOT}/bin/model-distribution-tool" push "$QUANTIZED_MODEL_FILE" "$TARGET"
fi

# Push the F16 model if not skipped and not already pushed (when QUANTIZATION=F16)
if [ "$SKIP_F16" != "true" ] && [ "$QUANTIZATION" != "F16" ]; then
    # Create F16 tag by appending "-F16" to the target tag
    F16_TARGET="${TARGET%:*}:${TARGET##*:}-F16"
    echo "Pushing F16 model to $F16_TARGET..."
    
    if [ -n "$LICENSE_FLAG" ]; then
        "${PROJECT_ROOT}/bin/model-distribution-tool" push $LICENSE_FLAG "$F16_MODEL_FILE" "$F16_TARGET"
    else
        "${PROJECT_ROOT}/bin/model-distribution-tool" push "$F16_MODEL_FILE" "$F16_TARGET"
    fi
    
    echo "F16 model successfully pushed to $F16_TARGET"
fi

echo "Quantized model successfully pushed to $TARGET"
