# MongoTron Event Monitor

A real-time blockchain event monitoring tool that subscribes to contract addresses and displays events as they occur.

## Features

- 🔔 Real-time event notifications via WebSocket
- 📊 Formatted event display with decoded data
- 💾 Automatic Transfer event decoding (for tokens like USDT)
- 🎯 Customizable filters
- 🧹 Automatic cleanup on exit
- ⌨️ Graceful shutdown with Ctrl+C

## Installation

Dependencies are already included in `requirements.txt`:

```bash
# If not already in venv
source venv/bin/activate

# Dependencies should already be installed
pip install -r requirements.txt
```

## Usage

### Basic Usage - Monitor USDT Contract

Monitor the USDT contract on Tron Nile testnet (default):

```bash
python event_monitor.py
```

**Default Address**: `TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf` (USDT on Tron Nile Testnet)

### Monitor a Specific Address

```bash
python event_monitor.py --address TRX9aKj8VrqEJMvM7vvkA2B3wYGpwj5YSj
```

### Connect to Different Server

```bash
python event_monitor.py --url http://staging-server:8080
```

### With Filters

Monitor only Transfer events:

```bash
python event_monitor.py --filters '{"event_name": "Transfer"}'
```

Monitor only successful transactions:

```bash
python event_monitor.py --filters '{"onlySuccess": true}'
```

## Command Line Options

| Option | Description | Default |
|--------|-------------|---------|
| `--address` | Contract address to monitor | USDT Nile: TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf |
| `--url` | MongoTron API base URL | http://localhost:8080 |
| `--filters` | Event filters as JSON string | None |

## Example Output

```
================================================================================
                            MongoTron Event Monitor                             
================================================================================

📋 Creating subscription for address: TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t
✅ Subscription created: sub_6da7ac49-9e6
   Network: tron-nile
   Active: active

🔌 Connecting to WebSocket...
   URL: ws://localhost:8080/api/v1/events/stream/sub_6da7ac49-9e6

🔌 WebSocket connected!
⏰ Started at: 2025-10-05 21:55:02

================================================================================
-------------------------------MONITORING EVENTS--------------------------------
================================================================================

✅ Connection confirmed by server
   Subscription: sub_6da7ac49-9e6

────────────────────────────────────────────────────────────────────────────────
🔔 EVENT #1 - Transfer
────────────────────────────────────────────────────────────────────────────────
📍 ID:          evt_12345678
📄 Contract:    TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf
🔗 TX Hash:     a1b2c3d4e5f6...9876543210
📦 Block:       45678901
⏰ Time:        2025-10-05 21:55:15

📋 Topics (3):
   [0] ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef
   [1] 000000000000000000000041a1b2c3d4e5f6...
   [2] 000000000000000000000041f6e5d4c3b2a1...

💾 Data:
   0000000000000000000000000000000000000000000000000000000005f5e100

📊 Decoded Transfer:
   From: 000000000000000000000041a1b2c3d4e5f6...
   To:   000000000000000000000041f6e5d4c3b2a1...
   Amount: 100.000000 USDT
────────────────────────────────────────────────────────────────────────────────


💡 Press Ctrl+C to stop monitoring
```

## Event Display Format

Each event shows:

- **Event Number**: Sequential number of events received
- **Event Name**: Type of event (Transfer, Approval, etc.)
- **ID**: Unique event identifier
- **Contract**: Contract address that emitted the event
- **TX Hash**: Transaction hash (truncated for readability)
- **Block**: Block number where event was recorded
- **Time**: Timestamp of the event
- **Topics**: Event topics (indexed parameters)
- **Data**: Event data (non-indexed parameters)
- **Decoded Info**: Automatically decoded information (e.g., Transfer amounts)

## Transfer Event Decoding

For Transfer events, the monitor automatically decodes:

- **From**: Sender address
- **To**: Recipient address
- **Amount**: Transfer amount (for USDT: divided by 1,000,000 for 6 decimals)

## Filters

Filters can be provided as JSON:

### Available Filter Options

```json
{
  "event_name": "Transfer",       // Specific event name
  "onlySuccess": true,            // Only successful transactions
  "minAmount": 1000000000000,     // Minimum amount (in base units)
  "maxAmount": 10000000000000     // Maximum amount (in base units)
}
```

### Filter Examples

Only Transfer events:
```bash
python event_monitor.py --filters '{"event_name": "Transfer"}'
```

Only successful transactions:
```bash
python event_monitor.py --filters '{"onlySuccess": true}'
```

## Session Statistics

When you stop monitoring (Ctrl+C), the tool displays:

- Total duration
- Number of events received
- Event rate (events per second)

```
📊 Session Summary:
   Duration: 0:15:42
   Events received: 127
   Rate: 0.13 events/second
```

## Graceful Shutdown

The monitor handles shutdown gracefully:

1. Catches Ctrl+C (SIGINT) and SIGTERM signals
2. Closes WebSocket connection
3. Deletes the subscription from the server
4. Displays session statistics

## Troubleshooting

### Connection Refused

```bash
# Make sure the API server is running
ps aux | grep api-server

# Check server health
curl http://localhost:8080/api/v1/health
```

### No Events Received

This is normal if:
- No transactions are happening on the monitored address
- The blockchain is idle
- You're monitoring a testnet with low activity

Try monitoring a high-activity contract like USDT to see more events.

### WebSocket Disconnect

If the WebSocket disconnects:
- Check server logs: `tail -f server.log`
- Verify network connectivity
- Restart the monitor

## Integration with MongoTron

This tool works with the MongoTron API server and requires:

1. **API Server**: Running on specified URL (default: localhost:8080)
2. **MongoDB**: Connected and operational (on nileVM.lan)
3. **Tron Node**: Connected for blockchain data (on nileVM.lan)

## Technical Details

### WebSocket Protocol

- Connects to: `ws://[host]/api/v1/events/stream/[subscription_id]`
- Receives JSON-formatted messages
- Message types: `connected`, `event`, `error`

### Subscription Lifecycle

1. Create subscription via REST API (POST /api/v1/subscriptions)
2. Connect WebSocket with subscription ID
3. Receive real-time events
4. Delete subscription on exit (DELETE /api/v1/subscriptions/:id)

### Event Format

Events follow the MongoTron event schema:
- ID, timestamp, block number
- Contract address, transaction hash
- Topics (indexed parameters)
- Data (non-indexed parameters)
- Event name and additional metadata

## Advanced Usage

### Background Monitoring

Run in background and log to file:

```bash
nohup python event_monitor.py --address YOUR_ADDRESS > events.log 2>&1 &
```

### Multiple Monitors

Run multiple monitors for different addresses:

```bash
# Terminal 1 - USDT
python event_monitor.py

# Terminal 2 - Another contract
python event_monitor.py --address TRX9aKj8VrqEJMvM7vvkA2B3wYGpwj5YSj
```

### Custom Processing

Modify the `display_event()` method to:
- Save events to a database
- Send notifications
- Trigger automated actions
- Export to CSV/JSON

## Common Contract Addresses (Tron Nile Testnet)

- **USDT**: TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf (default - Nile Testnet)
- **USDT Mainnet**: TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t (for reference)
- **TRX**: Native token (no contract address)
- Add your own contract addresses here

**Note**: MongoTron is currently configured to connect to Tron Nile testnet on `nileVM.lan:50051`.

## License

Part of the MongoTron project.
