# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=tfsort
CMD_PATH=./cmd/$(BINARY_NAME)
OUTPUT_DIR=./bin
VERSION_PKG=main # Package where version variables are defined (e.g., main)

# Installation directory for golangci-lint, defaults to $(OUTPUT_DIR) (./bin)
# To override: make install-lint GOLANGCI_LINT_INSTALL_DIR=/path/to/dir
GOLANGCI_LINT_INSTALL_DIR ?= $(OUTPUT_DIR)
GOLANGCI_LINT = $(GOLANGCI_LINT_INSTALL_DIR)/golangci-lint

# Default version for golangci-lint, can be overridden (e.g., make install-lint GOLANGCI_LINT_VERSION=v1.58.1)
GOLANGCI_LINT_VERSION ?= v2.1.6

# Default install directory for the main binary tfsort
INSTALL_DIR ?= $(shell $(GOCMD) env GOPATH)/bin

# Version information (attempt to get from Git, fallback for non-Git environments)
GIT_TAG := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

# Go LDFLAGS to inject version information and reduce binary size
# Use single quotes around each -X argument
LDFLAGS = -s -w \
          -X '$'(VERSION_PKG).version=$(GIT_TAG)\' \
          -X '$'(VERSION_PKG).commit=$(GIT_COMMIT)\' \
          -X '$'(VERSION_PKG).date=$(BUILD_DATE)\'

# Lint parameters
# LINT_CMD is now defined by the target GOLANGCI_LINT
LINT_FLAGS=run

.PHONY: all test build clean lint help release install-lint install
.DEFAULT_GOAL := help

all: test build

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Build the application binary for the current system
build:
	@echo "Building $(BINARY_NAME) version $(GIT_TAG) (commit $(GIT_COMMIT), built $(BUILD_DATE))..."
	@mkdir -p $(OUTPUT_DIR)
	$(GOBUILD) -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/$(BINARY_NAME) $(CMD_PATH)
	@echo "$(BINARY_NAME) built in $(OUTPUT_DIR)/"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf $(OUTPUT_DIR)

# Run linter using the locally installed golangci-lint
# Depends on the $(GOLANGCI_LINT) file target to ensure it's installed.
lint: $(GOLANGCI_LINT)
	@echo "Running linter..."
	$(GOLANGCI_LINT) $(LINT_FLAGS) ./...

# Rule to install golangci-lint if it doesn't exist or if forced by make install-lint
# Usage: make install-lint [GOLANGCI_LINT_VERSION=vx.y.z]
# If GOLANGCI_LINT_VERSION is not set, it installs the version specified by the script (usually latest stable).
$(GOLANGCI_LINT):
	@echo "Installing golangci-lint to $(GOLANGCI_LINT_INSTALL_DIR)..."
	@echo "(Version: $(or $(GOLANGCI_LINT_VERSION), 'latest stable (script default)'))"
	@mkdir -p $(GOLANGCI_LINT_INSTALL_DIR)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOLANGCI_LINT_INSTALL_DIR) $(GOLANGCI_LINT_VERSION)
	@echo "golangci-lint installed successfully in $(GOLANGCI_LINT_INSTALL_DIR)/."

# Explicit target to force re-installation or install a specific version
install-lint: $(GOLANGCI_LINT)
	@echo "Ensuring golangci-lint version $(or $(GOLANGCI_LINT_VERSION), 'latest') is installed in $(GOLANGCI_LINT_INSTALL_DIR). Run \`make clean\` first to force re-download."
	# The dependency $(GOLANGCI_LINT) handles the actual installation if missing.

# Install the binary to $(INSTALL_DIR) (requires write permission, maybe sudo)
install: build
	@echo "Installing $(BINARY_NAME) to $(INSTALL_DIR)..."
	@mkdir -p $(INSTALL_DIR)
	@cp $(OUTPUT_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "$(BINARY_NAME) installed successfully to $(INSTALL_DIR)/"

# Release builds for common platforms
release: clean # Clean before release build
	@echo "Building release binaries for $(BINARY_NAME) version $(GIT_TAG)..."
	@mkdir -p $(OUTPUT_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/$(BINARY_NAME)_linux_amd64 $(CMD_PATH)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/$(BINARY_NAME)_darwin_amd64 $(CMD_PATH)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/$(BINARY_NAME)_darwin_arm64 $(CMD_PATH)
	GOOS=windows GOARCH=amd64 $(GOBUILD) -ldflags="$(LDFLAGS)" -o $(OUTPUT_DIR)/$(BINARY_NAME)_windows_amd64.exe $(CMD_PATH)
	@echo "Release binaries built in $(OUTPUT_DIR)/"

# Show help
help:
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  test         Run tests"
	@echo "  build        Build the application binary for the current system"
	@echo "  install      Install the application binary to $(INSTALL_DIR)"
	@echo "  release      Build release binaries for multiple platforms"
	@echo "  lint         Run linter (installs golangci-lint to $(GOLANGCI_LINT_INSTALL_DIR) if needed)"
	@echo "  install-lint Install/ensure golangci-lint is installed locally (e.g., make install-lint GOLANGCI_LINT_VERSION=v1.58.1)"
	@echo "  clean        Remove build artifacts (including locally installed tools)"
	@echo "  all          Run test and build"
	@echo "  help         Show this help message"
	@echo ""
