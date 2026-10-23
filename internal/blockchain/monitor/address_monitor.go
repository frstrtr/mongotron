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

// AddressMonitor watches blockchain for a specific address.
//
// With Config.Scanner set it reads blocks from that shared BlockScanner (each block is
// fetched and parsed once for every watched address); otherwise it polls the node on
// its own, one block fetch per block per watched address.
type AddressMonitor struct {
	client          BlockSource
	scanner         *BlockScanner
	feed            *scanFeed
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
	// RawTransaction and RawTxInfo may be shared by the events of every watched
	// address in the same transaction: consumers must not modify them.
	RawTransaction *core.Transaction
	RawTxInfo      *core.TransactionInfo
}

// Config holds address monitor configuration
type Config struct {
	WatchAddress string
	PollInterval time.Duration // own polling only; a shared scanner has its own interval
	StartBlock   int64         // last block already covered; 0 or negative means start from the current block
	// Scanner, when set, makes the monitor read blocks from this shared scanner instead
	// of polling the node itself.
	Scanner *BlockScanner
}

// NewAddressMonitor creates a new address monitor
func NewAddressMonitor(
	tronClient *client.TronClient,
	cfg Config,
	log *logger.Logger,
) (*AddressMonitor, error) {
	var src BlockSource
	if tronClient != nil {
		src = tronClient
	}
	return NewAddressMonitorFromSource(src, cfg, log)
}

// NewAddressMonitorFromSource creates an address monitor reading from any BlockSource.
// With cfg.Scanner set and src nil, the scanner's source is used for the monitor's own
// reads (catch-up and historical scans).
func NewAddressMonitorFromSource(
	src BlockSource,
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

	if src == nil && cfg.Scanner != nil {
		src = cfg.Scanner.src
	}

	monitor := &AddressMonitor{
		client:       src,
		scanner:      cfg.Scanner,
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
		Bool("sharedScanner", cfg.Scanner != nil).
		Msg("Address monitor initialized")

	return monitor, nil
}

// Start begins monitoring the blockchain. A stopped monitor cannot be started again:
// create a new one.
func (m *AddressMonitor) Start() error {
	m.mu.Lock()
	if m.isRunning {
		m.mu.Unlock()
		return fmt.Errorf("monitor is already running")
	}
	if m.ctx.Err() != nil {
		m.mu.Unlock()
		return fmt.Errorf("monitor was stopped")
	}
	m.isRunning = true
	if m.scanner != nil {
		// Shared scanner: no node read here; the scanner places the monitor.
		feed := m.scanner.subscribe(m.watchAddress, m.watchAddressHex, m.lastBlockNum)
		m.feed = feed
		m.mu.Unlock()
		m.logger.Info().Str("address", m.watchAddress).Msg("Starting address monitor on the shared block scanner")
		go m.feedLoop(feed)
		return nil
	}
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
	// Cancel first: it wakes a send that waits for room in the event channel, which
	// holds the read lock Stop needs.
	m.cancel()

	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isRunning {
		return
	}

	m.logger.Info().Msg("Stopping address monitor")
	if m.scanner != nil && m.feed != nil {
		m.scanner.unsubscribe(m.feed)
	}
	m.isRunning = false
	close(m.eventChannel)
}

// emit hands an event to the channel and reports whether it was delivered. It never
// sends after Stop closed the channel.
//
// On the shared scanner it waits for room in the channel (until the monitor stops):
// the scanner counts a match as handled once it was handed over, so dropping it
// would let the stored block pass an event that was never routed. Waiting holds back
// only this address's queue, never the scanner or other addresses. A polling monitor
// keeps the non-blocking send of before (a full channel drops the event).
func (m *AddressMonitor) emit(event *AddressEvent) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.isRunning {
		return false
	}
	if m.scanner != nil {
		select {
		case m.eventChannel <- event:
			m.logDetected(event)
			return true
		case <-m.ctx.Done():
			m.logger.Warn().
				Str("address", m.watchAddress).
				Int64("block", event.BlockNumber).
				Str("txHash", event.TransactionHash).
				Msg("Monitor stopping: event not delivered (the stored block stays before it)")
			return false
		}
	}
	select {
	case m.eventChannel <- event:
		m.logDetected(event)
		return true
	default:
		m.logger.Warn().
			Str("address", m.watchAddress).
			Int64("block", event.BlockNumber).
			Str("txHash", event.TransactionHash).
			Msg("Event channel full, dropping event")
		return false
	}
}

func (m *AddressMonitor) logDetected(event *AddressEvent) {
	m.logger.Info().
		Int64("block", event.BlockNumber).
		Str("txHash", event.TransactionHash).
		Str("from", event.From).
		Str("to", event.To).
		Int64("amount", event.Amount).
		Str("type", event.ContractType).
		Msg("Address event detected")
}

// Events returns the channel for receiving address events
func (m *AddressMonitor) Events() <-chan *AddressEvent {
	return m.eventChannel
}

// GetLastBlockNumber returns the last processed block number. On a shared scanner it
// never passes a block whose matches for this address are still being handled.
func (m *AddressMonitor) GetLastBlockNumber() int64 {
	m.mu.RLock()
	feed, last := m.feed, m.lastBlockNum
	m.mu.RUnlock()
	if feed != nil {
		return feed.lastProcessed()
	}
	return last
}

// feedLoop handles the matches the shared scanner queues for this address, after a
// catch-up of its own when the address started behind the scanner.
func (m *AddressMonitor) feedLoop(feed *scanFeed) {
	for !feed.isResolved() {
		select {
		case <-m.ctx.Done():
			return
		case <-feed.notify:
		}
	}

	if from, to, ok := feed.catchUpRange(); ok {
		m.logger.Info().
			Str("address", m.watchAddress).
			Int64("from", from).
			Int64("to", to).
			Msg("Address is behind the shared scanner: catching up on its own first")
		if !m.catchUp(feed, from, to) {
			// Stopped before the range was done: the progress stays at the last
			// block the catch-up finished.
			return
		}
		feed.finishCatchUp()
	}

	for {
		for {
			if m.ctx.Err() != nil {
				return
			}
			it, ok := feed.next()
			if !ok {
				break
			}
			event, err := it.tx.eventCopy()
			if err != nil {
				m.logger.Error().
					Err(err).
					Int64("block", it.block).
					Msg("Error extracting event from transaction")
			} else if event != nil && !m.emit(event) {
				// Stopping: the match stays unhandled, so the progress stays before it.
				return
			}
			feed.done()
		}
		select {
		case <-m.ctx.Done():
			return
		case <-feed.notify:
		}
	}
}

// catchUp reads blocks from..to on its own (the scanner is already past them), with the
// scanner's policy: a block that fails is retried one poll interval later; a failure
// counts only while the node answers the head (a node that is down never makes the
// catch-up skip a block); after MaxBlockRetries counted failures the block is skipped
// and reported as a gap of this address (ScannerConfig.OnSkip). It reports whether the
// whole range was done; false when the monitor stopped first.
func (m *AddressMonitor) catchUp(feed *scanFeed, from, to int64) bool {
	sc := m.scanner
	for blockNum := from; blockNum <= to; blockNum++ {
		failures := 0
		for {
			if m.ctx.Err() != nil {
				return false
			}
			sc.catchUpFetches.Add(1)
			err := m.processBlock(blockNum)
			if m.ctx.Err() != nil {
				// Stopped while the block was handled: it does not count as done
				return false
			}
			if err == nil {
				break
			}
			if sc.nodeAnswers(m.ctx) {
				failures++
			}
			if failures >= sc.cfg.MaxBlockRetries {
				sc.reportSkip(blockNum, m.watchAddress, failures, err)
				break
			}
			m.logger.Warn().
				Err(err).
				Str("address", m.watchAddress).
				Int64("block", blockNum).
				Int("countedFailures", failures).
				Msg("Catch-up: block fetch failed, retrying after the poll interval")
			select {
			case <-m.ctx.Done():
				return false
			case <-time.After(sc.cfg.PollInterval):
			}
		}
		feed.catchUpProgress(blockNum)
	}
	return true
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
				m.emit(event)
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
	return buildAddressEvent(m.ctx, m.client, m.parser, m.logger, tx)
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

// ScanBlock hands the events of the watched address in an already fetched block to
// callback, in block order, and returns how many there were. It reads the transaction
// info of each matching transaction but never the block itself.
func (m *AddressMonitor) ScanBlock(block *core.Block, callback func(*AddressEvent)) int {
	if block == nil || block.GetBlockHeader() == nil {
		return 0
	}
	blockNum := block.GetBlockHeader().GetRawData().GetNumber()
	blockHash := blockHashOf(block)
	ts := block.GetBlockHeader().GetRawData().GetTimestamp()
	found := 0
	for _, tx := range block.GetTransactions() {
		if !m.isAddressInTransaction(tx) {
			continue
		}
		event, err := m.extractEvent(block, tx)
		if err != nil {
			m.logger.Warn().
				Err(err).
				Str("txID", m.parser.CalculateTransactionID(tx)).
				Int64("block", blockNum).
				Msg("Error extracting event during historical scan")
			continue
		}
		if event == nil {
			continue
		}
		event.BlockNumber = blockNum
		event.BlockHash = blockHash
		event.BlockTimestamp = ts
		if callback != nil {
			callback(event)
		}
		found++
		m.logger.Info().
			Int64("block", blockNum).
			Str("txHash", event.TransactionHash).
			Str("type", event.ContractType).
			Msg("Historical event found")
	}
	return found
}

// ScanHistorical scans a range of historical blocks for the watched address
// This is used to catch up on past transactions when a new wallet is added
// The callback function is called for each event found
func (m *AddressMonitor) ScanHistorical(fromBlock, toBlock int64, callback func(*AddressEvent)) error {
	m.logger.Info().
		Int64("fromBlock", fromBlock).
		Int64("toBlock", toBlock).
		Str("address", m.watchAddress).
		Msg("Starting historical block scan")

	// If toBlock is 0 or negative, get current block
	if toBlock <= 0 {
		currentBlock, err := m.client.GetNowBlock(m.ctx)
		if err != nil {
			return fmt.Errorf("failed to get current block: %w", err)
		}
		toBlock = currentBlock.GetBlockHeader().GetRawData().GetNumber()
	}

	if fromBlock > toBlock {
		return fmt.Errorf("fromBlock (%d) cannot be greater than toBlock (%d)", fromBlock, toBlock)
	}

	totalBlocks := toBlock - fromBlock + 1
	processedBlocks := int64(0)
	eventsFound := int64(0)

	m.logger.Info().
		Int64("totalBlocks", totalBlocks).
		Msg("Historical scan: processing blocks")

	for blockNum := fromBlock; blockNum <= toBlock; blockNum++ {
		// Check if context is cancelled
		select {
		case <-m.ctx.Done():
			m.logger.Warn().
				Int64("processedBlocks", processedBlocks).
				Int64("eventsFound", eventsFound).
				Msg("Historical scan cancelled")
			return m.ctx.Err()
		default:
		}

		// Get block with timeout
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		block, err := m.client.GetBlockByNum(ctx, blockNum)
		cancel()

		if err != nil {
			m.logger.Warn().
				Err(err).
				Int64("block", blockNum).
				Msg("Failed to get block during historical scan, continuing...")
			continue
		}

		if block == nil || block.GetBlockHeader() == nil {
			continue
		}

		eventsFound += int64(m.ScanBlock(block, callback))

		processedBlocks++

		// Log progress every 100 blocks
		if processedBlocks%100 == 0 {
			progress := float64(processedBlocks) / float64(totalBlocks) * 100
			m.logger.Info().
				Int64("processedBlocks", processedBlocks).
				Int64("totalBlocks", totalBlocks).
				Float64("progress", progress).
				Int64("eventsFound", eventsFound).
				Msg("Historical scan progress")
		}

		// Small delay to avoid hammering the node (10ms between blocks)
		time.Sleep(10 * time.Millisecond)
	}

	m.logger.Info().
		Int64("processedBlocks", processedBlocks).
		Int64("eventsFound", eventsFound).
		Str("address", m.watchAddress).
		Msg("Historical block scan completed")

	return nil
}
