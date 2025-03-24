#!/bin/bash
set -e

arg1="$1"
shift

if [[ "$arg1" == '--from-hf' || "$arg1" == '-f' ]]; then
    HF_REPO="$1"
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

        echo "Convert to 4-bit quantization..."
        exec ./llama-quantize "$GGUF_FILE" Q4_K_M
else
    exec ./entrypoint.sh "$arg1" "$@"
fi
