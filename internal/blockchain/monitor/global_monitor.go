package monitor

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/frstrtr/mongotron/internal/blockchain/client"
	"github.com/frstrtr/mongotron/internal/blockchain/parser"
	"github.com/frstrtr/mongotron/pkg/logger"
)

// GlobalMonitor watches all blockchain transactions without address filtering
type GlobalMonitor struct {
	client       *client.TronClient
	parser       *parser.TronParser
	logger       *logger.Logger
	pollInterval time.Duration
	lastBlockNum int64
	isRunning    bool
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	eventChannel chan *AddressEvent
}

// GlobalConfig holds global monitor configuration
type GlobalConfig struct {
	PollInterval time.Duration
	StartBlock   int64 // 0 means start from current block
}

// NewGlobalMonitor creates a new global monitor that watches all transactions
func NewGlobalMonitor(
	tronClient *client.TronClient,
	cfg GlobalConfig,
	log *logger.Logger,
) (*GlobalMonitor, error) {
	if log == nil {
		defaultLog := logger.NewDefault()
		log = &defaultLog
	}

	if cfg.PollInterval == 0 {
		cfg.PollInterval = 3 * time.Second // Default 3 seconds (Tron block time)
	}

	ctx, cancel := context.WithCancel(context.Background())

	monitor := &GlobalMonitor{
		client:       tronClient,
		parser:       parser.NewTronParser(log),
		logger:       log,
		pollInterval: cfg.PollInterval,
		lastBlockNum: cfg.StartBlock,
		ctx:          ctx,
		cancel:       cancel,
		eventChannel: make(chan *AddressEvent, 1000), // Larger buffer for global monitoring
	}

	log.Info().
		Int64("startBlock", cfg.StartBlock).
		Dur("pollInterval", cfg.PollInterval).
		Msg("Global monitor initialized")

	return monitor, nil
}

// Start begins monitoring the blockchain
func (m *GlobalMonitor) Start() error {
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

	m.logger.Info().Msg("Starting global monitor")

	go m.monitorLoop()

	return nil
}

// Stop stops the monitor
func (m *GlobalMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isRunning {
		return
	}

	m.logger.Info().Msg("Stopping global monitor")
	m.cancel()
	m.isRunning = false
	close(m.eventChannel)
}

// Events returns the channel for receiving address events
func (m *GlobalMonitor) Events() <-chan *AddressEvent {
	return m.eventChannel
}

// GetLastBlockNumber returns the last processed block number
func (m *GlobalMonitor) GetLastBlockNumber() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastBlockNum
}

// monitorLoop continuously polls for new blocks
func (m *GlobalMonitor) monitorLoop() {
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			m.logger.Info().Msg("Global monitor loop stopped")
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
func (m *GlobalMonitor) processNewBlocks() error {
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
			Msg("Processing new blocks (global)")

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

// processBlock processes a single block and emits events for ALL transactions
func (m *GlobalMonitor) processBlock(blockNum int64) error {
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
		Msg("Processing block (global)")

	// Process ALL transactions in the block
	for _, tx := range transactions {
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
				m.logger.Debug().
					Int64("block", blockNum).
					Str("txHash", event.TransactionHash).
					Str("from", event.From).
					Str("to", event.To).
					Str("type", event.ContractType).
					Msg("Global event detected")
			default:
				m.logger.Warn().Msg("Event channel full, dropping event")
			}
		}
	}

	return nil
}

// extractEvent extracts event information from a transaction
func (m *GlobalMonitor) extractEvent(block *core.Block, tx *core.Transaction) (*AddressEvent, error) {
	if tx == nil || tx.GetRawData() == nil {
		return nil, fmt.Errorf("invalid transaction")
	}

	// Calculate the correct transaction ID (SHA256 of raw transaction data)
	txID := m.parser.CalculateTransactionID(tx)
	if txID == "" {
		return nil, fmt.Errorf("failed to calculate transaction ID")
	}

	// Get transaction info for success status and receipt
	ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
	defer cancel()

	txInfo, err := m.client.GetTransactionInfoById(ctx, txID)
	if err != nil {
		// Transaction might not be finalized yet
		return nil, nil
	}

	if txInfo == nil {
		return nil, nil
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
	event.Success = txInfo.GetResult() == core.TransactionInfo_SUCESS

	return event, nil
}
