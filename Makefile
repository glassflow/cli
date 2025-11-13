# GlassFlow CLI Makefile

# Variables
BINARY_NAME=glassflow
BUILD_DIR=build
GO_VERSION=1.21
LDFLAGS=-ldflags "-s -w"

# Default target
.PHONY: all
all: build

# Build the CLI binary
.PHONY: build
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "Binary built: $(BUILD_DIR)/$(BINARY_NAME)"

# Build for multiple platforms
.PHONY: build-all
build-all:
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .
	@GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .
	@GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .
	@GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe .
	@echo "Binaries built in $(BUILD_DIR)/"

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@go clean

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	@go test -v ./...

# Run linting
.PHONY: lint
lint:
	@echo "Running linter..."
	@golangci-lint run

# Format code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Install dependencies
.PHONY: deps
deps:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy

# Run the CLI (for testing)
# Usage: make run ARGS="up --demo" or make run ARGS="down"
.PHONY: run
run: build
	@echo "Running $(BINARY_NAME) $(ARGS)..."
	@./$(BUILD_DIR)/$(BINARY_NAME) up --demo --verbose

# Install GoReleaser
.PHONY: goreleaser-install
goreleaser-install:
	@echo "Installing GoReleaser..."
	@go install github.com/goreleaser/goreleaser@latest

# Run GoReleaser (for local testing with --snapshot)
.PHONY: goreleaser-release
goreleaser-release:
	@echo "Running GoReleaser..."
	@goreleaser release --clean

# Run GoReleaser snapshot (for local testing)
.PHONY: goreleaser-snapshot
goreleaser-snapshot:
	@echo "Running GoReleaser snapshot..."
	@goreleaser release --snapshot --clean

# Show help
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build              - Build the CLI binary"
	@echo "  build-all          - Build for multiple platforms"
	@echo "  clean              - Clean build artifacts"
	@echo "  test               - Run tests"
	@echo "  lint               - Run linter"
	@echo "  fmt                - Format code"
	@echo "  deps               - Install dependencies"
	@echo "  run                - Build and run the CLI (use ARGS='command' to pass arguments)"
	@echo "  goreleaser-install - Install GoReleaser"
	@echo "  goreleaser-release - Run GoReleaser release (requires GITHUB_TOKEN)"
	@echo "  goreleaser-snapshot - Run GoReleaser snapshot (for local testing)"
	@echo "  help               - Show this help"
