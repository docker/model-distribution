# syntax=docker/dockerfile:1

ARG GO_VERSION=1.24.2
ARG GIT_VERSION=v2.47.2

FROM golang:${GO_VERSION}-bookworm AS builder

# Install git for go mod download if needed
RUN apt-get update && apt-get install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy go mod/sum first for better caching
COPY --link go.mod go.sum ./

# Download dependencies (with cache mounts)
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# Copy the rest of the source code
COPY --link . .

# Build the application
RUN make build

FROM alpine/git:${GIT_VERSION} AS cloner

ARG HUGGINGFACE_REPOSITORY

WORKDIR /app

RUN git lfs install

RUN --mount=type=secret,id=HUGGINGFACE_TOKEN,env=HUGGINGFACE_TOKEN \
    git clone --depth=1 "https://user:$HUGGINGFACE_TOKEN@huggingface.co/$HUGGINGFACE_REPOSITORY" "model"

FROM ignaciolopezluna020/llama-converter:latest AS ggufier

# Copy the cloned model from the cloner stage
COPY --from=cloner /app/model /model

RUN ./convert_hf_to_gguf.py --outfile /model/model.gguf /model

FROM ignaciolopezluna020/llama-converter:latest AS quantizier

ARG QUANTIZATION

# Copy the model in GGUF format from the ggufier stage
COPY --from=ggufier /model/model.gguf /model/model.gguf

RUN ./llama-quantize /model/model.gguf $QUANTIZATION

FROM ubuntu:24.04 AS packager

ARG HUB_REPOSITORY
ARG QUANTIZATION
ARG WEIGHTS

COPY --from=quantizier /model/ggml-model-$QUANTIZATION.gguf /model/model.gguf
COPY --from=builder /app/bin/model-distribution-tool /usr/local/bin/model-distribution-tool

# Install CA certificates for SSL verification
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

# Login to Docker Hub using build secrets
RUN --mount=type=secret,id=docker_username,env=DOCKER_USERNAME \
    --mount=type=secret,id=docker_password,env=DOCKER_PASSWORD \
    model-distribution-tool package \
    /model/model.gguf \
    $HUB_REPOSITORY:$WEIGHTS-$QUANTIZATION
