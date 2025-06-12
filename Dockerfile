# syntax=docker/dockerfile:1

ARG GO_VERSION=1.24.2
ARG GIT_VERSION=v2.47.2
ARG UBUNTU_VERSION=24.04

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

FROM ubuntu:${UBUNTU_VERSION} AS downloader

ARG GGUF_FILE_URL
ARG LICENSE_URL

WORKDIR /app

# Install curl for downloading the files
RUN apt-get update && apt-get install -y curl ca-certificates && rm -rf /var/lib/apt/lists/*

# Create model directory and download the GGUF file
RUN mkdir -p /model && \
    curl -L "$GGUF_FILE_URL" -o /model/model.gguf

# Create licenses directory and download the license file
RUN mkdir -p /licenses && \
    curl -L "$LICENSE_URL" -o /licenses/LICENSE

FROM ubuntu:${UBUNTU_VERSION} AS packager

ARG HUB_REPOSITORY
ARG TAG

COPY --link --from=downloader /model/model.gguf /model/model.gguf
COPY --link --from=downloader /licenses/LICENSE /licenses/LICENSE
COPY --from=builder /app/bin/model-distribution-tool /usr/local/bin/model-distribution-tool

# Login to Docker Hub using build secrets
RUN --mount=type=secret,id=DOCKER_USERNAME,env=DOCKER_USERNAME \
    --mount=type=secret,id=DOCKER_PASSWORD,env=DOCKER_PASSWORD \
    model-distribution-tool package \
    --licenses /licenses/LICENSE \
    /model/model.gguf \
    $HUB_REPOSITORY:$TAG
