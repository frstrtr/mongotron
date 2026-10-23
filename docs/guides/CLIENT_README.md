# MongoTron Python Test Client

A comprehensive Python test client for testing all MongoTron API endpoints and services.

## Features

- ✅ Tests all REST API endpoints (health, subscriptions, events)
- ✅ Tests WebSocket streaming endpoint
- ✅ Validates response formats and status codes
- ✅ Provides detailed test reports with pass/fail status
- ✅ Supports custom base URLs for different environments

## Prerequisites

- Python 3.8 or higher
- MongoTron API server running

## Installation

1. Create a virtual environment:
```bash
python3 -m venv venv
source venv/bin/activate  # On Windows: venv\Scripts\activate
```

2. Install dependencies:
```bash
pip install -r requirements.txt
```

## Usage

### Basic Usage

Run tests against local server (default: http://localhost:8080):
```bash
python test_client.py
```

### Custom Base URL

Test against a different server:
```bash
python test_client.py http://staging-server:8080
python test_client.py https://api.mongotron.io
```

## Test Categories

### 1. Root Endpoint
- **GET /** - API information and available endpoints

### 2. Health Endpoints
- **GET /api/v1/health** - Overall health status
- **GET /api/v1/ready** - Readiness check (K8s)
- **GET /api/v1/live** - Liveness check (K8s)

### 3. Subscription Endpoints
- **POST /api/v1/subscriptions** - Create new subscription
- **GET /api/v1/subscriptions** - List all subscriptions
- **GET /api/v1/subscriptions/:id** - Get specific subscription
- **DELETE /api/v1/subscriptions/:id** - Delete subscription

### 4. Event Endpoints
- **GET /api/v1/events** - List all events
- **GET /api/v1/events/:id** - Get specific event
- **GET /api/v1/events/tx/:hash** - Get events by transaction hash

### 5. WebSocket Endpoint
- **WebSocket /api/v1/events/stream/:subscriptionId** - Real-time event streaming

## Example Output

```
============================================================
🚀 MongoTron API Test Suite
============================================================

📍 Testing Root Endpoint
------------------------------------------------------------
✅ Root Endpoint: PASS - Version: 1.0.0

💚 Testing Health Endpoints
------------------------------------------------------------
✅ Health Check: PASS - Uptime: 598s, Active: 0
✅ Readiness Check: PASS - Status: ready, MongoDB: connected, Tron: connected
✅ Liveness Check: PASS

📋 Testing Subscription Endpoints
------------------------------------------------------------
✅ Create Subscription: PASS - ID: sub_0fb787f2-ba2
✅ List Subscriptions: PASS - Found 1 subscriptions (total: 1)
✅ Get Subscription: PASS - Address: TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t

📊 Testing Event Endpoints
------------------------------------------------------------
✅ List Events: PASS - Found 0 events (total: 0)
ℹ️ Get Event: INFO - No events exist (expected behavior)

🔌 Testing WebSocket Endpoint
------------------------------------------------------------
✅ WebSocket Stream: PASS - Connected successfully, received 1 messages

============================================================
📊 Test Summary
============================================================
Total Tests:  15
✅ Passed:    14
❌ Failed:    1
```

## Test Details

### Subscription Creation

The test creates a subscription for the USDT contract on Tron:
```json
{
  "address": "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t",
  "filters": {},
  "startBlock": -1
}
```

### WebSocket Testing

The WebSocket test:
1. Connects to the streaming endpoint
2. Waits for connection confirmation
3. Receives initial "connected" message
4. Monitors for 3 seconds
5. Closes connection gracefully

## Customization

### Modify Test Duration

Edit `test_websocket()` method to change duration:
```python
def test_websocket(self, sub_id: str, duration: int = 10):  # 10 seconds instead of 3
```

### Add Custom Tests

Extend the `MongoTronClient` class with additional test methods:
```python
def test_custom_endpoint(self):
    """Test your custom endpoint"""
    success, data, error = self._make_request("GET", "/custom")
    # Your test logic here
```

## Exit Codes

- **0**: All tests passed
- **1**: One or more tests failed

## Dependencies

- **requests**: HTTP client for REST API testing
- **websocket-client**: WebSocket client for streaming tests

## Troubleshooting

### Connection Refused

```bash
# Make sure the API server is running
./bin/api-server

# Check if server is listening
curl http://localhost:8080/api/v1/health
```

### ModuleNotFoundError

```bash
# Activate virtual environment
source venv/bin/activate

# Reinstall dependencies
pip install -r requirements.txt
```

### WebSocket Connection Failed

- Ensure the subscription exists before testing WebSocket
- Check that the WebSocket middleware is properly configured
- Verify firewall rules allow WebSocket connections

## Known Issues

1. **Delete Subscription Test**: May cause server panic due to WebSocket cleanup bug (see TEST_RESULTS.md)

## Contributing

To add new tests:

1. Add test method to `MongoTronClient` class
2. Follow naming convention: `test_<endpoint_name>`
3. Use `_print_test()` helper for consistent output
4. Add test call to `run_all_tests()` method

## License

Part of the MongoTron project.
