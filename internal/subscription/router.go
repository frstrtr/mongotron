package subscription

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/frstrtr/mongotron/internal/blockchain/monitor"
	"github.com/frstrtr/mongotron/internal/blockchain/parser"
	"github.com/frstrtr/mongotron/internal/storage"
	"github.com/frstrtr/mongotron/internal/storage/models"
	"github.com/frstrtr/mongotron/internal/webhook"
	"github.com/frstrtr/mongotron/pkg/logger"
)

// EventRouter routes events to clients (WebSocket and webhook)
type EventRouter struct {
	db            *storage.Database
	logger        *logger.Logger
	wsClients     map[string][]*WebSocketClient // key: subscription_id
	eventQueue    chan *RouteEventRequest
	webhookClient *http.Client
	portoClient   *webhook.PortoAPIClient
	trc20Parser   *parser.TRC20Parser
	network       string // "tron-mainnet" or "tron-nile"
	mu            sync.RWMutex
}

// RouteEventRequest contains event routing information
type RouteEventRequest struct {
	Subscription *models.Subscription
	Event        *monitor.AddressEvent
}

// WebSocketClient represents a connected WebSocket client
type WebSocketClient struct {
	ID       string
	SendChan chan []byte
	mu       sync.RWMutex
	closed   bool // Track if channel has been closed
}

// NewEventRouter creates a new event router
func NewEventRouter(db *storage.Database, log *logger.Logger) *EventRouter {
	return &EventRouter{
		db:          db,
		logger:      log,
		wsClients:   make(map[string][]*WebSocketClient),
		eventQueue:  make(chan *RouteEventRequest, 1000),
		trc20Parser: parser.NewTRC20Parser(),
		network:     "tron-nile", // Default to testnet
		webhookClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SetPortoClient sets the Porto API client for webhook notifications
func (r *EventRouter) SetPortoClient(client *webhook.PortoAPIClient) {
	r.portoClient = client
}

// SetNetwork sets the network name (tron-mainnet or tron-nile)
func (r *EventRouter) SetNetwork(network string) {
	r.network = network
}

// Run starts the event router
func (r *EventRouter) Run(ctx context.Context) {
	r.logger.Info().Msg("Event router started")

	for {
		select {
		case <-ctx.Done():
			r.logger.Info().Msg("Event router stopped")
			return

		case req := <-r.eventQueue:
			r.routeEvent(req)
		}
	}
}

// RouteEvent queues an event for routing
func (r *EventRouter) RouteEvent(sub *models.Subscription, event *monitor.AddressEvent) error {
	req := &RouteEventRequest{
		Subscription: sub,
		Event:        event,
	}

	select {
	case r.eventQueue <- req:
		return nil
	default:
		return fmt.Errorf("event queue full")
	}
}

// routeEvent routes an event to all registered destinations
func (r *EventRouter) routeEvent(req *RouteEventRequest) {
	// Convert event to JSON
	eventData, err := json.Marshal(req.Event)
	if err != nil {
		r.logger.Error().
			Err(err).
			Str("subscriptionId", req.Subscription.SubscriptionID).
			Msg("Failed to marshal event")
		return
	}

	// Route to WebSocket clients
	r.sendToWebSocketClients(req.Subscription.SubscriptionID, eventData)

	// Route to webhook if configured
	if req.Subscription.WebhookURL != "" {
		go r.sendToWebhook(req.Subscription, eventData)
	}

	// Check for TRC20 transfers and send to Porto API
	if r.portoClient != nil && req.Event.ContractType == "TriggerSmartContract" {
		go r.handleTRC20Transfer(req)
	}

	// Store in database (events collection)
	go r.storeEvent(req)
}

// handleTRC20Transfer checks if the event is a TRC20 transfer and sends to Porto API
func (r *EventRouter) handleTRC20Transfer(req *RouteEventRequest) {
	// Get smart contract data from EventData
	scData, ok := req.Event.EventData["smartContract"].(map[string]interface{})
	if !ok {
		return
	}

	// Check if this is a transfer method
	methodSig, _ := scData["methodSignature"].(string)
	if methodSig != "a9059cbb" && methodSig != "23b872dd" {
		return // Not a transfer
	}

	// Parse the transfer details
	transfer := &parser.TRC20Transfer{
		ContractAddress: req.Event.To, // Contract being called
		TxHash:          req.Event.TransactionID,
		BlockNumber:     req.Event.BlockNumber,
		BlockTimestamp:  req.Event.BlockTimestamp,
		Success:         req.Event.Success,
	}

	// Get token info
	transfer.TokenSymbol, transfer.TokenDecimals = r.getTokenInfo(req.Event.To)

	// Extract parameters
	if params, ok := scData["parameters"].(map[string]interface{}); ok {
		if to, ok := params["to"].(string); ok {
			transfer.ToHex = to
			transfer.To = parser.HexToBase58(to)
		}
		if from, ok := params["from"].(string); ok {
			transfer.FromHex = from
			transfer.From = parser.HexToBase58(from)
		}
		if amountStr, ok := params["amount"].(string); ok {
			transfer.AmountDecimal = r.formatAmount(amountStr, transfer.TokenDecimals)
		}
	}

	// For transfer() method, From is the transaction sender
	if methodSig == "a9059cbb" {
		transfer.From = parser.HexToBase58(req.Event.From)
		transfer.FromHex = req.Event.From
		transfer.MethodType = "transfer"
	} else {
		transfer.MethodType = "transferFrom"
	}

	// Create Porto event
	portoEvent := webhook.CreateTransferEvent(
		transfer,
		req.Subscription.Address, // The watched NPS wallet
		req.Subscription.SubscriptionID,
		r.network,
	)

	// Send to Porto API
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := r.portoClient.SendTransferNotification(ctx, portoEvent); err != nil {
		r.logger.Error().
			Err(err).
			Str("txHash", transfer.TxHash).
			Str("to", transfer.To).
			Msg("Failed to send transfer notification to Porto API")
	} else {
		r.logger.Info().
			Str("txHash", transfer.TxHash).
			Str("token", transfer.TokenSymbol).
			Str("to", transfer.To).
			Str("amount", transfer.AmountDecimal).
			Msg("TRC20 transfer detected and sent to Porto API")
	}
}

// getTokenInfo returns token symbol and decimals for known contracts
func (r *EventRouter) getTokenInfo(contractAddress string) (string, int) {
	if r.trc20Parser.IsUSDTContract(contractAddress) {
		return "USDT", 6
	}
	return "TRC20", 18 // Default
}

// formatAmount formats raw amount string with decimals
func (r *EventRouter) formatAmount(amountStr string, decimals int) string {
	// Simple formatting - for production use big.Int
	if amountStr == "" {
		return "0"
	}
	// Add decimal point
	if len(amountStr) <= decimals {
		return "0." + fmt.Sprintf("%0*s", decimals, amountStr)
	}
	pos := len(amountStr) - decimals
	return amountStr[:pos] + "." + amountStr[pos:]
}

// sendToWebSocketClients sends event to all WebSocket clients subscribed to this subscription
func (r *EventRouter) sendToWebSocketClients(subscriptionID string, eventData []byte) {
	r.mu.RLock()
	clients := r.wsClients[subscriptionID]
	r.mu.RUnlock()

	if len(clients) == 0 {
		return
	}

	r.logger.Debug().
		Str("subscriptionId", subscriptionID).
		Int("clientCount", len(clients)).
		Msg("Sending event to WebSocket clients")

	for _, client := range clients {
		select {
		case client.SendChan <- eventData:
			// Successfully queued
		default:
			// Client's send buffer is full, skip
			r.logger.Warn().
				Str("clientId", client.ID).
				Str("subscriptionId", subscriptionID).
				Msg("Client send buffer full, dropping event")
		}
	}
}

// sendToWebhook sends event to webhook URL
func (r *EventRouter) sendToWebhook(sub *models.Subscription, eventData []byte) {
	maxRetries := 3
	retryDelay := time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest("POST", sub.WebhookURL, bytes.NewReader(eventData))
		if err != nil {
			r.logger.Error().
				Err(err).
				Str("subscriptionId", sub.SubscriptionID).
				Msg("Failed to create webhook request")
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Subscription-ID", sub.SubscriptionID)
		req.Header.Set("X-MongoTron-Event", "address.transaction")

		resp, err := r.webhookClient.Do(req)
		if err != nil {
			r.logger.Warn().
				Err(err).
				Str("subscriptionId", sub.SubscriptionID).
				Int("attempt", attempt).
				Msg("Webhook delivery failed")

			if attempt < maxRetries {
				time.Sleep(retryDelay)
				retryDelay *= 2 // Exponential backoff
				continue
			}
			return
		}

		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			r.logger.Debug().
				Str("subscriptionId", sub.SubscriptionID).
				Int("statusCode", resp.StatusCode).
				Msg("Webhook delivered successfully")
			return
		}

		r.logger.Warn().
			Str("subscriptionId", sub.SubscriptionID).
			Int("statusCode", resp.StatusCode).
			Int("attempt", attempt).
			Msg("Webhook returned non-2xx status")

		if attempt < maxRetries {
			time.Sleep(retryDelay)
			retryDelay *= 2
		}
	}
}

// storeEvent stores event in database
func (r *EventRouter) storeEvent(req *RouteEventRequest) {
	event := &models.Event{
		EventID:        fmt.Sprintf("evt_%s_%d", req.Event.TransactionID[:16], time.Now().UnixNano()),
		Network:        "tron-nile", // TODO: Make configurable
		Type:           req.Event.ContractType,
		Address:        req.Subscription.Address,
		TxHash:         req.Event.TransactionID,
		BlockNumber:    req.Event.BlockNumber,
		BlockTimestamp: req.Event.BlockTimestamp,
		Data: map[string]interface{}{
			"from":      req.Event.From,
			"to":        req.Event.To,
			"amount":    req.Event.Amount,
			"asset":     req.Event.AssetName,
			"success":   req.Event.Success,
			"eventType": req.Event.EventType,
			"eventData": req.Event.EventData,
		},
		SubscriptionID: req.Subscription.SubscriptionID,
		Processed:      false,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := r.db.EventRepo.Create(ctx, event); err != nil {
		r.logger.Error().
			Err(err).
			Str("subscriptionId", req.Subscription.SubscriptionID).
			Str("txHash", req.Event.TransactionID).
			Msg("Failed to store event")
	}
}

// RegisterClient registers a WebSocket client for a subscription
func (r *EventRouter) RegisterClient(subscriptionID string, client *WebSocketClient) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.wsClients[subscriptionID] = append(r.wsClients[subscriptionID], client)

	r.logger.Info().
		Str("subscriptionId", subscriptionID).
		Str("clientId", client.ID).
		Int("totalClients", len(r.wsClients[subscriptionID])).
		Msg("WebSocket client registered")
}

// UnregisterClient unregisters a WebSocket client
func (r *EventRouter) UnregisterClient(subscriptionID string, clientID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	clients := r.wsClients[subscriptionID]
	for i, client := range clients {
		if client.ID == clientID {
			// Remove client from slice
			r.wsClients[subscriptionID] = append(clients[:i], clients[i+1:]...)

			// Safely close the channel only if not already closed
			client.mu.Lock()
			if !client.closed {
				close(client.SendChan)
				client.closed = true
			}
			client.mu.Unlock()

			r.logger.Info().
				Str("subscriptionId", subscriptionID).
				Str("clientId", clientID).
				Int("remainingClients", len(r.wsClients[subscriptionID])).
				Msg("WebSocket client unregistered")

			// Clean up empty subscription entries
			if len(r.wsClients[subscriptionID]) == 0 {
				delete(r.wsClients, subscriptionID)
			}
			break
		}
	}
}

// GetClientCount returns the number of connected clients for a subscription
func (r *EventRouter) GetClientCount(subscriptionID string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.wsClients[subscriptionID])
}

// GetTotalClientCount returns the total number of connected clients
func (r *EventRouter) GetTotalClientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	total := 0
	for _, clients := range r.wsClients {
		total += len(clients)
	}
	return total
}
