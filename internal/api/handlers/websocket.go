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
		rejectStream(c, "Missing subscription ID")
		return
	}

	// Verify subscription exists
	sub, err := h.manager.GetSubscription(subscriptionID)
	if err != nil {
		rejectStream(c, "Subscription not found")
		return
	}

	// Verify subscription is active
	if sub.Status != "active" {
		rejectStream(c, "Subscription is not active")
		return
	}

	// Handle WebSocket connection (blocking call)
	h.hub.HandleWebSocket(c, subscriptionID)
}

// rejectStream tells the client why the stream is refused with a close frame
// (policy violation) before the handler returns and the connection is closed.
// It is best effort: the connection is closed either way, and the handler has
// no one to report a failed write to.
func rejectStream(c *wsfiber.Conn, reason string) {
	_ = c.WriteMessage(wsfiber.CloseMessage, wsfiber.FormatCloseMessage(wsfiber.ClosePolicyViolation, reason))
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
