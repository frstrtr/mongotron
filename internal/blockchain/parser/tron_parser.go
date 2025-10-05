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
	logger *logger.Logger
}

// NewTronParser creates a new Tron parser
func NewTronParser(log *logger.Logger) *TronParser {
	if log == nil {
		defaultLog := logger.NewDefault()
		log = &defaultLog
	}

	return &TronParser{
		logger: log,
	}
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

	addresses := make([]string, 0, 2)
	if len(trigger.GetOwnerAddress()) > 0 {
		addresses = append(addresses, p.encodeAddress(trigger.GetOwnerAddress()))
	}
	if len(trigger.GetContractAddress()) > 0 {
		addresses = append(addresses, p.encodeAddress(trigger.GetContractAddress()))
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
