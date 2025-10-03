.PHONY: all build test clean docker run help

# Variables
PROJECT_NAME := mongotron
VERSION := 1.0.0
BUILD_DIR := ./build
BIN_DIR := $(BUILD_DIR)/bin
DOCKER_IMAGE := $(PROJECT_NAME):$(VERSION)

# Default target
all: clean build test

# Help target
help:
	@echo "MongoTron Makefile Commands:"
	@echo "  make build         - Build all binaries"
	@echo "  make test          - Run all tests"
	@echo "  make test-unit     - Run unit tests only"
	@echo "  make test-integration - Run integration tests only"
	@echo "  make benchmark     - Run performance benchmarks"
	@echo "  make lint          - Run linters"
	@echo "  make format        - Format code"
	@echo "  make docker-build  - Build Docker image"
	@echo "  make docker-run    - Run Docker container"
	@echo "  make docker-push   - Push Docker image"
	@echo "  make deploy-dev    - Deploy to development"
	@echo "  make deploy-prod   - Deploy to production"
	@echo "  make clean         - Clean build artifacts"
	@echo "  make run           - Run the application locally"

# Build targets
build:
	@echo "Building $(PROJECT_NAME)..."
	@./scripts/build.sh

# Test targets
test:
	@echo "Running tests..."
	@./scripts/test.sh

test-unit:
	@echo "Running unit tests..."
	@go test -v -race ./tests/unit/...

test-integration:
	@echo "Running integration tests..."
	@go test -v -race ./tests/integration/...

test-e2e:
	@echo "Running end-to-end tests..."
	@go test -v -race ./tests/e2e/...

# Benchmark target
benchmark:
	@echo "Running benchmarks..."
	@./scripts/benchmark.sh

# Code quality targets
lint:
	@echo "Running linters..."
	@golangci-lint run ./...

format:
	@echo "Formatting code..."
	@gofmt -s -w .
	@goimports -w .

# Docker targets
docker-build:
	@echo "Building Docker image..."
	@docker build -t $(DOCKER_IMAGE) -f deployments/docker/Dockerfile .

docker-run:
	@echo "Running Docker container..."
	@docker-compose -f deployments/docker/docker-compose.yml up

docker-run-prod:
	@echo "Running production Docker containers..."
	@docker-compose -f deployments/docker/docker-compose.prod.yml up -d

docker-push:
	@echo "Pushing Docker image..."
	@docker push $(DOCKER_IMAGE)

docker-stop:
	@echo "Stopping Docker containers..."
	@docker-compose -f deployments/docker/docker-compose.yml down

# Deployment targets
deploy-dev:
	@echo "Deploying to development..."
	@./scripts/deploy.sh development docker

deploy-prod:
	@echo "Deploying to production..."
	@./scripts/deploy.sh production kubernetes

# Run target
run:
	@echo "Running $(PROJECT_NAME) locally..."
	@go run ./cmd/mongotron/main.go

# Database migration targets
migrate-up:
	@echo "Running database migrations..."
	@go run ./cmd/migrate/main.go -d up

migrate-down:
	@echo "Rolling back database migrations..."
	@go run ./cmd/migrate/main.go -d down

# Clean target
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage*.txt coverage.html
	@rm -f *.prof *_profile.txt

# Dependencies
deps:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy

# Generate targets
generate:
	@echo "Running go generate..."
	@go generate ./...

# Protocol buffer generation
proto:
	@echo "Generating protobuf files..."
	@protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		api/proto/*.proto
