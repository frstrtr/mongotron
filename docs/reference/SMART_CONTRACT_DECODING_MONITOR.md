# Smart Contract Decoding in Event Monitor

## Overview

The MongoTron event monitoring system now includes **comprehensive smart contract parameter decoding** that displays transaction details in **human-readable format**. This enhancement makes it easy to understand exactly what functions are being called and what addresses are involved in smart contract interactions.

## What's New

### Backend Enhancements

1. **ABI Decoder Integration in Events**
   - Events now include decoded smart contract information
   - All addresses from function parameters are extracted
   - Function names and signatures are identified
   - Parameter values are decoded and stored

2. **New Parser Method: `DecodeSmartContract()`**
   - Returns `*DecodedCall` with full contract call details
   - Includes: method signature, method name, addresses, amounts, parameters
   - Automatically called during event extraction

3. **Enhanced Event Data Structure**
   - Events now contain `smartContract` field with:
     - `methodSignature`: Hex signature (e.g., "a9059cbb")
     - `methodName`: Human-readable function (e.g., "transfer(address,uint256)")
     - `addresses`: All addresses found in parameters
     - `amount`: Token amount if applicable
     - `parameters`: Other decoded parameters

### Frontend Enhancements (Python Monitor)

1. **New Function: `display_smart_contract_info()`**
   - Displays decoded smart contract calls in human-readable format
   - Converts hex addresses to Tron base58 format (T...)
   - Formats token amounts with proper decimals
   - Shows operation type (Transfer, Approval, etc.)

2. **Enhanced Display Formats**
   - Clear section headers with emojis
   - Both hex and base58 addresses shown
   - Token amounts formatted with decimals
   - Function-specific layouts

## Supported TRC20 Functions

The system recognizes and decodes these standard TRC20 functions:

| Function | Signature | Display Format |
|----------|-----------|----------------|
| `transfer(address,uint256)` | `0xa9059cbb` | 💸 TOKEN TRANSFER |
| `transferFrom(address,address,uint256)` | `0x23b872dd` | 💸 TOKEN TRANSFER (Approved) |
| `approve(address,uint256)` | `0x095ea7b3` | ✅ TOKEN APPROVAL |
| `balanceOf(address)` | `0x70a08231` | 💰 BALANCE QUERY |
| `allowance(address,address)` | `0xdd62ed3e` | 🔍 ALLOWANCE QUERY |
| `decimals()` | `0x313ce567` | Generic display |
| `name()` | `0x06fdde03` | Generic display |
| `symbol()` | `0x95d89b41` | Generic display |
| `totalSupply()` | `0x18160ddd` | Generic display |

## Example Output

### Example 1: Token Transfer

When someone sends USDT to an address:

```
================================================================================
🔔 TRANSACTION EVENT #1 - TriggerSmartContract
================================================================================
📍 TX ID:       a1b2c3d4e5f6...
📦 Block:       61115110
⏰ Time:        2025-10-06 20:09:57
✅ Success:     true

────────────────────────────────────────────────────────────────────────────────
📊 ADDRESSES:
────────────────────────────────────────────────────────────────────────────────
   From (hex):  41c6286105e5e7028a36f5fd2a57cda493fccb54c5
   From:        TSomeAddressInBase58Format
   To (hex):    41eca9bc828a3005b9a3b909f2cc5c2a54794de05f
   To:          TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf (USDT Contract)

────────────────────────────────────────────────────────────────────────────────
🔍 SMART CONTRACT CALL DECODED:
────────────────────────────────────────────────────────────────────────────────
📝 Function:     transfer(address,uint256)
🔑 Signature:    0xa9059cbb

💸 TOKEN TRANSFER:
   To (hex):     41d3682962a219c38a5a39c27d21a6c59a0b1e52f8
   To:           THpjvxomBhvZUodJ3FHFY1szQxAidxejy8
   Amount:       1,000.000000 tokens
   Amount (raw): 1000000000
────────────────────────────────────────────────────────────────────────────────
```

### Example 2: TransferFrom (DEX Transfer)

When a DEX moves tokens between users:

```
────────────────────────────────────────────────────────────────────────────────
🔍 SMART CONTRACT CALL DECODED:
────────────────────────────────────────────────────────────────────────────────
📝 Function:     transferFrom(address,address,uint256)
🔑 Signature:    0x23b872dd

💸 TOKEN TRANSFER (Approved):
   From (hex):   41a1b2c3d4e5f6...
   From:         TUserAddress1
   To (hex):     41b2c3d4e5f6a7...
   To:           TUserAddress2
   Amount:       500.000000 tokens
   Amount (raw): 500000000
────────────────────────────────────────────────────────────────────────────────
```

### Example 3: Token Approval

When approving a spender:

```
────────────────────────────────────────────────────────────────────────────────
🔍 SMART CONTRACT CALL DECODED:
────────────────────────────────────────────────────────────────────────────────
📝 Function:     approve(address,uint256)
🔑 Signature:    0x095ea7b3

✅ TOKEN APPROVAL:
   Spender (hex): 41def1234567890...
   Spender:       TDEXContractAddress
   Allowance:     1,000,000.000000 tokens
   Allowance (raw): 1000000000000000
────────────────────────────────────────────────────────────────────────────────
```

### Example 4: Balance Query

When checking a balance:

```
────────────────────────────────────────────────────────────────────────────────
🔍 SMART CONTRACT CALL DECODED:
────────────────────────────────────────────────────────────────────────────────
📝 Function:     balanceOf(address)
🔑 Signature:    0x70a08231

💰 BALANCE QUERY:
   Owner (hex):  41abc1234567890...
   Owner:        TQueryAddress
────────────────────────────────────────────────────────────────────────────────
```

## How It Works

### Backend Flow

1. **Transaction Detected**
   ```go
   // In address_monitor.go
   func (m *AddressMonitor) extractEvent(...) {
       // ... extract basic transaction data ...
       
       // Decode smart contract if present
       if decoded := m.parser.DecodeSmartContract(contract); decoded != nil {
           event.EventData["smartContract"] = map[string]interface{}{
               "methodSignature": decoded.MethodSignature,
               "methodName":      decoded.MethodName,
               "addresses":       decoded.Addresses,
               "parameters":      decoded.Parameters,
           }
       }
   }
   ```

2. **ABI Decoding**
   ```go
   // In tron_parser.go
   func (p *TronParser) DecodeSmartContract(contract) *DecodedCall {
       // Extract call data from contract
       callData := trigger.GetData()
       
       // Decode using ABI decoder
       return p.abiDecoder.DecodeContractData(callData)
   }
   ```

3. **Parameter Extraction**
   ```go
   // In abi_decoder.go
   func (d *ABIDecoder) decodeTransfer(data []byte) *DecodedCall {
       // Extract recipient address (bytes 4-36)
       toAddress := data[16:36]
       
       // Extract amount (bytes 36-68)
       amount := new(big.Int).SetBytes(data[36:68])
       
       return &DecodedCall{
           MethodName: "transfer(address,uint256)",
           Addresses:  []string{formatTronAddress(toAddress)},
           Amount:     amount,
       }
   }
   ```

### Frontend Flow

1. **Event Received via WebSocket**
   ```python
   def on_ws_message(self, ws, message):
       data = json.loads(message)
       if data.get("type") == "event":
           self.display_transaction_event(data, count)
   ```

2. **Smart Contract Info Extracted**
   ```python
   def display_transaction_event(self, event_data, count):
       # ... display basic transaction info ...
       
       # Check for smart contract data
       smart_contract = event_data.get("EventData", {}).get("smartContract")
       if smart_contract:
           self.display_smart_contract_info(smart_contract)
   ```

3. **Human-Readable Display**
   ```python
   def display_smart_contract_info(self, smart_contract):
       method_name = smart_contract.get("methodName")
       
       if "transfer(address,uint256)" in method_name:
           # Display as token transfer
           recipient = smart_contract["addresses"][0]
           recipient_readable = hex_to_tron_address(recipient)
           amount_formatted = int(smart_contract["amount"]) / 1_000_000
           
           print(f"💸 TOKEN TRANSFER:")
           print(f"   To: {recipient_readable}")
           print(f"   Amount: {amount_formatted:,.6f} tokens")
   ```

## Technical Details

### Address Format Conversion

**Hex to Base58 (T...)**:
```python
def hex_to_tron_address(hex_address: str) -> str:
    # Remove 0x prefix and padding
    hex_address = hex_address.lstrip('0x').lstrip('0')
    
    # Add 41 prefix (Tron mainnet/testnet)
    if not hex_address.startswith('41'):
        hex_address = '41' + hex_address
    
    # Convert to bytes
    addr_bytes = bytes.fromhex(hex_address)
    
    # Add checksum (double SHA256)
    hash1 = hashlib.sha256(addr_bytes).digest()
    hash2 = hashlib.sha256(hash1).digest()
    checksum = hash2[:4]
    
    # Encode to base58
    return base58.b58encode(addr_bytes + checksum).decode('utf-8')
```

### Amount Formatting

**Raw to Human-Readable**:
```python
# USDT has 6 decimals
raw_amount = 1000000000  # From smart contract
formatted = raw_amount / 1_000_000
print(f"{formatted:,.6f} USDT")  # Output: 1,000.000000 USDT
```

### Method Signature Recognition

**Keccak256 Hash**:
```
Function: transfer(address,uint256)
Keccak256: 0xa9059cbb...
First 4 bytes: 0xa9059cbb → Method signature
```

## Usage

### Start Monitoring

```bash
# Activate Python virtual environment
source .venv/bin/activate

# Monitor all events
python event_monitor.py

# Monitor specific address
python event_monitor.py --address THpjvxomBhvZUodJ3FHFY1szQxAidxejy8

# Monitor with filters
python event_monitor.py --filters '{"onlySuccess": true}'
```

### What You'll See

Every smart contract interaction will display:

1. **Transaction Header**
   - Transaction ID
   - Block number
   - Timestamp
   - Success status

2. **Address Section**
   - From/To addresses in both hex and base58
   - Transaction amount (if any)

3. **Smart Contract Decoded Section** (NEW!)
   - Function name and signature
   - Operation type (Transfer, Approval, etc.)
   - All addresses involved with readable format
   - Token amounts with proper decimals
   - Other parameters

4. **Full JSON Data**
   - Complete event data for debugging
   - All raw fields available

## Benefits

### For Users

✅ **Clear Understanding**: Know exactly what function was called
✅ **Address Visibility**: See all addresses in readable format (T...)
✅ **Amount Clarity**: Token amounts with proper decimal formatting
✅ **Operation Context**: Understand the type of transaction (transfer, approval, etc.)

### For Developers

✅ **Complete Data**: Full event data still available in JSON
✅ **Debugging**: Clear visibility into contract interactions
✅ **Integration**: Easy to parse both human-readable and raw data
✅ **Extensibility**: Add new function decoders easily

## Files Modified

### Backend
- `internal/blockchain/parser/tron_parser.go`
  - Added `DecodeSmartContract()` method
  - Returns decoded call information

- `internal/blockchain/monitor/address_monitor.go`
  - Enhanced `extractEvent()` to include decoded data
  - Stores smart contract info in `EventData["smartContract"]`

### Frontend
- `event_monitor.py`
  - Added `display_smart_contract_info()` function
  - Enhanced `display_transaction_event()` to show decoded data
  - Improved address formatting with base58 conversion
  - Added token amount formatting

## Extending Support

### Adding New Functions

To add support for new smart contract functions:

1. **Add signature to `abi_decoder.go`**:
   ```go
   var TRC20Methods = map[string]TRC20Method{
       "newSig": {Signature: "newSig", Name: "newFunction(params)"},
   }
   ```

2. **Add decoder function**:
   ```go
   func (d *ABIDecoder) decodeNewFunction(data []byte) (*DecodedCall, error) {
       // Extract parameters
       // Return DecodedCall
   }
   ```

3. **Add display format in `event_monitor.py`**:
   ```python
   elif "newFunction" in method_name:
       print(f"\n🎯 NEW OPERATION:")
       # Display parameters
   ```

## Date

October 6, 2025

## Status

✅ **DEPLOYED AND ACTIVE**

The enhanced monitoring is now live and processing all transactions with full smart contract parameter decoding!

## Testing

Send a test USDT transaction and watch the monitor display:

```bash
# Start monitoring
source .venv/bin/activate
python event_monitor.py --address YOUR_ADDRESS

# Send USDT from any wallet
# Monitor will show:
# - Transaction details
# - Smart contract function (transfer)
# - Recipient address (in T... format)
# - Amount (formatted with decimals)
```

The system now provides **complete visibility** into all smart contract interactions! 🎉
