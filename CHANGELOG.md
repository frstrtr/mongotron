# MongoTron Changelog

All notable changes to MongoTron will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.1.0-mvp] - 2025-10-05

### 🎉 Initial MVP Release

First working version of MongoTron with complete blockchain monitoring capabilities.

**Release Highlights:**
- ✅ Dual-mode monitoring (single address + comprehensive block scanning)
- ✅ Smart contract ABI decoder with 60+ fallback method signatures
- ✅ Dual transaction type logging (TronTXType + SCTXType)
- ✅ 50+ native Tron transaction types supported
- ✅ Base58 address formatting for human readability
- ✅ 370+ blocks/minute processing speed
- ✅ MongoDB integration with full persistence

**Quick Start:**
```bash
# Build
go build -o bin/mongotron-mvp ./cmd/mvp/main.go

# Run comprehensive monitoring
./bin/mongotron-mvp --monitor --verbose --start-block=0

# Run single address monitoring
./bin/mongotron-mvp --address=TMCwUb3kxj7BFvmuxRntq6YfDEi9FeDy4M --verbose
```

### Added

#### 1. Dual-Mode Monitoring
- **Single Address Mode** (`--address`): Monitor specific address activity
- **Comprehensive Block Mode** (`--monitor`): Monitor all addresses in every block
- Configurable start block with `--start-block` flag
- 3-second polling interval for real-time monitoring

#### 2. Verbose Logging System
- **`--verbose` flag**: Displays all parsed data before storage
- Block information with timestamp and transaction count
- Transaction details with addresses in Base58 format
- Smart contract interaction decoding
- Event logs and internal transactions

#### 3. Address Format Enhancement
- **Base58 Encoding**: All addresses displayed in human-readable Tron format
  - Example: `TMCwUb3kxj7BFvmuxRntq6YfDEi9FeDy4M`
- Hex format preserved internally for efficiency
- Automatic conversion in all log outputs

#### 4. Transaction Type Classification
- **50+ Native Tron Transaction Types** including:
  - Transfer (TRX) - Native TRX transfers
  - Transfer (TRC10) - TRC10 token transfers
  - Smart Contract - Smart contract interactions
  - Vote Witness - Witness voting
  - Delegate Resource - Resource delegation
  - Create Account - Account creation
  - Stake (v2) - Energy/bandwidth staking
  - Unstake (v2) - Energy/bandwidth unstaking
  - Withdraw Expire Unfreeze - Claim unstaked resources
  - Undelegate Resource - Remove resource delegation
  - And 40+ more contract types

#### 5. Smart Contract ABI Decoder
- **Automatic ABI Fetching**: Uses GetContract API to fetch ABIs from Tron network
- **ABI Caching**: Caches loaded ABIs in memory for performance
- **Method Decoding**: Decodes smart contract method calls from input data
- **60+ Common Method Signatures** fallback including:
  - **ERC20 Tokens**: transfer, approve, transferFrom, mint, burn, increaseAllowance, decreaseAllowance
  - **DEX Operations**: 
    - Swaps: swapExactTokensForTokens, swapTokensForExactTokens, swapExactETHForTokens, swapExactTokensForETH
    - Liquidity: addLiquidity, removeLiquidity, addLiquidityETH, removeLiquidityETH
  - **Staking**: stake, unstake, claim, getReward, withdraw
  - **NFTs**: safeTransferFrom, transferFrom, burn, ownerOf, setApprovalForAll, approve
  - **Governance**: vote, propose, execute, delegate
  - **And many more...**
- **Human-Readable Types**: Maps method names to readable strings
  - "transfer" → "Token Transfer"
  - "swapExactTokensForTokens" → "Swap Exact Tokens"
  - "stake" → "Stake Tokens"

#### 6. Dual Transaction Type Logging
**Major Enhancement**: Clear distinction between native Tron transactions and smart contract interactions

- **TronTXType**: Native blockchain transaction type (always present)
  - Shows the actual blockchain transaction type
  - Examples: "Transfer (TRX)", "Smart Contract", "Vote Witness"
  
- **SCTXType**: Decoded smart contract interaction type (conditional)
  - Only shown when smart contract method is successfully decoded
  - Examples: "Token Transfer", "Swap Tokens", "Stake Tokens"
  - Shows "Unknown Method (0xSIGNATURE)" for unrecognized methods

**Example Logs**:
```
TronTXType="Transfer (TRX)"  
→ Native TRX transfer

TronTXType="Smart Contract" SCTXType="Token Transfer"
→ Smart contract with decoded token transfer

TronTXType="Smart Contract" SCTXType="Unknown Method (0xb9f412b0)"
→ Smart contract with unrecognized method
```

### Technical Details

#### Architecture
- **Go 1.24.0**: Core language
- **fbsobreira/gotron-sdk v0.24.1**: Tron protocol integration
- **MongoDB 7.0.25**: Data persistence
- **ethereum/go-ethereum**: ABI parsing (compatible with Tron)
### Technical Details

#### Architecture
- **Language**: Go 1.24.0
- **Blockchain SDK**: fbsobreira/gotron-sdk v0.24.1
- **Database**: MongoDB 7.0.25 on nileVM.lan:27017
- **Blockchain Node**: Tron Nile Testnet v4.8.0 (nileVM.lan:50051)
- **ABI Parsing**: ethereum/go-ethereum v1.15.6
- **Logging**: rs/zerolog (structured JSON logging)

#### Performance Metrics
- **Block Processing**: 370+ blocks per minute
- **Binary Size**: 27MB (standalone executable)
- **Memory Usage**: Efficient with in-memory ABI caching
- **ABI Fetch Timeout**: 5 seconds per contract
- **Polling Interval**: 3 seconds

#### Database Collections
- **blocks**: Block metadata with transaction references
- **transactions**: Complete transaction data with decoded types
- **addresses**: Address activity tracking and statistics
- **events**: Single-address mode event history

### Configuration

**Default Settings:**
- MongoDB: `nileVM.lan:27017` (database: `mongotron`)
- Tron Node: `nileVM.lan:50051` (Nile Testnet v4.8.0)
- Poll Interval: 3 seconds
- Network: Tron Nile Testnet

### Usage Examples

**Monitor Single Address:**
```bash
./bin/mongotron-mvp --address=TMCwUb3kxj7BFvmuxRntq6YfDEi9FeDy4M --verbose
```

**Monitor All Addresses (Comprehensive Block Mode):**
```bash
./bin/mongotron-mvp --monitor --verbose --start-block=0
```

**Monitor from Specific Block:**
```bash
./bin/mongotron-mvp --monitor --start-block=61082220
```

**Sample Output:**
```
4:51PM INF Transaction in block TronTXType="Transfer (TRX)" amount=39137 
       from=TMCwUb3kxj7BFvmuxRntq6YfDEi9FeDy4M 
       to=TCxqcZtbq3hijr7MEoTRqSYZd1jGGmJBt9

4:51PM INF Transaction in block TronTXType="Smart Contract" SCTXType="Token Transfer"
       amount=0 from=TQ3YZ56STTXqe3MmcZcitPkBWDGcU7EcAh 
       to=TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf
```

### Known Issues

- Some proprietary smart contract methods show as "Unknown Method (0xSIGNATURE)"
- ABI fetching limited to 5-second timeout (prevents hanging)
- Currently configured for Nile Testnet only (Mainnet support pending)

### Fixed

- Smart contract data extraction from protobuf `*anypb.Any` type
- Dual transaction type logging (TronTXType vs SCTXType)
- ABI decoder initialization and caching
- Base58 address conversion in all log outputs

### Changed

- Logging format: Split transaction type into TronTXType and SCTXType fields
- Smart contract parameter extraction: Now properly handles protobuf Any type
- Import structure: Added `google.golang.org/protobuf/types/known/anypb`

---

## [Unreleased]

### Planned Features

#### High Priority
- [ ] Mainnet configuration support
- [ ] WebSocket API for real-time event streaming
- [ ] REST API for historical data queries
- [ ] Custom ABI repository for popular contracts
- [ ] Configurable MongoDB and Tron node endpoints

#### Medium Priority
- [ ] Performance optimizations for 1000+ blocks/min
- [ ] Enhanced error handling and retry mechanisms
- [ ] Rate limiting for ABI fetching
- [ ] Transaction receipt parsing improvements
- [ ] Multi-network support (Mainnet, Shasta, Nile)

#### Low Priority
- [ ] Web dashboard for monitoring
- [ ] GraphQL API
- [ ] Prometheus metrics export
- [ ] Docker containerization
- [ ] Kubernetes deployment manifests

---

## Development Guide

### Building from Source

```bash
# Clone repository
git clone https://github.com/frstrtr/mongotron.git
cd mongotron

# Install dependencies
go mod download

# Build MVP
go build -o bin/mongotron-mvp ./cmd/mvp/main.go

# Run
./bin/mongotron-mvp --monitor --verbose
```

### Dependencies

**Core Dependencies:**
```bash
go get github.com/fbsobreira/gotron-sdk@v0.24.1
go get github.com/ethereum/go-ethereum@v1.15.6
go get go.mongodb.org/mongo-driver/mongo@v1.17.1
go get github.com/rs/zerolog@v1.33.0
go get google.golang.org/protobuf@v1.36.0
```

### Testing

**Tested Scenarios:**
- ✅ Tron Nile testnet blocks 61000000+
- ✅ Native TRX transfers
- ✅ Smart contract interactions (TRC20 tokens)
- ✅ Token transfers with successful decoding
- ✅ Unknown smart contract methods handling
- ✅ ABI fetching and caching
- ✅ Dual transaction type logging

**Test Command:**
```bash
# Test comprehensive mode
timeout 120s ./bin/mongotron-mvp --monitor --verbose --start-block=61082220

# Test single address mode
./bin/mongotron-mvp --address=TMCwUb3kxj7BFvmuxRntq6YfDEi9FeDy4M --verbose
```

---

## Version History

### v0.1.0-mvp (2025-10-05)
- Initial MVP release with dual-mode monitoring
- Smart contract ABI decoder with 60+ fallback signatures
- Dual transaction type logging (TronTXType + SCTXType)
- 50+ native Tron transaction types
- Base58 address formatting
- MongoDB integration
- 370+ blocks/min processing speed

---

## Migration Guide

### From Development to MVP

No migration needed - this is the initial release.

### Future Mainnet Migration

When migrating to mainnet:
1. Update Tron node endpoint in `cmd/mvp/main.go`
2. Update MongoDB connection string if needed
3. Rebuild: `go build -o bin/mongotron-mvp ./cmd/mvp/main.go`
4. Test with `--start-block` set to recent block
5. Monitor initial sync for any issues

---

## Contributing

This is currently an MVP. Contributions welcome for:
- Additional smart contract method signatures
- Performance improvements
- Bug fixes
- Documentation improvements

---

## License

See LICENSE file for details.

---

*For more information, see [README.md](README.md)*### Testing
The system has been tested with:
- Tron Nile testnet blocks 61000000+
- Various transaction types including transfers, smart contracts, staking
- Token transfers (TRC20) with successful decoding
- Multiple unknown smart contract methods

---

For more information, see README.md
