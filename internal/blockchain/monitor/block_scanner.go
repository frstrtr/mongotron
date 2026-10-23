package monitor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/frstrtr/mongotron/internal/blockchain/client"
	"github.com/frstrtr/mongotron/internal/blockchain/parser"
	"github.com/frstrtr/mongotron/pkg/logger"
)

const (
	// DefaultScannerPollInterval is how often the scanner reads the head (TRON block time).
	DefaultScannerPollInterval = 3 * time.Second
	// DefaultCatchUpWindow is how far (in blocks, about one hour) a watched address may be
	// behind the newest one at startup and still be caught up by the shared scanner.
	DefaultCatchUpWindow int64 = 1200
	// DefaultMaxBlockRetries is how many times in a row the scanner retries one block
	// before it skips it (the per-address monitors skipped a failed block at once).
	DefaultMaxBlockRetries = 10

	// catchUpAttempts is how many times a monitor catching up on its own tries one block.
	catchUpAttempts   = 3
	catchUpRetryDelay = time.Second
	// statsLogEvery is how often (in scanned blocks, about one hour) the scanner logs its counters.
	statsLogEvery = 1200
)

// ScannerConfig configures a BlockScanner. Zero values take the defaults.
type ScannerConfig struct {
	// PollInterval is how often the head is read (default 3 s).
	PollInterval time.Duration
	// CatchUpWindow: at the first head read, the watched addresses whose last processed
	// block is at most CatchUpWindow blocks behind the newest one are caught up together
	// by the scanner (one fetch per block for all of them); an address further behind
	// catches up on its own and then joins. Default 1200 blocks.
	CatchUpWindow int64
	// MaxBlockRetries is how many consecutive failed fetches of one block the scanner
	// accepts before it skips that block (default 10).
	MaxBlockRetries int
}

// ScannerStats are the scanner's counters since it was created.
type ScannerStats struct {
	// HeadReads counts head polls; HeaderOnlyReads of them were header-only (a few
	// hundred bytes) and FullHeadReads downloaded the whole head block (GetNowBlock).
	HeadReads       int64 `json:"headReads"`
	HeaderOnlyReads int64 `json:"headerOnlyReads"`
	FullHeadReads   int64 `json:"fullHeadReads"`
	// BlockFetches counts GetBlockByNum calls of the scanner itself.
	BlockFetches int64 `json:"blockFetches"`
	// BlocksScanned counts blocks parsed and fanned out (fetched by number or taken
	// from a full head read).
	BlocksScanned int64 `json:"blocksScanned"`
	// CatchUpFetches counts GetBlockByNum calls of monitors catching up on their own.
	CatchUpFetches int64 `json:"catchUpFetches"`
	// SkippedBlocks counts blocks given up after MaxBlockRetries failed fetches.
	SkippedBlocks int64 `json:"skippedBlocks"`
	// Matches counts (transaction, watched address) deliveries; TxEvents counts the
	// events built from them (one transaction info read per matched transaction, shared
	// by every watched address in it).
	Matches  int64 `json:"matches"`
	TxEvents int64 `json:"txEvents"`
	// Subscribers is the number of watched addresses reading from the scanner.
	Subscribers int `json:"subscribers"`
	// Cursor is the last block fanned out, Head the last head seen.
	Cursor int64 `json:"cursor"`
	Head   int64 `json:"head"`
}

// BlockScanner reads every block once and fans its transactions out to all the
// address monitors subscribed to it, so the number of watched addresses does not
// multiply the load on the node: N watched addresses cost one block fetch per block,
// not N. Monitors keep their own event channel, progress and filters (see
// AddressMonitor with Config.Scanner).
type BlockScanner struct {
	src    BlockSource
	parser *parser.TronParser
	logger *logger.Logger
	cfg    ScannerConfig

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	mu          sync.Mutex
	running     bool
	feeds       map[*scanFeed]struct{}
	index       map[string]map[*scanFeed]struct{} // key: base58 and hex form of each watched address
	initialized bool
	cursor      int64
	head        int64
	failBlock   int64
	failCount   int
	headerOff   bool // the node does not implement the header-only head read

	dispatched atomic.Int64 // last block fanned out (read by monitors without the lock)

	headReads, headerOnlyReads, fullHeadReads   atomic.Int64
	blockFetches, blocksScanned, catchUpFetches atomic.Int64
	skippedBlocks, matches, txEvents            atomic.Int64
}

// NewBlockScanner creates a scanner reading from src. It does nothing until Start.
func NewBlockScanner(src BlockSource, cfg ScannerConfig, log *logger.Logger) *BlockScanner {
	if log == nil {
		defaultLog := logger.NewDefault()
		log = &defaultLog
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = DefaultScannerPollInterval
	}
	if cfg.CatchUpWindow <= 0 {
		cfg.CatchUpWindow = DefaultCatchUpWindow
	}
	if cfg.MaxBlockRetries <= 0 {
		cfg.MaxBlockRetries = DefaultMaxBlockRetries
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &BlockScanner{
		src:    src,
		parser: parser.NewTronParser(log),
		logger: log,
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
		feeds:  make(map[*scanFeed]struct{}),
		index:  make(map[string]map[*scanFeed]struct{}),
	}
}

// Start runs the scan loop in the background. The first head read places every
// monitor that subscribed before it (see ScannerConfig.CatchUpWindow).
func (s *BlockScanner) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return fmt.Errorf("block scanner is already running")
	}
	if s.ctx.Err() != nil {
		return fmt.Errorf("block scanner was stopped")
	}
	s.running = true
	go s.loop()
	s.logger.Info().
		Dur("pollInterval", s.cfg.PollInterval).
		Int64("catchUpWindow", s.cfg.CatchUpWindow).
		Int("subscribers", len(s.feeds)).
		Msg("Shared block scanner started")
	return nil
}

// Stop ends the scan loop and waits for it.
func (s *BlockScanner) Stop() {
	s.mu.Lock()
	running := s.running
	s.running = false
	s.mu.Unlock()
	s.cancel()
	if running {
		<-s.done
	}
}

// Stats returns the scanner's counters.
func (s *BlockScanner) Stats() ScannerStats {
	s.mu.Lock()
	subscribers, cursor, head := len(s.feeds), s.cursor, s.head
	s.mu.Unlock()
	return ScannerStats{
		HeadReads:       s.headReads.Load(),
		HeaderOnlyReads: s.headerOnlyReads.Load(),
		FullHeadReads:   s.fullHeadReads.Load(),
		BlockFetches:    s.blockFetches.Load(),
		BlocksScanned:   s.blocksScanned.Load(),
		CatchUpFetches:  s.catchUpFetches.Load(),
		SkippedBlocks:   s.skippedBlocks.Load(),
		Matches:         s.matches.Load(),
		TxEvents:        s.txEvents.Load(),
		Subscribers:     subscribers,
		Cursor:          cursor,
		Head:            head,
	}
}

func (s *BlockScanner) loop() {
	defer close(s.done)
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()
	for {
		if err := s.scanOnce(s.ctx); err != nil && s.ctx.Err() == nil {
			s.logger.Error().Err(err).Msg("Block scanner: error processing new blocks")
		}
		select {
		case <-s.ctx.Done():
			s.logger.Info().Msg("Shared block scanner stopped")
			return
		case <-ticker.C:
		}
	}
}

// readHead returns the head block number and, when it had to download the head block
// to learn it, the block itself (so it is not fetched a second time).
func (s *BlockScanner) readHead(ctx context.Context) (int64, *core.Block, error) {
	s.headReads.Add(1)
	s.mu.Lock()
	tryHeader := !s.headerOff
	s.mu.Unlock()
	if hr, ok := s.src.(headReader); ok && tryHeader {
		num, err := hr.GetHeadBlockNumber(ctx)
		if err == nil {
			s.headerOnlyReads.Add(1)
			return num, nil, nil
		}
		if errors.Is(err, client.ErrHeaderOnlyUnsupported) {
			s.mu.Lock()
			s.headerOff = true
			s.mu.Unlock()
			s.logger.Warn().Err(err).Msg("Node has no header-only head read: polling the full head block instead")
		} else {
			s.logger.Debug().Err(err).Msg("Header-only head read failed: reading the full head block this time")
		}
	}

	block, err := s.src.GetNowBlock(ctx)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to get current block: %w", err)
	}
	if block == nil || block.GetBlockHeader() == nil {
		return 0, nil, fmt.Errorf("received nil head block")
	}
	s.fullHeadReads.Add(1)
	return block.GetBlockHeader().GetRawData().GetNumber(), block, nil
}

// scanOnce reads the head and fans out every block after the cursor up to it. A block
// that cannot be fetched stops the pass; the next pass starts from it again.
func (s *BlockScanner) scanOnce(ctx context.Context) error {
	head, headBlock, err := s.readHead(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	if head > s.head {
		s.head = head
	}
	var placed []*scanFeed
	if !s.initialized {
		placed = s.initLocked(head)
	}
	if len(s.feeds) == 0 {
		// Nobody to deliver to: follow the head without downloading blocks.
		if head > s.cursor {
			s.cursor = head
			s.dispatched.Store(head)
		}
		s.mu.Unlock()
		return nil
	}
	from := s.cursor + 1
	s.mu.Unlock()
	for _, f := range placed {
		f.signal()
	}

	for n := from; n <= head; n++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		block := headBlock
		if headBlock == nil || n != head {
			block, err = s.fetchBlock(ctx, n)
			if err != nil {
				return s.blockFailed(n, err)
			}
		}
		s.dispatchBlock(n, block)
	}
	return nil
}

func (s *BlockScanner) fetchBlock(ctx context.Context, num int64) (*core.Block, error) {
	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	s.blockFetches.Add(1)
	block, err := s.src.GetBlockByNum(fetchCtx, num)
	if err != nil {
		return nil, err
	}
	if block == nil || block.GetBlockHeader() == nil {
		return nil, fmt.Errorf("received nil block or header for block %d", num)
	}
	return block, nil
}

// blockFailed records a failed fetch of block num. After MaxBlockRetries failures in a
// row the block is skipped (logged as an error) so one bad block cannot stop the scanner.
func (s *BlockScanner) blockFailed(num int64, err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failBlock == num {
		s.failCount++
	} else {
		s.failBlock, s.failCount = num, 1
	}
	if s.failCount < s.cfg.MaxBlockRetries {
		return fmt.Errorf("block %d (attempt %d of %d): %w", num, s.failCount, s.cfg.MaxBlockRetries, err)
	}
	s.failCount = 0
	if num > s.cursor {
		s.cursor = num
		s.dispatched.Store(num)
	}
	s.skippedBlocks.Add(1)
	return fmt.Errorf("block %d skipped after %d failed fetches: %w", num, s.cfg.MaxBlockRetries, err)
}

// dispatchBlock matches every transaction of the block against the watched addresses
// once and queues each match on the monitors concerned.
func (s *BlockScanner) dispatchBlock(num int64, block *core.Block) {
	hash := blockHashOf(block)
	ts := block.GetBlockHeader().GetRawData().GetTimestamp()

	s.mu.Lock()
	if num <= s.cursor {
		s.mu.Unlock()
		return
	}
	var touched []*scanFeed
	if len(s.index) > 0 {
		for _, tx := range block.GetTransactions() {
			matched := s.matchLocked(tx)
			if len(matched) == 0 {
				continue
			}
			shared := &sharedTx{scanner: s, tx: tx, block: num, hash: hash, ts: ts}
			for _, f := range matched {
				if f.push(num, shared) {
					s.matches.Add(1)
					touched = appendFeed(touched, f)
				}
			}
		}
	}
	s.cursor = num
	s.dispatched.Store(num)
	if s.failBlock == num {
		s.failCount = 0
	}
	scanned := s.blocksScanned.Add(1)
	subscribers := len(s.feeds)
	s.mu.Unlock()

	for _, f := range touched {
		f.signal()
	}
	if scanned%statsLogEvery == 0 {
		st := s.Stats()
		s.logger.Info().
			Int64("cursor", num).
			Int("subscribers", subscribers).
			Int64("blocksScanned", st.BlocksScanned).
			Int64("blockFetches", st.BlockFetches).
			Int64("headReads", st.HeadReads).
			Int64("catchUpFetches", st.CatchUpFetches).
			Int64("matches", st.Matches).
			Msg("Shared block scanner progress")
	}
}

// matchLocked returns the feeds watching an address of tx (caller holds s.mu).
func (s *BlockScanner) matchLocked(tx *core.Transaction) []*scanFeed {
	if tx == nil || tx.GetRawData() == nil {
		return nil
	}
	var out []*scanFeed
	for _, contract := range tx.GetRawData().GetContract() {
		for _, addr := range s.parser.ExtractAddresses(contract) {
			for f := range s.index[addr] {
				out = appendFeed(out, f)
			}
		}
	}
	return out
}

func appendFeed(list []*scanFeed, f *scanFeed) []*scanFeed {
	for _, existing := range list {
		if existing == f {
			return list
		}
	}
	return append(list, f)
}

// initLocked places the monitors that subscribed before the first head read and sets
// the cursor (caller holds s.mu). It returns the feeds to wake.
func (s *BlockScanner) initLocked(head int64) []*scanFeed {
	newest := int64(0)
	for f := range s.feeds {
		if f.initial > newest {
			newest = f.initial
		}
	}
	start := head
	if newest > 0 {
		if newest < start {
			start = newest
		}
		for f := range s.feeds {
			if f.initial > 0 && f.initial >= newest-s.cfg.CatchUpWindow && f.initial < start {
				start = f.initial
			}
		}
	}
	s.cursor = start
	s.dispatched.Store(start)
	s.initialized = true

	placed := make([]*scanFeed, 0, len(s.feeds))
	for f := range s.feeds {
		s.placeLocked(f)
		placed = append(placed, f)
	}
	s.logger.Info().
		Int64("head", head).
		Int64("startAfterBlock", start).
		Int("subscribers", len(s.feeds)).
		Msg("Shared block scanner positioned")
	return placed
}

// placeLocked decides from which block the scanner delivers to f (caller holds s.mu
// and the scanner is initialized):
//   - no start block (0 or negative): from the block after the current head, as a
//     monitor started now would;
//   - a start at or after the cursor: from the block after it;
//   - a start behind the cursor: the monitor first catches up on its own up to the
//     cursor, then the scanner delivers from the block after the cursor.
func (s *BlockScanner) placeLocked(f *scanFeed) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case f.initial <= 0:
		f.skipThrough = s.cursor
		if s.head > f.skipThrough {
			f.skipThrough = s.head
		}
	case f.initial >= s.cursor:
		f.skipThrough = f.initial
	default:
		f.catchFrom, f.catchTo = f.initial+1, s.cursor
		f.catching = true
		f.catchDone = f.initial
		f.skipThrough = s.cursor
	}
	f.resolved = true
}

// subscribe registers a watched address. lastProcessed is the last block the monitor
// already covered (0 or negative: start from the current head).
func (s *BlockScanner) subscribe(address, hexAddress string, lastProcessed int64) *scanFeed {
	f := &scanFeed{
		scanner: s,
		address: address,
		hexAddr: hexAddress,
		initial: lastProcessed,
		notify:  make(chan struct{}, 1),
	}
	s.mu.Lock()
	s.feeds[f] = struct{}{}
	for _, key := range []string{address, hexAddress} {
		if key == "" {
			continue
		}
		if s.index[key] == nil {
			s.index[key] = make(map[*scanFeed]struct{})
		}
		s.index[key][f] = struct{}{}
	}
	placed := s.initialized
	if placed {
		s.placeLocked(f)
	}
	s.mu.Unlock()
	if placed {
		f.signal()
	}
	return f
}

// unsubscribe removes a feed; nothing more is delivered to it.
func (s *BlockScanner) unsubscribe(f *scanFeed) {
	s.mu.Lock()
	delete(s.feeds, f)
	for _, key := range []string{f.address, f.hexAddr} {
		if set := s.index[key]; set != nil {
			delete(set, f)
			if len(set) == 0 {
				delete(s.index, key)
			}
		}
	}
	s.mu.Unlock()
	f.close()
}

// sharedTx is a matched transaction shared by every watched address it involves: its
// event (and the node read of its transaction info) is built once, by the first
// monitor that needs it, outside the scan loop.
type sharedTx struct {
	scanner *BlockScanner
	tx      *core.Transaction
	block   int64
	hash    string
	ts      int64

	once  sync.Once
	event *AddressEvent
	err   error
}

// eventCopy returns a copy of the transaction's event for one monitor.
func (t *sharedTx) eventCopy() (*AddressEvent, error) {
	t.once.Do(func() {
		s := t.scanner
		t.event, t.err = buildAddressEvent(s.ctx, s.src, s.parser, s.logger, t.tx)
		if t.event != nil {
			t.event.BlockNumber = t.block
			t.event.BlockHash = t.hash
			t.event.BlockTimestamp = t.ts
			s.txEvents.Add(1)
		}
	})
	if t.err != nil {
		return nil, t.err
	}
	cp := *t.event
	return &cp, nil
}

type feedItem struct {
	block int64
	tx    *sharedTx
}

// scanFeed is one monitor's queue of matches. The scanner appends without waiting for
// the monitor, so a slow monitor never holds back the scan or the other monitors.
type scanFeed struct {
	scanner *BlockScanner
	address string
	hexAddr string
	notify  chan struct{}

	mu          sync.Mutex
	initial     int64 // last processed block given at subscription
	resolved    bool  // the scanner decided where this feed starts
	skipThrough int64 // blocks up to here are not delivered by the scanner
	catching    bool  // the monitor is catching up on its own
	catchFrom   int64
	catchTo     int64
	catchDone   int64 // last block the catch-up finished
	items       []feedItem
	busy        bool
	busyBlock   int64
	closed      bool
}

func (f *scanFeed) signal() {
	select {
	case f.notify <- struct{}{}:
	default:
	}
}

func (f *scanFeed) push(block int64, tx *sharedTx) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed || !f.resolved || block <= f.skipThrough {
		return false
	}
	f.items = append(f.items, feedItem{block: block, tx: tx})
	return true
}

func (f *scanFeed) close() {
	f.mu.Lock()
	f.closed = true
	f.items = nil
	f.mu.Unlock()
	f.signal()
}

func (f *scanFeed) isResolved() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.resolved
}

// catchUpRange returns the blocks the monitor must read on its own, if any.
func (f *scanFeed) catchUpRange() (from, to int64, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.catchFrom, f.catchTo, f.catching && f.catchFrom <= f.catchTo
}

func (f *scanFeed) catchUpProgress(block int64) {
	f.mu.Lock()
	if block > f.catchDone {
		f.catchDone = block
	}
	f.mu.Unlock()
}

func (f *scanFeed) finishCatchUp() {
	f.mu.Lock()
	f.catching = false
	f.mu.Unlock()
}

// next takes the oldest queued match; the feed counts as busy until done is called.
func (f *scanFeed) next() (feedItem, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed || len(f.items) == 0 {
		return feedItem{}, false
	}
	it := f.items[0]
	f.items[0] = feedItem{}
	f.items = f.items[1:]
	f.busy, f.busyBlock = true, it.block
	return it, true
}

func (f *scanFeed) done() {
	f.mu.Lock()
	f.busy = false
	f.mu.Unlock()
}

// lastProcessed is the last block whose transactions for this address were all
// handled, the value a subscription stores as its current block. It never passes a
// match that is still queued or being handled.
func (f *scanFeed) lastProcessed() int64 {
	dispatched := f.scanner.dispatched.Load()
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case !f.resolved:
		return f.initial
	case f.catching:
		return f.catchDone
	case f.busy:
		return f.busyBlock - 1
	case len(f.items) > 0:
		return f.items[0].block - 1
	case dispatched > f.skipThrough:
		return dispatched
	default:
		return f.skipThrough
	}
}
