package monitor

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/frstrtr/mongotron/internal/blockchain/parser"
	"github.com/frstrtr/mongotron/pkg/logger"
)

// BlockSource is the part of the TRON client the monitors read the chain from.
// *client.TronClient implements it.
type BlockSource interface {
	GetNowBlock(ctx context.Context) (*core.Block, error)
	GetBlockByNum(ctx context.Context, blockNum int64) (*core.Block, error)
	GetTransactionInfoById(ctx context.Context, txID string) (*core.TransactionInfo, error)
}

// headReader is implemented by a BlockSource that can read the head block number
// without downloading the head block's transactions.
type headReader interface {
	GetHeadBlockNumber(ctx context.Context) (int64, error)
}

// blockHashOf returns the identifier the monitors have always reported as BlockHash
// (the hex witness signature of the header).
func blockHashOf(block *core.Block) string {
	return hex.EncodeToString(block.GetBlockHeader().GetWitnessSignature())
}

// buildAddressEvent turns a transaction into an AddressEvent: contract type, parties,
// amount, decoded smart contract call and, from the node's transaction info, the
// outcome and the receipt. Without transaction info the event is still built, with
// Success=false (an outcome that was not verified is never assumed to be a success).
// Block fields are left to the caller.
func buildAddressEvent(ctx context.Context, src txInfoFetcher, p *parser.TronParser, log *logger.Logger, tx *core.Transaction) (*AddressEvent, error) {
	if tx == nil || tx.GetRawData() == nil {
		return nil, fmt.Errorf("invalid transaction")
	}

	// Calculate the correct transaction ID (SHA256 of raw transaction data)
	txID := p.CalculateTransactionID(tx)
	if txID == "" {
		return nil, fmt.Errorf("failed to calculate transaction ID")
	}

	txInfo, err := fetchTxInfo(ctx, src, txID, txInfoAttempts, txInfoRetryDelay, txInfoTimeout)
	if err != nil {
		log.Warn().
			Err(err).
			Str("txID", txID).
			Msg("Failed to get transaction info: event delivered with success=false (outcome not verified)")
		txInfo = nil
	}

	event := &AddressEvent{
		TransactionID:   txID,
		TransactionHash: txID,
		RawTransaction:  tx,
		RawTxInfo:       txInfo,
		Success:         false,
		EventData:       make(map[string]interface{}),
	}

	txRaw := tx.GetRawData()

	// Extract transaction details
	if len(txRaw.GetContract()) > 0 {
		contract := txRaw.GetContract()[0]
		event.ContractType = contract.GetType().String()

		// Parse contract data
		from, to, amount := p.ParseContract(contract)
		event.From = from
		event.To = to
		event.Amount = amount

		// Decode smart contract call data if available
		if decoded := p.DecodeSmartContract(contract); decoded != nil {
			sc := map[string]interface{}{
				"methodSignature": decoded.MethodSignature,
				"methodName":      decoded.MethodName,
				"addresses":       decoded.Addresses,
				"parameters":      decoded.Parameters,
			}
			if decoded.Amount != nil {
				sc["amount"] = decoded.Amount.String()
			}
			event.EventData["smartContract"] = sc
		}
	}

	// Extract transaction result from txInfo
	if txInfo != nil {
		event.Success = TxInfoSucceeded(txInfo)

		// Extract contract events/logs
		if len(txInfo.GetLog()) > 0 {
			event.EventType = "ContractEvent"
			event.EventData["logs"] = p.ParseLogs(txInfo.GetLog())
		}

		// Add receipt info
		if txInfo.GetReceipt() != nil {
			receipt := txInfo.GetReceipt()
			event.EventData["energyUsage"] = receipt.GetEnergyUsage()
			event.EventData["energyFee"] = receipt.GetEnergyFee()
			event.EventData["netUsage"] = receipt.GetNetUsage()
			event.EventData["netFee"] = receipt.GetNetFee()
			event.EventData["energyUsageTotal"] = receipt.GetEnergyUsageTotal()
			event.EventData["originEnergyUsage"] = receipt.GetOriginEnergyUsage()
			event.EventData["energyPenaltyTotal"] = receipt.GetEnergyPenaltyTotal()
			event.EventData["receiptResult"] = receipt.GetResult().String()
		}
		event.EventData["fee"] = txInfo.GetFee()
	}

	return event, nil
}
