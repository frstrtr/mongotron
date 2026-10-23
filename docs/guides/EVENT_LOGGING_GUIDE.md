# Event Monitor - Detailed Logging Guide

## Overview

The event monitor now provides comprehensive logging of all received events with multiple levels of detail.

## Features

### 1. Console Output - Detailed Event Display

When an event is received, the monitor displays:

#### Event Header
```
================================================================================
🔔 EVENT #1 - Transfer
================================================================================
📍 ID:          evt_abc123...
📄 Contract:    TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf
🔗 TX Hash:     cc86e5c02eb8531c...83dbbff1
📦 Block:       61089041
⏰ Time:        2025-10-05 22:22:10
```

#### Full JSON Data
The complete event data structure in JSON format:
```
────────────────────────────────────────────────────────────────────────────────
📋 FULL EVENT DATA (JSON):
────────────────────────────────────────────────────────────────────────────────
{
  "id": "evt_...",
  "event_name": "Transfer",
  "contract_address": "...",
  "transaction_hash": "...",
  "block_number": 61089041,
  "timestamp": 1759688529,
  "topics": [...],
  "data": "..."
}
```

#### Complete Message Structure
The entire WebSocket message including metadata:
```
────────────────────────────────────────────────────────────────────────────────
📦 COMPLETE MESSAGE STRUCTURE:
────────────────────────────────────────────────────────────────────────────────
{
  "type": "event",
  "data": {
    ...full event details...
  }
}
```

#### Parsed Fields
Human-readable parsed data:
```
────────────────────────────────────────────────────────────────────────────────
📊 PARSED FIELDS:
────────────────────────────────────────────────────────────────────────────────

📋 Topics (3):
   [0] ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef
   [1] 000000000000000000000041d3682962027e721c5247a9faf7865fe4a71d5438
   [2] 000000000000000000000041eca9bc828a3005b9a3b909f2cc5c2a54794de05f

💾 Hex Data:
   0000000000000000000000000000000000000000000000000000000005f5e100
```

#### Decoded Transfer Events
For Transfer events (like USDT), automatic decoding:
```
🔓 DECODED TRANSFER:
   From: 41d3682962027e721c5247a9faf7865fe4a71d5438
   To:   41eca9bc828a3005b9a3b909f2cc5c2a54794de05f
   Amount: 100.000000 USDT
```

### 2. File Logging - Persistent Event Records

All events are automatically saved to a timestamped log file:

**Log File Name Format:**
```
events_YYYYMMDD_HHMMSS.log
```

Example: `events_20251005_222241.log`

**Log File Location:**
- Saved in the current working directory
- Announced at startup: `📝 Logging events to file: events_20251005_222241.log`
- Shown on exit with total count

**Log File Format:**
```
2025-10-05 22:22:41,123 - EVENT #1 - {
  "type": "event",
  "data": {
    "id": "evt_...",
    "event_name": "Transfer",
    ...complete event data...
  }
}
```

Each log entry includes:
- Timestamp (millisecond precision)
- Event number
- Complete JSON structure of the event

### 3. Session Summary

When you stop the monitor (Ctrl+C), you'll see:

```
📊 Session Summary:
   Duration: 0:05:32.123456
   Events received: 15
   Rate: 0.045 events/second

📝 Events saved to: events_20251005_222241.log
   Total events logged: 15
```

## Usage Examples

### Basic Usage (with logging)
```bash
python event_monitor.py
```

This will:
1. Create a timestamped log file
2. Display detailed events in console
3. Save all events to the log file
4. Show summary on exit

### Monitor Specific Address
```bash
python event_monitor.py --address TYourContractAddress123
```

### With Filters
```bash
python event_monitor.py --filters '{"event_name": "Transfer"}'
```

### Review Logged Events
```bash
# View the log file
cat events_20251005_222241.log

# Search for specific events
grep "Transfer" events_20251005_222241.log

# Pretty print JSON
cat events_20251005_222241.log | python -m json.tool

# Count events
grep "EVENT #" events_20251005_222241.log | wc -l
```

## Log File Analysis

### Extract All Transaction Hashes
```bash
grep -oP '"transaction_hash":\s*"\K[^"]+' events_20251005_222241.log
```

### Extract All Block Numbers
```bash
grep -oP '"block_number":\s*\K\d+' events_20251005_222241.log | sort -n
```

### Find Events from Specific Address
```bash
grep "41d3682962027e721c5247a9faf7865fe4a71d5438" events_20251005_222241.log
```

### Count Events by Type
```bash
grep -oP '"event_name":\s*"\K[^"]+' events_20251005_222241.log | sort | uniq -c
```

## Benefits

### Console Output Benefits
1. **Immediate visibility** - See events as they happen
2. **Multiple detail levels** - From summary to raw JSON
3. **Human-readable parsing** - Decoded Transfer amounts
4. **Color coding** - Easy to spot different sections

### File Logging Benefits
1. **Permanent record** - Events saved for later analysis
2. **Machine-readable** - JSON format for easy parsing
3. **Timestamped** - Exact timing of each event
4. **Searchable** - Use grep, jq, or custom tools
5. **Audit trail** - Complete history of monitored events

## Advanced: Processing Log Files with Python

```python
import json
import re

# Read and parse events from log file
def parse_event_log(filename):
    events = []
    with open(filename, 'r') as f:
        content = f.read()
        # Extract JSON events
        pattern = r'EVENT #\d+ - ({.*?})\n(?=\d{4}-\d{2}-\d{2}|$)'
        matches = re.findall(pattern, content, re.DOTALL)
        for match in matches:
            try:
                event = json.loads(match)
                events.append(event)
            except:
                pass
    return events

# Example usage
events = parse_event_log('events_20251005_222241.log')
print(f"Total events: {len(events)}")

# Analyze Transfer events
transfers = [e for e in events if e.get('data', {}).get('event_name') == 'Transfer']
print(f"Transfer events: {len(transfers)}")
```

## Troubleshooting

### Log File Not Created
- Check write permissions in current directory
- Ensure Python has access to create files
- Check disk space

### Large Log Files
- Log files grow with each event
- Rotate log files if running for extended periods
- Use compression for archived logs: `gzip events_*.log`

### Parsing Issues
- Ensure JSON is valid
- Use `jq` for safe JSON parsing: `jq . < events_20251005_222241.log`
- Check for line breaks in the middle of JSON objects

## Date

October 5, 2025
