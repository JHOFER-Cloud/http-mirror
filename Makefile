# HTTP Mirror Makefile

.PHONY: help build test lint docker docker-server docker-updater helm-lint clean run-server run-updater

# Default target
help:
	@echo "HTTP Mirror - Available targets:"
	@echo "  build         - Build both server and updater binaries"
	@echo "  test          - Run all tests with coverage"
	@echo "  lint          - Run golangci-lint"
	@echo "  docker        - Build both Docker images"
	@echo "  docker-server - Build server Docker image"
	@echo "  docker-updater- Build updater Docker image"  
	@echo "  helm-lint     - Lint Helm chart"
	@echo "  run-server    - Run server locally"
	@echo "  run-updater   - Run updater locally"
	@echo "  clean         - Clean build artifacts"

# Build targets
build: build-server build-updater

build-server:
	@echo "Building server..."
	@go build -o bin/server ./cmd/server

build-updater:
	@echo "Building updater..."
	@go build -o bin/updater ./cmd/updater

# Test targets
test:
	@echo "Running tests..."
	@go test -v -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

test-short:
	@go test -short ./...

# Lint targets
lint:
	@echo "Running golangci-lint..."
	@golangci-lint run

fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@goimports -w .

# Docker targets
docker: docker-server docker-updater

docker-server:
	@echo "Building server Docker image..."
	@docker build -f Dockerfile.server -t http-mirror-server:latest .

docker-updater:
	@echo "Building updater Docker image..."
	@docker build -f Dockerfile.updater -t http-mirror-updater:latest .

# Helm targets
helm-lint:
	@echo "Linting Helm chart..."
	@helm lint charts/http-mirror

# Run targets
run-server: build-server
	@echo "Starting server on :8080..."
	@./bin/server

run-updater: build-updater
	@echo "Running updater..."
	@./bin/updater

# Utility targets
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/ coverage.out coverage.html .go/

# Development helpers
dev-setup:
	@echo "Setting up development environment..."
	@go mod download
	@go install golang.org/x/tools/cmd/goimports@latest

# Generate test data
test-data:
	@echo "Generating test data..."
	@mkdir -p testdata
	@echo "Creating mock HTTP server files..."

# Check dependencies
check-deps:
	@echo "Checking for required dependencies..."
	@command -v go >/dev/null 2>&1 || { echo "Go is required but not installed"; exit 1; }
	@command -v docker >/dev/null 2>&1 || { echo "Docker is required but not installed"; exit 1; }
	@echo "All dependencies found"
