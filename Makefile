.PHONY: help build test test-unit test-integration test-coverage test-bench clean run-api run-cli

# Variables
PROJECT_NAME := mongotron
VERSION := 1.0.0

# Default target
help:
	@echo "MongoTron - Makefile Commands"
	@echo ""
	@echo "Build Commands:"
	@echo "  make build-api        - Build API server binary"
	@echo "  make build-cli        - Build CLI binary"
	@echo "  make build-all        - Build both binaries"
	@echo ""
	@echo "Test Commands:"
	@echo "  make test             - Run all unit tests"
	@echo "  make test-unit        - Run unit tests"
	@echo "  make test-integration - Run integration tests (requires API server)"
	@echo "  make test-coverage    - Run tests with coverage report"
	@echo "  make test-bench       - Run benchmark tests"
	@echo "  make test-verbose     - Run tests with verbose output"
	@echo ""
	@echo "Run Commands:"
	@echo "  make run-api          - Run API server"
	@echo "  make run-cli          - Run CLI monitor"
	@echo ""
	@echo "Utility Commands:"
	@echo "  make clean            - Remove build artifacts"
	@echo "  make deps             - Install dependencies"
	@echo "  make fmt              - Format code"
	@echo "  make lint             - Run linter"
	@echo ""

# Build targets
build-api:
	@echo "Building API server..."
	@go build -o bin/mongotron-api cmd/api-server/main.go
	@echo "✅ API server built: bin/mongotron-api"

build-cli:
	@echo "Building CLI..."
	@go build -o bin/mongotron-mvp cmd/mvp/main.go
	@echo "✅ CLI built: bin/mongotron-mvp"

build-all: build-api build-cli
	@echo "✅ All binaries built"

# Test targets
test: test-unit
	@echo "✅ All tests passed"

test-unit:
	@echo "Running unit tests..."
	@./run_tests.sh unit

test-integration:
	@echo "Running integration tests..."
	@./run_tests.sh integration

test-coverage:
	@echo "Running tests with coverage..."
	@./run_tests.sh coverage

test-bench:
	@echo "Running benchmark tests..."
	@./run_tests.sh bench

test-verbose:
	@echo "Running tests (verbose)..."
	@./run_tests.sh unit -v

# Run targets
run-api: build-api
	@echo "Starting API server..."
	@./bin/mongotron-api

run-cli: build-cli
	@echo "Starting CLI monitor..."
	@./bin/mongotron-mvp --address TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf

# Utility targets
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/*
	@rm -f coverage.out coverage.html
	@echo "✅ Clean complete"

deps:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy
	@echo "✅ Dependencies installed"

fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@echo "✅ Code formatted"

lint:
	@echo "Running linter..."
	@golangci-lint run ./... || echo "⚠️  Install golangci-lint: https://golangci-lint.run/usage/install/"

# Development targets
dev-setup: deps
	@echo "Setting up development environment..."
	@chmod +x run_tests.sh
	@chmod +x test_api.sh
	@chmod +x test_websocket.js
	@echo "✅ Development environment ready"

# Quick test-and-build workflow
quick: clean build-all test
	@echo "✅ Quick build and test complete"
