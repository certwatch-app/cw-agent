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
BINARY_CERTMANAGER=cw-agent-certmanager
BINARY_DIR=bin

# Default target
.DEFAULT_GOAL := build

.PHONY: all build build-certmanager clean test lint fmt vet deps tidy help

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

# ============================================================================
# cert-manager Controller Targets
# ============================================================================

## build-certmanager: Build the cert-manager controller
build-certmanager:
	@echo "Building $(BINARY_CERTMANAGER)..."
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_CERTMANAGER) ./cmd/cw-agent-certmanager

## build-certmanager-linux: Build cert-manager controller for Linux
build-certmanager-linux:
	@echo "Building $(BINARY_CERTMANAGER) for Linux..."
	@mkdir -p $(BINARY_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_CERTMANAGER)-linux-amd64 ./cmd/cw-agent-certmanager
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_CERTMANAGER)-linux-arm64 ./cmd/cw-agent-certmanager

## docker-build-certmanager: Build Docker image for cert-manager controller
docker-build-certmanager:
	@echo "Building Docker image for cert-manager controller..."
	docker build -f Dockerfile.certmanager \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(GIT_COMMIT) \
		--build-arg DATE=$(BUILD_DATE) \
		-t certwatch-app/cw-agent-certmanager:$(VERSION) .
	docker tag certwatch-app/cw-agent-certmanager:$(VERSION) certwatch-app/cw-agent-certmanager:latest

## test-certmanager: Run cert-manager controller tests
test-certmanager:
	@echo "Running cert-manager controller tests..."
	$(GOTEST) -v -race ./internal/certmanager/...

## run-certmanager: Run the cert-manager controller with test config
run-certmanager: build-certmanager
	@echo "Running cert-manager controller..."
	./$(BINARY_DIR)/$(BINARY_CERTMANAGER) -c testdata/certwatch-certmanager.yaml

## validate-certmanager: Validate cert-manager controller config
validate-certmanager: build-certmanager
	./$(BINARY_DIR)/$(BINARY_CERTMANAGER) validate -c testdata/certwatch-certmanager.yaml

# ============================================================================
# CA Validation Testing Targets (Phase 2)
# ============================================================================

## test-ca: Run CA validation tests with race detection
test-ca:
	@echo "Running CA validation tests..."
	$(GOTEST) -v -race ./internal/ca/...

## test-ca-coverage: Run CA validation tests with coverage report
test-ca-coverage:
	@echo "Running CA validation tests with coverage..."
	$(GOTEST) -v -race -coverprofile=coverage-ca.out ./internal/ca/...
	@echo ""
	@echo "Coverage summary:"
	$(GOCMD) tool cover -func=coverage-ca.out | grep total
	@echo ""
	@echo "Generating HTML coverage report..."
	$(GOCMD) tool cover -html=coverage-ca.out -o coverage-ca.html
	@echo "Coverage report saved to coverage-ca.html"

## test-ca-verbose: Run CA validation tests with verbose output
test-ca-verbose:
	@echo "Running CA validation tests (verbose)..."
	$(GOTEST) -v -race -count=1 ./internal/ca/...

# ============================================================================
# CA Validation Integration Testing Targets (Phase 1)
# ============================================================================

## test-integration: Run CA validation integration tests
test-integration: build
	@echo "Running CA validation integration tests..."
	$(GOTEST) -v -timeout 5m ./internal/ca/integration/...

## test-integration-race: Run integration tests with race detector
test-integration-race: build
	@echo "Running integration tests with race detector..."
	$(GOTEST) -v -race -timeout 5m ./internal/ca/integration/...

## test-integration-coverage: Run integration tests with coverage
test-integration-coverage: build
	@echo "Running integration tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage-integration.out ./internal/ca/integration
	@echo ""
	@echo "Coverage summary:"
	$(GOCMD) tool cover -func=coverage-integration.out | grep total
	@echo ""
	@echo "Generating HTML coverage report..."
	$(GOCMD) tool cover -html=coverage-integration.out -o coverage-integration.html
	@echo "Coverage report saved to coverage-integration.html"

## demo-custom-ca: Run demo 1 - Custom CA for internal services
demo-custom-ca:
	@./internal/ca/integration/demo_scripts/demo1_custom_ca.sh

## demo-mitm: Run demo 2 - MITM detection
demo-mitm:
	@./internal/ca/integration/demo_scripts/demo2_mitm_detection.sh

## demo-hotreload: Run demo 3 - CA bundle hot-reload
demo-hotreload:
	@./internal/ca/integration/demo_scripts/demo3_hotreload.sh

## demos: Run all integration test demos
demos: demo-custom-ca demo-mitm demo-hotreload

# ============================================================================
# CA Validation Kubernetes Integration Testing Targets (Phase 2)
# ============================================================================

## setup-envtest: Download EnvTest binaries
setup-envtest:
	@echo "Setting up EnvTest binaries..."
	@mkdir -p testdata/envtest
	@go run sigs.k8s.io/controller-runtime/tools/setup-envtest use --bin-dir testdata/envtest

## test-integration-k8s: Run Kubernetes integration tests (EnvTest)
test-integration-k8s: build
	@echo "Running Kubernetes integration tests with EnvTest..."
	$(GOTEST) -v -timeout 5m -run TestIntegration_K8s ./internal/ca/integration/

## test-integration-k8s-race: Run K8s integration tests with race detector
test-integration-k8s-race: build
	@echo "Running K8s integration tests with race detector..."
	$(GOTEST) -v -race -timeout 5m -run TestIntegration_K8s ./internal/ca/integration/

## test-integration-all: Run all integration tests (Phase 1 + Phase 2)
test-integration-all: build
	@echo "Running all integration tests (Phase 1 + Phase 2)..."
	$(GOTEST) -v -timeout 10m ./internal/ca/integration/

## test-integration-all-race: Run all integration tests with race detector
test-integration-all-race: build
	@echo "Running all integration tests with race detector..."
	$(GOTEST) -v -race -timeout 10m ./internal/ca/integration/

## help: Show this help
help:
	@echo "CertWatch Agent - Available targets:"
	@echo ""
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'
