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

// BlockMonitor watches all blockchain activity
type BlockMonitor struct {
	client       *client.TronClient
	parser       *parser.TronParser
	logger       *logger.Logger
	pollInterval time.Duration
	lastBlockNum int64
	isRunning    bool
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	eventChannel chan *BlockEvent
}

// BlockEvent represents comprehensive block data with all addresses and interactions
type BlockEvent struct {
	BlockNumber    int64
	BlockHash      string
	BlockTimestamp int64
	Addresses      map[string]*AddressInfo // Key: address string
	Transactions   []*TransactionData
	TotalTxCount   int
}

// AddressInfo contains comprehensive address information
type AddressInfo struct {
	Address         string
	Type            string // "account", "contract", "unknown"
	Balance         int64
	Interactions    []string // List of interaction types
	TxCount         int
	IncomingTx      int
	OutgoingTx      int
	ContractCalls   int
	ContractAddress string // If this is a contract interaction
}

// TransactionData contains detailed transaction information
type TransactionData struct {
	TxHash         string
	TxID           string
	FromAddress    string
	ToAddress      string
	Amount         int64
	ContractType   string
	Success        bool
	EnergyUsage    int64
	EnergyFee      int64
	NetUsage       int64
	NetFee         int64
	ContractData   map[string]interface{}
	Logs           []map[string]interface{}
	InternalTxs    []map[string]interface{}
	RawTransaction *core.Transaction
	RawTxInfo      *core.TransactionInfo
}

// BlockMonitorConfig holds block monitor configuration
type BlockMonitorConfig struct {
	PollInterval time.Duration
	StartBlock   int64
}

// NewBlockMonitor creates a new comprehensive block monitor
func NewBlockMonitor(
	tronClient *client.TronClient,
	cfg BlockMonitorConfig,
	log *logger.Logger,
) (*BlockMonitor, error) {
	if log == nil {
		defaultLog := logger.NewDefault()
		log = &defaultLog
	}

	if cfg.PollInterval == 0 {
		cfg.PollInterval = 3 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	monitor := &BlockMonitor{
		client:       tronClient,
		parser:       parser.NewTronParser(log),
		logger:       log,
		pollInterval: cfg.PollInterval,
		lastBlockNum: cfg.StartBlock,
		ctx:          ctx,
		cancel:       cancel,
		eventChannel: make(chan *BlockEvent, 100),
	}

	log.Info().
		Int64("startBlock", cfg.StartBlock).
		Dur("pollInterval", cfg.PollInterval).
		Msg("Block monitor initialized")

	return monitor, nil
}

// Start begins monitoring the blockchain
func (m *BlockMonitor) Start() error {
	m.mu.Lock()
	if m.isRunning {
		m.mu.Unlock()
		return fmt.Errorf("monitor is already running")
	}
	m.isRunning = true
	m.mu.Unlock()

	// If starting from block 0, get current block
	if m.lastBlockNum == 0 {
		block, err := m.client.GetNowBlock(m.ctx)
		if err != nil {
			return fmt.Errorf("failed to get current block: %w", err)
		}
		m.lastBlockNum = block.GetBlockHeader().GetRawData().GetNumber()
		m.logger.Info().
			Int64("startBlock", m.lastBlockNum).
			Msg("Starting from current block")
	}

	m.logger.Info().Msg("Starting comprehensive block monitor")

	go m.monitorLoop()

	return nil
}

// Stop stops the monitor
func (m *BlockMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isRunning {
		return
	}

	m.logger.Info().Msg("Stopping block monitor")
	m.cancel()
	m.isRunning = false
	close(m.eventChannel)
}

// Events returns the channel for receiving block events
func (m *BlockMonitor) Events() <-chan *BlockEvent {
	return m.eventChannel
}

// GetLastBlockNumber returns the last processed block number
func (m *BlockMonitor) GetLastBlockNumber() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastBlockNum
}

// monitorLoop continuously polls for new blocks
func (m *BlockMonitor) monitorLoop() {
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
			}
		}
	}
}

// processNewBlocks checks for and processes new blocks
func (m *BlockMonitor) processNewBlocks() error {
	currentBlock, err := m.client.GetNowBlock(m.ctx)
	if err != nil {
		return fmt.Errorf("failed to get current block: %w", err)
	}

	currentBlockNum := currentBlock.GetBlockHeader().GetRawData().GetNumber()

	m.mu.RLock()
	lastProcessed := m.lastBlockNum
	m.mu.RUnlock()

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
				continue
			}

			m.mu.Lock()
			m.lastBlockNum = blockNum
			m.mu.Unlock()
		}
	}

	return nil
}

// processBlock processes a single block and extracts all address information
func (m *BlockMonitor) processBlock(blockNum int64) error {
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

	// Create block event
	blockEvent := &BlockEvent{
		BlockNumber:    blockNum,
		BlockHash:      blockHash,
		BlockTimestamp: blockHeader.GetTimestamp(),
		Addresses:      make(map[string]*AddressInfo),
		Transactions:   make([]*TransactionData, 0, len(transactions)),
		TotalTxCount:   len(transactions),
	}

	// Process each transaction
	for _, tx := range transactions {
		txData, err := m.extractTransactionData(ctx, block, tx)
		if err != nil {
			m.logger.Error().
				Err(err).
				Msg("Error extracting transaction data")
			continue
		}

		if txData != nil {
			blockEvent.Transactions = append(blockEvent.Transactions, txData)

			// Update address information
			m.updateAddressInfo(blockEvent.Addresses, txData)
		}
	}

	// Send event to channel (non-blocking)
	select {
	case m.eventChannel <- blockEvent:
		m.logger.Info().
			Int64("block", blockNum).
			Int("txCount", len(blockEvent.Transactions)).
			Int("uniqueAddresses", len(blockEvent.Addresses)).
			Msg("Block processed")
	default:
		m.logger.Warn().Msg("Event channel full, dropping block event")
	}

	return nil
}

// extractTransactionData extracts comprehensive transaction data
func (m *BlockMonitor) extractTransactionData(ctx context.Context, block *core.Block, tx *core.Transaction) (*TransactionData, error) {
	if tx == nil || tx.GetRawData() == nil {
		return nil, fmt.Errorf("invalid transaction")
	}

	// Calculate the correct transaction ID (SHA256 of raw transaction data)
	txID := m.parser.CalculateTransactionID(tx)
	if txID == "" {
		return nil, fmt.Errorf("failed to calculate transaction ID")
	}

	// Get transaction info (includes events, logs, receipt)
	txInfoCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	txInfo, err := m.client.GetTransactionInfoById(txInfoCtx, txID)
	if err != nil {
		m.logger.Debug().
			Err(err).
			Str("txID", txID).
			Msg("Failed to get transaction info, using basic data")
		txInfo = nil
	}

	txData := &TransactionData{
		TxID:           txID,
		TxHash:         txID,
		RawTransaction: tx,
		RawTxInfo:      txInfo,
		Success:        true,
		ContractData:   make(map[string]interface{}),
	}

	txRaw := tx.GetRawData()

	// Extract contract details
	if len(txRaw.GetContract()) > 0 {
		contract := txRaw.GetContract()[0]
		txData.ContractType = contract.GetType().String()

		from, to, amount := m.parser.ParseContract(contract)
		txData.FromAddress = from
		txData.ToAddress = to
		txData.Amount = amount

		// Extract additional contract data
		txData.ContractData["type"] = txData.ContractType
		txData.ContractData["parameter"] = contract.GetParameter()

		// Decode smart contract call data if available
		if decoded := m.parser.DecodeSmartContract(contract); decoded != nil {
			txData.ContractData["smartContract"] = map[string]interface{}{
				"methodSignature": decoded.MethodSignature,
				"methodName":      decoded.MethodName,
				"addresses":       decoded.Addresses,
				"parameters":      decoded.Parameters,
			}
			if decoded.Amount != nil {
				txData.ContractData["smartContract"].(map[string]interface{})["amount"] = decoded.Amount.String()
			}
		}
	}

	// Extract transaction info details
	if txInfo != nil {
		txData.Success = txInfo.GetResult() == core.TransactionInfo_SUCESS

		// Extract logs
		if len(txInfo.GetLog()) > 0 {
			txData.Logs = m.parser.ParseLogs(txInfo.GetLog())
		}

		// Extract receipt info
		if txInfo.GetReceipt() != nil {
			receipt := txInfo.GetReceipt()
			txData.EnergyUsage = receipt.GetEnergyUsage()
			txData.EnergyFee = receipt.GetEnergyFee()
			txData.NetUsage = receipt.GetNetUsage()
			txData.NetFee = receipt.GetNetFee()
		}

		// Extract internal transactions
		txData.InternalTxs = m.parser.ParseInternalTransactions(txInfo)
	}

	return txData, nil
}

// updateAddressInfo updates address information based on transaction data
func (m *BlockMonitor) updateAddressInfo(addresses map[string]*AddressInfo, txData *TransactionData) {
	// Update from address
	if txData.FromAddress != "" && txData.FromAddress != "0" {
		addr := m.getOrCreateAddressInfo(addresses, txData.FromAddress)
		addr.TxCount++
		addr.OutgoingTx++
		addr.Interactions = append(addr.Interactions, fmt.Sprintf("sent_%s", txData.ContractType))
	}

	// Update to address
	if txData.ToAddress != "" && txData.ToAddress != "0" {
		addr := m.getOrCreateAddressInfo(addresses, txData.ToAddress)
		addr.TxCount++
		addr.IncomingTx++
		addr.Interactions = append(addr.Interactions, fmt.Sprintf("received_%s", txData.ContractType))

		// Check if it's a contract
		if txData.ContractType == "TriggerSmartContract" || txData.ContractType == "CreateSmartContract" {
			addr.Type = "contract"
			addr.ContractCalls++
			addr.ContractAddress = txData.ToAddress
		}
	}

	// Update addresses from logs (contract events)
	for _, log := range txData.Logs {
		if logAddr, ok := log["address"].(string); ok && logAddr != "" {
			addr := m.getOrCreateAddressInfo(addresses, logAddr)
			addr.Type = "contract"
			addr.Interactions = append(addr.Interactions, "contract_event")
		}
	}

	// Update addresses from internal transactions
	for _, itx := range txData.InternalTxs {
		if fromAddr, ok := itx["from"].(string); ok && fromAddr != "" {
			addr := m.getOrCreateAddressInfo(addresses, fromAddr)
			addr.Interactions = append(addr.Interactions, "internal_tx_from")
		}
		if toAddr, ok := itx["to"].(string); ok && toAddr != "" {
			addr := m.getOrCreateAddressInfo(addresses, toAddr)
			addr.Interactions = append(addr.Interactions, "internal_tx_to")
		}
	}
}

// getOrCreateAddressInfo gets or creates address info
func (m *BlockMonitor) getOrCreateAddressInfo(addresses map[string]*AddressInfo, address string) *AddressInfo {
	if addr, exists := addresses[address]; exists {
		return addr
	}

	addr := &AddressInfo{
		Address:      address,
		Type:         "account",
		Interactions: make([]string, 0),
	}
	addresses[address] = addr
	return addr
}
