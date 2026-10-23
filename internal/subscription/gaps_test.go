package subscription

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/frstrtr/mongotron/internal/blockchain/client"
	"github.com/frstrtr/mongotron/internal/blockchain/monitor"
	"github.com/frstrtr/mongotron/internal/blockchain/parser"
	"github.com/frstrtr/mongotron/internal/storage/models"
	"github.com/frstrtr/mongotron/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"google.golang.org/protobuf/types/known/anypb"
)

// gapSource is a node whose blocks can be made to fail.
type gapSource struct {
	mu     sync.Mutex
	head   int64
	blocks map[int64]*core.Block
	fail   map[int64]int // remaining failures; negative = always
	infos  map[string]*core.TransactionInfo
	down   bool // every read fails (node unreachable)
}

func (s *gapSource) setDown(down bool) {
	s.mu.Lock()
	s.down = down
	s.mu.Unlock()
}

func newGapSource(head int64) *gapSource {
	s := &gapSource{head: head, blocks: map[int64]*core.Block{}, fail: map[int64]int{}, infos: map[string]*core.TransactionInfo{}}
	for n := head - 20; n <= head; n++ {
		s.blocks[n] = gapBlock(n)
	}
	return s
}

func gapBlock(n int64, txs ...*core.Transaction) *core.Block {
	return &core.Block{BlockHeader: &core.BlockHeader{RawData: &core.BlockHeaderRaw{Number: n, Timestamp: n * 3000}}, Transactions: txs}
}

var gapNonce int64

// addTransfers appends a block with one TRX transfer from payerAddr to each address.
func (s *gapSource) addTransfers(t *testing.T, to ...string) (int64, []string) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.head++
	var txs []*core.Transaction
	var ids []string
	for _, addr := range to {
		gapNonce++
		param, err := anypb.New(&core.TransferContract{OwnerAddress: rawAddr(t, payerAddr), ToAddress: rawAddr(t, addr), Amount: 1_000_000})
		require.NoError(t, err)
		tx := &core.Transaction{RawData: &core.TransactionRaw{Timestamp: gapNonce, Contract: []*core.Transaction_Contract{
			{Type: core.Transaction_Contract_TransferContract, Parameter: param},
		}}}
		id := parser.NewTronParser(nil).CalculateTransactionID(tx)
		raw, _ := client.TxIDBytes(id)
		s.infos[id] = &core.TransactionInfo{Id: raw, BlockNumber: s.head, Result: core.TransactionInfo_SUCESS,
			Receipt: &core.ResourceReceipt{NetUsage: 268}}
		txs = append(txs, tx)
		ids = append(ids, id)
	}
	s.blocks[s.head] = gapBlock(s.head, txs...)
	return s.head, ids
}

func (s *gapSource) setFail(n int64, times int) {
	s.mu.Lock()
	s.fail[n] = times
	s.mu.Unlock()
}

func (s *gapSource) GetNowBlock(ctx context.Context) (*core.Block, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.down {
		return nil, errors.New("node unreachable")
	}
	return s.blocks[s.head], nil
}

func (s *gapSource) GetBlockByNum(ctx context.Context, n int64) (*core.Block, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.down {
		return nil, errors.New("node unreachable")
	}
	if left := s.fail[n]; left != 0 {
		if left > 0 {
			s.fail[n] = left - 1
		}
		return nil, errors.New("transient node error")
	}
	b, ok := s.blocks[n]
	if !ok {
		return nil, fmt.Errorf("block %d not found", n)
	}
	return b, nil
}

func (s *gapSource) GetTransactionInfoById(ctx context.Context, id string) (*core.TransactionInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if info, ok := s.infos[id]; ok {
		return info, nil
	}
	return nil, client.ErrTransactionInfoNotFound
}

// fakeGapStore records what the manager writes to scanner_gaps.
type fakeGapStore struct {
	mu       sync.Mutex
	created  []*models.ScannerGap
	retried  map[primitive.ObjectID]int
	failures map[primitive.ObjectID][]bool // abandoned flag per recorded failure
	pending  []*models.ScannerGap
}

func newFakeGapStore() *fakeGapStore {
	return &fakeGapStore{retried: map[primitive.ObjectID]int{}, failures: map[primitive.ObjectID][]bool{}}
}

func (f *fakeGapStore) Create(ctx context.Context, gap *models.ScannerGap) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	gap.ID = primitive.NewObjectID()
	cp := *gap
	f.created = append(f.created, &cp)
	return nil
}

func (f *fakeGapStore) ListPending(ctx context.Context, limit int64) ([]*models.ScannerGap, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pending, nil
}

func (f *fakeGapStore) MarkRetried(ctx context.Context, id primitive.ObjectID, attempts int, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retried[id] = attempts
	return nil
}

func (f *fakeGapStore) RecordFailure(ctx context.Context, id primitive.ObjectID, attempts int, lastErr string, abandoned bool, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures[id] = append(f.failures[id], abandoned)
	return nil
}

func (f *fakeGapStore) snapshot() ([]*models.ScannerGap, map[primitive.ObjectID]int, map[primitive.ObjectID][]bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*models.ScannerGap(nil), f.created...), f.retried, f.failures
}

func gapManager(t *testing.T, src *gapSource, opts ManagerOptions, addrs ...string) (*Manager, *fakeGapStore) {
	t.Helper()
	l := logger.NewDefault()
	m := newManager(nil, src, &l, opts)
	store := newFakeGapStore()
	m.gapStore = store
	for i, addr := range addrs {
		require.NoError(t, m.startMonitor(&models.Subscription{
			SubscriptionID: fmt.Sprintf("sub_gap%d", i), Address: addr, Status: "active", WalletType: "invoice",
			Filters: FiltersForAssets([]string{"TRX"}, nil), CurrentBlock: src.head,
		}))
	}
	t.Cleanup(func() { _ = m.Stop() })
	return m, store
}

// queued drains the router queue (the router is not running in these tests) and
// returns "txHash@watchedAddress" per routed event.
func queued(m *Manager) []string {
	var out []string
	for {
		select {
		case req := <-m.eventRouter.eventQueue:
			out = append(out, req.Event.TransactionID+"@"+req.Subscription.Address)
		default:
			return out
		}
	}
}

func TestSkippedBlockIsPersistedAndRetriedUntilItsEventsAreRouted(t *testing.T) {
	src := newGapSource(5000)
	m, store := gapManager(t, src, DefaultManagerOptions(), invoiceAdr)
	block, ids := src.addTransfers(t, invoiceAdr)
	src.setFail(block, 1) // the first retry still fails

	m.recordGap(monitor.SkippedBlock{Block: block, Err: "node error", Attempts: 10, At: time.Now()})
	created, _, _ := store.snapshot()
	require.Len(t, created, 1)
	assert.Equal(t, block, created[0].BlockNumber)
	assert.Empty(t, created[0].Address, "a gap of the shared scanner concerns every watched address")
	assert.Equal(t, "node error", created[0].Error)
	st := m.MonitoringStats().Gaps
	assert.Equal(t, int64(1), st.Recorded)
	assert.Equal(t, 1, st.Pending)
	assert.Equal(t, []int64{block}, st.PendingBlocks)

	m.retryGaps(context.Background())
	_, retried, failures := store.snapshot()
	assert.Empty(t, retried)
	assert.Equal(t, []bool{false}, failures[created[0].ID], "a failed retry is recorded, not abandoned")
	assert.Equal(t, 1, m.MonitoringStats().Gaps.Pending)
	assert.Empty(t, queued(m))

	m.retryGaps(context.Background())
	_, retried, _ = store.snapshot()
	assert.Equal(t, 2, retried[created[0].ID], "marked retried after the second attempt")
	assert.Equal(t, []string{ids[0] + "@" + invoiceAdr}, queued(m), "the deposit in the skipped block is routed")
	st = m.MonitoringStats().Gaps
	assert.Equal(t, 0, st.Pending)
	assert.Equal(t, int64(1), st.Recovered)
}

func TestCatchUpGapIsRetriedForItsAddressOnly(t *testing.T) {
	src := newGapSource(5000)
	m, _ := gapManager(t, src, DefaultManagerOptions(), invoiceAdr, payerAddr)
	block, ids := src.addTransfers(t, invoiceAdr)

	// payerAddr sends, so both addresses are in the transaction; the gap is invoiceAdr's
	m.recordGap(monitor.SkippedBlock{Block: block, Address: invoiceAdr, Err: "x", At: time.Now()})
	m.retryGaps(context.Background())
	assert.Equal(t, []string{ids[0] + "@" + invoiceAdr}, queued(m))

	// A gap of the shared scanner goes to every watched address in the block
	m.recordGap(monitor.SkippedBlock{Block: block, Err: "x", At: time.Now()})
	m.retryGaps(context.Background())
	assert.ElementsMatch(t, []string{ids[0] + "@" + invoiceAdr, ids[0] + "@" + payerAddr}, queued(m))
}

func TestGapIsAbandonedAfterItsAttempts(t *testing.T) {
	src := newGapSource(5000)
	m, store := gapManager(t, src, DefaultManagerOptions(), invoiceAdr)
	m.gapMaxAttempts = 2
	block, _ := src.addTransfers(t, invoiceAdr)
	src.setFail(block, -1)

	m.recordGap(monitor.SkippedBlock{Block: block, Err: "x", At: time.Now()})
	m.retryGaps(context.Background())
	m.retryGaps(context.Background())
	created, retried, failures := store.snapshot()
	assert.Empty(t, retried)
	assert.Equal(t, []bool{false, true}, failures[created[0].ID])
	st := m.MonitoringStats().Gaps
	assert.Equal(t, 0, st.Pending)
	assert.Equal(t, int64(1), st.Abandoned)
}

func TestPendingGapsOfAPreviousRunAreLoaded(t *testing.T) {
	src := newGapSource(5000)
	m, store := gapManager(t, src, DefaultManagerOptions())
	store.pending = []*models.ScannerGap{{ID: primitive.NewObjectID(), BlockNumber: 4990}, {ID: primitive.NewObjectID(), BlockNumber: 4980}}
	m.loadPendingGaps()
	st := m.MonitoringStats().Gaps
	assert.Equal(t, 2, st.Pending)
	assert.Equal(t, []int64{4980, 4990}, st.PendingBlocks, "oldest first")
}

func TestScannerSkipReachesTheGapStoreAndTheRetryRoutesTheDeposit(t *testing.T) {
	src := newGapSource(5000)
	opts := DefaultManagerOptions()
	opts.PollInterval = 2 * time.Millisecond
	m, store := gapManager(t, src, opts, invoiceAdr)

	block, ids := src.addTransfers(t, invoiceAdr)
	src.setFail(block, -1)
	src.addTransfers(t) // a newer head, so the failing block is fetched by number
	require.NoError(t, m.scanner.Start())

	waitUntil(t, func() bool { created, _, _ := store.snapshot(); return len(created) == 1 })
	created, _, _ := store.snapshot()
	assert.Equal(t, block, created[0].BlockNumber)
	st := m.MonitoringStats()
	require.NotNil(t, st.Scanner)
	assert.Equal(t, int64(1), st.Scanner.SkippedBlocks)
	assert.Equal(t, 1, st.Gaps.Pending)

	src.setFail(block, 0)
	m.retryGaps(context.Background())
	assert.Contains(t, queued(m), ids[0]+"@"+invoiceAdr, "the skipped deposit is routed by the retry")
	assert.Equal(t, int64(1), m.MonitoringStats().Gaps.Recovered)
}

func TestGapRetriesCountNoAttemptWhileTheNodeIsDown(t *testing.T) {
	src := newGapSource(5000)
	m, store := gapManager(t, src, DefaultManagerOptions(), invoiceAdr)
	m.gapMaxAttempts = 3
	block, ids := src.addTransfers(t, invoiceAdr)
	m.recordGap(monitor.SkippedBlock{Block: block, Err: "x", At: time.Now()})

	src.setDown(true)
	for round := 0; round < 5; round++ { // more rounds than gapMaxAttempts
		m.retryGaps(context.Background())
	}
	st := m.MonitoringStats().Gaps
	assert.Equal(t, 1, st.Pending, "an outage must not abandon the gap")
	assert.Equal(t, int64(0), st.Abandoned)
	created, _, failures := store.snapshot()
	assert.Empty(t, failures[created[0].ID], "no attempt recorded during the outage")

	src.setDown(false)
	m.retryGaps(context.Background())
	st = m.MonitoringStats().Gaps
	assert.Equal(t, int64(1), st.Recovered)
	assert.Equal(t, 0, st.Pending)
	assert.Equal(t, []string{ids[0] + "@" + invoiceAdr}, queued(m), "the deposit is routed once the node is back")
	_, retried, _ := store.snapshot()
	assert.Equal(t, 1, retried[created[0].ID], "one counted attempt")
}

func TestUnreadableOldGapsDoNotStarveNewerOnes(t *testing.T) {
	src := newGapSource(5000)
	m, _ := gapManager(t, src, DefaultManagerOptions(), invoiceAdr)
	for b := int64(4981); b <= 4995; b++ { // 15 old gaps that never read
		src.setFail(b, -1)
		m.recordGap(monitor.SkippedBlock{Block: b, Err: "x", At: time.Now()})
	}
	newer, ids := src.addTransfers(t, invoiceAdr)
	m.recordGap(monitor.SkippedBlock{Block: newer, Err: "x", At: time.Now()})

	m.retryGaps(context.Background()) // the 10 lowest blocks, all fail
	assert.Equal(t, int64(0), m.MonitoringStats().Gaps.Recovered)
	m.retryGaps(context.Background()) // the 6 not tried yet come first
	st := m.MonitoringStats().Gaps
	assert.Equal(t, int64(1), st.Recovered, "the newer gap is reached in the second round")
	assert.Equal(t, 15, st.Pending)
	assert.Equal(t, []string{ids[0] + "@" + invoiceAdr}, queued(m))
}

// chanMonitor is a monitor whose channel the test fills.
type chanMonitor struct {
	ch   chan *monitor.AddressEvent
	last int64
	once sync.Once
}

func (c *chanMonitor) Start() error                         { return nil }
func (c *chanMonitor) Stop()                                { c.once.Do(func() { close(c.ch) }) }
func (c *chanMonitor) Events() <-chan *monitor.AddressEvent { return c.ch }
func (c *chanMonitor) GetLastBlockNumber() int64            { return c.last }

func chanWrapper(mon *chanMonitor, current int64) *MonitorWrapper {
	return &MonitorWrapper{
		Monitor: mon,
		Subscription: &models.Subscription{SubscriptionID: "sub_chan", Address: invoiceAdr, Status: "active",
			WalletType: "invoice", Filters: FiltersForAssets([]string{"TRX"}, nil), CurrentBlock: current},
		EventChan: mon.ch,
		StopChan:  make(chan struct{}),
	}
}

func TestProgressIsNotStoredWhileEventsWaitInTheChannel(t *testing.T) {
	l := logger.NewDefault()
	m := newManager(nil, nil, &l, DefaultManagerOptions())
	mon := &chanMonitor{ch: make(chan *monitor.AddressEvent, 10), last: 68900010}
	w := chanWrapper(mon, 68900000)

	mon.ch <- trxTransferEvent(1) // block 68900001, already counted by the monitor
	assert.False(t, m.saveProgress(w), "an event is still in the channel")
	assert.Equal(t, int64(68900000), w.current().CurrentBlock)

	m.handleMonitorEvent(w, <-mon.ch)
	assert.True(t, m.saveProgress(w))
	assert.Equal(t, int64(68900010), w.current().CurrentBlock)
}

func TestStoppingRoutesTheEventsStillInTheChannel(t *testing.T) {
	l := logger.NewDefault()
	m := newManager(nil, nil, &l, DefaultManagerOptions())
	mon := &chanMonitor{ch: make(chan *monitor.AddressEvent, 10), last: 68900001}
	w := chanWrapper(mon, 68900000)
	for i := 1; i <= 3; i++ {
		mon.ch <- trxTransferEvent(int64(i))
	}
	close(w.StopChan) // the stop is seen before any event

	done := make(chan struct{})
	go func() { m.processEvents(w); close(done) }()
	waitUntil(t, func() bool { return len(m.eventRouter.eventQueue) == 3 })
	mon.Stop() // closes the channel: the drain ends
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("processEvents did not end after the channel closed")
	}
	assert.Len(t, queued(m), 3, "all buffered events are routed")
}

func TestRoutingWaitsForRoomInsteadOfDropping(t *testing.T) {
	l := logger.NewDefault()
	m := newManager(nil, nil, &l, DefaultManagerOptions())
	m.eventRouter.eventQueue = make(chan *RouteEventRequest, 1)
	m.eventRouter.eventQueue <- &RouteEventRequest{}
	sub := chanWrapper(&chanMonitor{ch: make(chan *monitor.AddressEvent)}, 0).Subscription

	type result struct {
		routed bool
		err    error
	}
	res := make(chan result, 1)
	go func() {
		routed, err := m.routeMonitorEvent(sub, trxTransferEvent(1))
		res <- result{routed, err}
	}()
	select {
	case r := <-res:
		t.Fatalf("returned %+v with the queue full: the event must wait, not be dropped", r)
	case <-time.After(50 * time.Millisecond):
	}
	<-m.eventRouter.eventQueue // room
	r := <-res
	assert.True(t, r.routed)
	assert.NoError(t, r.err)
	assert.Len(t, m.eventRouter.eventQueue, 1, "the event is queued")
}

func TestAnEventThatCannotBeQueuedIsNotCountedAsHandled(t *testing.T) {
	l := logger.NewDefault()
	m := newManager(nil, nil, &l, DefaultManagerOptions())
	m.eventRouter.eventQueue = make(chan *RouteEventRequest, 1)
	m.eventRouter.eventQueue <- &RouteEventRequest{}
	w := chanWrapper(&chanMonitor{ch: make(chan *monitor.AddressEvent)}, 68900000)

	done := make(chan struct{})
	go func() { m.handleMonitorEvent(w, trxTransferEvent(1)); close(done) }()
	time.Sleep(20 * time.Millisecond)
	m.cancel() // the manager stops while the queue is full
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("routing did not give up on stop")
	}
	assert.Equal(t, int64(68900000), w.current().CurrentBlock, "the current block must not pass the unrouted event")

	routed, err := m.routeMonitorEvent(w.Subscription, trxTransferEvent(1))
	assert.False(t, routed)
	assert.Error(t, err)
}
