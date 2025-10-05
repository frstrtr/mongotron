package handlers

import (
	"github.com/frstrtr/mongotron/internal/api/websocket"
	"github.com/frstrtr/mongotron/internal/subscription"
	wsfiber "github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
)

// WebSocketHandler handles WebSocket connection requests
type WebSocketHandler struct {
	hub     *websocket.Hub
	manager *subscription.Manager
}

// NewWebSocketHandler creates a new WebSocket handler
func NewWebSocketHandler(hub *websocket.Hub, manager *subscription.Manager) *WebSocketHandler {
	return &WebSocketHandler{
		hub:     hub,
		manager: manager,
	}
}

// StreamEvents handles WebSocket connection for event streaming
// Route: GET /api/v1/events/stream/:subscriptionId
func (h *WebSocketHandler) StreamEvents(c *wsfiber.Conn) {
	// Get subscription ID from path parameter
	subscriptionID := c.Params("subscriptionId")
	if subscriptionID == "" {
		c.WriteMessage(wsfiber.CloseMessage, []byte("Missing subscription ID"))
		return
	}

	// Verify subscription exists
	sub, err := h.manager.GetSubscription(subscriptionID)
	if err != nil {
		c.WriteMessage(wsfiber.CloseMessage, []byte("Subscription not found"))
		return
	}

	// Verify subscription is active
	if sub.Status != "active" {
		c.WriteMessage(wsfiber.CloseMessage, []byte("Subscription is not active"))
		return
	}

	// Handle WebSocket connection (blocking call)
	h.hub.HandleWebSocket(c, subscriptionID)
}

// Middleware to upgrade HTTP connection to WebSocket
func WebSocketMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// IsWebSocketUpgrade returns true if the client requested upgrade to the WebSocket protocol
		if wsfiber.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	}
}
