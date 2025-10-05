package handlers

import (
	"time"

	"github.com/frstrtr/mongotron/internal/subscription"
	"github.com/gofiber/fiber/v2"
)

// HealthHandler handles health check requests
type HealthHandler struct {
	manager subscription.ManagerInterface
	version string
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(manager subscription.ManagerInterface, version string) *HealthHandler {
	return &HealthHandler{
		manager: manager,
		version: version,
	}
}

// HealthResponse represents health check response
type HealthResponse struct {
	Status           string `json:"status"`
	Version          string `json:"version"`
	Timestamp        int64  `json:"timestamp"`
	ActiveMonitors   int    `json:"activeMonitors"`
	ConnectedClients int    `json:"connectedClients,omitempty"`
	Uptime           int64  `json:"uptime,omitempty"`
}

var startTime = time.Now()

// HealthCheck handles GET /api/v1/health
func (h *HealthHandler) HealthCheck(c *fiber.Ctx) error {
	uptime := int64(time.Since(startTime).Seconds())

	return c.JSON(HealthResponse{
		Status:         "ok",
		Version:        h.version,
		Timestamp:      time.Now().Unix(),
		ActiveMonitors: h.manager.GetActiveMonitorsCount(),
		Uptime:         uptime,
	})
}

// ReadinessCheck handles GET /api/v1/ready
func (h *HealthHandler) ReadinessCheck(c *fiber.Ctx) error {
	// Check if manager is ready
	if h.manager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
			Error:   "not_ready",
			Message: "Service is not ready",
		})
	}

	return c.JSON(fiber.Map{
		"status":    "ready",
		"timestamp": time.Now().Unix(),
	})
}

// LivenessCheck handles GET /api/v1/live
func (h *HealthHandler) LivenessCheck(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":    "alive",
		"timestamp": time.Now().Unix(),
	})
}
