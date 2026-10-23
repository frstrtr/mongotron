#!/bin/bash
# MongoTron API Test Runner

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}================================${NC}"
echo -e "${BLUE}MongoTron API Test Suite${NC}"
echo -e "${BLUE}================================${NC}"
echo ""

# Parse command line arguments
TEST_TYPE="${1:-unit}"
VERBOSE="${2:-}"

case "$TEST_TYPE" in
    unit)
        echo -e "${YELLOW}Running Unit Tests...${NC}"
        echo ""
        if [ "$VERBOSE" == "-v" ]; then
            go test -v ./internal/api/handlers/...
        else
            go test ./internal/api/handlers/...
        fi
        ;;
    
    integration)
        echo -e "${YELLOW}Running Integration Tests...${NC}"
        echo ""
        echo -e "${YELLOW}⚠️  Note: API server must be running on localhost:8080${NC}"
        echo ""
        read -p "Is the API server running? (y/n) " -n 1 -r
        echo ""
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            echo -e "${RED}Please start the API server first:${NC}"
            echo -e "${BLUE}  ./bin/mongotron-api${NC}"
            exit 1
        fi
        
        if [ "$VERBOSE" == "-v" ]; then
            go test -v ./test/integration/...
        else
            go test ./test/integration/...
        fi
        ;;
    
    all)
        echo -e "${YELLOW}Running All Tests...${NC}"
        echo ""
        
        # Run unit tests
        echo -e "${BLUE}1. Unit Tests${NC}"
        if [ "$VERBOSE" == "-v" ]; then
            go test -v ./internal/api/handlers/...
        else
            go test ./internal/api/handlers/...
        fi
        
        echo ""
        echo -e "${BLUE}2. Integration Tests${NC}"
        echo -e "${YELLOW}⚠️  Skipping integration tests (use 'make test-integration' to run)${NC}"
        ;;
    
    coverage)
        echo -e "${YELLOW}Running Tests with Coverage...${NC}"
        echo ""
        go test -coverprofile=coverage.out ./internal/api/handlers/...
        go tool cover -html=coverage.out -o coverage.html
        echo ""
        echo -e "${GREEN}Coverage report generated: coverage.html${NC}"
        ;;
    
    bench)
        echo -e "${YELLOW}Running Benchmark Tests...${NC}"
        echo ""
        go test -bench=. -benchmem ./internal/api/handlers/...
        ;;
    
    *)
        echo -e "${RED}Unknown test type: $TEST_TYPE${NC}"
        echo ""
        echo "Usage: $0 [unit|integration|all|coverage|bench] [-v]"
        echo ""
        echo "Test Types:"
        echo "  unit        - Run unit tests only (default)"
        echo "  integration - Run integration tests (requires running API server)"
        echo "  all         - Run all unit tests"
        echo "  coverage    - Run tests with coverage report"
        echo "  bench       - Run benchmark tests"
        echo ""
        echo "Options:"
        echo "  -v          - Verbose output"
        echo ""
        echo "Examples:"
        echo "  $0 unit"
        echo "  $0 unit -v"
        echo "  $0 integration"
        echo "  $0 coverage"
        exit 1
        ;;
esac

echo ""
if [ $? -eq 0 ]; then
    echo -e "${GREEN}================================${NC}"
    echo -e "${GREEN}✅ All tests passed!${NC}"
    echo -e "${GREEN}================================${NC}"
else
    echo -e "${RED}================================${NC}"
    echo -e "${RED}❌ Some tests failed${NC}"
    echo -e "${RED}================================${NC}"
    exit 1
fi
