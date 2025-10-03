#!/bin/bash

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${GREEN}Running MongoTron Performance Benchmarks...${NC}"

# Build the project first
echo -e "${YELLOW}Building project...${NC}"
./scripts/build.sh

# Run Go benchmarks
echo -e "${YELLOW}Running Go benchmarks...${NC}"
go test -bench=. -benchmem -benchtime=10s ./tests/performance/...

# Run load tests
echo -e "${YELLOW}Running load tests...${NC}"

# Test concurrent address monitoring
echo -e "${BLUE}Testing concurrent address monitoring...${NC}"
# TODO: Implement load test script

# Test event processing throughput
echo -e "${BLUE}Testing event processing throughput...${NC}"
# TODO: Implement throughput test

# Test webhook delivery rate
echo -e "${BLUE}Testing webhook delivery rate...${NC}"
# TODO: Implement webhook test

# Memory profiling
echo -e "${YELLOW}Generating memory profile...${NC}"
go test -memprofile=mem.prof -bench=. ./tests/performance/...
go tool pprof -text mem.prof > mem_profile.txt

# CPU profiling
echo -e "${YELLOW}Generating CPU profile...${NC}"
go test -cpuprofile=cpu.prof -bench=. ./tests/performance/...
go tool pprof -text cpu.prof > cpu_profile.txt

echo -e "${GREEN}Benchmark completed!${NC}"
echo -e "Results:"
echo -e "  - Memory profile: mem_profile.txt"
echo -e "  - CPU profile: cpu_profile.txt"
