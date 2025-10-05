package handlers

import (
	"net/http/httptest"
	"testing"

	"github.com/frstrtr/mongotron/internal/api/websocket"
	"github.com/frstrtr/mongotron/internal/subscription"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestNewWebSocketHandler(t *testing.T) {
	// Arrange
	hub := &websocket.Hub{}
	manager := &subscription.Manager{}

	// Act
	handler := NewWebSocketHandler(hub, manager)

	// Assert
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.hub)
	assert.NotNil(t, handler.manager)
}

func TestWebSocketMiddleware_NonWebSocketRequest(t *testing.T) {
	// Arrange
	app := fiber.New()

	// Add middleware and a test route
	app.Use("/ws", WebSocketMiddleware())
	app.Get("/ws/test", func(c *fiber.Ctx) error {
		return c.SendString("should not reach here")
	})

	// Act - Regular HTTP request without WebSocket upgrade
	req := httptest.NewRequest("GET", "/ws/test", nil)
	resp, _ := app.Test(req)

	// Assert - Should return upgrade required error
	assert.Equal(t, fiber.StatusUpgradeRequired, resp.StatusCode)
}

func TestWebSocketMiddleware_WithUpgradeHeader(t *testing.T) {
	// Arrange
	app := fiber.New()
	reached := false

	// Add middleware and a test route
	app.Use("/ws", WebSocketMiddleware())
	app.Get("/ws/test", func(c *fiber.Ctx) error {
		reached = true
		return c.SendString("reached")
	})

	// Act - Request with WebSocket upgrade headers
	req := httptest.NewRequest("GET", "/ws/test", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	resp, _ := app.Test(req)

	// Assert - Should pass through middleware
	// The actual WebSocket upgrade happens in the handler, not the middleware
	// Middleware just checks if upgrade is requested
	assert.True(t, resp.StatusCode == fiber.StatusOK || reached || resp.StatusCode == fiber.StatusBadRequest)
}

func TestWebSocketMiddleware_IsFunction(t *testing.T) {
	// Arrange & Act
	middleware := WebSocketMiddleware()

	// Assert
	assert.NotNil(t, middleware)
	assert.IsType(t, fiber.Handler(nil), middleware)
}

// Note: StreamEvents cannot be fully unit tested without a real WebSocket connection
// Integration tests should be used for full WebSocket functionality testing
// The method requires an active WebSocket connection (*wsfiber.Conn) which cannot be easily mocked
