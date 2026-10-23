# Smart Contract Decoding - Quick Reference

## What You Get Now

When monitoring events, you'll see **complete smart contract information** including:

✅ Function name (e.g., "transfer", "approve")  
✅ All addresses in readable format (T...)  
✅ Token amounts with proper decimals  
✅ Operation type clearly labeled  

## Quick Start

```bash
# Start monitoring
source .venv/bin/activate
python event_monitor.py
```

## Example Output

### Token Transfer
```
💸 TOKEN TRANSFER:
   To (hex):     4190517a46c051dbe39eaf2418befe71694a570521
   To:           TP8HurAGMeJpx3SXLdK3mV31rEFe7TqUBQ
   Amount:       1,000.000000 tokens
```

### Token Approval
```
✅ TOKEN APPROVAL:
   Spender:      TDEXContractAddress
   Allowance:    1,000,000.000000 tokens
```

### TransferFrom (DEX)
```
💸 TOKEN TRANSFER (Approved):
   From:         TUserAddress1
   To:           TUserAddress2
   Amount:       500.000000 tokens
```

## What Changed

### Before
- ❌ Only saw: `Sender → USDT Contract`
- ❌ Missed recipient address (in parameters)
- ❌ No function information
- ❌ Raw hex addresses only

### After
- ✅ Sees: `Sender → USDT Contract`
- ✅ Decodes: `transfer(Recipient, 1000 USDT)`
- ✅ Shows function name and signature
- ✅ Converts all addresses to T... format

## Supported Functions

| Icon | Function | What It Shows |
|------|----------|---------------|
| 💸 | transfer | Recipient + Amount |
| 💸 | transferFrom | From + To + Amount |
| ✅ | approve | Spender + Allowance |
| 💰 | balanceOf | Account queried |
| 🔍 | allowance | Owner + Spender |

## Test It

```bash
# Display recent events with decoding
python test_smart_contract_display.py
```

## API Access

Events include `smartContract` field:

```bash
curl http://localhost:8080/api/v1/events?limit=1 | jq '.events[0].data.eventData.smartContract'
```

**Returns**:
```json
{
  "methodName": "transfer(address,uint256)",
  "methodSignature": "a9059cbb",
  "addresses": ["41..."],
  "amount": "1000000",
  "parameters": { ... }
}
```

## Documentation

📖 Full guide: `SMART_CONTRACT_DECODING_MONITOR.md`  
📋 Summary: `ENHANCED_MONITOR_SUMMARY.md`  
🚀 Implementation: `ABI_DECODING_IMPLEMENTATION.md`  

## Status

✅ **ACTIVE** - All events now include smart contract decoding!

---

**Date**: October 6, 2025  
**Feature**: Smart Contract Parameter Decoding with Human-Readable Display
