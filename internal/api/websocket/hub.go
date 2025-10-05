package websocket

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/frstrtr/mongotron/internal/subscription"
	"github.com/frstrtr/mongotron/pkg/logger"
	"github.com/gofiber/contrib/websocket"
)

// Hub manages WebSocket clients and broadcasts events
type Hub struct {
	// Registered clients by subscription ID
	clients map[string]map[*Client]bool

	// Register requests from clients
	register chan *ClientRegistration

	// Unregister requests from clients
	unregister chan *Client

	// Event router for integration
	eventRouter *subscription.EventRouter

	// Logger
	logger *logger.Logger

	// Mutex for thread safety
	mu sync.RWMutex

	// Context for shutdown
	ctx    context.Context
	cancel context.CancelFunc
}

// ClientRegistration contains client registration information
type ClientRegistration struct {
	Client         *Client
	SubscriptionID string
}

// NewHub creates a new WebSocket hub
func NewHub(eventRouter *subscription.EventRouter, log *logger.Logger) *Hub {
	ctx, cancel := context.WithCancel(context.Background())

	return &Hub{
		clients:     make(map[string]map[*Client]bool),
		register:    make(chan *ClientRegistration),
		unregister:  make(chan *Client),
		eventRouter: eventRouter,
		logger:      log,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Run starts the hub's main event loop
func (h *Hub) Run() {
	h.logger.Info().Msg("WebSocket hub started")

	for {
		select {
		case <-h.ctx.Done():
			h.logger.Info().Msg("WebSocket hub stopped")
			return

		case registration := <-h.register:
			h.registerClient(registration)

		case client := <-h.unregister:
			h.unregisterClient(client)
		}
	}
}

// Stop stops the hub
func (h *Hub) Stop() {
	h.logger.Info().Msg("Stopping WebSocket hub")
	h.cancel()

	// Close all client connections
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, clients := range h.clients {
		for client := range clients {
			close(client.send)
		}
	}
}

// registerClient registers a new client
func (h *Hub) registerClient(reg *ClientRegistration) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Create subscription map if doesn't exist
	if h.clients[reg.SubscriptionID] == nil {
		h.clients[reg.SubscriptionID] = make(map[*Client]bool)
	}

	// Add client to subscription
	h.clients[reg.SubscriptionID][reg.Client] = true

	// Register with event router
	wsClient := &subscription.WebSocketClient{
		ID:       reg.Client.id,
		SendChan: reg.Client.send,
	}
	h.eventRouter.RegisterClient(reg.SubscriptionID, wsClient)

	h.logger.Info().
		Str("clientId", reg.Client.id).
		Str("subscriptionId", reg.SubscriptionID).
		Int("totalClients", len(h.clients[reg.SubscriptionID])).
		Msg("WebSocket client registered")
}

// unregisterClient unregisters a client
func (h *Hub) unregisterClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Find and remove client from all subscriptions
	for subscriptionID, clients := range h.clients {
		if _, ok := clients[client]; ok {
			delete(clients, client)
			close(client.send)

			// Unregister from event router
			h.eventRouter.UnregisterClient(subscriptionID, client.id)

			h.logger.Info().
				Str("clientId", client.id).
				Str("subscriptionId", subscriptionID).
				Int("remainingClients", len(clients)).
				Msg("WebSocket client unregistered")

			// Clean up empty subscription maps
			if len(clients) == 0 {
				delete(h.clients, subscriptionID)
			}

			break
		}
	}
}

// GetClientCount returns the number of clients for a subscription
func (h *Hub) GetClientCount(subscriptionID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if clients, ok := h.clients[subscriptionID]; ok {
		return len(clients)
	}
	return 0
}

// GetTotalClientCount returns the total number of connected clients
func (h *Hub) GetTotalClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	total := 0
	for _, clients := range h.clients {
		total += len(clients)
	}
	return total
}

// HandleWebSocket handles WebSocket connection upgrade and client lifecycle
func (h *Hub) HandleWebSocket(c *websocket.Conn, subscriptionID string) {
	// Create client
	client := NewClient(h, c, subscriptionID)

	// Register client
	registration := &ClientRegistration{
		Client:         client,
		SubscriptionID: subscriptionID,
	}
	h.register <- registration

	// Send welcome message
	welcome := map[string]interface{}{
		"type":           "connected",
		"subscriptionId": subscriptionID,
		"timestamp":      time.Now().Unix(),
		"message":        "Connected to MongoTron event stream",
	}
	welcomeData, _ := json.Marshal(welcome)
	client.send <- welcomeData

	// Start client goroutines
	go client.writePump()
	client.readPump() // Blocking call
}
