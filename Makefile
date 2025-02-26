.PHONY: all build test clean lint run

# Import env file if it exists
-include .env

# Build variables
BINARY_NAME=model-distribution-tool
VERSION?=0.1.0

# Go related variables
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/bin

# Run configuration
SOURCE?=
TAG?=

# Use linker flags to provide version/build information
LDFLAGS=-ldflags "-X main.Version=${VERSION}"

all: clean lint build test

build:
	@echo "Building ${BINARY_NAME}..."
	@go build ${LDFLAGS} -o ${GOBIN}/${BINARY_NAME} .

test:
	@echo "Running unit tests..."
	@go test -v ./...

clean:
	@echo "Cleaning..."
	@rm -rf ${GOBIN}
	@rm -f ${BINARY_NAME}
	@rm -f *.test
	@rm -rf test/artifacts/*

lint:
	@echo "Running linters..."
	@gofmt -s -l . | tee /dev/stderr | xargs -r false
	@go vet ./...

run: build
	@if [ -z "$(SOURCE)" ] || [ -z "$(TAG)" ]; then \
		echo "Error: SOURCE and TAG must be provided"; \
		echo "Usage: make run SOURCE=<path-or-url> TAG=<registry/repo:tag>"; \
		exit 1; \
	fi
	@echo "Running ${BINARY_NAME}..."
	${GOBIN}/${BINARY_NAME} --source "$(SOURCE)" --tag "$(TAG)"

help:
	@echo "Available targets:"
	@echo "  all              - Clean, build, and test"
	@echo "  build           - Build the binary"
	@echo "  test            - Run tests"
	@echo "  clean           - Clean build artifacts"
	@echo "  run             - Build and run the tool (requires SOURCE and TAG)"
	@echo ""
	@echo "Run example:"
	@echo "  make run SOURCE=path/to/model.gguf TAG=registry.example.com/model:latest"