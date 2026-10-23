# 🚀 MongoTron Startup Guide

Complete guide to start MongoTron with MongoDB and Tron Nile Testnet node at nileVM.lan

## Prerequisites Check

### 1. Verify MongoDB Connection
```bash
# Test MongoDB connectivity
mongosh "mongodb://mongotron:MongoTron2025@nileVM.lan:27017/mongotron" --eval "db.runCommand({ ping: 1 })"
```

Expected output: `{ ok: 1 }`

### 2. Verify Tron Node Connection
```bash
# Test Tron node gRPC connection
grpcurl -plaintext nileVM.lan:50051 list

# Or check with curl (if HTTP API is available)
curl -s http://nileVM.lan:8090/wallet/getnowblock | jq '.block_header.raw_data.number'
```

## Quick Start (Recommended)

### Option 1: Start with Existing Configuration

The configuration is already set up in `configs/.env`:

```bash
cd /home/user0/Github/mongotron

# Start the API server
./bin/api-server

# Or start in background with logging
nohup ./bin/api-server > /tmp/api-server.log 2>&1 &

# Check logs
tail -f /tmp/api-server.log
```

### Option 2: Start with Custom Configuration

```bash
cd /home/user0/Github/mongotron

# Export environment variables
export TRON_NODE_HOST=nileVM.lan
export TRON_NODE_PORT=50051
export MONGODB_URI=mongodb://mongotron:MongoTron2025@nileVM.lan:27017/mongotron

# Start the server
./bin/api-server
```

## Detailed Startup Process

### Step 1: Navigate to Project Directory
```bash
cd /home/user0/Github/mongotron
```

### Step 2: Verify Binary Exists
```bash
ls -lh bin/api-server

# If binary doesn't exist, build it:
go build -o bin/api-server cmd/api-server/main.go
```

### Step 3: Check Configuration
```bash
# View current configuration
cat configs/.env

# Current settings:
# - MongoDB: nileVM.lan:27017
# - Tron Node: nileVM.lan:50051 (Nile testnet)
# - API Port: 8080
```

### Step 4: Start MongoTron API Server

**Foreground (interactive)**:
```bash
./bin/api-server
```

**Background (daemon mode)**:
```bash
# Start in background with logging
nohup ./bin/api-server > /tmp/api-server.log 2>&1 &

# Save the process ID
echo $! > /tmp/api-server.pid

# View logs
tail -f /tmp/api-server.log

# Check if running
ps aux | grep api-server | grep -v grep
```

### Step 5: Verify Server is Running

```bash
# Check health endpoint
curl http://localhost:8080/health

# Expected response:
# {"status":"ok","timestamp":"2025-10-06T..."}

# Check API info
curl http://localhost:8080/api/v1/info

# List active subscriptions
curl http://localhost:8080/api/v1/subscriptions | jq .
```

## Configuration Details

### Current Configuration (configs/.env)

```properties
# API Server
MONGOTRON_PORT=8080
MONGOTRON_HOST=0.0.0.0

# Tron Node (Nile Testnet)
TRON_NODE_HOST=nileVM.lan
TRON_NODE_PORT=50051

# MongoDB
MONGODB_URI=mongodb://mongotron:MongoTron2025@nileVM.lan:27017/mongotron
MONGODB_DATABASE=mongotron

# Performance
MONGOTRON_WORKERS=1000
MONGOTRON_MAX_ADDRESSES=50000
MONGOTRON_BATCH_SIZE=1000
```

### Network Information

**Tron Nile Testnet:**
- Network ID: Nile
- Node: nileVM.lan:50051 (gRPC)
- Block time: ~3 seconds
- Explorer: https://nile.tronscan.org

**MongoDB:**
- Host: nileVM.lan:27017
- Database: mongotron
- User: mongotron
- Auth: Password-based (MongoTron2025)

## Using the Event Monitor

Once the API server is running, you can monitor blockchain events:

```bash
cd /home/user0/Github/mongotron
source .venv/bin/activate

# Monitor USDT transfers on Nile testnet
python event_monitor.py

# The monitor will:
# - Create a subscription for TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf (USDT)
# - Connect via WebSocket
# - Display events with human-readable addresses
# - Log events to timestamped file (events_YYYYMMDD_HHMMSS.log)
```

## Management Commands

### Check Server Status
```bash
# Check if running
curl http://localhost:8080/health

# View metrics
curl http://localhost:8080/metrics

# Check subscriptions
curl http://localhost:8080/api/v1/subscriptions | jq '.subscriptions[] | {id: .subscriptionId, status: .status, events: .eventsCount}'
```

### Stop the Server
```bash
# If running in foreground: Press Ctrl+C

# If running in background:
kill $(cat /tmp/api-server.pid)

# Or find and kill by name:
pkill -f "api-server"

# Verify it stopped:
ps aux | grep api-server | grep -v grep
```

### Restart the Server
```bash
# Stop existing server
pkill -f "api-server"

# Wait a moment
sleep 2

# Start new instance
nohup ./bin/api-server > /tmp/api-server.log 2>&1 &
echo $! > /tmp/api-server.pid

# Verify it's running
curl http://localhost:8080/health
```

### View Logs
```bash
# Real-time log monitoring
tail -f /tmp/api-server.log

# Last 100 lines
tail -100 /tmp/api-server.log

# Search for errors
grep -i error /tmp/api-server.log

# Search for specific subscription
grep "sub_[a-z0-9-]*" /tmp/api-server.log

# Filter by log level
grep '"level":"error"' /tmp/api-server.log | jq .
```

## API Endpoints

Once started, MongoTron provides these endpoints:

### Core API (Port 8080)

**Health & Info:**
- `GET /health` - Health check
- `GET /api/v1/info` - System information

**Subscriptions:**
- `POST /api/v1/subscriptions` - Create subscription
- `GET /api/v1/subscriptions` - List all subscriptions
- `GET /api/v1/subscriptions/:id` - Get subscription details
- `DELETE /api/v1/subscriptions/:id` - Stop subscription

**Events:**
- `GET /api/v1/events` - List captured events
- `GET /api/v1/events/stream/:subscriptionId` - WebSocket stream

**Example Usage:**
```bash
# Create a subscription
curl -X POST http://localhost:8080/api/v1/subscriptions \
  -H "Content-Type: application/json" \
  -d '{
    "address": "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
    "filters": {},
    "startBlock": -1
  }'

# List events
curl http://localhost:8080/api/v1/events?limit=10 | jq .
```

## Troubleshooting

### Server Won't Start

**Check if port 8080 is already in use:**
```bash
lsof -i :8080
netstat -tuln | grep 8080
```

**Check MongoDB connection:**
```bash
mongosh "mongodb://mongotron:MongoTron2025@nileVM.lan:27017/mongotron" --eval "db.runCommand({ ping: 1 })"
```

**Check Tron node connectivity:**
```bash
# Test gRPC connection
grpcurl -plaintext nileVM.lan:50051 list

# Or check with telnet
telnet nileVM.lan 50051
```

**Check logs for errors:**
```bash
tail -50 /tmp/api-server.log | grep -i error
```

### MongoDB Connection Issues

```bash
# Test connection string
mongosh "mongodb://mongotron:MongoTron2025@nileVM.lan:27017/mongotron" --eval "db.adminCommand('ping')"

# Check MongoDB service
ssh nileVM.lan "systemctl status mongod"

# Check network connectivity
ping -c 3 nileVM.lan
```

### Tron Node Connection Issues

```bash
# Test gRPC port
nc -zv nileVM.lan 50051

# Check if Tron node is running
ssh nileVM.lan "ps aux | grep java | grep FullNode"

# View Tron node logs
ssh nileVM.lan "tail -100 /path/to/tron/logs/tron.log"
```

### No Events Being Captured

```bash
# Check if monitor is running
curl http://localhost:8080/api/v1/subscriptions | jq '.subscriptions[] | select(.status=="active")'

# Check block processing
curl http://localhost:8080/api/v1/subscriptions | jq '.subscriptions[] | {id: .subscriptionId, currentBlock: .currentBlock}'

# Manually trigger a test transaction on Nile testnet
# Then check events
curl http://localhost:8080/api/v1/events?limit=5 | jq .
```

## Complete Startup Example

Here's a complete example from scratch:

```bash
# 1. Navigate to project
cd /home/user0/Github/mongotron

# 2. Stop any existing instances
pkill -f "api-server"
sleep 2

# 3. Verify configuration
echo "MongoDB: $(grep MONGODB_URI configs/.env | cut -d= -f2)"
echo "Tron Node: $(grep TRON_NODE_HOST configs/.env | cut -d= -f2):$(grep TRON_NODE_PORT configs/.env | cut -d= -f2)"

# 4. Start the server
nohup ./bin/api-server > /tmp/api-server.log 2>&1 &
echo $! > /tmp/api-server.pid

# 5. Wait for startup
sleep 3

# 6. Verify it's running
curl -s http://localhost:8080/health | jq .

# 7. Check logs
tail -20 /tmp/api-server.log

# 8. Start monitoring (in new terminal)
source .venv/bin/activate
python event_monitor.py
```

## System Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         MongoTron                            │
│                                                              │
│  ┌──────────────┐    ┌──────────────┐   ┌──────────────┐  │
│  │  API Server  │    │ Subscription │   │    Event     │  │
│  │  (Port 8080) │───→│   Manager    │──→│   Router     │  │
│  └──────────────┘    └──────────────┘   └──────────────┘  │
│         │                    │                    │         │
└─────────┼────────────────────┼────────────────────┼─────────┘
          │                    │                    │
          │                    ↓                    ↓
          │          ┌──────────────┐    ┌──────────────┐
          │          │   MongoDB    │    │  WebSocket   │
          │          │ nileVM.lan   │    │     Hub      │
          │          │   :27017     │    │              │
          │          └──────────────┘    └──────────────┘
          │                                       │
          ↓                                       ↓
┌──────────────────┐                  ┌──────────────────┐
│   Tron Node      │                  │  Event Monitor   │
│   (Nile Testnet) │                  │   (Python)       │
│   nileVM.lan     │                  │                  │
│   :50051 (gRPC)  │                  │  - WebSocket     │
└──────────────────┘                  │  - Display       │
                                      │  - File logging  │
                                      └──────────────────┘
```

## Next Steps

After starting MongoTron:

1. ✅ **Verify Health**: `curl http://localhost:8080/health`
2. ✅ **Create Subscription**: Use API or start event_monitor.py
3. ✅ **Monitor Events**: Watch the WebSocket stream or check logs
4. ✅ **Check Database**: Verify events are being stored in MongoDB

## Quick Reference

| Action | Command |
|--------|---------|
| **Start Server** | `nohup ./bin/api-server > /tmp/api-server.log 2>&1 &` |
| **Stop Server** | `pkill -f "api-server"` |
| **View Logs** | `tail -f /tmp/api-server.log` |
| **Check Health** | `curl http://localhost:8080/health` |
| **List Subscriptions** | `curl http://localhost:8080/api/v1/subscriptions \| jq .` |
| **View Events** | `curl http://localhost:8080/api/v1/events \| jq .` |
| **Start Monitor** | `python event_monitor.py` |

## Support

- **Documentation**: See README.md and other guides in the repository
- **Logs**: Check `/tmp/api-server.log` for server logs
- **Event Logs**: Check `events_*.log` files for captured events

---

**Last Updated**: October 6, 2025  
**Status**: ✅ Ready to use with MongoDB and Tron Nile testnet at nileVM.lan
