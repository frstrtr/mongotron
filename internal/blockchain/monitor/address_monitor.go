package monitor

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/fbsobreira/gotron-sdk/pkg/common"
	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/frstrtr/mongotron/internal/blockchain/client"
	"github.com/frstrtr/mongotron/internal/blockchain/parser"
	"github.com/frstrtr/mongotron/pkg/logger"
)

// AddressMonitor watches blockchain for a specific address
type AddressMonitor struct {
	client          *client.TronClient
	parser          *parser.TronParser
	logger          *logger.Logger
	watchAddress    string
	watchAddressHex string
	pollInterval    time.Duration
	lastBlockNum    int64
	isRunning       bool
	mu              sync.RWMutex
	ctx             context.Context
	cancel          context.CancelFunc
	eventChannel    chan *AddressEvent
}

// AddressEvent represents an event related to the watched address
type AddressEvent struct {
	BlockNumber     int64
	BlockHash       string
	BlockTimestamp  int64
	TransactionID   string
	TransactionHash string
	From            string
	To              string
	Amount          int64
	AssetName       string
	ContractType    string
	Success         bool
	EventType       string
	EventData       map[string]interface{}
	RawTransaction  *core.Transaction
	RawTxInfo       *core.TransactionInfo
}

// Config holds address monitor configuration
type Config struct {
	WatchAddress string
	PollInterval time.Duration
	StartBlock   int64 // 0 means start from current block
}

// NewAddressMonitor creates a new address monitor
func NewAddressMonitor(
	tronClient *client.TronClient,
	cfg Config,
	log *logger.Logger,
) (*AddressMonitor, error) {
	if log == nil {
		defaultLog := logger.NewDefault()
		log = &defaultLog
	}

	if cfg.WatchAddress == "" {
		return nil, fmt.Errorf("watch address cannot be empty")
	}

	if cfg.PollInterval == 0 {
		cfg.PollInterval = 3 * time.Second // Default 3 seconds (Tron block time)
	}

	ctx, cancel := context.WithCancel(context.Background())

	monitor := &AddressMonitor{
		client:       tronClient,
		parser:       parser.NewTronParser(log),
		logger:       log,
		watchAddress: cfg.WatchAddress,
		pollInterval: cfg.PollInterval,
		lastBlockNum: cfg.StartBlock,
		ctx:          ctx,
		cancel:       cancel,
		eventChannel: make(chan *AddressEvent, 100),
	}

	// Convert address to hex for comparison
	monitor.watchAddressHex = monitor.addressToHex(cfg.WatchAddress)

	log.Info().
		Str("address", cfg.WatchAddress).
		Str("hex", monitor.watchAddressHex).
		Int64("startBlock", cfg.StartBlock).
		Dur("pollInterval", cfg.PollInterval).
		Msg("Address monitor initialized")

	return monitor, nil
}

// Start begins monitoring the blockchain
func (m *AddressMonitor) Start() error {
	m.mu.Lock()
	if m.isRunning {
		m.mu.Unlock()
		return fmt.Errorf("monitor is already running")
	}
	m.isRunning = true
	m.mu.Unlock()

	// If starting from block 0 or negative (use current block)
	if m.lastBlockNum <= 0 {
		block, err := m.client.GetNowBlock(m.ctx)
		if err != nil {
			return fmt.Errorf("failed to get current block: %w", err)
		}
		m.lastBlockNum = block.GetBlockHeader().GetRawData().GetNumber()
		m.logger.Info().
			Int64("startBlock", m.lastBlockNum).
			Msg("Starting from current block")
	}

	m.logger.Info().Msg("Starting address monitor")

	go m.monitorLoop()

	return nil
}

// Stop stops the monitor
func (m *AddressMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isRunning {
		return
	}

	m.logger.Info().Msg("Stopping address monitor")
	m.cancel()
	m.isRunning = false
	close(m.eventChannel)
}

// Events returns the channel for receiving address events
func (m *AddressMonitor) Events() <-chan *AddressEvent {
	return m.eventChannel
}

// GetLastBlockNumber returns the last processed block number
func (m *AddressMonitor) GetLastBlockNumber() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastBlockNum
}

// monitorLoop continuously polls for new blocks
func (m *AddressMonitor) monitorLoop() {
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			m.logger.Info().Msg("Monitor loop stopped")
			return

		case <-ticker.C:
			if err := m.processNewBlocks(); err != nil {
				m.logger.Error().
					Err(err).
					Msg("Error processing new blocks")
				// Continue monitoring despite errors
			}
		}
	}
}

// processNewBlocks checks for and processes new blocks
func (m *AddressMonitor) processNewBlocks() error {
	// Get current block number
	currentBlock, err := m.client.GetNowBlock(m.ctx)
	if err != nil {
		return fmt.Errorf("failed to get current block: %w", err)
	}

	currentBlockNum := currentBlock.GetBlockHeader().GetRawData().GetNumber()

	m.mu.RLock()
	lastProcessed := m.lastBlockNum
	m.mu.RUnlock()

	// Process any new blocks
	if currentBlockNum > lastProcessed {
		m.logger.Debug().
			Int64("from", lastProcessed+1).
			Int64("to", currentBlockNum).
			Msg("Processing new blocks")

		for blockNum := lastProcessed + 1; blockNum <= currentBlockNum; blockNum++ {
			if err := m.processBlock(blockNum); err != nil {
				m.logger.Error().
					Err(err).
					Int64("block", blockNum).
					Msg("Error processing block")
				// Continue with next block
				continue
			}

			m.mu.Lock()
			m.lastBlockNum = blockNum
			m.mu.Unlock()
		}
	}

	return nil
}

// processBlock processes a single block
func (m *AddressMonitor) processBlock(blockNum int64) error {
	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	defer cancel()

	block, err := m.client.GetBlockByNum(ctx, blockNum)
	if err != nil {
		return fmt.Errorf("failed to get block %d: %w", blockNum, err)
	}

	if block == nil || block.GetBlockHeader() == nil {
		return fmt.Errorf("received nil block or header for block %d", blockNum)
	}

	blockHeader := block.GetBlockHeader().GetRawData()
	blockHash := hex.EncodeToString(block.GetBlockHeader().GetWitnessSignature())
	transactions := block.GetTransactions()

	m.logger.Debug().
		Int64("block", blockNum).
		Int("txCount", len(transactions)).
		Msg("Processing block")

	// Check each transaction in the block
	for _, tx := range transactions {
		if m.isAddressInTransaction(tx) {
			event, err := m.extractEvent(block, tx)
			if err != nil {
				txID := m.parser.CalculateTransactionID(tx)
				m.logger.Error().
					Err(err).
					Str("txID", txID).
					Msg("Error extracting event from transaction")
				continue
			}

			if event != nil {
				event.BlockNumber = blockNum
				event.BlockHash = blockHash
				event.BlockTimestamp = blockHeader.GetTimestamp()

				// Send event to channel (non-blocking)
				select {
				case m.eventChannel <- event:
					m.logger.Info().
						Int64("block", blockNum).
						Str("txHash", event.TransactionHash).
						Str("from", event.From).
						Str("to", event.To).
						Int64("amount", event.Amount).
						Str("type", event.ContractType).
						Msg("Address event detected")
				default:
					m.logger.Warn().Msg("Event channel full, dropping event")
				}
			}
		}
	}

	return nil
}

// isAddressInTransaction checks if the watched address is involved in the transaction
func (m *AddressMonitor) isAddressInTransaction(tx *core.Transaction) bool {
	if tx == nil || tx.GetRawData() == nil {
		return false
	}

	contracts := tx.GetRawData().GetContract()
	for _, contract := range contracts {
		addresses := m.parser.ExtractAddresses(contract)
		for _, addr := range addresses {
			if addr == m.watchAddress || addr == m.watchAddressHex {
				return true
			}
		}
	}

	return false
}

// extractEvent extracts detailed event information from a transaction
func (m *AddressMonitor) extractEvent(block *core.Block, tx *core.Transaction) (*AddressEvent, error) {
	if tx == nil || tx.GetRawData() == nil {
		return nil, fmt.Errorf("invalid transaction")
	}

	// Calculate the correct transaction ID (SHA256 of raw transaction data)
	txID := m.parser.CalculateTransactionID(tx)
	if txID == "" {
		return nil, fmt.Errorf("failed to calculate transaction ID")
	}

	// Get transaction info (includes events, logs, receipt)
	ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
	defer cancel()

	txInfo, err := m.client.GetTransactionInfoById(ctx, txID)
	if err != nil {
		m.logger.Warn().
			Err(err).
			Str("txID", txID).
			Msg("Failed to get transaction info, using basic data")
		// Continue with basic transaction data
		txInfo = nil
	}

	event := &AddressEvent{
		TransactionID:   txID,
		TransactionHash: txID,
		RawTransaction:  tx,
		RawTxInfo:       txInfo,
		Success:         true,
		EventData:       make(map[string]interface{}),
	}

	txRaw := tx.GetRawData()

	// Extract transaction details
	if len(txRaw.GetContract()) > 0 {
		contract := txRaw.GetContract()[0]
		event.ContractType = contract.GetType().String()

		// Parse contract data
		from, to, amount := m.parser.ParseContract(contract)
		event.From = from
		event.To = to
		event.Amount = amount

		// Decode smart contract call data if available
		if decoded := m.parser.DecodeSmartContract(contract); decoded != nil {
			event.EventData["smartContract"] = map[string]interface{}{
				"methodSignature": decoded.MethodSignature,
				"methodName":      decoded.MethodName,
				"addresses":       decoded.Addresses,
				"parameters":      decoded.Parameters,
			}
			if decoded.Amount != nil {
				event.EventData["smartContract"].(map[string]interface{})["amount"] = decoded.Amount.String()
			}
		}
	}

	// Extract transaction result from txInfo
	if txInfo != nil {
		event.Success = txInfo.GetResult() == core.TransactionInfo_SUCESS

		// Extract contract events/logs
		if len(txInfo.GetLog()) > 0 {
			event.EventType = "ContractEvent"
			event.EventData["logs"] = m.parser.ParseLogs(txInfo.GetLog())
		}

		// Add receipt info
		if txInfo.GetReceipt() != nil {
			receipt := txInfo.GetReceipt()
			event.EventData["energyUsage"] = receipt.GetEnergyUsage()
			event.EventData["energyFee"] = receipt.GetEnergyFee()
			event.EventData["netUsage"] = receipt.GetNetUsage()
			event.EventData["netFee"] = receipt.GetNetFee()
		}
	}

	return event, nil
}

// addressToHex converts a Tron base58 address to hex format
func (m *AddressMonitor) addressToHex(address string) string {
	if address == "" {
		return ""
	}
	// Decode base58 address to raw bytes
	decoded, err := common.DecodeCheck(address)
	if err != nil {
		m.logger.Error().Err(err).Str("address", address).Msg("Failed to decode base58 address")
		return ""
	}
	// Convert to hex string
	return hex.EncodeToString(decoded)
}
