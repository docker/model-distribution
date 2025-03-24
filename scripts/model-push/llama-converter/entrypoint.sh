#!/bin/bash
set -e

# Default quantization type
QUANTIZATION="Q4_K_M"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        '--from-hf'|'-f')
            FROM_HF=true
            shift
            HF_REPO="$1"
            shift
            ;;
        '--quantization'|'-q')
            shift
            QUANTIZATION="$1"
            shift
            ;;
        *)
            # Pass other arguments to the original entrypoint
            EXTRA_ARGS+=("$1")
            shift
            ;;
    esac
done

# Validate quantization type
VALID_QUANTIZATIONS=("Q4_0" "Q4_1" "Q5_0" "Q5_1" "Q8_0" "Q8_1" "Q2_K" "Q3_K_S" "Q3_K_M" "Q3_K_L" "Q4_K_S" "Q4_K_M" "Q5_K_S" "Q5_K_M" "Q6_K" "F16" "F32")
VALID=false
for q in "${VALID_QUANTIZATIONS[@]}"; do
    if [[ "$q" == "$QUANTIZATION" ]]; then
        VALID=true
        break
    fi
done

if [[ "$VALID" == "false" ]]; then
    echo "Error: Invalid quantization type: $QUANTIZATION"
    echo "Valid options are: ${VALID_QUANTIZATIONS[*]}"
    exit 1
fi

if [[ "$FROM_HF" == "true" ]]; then
    TARGET_DIR="/models/$(basename $HF_REPO)"

    if [[ -z "$HUGGINGFACE_TOKEN" ]]; then
        echo "Error: Hugging Face token is missing. Set HUGGINGFACE_TOKEN environment variable."
        exit 1
    fi

    if [[ -d "$TARGET_DIR" ]]; then
        echo "Repository already cloned at $TARGET_DIR. Skipping cloning."
    else
        echo "Cloning Hugging Face repository: $HF_REPO into $TARGET_DIR..."
        git lfs install
        git clone "https://user:$HUGGINGFACE_TOKEN@huggingface.co/$HF_REPO" "$TARGET_DIR"
    fi

    echo "Running conversion..."
    python3 ./convert_hf_to_gguf.py "$TARGET_DIR"

    # Find the correct *-F16.gguf file
    GGUF_FILE=$(find "$TARGET_DIR" -type f -name "*-F16.gguf" | head -n 1)

    if [[ -z "$GGUF_FILE" ]]; then
        echo "Error: No F16 GGUF file found in $TARGET_DIR."
        exit 1
    fi

    # Skip quantization if F16 is requested
    if [[ "$QUANTIZATION" == "F16" ]]; then
        echo "F16 format requested, skipping quantization..."
    else
        echo "Converting to $QUANTIZATION quantization..."
        ./llama-quantize "$GGUF_FILE" "$QUANTIZATION"
    fi
else
    # If not processing from Hugging Face, pass all arguments to the original entrypoint
    exec ./entrypoint.sh "${EXTRA_ARGS[@]}"
fi
