ARG LLAMA_CPP_TAG=full-b5763
FROM ghcr.io/ggml-org/llama.cpp:${LLAMA_CPP_TAG}

# Install git-lfs to handle Hugging Face repositories
RUN apt-get update && apt-get install -y git-lfs && \
    git lfs install

# Copy the modified entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Allow passing Hugging Face API Token as an environment variable
ENV HUGGINGFACE_TOKEN=""

ENTRYPOINT ["/entrypoint.sh"]
