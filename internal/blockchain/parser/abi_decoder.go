package parser

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
)

// ABIDecoder provides methods to decode Tron smart contract ABI parameters
type ABIDecoder struct{}

// NewABIDecoder creates a new ABI decoder
func NewABIDecoder() *ABIDecoder {
	return &ABIDecoder{}
}

// TRC20Method represents known TRC20 function signatures
type TRC20Method struct {
	Signature string
	Name      string
}

var (
	// Common TRC20 function signatures (first 4 bytes of keccak256 hash)
	TRC20Methods = map[string]TRC20Method{
		"a9059cbb": {Signature: "a9059cbb", Name: "transfer(address,uint256)"},             // transfer
		"23b872dd": {Signature: "23b872dd", Name: "transferFrom(address,address,uint256)"}, // transferFrom
		"095ea7b3": {Signature: "095ea7b3", Name: "approve(address,uint256)"},              // approve
		"dd62ed3e": {Signature: "dd62ed3e", Name: "allowance(address,address)"},            // allowance
		"70a08231": {Signature: "70a08231", Name: "balanceOf(address)"},                    // balanceOf
		"313ce567": {Signature: "313ce567", Name: "decimals()"},                            // decimals
		"06fdde03": {Signature: "06fdde03", Name: "name()"},                                // name
		"95d89b41": {Signature: "95d89b41", Name: "symbol()"},                              // symbol
		"18160ddd": {Signature: "18160ddd", Name: "totalSupply()"},                         // totalSupply
	}
)

// DecodedCall represents a decoded smart contract call
type DecodedCall struct {
	MethodSignature string
	MethodName      string
	Addresses       []string // All addresses found in parameters
	Amount          *big.Int // Amount if applicable
	Parameters      map[string]interface{}
}

// DecodeContractData decodes smart contract call data and extracts all addresses
func (d *ABIDecoder) DecodeContractData(data []byte) (*DecodedCall, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("data too short to contain method signature")
	}

	// First 4 bytes are the method signature
	methodSig := hex.EncodeToString(data[:4])

	result := &DecodedCall{
		MethodSignature: methodSig,
		Addresses:       make([]string, 0),
		Parameters:      make(map[string]interface{}),
	}

	// Check if it's a known TRC20 method
	if method, exists := TRC20Methods[methodSig]; exists {
		result.MethodName = method.Name

		// Decode based on method signature
		switch methodSig {
		case "a9059cbb": // transfer(address,uint256)
			return d.decodeTransfer(data)
		case "23b872dd": // transferFrom(address,address,uint256)
			return d.decodeTransferFrom(data)
		case "095ea7b3": // approve(address,uint256)
			return d.decodeApprove(data)
		case "70a08231": // balanceOf(address)
			return d.decodeBalanceOf(data)
		case "dd62ed3e": // allowance(address,address)
			return d.decodeAllowance(data)
		}
	}

	// For unknown methods, try to extract any addresses from the data
	result.Addresses = d.extractAddressesFromData(data[4:])

	return result, nil
}

// decodeTransfer decodes transfer(address to, uint256 amount)
func (d *ABIDecoder) decodeTransfer(data []byte) (*DecodedCall, error) {
	if len(data) < 68 { // 4 (method) + 32 (address) + 32 (amount)
		return nil, fmt.Errorf("insufficient data for transfer")
	}

	result := &DecodedCall{
		MethodSignature: "a9059cbb",
		MethodName:      "transfer(address,uint256)",
		Addresses:       make([]string, 0, 1),
		Parameters:      make(map[string]interface{}),
	}

	// Parameter 1: to address (32 bytes, last 20 bytes are the address, first byte should be 41 for Tron)
	toAddressBytes := data[16:36] // Skip first 12 bytes of padding, take 20 bytes
	toAddress := d.formatTronAddress(toAddressBytes)
	result.Addresses = append(result.Addresses, toAddress)
	result.Parameters["to"] = toAddress

	// Parameter 2: amount (32 bytes)
	amountBytes := data[36:68]
	amount := new(big.Int).SetBytes(amountBytes)
	result.Amount = amount
	result.Parameters["amount"] = amount.String()

	return result, nil
}

// decodeTransferFrom decodes transferFrom(address from, address to, uint256 amount)
func (d *ABIDecoder) decodeTransferFrom(data []byte) (*DecodedCall, error) {
	if len(data) < 100 { // 4 + 32 + 32 + 32
		return nil, fmt.Errorf("insufficient data for transferFrom")
	}

	result := &DecodedCall{
		MethodSignature: "23b872dd",
		MethodName:      "transferFrom(address,address,uint256)",
		Addresses:       make([]string, 0, 2),
		Parameters:      make(map[string]interface{}),
	}

	// Parameter 1: from address
	fromAddressBytes := data[16:36]
	fromAddress := d.formatTronAddress(fromAddressBytes)
	result.Addresses = append(result.Addresses, fromAddress)
	result.Parameters["from"] = fromAddress

	// Parameter 2: to address
	toAddressBytes := data[48:68]
	toAddress := d.formatTronAddress(toAddressBytes)
	result.Addresses = append(result.Addresses, toAddress)
	result.Parameters["to"] = toAddress

	// Parameter 3: amount
	amountBytes := data[68:100]
	amount := new(big.Int).SetBytes(amountBytes)
	result.Amount = amount
	result.Parameters["amount"] = amount.String()

	return result, nil
}

// decodeApprove decodes approve(address spender, uint256 amount)
func (d *ABIDecoder) decodeApprove(data []byte) (*DecodedCall, error) {
	if len(data) < 68 {
		return nil, fmt.Errorf("insufficient data for approve")
	}

	result := &DecodedCall{
		MethodSignature: "095ea7b3",
		MethodName:      "approve(address,uint256)",
		Addresses:       make([]string, 0, 1),
		Parameters:      make(map[string]interface{}),
	}

	// Parameter 1: spender address
	spenderBytes := data[16:36]
	spender := d.formatTronAddress(spenderBytes)
	result.Addresses = append(result.Addresses, spender)
	result.Parameters["spender"] = spender

	// Parameter 2: amount
	amountBytes := data[36:68]
	amount := new(big.Int).SetBytes(amountBytes)
	result.Amount = amount
	result.Parameters["amount"] = amount.String()

	return result, nil
}

// decodeBalanceOf decodes balanceOf(address owner)
func (d *ABIDecoder) decodeBalanceOf(data []byte) (*DecodedCall, error) {
	if len(data) < 36 {
		return nil, fmt.Errorf("insufficient data for balanceOf")
	}

	result := &DecodedCall{
		MethodSignature: "70a08231",
		MethodName:      "balanceOf(address)",
		Addresses:       make([]string, 0, 1),
		Parameters:      make(map[string]interface{}),
	}

	// Parameter 1: owner address
	ownerBytes := data[16:36]
	owner := d.formatTronAddress(ownerBytes)
	result.Addresses = append(result.Addresses, owner)
	result.Parameters["owner"] = owner

	return result, nil
}

// decodeAllowance decodes allowance(address owner, address spender)
func (d *ABIDecoder) decodeAllowance(data []byte) (*DecodedCall, error) {
	if len(data) < 68 {
		return nil, fmt.Errorf("insufficient data for allowance")
	}

	result := &DecodedCall{
		MethodSignature: "dd62ed3e",
		MethodName:      "allowance(address,address)",
		Addresses:       make([]string, 0, 2),
		Parameters:      make(map[string]interface{}),
	}

	// Parameter 1: owner address
	ownerBytes := data[16:36]
	owner := d.formatTronAddress(ownerBytes)
	result.Addresses = append(result.Addresses, owner)
	result.Parameters["owner"] = owner

	// Parameter 2: spender address
	spenderBytes := data[48:68]
	spender := d.formatTronAddress(spenderBytes)
	result.Addresses = append(result.Addresses, spender)
	result.Parameters["spender"] = spender

	return result, nil
}

// formatTronAddress converts 20-byte address to Tron hex format (41 prefix + 20 bytes)
func (d *ABIDecoder) formatTronAddress(addressBytes []byte) string {
	if len(addressBytes) != 20 {
		return ""
	}

	// Tron addresses start with 0x41 (mainnet) or 0xa0 (testnet)
	// For contract parameters, addresses are stored without the prefix
	// We need to add 0x41 prefix for Tron mainnet/nile
	tronAddress := make([]byte, 21)
	tronAddress[0] = 0x41 // Tron address prefix
	copy(tronAddress[1:], addressBytes)

	return hex.EncodeToString(tronAddress)
}

// extractAddressesFromData attempts to extract any 20-byte sequences that look like addresses
func (d *ABIDecoder) extractAddressesFromData(data []byte) []string {
	addresses := make([]string, 0)

	// Look for 32-byte aligned addresses (12 bytes padding + 20 bytes address)
	for i := 0; i+32 <= len(data); i += 32 {
		chunk := data[i : i+32]

		// Check if first 12 bytes are zero (typical address padding)
		isZeroPadded := true
		for j := 0; j < 12; j++ {
			if chunk[j] != 0 {
				isZeroPadded = false
				break
			}
		}

		if isZeroPadded {
			addressBytes := chunk[12:]
			// Check if it looks like a valid address (not all zeros)
			hasNonZero := false
			for _, b := range addressBytes {
				if b != 0 {
					hasNonZero = true
					break
				}
			}

			if hasNonZero {
				addr := d.formatTronAddress(addressBytes)
				if addr != "" {
					addresses = append(addresses, addr)
				}
			}
		}
	}

	return addresses
}

// IsKnownMethod checks if a method signature is recognized
func (d *ABIDecoder) IsKnownMethod(methodSig string) bool {
	methodSig = strings.TrimPrefix(methodSig, "0x")
	_, exists := TRC20Methods[methodSig]
	return exists
}

// GetMethodName returns the human-readable method name
func (d *ABIDecoder) GetMethodName(methodSig string) string {
	methodSig = strings.TrimPrefix(methodSig, "0x")
	if method, exists := TRC20Methods[methodSig]; exists {
		return method.Name
	}
	return "unknown(" + methodSig + ")"
}
