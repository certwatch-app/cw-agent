# CertWatch Agent Makefile

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -ldflags "-X github.com/certwatch-app/cw-agent/internal/version.Version=$(VERSION) \
                     -X github.com/certwatch-app/cw-agent/internal/version.GitCommit=$(GIT_COMMIT) \
                     -X github.com/certwatch-app/cw-agent/internal/version.BuildDate=$(BUILD_DATE)"

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet

# Binary names
BINARY_NAME=cw-agent
BINARY_DIR=bin

# Default target
.DEFAULT_GOAL := build

.PHONY: all build clean test lint fmt vet deps tidy help

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME) ./cmd/cw-agent

## build-all: Build for all platforms
build-all: build-linux build-darwin build-windows

## build-linux: Build for Linux (amd64 and arm64)
build-linux:
	@echo "Building for Linux..."
	@mkdir -p $(BINARY_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/cw-agent
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/cw-agent

## build-darwin: Build for macOS (amd64 and arm64)
build-darwin:
	@echo "Building for macOS..."
	@mkdir -p $(BINARY_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/cw-agent
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/cw-agent

## build-windows: Build for Windows (amd64)
build-windows:
	@echo "Building for Windows..."
	@mkdir -p $(BINARY_DIR)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/cw-agent

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BINARY_DIR)
	@rm -f coverage.out

## test: Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v -race ./...

## test-coverage: Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

## lint: Run linter
lint:
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found, install from https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run ./...

## fmt: Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) ./...

## vet: Run go vet
vet:
	@echo "Running go vet..."
	$(GOVET) ./...

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download

## tidy: Tidy go.mod
tidy:
	@echo "Tidying go.mod..."
	$(GOMOD) tidy

## run: Run the agent with example config
run: build
	@echo "Running agent..."
	./$(BINARY_DIR)/$(BINARY_NAME) start -c certwatch.yaml

## validate: Validate config file
validate: build
	./$(BINARY_DIR)/$(BINARY_NAME) validate -c certwatch.yaml

## version: Show version
version: build
	./$(BINARY_DIR)/$(BINARY_NAME) version

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t certwatch-app/cw-agent:$(VERSION) .
	docker tag certwatch-app/cw-agent:$(VERSION) certwatch-app/cw-agent:latest

## help: Show this help
help:
	@echo "CertWatch Agent - Available targets:"
	@echo ""
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'
