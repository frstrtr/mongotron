package parser

import (
	"encoding/hex"
	"math/big"

	"github.com/fbsobreira/gotron-sdk/pkg/common"
)

// Known USDT contract addresses on different networks
var (
	// Mainnet USDT (TRC20)
	USDTMainnet = "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"
	// Nile Testnet USDT (TRC20)
	USDTNile = "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf"
	// Shasta Testnet USDT
	USDTShasta = "TG3XXyExBkPp9nzdajDZsozEu4BkaSJozs"

	// Hex versions (41 prefix)
	USDTMainnetHex string
	USDTNileHex    string
	USDTShastaHex  string
)

func init() {
	// Pre-compute hex addresses
	USDTMainnetHex = base58ToHex(USDTMainnet)
	USDTNileHex = base58ToHex(USDTNile)
	USDTShastaHex = base58ToHex(USDTShasta)
}

// TRC20Transfer represents a decoded TRC20 token transfer
type TRC20Transfer struct {
	ContractAddress    string   `json:"contractAddress"`    // Token contract address (e.g., USDT contract)
	ContractAddressHex string   `json:"contractAddressHex"` // Hex format of contract address
	TokenSymbol        string   `json:"tokenSymbol"`        // Token symbol (USDT, USDC, etc.)
	TokenDecimals      int      `json:"tokenDecimals"`      // Token decimals (usually 6 for USDT)
	From               string   `json:"from"`               // Sender address (base58)
	FromHex            string   `json:"fromHex"`            // Sender address (hex)
	To                 string   `json:"to"`                 // Recipient address (base58)
	ToHex              string   `json:"toHex"`              // Recipient address (hex)
	Amount             *big.Int `json:"amount"`             // Raw amount (in smallest unit)
	AmountDecimal      string   `json:"amountDecimal"`      // Human-readable amount with decimals
	MethodType         string   `json:"methodType"`         // "transfer" or "transferFrom"
	TxHash             string   `json:"txHash"`             // Transaction hash
	BlockNumber        int64    `json:"blockNumber"`        // Block number
	BlockTimestamp     int64    `json:"blockTimestamp"`     // Block timestamp
	Success            bool     `json:"success"`            // Transaction success status
}

// TRC20Parser handles TRC20 token transfer parsing
type TRC20Parser struct {
	decoder *ABIDecoder
}

// NewTRC20Parser creates a new TRC20 parser
func NewTRC20Parser() *TRC20Parser {
	return &TRC20Parser{
		decoder: NewABIDecoder(),
	}
}

// ParseTransfer parses TRC20 transfer from contract call data
// Returns nil if not a transfer or transferFrom call
func (p *TRC20Parser) ParseTransfer(contractAddress string, data []byte) *TRC20Transfer {
	if len(data) < 4 {
		return nil
	}

	methodSig := hex.EncodeToString(data[:4])

	// Only handle transfer and transferFrom
	if methodSig != "a9059cbb" && methodSig != "23b872dd" {
		return nil
	}

	decoded, err := p.decoder.DecodeContractData(data)
	if err != nil {
		return nil
	}

	transfer := &TRC20Transfer{
		ContractAddress:    contractAddress,
		ContractAddressHex: base58ToHex(contractAddress),
		Amount:             decoded.Amount,
	}

	// Determine token info based on contract address
	transfer.TokenSymbol, transfer.TokenDecimals = p.getTokenInfo(contractAddress)

	// Calculate decimal amount
	if transfer.Amount != nil && transfer.TokenDecimals > 0 {
		transfer.AmountDecimal = formatTokenAmount(transfer.Amount, transfer.TokenDecimals)
	}

	switch methodSig {
	case "a9059cbb": // transfer(address,uint256)
		transfer.MethodType = "transfer"
		if to, ok := decoded.Parameters["to"].(string); ok {
			transfer.ToHex = to
			transfer.To = hexToBase58(to)
		}

	case "23b872dd": // transferFrom(address,address,uint256)
		transfer.MethodType = "transferFrom"
		if from, ok := decoded.Parameters["from"].(string); ok {
			transfer.FromHex = from
			transfer.From = hexToBase58(from)
		}
		if to, ok := decoded.Parameters["to"].(string); ok {
			transfer.ToHex = to
			transfer.To = hexToBase58(to)
		}
	}

	return transfer
}

// IsUSDTContract checks if the contract address is a known USDT contract
func (p *TRC20Parser) IsUSDTContract(address string) bool {
	// Check base58 addresses
	if address == USDTMainnet || address == USDTNile || address == USDTShasta {
		return true
	}
	// Check hex addresses
	if address == USDTMainnetHex || address == USDTNileHex || address == USDTShastaHex {
		return true
	}
	return false
}

// IsTransferMethod checks if the method signature is a transfer or transferFrom
func (p *TRC20Parser) IsTransferMethod(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	methodSig := hex.EncodeToString(data[:4])
	return methodSig == "a9059cbb" || methodSig == "23b872dd"
}

// getTokenInfo returns token symbol and decimals for known contracts
func (p *TRC20Parser) getTokenInfo(contractAddress string) (string, int) {
	// Normalize to base58 if hex
	if len(contractAddress) == 42 && contractAddress[:2] == "41" {
		contractAddress = hexToBase58(contractAddress)
	}

	switch contractAddress {
	case USDTMainnet, USDTNile, USDTShasta:
		return "USDT", 6
	// Add more tokens as needed
	default:
		return "TRC20", 18 // Default for unknown tokens
	}
}

// formatTokenAmount formats raw amount with decimals
func formatTokenAmount(amount *big.Int, decimals int) string {
	if amount == nil {
		return "0"
	}

	// Create divisor (10^decimals)
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)

	// Calculate integer and fractional parts
	integer := new(big.Int).Div(amount, divisor)
	fractional := new(big.Int).Mod(amount, divisor)

	// Format with proper decimal places
	if fractional.Sign() == 0 {
		return integer.String()
	}

	// Pad fractional part with leading zeros if needed
	fracStr := fractional.String()
	for len(fracStr) < decimals {
		fracStr = "0" + fracStr
	}

	// Trim trailing zeros
	fracStr = trimTrailingZeros(fracStr)

	return integer.String() + "." + fracStr
}

func trimTrailingZeros(s string) string {
	for len(s) > 0 && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}
	return s
}

// Base58ToHex converts Tron base58 address to hex format with 41 prefix
func Base58ToHex(address string) string {
	return base58ToHex(address)
}

// HexToBase58 converts Tron hex address (41...) to base58 format
func HexToBase58(hexAddr string) string {
	return hexToBase58(hexAddr)
}

// base58ToHex converts Tron base58 address to hex format with 41 prefix
func base58ToHex(address string) string {
	if address == "" {
		return ""
	}
	decoded, err := common.DecodeCheck(address)
	if err != nil {
		return ""
	}
	return hex.EncodeToString(decoded)
}

// hexToBase58 converts Tron hex address (41...) to base58 format
func hexToBase58(hexAddr string) string {
	if hexAddr == "" {
		return ""
	}
	// Remove 0x prefix if present
	if len(hexAddr) > 2 && hexAddr[:2] == "0x" {
		hexAddr = hexAddr[2:]
	}
	// Decode hex
	decoded, err := hex.DecodeString(hexAddr)
	if err != nil {
		return ""
	}
	// Encode to base58check
	return common.EncodeCheck(decoded)
}
