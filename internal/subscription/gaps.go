package subscription

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/frstrtr/mongotron/internal/blockchain/monitor"
	"github.com/frstrtr/mongotron/internal/storage/models"
	"github.com/frstrtr/mongotron/internal/storage/repositories"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	// DefaultGapRetryInterval is how often skipped blocks are retried.
	DefaultGapRetryInterval = time.Minute
	// DefaultGapMaxAttempts is how many retries a skipped block gets before it is
	// abandoned (about an hour at the default interval).
	DefaultGapMaxAttempts = 60

	gapRetryBatch    = 10   // gaps retried per round
	maxPendingGaps   = 1000 // gaps kept in memory (all are persisted)
	gapStoreTimeout  = 5 * time.Second
	gapFetchTimeout  = 30 * time.Second
	gapLoadAtStartup = maxPendingGaps
)

// GapStore persists the blocks the scanner skipped (the scanner_gaps collection).
// *repositories.ScannerGapRepository implements it.
type GapStore interface {
	Create(ctx context.Context, gap *models.ScannerGap) error
	ListPending(ctx context.Context, limit int64) ([]*models.ScannerGap, error)
	MarkRetried(ctx context.Context, id primitive.ObjectID, attempts int, at time.Time) error
	RecordFailure(ctx context.Context, id primitive.ObjectID, attempts int, lastErr string, abandoned bool, at time.Time) error
}

var _ GapStore = (*repositories.ScannerGapRepository)(nil)

// GapStats are the skipped-block counters of the manager.
type GapStats struct {
	// Recorded counts skipped blocks reported since start; Pending is how many still
	// wait for a retry; Recovered were read again and routed; Abandoned ran out of
	// retries; PersistFailures could not be written to the scanner_gaps collection.
	Recorded        int64   `json:"recorded"`
	Pending         int     `json:"pending"`
	PendingBlocks   []int64 `json:"pendingBlocks,omitempty"` // up to 20, oldest first
	Recovered       int64   `json:"recovered"`
	Abandoned       int64   `json:"abandoned"`
	PersistFailures int64   `json:"persistFailures"`
}

// MonitoringStats are the counters a monitoring system can alert on.
type MonitoringStats struct {
	SharedScanner bool                  `json:"sharedScanner"`
	Scanner       *monitor.ScannerStats `json:"scanner,omitempty"`
	Usage         UsageStats            `json:"usage"`
	Gaps          GapStats              `json:"gaps"`
}

type gapTracker struct {
	mu              sync.Mutex
	pending         []*models.ScannerGap
	recorded        atomic.Int64
	recovered       atomic.Int64
	abandoned       atomic.Int64
	persistFailures atomic.Int64
}

func (g *gapTracker) add(gaps ...*models.ScannerGap) (dropped int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, gap := range gaps {
		if len(g.pending) >= maxPendingGaps {
			dropped++
			continue
		}
		g.pending = append(g.pending, gap)
	}
	sort.SliceStable(g.pending, func(i, j int) bool { return g.pending[i].BlockNumber < g.pending[j].BlockNumber })
	return dropped
}

func (g *gapTracker) batch(n int) []*models.ScannerGap {
	g.mu.Lock()
	defer g.mu.Unlock()
	if n > len(g.pending) {
		n = len(g.pending)
	}
	return append([]*models.ScannerGap(nil), g.pending[:n]...)
}

func (g *gapTracker) remove(gap *models.ScannerGap) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for i, p := range g.pending {
		if p == gap {
			g.pending = append(g.pending[:i], g.pending[i+1:]...)
			return
		}
	}
}

func (g *gapTracker) stats() GapStats {
	g.mu.Lock()
	pending := len(g.pending)
	var blocks []int64
	for i := 0; i < pending && i < 20; i++ {
		blocks = append(blocks, g.pending[i].BlockNumber)
	}
	g.mu.Unlock()
	return GapStats{
		Recorded:        g.recorded.Load(),
		Pending:         pending,
		PendingBlocks:   blocks,
		Recovered:       g.recovered.Load(),
		Abandoned:       g.abandoned.Load(),
		PersistFailures: g.persistFailures.Load(),
	}
}

// MonitoringStats returns the scanner, usage and skipped-block counters.
func (m *Manager) MonitoringStats() MonitoringStats {
	st := MonitoringStats{
		SharedScanner: m.scanner != nil,
		Usage:         m.eventRouter.UsageStats(),
		Gaps:          m.gaps.stats(),
	}
	if m.scanner != nil {
		sc := m.scanner.Stats()
		st.Scanner = &sc
	}
	return st
}

// recordGap is the scanner's OnSkip: the skipped block is persisted (scanner_gaps) and
// queued for a retry.
func (m *Manager) recordGap(sb monitor.SkippedBlock) {
	gap := &models.ScannerGap{
		BlockNumber: sb.Block,
		Address:     sb.Address,
		Error:       sb.Err,
		SkippedAt:   sb.At,
	}
	m.gaps.recorded.Add(1)
	if m.gapStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), gapStoreTimeout)
		err := m.gapStore.Create(ctx, gap)
		cancel()
		if err != nil {
			m.gaps.persistFailures.Add(1)
			m.logger.Error().Err(err).Int64("block", sb.Block).Str("address", sb.Address).
				Msg("Failed to record a skipped block in scanner_gaps (kept in memory only)")
		}
	}
	if dropped := m.gaps.add(gap); dropped > 0 {
		m.logger.Error().Int64("block", sb.Block).Int("pendingLimit", maxPendingGaps).
			Msg("Too many skipped blocks pending: this one is persisted but not retried before a restart")
	}
	m.logger.Warn().
		Str("event", "scanner_gap_recorded").
		Int64("block", sb.Block).
		Str("address", sb.Address).
		Str("error", sb.Err).
		Msg("Skipped block recorded; it will be retried")
}

// loadPendingGaps queues the gaps a previous run left unretried.
func (m *Manager) loadPendingGaps() {
	if m.gapStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(m.ctx, gapStoreTimeout)
	defer cancel()
	gaps, err := m.gapStore.ListPending(ctx, gapLoadAtStartup)
	if err != nil {
		m.logger.Error().Err(err).Msg("Failed to load pending scanner gaps")
		return
	}
	if len(gaps) > 0 {
		m.gaps.add(gaps...)
		m.logger.Warn().Int("count", len(gaps)).Msg("Pending skipped blocks loaded from scanner_gaps; they will be retried")
	}
}

func (m *Manager) gapRetryLoop() {
	ticker := time.NewTicker(m.gapRetryEvery)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.retryGaps(m.ctx)
		}
	}
}

// retryGaps retries the oldest pending gaps once each.
func (m *Manager) retryGaps(ctx context.Context) {
	for _, gap := range m.gaps.batch(gapRetryBatch) {
		if ctx.Err() != nil {
			return
		}
		gap.Attempts++
		err := m.retryGap(ctx, gap)
		now := time.Now()
		if err == nil {
			m.gaps.remove(gap)
			m.gaps.recovered.Add(1)
			m.storeGap(gap, func(ctx context.Context) error {
				return m.gapStore.MarkRetried(ctx, gap.ID, gap.Attempts, now)
			})
			m.logger.Info().
				Str("event", "scanner_gap_recovered").
				Int64("block", gap.BlockNumber).
				Str("address", gap.Address).
				Int("attempts", gap.Attempts).
				Msg("Skipped block read again and its events routed")
			continue
		}
		gap.LastError = err.Error()
		abandoned := gap.Attempts >= m.gapMaxAttempts
		if abandoned {
			m.gaps.remove(gap)
			m.gaps.abandoned.Add(1)
			m.logger.Error().
				Str("event", "scanner_gap_abandoned").
				Int64("block", gap.BlockNumber).
				Str("address", gap.Address).
				Int("attempts", gap.Attempts).
				Err(err).
				Msg("Skipped block could not be read again: given up (see scanner_gaps)")
		}
		m.storeGap(gap, func(ctx context.Context) error {
			return m.gapStore.RecordFailure(ctx, gap.ID, gap.Attempts, gap.LastError, abandoned, now)
		})
	}
}

func (m *Manager) storeGap(gap *models.ScannerGap, write func(ctx context.Context) error) {
	if m.gapStore == nil || gap.ID.IsZero() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), gapStoreTimeout)
	defer cancel()
	if err := write(ctx); err != nil {
		m.logger.Warn().Err(err).Int64("block", gap.BlockNumber).Msg("Failed to update scanner gap")
	}
}

// retryGap reads the gap's block once and routes its events for the watched addresses
// concerned: every address for a gap of the shared scanner, the one address for a gap
// of a catch-up. Porto deduplicates on the transaction, so an event that was already
// delivered is harmless.
func (m *Manager) retryGap(ctx context.Context, gap *models.ScannerGap) error {
	if m.source == nil {
		return errors.New("no block source")
	}
	fetchCtx, cancel := context.WithTimeout(ctx, gapFetchTimeout)
	block, err := m.source.GetBlockByNum(fetchCtx, gap.BlockNumber)
	cancel()
	if err != nil {
		return fmt.Errorf("block %d: %w", gap.BlockNumber, err)
	}
	if block == nil || block.GetBlockHeader() == nil {
		return fmt.Errorf("block %d: empty reply", gap.BlockNumber)
	}

	m.mu.RLock()
	targets := make([]*MonitorWrapper, 0, len(m.monitors))
	for _, w := range m.monitors {
		if _, ok := w.Monitor.(*monitor.AddressMonitor); !ok {
			continue
		}
		if gap.Address != "" && w.current().Address != gap.Address {
			continue
		}
		targets = append(targets, w)
	}
	m.mu.RUnlock()

	for _, w := range targets {
		w := w
		w.Monitor.(*monitor.AddressMonitor).ScanBlock(block, func(event *monitor.AddressEvent) {
			sub := w.current()
			if m.routeMonitorEvent(sub, event) && m.db != nil {
				if err := m.db.SubscriptionRepo.IncrementEventsCount(m.ctx, sub.SubscriptionID); err != nil {
					m.logger.Warn().Err(err).Str("subscriptionId", sub.SubscriptionID).Msg("Failed to count event")
				}
			}
		})
	}
	return nil
}
