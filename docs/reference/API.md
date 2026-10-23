# MongoTron API Documentation

## Overview

MongoTron provides three primary APIs for interacting with the blockchain monitoring service:

1. **REST API** - HTTP/JSON endpoints for subscription management
2. **WebSocket API** - Real-time event streaming
3. **gRPC API** - High-performance internal service communication

## REST API

### Base URL
```
https://api.mongotron.io/api/v1
```

### Authentication
All requests require an API key in the header:
```
X-API-Key: your-api-key
```

### Endpoints

Refer to the main README.md for detailed endpoint documentation.

## WebSocket API

### Connection
```
wss://api.mongotron.io/ws
```

### Message Format
All messages are JSON formatted.

## Rate Limits

- REST API: 1000 requests per minute per API key
- WebSocket: 10,000 events per minute per connection
- Burst limit: 100 requests

## Error Codes

| Code | Description |
|------|-------------|
| 400  | Bad Request |
| 401  | Unauthorized |
| 403  | Forbidden |
| 404  | Not Found |
| 429  | Too Many Requests |
| 500  | Internal Server Error |
| 503  | Service Unavailable |
