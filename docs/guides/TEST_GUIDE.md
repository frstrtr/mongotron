# MongoTron API Testing Guide

## Overview

MongoTron API includes a comprehensive test suite covering unit tests, integration tests, and performance benchmarks. This guide explains how to run and write tests for the API server.

## Test Structure

```
mongotron/
├── internal/api/handlers/
│   ├── subscription_test.go    # Unit tests for subscription handlers
│   ├── health_test.go          # Unit tests for health endpoints
│   ├── subscription.go
│   └── health.go
├── test/integration/
│   └── api_integration_test.go # Integration tests
├── run_tests.sh                # Test runner script
└── Makefile                    # Build and test automation
```

## Running Tests

### Using Makefile (Recommended)

```bash
# Run all unit tests
make test

# Run unit tests with verbose output
make test-verbose

# Run integration tests (requires running API server)
make test-integration

# Generate coverage report
make test-coverage

# Run benchmark tests
make test-bench

# Quick build and test
make quick
```

### Using Test Runner Script

```bash
# Run unit tests
./run_tests.sh unit

# Run unit tests (verbose)
./run_tests.sh unit -v

# Run integration tests
./run_tests.sh integration

# Generate coverage report
./run_tests.sh coverage

# Run benchmarks
./run_tests.sh bench
```

### Using Go Test Directly

```bash
# Run all tests
go test ./...

# Run specific package tests
go test ./internal/api/handlers/...

# Run with coverage
go test -cover ./...

# Run with race detection
go test -race ./...

# Run specific test
go test -run TestCreateSubscription_Success ./internal/api/handlers/...
```

## Unit Tests

Unit tests use mocks to isolate handler logic and test individual endpoints without external dependencies.

### Unit Tests (38 tests)

**Event Handler Tests** (16 tests) - NEW!:
- ✅ NewEventHandler - Constructor validation
- ✅ ListEvents (success, pagination, validation, by address, errors)
- ✅ GetEvent (success, not found, missing ID)
- ✅ GetEventByTransactionHash (success, multiple events, not found, errors)
- ✅ toEventResponse - Response transformation

**Health Check Tests** (4 tests):
- ✅ Health endpoint validation
- ✅ Readiness probe (K8s)
- ✅ Liveness probe (K8s)  
- ✅ Service unavailable handling

**Subscription Handler Tests** (14 tests):
- ✅ Create subscription (success, validation, filters, database errors)
- ✅ Get subscription (success, not found, invalid ID)
- ✅ List subscriptions (default, custom pagination, manager errors)
- ✅ Delete subscription (success, error handling)
- ✅ toSubscriptionResponse (with/without LastEventAt)

**WebSocket Handler Tests** (4 tests) - NEW!:
- ✅ NewWebSocketHandler - Constructor validation
- ✅ WebSocket middleware (non-WebSocket request, upgrade header, type validation)
- ⚠️ StreamEvents - Requires integration tests with real WebSocket connection

### Mock Objects

The test suite uses `testify/mock` to create mock implementations:

```go
type MockSubscriptionManager struct {
    mock.Mock
}

func (m *MockSubscriptionManager) Subscribe(address string, webhookURL string, 
    filters models.SubscriptionFilters, startBlock int64) (*models.Subscription, error) {
    args := m.Called(address, webhookURL, filters, startBlock)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*models.Subscription), args.Error(1)
}
```

## Integration Tests

Integration tests require a running API server and test the complete request/response cycle.

### Prerequisites

1. Start MongoDB: `mongod` or ensure MongoDB is running
2. Start Tron node or connect to testnet
3. Start API server: `./bin/mongotron-api` or `make run-api`

### Running Integration Tests

```bash
# Using Makefile
make test-integration

# Using test runner
./run_tests.sh integration

# Using Go directly
go test ./test/integration/... -v
```

### Skip Integration Tests

Use the `-short` flag to skip integration tests:

```bash
go test -short ./...
```

### Integration Test Suites (5 suites)

Located in `test/integration/api_integration_test.go`:

1. **TestAPIIntegration_FullFlow** - Complete lifecycle test:
   - Health check
   - Create subscription
   - Get subscription
   - List subscriptions
   - Wait for events
   - List events
   - Delete subscription
   - Verify deletion

2. **TestAPIIntegration_ErrorHandling** - Error scenarios:
   - Missing address (400)
   - Subscription not found (404)
   - Delete non-existent subscription

3. **TestAPIIntegration_RateLimiting** - Tests rate limit (100 req/min):
   - Makes 150 requests
   - Verifies rate limit kicks in

4. **TestAPIIntegration_Pagination** - Tests pagination:
   - Creates 5 subscriptions
   - Tests limit and skip parameters
   - Verifies correct pagination

5. **TestAPIIntegration_Concurrent** - Concurrent requests:
   - 10 concurrent health checks
   - Tests goroutine safety

## Test Coverage

Current coverage: **82.9%** of handler code ✅

### Generate Coverage Report

```bash
# Generate HTML coverage report
make test-coverage

# View coverage report
open coverage.html

# Or generate manually
go test -coverprofile=coverage.out ./internal/api/handlers/...
go tool cover -html=coverage.out -o coverage.html
```

### Coverage by Component

| Component | Coverage | Status |
|-----------|----------|--------|
| Event Handlers | 95.5% | ✅ Excellent |
| Health Handlers | 100.0% | ✅ Complete |
| Subscription Handlers | 90.2% | ✅ Excellent |
| WebSocket Handlers | 66.7% | ⚠️ Good (StreamEvents needs integration test) |
| **Overall** | **82.9%** | **✅ Exceeds Target** |

### Coverage Goals

- **Handlers**: 80%+ coverage ✅ Achieved (82.9%)
- **Subscription Manager**: 70%+ coverage (target)
- **Event Router**: 60%+ coverage (target)
- **Overall**: 65%+ coverage (target)

## Writing New Tests

### Unit Test Template

```go
package handlers

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

func TestYourHandler_Success(t *testing.T) {
    // Arrange
    mockManager := new(MockSubscriptionManager)
    mockManager.On("YourMethod", mock.Anything).Return(expectedResult, nil)
    
    handler := NewYourHandler(mockManager)
    app := fiber.New()
    app.Post("/endpoint", handler.YourEndpoint)
    
    // Act
    req := httptest.NewRequest("POST", "/endpoint", nil)
    resp, _ := app.Test(req)
    
    // Assert
    assert.Equal(t, 200, resp.StatusCode)
    mockManager.AssertExpectations(t)
}
```

### Integration Test Template

```go
package integration

import (
    "testing"
    "net/http"
)

func TestAPI_YourFeature(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }
    
    // Test implementation
    resp, err := http.Get("http://localhost:8080/api/v1/endpoint")
    assert.NoError(t, err)
    assert.Equal(t, 200, resp.StatusCode)
}
```

## Benchmark Tests

Benchmark tests measure performance characteristics.

### Running Benchmarks

```bash
# Run all benchmarks
make test-bench

# Run benchmarks with memory profiling
go test -bench=. -benchmem ./internal/api/handlers/...

# Run specific benchmark
go test -bench=BenchmarkCreateSubscription ./internal/api/handlers/...

# Save benchmark results
go test -bench=. -benchmem ./... > bench.txt
```

### Benchmark Example

```go
func BenchmarkCreateSubscription(b *testing.B) {
    mockManager := new(MockSubscriptionManager)
    handler := NewSubscriptionHandler(mockManager)
    app := fiber.New()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        req := httptest.NewRequest("POST", "/api/v1/subscriptions", body)
        app.Test(req)
    }
}
```

## Continuous Integration

### GitHub Actions Example

```yaml
name: Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.24'
      
      - name: Run tests
        run: make test
      
      - name: Generate coverage
        run: make test-coverage
      
      - name: Upload coverage
        uses: codecov/codecov-action@v3
        with:
          file: ./coverage.out
```

## Debugging Tests

### Verbose Output

```bash
# Run with verbose output
go test -v ./...

# Run specific test with verbose output
go test -v -run TestCreateSubscription_Success ./internal/api/handlers/...
```

### Debug with Delve

```bash
# Install Delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug test
dlv test ./internal/api/handlers/ -- -test.run TestCreateSubscription_Success
```

### Print Debug Information

```go
func TestDebug(t *testing.T) {
    t.Logf("Debug info: %+v", yourVariable)
    t.Logf("Request body: %s", string(bodyBytes))
}
```

## Best Practices

### 1. Test Naming

- Use descriptive names: `TestFunctionName_Scenario`
- Examples: `TestCreateSubscription_Success`, `TestGetSubscription_NotFound`

### 2. Test Structure (AAA Pattern)

```go
func TestExample(t *testing.T) {
    // Arrange - Set up test data and mocks
    mockManager := new(MockSubscriptionManager)
    
    // Act - Execute the code under test
    result := handler.DoSomething()
    
    // Assert - Verify the results
    assert.Equal(t, expected, result)
}
```

### 3. Use Table-Driven Tests

```go
func TestValidation(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {"valid address", "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf", false},
        {"empty address", "", true},
        {"invalid address", "invalid", true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validateAddress(tt.input)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### 4. Clean Up Resources

```go
func TestWithCleanup(t *testing.T) {
    resource := setupResource()
    defer func() {
        cleanupResource(resource)
    }()
    
    // Test code
}
```

### 5. Mock Expectations

```go
// Set up mock expectations
mockManager.On("Subscribe", "address", "", mock.Anything, int64(0)).
    Return(&models.Subscription{ID: "123"}, nil).
    Once()

// Verify all expectations were met
mockManager.AssertExpectations(t)
```

## Troubleshooting

### Test Failures

1. **Mock expectations not met**: Check `mockManager.AssertExpectations(t)`
2. **Connection refused**: Ensure API server is running for integration tests
3. **Timeout errors**: Increase timeout in test context
4. **Race conditions**: Run with `-race` flag to detect

### Common Issues

**Issue**: Integration tests fail with "connection refused"
**Solution**: Start the API server: `./bin/mongotron-api`

**Issue**: Tests pass individually but fail when run together
**Solution**: Ensure proper cleanup between tests, avoid shared state

**Issue**: Coverage report not generated
**Solution**: Check file permissions, ensure `coverage.out` can be created

## Test Metrics

### Current Test Statistics

- **Total Tests**: 13
- **Unit Tests**: 13
- **Integration Tests**: 8+ sub-tests across 5 suites
- **Coverage**: 38.1% (handlers)
- **Execution Time**: <10ms (unit tests)

### Goals

- Increase coverage to 80%+
- Add performance benchmarks
- Add load testing suite
- Implement chaos testing for resilience

## Resources

- [Go Testing Package](https://pkg.go.dev/testing)
- [Testify Documentation](https://github.com/stretchr/testify)
- [Fiber Testing Guide](https://docs.gofiber.io/guide/testing)
- [Table-Driven Tests in Go](https://dave.cheney.net/2019/05/07/prefer-table-driven-tests)

## Next Steps

1. **Increase Coverage**: Add more unit tests for edge cases
2. **Load Testing**: Use `vegeta` or `k6` for load testing
3. **E2E Tests**: Add Selenium/Playwright tests for full stack
4. **Mutation Testing**: Use `go-mutesting` to verify test quality
5. **Contract Testing**: Implement Pact tests for API contracts
