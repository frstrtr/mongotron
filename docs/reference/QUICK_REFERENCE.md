# MongoTron - Quick Reference Card

## 🚀 Start/Stop Services

### Start API Server
```bash
cd /home/user0/Github/mongotron
go build -o bin/api-server cmd/api-server/main.go
nohup ./bin/api-server > /tmp/api-server.log 2>&1 &
```

### Start Event Monitor
```bash
cd /home/user0/Github/mongotron
source venv/bin/activate
python event_monitor.py
# Or in background:
nohup python event_monitor.py > monitor.log 2>&1 &
```

### Stop Services
```bash
# Stop API server
pkill -f api-server

# Stop event monitor
pkill -f "python.*event_monitor"
# Or press Ctrl+C if in foreground
```

## 📊 Check Status

### Service Health
```bash
# API server ready check
curl http://localhost:8080/api/v1/ready

# List active subscriptions
curl http://localhost:8080/api/v1/subscriptions | jq '.subscriptions[] | select(.status=="active")'

# Count active monitors
curl http://localhost:8080/api/v1/ready | jq '.active_monitors'
```

### Recent Events
```bash
# Get last 5 events
curl http://localhost:8080/api/v1/events?limit=5 | jq .

# Get event by ID
curl http://localhost:8080/api/v1/events/evt_abc123 | jq .

# Get events by TX hash
curl http://localhost:8080/api/v1/events/tx/cc86e5c0... | jq .
```

### Monitor Logs
```bash
# Watch server logs
tail -f /tmp/api-server.log

# Watch event monitor logs
tail -f events_*.log

# Count events in log
grep -c "EVENT #" events_*.log
```

## 🔧 Common Operations

### Create Subscription (API)
```bash
curl -X POST http://localhost:8080/api/v1/subscriptions \
  -H "Content-Type: application/json" \
  -d '{
    "address": "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
    "filters": {"onlySuccess": true}
  }'
```

### Delete Subscription
```bash
curl -X DELETE http://localhost:8080/api/v1/subscriptions/sub_abc123
```

### Monitor Specific Address
```bash
python event_monitor.py --address TYourAddress123
```

### Monitor with Filters
```bash
python event_monitor.py --filters '{"event_name": "Transfer"}'
```

## 📁 Important Files

### Configuration
- `configs/mongotron.yaml` - Server config (MongoDB, Tron node)

### Code
- `cmd/api-server/main.go` - API server entry point
- `internal/subscription/manager.go` - Subscription management
- `internal/blockchain/monitor/address_monitor.go` - Block monitoring
- `event_monitor.py` - Python monitoring client

### Logs
- `/tmp/api-server.log` - API server logs
- `events_YYYYMMDD_HHMMSS.log` - Event monitor logs

### Documentation
- `README.md` - Project overview
- `SESSION_SUMMARY.md` - Latest session summary
- `BUGFIX_BLOCK_PROCESSING.md` - Bug fix details
- `EVENT_LOGGING_GUIDE.md` - Logging guide
- `EVENT_MONITOR_README.md` - Monitor usage

## 🔍 Troubleshooting

### Server Won't Start
```bash
# Check if already running
ps aux | grep api-server

# Check port 8080 is free
netstat -tlnp | grep 8080

# Check MongoDB connection
mongo mongodb://mongotron:MongoTron2025@nileVM.lan:27017/mongotron
```

### No Events Received
```bash
# Check currentBlock is advancing
curl http://localhost:8080/api/v1/subscriptions/sub_xxx | jq .currentBlock
# Wait 10-15 seconds and check again - should increase

# Check subscription is active
curl http://localhost:8080/api/v1/subscriptions/sub_xxx | jq .status
# Should be "active"

# Check server logs for errors
tail -50 /tmp/api-server.log | grep -i error
```

### Monitor Crashes
```bash
# Check Python environment
source venv/bin/activate
pip list | grep -E "requests|websocket"

# Test connection
curl http://localhost:8080/api/v1/ready

# Check for port conflicts
netstat -tlnp | grep 8080
```

## 🎯 Quick Tests

### Test API Server
```bash
curl http://localhost:8080/
# Should return welcome message

curl http://localhost:8080/api/v1/ready
# Should return {"status":"ready"}
```

### Test Event Monitor
```bash
# Run for 30 seconds
timeout 30 python event_monitor.py
# Should connect and monitor (may or may not see events)
```

### Test Subscription Flow
```bash
# 1. Create
SUB_ID=$(curl -s -X POST http://localhost:8080/api/v1/subscriptions \
  -H "Content-Type: application/json" \
  -d '{"address":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf"}' | jq -r .subscriptionId)

# 2. Check status
curl http://localhost:8080/api/v1/subscriptions/$SUB_ID | jq .

# 3. Wait and check currentBlock increased
sleep 15
curl http://localhost:8080/api/v1/subscriptions/$SUB_ID | jq .currentBlock

# 4. Delete
curl -X DELETE http://localhost:8080/api/v1/subscriptions/$SUB_ID
```

## 📊 Log Analysis

### Event Statistics
```bash
# Count total events
grep -c "EVENT #" events_*.log

# List all transaction hashes
grep -oP '"transaction_hash":\s*"\K[^"]+' events_*.log

# Count events by block
grep -oP '"block_number":\s*\K\d+' events_*.log | sort | uniq -c

# Find Transfer events
grep "Transfer" events_*.log | wc -l
```

### Performance Metrics
```bash
# Calculate events per minute
EVENTS=$(grep -c "EVENT #" events_*.log)
START=$(head -1 events_*.log | awk '{print $1,$2}')
END=$(tail -1 events_*.log | awk '{print $1,$2}')
# Calculate duration and divide
```

## 🌐 Infrastructure

### Nile Testnet
- **Network:** Tron Nile Testnet
- **Tron Node:** nileVM.lan:50051
- **MongoDB:** nileVM.lan:27017
- **API Server:** localhost:8080

### Contracts
- **USDT (Nile):** TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf
- **USDT (Mainnet):** TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t

## 🔗 Useful Links

- Tron Nile Explorer: https://nile.tronscan.org/
- API Docs: http://localhost:8080/ (when running)
- WebSocket: ws://localhost:8080/api/v1/events/stream/{subscriptionId}

---

**Last Updated:** October 5, 2025  
**System Status:** ✅ Operational
