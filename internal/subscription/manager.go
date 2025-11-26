package subscription

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/frstrtr/mongotron/internal/blockchain/client"
	"github.com/frstrtr/mongotron/internal/blockchain/monitor"
	"github.com/frstrtr/mongotron/internal/storage"
	"github.com/frstrtr/mongotron/internal/storage/models"
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

// MonitorWrapper wraps a blockchain monitor with subscription info
type MonitorWrapper struct {
	Monitor      BlockchainMonitor
	Subscription *models.Subscription
	EventChan    <-chan *monitor.AddressEvent
	StopChan     chan struct{}
	Stopped      bool
	mu           sync.RWMutex
}

// Manager manages active subscriptions and monitors
type Manager struct {
	db          *storage.Database
	tronClient  *client.TronClient
	logger      *logger.Logger
	monitors    map[string]*MonitorWrapper // key: subscription_id
	eventRouter *EventRouter
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewManager creates a new subscription manager
func NewManager(db *storage.Database, tronClient *client.TronClient, log *logger.Logger) *Manager {
	if log == nil {
		defaultLog := logger.NewDefault()
		log = &defaultLog
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		db:          db,
		tronClient:  tronClient,
		logger:      log,
		monitors:    make(map[string]*MonitorWrapper),
		eventRouter: NewEventRouter(db, log),
		ctx:         ctx,
		cancel:      cancel,
	}

	return m
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

	return nil
}

// Subscribe creates a new subscription and starts monitoring
func (m *Manager) Subscribe(address string, webhookURL string, filters models.SubscriptionFilters, startBlock int64) (*models.Subscription, error) {
	// Generate subscription ID
	subscriptionID := fmt.Sprintf("sub_%s", uuid.New().String()[:12])

	// Create subscription model
	sub := &models.Subscription{
		SubscriptionID: subscriptionID,
		Address:        address,
		Network:        "tron-nile", // TODO: Make configurable
		WebhookURL:     webhookURL,
		Filters:        filters,
		Status:         "active",
		EventsCount:    0,
		StartBlock:     startBlock,
		CurrentBlock:   startBlock,
	}

	// Save to database
	if err := m.db.SubscriptionRepo.Create(m.ctx, sub); err != nil {
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}

	// Start monitoring
	if err := m.startMonitor(sub); err != nil {
		// Rollback: delete subscription
		m.db.SubscriptionRepo.Delete(m.ctx, sub.ID)
		return nil, fmt.Errorf("failed to start monitor: %w", err)
	}

	m.logger.Info().
		Str("subscriptionId", sub.SubscriptionID).
		Str("address", address).
		Msg("Subscription created")

	return sub, nil
}

// Unsubscribe stops monitoring and marks subscription as stopped
func (m *Manager) Unsubscribe(subscriptionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	wrapper, exists := m.monitors[subscriptionID]
	if !exists {
		return fmt.Errorf("subscription not found")
	}

	// Stop the monitor
	m.stopMonitorUnsafe(wrapper)

	// Update database
	if err := m.db.SubscriptionRepo.UpdateStatus(m.ctx, wrapper.Subscription.ID, "stopped"); err != nil {
		m.logger.Error().
			Err(err).
			Str("subscriptionId", subscriptionID).
			Msg("Failed to update subscription status")
	}

	// Remove from active monitors
	delete(m.monitors, subscriptionID)

	m.logger.Info().
		Str("subscriptionId", subscriptionID).
		Msg("Subscription stopped")

	return nil
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
		return nil, fmt.Errorf("subscription not found for address: %s", address)
	}
	// Return the first active subscription for this address
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
		// Address-specific monitoring
		cfg := monitor.Config{
			WatchAddress: sub.Address,
			PollInterval: 3 * time.Second,
			StartBlock:   sub.CurrentBlock,
		}

		addrMonitor, err := monitor.NewAddressMonitor(
			m.tronClient,
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

	// Start monitor goroutine
	go func() {
		if err := monitorInstance.Start(); err != nil {
			m.logger.Error().
				Err(err).
				Str("subscriptionId", sub.SubscriptionID).
				Msg("Monitor error")
		}
	}()

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
				if currentBlock > wrapper.Subscription.CurrentBlock {
					m.db.SubscriptionRepo.UpdateCurrentBlock(m.ctx, wrapper.Subscription.SubscriptionID, currentBlock)
					wrapper.Subscription.CurrentBlock = currentBlock
					m.logger.Debug().
						Str("subscriptionId", wrapper.Subscription.SubscriptionID).
						Int64("currentBlock", currentBlock).
						Msg("Updated current block")
				}
			}

		case event := <-wrapper.EventChan:
			// Apply filters
			if !m.matchesFilters(event, wrapper.Subscription.Filters) {
				continue
			}

			// Route event to clients
			if err := m.eventRouter.RouteEvent(wrapper.Subscription, event); err != nil {
				m.logger.Error().
					Err(err).
					Str("subscriptionId", wrapper.Subscription.SubscriptionID).
					Msg("Failed to route event")
			}

			// Update subscription stats
			m.db.SubscriptionRepo.IncrementEventsCount(m.ctx, wrapper.Subscription.SubscriptionID)

			// Update current block
			if event.BlockNumber > wrapper.Subscription.CurrentBlock {
				m.db.SubscriptionRepo.UpdateCurrentBlock(m.ctx, wrapper.Subscription.SubscriptionID, event.BlockNumber)
				wrapper.Subscription.CurrentBlock = event.BlockNumber
			}

			m.logger.Debug().
				Str("subscriptionId", wrapper.Subscription.SubscriptionID).
				Str("txHash", event.TransactionID).
				Int64("block", event.BlockNumber).
				Msg("Event processed")
		}
	}
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
