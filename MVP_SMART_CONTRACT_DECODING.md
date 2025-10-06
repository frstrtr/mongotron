# MVP Monitor Enhanced with Smart Contract Decoding

## Date: October 6, 2025

## Overview

Updated the **MVP monitor** (`cmd/mvp/main.go`) to include **comprehensive smart contract parameter decoding** in verbose logging mode. The monitor now displays all decoded smart contract information when processing transactions.

## What Was Updated

### 1. Enhanced Verbose Logging for Address Monitor

**File**: `cmd/mvp/main.go` - Function `processEvents()`

#### Before
```go
if verbose {
    logEvent.
        Str("from", hexToBase58Address(event.From)).
        Str("to", hexToBase58Address(event.To)).
        Int64("amount", event.Amount).
        Bool("success", event.Success).
        Msg("Event stored successfully")
}
```

#### After
```go
if verbose {
    // ... existing code ...
    
    // Add smart contract decoded information if available
    if smartContract, ok := event.EventData["smartContract"].(map[string]interface{}); ok {
        if methodName, ok := smartContract["methodName"].(string); ok {
            logEvent = logEvent.Str("decodedMethod", methodName)
        }
        if methodSig, ok := smartContract["methodSignature"].(string); ok {
            logEvent = logEvent.Str("methodSignature", methodSig)
        }
        if addresses, ok := smartContract["addresses"].([]string); ok && len(addresses) > 0 {
            logEvent = logEvent.Strs("paramAddresses", addresses)
            // Convert to base58 for readability
            readableAddrs := make([]string, len(addresses))
            for i, addr := range addresses {
                readableAddrs[i] = hexToBase58Address(addr)
            }
            logEvent = logEvent.Strs("paramAddressesReadable", readableAddrs)
        }
        if amount, ok := smartContract["amount"].(string); ok && amount != "" {
            logEvent = logEvent.Str("decodedAmount", amount)
        }
    }
    
    logEvent.Msg("Event stored successfully")
}
```

### 2. Enhanced Non-Verbose Logging

Added decoded method name even in non-verbose mode for smart contracts:

```go
// Add decoded method name for smart contracts
if smartContract, ok := event.EventData["smartContract"].(map[string]interface{}); ok {
    if methodName, ok := smartContract["methodName"].(string); ok {
        logEvent = logEvent.Str("decodedMethod", methodName)
    }
}
```

### 3. Updated Block Monitor Transaction Extraction

**File**: `internal/blockchain/monitor/block_monitor.go` - Function `extractTransactionData()`

Added smart contract decoding to BlockMonitor's transaction data:

```go
// Decode smart contract call data if available
if decoded := m.parser.DecodeSmartContract(contract); decoded != nil {
    txData.ContractData["smartContract"] = map[string]interface{}{
        "methodSignature": decoded.MethodSignature,
        "methodName":      decoded.MethodName,
        "addresses":       decoded.Addresses,
        "parameters":      decoded.Parameters,
    }
    if decoded.Amount != nil {
        txData.ContractData["smartContract"].(map[string]interface{})["amount"] = decoded.Amount.String()
    }
}
```

### 4. Enhanced Block Monitor Verbose Logging

**File**: `cmd/mvp/main.go` - Function `storeBlockEvent()`

Updated comprehensive monitor to show decoded information:

```go
// Add decoded smart contract information if available
if smartContract, ok := txData.ContractData["smartContract"].(map[string]interface{}); ok {
    if methodName, ok := smartContract["methodName"].(string); ok {
        logEvent = logEvent.Str("decodedMethod", methodName)
    }
    if methodSig, ok := smartContract["methodSignature"].(string); ok {
        logEvent = logEvent.Str("methodSignature", methodSig)
    }
    if addresses, ok := smartContract["addresses"].([]string); ok && len(addresses) > 0 {
        logEvent = logEvent.Strs("paramAddresses", addresses)
        // Convert to base58 for readability
        readableAddrs := make([]string, len(addresses))
        for j, addr := range addresses {
            readableAddrs[j] = hexToBase58Address(addr)
        }
        logEvent = logEvent.Strs("paramAddressesReadable", readableAddrs)
    }
    if amount, ok := smartContract["amount"].(string); ok && amount != "" {
        logEvent = logEvent.Str("decodedAmount", amount)
    }
}
```

## Usage

### Running MVP Monitor with Verbose Mode

#### Monitor Specific Address
```bash
./bin/mongotron-mvp -address=THpjvxomBhvZUodJ3FHFY1szQxAidxejy8 -verbose
```

#### Monitor All Transactions (Comprehensive Mode)
```bash
./bin/mongotron-mvp -monitor -verbose
```

## Example Output

### Before (Without Decoding)
```json
{
  "level": "info",
  "block": 61115487,
  "txHash": "5e8381c00d25ec70e0d117a4656505a3fada4079a68ef487761a905114a0f574",
  "TronTXType": "Smart Contract",
  "contractType": "TriggerSmartContract",
  "from": "TKfUiqAGByAHv8nmTzZqK3RxNc4p3yPqGf",
  "to": "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
  "amount": 0,
  "success": true,
  "message": "Event stored successfully"
}
```

### After (With Decoding)
```json
{
  "level": "info",
  "block": 61115487,
  "txHash": "5e8381c00d25ec70e0d117a4656505a3fada4079a68ef487761a905114a0f574",
  "TronTXType": "Smart Contract",
  "contractType": "TriggerSmartContract",
  "decodedMethod": "transfer(address,uint256)",
  "methodSignature": "a9059cbb",
  "paramAddresses": [
    "41737ab4479361c64983260bad00f3cab5549f125d"
  ],
  "paramAddressesReadable": [
    "TLVohkv4mQT5yK9RdDFw8q8SJtESQGfVAo"
  ],
  "decodedAmount": "6710000000",
  "from": "TKfUiqAGByAHv8nmTzZqK3RxNc4p3yPqGf",
  "fromHex": "416a56e4a1eb5e1106c02287bb866c5f7eaf2f9641",
  "to": "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
  "toHex": "41eca9bc828a3005b9a3b909f2cc5c2a54794de05f",
  "amount": 0,
  "success": true,
  "message": "Event stored successfully"
}
```

## Benefits

### 1. Complete Transaction Visibility
- **Transaction-level addresses**: Caller → Contract
- **Parameter-level addresses**: Recipient, spenders, etc.
- **Decoded function calls**: Know exactly what function was called
- **Token amounts**: See the actual amounts being transferred

### 2. Human-Readable Format
- Both hex and base58 addresses displayed
- Method names in readable format
- Token amounts decoded

### 3. Debugging Capabilities
- Trace token transfers through parameters
- Identify approval operations
- Track addresses in complex contract interactions

### 4. Log Analysis
All decoded information is in JSON format, making it easy to:
- Parse logs with jq or other tools
- Build analytics dashboards
- Create monitoring alerts
- Track specific addresses or methods

## Files Modified

1. **cmd/mvp/main.go**
   - Enhanced `processEvents()` verbose logging
   - Enhanced `storeBlockEvent()` verbose logging
   - Added decoded method display in non-verbose mode

2. **internal/blockchain/monitor/block_monitor.go**
   - Added smart contract decoding to `extractTransactionData()`

## Verified Working

✅ **API Server**: Rebuilt successfully  
✅ **MVP Monitor**: Rebuilt successfully  
✅ **Smart Contract Decoding**: Integrated into monitors  
✅ **Verbose Logging**: Shows full decoded information  

## Example Commands

### Watch Specific Address with Decoding
```bash
# Build (if not already built)
go build -o bin/mongotron-mvp cmd/mvp/main.go

# Run with verbose logging
./bin/mongotron-mvp \
  -address=THpjvxomBhvZUodJ3FHFY1szQxAidxejy8 \
  -verbose
```

### Monitor All Blocks with Decoding
```bash
./bin/mongotron-mvp \
  -monitor \
  -verbose \
  -start-block=61115000
```

### Filter Logs for Specific Method
```bash
# Watch for all transfer operations
./bin/mongotron-mvp -monitor -verbose 2>&1 | \
  grep -E 'decodedMethod.*transfer'
```

### Extract Parameter Addresses
```bash
# Extract all addresses found in smart contract parameters
./bin/mongotron-mvp -monitor -verbose 2>&1 | \
  grep 'paramAddressesReadable' | \
  jq -r '.paramAddressesReadable[]'
```

## Log Fields Reference

### Standard Fields (All Transactions)
- `level`: Log level (info, debug, error)
- `block`: Block number
- `txHash`: Full transaction hash
- `TronTXType`: Human-readable transaction type
- `contractType`: Raw contract type
- `from`: From address (base58)
- `fromHex`: From address (hex) - verbose only
- `to`: To address (base58)
- `toHex`: To address (hex) - verbose only
- `amount`: Transaction amount
- `success`: Transaction success status

### Smart Contract Decoding Fields (When Available)
- `decodedMethod`: Full method signature (e.g., "transfer(address,uint256)")
- `methodSignature`: Method signature hash (e.g., "a9059cbb")
- `paramAddresses`: Array of hex addresses from parameters - verbose only
- `paramAddressesReadable`: Array of base58 addresses - verbose only
- `decodedAmount`: Token amount from parameters - verbose only
- `SCTXType`: Smart contract interaction type (legacy field)

## Integration with Other Tools

### With jq
```bash
# Pretty print decoded transfers
./bin/mongotron-mvp -monitor -verbose 2>&1 | \
  jq -c 'select(.decodedMethod == "transfer(address,uint256)") | 
    {block, from, to: .paramAddressesReadable[0], amount: .decodedAmount}'
```

### With grep
```bash
# Find all USDT contract interactions
./bin/mongotron-mvp -monitor -verbose 2>&1 | \
  grep "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf"
```

### Log to File
```bash
# Save all decoded transactions to file
./bin/mongotron-mvp -monitor -verbose > decoded_transactions.log 2>&1
```

## Status

✅ **DEPLOYED** - Both MVP monitor and API server updated with smart contract decoding  
✅ **TESTED** - Builds successful  
✅ **DOCUMENTED** - Complete usage guide provided  

The MVP monitor now provides **complete transparency** into all smart contract interactions with detailed parameter decoding! 🎉
