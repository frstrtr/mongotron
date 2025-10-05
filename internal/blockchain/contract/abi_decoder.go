package contract

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/crypto"
)

// ABIDecoder handles smart contract ABI decoding
type ABIDecoder struct {
	cache map[string]*ContractABI
	mu    sync.RWMutex
}

// ContractABI represents a parsed contract ABI
type ContractABI struct {
	ABI      abi.ABI
	Methods  map[string]*abi.Method // method signature -> method
	RawABI   string
	Address  string
}

// DecodedMethod represents a decoded smart contract method call
type DecodedMethod struct {
	Name      string                 `json:"name"`
	Signature string                 `json:"signature"`
	Inputs    map[string]interface{} `json:"inputs,omitempty"`
}

// NewABIDecoder creates a new ABI decoder
func NewABIDecoder() *ABIDecoder {
	return &ABIDecoder{
		cache: make(map[string]*ContractABI),
	}
}

// LoadABI parses and caches an ABI for a contract address
func (d *ABIDecoder) LoadABI(address string, abiJSON string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if abiJSON == "" {
		return fmt.Errorf("empty ABI")
	}

	// Parse the ABI
	parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return fmt.Errorf("failed to parse ABI: %w", err)
	}

	// Build method signature map
	methods := make(map[string]*abi.Method)
	for _, method := range parsedABI.Methods {
		// Get method signature (first 4 bytes of keccak256 hash)
		sig := method.Sig
		methods[sig] = &method
	}

	d.cache[address] = &ContractABI{
		ABI:      parsedABI,
		Methods:  methods,
		RawABI:   abiJSON,
		Address:  address,
	}

	return nil
}

// DecodeMethodCall decodes a smart contract method call from input data
func (d *ABIDecoder) DecodeMethodCall(contractAddress string, inputData []byte) (*DecodedMethod, error) {
	d.mu.RLock()
	contractABI, exists := d.cache[contractAddress]
	d.mu.RUnlock()

	if !exists {
		// Try to decode without ABI - just get the method signature
		return d.decodeWithoutABI(inputData)
	}

	if len(inputData) < 4 {
		return nil, fmt.Errorf("input data too short")
	}

	// First 4 bytes are the method signature
	methodSig := hex.EncodeToString(inputData[0:4])
	
	// Try to find the method in the ABI
	for _, method := range contractABI.ABI.Methods {
		methodID := hex.EncodeToString(method.ID[:4])
		if methodID == methodSig {
			result := &DecodedMethod{
				Name:      method.RawName,
				Signature: method.Sig,
				Inputs:    make(map[string]interface{}),
			}

			// Try to decode inputs if we have parameter data
			if len(inputData) > 4 {
				inputs := make(map[string]interface{})
				err := method.Inputs.UnpackIntoMap(inputs, inputData[4:])
				if err == nil {
					result.Inputs = inputs
				}
			}

			return result, nil
		}
	}

	// Method not found in ABI
	return d.decodeWithoutABI(inputData)
}

// decodeWithoutABI attempts to decode common method signatures without ABI
func (d *ABIDecoder) decodeWithoutABI(inputData []byte) (*DecodedMethod, error) {
	if len(inputData) < 4 {
		return &DecodedMethod{
			Name:      "unknown",
			Signature: "unknown",
		}, nil
	}

	methodSig := hex.EncodeToString(inputData[0:4])
	
	// Common method signatures (ERC20, ERC721, etc.)
	commonMethods := map[string]string{
		"a9059cbb": "transfer(address,uint256)",
		"095ea7b3": "approve(address,uint256)",
		"23b872dd": "transferFrom(address,address,uint256)",
		"70a08231": "balanceOf(address)",
		"dd62ed3e": "allowance(address,address)",
		"18160ddd": "totalSupply()",
		"06fdde03": "name()",
		"95d89b41": "symbol()",
		"313ce567": "decimals()",
		"40c10f19": "mint(address,uint256)",
		"42966c68": "burn(uint256)",
		"79cc6790": "burnFrom(address,uint256)",
		"42842e0e": "safeTransferFrom(address,address,uint256)",
		"b88d4fde": "safeTransferFrom(address,address,uint256,bytes)",
		"6352211e": "ownerOf(uint256)",
		"081812fc": "getApproved(uint256)",
		"e985e9c5": "isApprovedForAll(address,address)",
		"a22cb465": "setApprovalForAll(address,bool)",
		"4e71d92d": "claim()",
		"d96a094a": "buy(uint256)",
		"e2bbb158": "deposit(uint256)",
		"2e1a7d4d": "withdraw(uint256)",
		"38ed1739": "swapExactTokensForTokens(uint256,uint256,address[],address,uint256)",
		"7ff36ab5": "swapExactETHForTokens(uint256,address[],address,uint256)",
		"18cbafe5": "swapExactTokensForETH(uint256,uint256,address[],address,uint256)",
		"fb3bdb41": "swapETHForExactTokens(uint256,address[],address,uint256)",
		"8803dbee": "swapTokensForExactTokens(uint256,uint256,address[],address,uint256)",
		"4a25d94a": "swapTokensForExactETH(uint256,uint256,address[],address,uint256)",
		"5c11d795": "swapExactTokensForTokensSupportingFeeOnTransferTokens(uint256,uint256,address[],address,uint256)",
		"791ac947": "swapExactETHForTokensSupportingFeeOnTransferTokens(uint256,address[],address,uint256)",
		"b6f9de95": "swapExactTokensForETHSupportingFeeOnTransferTokens(uint256,uint256,address[],address,uint256)",
		"e8e33700": "addLiquidity(address,address,uint256,uint256,uint256,uint256,address,uint256)",
		"f305d719": "addLiquidityETH(address,uint256,uint256,uint256,address,uint256)",
		"baa2abde": "removeLiquidity(address,address,uint256,uint256,uint256,address,uint256)",
		"02751cec": "removeLiquidityETH(address,uint256,uint256,uint256,address,uint256)",
		"ded9382a": "removeLiquidityETHSupportingFeeOnTransferTokens(address,uint256,uint256,uint256,address,uint256)",
		"af2979eb": "removeLiquidityETHWithPermit(address,uint256,uint256,uint256,address,uint256,bool,uint8,bytes32,bytes32)",
		"5b0d5984": "removeLiquidityETHWithPermitSupportingFeeOnTransferTokens(address,uint256,uint256,uint256,address,uint256,bool,uint8,bytes32,bytes32)",
		"2195995c": "removeLiquidityWithPermit(address,address,uint256,uint256,uint256,address,uint256,bool,uint8,bytes32,bytes32)",
		"022c0d9f": "swap(uint256,uint256,address,bytes)",
		"3ccfd60b": "withdraw()",
		"d0e30db0": "deposit()",
		"a694fc3a": "stake(uint256)",
		"2e17de78": "unstake(uint256)",
		"e9fad8ee": "exit()",
		"3d18b912": "getReward()",
		"8b876347": "earned(address)",
	}

	if methodName, exists := commonMethods[methodSig]; exists {
		parts := strings.Split(methodName, "(")
		name := parts[0]
		return &DecodedMethod{
			Name:      name,
			Signature: methodName,
		}, nil
	}

	// Unknown method
	return &DecodedMethod{
		Name:      fmt.Sprintf("0x%s", methodSig),
		Signature: fmt.Sprintf("0x%s", methodSig),
	}, nil
}

// GetMethodSignature computes the method signature (first 4 bytes of keccak256)
func GetMethodSignature(methodDef string) string {
	hash := crypto.Keccak256([]byte(methodDef))
	return hex.EncodeToString(hash[:4])
}

// GetHumanReadableType converts method name to human-readable interaction type
func GetHumanReadableType(methodName string) string {
	// Convert method name to human-readable format
	typeMap := map[string]string{
		"transfer":                    "Token Transfer",
		"transferFrom":                "Token Transfer From",
		"approve":                     "Token Approve",
		"mint":                        "Token Mint",
		"burn":                        "Token Burn",
		"burnFrom":                    "Token Burn From",
		"stake":                       "Stake Tokens",
		"unstake":                     "Unstake Tokens",
		"claim":                       "Claim Rewards",
		"getReward":                   "Get Reward",
		"deposit":                     "Deposit",
		"withdraw":                    "Withdraw",
		"buy":                         "Buy",
		"sell":                        "Sell",
		"swap":                        "Swap",
		"swapExactTokensForTokens":    "Swap Tokens",
		"swapExactETHForTokens":       "Swap TRX for Tokens",
		"swapExactTokensForETH":       "Swap Tokens for TRX",
		"swapETHForExactTokens":       "Swap TRX for Exact Tokens",
		"swapTokensForExactTokens":    "Swap for Exact Tokens",
		"swapTokensForExactETH":       "Swap Tokens for Exact TRX",
		"addLiquidity":                "Add Liquidity",
		"addLiquidityETH":             "Add Liquidity (TRX)",
		"removeLiquidity":             "Remove Liquidity",
		"removeLiquidityETH":          "Remove Liquidity (TRX)",
		"safeTransferFrom":            "Safe Transfer",
		"setApprovalForAll":           "Set Approval For All",
		"balanceOf":                   "Balance Query",
		"ownerOf":                     "Owner Query",
		"allowance":                   "Allowance Query",
		"totalSupply":                 "Total Supply Query",
		"name":                        "Name Query",
		"symbol":                      "Symbol Query",
		"decimals":                    "Decimals Query",
		"getApproved":                 "Get Approved Query",
		"isApprovedForAll":            "Is Approved Query",
		"earned":                      "Earned Query",
		"exit":                        "Exit",
	}

	if readable, exists := typeMap[methodName]; exists {
		return readable
	}

	// If starts with 0x, it's an unknown method signature
	if strings.HasPrefix(methodName, "0x") {
		return fmt.Sprintf("Unknown Method (%s)", methodName)
	}

	// Convert camelCase to Title Case
	return toTitleCase(methodName)
}

// toTitleCase converts camelCase to Title Case
func toTitleCase(s string) string {
	if s == "" {
		return s
	}
	
	var result strings.Builder
	result.WriteRune([]rune(strings.ToUpper(string(s[0])))[0])
	
	for i := 1; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			result.WriteRune(' ')
		}
		result.WriteRune([]rune(s)[i])
	}
	
	return result.String()
}

// HasABI checks if ABI is cached for a contract
func (d *ABIDecoder) HasABI(contractAddress string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	_, exists := d.cache[contractAddress]
	return exists
}

// GetCachedABI returns cached ABI for a contract
func (d *ABIDecoder) GetCachedABI(contractAddress string) (*ContractABI, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	abi, exists := d.cache[contractAddress]
	return abi, exists
}

// ClearCache clears the ABI cache
func (d *ABIDecoder) ClearCache() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cache = make(map[string]*ContractABI)
}

// ExportCache exports the ABI cache as JSON
func (d *ABIDecoder) ExportCache() ([]byte, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return json.Marshal(d.cache)
}

// GetCacheSize returns the number of cached ABIs
func (d *ABIDecoder) GetCacheSize() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.cache)
}
