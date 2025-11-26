package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/frstrtr/mongotron/pkg/logger"
	"google.golang.org/protobuf/proto"
)

// TronParser parses Tron blockchain data
type TronParser struct {
	logger     *logger.Logger
	abiDecoder *ABIDecoder
}

// NewTronParser creates a new Tron parser
func NewTronParser(log *logger.Logger) *TronParser {
	if log == nil {
		defaultLog := logger.NewDefault()
		log = &defaultLog
	}

	return &TronParser{
		logger:     log,
		abiDecoder: NewABIDecoder(),
	}
}

// DecodeSmartContract decodes smart contract call data and returns decoded information
// Returns nil if not a smart contract call or if decoding fails
func (p *TronParser) DecodeSmartContract(contract *core.Transaction_Contract) *DecodedCall {
	if contract == nil || contract.GetType() != core.Transaction_Contract_TriggerSmartContract {
		return nil
	}

	var trigger core.TriggerSmartContract
	if err := proto.Unmarshal(contract.GetParameter().GetValue(), &trigger); err != nil {
		p.logger.Debug().Err(err).Msg("Failed to unmarshal TriggerSmartContract")
		return nil
	}

	callData := trigger.GetData()
	if len(callData) < 4 {
		return nil
	}

	decoded, err := p.abiDecoder.DecodeContractData(callData)
	if err != nil {
		p.logger.Debug().
			Err(err).
			Str("methodSig", hex.EncodeToString(callData[:4])).
			Msg("Could not decode contract data")
		return nil
	}

	return decoded
}

// ExtractAddresses extracts all addresses from a contract
func (p *TronParser) ExtractAddresses(contract *core.Transaction_Contract) []string {
	if contract == nil {
		return nil
	}

	addresses := make([]string, 0)

	switch contract.GetType() {
	case core.Transaction_Contract_TransferContract:
		addresses = append(addresses, p.parseTransferContract(contract)...)
	case core.Transaction_Contract_TransferAssetContract:
		addresses = append(addresses, p.parseTransferAssetContract(contract)...)
	case core.Transaction_Contract_TriggerSmartContract:
		addresses = append(addresses, p.parseTriggerSmartContract(contract)...)
	case core.Transaction_Contract_CreateSmartContract:
		addresses = append(addresses, p.parseCreateSmartContract(contract)...)
	// Gas station operations
	case core.Transaction_Contract_FreezeBalanceV2Contract:
		addresses = append(addresses, p.parseFreezeBalanceV2Contract(contract)...)
	case core.Transaction_Contract_UnfreezeBalanceV2Contract:
		addresses = append(addresses, p.parseUnfreezeBalanceV2Contract(contract)...)
	case core.Transaction_Contract_WithdrawExpireUnfreezeContract:
		addresses = append(addresses, p.parseWithdrawExpireUnfreezeContract(contract)...)
	case core.Transaction_Contract_DelegateResourceContract:
		addresses = append(addresses, p.parseDelegateResourceContract(contract)...)
	case core.Transaction_Contract_UnDelegateResourceContract:
		addresses = append(addresses, p.parseUnDelegateResourceContract(contract)...)
	case core.Transaction_Contract_VoteWitnessContract:
		addresses = append(addresses, p.parseVoteWitnessContract(contract)...)
	case core.Transaction_Contract_AccountPermissionUpdateContract:
		addresses = append(addresses, p.parseAccountPermissionUpdateContract(contract)...)
	default:
		// Extract addresses from raw parameter data
		addresses = append(addresses, p.parseGenericContract(contract)...)
	}

	return addresses
}

// ParseContract parses a contract and returns from, to addresses and amount
func (p *TronParser) ParseContract(contract *core.Transaction_Contract) (from, to string, amount int64) {
	if contract == nil {
		return "", "", 0
	}

	switch contract.GetType() {
	case core.Transaction_Contract_TransferContract:
		return p.parseTransfer(contract)
	case core.Transaction_Contract_TransferAssetContract:
		return p.parseAssetTransfer(contract)
	case core.Transaction_Contract_TriggerSmartContract:
		return p.parseSmartContractCall(contract)
	default:
		return p.parseGeneric(contract)
	}
}

// parseTransferContract extracts addresses from TRX transfer
func (p *TronParser) parseTransferContract(contract *core.Transaction_Contract) []string {
	var transfer core.TransferContract
	if err := proto.Unmarshal(contract.GetParameter().GetValue(), &transfer); err != nil {
		p.logger.Error().Err(err).Msg("Failed to unmarshal TransferContract")
		return nil
	}

	addresses := make([]string, 0, 2)
	if len(transfer.GetOwnerAddress()) > 0 {
		addresses = append(addresses, p.encodeAddress(transfer.GetOwnerAddress()))
	}
	if len(transfer.GetToAddress()) > 0 {
		addresses = append(addresses, p.encodeAddress(transfer.GetToAddress()))
	}

	return addresses
}

// parseTransfer extracts transfer details
func (p *TronParser) parseTransfer(contract *core.Transaction_Contract) (from, to string, amount int64) {
	var transfer core.TransferContract
	if err := proto.Unmarshal(contract.GetParameter().GetValue(), &transfer); err != nil {
		p.logger.Error().Err(err).Msg("Failed to unmarshal TransferContract")
		return "", "", 0
	}

	from = p.encodeAddress(transfer.GetOwnerAddress())
	to = p.encodeAddress(transfer.GetToAddress())
	amount = transfer.GetAmount()

	return from, to, amount
}

// parseTransferAssetContract extracts addresses from TRC10 token transfer
func (p *TronParser) parseTransferAssetContract(contract *core.Transaction_Contract) []string {
	var transfer core.TransferAssetContract
	if err := proto.Unmarshal(contract.GetParameter().GetValue(), &transfer); err != nil {
		p.logger.Error().Err(err).Msg("Failed to unmarshal TransferAssetContract")
		return nil
	}

	addresses := make([]string, 0, 2)
	if len(transfer.GetOwnerAddress()) > 0 {
		addresses = append(addresses, p.encodeAddress(transfer.GetOwnerAddress()))
	}
	if len(transfer.GetToAddress()) > 0 {
		addresses = append(addresses, p.encodeAddress(transfer.GetToAddress()))
	}

	return addresses
}

// parseAssetTransfer extracts asset transfer details
func (p *TronParser) parseAssetTransfer(contract *core.Transaction_Contract) (from, to string, amount int64) {
	var transfer core.TransferAssetContract
	if err := proto.Unmarshal(contract.GetParameter().GetValue(), &transfer); err != nil {
		p.logger.Error().Err(err).Msg("Failed to unmarshal TransferAssetContract")
		return "", "", 0
	}

	from = p.encodeAddress(transfer.GetOwnerAddress())
	to = p.encodeAddress(transfer.GetToAddress())
	amount = transfer.GetAmount()

	return from, to, amount
}

// parseTriggerSmartContract extracts addresses from smart contract interaction
func (p *TronParser) parseTriggerSmartContract(contract *core.Transaction_Contract) []string {
	var trigger core.TriggerSmartContract
	if err := proto.Unmarshal(contract.GetParameter().GetValue(), &trigger); err != nil {
		p.logger.Error().Err(err).Msg("Failed to unmarshal TriggerSmartContract")
		return nil
	}

	addresses := make([]string, 0, 4)

	// Add caller address (owner)
	if len(trigger.GetOwnerAddress()) > 0 {
		addresses = append(addresses, p.encodeAddress(trigger.GetOwnerAddress()))
	}

	// Add contract address
	if len(trigger.GetContractAddress()) > 0 {
		addresses = append(addresses, p.encodeAddress(trigger.GetContractAddress()))
	}

	// Decode contract call data to extract addresses from parameters
	callData := trigger.GetData()
	if len(callData) >= 4 {
		decoded, err := p.abiDecoder.DecodeContractData(callData)
		if err != nil {
			p.logger.Debug().
				Err(err).
				Str("methodSig", hex.EncodeToString(callData[:4])).
				Msg("Could not decode contract data")
		} else {
			// Add all addresses found in the contract parameters
			for _, addr := range decoded.Addresses {
				// Avoid duplicates
				duplicate := false
				for _, existing := range addresses {
					if existing == addr {
						duplicate = true
						break
					}
				}
				if !duplicate {
					addresses = append(addresses, addr)
				}
			}

			// Log the decoded call for debugging
			if len(decoded.Addresses) > 0 {
				p.logger.Debug().
					Str("method", decoded.MethodName).
					Strs("paramAddresses", decoded.Addresses).
					Msg("Decoded smart contract call")
			}
		}
	}

	return addresses
}

// parseSmartContractCall extracts smart contract call details
func (p *TronParser) parseSmartContractCall(contract *core.Transaction_Contract) (from, to string, amount int64) {
	var trigger core.TriggerSmartContract
	if err := proto.Unmarshal(contract.GetParameter().GetValue(), &trigger); err != nil {
		p.logger.Error().Err(err).Msg("Failed to unmarshal TriggerSmartContract")
		return "", "", 0
	}

	from = p.encodeAddress(trigger.GetOwnerAddress())
	to = p.encodeAddress(trigger.GetContractAddress())
	amount = trigger.GetCallValue()

	return from, to, amount
}

// parseCreateSmartContract extracts addresses from contract creation
func (p *TronParser) parseCreateSmartContract(contract *core.Transaction_Contract) []string {
	var create core.CreateSmartContract
	if err := proto.Unmarshal(contract.GetParameter().GetValue(), &create); err != nil {
		p.logger.Error().Err(err).Msg("Failed to unmarshal CreateSmartContract")
		return nil
	}

	addresses := make([]string, 0, 1)
	if len(create.GetOwnerAddress()) > 0 {
		addresses = append(addresses, p.encodeAddress(create.GetOwnerAddress()))
	}

	return addresses
}

// parseGenericContract attempts to extract addresses from unknown contract types
func (p *TronParser) parseGenericContract(contract *core.Transaction_Contract) []string {
	// For generic contracts, we can try to extract addresses from the raw data
	// This is a basic implementation - you may want to enhance this based on specific contract types
	return []string{}
}

// parseGeneric extracts generic contract details
func (p *TronParser) parseGeneric(contract *core.Transaction_Contract) (from, to string, amount int64) {
	// For generic contracts, return contract type as metadata
	return "", contract.GetType().String(), 0
}

// ParseLogs parses transaction logs/events
func (p *TronParser) ParseLogs(logs []*core.TransactionInfo_Log) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(logs))

	for _, log := range logs {
		logData := map[string]interface{}{
			"address": p.encodeAddress(log.GetAddress()),
			"data":    hex.EncodeToString(log.GetData()),
			"topics":  make([]string, 0, len(log.GetTopics())),
		}

		for _, topic := range log.GetTopics() {
			logData["topics"] = append(logData["topics"].([]string), hex.EncodeToString(topic))
		}

		result = append(result, logData)
	}

	return result
}

// encodeAddress converts a raw address to hex string
func (p *TronParser) encodeAddress(address []byte) string {
	if len(address) == 0 {
		return ""
	}
	return hex.EncodeToString(address)
}

// DecodeAddress converts hex address to bytes
func (p *TronParser) DecodeAddress(hexAddress string) ([]byte, error) {
	return hex.DecodeString(hexAddress)
}

// FormatAmount formats amount from sun to TRX (1 TRX = 1,000,000 sun)
func (p *TronParser) FormatAmount(sun int64) string {
	trx := float64(sun) / 1_000_000
	return fmt.Sprintf("%.6f TRX", trx)
}

// CalculateTransactionID calculates the transaction ID from a transaction
// Transaction ID = SHA256(RawData)
func (p *TronParser) CalculateTransactionID(tx *core.Transaction) string {
	if tx == nil || tx.GetRawData() == nil {
		return ""
	}

	// Serialize the raw transaction data
	rawBytes, err := proto.Marshal(tx.GetRawData())
	if err != nil {
		p.logger.Error().Err(err).Msg("Failed to marshal transaction raw data")
		return ""
	}

	// Calculate SHA256 hash
	hash := sha256.Sum256(rawBytes)
	return hex.EncodeToString(hash[:])
}

// ParseInternalTransactions extracts internal transactions from transaction info
func (p *TronParser) ParseInternalTransactions(txInfo *core.TransactionInfo) []map[string]interface{} {
	if txInfo == nil {
		return nil
	}

	internalTxs := txInfo.GetInternalTransactions()
	result := make([]map[string]interface{}, 0, len(internalTxs))

	for _, itx := range internalTxs {
		txData := map[string]interface{}{
			"hash":     hex.EncodeToString(itx.GetHash()),
			"from":     p.encodeAddress(itx.GetCallerAddress()),
			"to":       p.encodeAddress(itx.GetTransferToAddress()),
			"value":    itx.GetCallValueInfo(),
			"rejected": itx.GetRejected(),
			"note":     string(itx.GetNote()),
		}

		result = append(result, txData)
	}

	return result
}

// ============================================================================
// Gas Station Operation Parsers
// ============================================================================

// parseFreezeBalanceV2Contract extracts addresses from stake operation
func (p *TronParser) parseFreezeBalanceV2Contract(contract *core.Transaction_Contract) []string {
	var freeze core.FreezeBalanceV2Contract
	if err := proto.Unmarshal(contract.GetParameter().GetValue(), &freeze); err != nil {
		p.logger.Error().Err(err).Msg("Failed to unmarshal FreezeBalanceV2Contract")
		return nil
	}

	addresses := make([]string, 0, 1)
	if len(freeze.GetOwnerAddress()) > 0 {
		addresses = append(addresses, p.encodeAddress(freeze.GetOwnerAddress()))
	}
	return addresses
}

// parseUnfreezeBalanceV2Contract extracts addresses from unstake operation
func (p *TronParser) parseUnfreezeBalanceV2Contract(contract *core.Transaction_Contract) []string {
	var unfreeze core.UnfreezeBalanceV2Contract
	if err := proto.Unmarshal(contract.GetParameter().GetValue(), &unfreeze); err != nil {
		p.logger.Error().Err(err).Msg("Failed to unmarshal UnfreezeBalanceV2Contract")
		return nil
	}

	addresses := make([]string, 0, 1)
	if len(unfreeze.GetOwnerAddress()) > 0 {
		addresses = append(addresses, p.encodeAddress(unfreeze.GetOwnerAddress()))
	}
	return addresses
}

// parseWithdrawExpireUnfreezeContract extracts addresses from withdraw unstaked TRX
func (p *TronParser) parseWithdrawExpireUnfreezeContract(contract *core.Transaction_Contract) []string {
	var withdraw core.WithdrawExpireUnfreezeContract
	if err := proto.Unmarshal(contract.GetParameter().GetValue(), &withdraw); err != nil {
		p.logger.Error().Err(err).Msg("Failed to unmarshal WithdrawExpireUnfreezeContract")
		return nil
	}

	addresses := make([]string, 0, 1)
	if len(withdraw.GetOwnerAddress()) > 0 {
		addresses = append(addresses, p.encodeAddress(withdraw.GetOwnerAddress()))
	}
	return addresses
}

// parseDelegateResourceContract extracts addresses from resource delegation
func (p *TronParser) parseDelegateResourceContract(contract *core.Transaction_Contract) []string {
	var delegate core.DelegateResourceContract
	if err := proto.Unmarshal(contract.GetParameter().GetValue(), &delegate); err != nil {
		p.logger.Error().Err(err).Msg("Failed to unmarshal DelegateResourceContract")
		return nil
	}

	addresses := make([]string, 0, 2)
	if len(delegate.GetOwnerAddress()) > 0 {
		addresses = append(addresses, p.encodeAddress(delegate.GetOwnerAddress()))
	}
	if len(delegate.GetReceiverAddress()) > 0 {
		addresses = append(addresses, p.encodeAddress(delegate.GetReceiverAddress()))
	}
	return addresses
}

// parseUnDelegateResourceContract extracts addresses from resource reclamation
func (p *TronParser) parseUnDelegateResourceContract(contract *core.Transaction_Contract) []string {
	var undelegate core.UnDelegateResourceContract
	if err := proto.Unmarshal(contract.GetParameter().GetValue(), &undelegate); err != nil {
		p.logger.Error().Err(err).Msg("Failed to unmarshal UnDelegateResourceContract")
		return nil
	}

	addresses := make([]string, 0, 2)
	if len(undelegate.GetOwnerAddress()) > 0 {
		addresses = append(addresses, p.encodeAddress(undelegate.GetOwnerAddress()))
	}
	if len(undelegate.GetReceiverAddress()) > 0 {
		addresses = append(addresses, p.encodeAddress(undelegate.GetReceiverAddress()))
	}
	return addresses
}

// parseVoteWitnessContract extracts addresses from voting operation
func (p *TronParser) parseVoteWitnessContract(contract *core.Transaction_Contract) []string {
	var vote core.VoteWitnessContract
	if err := proto.Unmarshal(contract.GetParameter().GetValue(), &vote); err != nil {
		p.logger.Error().Err(err).Msg("Failed to unmarshal VoteWitnessContract")
		return nil
	}

	addresses := make([]string, 0, 1+len(vote.GetVotes()))
	if len(vote.GetOwnerAddress()) > 0 {
		addresses = append(addresses, p.encodeAddress(vote.GetOwnerAddress()))
	}
	// Also extract SR addresses being voted for
	for _, v := range vote.GetVotes() {
		if len(v.GetVoteAddress()) > 0 {
			addresses = append(addresses, p.encodeAddress(v.GetVoteAddress()))
		}
	}
	return addresses
}

// parseAccountPermissionUpdateContract extracts addresses from permission change
// CRITICAL: This should trigger security alerts
func (p *TronParser) parseAccountPermissionUpdateContract(contract *core.Transaction_Contract) []string {
	var permUpdate core.AccountPermissionUpdateContract
	if err := proto.Unmarshal(contract.GetParameter().GetValue(), &permUpdate); err != nil {
		p.logger.Error().Err(err).Msg("Failed to unmarshal AccountPermissionUpdateContract")
		return nil
	}

	addresses := make([]string, 0, 1)
	if len(permUpdate.GetOwnerAddress()) > 0 {
		addresses = append(addresses, p.encodeAddress(permUpdate.GetOwnerAddress()))
	}
	return addresses
}

// ============================================================================
// Gas Station Operation Detail Parsers
// ============================================================================

// ParseFreezeDetails returns stake operation details
func (p *TronParser) ParseFreezeDetails(contract *core.Transaction_Contract) (owner, resourceType string, amount int64) {
	var freeze core.FreezeBalanceV2Contract
	if err := proto.Unmarshal(contract.GetParameter().GetValue(), &freeze); err != nil {
		return "", "", 0
	}

	owner = p.encodeAddress(freeze.GetOwnerAddress())
	resourceType = "BANDWIDTH"
	if freeze.GetResource() == core.ResourceCode_ENERGY {
		resourceType = "ENERGY"
	}

	return owner, resourceType, freeze.GetFrozenBalance()
}

// ParseUnfreezeDetails returns unstake operation details
func (p *TronParser) ParseUnfreezeDetails(contract *core.Transaction_Contract) (owner, resourceType string, amount int64) {
	var unfreeze core.UnfreezeBalanceV2Contract
	if err := proto.Unmarshal(contract.GetParameter().GetValue(), &unfreeze); err != nil {
		return "", "", 0
	}

	owner = p.encodeAddress(unfreeze.GetOwnerAddress())
	resourceType = "BANDWIDTH"
	if unfreeze.GetResource() == core.ResourceCode_ENERGY {
		resourceType = "ENERGY"
	}

	return owner, resourceType, unfreeze.GetUnfreezeBalance()
}

// ParseWithdrawDetails returns withdraw unstaked TRX details
func (p *TronParser) ParseWithdrawDetails(contract *core.Transaction_Contract) (owner string) {
	var withdraw core.WithdrawExpireUnfreezeContract
	if err := proto.Unmarshal(contract.GetParameter().GetValue(), &withdraw); err != nil {
		return ""
	}

	return p.encodeAddress(withdraw.GetOwnerAddress())
}

// DelegateDetails contains delegation operation details
type DelegateDetails struct {
	Owner        string
	Receiver     string
	ResourceType string
	Amount       int64
	Lock         bool
	LockPeriod   int64
}

// ParseDelegateDetails returns delegation operation details
func (p *TronParser) ParseDelegateDetails(contract *core.Transaction_Contract) *DelegateDetails {
	var delegate core.DelegateResourceContract
	if err := proto.Unmarshal(contract.GetParameter().GetValue(), &delegate); err != nil {
		return nil
	}

	resourceType := "BANDWIDTH"
	if delegate.GetResource() == core.ResourceCode_ENERGY {
		resourceType = "ENERGY"
	}

	return &DelegateDetails{
		Owner:        p.encodeAddress(delegate.GetOwnerAddress()),
		Receiver:     p.encodeAddress(delegate.GetReceiverAddress()),
		ResourceType: resourceType,
		Amount:       delegate.GetBalance(),
		Lock:         delegate.GetLock(),
		LockPeriod:   delegate.GetLockPeriod(),
	}
}

// UnDelegateDetails contains undelegation operation details
type UnDelegateDetails struct {
	Owner        string
	Receiver     string
	ResourceType string
	Amount       int64
}

// ParseUnDelegateDetails returns undelegation operation details
func (p *TronParser) ParseUnDelegateDetails(contract *core.Transaction_Contract) *UnDelegateDetails {
	var undelegate core.UnDelegateResourceContract
	if err := proto.Unmarshal(contract.GetParameter().GetValue(), &undelegate); err != nil {
		return nil
	}

	resourceType := "BANDWIDTH"
	if undelegate.GetResource() == core.ResourceCode_ENERGY {
		resourceType = "ENERGY"
	}

	return &UnDelegateDetails{
		Owner:        p.encodeAddress(undelegate.GetOwnerAddress()),
		Receiver:     p.encodeAddress(undelegate.GetReceiverAddress()),
		ResourceType: resourceType,
		Amount:       undelegate.GetBalance(),
	}
}

// VoteDetails contains voting operation details
type VoteDetails struct {
	Owner      string
	Votes      []VoteEntry
	TotalVotes int64
}

// VoteEntry represents a single vote for an SR
type VoteEntry struct {
	SRAddress string
	VoteCount int64
}

// ParseVoteDetails returns voting operation details
func (p *TronParser) ParseVoteDetails(contract *core.Transaction_Contract) *VoteDetails {
	var vote core.VoteWitnessContract
	if err := proto.Unmarshal(contract.GetParameter().GetValue(), &vote); err != nil {
		return nil
	}

	details := &VoteDetails{
		Owner: p.encodeAddress(vote.GetOwnerAddress()),
		Votes: make([]VoteEntry, 0, len(vote.GetVotes())),
	}

	for _, v := range vote.GetVotes() {
		entry := VoteEntry{
			SRAddress: p.encodeAddress(v.GetVoteAddress()),
			VoteCount: v.GetVoteCount(),
		}
		details.Votes = append(details.Votes, entry)
		details.TotalVotes += v.GetVoteCount()
	}

	return details
}

// PermissionDetails contains permission change details
type PermissionDetails struct {
	Owner            string
	OwnerPermission  *PermissionInfo
	ActivePermission []*PermissionInfo
}

// PermissionInfo contains permission details
type PermissionInfo struct {
	Name      string
	Threshold int64
	Keys      []KeyInfo
}

// KeyInfo contains key details
type KeyInfo struct {
	Address string
	Weight  int64
}

// ParsePermissionDetails returns permission change details
// CRITICAL: Used for security alerts
func (p *TronParser) ParsePermissionDetails(contract *core.Transaction_Contract) *PermissionDetails {
	var permUpdate core.AccountPermissionUpdateContract
	if err := proto.Unmarshal(contract.GetParameter().GetValue(), &permUpdate); err != nil {
		return nil
	}

	details := &PermissionDetails{
		Owner: p.encodeAddress(permUpdate.GetOwnerAddress()),
	}

	// Parse owner permission
	if owner := permUpdate.GetOwner(); owner != nil {
		details.OwnerPermission = &PermissionInfo{
			Name:      owner.GetPermissionName(),
			Threshold: owner.GetThreshold(),
			Keys:      make([]KeyInfo, 0, len(owner.GetKeys())),
		}
		for _, key := range owner.GetKeys() {
			details.OwnerPermission.Keys = append(details.OwnerPermission.Keys, KeyInfo{
				Address: p.encodeAddress(key.GetAddress()),
				Weight:  key.GetWeight(),
			})
		}
	}

	// Parse active permissions
	for _, active := range permUpdate.GetActives() {
		permInfo := &PermissionInfo{
			Name:      active.GetPermissionName(),
			Threshold: active.GetThreshold(),
			Keys:      make([]KeyInfo, 0, len(active.GetKeys())),
		}
		for _, key := range active.GetKeys() {
			permInfo.Keys = append(permInfo.Keys, KeyInfo{
				Address: p.encodeAddress(key.GetAddress()),
				Weight:  key.GetWeight(),
			})
		}
		details.ActivePermission = append(details.ActivePermission, permInfo)
	}

	return details
}
