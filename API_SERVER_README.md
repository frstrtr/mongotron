# MongoTron API Server

The MongoTron API Server provides a REST API and WebSocket interface for subscription-based blockchain monitoring on the Tron network.

## Features

- **Subscription-Based Monitoring**: Create subscriptions to monitor specific Tron addresses
- **Real-time WebSocket Streaming**: Receive blockchain events in real-time via WebSocket
- **REST API**: Full CRUD operations for subscriptions and event queries
- **Webhook Support**: Optional webhook delivery for events
- **Event Filtering**: Filter events by contract type, amount range, and success status
- **Persistent Storage**: All events stored in MongoDB for historical queries

## Quick Start

### 1. Start the API Server

```bash
./bin/mongotron-api
```

The server will start on `localhost:8080` by default.

### 2. Check Health

```bash
curl http://localhost:8080/api/v1/health
```

### 3. Create a Subscription

```bash
curl -X POST http://localhost:8080/api/v1/subscriptions \
  -H "Content-Type: application/json" \
  -d '{
    "address": "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
    "webhookUrl": "https://your-webhook-endpoint.com/events",
    "filters": {
      "contractTypes": ["TransferContract", "TriggerSmartContract"],
      "minAmount": 0,
      "maxAmount": 0,
      "onlySuccess": true
    },
    "startBlock": 0
  }'
```

Response:
```json
{
  "subscriptionId": "sub_abc123xyz",
  "address": "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
  "network": "tron-nile",
  "status": "active",
  "eventsCount": 0,
  "startBlock": 123456,
  "currentBlock": 123456,
  "createdAt": "2025-01-15T10:30:00Z",
  "updatedAt": "2025-01-15T10:30:00Z"
}
```

### 4. Connect to WebSocket for Real-time Events

```javascript
const ws = new WebSocket('ws://localhost:8080/api/v1/events/stream/sub_abc123xyz');

ws.onopen = () => {
  console.log('Connected to event stream');
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Received event:', data);
};

ws.onerror = (error) => {
  console.error('WebSocket error:', error);
};

ws.onclose = () => {
  console.log('Disconnected from event stream');
};
```

## API Endpoints

### Health & Status

- `GET /api/v1/health` - Health check with service status
- `GET /api/v1/ready` - Readiness probe
- `GET /api/v1/live` - Liveness probe

### Subscriptions

- `POST /api/v1/subscriptions` - Create new subscription
- `GET /api/v1/subscriptions` - List all subscriptions (supports pagination)
- `GET /api/v1/subscriptions/:id` - Get subscription by ID
- `DELETE /api/v1/subscriptions/:id` - Stop and remove subscription

### Events

- `GET /api/v1/events` - List events (supports pagination and filtering)
- `GET /api/v1/events/:id` - Get event by ID
- `GET /api/v1/events/tx/:hash` - Get events by transaction hash

### WebSocket

- `WS /api/v1/events/stream/:subscriptionId` - Real-time event stream for subscription

## Subscription Filters

Filters allow you to narrow down which events you receive:

- **contractTypes**: Array of contract types to monitor (e.g., `["TransferContract", "TriggerSmartContract"]`)
- **minAmount**: Minimum transaction amount (in SUN, 1 TRX = 1,000,000 SUN)
- **maxAmount**: Maximum transaction amount (0 = no limit)
- **onlySuccess**: If true, only successful transactions are included

## Event Structure

Events received via WebSocket or REST API:

```json
{
  "eventId": "evt_abc123...",
  "subscriptionId": "sub_abc123xyz",
  "network": "tron-nile",
  "type": "TransferContract",
  "address": "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
  "txHash": "0123456789abcdef...",
  "blockNumber": 123456,
  "blockTimestamp": 1736945400,
  "data": {
    "from": "TFromAddress...",
    "to": "TToAddress...",
    "amount": 1000000,
    "asset": "TRX",
    "success": true,
    "eventType": "Transfer",
    "eventData": {}
  },
  "processed": false,
  "createdAt": "2025-01-15T10:30:00Z"
}
```

## Configuration

The server is configured via `mongotron.yaml` or environment variables:

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  read_timeout: "30s"
  write_timeout: "30s"
  idle_timeout: "120s"

database:
  mongodb:
    uri: "mongodb://localhost:27017"
    database: "mongotron"

blockchain:
  tron:
    node:
      host: "nilevm.lan"
      port: 50051
      use_tls: false
    connection:
      timeout: "30s"
      max_retries: 3
      backoff_interval: "5s"
      keep_alive: "60s"

logging:
  level: "info"
  format: "json"
  output: "stdout"
```

Environment variables (prefixed with `MONGOTRON_`):

```bash
export MONGOTRON_SERVER_PORT=8080
export MONGOTRON_DATABASE_MONGODB_URI="mongodb://localhost:27017"
export MONGOTRON_LOGGING_LEVEL="debug"
```

## Architecture

```
Client → REST API (Fiber) → Subscription Manager → AddressMonitor Pool → Tron Node
              ↓                    ↓
       WebSocket Hub ← Event Router ← MongoDB
```

**Components:**

1. **Subscription Manager**: Orchestrates multiple AddressMonitor instances based on active subscriptions
2. **Event Router**: Routes events to WebSocket clients and webhooks, stores in database
3. **WebSocket Hub**: Manages WebSocket client connections and broadcasts events
4. **AddressMonitor**: Polls Tron blockchain for specific address activity
5. **MongoDB**: Persistent storage for subscriptions, events, and metadata

## Examples

### List All Subscriptions

```bash
curl http://localhost:8080/api/v1/subscriptions?limit=10&skip=0
```

### Query Events by Address

```bash
curl "http://localhost:8080/api/v1/events?address=TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf&limit=50"
```

### Get Specific Event

```bash
curl http://localhost:8080/api/v1/events/evt_abc123...
```

### Stop a Subscription

```bash
curl -X DELETE http://localhost:8080/api/v1/subscriptions/sub_abc123xyz
```

## Development

### Build

```bash
go build -o bin/mongotron-api cmd/api-server/main.go
```

### Run Tests

```bash
go test ./...
```

### Docker

```bash
docker build -t mongotron-api -f docker/Dockerfile.api .
docker run -p 8080:8080 mongotron-api
```

## Monitoring

The API provides several endpoints for monitoring:

- Health check: `/api/v1/health` - Returns service status, version, active monitors, uptime
- Readiness: `/api/v1/ready` - Kubernetes readiness probe
- Liveness: `/api/v1/live` - Kubernetes liveness probe

## Rate Limiting

The API includes built-in rate limiting (100 requests per minute per IP). Clients exceeding the limit will receive a `429 Too Many Requests` response.

## WebSocket Protocol

**Connection:**
```
WS /api/v1/events/stream/:subscriptionId
```

**Welcome Message:**
```json
{
  "type": "connected",
  "subscriptionId": "sub_abc123xyz",
  "timestamp": 1736945400,
  "message": "Connected to MongoTron event stream"
}
```

**Event Messages:**
Events are sent as JSON objects (see Event Structure above).

**Heartbeat:**
The server sends ping frames every 54 seconds. Clients should respond with pong frames.

**Disconnection:**
Client can close the connection normally. Server will automatically unregister the client.

## License

MIT

## Support

For issues and questions, please file an issue on GitHub.
