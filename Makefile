.PHONY: all build test clean lint

# Import env file if it exists
-include .env

# Build variables
BINARY_NAME=model-distribution-tool
VERSION?=0.1.0
GOARCH=amd64

# Go related variables
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/bin
GOFILES=$(wildcard *.go)

# Use linker flags to provide version/build information
LDFLAGS=-ldflags "-X main.Version=${VERSION}"

all: clean build test

build:
	@echo "Building ${BINARY_NAME}..."
	@go build ${LDFLAGS} -o ${GOBIN}/${BINARY_NAME} .

test:
	@echo "Running unit tests..."
	@DOCKER_REGISTRY=${DOCKER_REGISTRY} \
	DOCKER_USERNAME=${DOCKER_USERNAME} \
	DOCKER_PASSWORD=${DOCKER_PASSWORD} \
	go test -v ./...

clean:
	@echo "Cleaning..."
	@rm -rf ${GOBIN}
	@rm -f ${BINARY_NAME}
	@rm -f *.test
	@rm -rf test/artifacts/*

lint:
	@echo "Running linter..."
	@golangci-lint run
# Cross compilation targets
build-linux:
	@echo "Building for Linux..."
	@GOOS=linux GOARCH=${GOARCH} go build ${LDFLAGS} -o ${GOBIN}/${BINARY_NAME}-linux-${GOARCH} .

build-darwin:
	@echo "Building for macOS..."
	@GOOS=darwin GOARCH=${GOARCH} go build ${LDFLAGS} -o ${GOBIN}/${BINARY_NAME}-darwin-${GOARCH} .

build-windows:
	@echo "Building for Windows..."
	@GOOS=windows GOARCH=${GOARCH} go build ${LDFLAGS} -o ${GOBIN}/${BINARY_NAME}-windows-${GOARCH}.exe .

build-all: build-linux build-darwin build-windows

# Help target
help:
	@echo "Available targets:"
	@echo "  all              - Clean, build, and test"
	@echo "  build           - Build the binary"
	@echo "  test            - Run tests"
	@echo "  clean           - Clean build artifacts"
	@echo "  build-linux     - Build for Linux"
	@echo "  build-darwin    - Build for macOS"
	@echo "  build-windows   - Build for Windows"
	@echo "  build-all       - Build for all platforms" 