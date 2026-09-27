package subscription

import (
	"fmt"
	"sort"

	"github.com/frstrtr/mongotron/internal/storage/models"
)

// Resubscribe actions, reported in ResubscribeResult.Action.
const (
	// ResubscribeRefreshed means the address already had an active subscription;
	// it was updated in place and no second subscription was created.
	ResubscribeRefreshed = "refreshed"
	// ResubscribeReactivated means the address's latest (stopped) subscription
	// was reactivated.
	ResubscribeReactivated = "reactivated"
	// ResubscribeCreated means the address was never watched; a new
	// subscription was created.
	ResubscribeCreated = "created"
)

// ResubscribeOptions controls a resubscription.
type ResubscribeOptions struct {
	Address string
	// WebhookURL replaces the stored webhook URL when a stopped subscription is
	// reactivated or a new one is created. Empty keeps the stored URL. Like the
	// watchlist upsert, it never changes an active subscription.
	WebhookURL string
	// AssetTypes overrides the stored asset types (an empty list means all
	// transfer types). Nil keeps the asset types of the latest subscription row.
	AssetTypes []string
	// WalletType replaces the stored wallet type when non-empty.
	WalletType string
	// ScanGap starts a background scan of a detected gap.
	ScanGap bool
}

// ResubscribeResult contains the result of a resubscription operation
type ResubscribeResult struct {
	Subscription *models.Subscription
	// Action is one of ResubscribeRefreshed, ResubscribeReactivated, ResubscribeCreated.
	Action      string
	GapDetected bool
	GapStart    int64
	GapEnd      int64
	GapScanning bool
	// MonitorStarted reports that an active subscription had no running monitor
	// and one was started (it resumes from the stored current block).
	MonitorStarted bool
}

// resubscribePlan is what Resubscribe will do, decided from the stored rows only.
type resubscribePlan struct {
	action string
	// target is the active subscription to refresh or the stopped one to
	// reactivate; nil when a new subscription is created.
	target *models.Subscription
	// otherActive lists further active rows for the address (left by older
	// versions); they are reported, never multiplied.
	otherActive []*models.Subscription
	filters     models.SubscriptionFilters
	walletType  string

	// gapRow is the stopped row whose last seen block opens the gap, nil when
	// there is no gap to recover.
	gapRow   *models.Subscription
	gapStart int64
	// gapEnd is the end of the gap when it is known from the stored rows. Zero
	// means "up to the current block".
	gapEnd int64
}

// newestFirst returns the rows ordered by creation time, newest first.
func newestFirst(subs []*models.Subscription) []*models.Subscription {
	ordered := make([]*models.Subscription, 0, len(subs))
	for _, sub := range subs {
		if sub != nil {
			ordered = append(ordered, sub)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].CreatedAt.After(ordered[j].CreatedAt)
	})
	return ordered
}

// planResubscribe decides how to resubscribe an address from its stored rows.
//
//   - An active row exists: it is refreshed in place (the newest one when there
//     are several). A gap is only recovered when that row was created as a
//     replacement for a stopped row (a new subscription after an unsubscribe,
//     not a reactivation) and that stop's gap was not scanned yet: the gap runs
//     from the stopped row's last seen block to the active row's start block,
//     or to the current block when the start was not recorded.
//   - Otherwise the newest row is reactivated; its gap runs from its last seen
//     block to the current block.
//   - Without any row, a new subscription is created.
//
// Filters keep the stored ones of the target row (contract types rebuilt from
// its asset types when it has them), unless opts.AssetTypes overrides them.
func planResubscribe(subs []*models.Subscription, opts ResubscribeOptions) resubscribePlan {
	ordered := newestFirst(subs)

	var active []*models.Subscription
	for _, sub := range ordered {
		if sub.Status == "active" {
			active = append(active, sub)
		}
	}

	var p resubscribePlan
	switch {
	case len(active) > 0:
		p.action = ResubscribeRefreshed
		p.target = active[0]
		p.otherActive = active[1:]
		if gapRow := replacedStop(ordered, p.target); gapRow != nil {
			p.gapRow = gapRow
			p.gapStart = gapRow.LastSeenBlock
			if p.target.StartBlock > 0 {
				p.gapEnd = p.target.StartBlock
			}
		}
	case len(ordered) > 0:
		p.action = ResubscribeReactivated
		p.target = ordered[0]
		if p.target.LastSeenBlock > 0 {
			p.gapRow = p.target
			p.gapStart = p.target.LastSeenBlock
		}
	default:
		p.action = ResubscribeCreated
	}

	if p.target != nil {
		p.filters = storedFilters(p.target.Filters, opts.AssetTypes)
		p.walletType = p.target.WalletType
	} else {
		p.filters = FiltersForAssets(opts.AssetTypes, nil)
		p.walletType = "general"
	}
	if opts.WalletType != "" {
		p.walletType = opts.WalletType
	}

	// An explicit start at or before the stop means the active row covered it
	if p.gapRow != nil && p.gapEnd != 0 && p.gapEnd <= p.gapStart {
		p.gapRow, p.gapStart, p.gapEnd = nil, 0, 0
	}

	return p
}

// replacedStop returns the most recent stopped row that the active row replaced
// and whose gap was not scanned yet, or nil. A row that was reactivated (it has
// its own stop time) resumed its own gap at reactivation, so it has none.
func replacedStop(ordered []*models.Subscription, active *models.Subscription) *models.Subscription {
	if active.StoppedAt != nil {
		return nil
	}
	for _, sub := range ordered {
		if sub == active || sub.Status == "active" {
			continue
		}
		if sub.StoppedAt == nil || sub.StoppedAt.After(active.CreatedAt) {
			// Stopped while the active row was already running: nothing was missed
			continue
		}
		if sub.LastSeenBlock <= 0 || sub.GapScannedAt != nil {
			// The latest stop before the active row has no open gap
			return nil
		}
		return sub
	}
	return nil
}

// storedFilters keeps the stored filters, rebuilding the contract types from the
// stored asset types with the shared mapping, or applies the asset types given.
// Legacy rows without asset types keep their contract types as stored.
func storedFilters(stored models.SubscriptionFilters, assetTypes []string) models.SubscriptionFilters {
	if assetTypes != nil {
		return WithAssetTypes(stored, assetTypes)
	}
	if stored.AssetTypes != nil {
		stored.ContractTypes = ContractTypesForAssets(stored.AssetTypes)
	}
	return stored
}

// Resubscribe makes sure the address is watched, with at most one active
// subscription, and optionally scans for transactions missed while it was not:
//
//   - If the address has an active subscription, it is updated in place (filters
//     and wallet type) and the running monitor applies them from its next event
//     on; if no monitor is running for it, one is started from its stored block.
//   - Otherwise the latest subscription row is reactivated from the current block.
//   - Without any row, a new subscription is created from the current block.
//
// See planResubscribe for filters and gap detection. A detected gap is scanned in
// the background when opts.ScanGap is set.
func (m *Manager) Resubscribe(opts ResubscribeOptions) (*ResubscribeResult, error) {
	subs, err := m.db.SubscriptionRepo.FindByAddress(m.ctx, opts.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup address: %w", err)
	}

	plan := planResubscribe(subs, opts)
	for _, other := range plan.otherActive {
		m.logger.Warn().
			Str("address", opts.Address).
			Str("subscriptionId", other.SubscriptionID).
			Str("keptSubscriptionId", plan.target.SubscriptionID).
			Msg("Address has more than one active subscription; resubscribe refreshes the newest only")
	}

	result := &ResubscribeResult{Action: plan.action}

	switch plan.action {
	case ResubscribeRefreshed:
		sub, started, err := m.refreshActive(plan)
		if err != nil {
			return nil, err
		}
		result.Subscription = sub
		result.MonitorStarted = started

	case ResubscribeReactivated:
		currentBlock, err := m.currentBlock()
		if err != nil {
			return nil, err
		}
		sub, err := m.reactivate(plan, opts.WebhookURL, currentBlock)
		if err != nil {
			return nil, err
		}
		result.Subscription = sub
		plan.gapEnd = currentBlock

	case ResubscribeCreated:
		sub, err := m.SubscribeWithOptions(SubscribeOptions{
			Address:    opts.Address,
			WebhookURL: opts.WebhookURL,
			Filters:    plan.filters,
			StartBlock: -1, // current block
			WalletType: plan.walletType,
		})
		if err != nil {
			return nil, err
		}
		result.Subscription = sub
		return result, nil
	}

	if plan.gapRow == nil {
		return result, nil
	}

	if plan.gapEnd == 0 {
		// The active row's start was not recorded: scan up to now (blocks the
		// monitor already covered may be delivered again)
		currentBlock, err := m.currentBlock()
		if err != nil {
			return nil, err
		}
		plan.gapEnd = currentBlock
	}
	if plan.gapEnd <= plan.gapStart {
		return result, nil
	}

	result.GapDetected = true
	result.GapStart = plan.gapStart
	result.GapEnd = plan.gapEnd

	m.logger.Info().
		Str("address", opts.Address).
		Str("action", plan.action).
		Int64("gapStart", result.GapStart).
		Int64("gapEnd", result.GapEnd).
		Int64("gapBlocks", result.GapEnd-result.GapStart).
		Msg("Gap detected for resubscription")

	if opts.ScanGap {
		result.GapScanning = true
		if plan.gapRow != plan.target {
			// The gap belongs to a replaced row: record it so it is scanned once
			if err := m.db.SubscriptionRepo.MarkGapScanned(m.ctx, plan.gapRow.ID); err != nil {
				m.logger.Warn().
					Err(err).
					Str("subscriptionId", plan.gapRow.SubscriptionID).
					Msg("Failed to record the gap scan; a later resubscribe may scan it again")
			}
		}
		m.scanGapInBackground(result.Subscription.SubscriptionID, result.GapStart, result.GapEnd)
	}

	return result, nil
}

// currentBlock returns the latest block number from the node.
func (m *Manager) currentBlock() (int64, error) {
	block, err := m.tronClient.GetNowBlock(m.ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get current block: %w", err)
	}
	return block.BlockHeader.RawData.Number, nil
}

// refreshActive updates the active subscription in place and starts its monitor
// when none is running. It reports whether a monitor was started.
func (m *Manager) refreshActive(plan resubscribePlan) (*models.Subscription, bool, error) {
	target := plan.target
	sub, err := m.UpdateSubscription(target.SubscriptionID, SubscriptionUpdate{
		Filters:    plan.filters,
		WalletType: plan.walletType,
		UserID:     target.UserID,
		Label:      target.Label,
		Metadata:   target.Metadata,
	})
	if err != nil {
		return nil, false, err
	}

	m.mu.RLock()
	_, running := m.monitors[sub.SubscriptionID]
	m.mu.RUnlock()
	if running {
		return sub, false, nil
	}

	if err := m.startMonitor(sub); err != nil {
		return nil, false, fmt.Errorf("failed to start monitor: %w", err)
	}
	m.logger.Info().
		Str("subscriptionId", sub.SubscriptionID).
		Int64("currentBlock", sub.CurrentBlock).
		Msg("Active subscription had no running monitor; started it from the stored block")
	return sub, true, nil
}

// reactivate marks the stopped target active with the planned settings and starts
// its monitor from currentBlock. If the monitor cannot start, the row is put back
// to stopped so the address is not left active without a monitor.
func (m *Manager) reactivate(plan resubscribePlan, webhookURL string, currentBlock int64) (*models.Subscription, error) {
	sub := *plan.target
	previousStatus := sub.Status
	sub.Status = "active"
	if webhookURL != "" {
		sub.WebhookURL = webhookURL
	}
	sub.Filters = plan.filters
	sub.WalletType = plan.walletType
	sub.CurrentBlock = currentBlock

	if err := m.db.SubscriptionRepo.Update(m.ctx, &sub); err != nil {
		return nil, fmt.Errorf("failed to reactivate subscription: %w", err)
	}

	if err := m.startMonitor(&sub); err != nil {
		if revertErr := m.db.SubscriptionRepo.UpdateStatus(m.ctx, sub.ID, previousStatus); revertErr != nil {
			m.logger.Error().
				Err(revertErr).
				Str("subscriptionId", sub.SubscriptionID).
				Msg("Failed to revert subscription status after monitor start failure")
		}
		return nil, fmt.Errorf("failed to start monitor: %w", err)
	}

	m.logger.Info().
		Str("subscriptionId", sub.SubscriptionID).
		Str("address", sub.Address).
		Int64("currentBlock", currentBlock).
		Strs("assetTypes", sub.Filters.AssetTypes).
		Msg("Subscription reactivated")

	return &sub, nil
}

// scanGapInBackground scans [from, to] for the subscription through the normal
// event pipeline.
func (m *Manager) scanGapInBackground(subscriptionID string, from, to int64) {
	go func() {
		m.logger.Info().
			Str("subscriptionId", subscriptionID).
			Int64("startBlock", from).
			Int64("endBlock", to).
			Msg("Starting background gap scan")

		if err := m.ScanHistorical(subscriptionID, from, to); err != nil {
			m.logger.Error().
				Err(err).
				Str("subscriptionId", subscriptionID).
				Msg("Gap scan failed")
			return
		}
		m.logger.Info().
			Str("subscriptionId", subscriptionID).
			Msg("Gap scan completed successfully")
	}()
}
