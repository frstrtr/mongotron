package handlers

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestHealthCheck_Success(t *testing.T) {
	// Setup
	app := fiber.New()
	mockManager := new(MockSubscriptionManager)
	handler := NewHealthHandler(mockManager, "1.0.0-test")

	mockManager.On("GetActiveMonitorsCount").Return(5)

	app.Get("/health", handler.HealthCheck)

	// Execute request
	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Parse response
	body, _ := io.ReadAll(resp.Body)
	var response HealthResponse
	json.Unmarshal(body, &response)

	assert.Equal(t, "ok", response.Status)
	assert.Equal(t, "1.0.0-test", response.Version)
	assert.Equal(t, 5, response.ActiveMonitors)
	assert.Greater(t, response.Timestamp, int64(0))

	mockManager.AssertExpectations(t)
}

func TestReadinessCheck_Success(t *testing.T) {
	// Setup
	app := fiber.New()
	mockManager := new(MockSubscriptionManager)
	handler := NewHealthHandler(mockManager, "1.0.0-test")

	app.Get("/ready", handler.ReadinessCheck)

	// Execute request
	req := httptest.NewRequest("GET", "/ready", nil)
	resp, err := app.Test(req)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Parse response
	body, _ := io.ReadAll(resp.Body)
	var response map[string]interface{}
	json.Unmarshal(body, &response)

	assert.Equal(t, "ready", response["status"])
	assert.NotNil(t, response["timestamp"])
}

func TestLivenessCheck_Success(t *testing.T) {
	// Setup
	app := fiber.New()
	mockManager := new(MockSubscriptionManager)
	handler := NewHealthHandler(mockManager, "1.0.0-test")

	app.Get("/live", handler.LivenessCheck)

	// Execute request
	req := httptest.NewRequest("GET", "/live", nil)
	resp, err := app.Test(req)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Parse response
	body, _ := io.ReadAll(resp.Body)
	var response map[string]interface{}
	json.Unmarshal(body, &response)

	assert.Equal(t, "alive", response["status"])
	assert.NotNil(t, response["timestamp"])
}

func TestReadinessCheck_NotReady(t *testing.T) {
	// Setup
	app := fiber.New()
	handler := NewHealthHandler(nil, "1.0.0-test")

	app.Get("/ready", handler.ReadinessCheck)

	// Execute request
	req := httptest.NewRequest("GET", "/ready", nil)
	resp, err := app.Test(req)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusServiceUnavailable, resp.StatusCode)

	// Parse response
	body, _ := io.ReadAll(resp.Body)
	var response ErrorResponse
	json.Unmarshal(body, &response)

	assert.Equal(t, "not_ready", response.Error)
}
