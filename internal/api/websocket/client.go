package websocket

import (
	"time"

	"github.com/frstrtr/mongotron/pkg/logger"
	"github.com/gofiber/contrib/websocket"
	"github.com/google/uuid"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 512
)

// Client represents a WebSocket client connection
type Client struct {
	// Unique client ID
	id string

	// The hub
	hub *Hub

	// The websocket connection
	conn *websocket.Conn

	// Subscription ID this client is listening to
	subscriptionID string

	// Buffered channel of outbound messages
	send chan []byte

	// Logger
	logger *logger.Logger
}

// NewClient creates a new WebSocket client
func NewClient(hub *Hub, conn *websocket.Conn, subscriptionID string) *Client {
	return &Client{
		id:             uuid.New().String()[:8],
		hub:            hub,
		conn:           conn,
		subscriptionID: subscriptionID,
		send:           make(chan []byte, 256),
		logger:         hub.logger,
	}
}

// readPump pumps messages from the websocket connection to the hub
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		messageType, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.logger.Error().
					Err(err).
					Str("clientId", c.id).
					Msg("WebSocket read error")
			}
			break
		}

		// Log received message (for debugging/heartbeat)
		c.logger.Debug().
			Str("clientId", c.id).
			Int("messageType", messageType).
			Int("messageSize", len(message)).
			Msg("Received message from client")

		// We don't process client messages in MVP, but we could handle:
		// - Subscription filter updates
		// - Client heartbeat/keepalive
		// - Command messages (pause/resume)
	}
}

// writePump pumps messages from the hub to the websocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))

			if !ok {
				// The hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued messages to the current websocket message
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// GetID returns the client ID
func (c *Client) GetID() string {
	return c.id
}

// GetSubscriptionID returns the subscription ID
func (c *Client) GetSubscriptionID() string {
	return c.subscriptionID
}
