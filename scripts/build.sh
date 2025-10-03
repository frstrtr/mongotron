#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Project info
PROJECT_NAME="mongotron"
VERSION=${VERSION:-"1.0.0"}
BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Build directories
BUILD_DIR="./build"
BIN_DIR="${BUILD_DIR}/bin"

echo -e "${GREEN}Building ${PROJECT_NAME}...${NC}"

# Create build directories
mkdir -p ${BIN_DIR}

# Build flags
LDFLAGS="-w -s \
  -X main.version=${VERSION} \
  -X main.buildTime=${BUILD_TIME} \
  -X main.gitCommit=${GIT_COMMIT}"

# Build main application
echo -e "${YELLOW}Building main application...${NC}"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="${LDFLAGS}" \
  -o ${BIN_DIR}/${PROJECT_NAME} \
  ./cmd/mongotron

# Build CLI tool
echo -e "${YELLOW}Building CLI tool...${NC}"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="${LDFLAGS}" \
  -o ${BIN_DIR}/${PROJECT_NAME}-cli \
  ./cmd/cli

# Build migration tool
echo -e "${YELLOW}Building migration tool...${NC}"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="${LDFLAGS}" \
  -o ${BIN_DIR}/${PROJECT_NAME}-migrate \
  ./cmd/migrate

echo -e "${GREEN}Build completed successfully!${NC}"
echo -e "Binaries are located in: ${BIN_DIR}"
ls -lh ${BIN_DIR}
