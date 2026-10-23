package subscription

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/frstrtr/mongotron/internal/blockchain/client"
	"github.com/frstrtr/mongotron/internal/blockchain/monitor"
	"github.com/frstrtr/mongotron/internal/storage"
	"github.com/frstrtr/mongotron/internal/storage/models"
	"github.com/frstrtr/mongotron/internal/storage/repositories"
	"github.com/frstrtr/mongotron/pkg/logger"
	"github.com/google/uuid"
)

// BlockchainMonitor is an interface for blockchain monitors
type BlockchainMonitor interface {
	Start() error
	Stop()
	Events() <-chan *monitor.AddressEvent
	GetLastBlockNumber() int64
}

// MonitorWrapper wraps a blockchain monitor with subscription info.
//
// Subscription is an immutable snapshot: it is never modified in place once
// published. Changes (settings updates, block progress) build a copy and swap the
// pointer under mu, so the event loop, the router goroutines holding an older
// snapshot and UpdateSubscription never race. Read it through current().
type MonitorWrapper struct {
	Monitor      BlockchainMonitor
	Subscription *models.Subscription
	EventChan    <-chan *monitor.AddressEvent
	StopChan     chan struct{}
	Stopped      bool
	mu           sync.RWMutex
}

// current returns the subscription snapshot the monitor is applying right now.
func (w *MonitorWrapper) current() *models.Subscription {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.Subscription
}

// swap publishes a modified copy of the current subscription snapshot and returns it.
func (w *MonitorWrapper) swap(change func(next *models.Subscription)) *models.Subscription {
	w.mu.Lock()
	defer w.mu.Unlock()
	next := *w.Subscription
	change(&next)
	w.Subscription = &next
	return w.Subscription
}

// advanceCurrentBlock records block progress; it reports whether the block moved forward.
func (w *MonitorWrapper) advanceCurrentBlock(block int64) bool {
	advanced := false
	w.swap(func(next *models.Subscription) {
		if block > next.CurrentBlock {
			next.CurrentBlock = block
			advanced = true
		}
	})
	return advanced
}

// applySettings makes the running monitor use the new settings from its next event on.
func (w *MonitorWrapper) applySettings(upd SubscriptionUpdate) *models.Subscription {
	return w.swap(func(next *models.Subscription) {
		next.Filters = upd.Filters
		next.WalletType = upd.WalletType
		next.UserID = upd.UserID
		next.Label = upd.Label
		next.Metadata = upd.Metadata
	})
}

var (
	// ErrSubscriptionNotFound is returned when no subscription matches.
	ErrSubscriptionNotFound = errors.New("subscription not found")
	// ErrSubscriptionNotActive is returned when an operation needs an active subscription.
	ErrSubscriptionNotActive = errors.New("subscription is not active")
)

// Manager manages active subscriptions and monitors
type Manager struct {
	db          *storage.Database
	tronClient  *client.TronClient
	source      monitor.BlockSource
	scanner     *monitor.BlockScanner // nil when every address polls on its own
	opts        ManagerOptions
	logger      *logger.Logger
	monitors    map[string]*MonitorWrapper // key: subscription_id
	eventRouter *EventRouter
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc

	// Skipped blocks: persisted in scanner_gaps (gapStore, nil without a database)
	// and retried every gapRetryEvery, at most gapMaxAttempts times.
	gapStore       GapStore
	gaps           gapTracker
	gapRetryEvery  time.Duration
	gapMaxAttempts int
}

// ManagerOptions configures how the manager reads blocks.
type ManagerOptions struct {
	// SharedScanner makes every address monitor read from one shared block scanner:
	// each block is fetched and parsed once for all watched addresses. False keeps one
	// polling loop (and one block fetch per block) per watched address.
	SharedScanner bool
	// PollInterval is how often the head is polled (default 3 s).
	PollInterval time.Duration
	// CatchUpWindow is how far behind the newest watched address (in blocks) another
	// one may be at startup and still be caught up by the shared scanner
	// (default monitor.DefaultCatchUpWindow).
	CatchUpWindow int64
}

// DefaultManagerOptions returns the defaults: shared scanner, 3 s poll.
func DefaultManagerOptions() ManagerOptions {
	return ManagerOptions{
		SharedScanner: true,
		PollInterval:  monitor.DefaultScannerPollInterval,
		CatchUpWindow: monitor.DefaultCatchUpWindow,
	}
}

// NewManager creates a new subscription manager with the default options.
func NewManager(db *storage.Database, tronClient *client.TronClient, log *logger.Logger) *Manager {
	return NewManagerWithOptions(db, tronClient, log, DefaultManagerOptions())
}

// NewManagerWithOptions creates a new subscription manager.
func NewManagerWithOptions(db *storage.Database, tronClient *client.TronClient, log *logger.Logger, opts ManagerOptions) *Manager {
	var src monitor.BlockSource
	if tronClient != nil {
		src = tronClient
	}
	m := newManager(db, src, log, opts)
	m.tronClient = tronClient
	return m
}

func newManager(db *storage.Database, src monitor.BlockSource, log *logger.Logger, opts ManagerOptions) *Manager {
	if log == nil {
		defaultLog := logger.NewDefault()
		log = &defaultLog
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = monitor.DefaultScannerPollInterval
	}
	if opts.CatchUpWindow <= 0 {
		opts.CatchUpWindow = monitor.DefaultCatchUpWindow
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		db:             db,
		source:         src,
		opts:           opts,
		logger:         log,
		monitors:       make(map[string]*MonitorWrapper),
		eventRouter:    NewEventRouter(db, log),
		ctx:            ctx,
		cancel:         cancel,
		gapRetryEvery:  DefaultGapRetryInterval,
		gapMaxAttempts: DefaultGapMaxAttempts,
	}
	if db != nil && db.ScannerGapRepo != nil {
		m.gapStore = db.ScannerGapRepo
	}
	if opts.SharedScanner && src != nil {
		m.scanner = monitor.NewBlockScanner(src, monitor.ScannerConfig{
			PollInterval:  opts.PollInterval,
			CatchUpWindow: opts.CatchUpWindow,
			OnSkip:        m.recordGap,
		}, log)
	}

	return m
}

// ScannerStats returns the shared block scanner's counters; false when the manager
// runs one polling loop per address.
func (m *Manager) ScannerStats() (monitor.ScannerStats, bool) {
	if m.scanner == nil {
		return monitor.ScannerStats{}, false
	}
	return m.scanner.Stats(), true
}

// Start starts the subscription manager
func (m *Manager) Start() error {
	m.logger.Info().Msg("Starting subscription manager")

	// Load active subscriptions from database
	subs, err := m.db.SubscriptionRepo.FindActive(m.ctx)
	if err != nil {
		return fmt.Errorf("failed to load active subscriptions: %w", err)
	}

	m.logger.Info().Int("count", len(subs)).Msg("Loading active subscriptions")

	// Start monitors for each active subscription
	for _, sub := range subs {
		if err := m.startMonitor(sub); err != nil {
			m.logger.Error().
				Err(err).
				Str("subscriptionId", sub.SubscriptionID).
				Msg("Failed to start monitor for subscription")
			continue
		}
	}

	// Start event router
	go m.eventRouter.Run(m.ctx)

	// The shared scanner starts after the stored subscriptions subscribed to it, so its
	// first head read can catch them all up together. Blocks it (or a catch-up) has to
	// skip are recorded and retried; those a previous run left are loaded first.
	if m.scanner != nil {
		m.loadPendingGaps()
		if err := m.scanner.Start(); err != nil {
			return fmt.Errorf("failed to start block scanner: %w", err)
		}
		go m.gapRetryLoop()
	}

	return nil
}

// Stop stops the subscription manager
func (m *Manager) Stop() error {
	m.logger.Info().Msg("Stopping subscription manager")

	m.cancel()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop all monitors
	for _, wrapper := range m.monitors {
		m.stopMonitorUnsafe(wrapper)
	}

	if m.scanner != nil {
		m.scanner.Stop()
	}

	return nil
}

// Subscribe creates a new subscription and starts monitoring
func (m *Manager) Subscribe(address string, webhookURL string, filters models.SubscriptionFilters, startBlock int64) (*models.Subscription, error) {
	return m.SubscribeWithOptions(SubscribeOptions{
		Address:    address,
		WebhookURL: webhookURL,
		Filters:    filters,
		StartBlock: startBlock,
	})
}

// SubscribeWithOptions creates a new subscription with full options including metadata
func (m *Manager) SubscribeWithOptions(opts SubscribeOptions) (*models.Subscription, error) {
	// Generate subscription ID
	subscriptionID := fmt.Sprintf("sub_%s", uuid.New().String()[:12])

	// Create subscription model
	sub := &models.Subscription{
		SubscriptionID: subscriptionID,
		Address:        opts.Address,
		Network:        "tron-nile", // TODO: Make configurable
		WebhookURL:     opts.WebhookURL,
		Filters:        opts.Filters,
		Status:         "active",
		EventsCount:    0,
		StartBlock:     opts.StartBlock,
		CurrentBlock:   opts.StartBlock,
		WalletType:     opts.WalletType,
		UserID:         opts.UserID,
		Label:          opts.Label,
		Metadata:       opts.Metadata,
	}

	// Save to database
	if err := m.db.SubscriptionRepo.Create(m.ctx, sub); err != nil {
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}

	// Start monitoring
	if err := m.startMonitor(sub); err != nil {
		// Rollback: delete subscription
		if delErr := m.db.SubscriptionRepo.Delete(m.ctx, sub.ID); delErr != nil {
			m.logger.Error().
				Err(delErr).
				Str("subscriptionId", sub.SubscriptionID).
				Msg("Failed to roll back subscription after monitor start failure")
		}
		return nil, fmt.Errorf("failed to start monitor: %w", err)
	}

	m.logger.Info().
		Str("subscriptionId", sub.SubscriptionID).
		Str("address", opts.Address).
		Str("walletType", opts.WalletType).
		Str("userId", opts.UserID).
		Msg("Subscription created")

	return sub, nil
}

// BatchSubscribe creates multiple subscriptions in a single operation
// This is optimized for bulk registration of platform deposit addresses
func (m *Manager) BatchSubscribe(opts []SubscribeOptions) (*BatchSubscribeResult, error) {
	result := &BatchSubscribeResult{
		Success: make([]*models.Subscription, 0, len(opts)),
		Failed:  make([]BatchSubscribeFailure, 0),
	}

	for _, opt := range opts {
		sub, err := m.SubscribeWithOptions(opt)
		if err != nil {
			result.Failed = append(result.Failed, BatchSubscribeFailure{
				Address: opt.Address,
				Error:   err.Error(),
			})
			m.logger.Warn().
				Err(err).
				Str("address", opt.Address).
				Msg("Failed to subscribe address in batch")
			continue
		}
		result.Success = append(result.Success, sub)
	}

	m.logger.Info().
		Int("total", len(opts)).
		Int("success", len(result.Success)).
		Int("failed", len(result.Failed)).
		Msg("Batch subscription completed")

	return result, nil
}

// Unsubscribe stops monitoring and marks subscription as stopped
// Records the last seen block for potential gap scanning on resubscribe
func (m *Manager) Unsubscribe(subscriptionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	wrapper, exists := m.monitors[subscriptionID]
	if !exists {
		return m.stopWithoutMonitor(subscriptionID)
	}

	// Get the last processed block from the monitor before stopping
	sub := wrapper.current()
	lastBlock := wrapper.Monitor.GetLastBlockNumber()
	if lastBlock == 0 {
		lastBlock = sub.CurrentBlock
	}

	// Stop the monitor
	m.stopMonitorUnsafe(wrapper)

	// Update database with status AND last seen block (for gap scanning on resubscribe)
	if err := m.db.SubscriptionRepo.UpdateStatusWithBlock(m.ctx, sub.ID, "stopped", lastBlock); err != nil {
		m.logger.Error().
			Err(err).
			Str("subscriptionId", subscriptionID).
			Int64("lastSeenBlock", lastBlock).
			Msg("Failed to update subscription status with last block")
	}

	// Remove from active monitors
	delete(m.monitors, subscriptionID)

	m.logger.Info().
		Str("subscriptionId", subscriptionID).
		Int64("lastSeenBlock", lastBlock).
		Msg("Subscription stopped - last seen block recorded for gap scanning")

	return nil
}

// stopWithoutMonitor stops a subscription that has no running monitor (caller holds
// m.mu). A row that is active in storage without a monitor (its monitor failed to
// start) is marked stopped at its stored block, so it can be removed and later
// resubscribed. A subscription that is not active returns ErrSubscriptionNotActive.
func (m *Manager) stopWithoutMonitor(subscriptionID string) error {
	sub, err := m.db.SubscriptionRepo.FindBySubscriptionID(m.ctx, subscriptionID)
	if err != nil {
		return err
	}
	if sub.Status != "active" {
		return fmt.Errorf("%w (status %q)", ErrSubscriptionNotActive, sub.Status)
	}

	if err := m.db.SubscriptionRepo.UpdateStatusWithBlock(m.ctx, sub.ID, "stopped", sub.CurrentBlock); err != nil {
		return fmt.Errorf("failed to stop subscription: %w", err)
	}

	m.logger.Warn().
		Str("subscriptionId", subscriptionID).
		Int64("lastSeenBlock", sub.CurrentBlock).
		Msg("Subscription was active in storage without a running monitor; marked stopped")

	return nil
}

// UpdateSubscription changes the filters, wallet type, user ID, label and metadata of
// an active subscription in place: the stored row is updated and the running monitor
// applies the new settings from its next event on. The monitor keeps running, so
// there is no gap in coverage and no second subscription for the address. The
// update is a full replacement of those five fields; the caller resolves which
// values to keep. It fails when the subscription is not active.
func (m *Manager) UpdateSubscription(subscriptionID string, upd SubscriptionUpdate) (*models.Subscription, error) {
	sub, err := m.db.SubscriptionRepo.FindBySubscriptionID(m.ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	if sub.Status != "active" {
		return nil, fmt.Errorf("subscription %s is not active (status %q)", subscriptionID, sub.Status)
	}

	if err := m.db.SubscriptionRepo.UpdateSettings(m.ctx, subscriptionID, repositories.SubscriptionSettings{
		Filters:    upd.Filters,
		WalletType: upd.WalletType,
		UserID:     upd.UserID,
		Label:      upd.Label,
		Metadata:   upd.Metadata,
	}); err != nil {
		return nil, fmt.Errorf("failed to update subscription: %w", err)
	}

	m.mu.RLock()
	wrapper, running := m.monitors[subscriptionID]
	m.mu.RUnlock()
	if running {
		wrapper.applySettings(upd)
	} else {
		m.logger.Warn().
			Str("subscriptionId", subscriptionID).
			Msg("Subscription updated in storage but no running monitor was found; it applies on the next start")
	}

	sub.Filters = upd.Filters
	sub.WalletType = upd.WalletType
	sub.UserID = upd.UserID
	sub.Label = upd.Label
	sub.Metadata = upd.Metadata

	m.logger.Info().
		Str("subscriptionId", subscriptionID).
		Str("address", sub.Address).
		Strs("assetTypes", upd.Filters.AssetTypes).
		Strs("contractTypes", upd.Filters.ContractTypes).
		Str("walletType", upd.WalletType).
		Bool("monitorRefreshed", running).
		Msg("Subscription updated in place")

	return sub, nil
}

// GetSubscription retrieves a subscription by ID
func (m *Manager) GetSubscription(subscriptionID string) (*models.Subscription, error) {
	return m.db.SubscriptionRepo.FindBySubscriptionID(m.ctx, subscriptionID)
}

// GetByAddress retrieves a subscription by wallet address
func (m *Manager) GetByAddress(address string) (*models.Subscription, error) {
	subs, err := m.db.SubscriptionRepo.FindByAddress(m.ctx, address)
	if err != nil {
		return nil, err
	}
	if len(subs) == 0 {
		return nil, fmt.Errorf("%w for address: %s", ErrSubscriptionNotFound, address)
	}
	// Return the newest active subscription for this address, else the newest row
	// (FindByAddress returns the newest first)
	for _, sub := range subs {
		if sub.Status == "active" {
			return sub, nil
		}
	}
	return subs[0], nil
}

// List lists all subscriptions with pagination (alias for ListSubscriptions)
func (m *Manager) List(limit, skip int64) ([]*models.Subscription, int64, error) {
	return m.db.SubscriptionRepo.List(m.ctx, limit, skip)
}

// ListSubscriptions lists all subscriptions with pagination
func (m *Manager) ListSubscriptions(limit, skip int64) ([]*models.Subscription, int64, error) {
	return m.db.SubscriptionRepo.List(m.ctx, limit, skip)
}

// GetActiveMonitorsCount returns the number of active monitors
func (m *Manager) GetActiveMonitorsCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.monitors)
}

// GetEventRouter returns the event router
func (m *Manager) GetEventRouter() *EventRouter {
	return m.eventRouter
}

// startMonitor creates and starts an address monitor for a subscription
func (m *Manager) startMonitor(sub *models.Subscription) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already monitoring
	if _, exists := m.monitors[sub.SubscriptionID]; exists {
		return fmt.Errorf("subscription already active")
	}

	var monitorInstance BlockchainMonitor
	var eventChan <-chan *monitor.AddressEvent

	// Create either global or address-specific monitor
	if sub.Address == "" {
		// Global monitoring - watch all transactions
		m.logger.Info().
			Str("subscriptionId", sub.SubscriptionID).
			Msg("Creating global monitor")

		globalCfg := monitor.GlobalConfig{
			PollInterval: 3 * time.Second,
			StartBlock:   sub.CurrentBlock,
		}

		if m.tronClient == nil {
			return fmt.Errorf("global monitoring needs a TRON client")
		}
		globalMonitor, err := monitor.NewGlobalMonitor(
			m.tronClient,
			globalCfg,
			m.logger,
		)
		if err != nil {
			return fmt.Errorf("failed to create global monitor: %w", err)
		}

		monitorInstance = globalMonitor
		eventChan = globalMonitor.Events()

	} else {
		// Address-specific monitoring (on the shared scanner when there is one)
		cfg := monitor.Config{
			WatchAddress: sub.Address,
			PollInterval: m.opts.PollInterval,
			StartBlock:   sub.CurrentBlock,
			Scanner:      m.scanner,
		}

		addrMonitor, err := monitor.NewAddressMonitorFromSource(
			m.source,
			cfg,
			m.logger,
		)
		if err != nil {
			return fmt.Errorf("failed to create address monitor: %w", err)
		}

		monitorInstance = addrMonitor
		eventChan = addrMonitor.Events()
	}

	// Create wrapper
	wrapper := &MonitorWrapper{
		Monitor:      monitorInstance,
		Subscription: sub,
		EventChan:    eventChan,
		StopChan:     make(chan struct{}),
		Stopped:      false,
	}

	// Store in map
	m.monitors[sub.SubscriptionID] = wrapper

	// Start the monitor. On the shared scanner Start only subscribes (no node read), so
	// it runs here: the subscriptions loaded at startup are all registered before the
	// scanner's first head read. A polling monitor reads the node in Start: background.
	start := func() {
		if err := monitorInstance.Start(); err != nil {
			m.logger.Error().
				Err(err).
				Str("subscriptionId", sub.SubscriptionID).
				Msg("Monitor error")
		}
	}
	if m.scanner != nil && sub.Address != "" {
		start()
	} else {
		go start()
	}

	// Start event processor goroutine
	go m.processEvents(wrapper)

	addressInfo := sub.Address
	if addressInfo == "" {
		addressInfo = "GLOBAL"
	}

	m.logger.Info().
		Str("subscriptionId", sub.SubscriptionID).
		Str("address", addressInfo).
		Int64("startBlock", sub.StartBlock).
		Msg("Monitor started")

	return nil
}

// stopMonitorUnsafe stops a monitor (caller must hold lock)
func (m *Manager) stopMonitorUnsafe(wrapper *MonitorWrapper) {
	wrapper.mu.Lock()
	defer wrapper.mu.Unlock()

	if wrapper.Stopped {
		return
	}

	wrapper.Stopped = true
	close(wrapper.StopChan)

	if wrapper.Monitor != nil {
		wrapper.Monitor.Stop()
	}
}

// processEvents processes events from a monitor
func (m *Manager) processEvents(wrapper *MonitorWrapper) {
	// Ticker to periodically update current block even without events
	blockUpdateTicker := time.NewTicker(10 * time.Second)
	defer blockUpdateTicker.Stop()

	for {
		select {
		case <-wrapper.StopChan:
			return

		case <-blockUpdateTicker.C:
			// Periodically update current block from monitor
			if wrapper.Monitor != nil {
				currentBlock := wrapper.Monitor.GetLastBlockNumber()
				if wrapper.advanceCurrentBlock(currentBlock) && m.db != nil {
					subscriptionID := wrapper.current().SubscriptionID
					if err := m.db.SubscriptionRepo.UpdateCurrentBlock(m.ctx, subscriptionID, currentBlock); err != nil {
						m.logger.Warn().Err(err).Str("subscriptionId", subscriptionID).Msg("Failed to store current block")
					}
					m.logger.Debug().
						Str("subscriptionId", subscriptionID).
						Int64("currentBlock", currentBlock).
						Msg("Updated current block")
				}
			}

		case event, ok := <-wrapper.EventChan:
			if !ok {
				// The monitor stopped and closed its channel
				return
			}
			// Take the current snapshot per event so a settings update applies
			// from the next event on, without restarting the monitor.
			sub := wrapper.current()

			// Usage event, then filters and routing
			if !m.routeMonitorEvent(sub, event) {
				continue
			}

			if m.db == nil {
				wrapper.advanceCurrentBlock(event.BlockNumber)
				continue
			}

			// Update subscription stats
			if err := m.db.SubscriptionRepo.IncrementEventsCount(m.ctx, sub.SubscriptionID); err != nil {
				m.logger.Warn().Err(err).Str("subscriptionId", sub.SubscriptionID).Msg("Failed to count event")
			}

			// Update current block
			if wrapper.advanceCurrentBlock(event.BlockNumber) {
				if err := m.db.SubscriptionRepo.UpdateCurrentBlock(m.ctx, sub.SubscriptionID, event.BlockNumber); err != nil {
					m.logger.Warn().Err(err).Str("subscriptionId", sub.SubscriptionID).Msg("Failed to store current block")
				}
			}

			m.logger.Debug().
				Str("subscriptionId", sub.SubscriptionID).
				Str("txHash", event.TransactionID).
				Int64("block", event.BlockNumber).
				Msg("Event processed")
		}
	}
}

// routeMonitorEvent hands a monitor event to the router: first the energy_usage event
// (independent of the transfer filters: a failed call, or a call of any contract,
// still consumed the owner's resources), then, when the event passes the
// subscription's filters, the normal routing. It reports whether the event passed the
// filters.
func (m *Manager) routeMonitorEvent(sub *models.Subscription, event *monitor.AddressEvent) bool {
	if sub == nil || event == nil {
		return false
	}

	m.eventRouter.RouteUsage(sub, event)

	if !m.matchesFilters(event, sub.Filters) {
		return false
	}

	if err := m.eventRouter.RouteEvent(sub, event); err != nil {
		m.logger.Error().
			Err(err).
			Str("subscriptionId", sub.SubscriptionID).
			Msg("Failed to route event")
	}
	return true
}

// matchesFilters checks if an event matches subscription filters
func (m *Manager) matchesFilters(event *monitor.AddressEvent, filters models.SubscriptionFilters) bool {
	// Contract type filter
	if len(filters.ContractTypes) > 0 {
		matched := false
		for _, ct := range filters.ContractTypes {
			if event.ContractType == ct {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Amount filters
	if filters.MinAmount > 0 && event.Amount < filters.MinAmount {
		return false
	}

	if filters.MaxAmount > 0 && event.Amount > filters.MaxAmount {
		return false
	}

	// Success filter
	if filters.OnlySuccess && !event.Success {
		return false
	}

	return true
}

// ScanHistorical triggers a historical block scan for a subscription
// This scans blocks from fromBlock to toBlock and routes any events found
// through the normal event processing pipeline (including Porto webhooks)
func (m *Manager) ScanHistorical(subscriptionID string, fromBlock, toBlock int64) error {
	m.mu.RLock()
	wrapper, exists := m.monitors[subscriptionID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("subscription not found or not active: %s", subscriptionID)
	}

	// Get the address monitor (we need the concrete type for ScanHistorical)
	addrMonitor, ok := wrapper.Monitor.(*monitor.AddressMonitor)
	if !ok {
		return fmt.Errorf("historical scan only supported for address monitors")
	}

	m.logger.Info().
		Str("subscriptionId", subscriptionID).
		Str("address", wrapper.current().Address).
		Int64("fromBlock", fromBlock).
		Int64("toBlock", toBlock).
		Msg("Starting historical scan for subscription")

	// Create a callback that routes events through the normal pipeline
	callback := func(event *monitor.AddressEvent) {
		// Apply the subscription's current filters (an update during the scan applies too)
		// and route the event through the event router (usage event, TRC20 detection and
		// Porto webhooks)
		sub := wrapper.current()
		if !m.routeMonitorEvent(sub, event) {
			return
		}

		// Update subscription stats
		if err := m.db.SubscriptionRepo.IncrementEventsCount(m.ctx, subscriptionID); err != nil {
			m.logger.Warn().Err(err).Str("subscriptionId", subscriptionID).Msg("Failed to count historical event")
		}

		m.logger.Debug().
			Str("subscriptionId", subscriptionID).
			Str("txHash", event.TransactionID).
			Int64("block", event.BlockNumber).
			Msg("Historical event routed")
	}

	// Run the scan (this is synchronous but can be made async if needed)
	if err := addrMonitor.ScanHistorical(fromBlock, toBlock, callback); err != nil {
		return fmt.Errorf("historical scan failed: %w", err)
	}

	return nil
}
