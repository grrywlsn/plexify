# Plexify Makefile
# Build and development tools for the Plexify application

.PHONY: help build run test clean deps format vet setup build-release build-all-platforms

# Default target
.DEFAULT_GOAL := help

# Variables
BINARY_NAME := plexify
BUILD_DIR := bin
DIST_DIR := dist
MAIN_FILE := main.go
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"

# Help target
help: ## Show this help message
	@echo "Plexify - Spotify to Plex Playlist Sync Tool"
	@echo ""
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Examples:"
	@echo "  make build          # Build the application"
	@echo "  make run            # Run the application"
	@echo "  make test           # Run tests"
	@echo "  make setup          # Setup development environment"
	@echo "  make build-release  # Build for all platforms"

# Build targets
build: ## Build the application for current platform
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_FILE)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

build-release: build-all-platforms ## Build the application with release optimizations for all platforms
	@echo "Release builds complete in $(DIST_DIR)/"

build-all-platforms: ## Build for all supported platforms
	@echo "Building $(BINARY_NAME) for all platforms..."
	@mkdir -p $(DIST_DIR)
	
	# Linux builds
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_FILE)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_FILE)
	
	# macOS builds
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_FILE)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_FILE)
	
	# Windows builds
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_FILE)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-windows-arm64.exe $(MAIN_FILE)
	
	# Create checksums
	@echo "Creating checksums..."
	@cd $(DIST_DIR) && sha256sum $(BINARY_NAME)-* > checksums.txt
	@echo "Build complete: $(DIST_DIR)/"

# Run targets
run: build ## Run the application
	./$(BUILD_DIR)/$(BINARY_NAME)

run-playlist: build ## Run with a specific playlist ID (set PLAYLIST_ID=your_playlist_id)
	@if [ -z "$(PLAYLIST_ID)" ]; then \
		echo "Error: PLAYLIST_ID not set. Usage: make run-playlist PLAYLIST_ID=your_playlist_id"; \
		exit 1; \
	fi
	@echo "Running $(BINARY_NAME) with playlist ID: $(PLAYLIST_ID)"
	./$(BUILD_DIR)/$(BINARY_NAME) -playlists $(PLAYLIST_ID)

run-username: build ## Run with a specific username (set USERNAME=your_username)
	@if [ -z "$(USERNAME)" ]; then \
		echo "Error: USERNAME not set. Usage: make run-username USERNAME=your_username"; \
		exit 1; \
	fi
	@echo "Running $(BINARY_NAME) with username: $(USERNAME)"
	./$(BUILD_DIR)/$(BINARY_NAME) -username $(USERNAME)

# Development targets
deps: ## Install and tidy dependencies
	@echo "Installing dependencies..."
	go mod download
	go mod tidy
	@echo "Dependencies updated"

test: ## Run tests
	@echo "Running tests..."
	go test ./...

test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

test-verbose: ## Run tests with verbose output
	@echo "Running tests with verbose output..."
	go test -v ./...

# Code quality targets
format: ## Format the code
	@echo "Formatting code..."
	go fmt ./...

vet: ## Vet the code
	@echo "Vetting code..."
	go vet ./...

# Setup targets
setup: deps ## Setup development environment
	@echo "Development environment setup complete!"
	@echo ""
	@echo "Next steps:"
	@echo "1. Copy env.template to .env"
	@echo "2. Fill in your configuration values in .env"
	@echo "3. Run 'make build' to build the application"
	@echo "4. Run 'make test' to run tests"

# Cleanup targets
clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)/
	rm -rf $(DIST_DIR)/
	rm -f coverage.out coverage.html
	@echo "Clean complete"

clean-deps: ## Clean and reinstall dependencies
	@echo "Cleaning and reinstalling dependencies..."
	rm -f go.sum
	go clean -modcache
	make deps

# Utility targets
version: ## Show version information
	@echo "Version: $(VERSION)"
	@echo "Go version:"
	go version
	@echo ""
	@echo "Module information:"
	go list -m
	@echo ""
	@echo "Dependencies:"
	go list -m all

check: format vet test ## Run all code quality checks
	@echo "All checks passed!"

# Development workflow
dev: check build ## Run full development workflow (format, vet, test, build)
	@echo "Development workflow complete!"

# Documentation
docs: ## Generate documentation
	@echo "Generating documentation..."
	@if command -v godoc >/dev/null 2>&1; then \
		echo "Starting godoc server on http://localhost:6060"; \
		godoc -http=:6060; \
	else \
		echo "godoc not found. Install with: go install golang.org/x/tools/cmd/godoc@latest"; \
	fi 