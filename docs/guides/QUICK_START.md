# 🎉 MongoTron Quick Start - READY TO USE!

## ✅ Your System is Already Running!

**MongoTron API Server**: ✅ Running on http://localhost:8080  
**MongoDB**: ✅ Connected to nileVM.lan:27017  
**Tron Node**: ✅ Connected to nileVM.lan:50051 (Nile Testnet)  
**Active Subscriptions**: ✅ 3 subscriptions, 3,538+ events captured

---

## 🚀 Quick Start Commands

### 1. Check Server Status
```bash
# Check if server is running
curl http://localhost:8080/api/v1/subscriptions | jq '.subscriptions[] | select(.status=="active") | {id: .subscriptionId, events: .eventsCount}'

# Expected output: List of active subscriptions with event counts
```

### 2. Start Event Monitor (See Live Events)
```bash
cd /home/user0/Github/mongotron
source .venv/bin/activate
python event_monitor.py
```

**What you'll see**:
- Real-time blockchain events
- Human-readable Tron addresses (not hex!)
- Transaction details (TX hash, block, timestamp)
- Events logged to timestamped file

**Example Output**:
```
================================================================================
🔔 TRANSACTION EVENT #1 - TriggerSmartContract
================================================================================
📍 TX ID:       28afe626e98ad2e3c2eb750da1999f585d398feccb21c87ff63c26970006016b
📦 Block:       61090425
⏰ Time:        2025-10-05 23:31:33
✅ Success:     True
────────────────────────────────────────────────────────────────────────────────
📊 ADDRESSES:
────────────────────────────────────────────────────────────────────────────────
   From (hex):  4185e86b92fe197f8e78fb2ea78c10122ed41118e1
   From:        TNBFJQqkebEQau7HsdxPzYJbB2XUrZK3Ue
   To (hex):    41eca9bc828a3005b9a3b909f2cc5c2a54794de05f
   To:          TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf
────────────────────────────────────────────────────────────────────────────────
```

### 3. View Recent Events (API)
```bash
# Get last 5 events
curl http://localhost:8080/api/v1/events?limit=5 | jq '.events[] | {block: .blockNumber, from: .data.from, to: .data.to}'

# Count total events
curl http://localhost:8080/api/v1/events | jq '.total'
```

### 4. View Server Logs
```bash
# Real-time log monitoring
tail -f /tmp/api-server.log

# Last 50 lines
tail -50 /tmp/api-server.log

# Search for errors
grep -i error /tmp/api-server.log
```

---

## 📋 Management Commands

### Stop the Server
```bash
pkill -f "bin/api-server"
```

### Restart the Server
```bash
cd /home/user0/Github/mongotron
pkill -f "bin/api-server" && sleep 2
nohup ./bin/api-server > /tmp/api-server.log 2>&1 &
```

### Rebuild the Server (after code changes)
```bash
cd /home/user0/Github/mongotron
go build -o bin/api-server cmd/api-server/main.go
```

---

## 🎯 Common Tasks

### Create a New Subscription
```bash
curl -X POST http://localhost:8080/api/v1/subscriptions \
  -H "Content-Type: application/json" \
  -d '{
    "address": "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
    "filters": {"onlySuccess": true},
    "startBlock": -1
  }' | jq .
```

### Monitor a Different Address
```bash
# Edit event_monitor.py line ~454
# Change: address = "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf"
# To: address = "YOUR_TRON_ADDRESS_HERE"

# Then run:
source .venv/bin/activate
python event_monitor.py
```

### Query Events by Block Range
```bash
# This feature requires API enhancement
# Currently, you can filter in MongoDB directly:

mongosh "mongodb://mongotron:MongoTron2025@nileVM.lan:27017/mongotron" --eval '
  db.events.find({
    blockNumber: { $gte: 61090000, $lte: 61100000 }
  }).limit(10).pretty()
'
```

---

## 📊 System Information

### Configuration Files
- **Main Config**: `configs/.env`
- **YAML Config**: `configs/mongotron.yaml`
- **Python Requirements**: `requirements.txt`

### Key Settings
```properties
# From configs/.env
MONGOTRON_PORT=8080
TRON_NODE_HOST=nileVM.lan
TRON_NODE_PORT=50051
MONGODB_URI=mongodb://mongotron:MongoTron2025@nileVM.lan:27017/mongotron
```

### Monitored Contract
**USDT on Nile Testnet**:
- Address: `TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf`
- Type: TRC20 Token
- Network: Tron Nile Testnet
- Explorer: https://nile.tronscan.org/#/contract/TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf

---

## 🔧 Troubleshooting

### Server Won't Start
```bash
# Check if port 8080 is in use
lsof -i :8080

# Check configuration
cat configs/.env | grep -E "TRON_NODE|MONGODB"

# View recent logs
tail -100 /tmp/api-server.log
```

### No Events Showing
```bash
# Check active subscriptions
curl http://localhost:8080/api/v1/subscriptions | jq '.subscriptions[] | select(.status=="active")'

# Verify block processing
curl http://localhost:8080/api/v1/subscriptions | jq '.subscriptions[0].currentBlock'

# Check MongoDB connection
mongosh "mongodb://mongotron:MongoTron2025@nileVM.lan:27017/mongotron" --eval "db.events.count()"
```

### Python Monitor Issues
```bash
# Check Python environment
source .venv/bin/activate
python --version

# Install dependencies
pip install -r requirements.txt

# Test connection
curl http://localhost:8080/api/v1/subscriptions
```

---

## 📚 Documentation

### Available Guides
- ✅ **STARTUP_GUIDE.md** - Complete startup procedures
- ✅ **SERVER_STATUS.md** - Current system status
- ✅ **ADDRESS_CONVERSION_SUCCESS.md** - Human-readable address feature
- ✅ **BUGFIX_DOUBLE_DELETION.md** - Recent bug fixes
- ✅ **EVENT_LOGGING_GUIDE.md** - Event logging details

### Quick Links
```bash
# View all documentation
ls -1 *.md

# Read startup guide
cat STARTUP_GUIDE.md

# Read server status
cat SERVER_STATUS.md
```

---

## 🎓 Tutorial: Watch Your First Event

1. **Open Terminal 1** - Start the monitor:
   ```bash
   cd /home/user0/Github/mongotron
   source .venv/bin/activate
   python event_monitor.py
   ```

2. **Open Terminal 2** - Trigger a transaction (optional):
   ```bash
   # Visit Nile testnet faucet to get test USDT
   # Send USDT to any address
   # Your monitor will capture the transaction!
   ```

3. **Watch Terminal 1** - See the event appear in real-time with:
   - Transaction hash
   - Block number
   - Human-readable addresses
   - Timestamp

4. **Check the Log File**:
   ```bash
   ls -lh events_*.log
   tail events_*.log
   ```

---

## 💡 Pro Tips

### Run Monitor in Background
```bash
cd /home/user0/Github/mongotron
source .venv/bin/activate
nohup python event_monitor.py > /tmp/monitor.log 2>&1 &

# View output
tail -f /tmp/monitor.log
```

### Filter Active Subscriptions
```bash
curl http://localhost:8080/api/v1/subscriptions | \
  jq '.subscriptions[] | select(.status=="active") | {id, events: .eventsCount, block: .currentBlock}'
```

### Export Events to JSON
```bash
curl http://localhost:8080/api/v1/events?limit=100 > events_backup.json
cat events_backup.json | jq '.events | length'
```

---

## 🚀 You're All Set!

Your MongoTron system is **fully operational**:

✅ API Server running on port 8080  
✅ Connected to MongoDB (nileVM.lan)  
✅ Connected to Tron Nile testnet (nileVM.lan)  
✅ 3 active subscriptions monitoring USDT  
✅ 3,538+ events already captured  
✅ Event monitor ready to use  
✅ Human-readable address conversion working  

**Just run**: `python event_monitor.py` to see live events!

---

**Created**: October 6, 2025  
**Status**: ✅ Everything is working!
