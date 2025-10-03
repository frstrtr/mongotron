#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Running MongoTron tests...${NC}"

# Run unit tests
echo -e "${YELLOW}Running unit tests...${NC}"
go test -v -race -coverprofile=coverage-unit.txt -covermode=atomic ./tests/unit/...

# Run integration tests
echo -e "${YELLOW}Running integration tests...${NC}"
go test -v -race -coverprofile=coverage-integration.txt -covermode=atomic ./tests/integration/...

# Run all package tests
echo -e "${YELLOW}Running package tests...${NC}"
go test -v -race -coverprofile=coverage-all.txt -covermode=atomic ./...

# Generate coverage report
echo -e "${YELLOW}Generating coverage report...${NC}"
go tool cover -html=coverage-all.txt -o coverage.html

# Calculate total coverage
COVERAGE=$(go tool cover -func=coverage-all.txt | grep total | awk '{print $3}')
echo -e "${GREEN}Total test coverage: ${COVERAGE}${NC}"

# Check if coverage meets threshold (90%)
THRESHOLD=90.0
COVERAGE_NUM=$(echo ${COVERAGE} | sed 's/%//')
if (( $(echo "$COVERAGE_NUM < $THRESHOLD" | bc -l) )); then
    echo -e "${RED}Coverage ${COVERAGE_NUM}% is below threshold ${THRESHOLD}%${NC}"
    exit 1
fi

echo -e "${GREEN}All tests passed!${NC}"
