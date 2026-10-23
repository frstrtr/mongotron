package monitor

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
)

const testHead int64 = 1000

// newScannerMonitors starts n monitors (addresses testAddr(0..n-1)) on scanner sc, each
// with StartBlock start.
func newScannerMonitors(t *testing.T, sc *BlockScanner, n int, start int64) []*AddressMonitor {
	t.Helper()
	mons := make([]*AddressMonitor, n)
	for i := 0; i < n; i++ {
		_, addr := testAddr(i)
		m, err := NewAddressMonitorFromSource(nil, Config{WatchAddress: addr, StartBlock: start, Scanner: sc}, testLog())
		if err != nil {
			t.Fatal(err)
		}
		if err := m.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(m.Stop)
		mons[i] = m
	}
	return mons
}

// newScannerMonitor starts one monitor for testAddr(i) on scanner sc.
func newScannerMonitor(t *testing.T, sc *BlockScanner, i int, start int64) *AddressMonitor {
	t.Helper()
	_, addr := testAddr(i)
	m, err := NewAddressMonitorFromSource(nil, Config{WatchAddress: addr, StartBlock: start, Scanner: sc}, testLog())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	return m
}

// newPollingMonitors starts n monitors that poll on their own (no scanner), the way
// every watched address was monitored before the shared scanner.
func newPollingMonitors(t *testing.T, src BlockSource, n int, start int64) []*AddressMonitor {
	t.Helper()
	mons := make([]*AddressMonitor, n)
	for i := 0; i < n; i++ {
		_, addr := testAddr(i)
		// A poll interval of an hour: the test drives processNewBlocks itself.
		m, err := NewAddressMonitorFromSource(src, Config{WatchAddress: addr, StartBlock: start, PollInterval: time.Hour}, testLog())
		if err != nil {
			t.Fatal(err)
		}
		if err := m.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(m.Stop)
		mons[i] = m
	}
	return mons
}

// addTrafficBlock adds a block with one TRX transfer from a stranger to address i.
func addTrafficBlock(c *fakeChain, i int) int64 {
	stranger, _ := testAddr(60000)
	to, _ := testAddr(i)
	return c.addBlock(trxTransferTx(stranger, to, int64(1000+i)))
}

func TestSharedScannerFetchesEachBlockOnceForAllWatchedAddresses(t *testing.T) {
	const n, blocks = 10, 20
	chain := newFakeChain(testHead)
	sc := NewBlockScanner(chain, ScannerConfig{PollInterval: time.Hour}, testLog())
	mons := newScannerMonitors(t, sc, n, testHead)

	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	for b := 1; b <= blocks; b++ {
		addTrafficBlock(chain, b%n)
		if err := sc.scanOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}

	// Every monitor gets exactly the transfers to its own address, in block order.
	for i, m := range mons {
		events := collect(t, m.Events(), blocks/n)
		for _, ev := range events {
			if ev.Amount != int64(1000+i) || ev.ContractType != "TransferContract" || !ev.Success {
				t.Fatalf("monitor %d got a wrong event: %+v", i, ev)
			}
		}
		expectNone(t, m.Events(), 20*time.Millisecond)
		waitUntil(t, "progress to the head", func() bool { return m.GetLastBlockNumber() == testHead+blocks })
	}

	got := chain.counts()
	st := sc.Stats()
	if got.byNum != blocks {
		t.Fatalf("GetBlockByNum calls = %d, want %d (one per block for %d watched addresses)", got.byNum, blocks, n)
	}
	if got.now != 0 {
		t.Fatalf("GetNowBlock calls = %d, want 0 (head read header-only)", got.now)
	}
	if got.info != blocks {
		t.Fatalf("transaction info reads = %d, want %d (one per matched transaction)", got.info, blocks)
	}
	if st.BlocksScanned != blocks || st.BlockFetches != blocks || st.Subscribers != n {
		t.Fatalf("unexpected stats: %+v", st)
	}
}

// TestBlockFetchesBeforeAndAfterSharedScanner measures the node reads of N watched
// addresses over K blocks with one polling loop per address (before) and with the
// shared scanner (after).
func TestBlockFetchesBeforeAndAfterSharedScanner(t *testing.T) {
	const blocks = 20
	for _, n := range []int{1, 10, 100} {
		t.Run(fmt.Sprintf("%d_addresses", n), func(t *testing.T) {
			// Before: every address polls on its own.
			before := newFakeChain(testHead)
			polling := newPollingMonitors(t, fullHeadOnly{before}, n, testHead)
			for b := 1; b <= blocks; b++ {
				addTrafficBlock(before, b%n)
				for _, m := range polling {
					if err := m.processNewBlocks(); err != nil {
						t.Fatal(err)
					}
				}
			}
			for _, m := range polling {
				if m.GetLastBlockNumber() != testHead+blocks {
					t.Fatalf("polling monitor stopped at %d", m.GetLastBlockNumber())
				}
			}

			// After: one shared scanner (header-only head reads).
			after := newFakeChain(testHead)
			sc := NewBlockScanner(after, ScannerConfig{PollInterval: time.Hour}, testLog())
			shared := newScannerMonitors(t, sc, n, testHead)
			if err := sc.scanOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
			for b := 1; b <= blocks; b++ {
				addTrafficBlock(after, b%n)
				if err := sc.scanOnce(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			for _, m := range shared {
				waitUntil(t, "progress", func() bool { return m.GetLastBlockNumber() == testHead+blocks })
			}

			// After, on a node without the header-only read: the head block itself is
			// used, never fetched twice.
			fullOnly := newFakeChain(testHead)
			sc2 := NewBlockScanner(fullHeadOnly{fullOnly}, ScannerConfig{PollInterval: time.Hour}, testLog())
			newScannerMonitors(t, sc2, n, testHead)
			if err := sc2.scanOnce(context.Background()); err != nil {
				t.Fatal(err)
			}
			for b := 1; b <= blocks; b++ {
				addTrafficBlock(fullOnly, b%n)
				if err := sc2.scanOnce(context.Background()); err != nil {
					t.Fatal(err)
				}
			}

			b, a, f := before.counts(), after.counts(), fullOnly.counts()
			t.Logf("%d watched addresses, %d blocks: before (per-address polling) %d GetBlockByNum + %d GetNowBlock = %d full block downloads; "+
				"after (shared scanner) %d GetBlockByNum + %d GetNowBlock (+%d header-only head reads) = %d; "+
				"after without header-only reads %d GetBlockByNum + %d GetNowBlock = %d",
				n, blocks, b.byNum, b.now, b.byNum+b.now, a.byNum, a.now, a.header, a.byNum+a.now, f.byNum, f.now, f.byNum+f.now)

			if b.byNum != n*blocks || b.now != n*blocks {
				t.Fatalf("before: byNum=%d now=%d, want %d each", b.byNum, b.now, n*blocks)
			}
			if a.byNum != blocks || a.now != 0 {
				t.Fatalf("after: byNum=%d now=%d, want %d and 0", a.byNum, a.now, blocks)
			}
			if f.byNum+f.now != blocks+1 {
				t.Fatalf("after without header reads: %d downloads for %d blocks (+1 initial head read)", f.byNum+f.now, blocks)
			}
		})
	}
}

func TestSharedTransactionInfoIsReadOnceForSeveralWatchedAddresses(t *testing.T) {
	chain := newFakeChain(testHead)
	sc := NewBlockScanner(chain, ScannerConfig{PollInterval: time.Hour}, testLog())
	mons := newScannerMonitors(t, sc, 2, testHead)
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	from, _ := testAddr(0)
	to, _ := testAddr(1)
	tx := trxTransferTx(from, to, 5_000_000)
	block := chain.addBlock(tx)
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	for _, m := range mons {
		ev := collect(t, m.Events(), 1)[0]
		if ev.TransactionID != txID(tx) || ev.BlockNumber != block || ev.Amount != 5_000_000 || !ev.Success {
			t.Fatalf("unexpected event: %+v", ev)
		}
	}
	if got := chain.counts().info; got != 1 {
		t.Fatalf("transaction info read %d times, want once", got)
	}
}

func TestLateSubscriberCatchesUpOnItsOwnThenJoinsInOrder(t *testing.T) {
	chain := newFakeChain(testHead)
	sc := NewBlockScanner(chain, ScannerConfig{PollInterval: time.Hour}, testLog())
	newScannerMonitors(t, sc, 1, testHead) // address 0 keeps the scanner busy
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Blocks 1001..1010, address 5 receives in 1003 and 1008
	var want []int64
	for b := 1; b <= 10; b++ {
		if b == 3 || b == 8 {
			want = append(want, addTrafficBlock(chain, 5))
		} else {
			addTrafficBlock(chain, 0)
		}
	}
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	fetchedByScanner := chain.counts().byNum

	// Address 5 subscribes late, last processed block 1001: it must read 1002..1010 itself
	late := newScannerMonitor(t, sc, 5, testHead+1)
	got := collect(t, late.Events(), 2)
	if got[0].BlockNumber != want[0] || got[1].BlockNumber != want[1] {
		t.Fatalf("catch-up events at %d, %d; want %v", got[0].BlockNumber, got[1].BlockNumber, want)
	}
	waitUntil(t, "catch-up to finish", func() bool { return late.GetLastBlockNumber() == testHead+10 })
	if st := sc.Stats(); st.CatchUpFetches != 9 {
		t.Fatalf("catch-up fetches = %d, want 9 (blocks 1002..1010)", st.CatchUpFetches)
	}

	// Then it is served by the scanner like the others
	live := addTrafficBlock(chain, 5)
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ev := collect(t, late.Events(), 1)[0]; ev.BlockNumber != live {
		t.Fatalf("live event at %d, want %d", ev.BlockNumber, live)
	}
	if extra := chain.counts().byNum - fetchedByScanner - 9; extra != 1 {
		t.Fatalf("block reads after the catch-up = %d, want 1 (the scanner's)", extra)
	}
}

func TestStartupCatchesUpCloseAddressesTogether(t *testing.T) {
	chain := newFakeChain(testHead)
	// Blocks 1001..1010 with traffic for addresses 0, 1, 2 in every block
	for b := 1; b <= 10; b++ {
		stranger, _ := testAddr(60000)
		var txs []*core.Transaction
		for i := 0; i < 3; i++ {
			to, _ := testAddr(i)
			txs = append(txs, trxTransferTx(stranger, to, int64(b)))
		}
		chain.addBlock(txs...)
	}

	sc := NewBlockScanner(chain, ScannerConfig{PollInterval: time.Hour, CatchUpWindow: 5}, testLog())
	// Stored progress after a restart: 1002 and 1004 are close; 990 is beyond the window.
	starts := []int64{1004, 1002, 990}
	var mons []*AddressMonitor
	for i, start := range starts {
		_, addr := testAddr(i)
		m, err := NewAddressMonitorFromSource(nil, Config{WatchAddress: addr, StartBlock: start, Scanner: sc}, testLog())
		if err != nil {
			t.Fatal(err)
		}
		if err := m.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(m.Stop)
		mons = append(mons, m)
	}
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Each address gets every block after its own start, exactly once
	for i, m := range mons {
		wantN := int(testHead + 10 - starts[i])
		if starts[i] < testHead {
			wantN = 10 // blocks before 1001 are empty
		}
		evs := collect(t, m.Events(), wantN)
		for j := 1; j < len(evs); j++ {
			if evs[j].BlockNumber <= evs[j-1].BlockNumber {
				t.Fatalf("address %d: events out of order: %d after %d", i, evs[j].BlockNumber, evs[j-1].BlockNumber)
			}
		}
		expectNone(t, m.Events(), 20*time.Millisecond)
		waitUntil(t, "progress", func() bool { return m.GetLastBlockNumber() == testHead+10 })
	}

	st := sc.Stats()
	// The scanner starts after 1002 (the oldest close start) and reads 1003..1010 once;
	// the far one reads 991..1002 on its own.
	if st.BlockFetches != 8 || st.CatchUpFetches != 12 {
		t.Fatalf("scanner fetches = %d (want 8), catch-up fetches = %d (want 12)", st.BlockFetches, st.CatchUpFetches)
	}
	for n := testHead + 3; n <= testHead+10; n++ {
		chain.mu.Lock()
		reads := chain.byNumBlocks[n]
		chain.mu.Unlock()
		if reads != 1 {
			t.Fatalf("block %d read %d times, want once", n, reads)
		}
	}
}

func TestNoStartBlockStartsAfterTheCurrentHead(t *testing.T) {
	chain := newFakeChain(testHead)
	addTrafficBlock(chain, 0) // 1001: before the monitor existed
	sc := NewBlockScanner(chain, ScannerConfig{PollInterval: time.Hour}, testLog())
	m := newScannerMonitors(t, sc, 1, -1)[0]
	if got := m.GetLastBlockNumber(); got != -1 {
		t.Fatalf("before the first head read GetLastBlockNumber = %d, want the configured -1", got)
	}
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	expectNone(t, m.Events(), 30*time.Millisecond)
	if got := m.GetLastBlockNumber(); got != testHead+1 {
		t.Fatalf("GetLastBlockNumber = %d, want the head %d", got, testHead+1)
	}

	next := addTrafficBlock(chain, 0)
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ev := collect(t, m.Events(), 1)[0]; ev.BlockNumber != next {
		t.Fatalf("event at %d, want %d", ev.BlockNumber, next)
	}
}

func TestFailedBlockIsRetriedNotSkipped(t *testing.T) {
	chain := newFakeChain(testHead)
	sc := NewBlockScanner(chain, ScannerConfig{PollInterval: time.Hour, MaxBlockRetries: 3}, testLog())
	m := newScannerMonitors(t, sc, 1, testHead)[0]
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	b1 := addTrafficBlock(chain, 0)
	chain.mu.Lock()
	chain.failByNum[b1] = 2
	chain.mu.Unlock()
	for i := 0; i < 2; i++ {
		if err := sc.scanOnce(context.Background()); err == nil {
			t.Fatal("expected a fetch error")
		}
		if got := m.GetLastBlockNumber(); got != testHead {
			t.Fatalf("progress moved to %d past a block that failed", got)
		}
	}
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ev := collect(t, m.Events(), 1)[0]; ev.BlockNumber != b1 {
		t.Fatalf("event at %d, want %d", ev.BlockNumber, b1)
	}

	// A block that keeps failing is skipped after MaxBlockRetries attempts
	b2 := addTrafficBlock(chain, 0)
	chain.mu.Lock()
	chain.failByNum[b2] = 100
	chain.mu.Unlock()
	for i := 0; i < 3; i++ {
		_ = sc.scanOnce(context.Background())
	}
	b3 := addTrafficBlock(chain, 0)
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ev := collect(t, m.Events(), 1)[0]; ev.BlockNumber != b3 {
		t.Fatalf("event at %d, want %d (after the skipped %d)", ev.BlockNumber, b3, b2)
	}
	if st := sc.Stats(); st.SkippedBlocks != 1 {
		t.Fatalf("skipped blocks = %d, want 1", st.SkippedBlocks)
	}
}

func TestHeadBlockIsReusedWhenTheNodeHasNoHeaderOnlyRead(t *testing.T) {
	chain := newFakeChain(testHead)
	chain.headerUnsupported = true
	sc := NewBlockScanner(chain, ScannerConfig{PollInterval: time.Hour}, testLog())
	m := newScannerMonitors(t, sc, 1, testHead)[0]
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	b := addTrafficBlock(chain, 0)
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ev := collect(t, m.Events(), 1)[0]; ev.BlockNumber != b {
		t.Fatalf("event at %d, want %d", ev.BlockNumber, b)
	}
	got := chain.counts()
	if got.header != 1 {
		t.Fatalf("header-only reads tried %d times, want once (then never again)", got.header)
	}
	if got.byNum != 0 || got.now != 2 {
		t.Fatalf("byNum=%d now=%d, want 0 and 2 (the new head block is the full head read)", got.byNum, got.now)
	}
}

func TestScannerWithoutWatchedAddressesDownloadsNoBlocks(t *testing.T) {
	chain := newFakeChain(testHead)
	sc := NewBlockScanner(chain, ScannerConfig{PollInterval: time.Hour}, testLog())
	for i := 0; i < 5; i++ {
		addTrafficBlock(chain, 0)
		if err := sc.scanOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := chain.counts(); got.byNum != 0 || got.now != 0 {
		t.Fatalf("byNum=%d now=%d with nobody watching, want 0", got.byNum, got.now)
	}
	if st := sc.Stats(); st.Cursor != testHead+5 {
		t.Fatalf("cursor = %d, want the head", st.Cursor)
	}

	// A monitor added now starts at the head without a catch-up
	m := newScannerMonitors(t, sc, 1, -1)[0]
	b := addTrafficBlock(chain, 0)
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ev := collect(t, m.Events(), 1)[0]; ev.BlockNumber != b {
		t.Fatalf("event at %d, want %d", ev.BlockNumber, b)
	}
	if st := sc.Stats(); st.CatchUpFetches != 0 || st.BlockFetches != 1 {
		t.Fatalf("unexpected fetches: %+v", st)
	}
}

func TestProgressNeverPassesAMatchStillBeingHandled(t *testing.T) {
	chain := newFakeChain(testHead)
	gate := make(chan struct{})
	chain.infoGate = gate
	sc := NewBlockScanner(chain, ScannerConfig{PollInterval: time.Hour}, testLog())
	m := newScannerMonitors(t, sc, 1, testHead)[0]
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	b := addTrafficBlock(chain, 0)
	for i := 0; i < 3; i++ {
		addTrafficBlock(chain, 1) // not watched by m
	}
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st := sc.Stats(); st.Cursor != b+3 {
		t.Fatalf("scanner cursor = %d, want %d", st.Cursor, b+3)
	}
	// The match in block b waits on its transaction info: progress stays before it
	waitUntil(t, "the info read to start", func() bool { return chain.counts().info == 1 })
	if got := m.GetLastBlockNumber(); got != b-1 {
		t.Fatalf("GetLastBlockNumber = %d while block %d is still handled, want %d", got, b, b-1)
	}
	close(gate)
	collect(t, m.Events(), 1)
	waitUntil(t, "progress to the cursor", func() bool { return m.GetLastBlockNumber() == b+3 })
}

func TestStopUnsubscribesAndClosesTheChannel(t *testing.T) {
	chain := newFakeChain(testHead)
	sc := NewBlockScanner(chain, ScannerConfig{PollInterval: time.Hour}, testLog())
	mons := newScannerMonitors(t, sc, 2, testHead)
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	mons[0].Stop()
	if st := sc.Stats(); st.Subscribers != 1 {
		t.Fatalf("subscribers = %d after a stop, want 1", st.Subscribers)
	}
	if _, ok := <-mons[0].Events(); ok {
		t.Fatal("the stopped monitor's channel must be closed")
	}
	addTrafficBlock(chain, 0)
	addTrafficBlock(chain, 1)
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	collect(t, mons[1].Events(), 1)
}

func TestScannerLoopDeliversAndStops(t *testing.T) {
	chain := newFakeChain(testHead)
	sc := NewBlockScanner(chain, ScannerConfig{PollInterval: 5 * time.Millisecond}, testLog())
	m := newScannerMonitors(t, sc, 1, testHead)[0]
	if err := sc.Start(); err != nil {
		t.Fatal(err)
	}
	if err := sc.Start(); err == nil {
		t.Fatal("a second Start must fail")
	}
	b := addTrafficBlock(chain, 0)
	if ev := collect(t, m.Events(), 1)[0]; ev.BlockNumber != b {
		t.Fatalf("event at %d, want %d", ev.BlockNumber, b)
	}
	done := make(chan struct{})
	go func() { sc.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return")
	}
}

func TestReceiptFiguresReachEventData(t *testing.T) {
	chain := newFakeChain(testHead)
	sc := NewBlockScanner(chain, ScannerConfig{PollInterval: time.Hour}, testLog())
	m := newScannerMonitors(t, sc, 1, testHead)[0]
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	owner, _ := testAddr(0)
	contract, _ := testAddr(777)
	tx := triggerTx(owner, contract, []byte{0x09, 0x5e, 0xa7, 0xb3, 0, 0})
	b := chain.addBlock(tx)
	id := txID(tx)
	chain.mu.Lock()
	info := successInfo(id, b)
	info.Result = core.TransactionInfo_FAILED
	info.Fee = 2_000_000
	info.Receipt = &core.ResourceReceipt{EnergyUsage: 64_000, EnergyUsageTotal: 65_000, OriginEnergyUsage: 1_000, EnergyFee: 0,
		NetUsage: 345, Result: core.Transaction_Result_OUT_OF_ENERGY}
	chain.infos[id] = info
	chain.mu.Unlock()
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	ev := collect(t, m.Events(), 1)[0]
	if ev.Success {
		t.Fatal("an out-of-energy call is not a success")
	}
	if ev.EventData["energyUsage"] != int64(64_000) || ev.EventData["energyUsageTotal"] != int64(65_000) ||
		ev.EventData["netUsage"] != int64(345) || ev.EventData["receiptResult"] != "OUT_OF_ENERGY" || ev.EventData["fee"] != int64(2_000_000) {
		t.Fatalf("receipt not in EventData: %+v", ev.EventData)
	}
	if ev.RawTxInfo == nil || ev.RawTransaction == nil {
		t.Fatal("raw transaction and info must be attached")
	}
}

// BenchmarkBlockFanOut compares the per-block work of N watched addresses: one polling
// loop per address parses every block N times, the shared scanner once.
func BenchmarkBlockFanOut(b *testing.B) {
	for _, n := range []int{10, 100} {
		for _, mode := range []string{"per-address", "shared"} {
			b.Run(fmt.Sprintf("%s/%d_addresses", mode, n), func(b *testing.B) {
				chain := newFakeChain(testHead)
				// A busy block: 300 transfers between strangers (no match)
				var txs []*core.Transaction
				for i := 0; i < 300; i++ {
					from, _ := testAddr(20000 + i)
					to, _ := testAddr(30000 + i)
					txs = append(txs, trxTransferTx(from, to, 1))
				}
				block := makeBlock(testHead+1, txs...)

				var sc *BlockScanner
				var mons []*AddressMonitor
				for i := 0; i < n; i++ {
					_, addr := testAddr(i)
					cfg := Config{WatchAddress: addr, StartBlock: testHead, PollInterval: time.Hour}
					if mode == "shared" {
						if sc == nil {
							sc = NewBlockScanner(chain, ScannerConfig{PollInterval: time.Hour}, testLog())
						}
						cfg.Scanner = sc
					}
					m, _ := NewAddressMonitorFromSource(chain, cfg, testLog())
					mons = append(mons, m)
				}
				if sc != nil {
					for _, m := range mons {
						_ = m.Start()
					}
					_ = sc.scanOnce(context.Background())
				}
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if sc != nil {
						sc.mu.Lock()
						sc.cursor = testHead
						sc.mu.Unlock()
						sc.dispatchBlock(testHead+1, block)
					} else {
						for _, m := range mons {
							for _, tx := range block.GetTransactions() {
								m.isAddressInTransaction(tx)
							}
						}
					}
				}
				b.StopTimer()
				reads := float64(n)
				if sc != nil {
					reads = 1
				}
				b.ReportMetric(reads, "block-reads/block")
				for _, m := range mons {
					m.Stop()
				}
			})
		}
	}
}

// skipRecorder collects the blocks a scanner reports as skipped.
type skipRecorder struct {
	mu    sync.Mutex
	skips []SkippedBlock
}

func (r *skipRecorder) record(s SkippedBlock) {
	r.mu.Lock()
	r.skips = append(r.skips, s)
	r.mu.Unlock()
}

func (r *skipRecorder) all() []SkippedBlock {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]SkippedBlock(nil), r.skips...)
}

// behindScanner sets up a scanner past block testHead+10 (blocks 1001..1010, address 5
// receiving in 1004 and 1008) and returns it with the chain; the caller subscribes a
// monitor for address 5 that starts behind.
func behindScanner(t *testing.T, cfg ScannerConfig) (*fakeChain, *BlockScanner, []int64) {
	t.Helper()
	chain := newFakeChain(testHead)
	sc := NewBlockScanner(chain, cfg, testLog())
	newScannerMonitor(t, sc, 0, testHead)
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	var want []int64
	for b := 1; b <= 10; b++ {
		if b == 4 || b == 8 {
			want = append(want, addTrafficBlock(chain, 5))
		} else {
			addTrafficBlock(chain, 0)
		}
	}
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	return chain, sc, want
}

func TestCatchUpRetriesATransientFailureWithoutSkipping(t *testing.T) {
	rec := &skipRecorder{}
	chain, sc, want := behindScanner(t, ScannerConfig{PollInterval: time.Millisecond, MaxBlockRetries: 5, OnSkip: rec.record})
	chain.setFailures(want[0], 4) // fails 4 times, below MaxBlockRetries

	late := newScannerMonitor(t, sc, 5, testHead+1)
	got := collect(t, late.Events(), 2)
	if got[0].BlockNumber != want[0] || got[1].BlockNumber != want[1] {
		t.Fatalf("events at %d, %d; want %v (the deposit after the transient failure must not be lost)", got[0].BlockNumber, got[1].BlockNumber, want)
	}
	waitUntil(t, "catch-up done", func() bool { return late.GetLastBlockNumber() == testHead+10 })
	if skips := rec.all(); len(skips) != 0 {
		t.Fatalf("a transient failure must not skip: %+v", skips)
	}
}

func TestCatchUpNeverSkipsWhileTheNodeIsDown(t *testing.T) {
	rec := &skipRecorder{}
	chain, sc, want := behindScanner(t, ScannerConfig{PollInterval: time.Millisecond, MaxBlockRetries: 3, OnSkip: rec.record})
	chain.setFailures(want[0], 1_000_000)
	chain.setHeadDown(true)

	late := newScannerMonitor(t, sc, 5, testHead+1)
	waitUntil(t, "many failed attempts", func() bool { return sc.Stats().CatchUpFetches > 30 })
	if skips := rec.all(); len(skips) != 0 {
		t.Fatalf("failures while the node is down must not skip: %+v", skips)
	}
	if got := late.GetLastBlockNumber(); got != want[0]-1 {
		t.Fatalf("progress = %d during the outage, want %d (before the failing block)", got, want[0]-1)
	}

	chain.setFailures(want[0], 0)
	chain.setHeadDown(false)
	got := collect(t, late.Events(), 2)
	if got[0].BlockNumber != want[0] || got[1].BlockNumber != want[1] {
		t.Fatalf("events at %d, %d; want %v", got[0].BlockNumber, got[1].BlockNumber, want)
	}
}

func TestCatchUpReportsAPersistentFailureAsAGapOfTheAddress(t *testing.T) {
	rec := &skipRecorder{}
	chain, sc, want := behindScanner(t, ScannerConfig{PollInterval: time.Millisecond, MaxBlockRetries: 3, OnSkip: rec.record})
	chain.setFailures(want[0], 1_000_000)

	late := newScannerMonitor(t, sc, 5, testHead+1)
	if ev := collect(t, late.Events(), 1)[0]; ev.BlockNumber != want[1] {
		t.Fatalf("event at %d, want %d", ev.BlockNumber, want[1])
	}
	skips := rec.all()
	_, addr := testAddr(5)
	if len(skips) != 1 || skips[0].Block != want[0] || skips[0].Address != addr || skips[0].Attempts != 3 || skips[0].Err == "" {
		t.Fatalf("skips = %+v, want block %d of %s after 3 attempts", skips, want[0], addr)
	}
	if st := sc.Stats(); st.SkippedBlocks != 1 {
		t.Fatalf("skipped blocks = %d, want 1", st.SkippedBlocks)
	}
}

func TestInterruptedCatchUpKeepsItsProgress(t *testing.T) {
	chain, sc, want := behindScanner(t, ScannerConfig{PollInterval: time.Millisecond, MaxBlockRetries: 1_000_000})
	chain.setFailures(want[0], 1_000_000)

	late := newScannerMonitor(t, sc, 5, testHead+1)
	waitUntil(t, "the catch-up to reach the failing block", func() bool { return sc.Stats().CatchUpFetches > 5 })
	late.Stop()
	if got := late.GetLastBlockNumber(); got != want[0]-1 {
		t.Fatalf("progress after an interrupted catch-up = %d, want %d (never past the unread block)", got, want[0]-1)
	}
}

func TestEventsAreNeverDroppedOnTheSharedScanner(t *testing.T) {
	const n = 150 // more than the 100-slot event channel
	chain := newFakeChain(testHead)
	sc := NewBlockScanner(chain, ScannerConfig{PollInterval: time.Hour}, testLog())
	m := newScannerMonitor(t, sc, 0, testHead)
	other := newScannerMonitor(t, sc, 1, testHead)
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	stranger, _ := testAddr(60000)
	to, _ := testAddr(0)
	toOther, _ := testAddr(1)
	var txs []*core.Transaction
	for i := 0; i < n; i++ {
		txs = append(txs, trxTransferTx(stranger, to, int64(i+1)))
	}
	b := chain.addBlock(txs...)
	b2 := chain.addBlock(trxTransferTx(stranger, toOther, 7))
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Nobody reads m's channel yet: m waits, the scanner and the other address do not.
	if ev := collect(t, other.Events(), 1)[0]; ev.BlockNumber != b2 {
		t.Fatalf("other address got block %d, want %d", ev.BlockNumber, b2)
	}
	waitUntil(t, "m's channel to fill", func() bool { return len(m.Events()) == cap(m.Events()) })
	if got := m.GetLastBlockNumber(); got != b-1 {
		t.Fatalf("progress = %d while events of block %d wait for room, want %d", got, b, b-1)
	}

	events := collect(t, m.Events(), n)
	for i, ev := range events {
		if ev.Amount != int64(i+1) {
			t.Fatalf("event %d has amount %d, want %d (none dropped, order kept)", i, ev.Amount, i+1)
		}
	}
	waitUntil(t, "progress", func() bool { return m.GetLastBlockNumber() == b2 })
}

func TestStopWakesASendWaitingForRoom(t *testing.T) {
	chain := newFakeChain(testHead)
	sc := NewBlockScanner(chain, ScannerConfig{PollInterval: time.Hour}, testLog())
	m := newScannerMonitor(t, sc, 0, testHead)
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	stranger, _ := testAddr(60000)
	to, _ := testAddr(0)
	var txs []*core.Transaction
	for i := 0; i < 120; i++ {
		txs = append(txs, trxTransferTx(stranger, to, 1))
	}
	b := chain.addBlock(txs...)
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the channel to fill", func() bool { return len(m.Events()) == cap(m.Events()) })

	done := make(chan struct{})
	go func() { m.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop deadlocked with a send waiting for room")
	}
	if got := m.GetLastBlockNumber(); got != b-1 {
		t.Fatalf("progress = %d after a stop with undelivered events of block %d, want %d", got, b, b-1)
	}
}

func TestStartAfterStopIsRefused(t *testing.T) {
	chain := newFakeChain(testHead)
	sc := NewBlockScanner(chain, ScannerConfig{PollInterval: time.Hour}, testLog())
	m := newScannerMonitor(t, sc, 0, testHead)
	m.Stop()
	if err := m.Start(); err == nil {
		t.Fatal("Start after Stop must fail")
	}
	if st := sc.Stats(); st.Subscribers != 0 {
		t.Fatalf("a refused Start must not subscribe: %d subscribers", st.Subscribers)
	}

	polling, err := NewAddressMonitorFromSource(chain, Config{WatchAddress: m.watchAddress, StartBlock: testHead, PollInterval: time.Hour}, testLog())
	if err != nil {
		t.Fatal(err)
	}
	polling.Stop()
	if err := polling.Start(); err == nil {
		t.Fatal("Start after Stop must fail for a polling monitor too")
	}
}

func TestSharedEventDataIsCopiedPerAddress(t *testing.T) {
	chain := newFakeChain(testHead)
	sc := NewBlockScanner(chain, ScannerConfig{PollInterval: time.Hour}, testLog())
	mons := newScannerMonitors(t, sc, 2, testHead)
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	from, _ := testAddr(0)
	to, _ := testAddr(1)
	tx := triggerTx(from, to, []byte{0xa9, 0x05, 0x9c, 0xbb, 0, 0})
	chain.addBlock(tx)
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	a := collect(t, mons[0].Events(), 1)[0]
	b := collect(t, mons[1].Events(), 1)[0]
	a.EventData["netUsage"] = int64(-1)
	a.EventData["added"] = true
	if b.EventData["netUsage"] != int64(268) || b.EventData["added"] != nil {
		t.Fatalf("one address's EventData change leaked into the other's: %+v", b.EventData)
	}
}

func TestScannerReportsASkippedBlock(t *testing.T) {
	rec := &skipRecorder{}
	chain := newFakeChain(testHead)
	sc := NewBlockScanner(chain, ScannerConfig{PollInterval: time.Hour, MaxBlockRetries: 2, OnSkip: rec.record}, testLog())
	newScannerMonitor(t, sc, 0, testHead)
	if err := sc.scanOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	b := addTrafficBlock(chain, 0)
	chain.setFailures(b, 100)
	_ = sc.scanOnce(context.Background())
	if len(rec.all()) != 0 {
		t.Fatal("skipped after one failure")
	}
	_ = sc.scanOnce(context.Background())
	skips := rec.all()
	if len(skips) != 1 || skips[0].Block != b || skips[0].Address != "" || skips[0].Attempts != 2 || skips[0].At.IsZero() {
		t.Fatalf("skips = %+v, want block %d for every address", skips, b)
	}

	// A head read that fails never counts toward a skip
	b2 := addTrafficBlock(chain, 0)
	chain.setFailures(b2, 100)
	chain.setHeadDown(true)
	for i := 0; i < 5; i++ {
		_ = sc.scanOnce(context.Background())
	}
	if len(rec.all()) != 1 {
		t.Fatal("a node that is down made the scanner skip a block")
	}
}

// hangingHead is a node that accepts head reads but never answers them.
type hangingHead struct{ *fakeChain }

func (h hangingHead) GetHeadBlockNumber(ctx context.Context) (int64, error) {
	<-ctx.Done()
	return 0, ctx.Err()
}

func (h hangingHead) GetNowBlock(ctx context.Context) (*core.Block, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestHeadReadsHaveADeadline(t *testing.T) {
	chain := newFakeChain(testHead)
	sc := NewBlockScanner(hangingHead{chain}, ScannerConfig{PollInterval: time.Hour, ReadTimeout: 20 * time.Millisecond}, testLog())

	start := time.Now()
	if sc.nodeAnswers(context.Background()) {
		t.Fatal("a node that never answers does not answer")
	}
	if err := sc.scanOnce(context.Background()); err == nil {
		t.Fatal("scanOnce must fail on a hanging head read")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("head reads took %v: no deadline", elapsed)
	}
}
