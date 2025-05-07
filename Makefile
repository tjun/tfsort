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

# Version information (attempt to get from Git, fallback for non-Git environments)
GIT_TAG := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

# Go LDFLAGS to inject version information and reduce binary size
LDFLAGS = -s -w \
          -X $(VERSION_PKG).version=$(GIT_TAG) \
          -X $(VERSION_PKG).commit=$(GIT_COMMIT) \
          -X $(VERSION_PKG).date=$(BUILD_DATE)

# Lint parameters
LINT_CMD=golangci-lint
LINT_FLAGS=run

.PHONY: all test build clean lint help release

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

# Run linter
lint:
	@echo "Running linter..."
	$(LINT_CMD) $(LINT_FLAGS) ./...

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
	@echo "  test        Run tests"
	@echo "  build       Build the application binary for the current system"
	@echo "  release     Build release binaries for multiple platforms"
	@echo "  lint        Run linter (golangci-lint)"
	@echo "  clean       Remove build artifacts"
	@echo "  all         Run test and build"
	@echo "  help        Show this help message"
	@echo ""
