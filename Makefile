# qwen-usage Makefile

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Binary names
BINARY_NAME=qwen-usage
BINARY_WINDOWS=$(BINARY_NAME).exe

# Main package
MAIN_PACKAGE=./cmd/qwen-usage

# Build directory
BUILD_DIR=./bin

# Version
VERSION?=1.0.0
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Linker flags
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.gitCommit=$(GIT_COMMIT)"

.PHONY: all build build-windows build-linux build-mac clean test test-coverage lint run install uninstall help

all: clean build

# Build for current platform
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PACKAGE)

# Build for Windows
build-windows:
	@echo "Building for Windows..."
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_WINDOWS) $(MAIN_PACKAGE)

# Build for Linux (WSL compatible)
build-linux:
	@echo "Building for Linux..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PACKAGE)

# Build for macOS
build-mac:
	@echo "Building for macOS..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PACKAGE)

# Build for all platforms
build-all: build-windows build-linux build-mac

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf $(BUILD_DIR)
	@rm -f $(BINARY_NAME) $(BINARY_WINDOWS)

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run linter
lint:
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found, installing..." && curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin v1.54.2)
	golangci-lint run ./...

# Run the binary
run: build
	@echo "Running..."
	@$(BUILD_DIR)/$(BINARY_NAME) help

# Install to system
install: build
	@echo "Installing..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(shell go env GOPATH)/bin/

# Uninstall from system
uninstall:
	@echo "Uninstalling..."
	@rm -f $(shell go env GOPATH)/bin/$(BINARY_NAME)

# Initialize module
mod-init:
	$(GOMOD) init github.com/panda/qwen-usage

# Download dependencies
mod-download:
	$(GOMOD) download

# Tidy dependencies
mod-tidy:
	$(GOMOD) tidy

# Update dependencies
mod-update:
	$(GOMOD) update

# Format code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...

# Check code
vet:
	@echo "Running go vet..."
	$(GOCMD) vet ./...

# Development setup
dev-setup: mod-download mod-tidy fmt vet test

# Help
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all            Clean and build for current platform"
	@echo "  build          Build for current platform"
	@echo "  build-windows  Build for Windows (amd64)"
	@echo "  build-linux    Build for Linux (amd64) - WSL compatible"
	@echo "  build-mac      Build for macOS (amd64)"
	@echo "  build-all      Build for all platforms"
	@echo "  clean          Clean build artifacts"
	@echo "  test           Run unit tests"
	@echo "  test-coverage  Run tests with coverage report"
	@echo "  lint           Run golangci-lint"
	@echo "  run            Build and run with help command"
	@echo "  install        Install binary to GOPATH/bin"
	@echo "  uninstall      Remove binary from GOPATH/bin"
	@echo "  mod-download   Download module dependencies"
	@echo "  mod-tidy       Tidy module dependencies"
	@echo "  fmt            Format source code"
	@echo "  vet            Run go vet"
	@echo "  dev-setup      Full development setup"
	@echo "  help           Show this help message"