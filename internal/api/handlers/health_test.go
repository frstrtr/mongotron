package handlers

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/frstrtr/mongotron/internal/blockchain/monitor"
	"github.com/frstrtr/mongotron/internal/subscription"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, json.Unmarshal(body, &response))

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
	require.NoError(t, json.Unmarshal(body, &response))

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
	require.NoError(t, json.Unmarshal(body, &response))

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
	require.NoError(t, json.Unmarshal(body, &response))

	assert.Equal(t, "not_ready", response.Error)
}

// statsManager is a manager that exposes monitoring counters.
type statsManager struct {
	MockSubscriptionManager
	stats subscription.MonitoringStats
}

func (s *statsManager) MonitoringStats() subscription.MonitoringStats { return s.stats }

func TestHealthCheck_ExposesScannerUsageAndGapCounters(t *testing.T) {
	app := fiber.New()
	m := &statsManager{stats: subscription.MonitoringStats{
		SharedScanner: true,
		Scanner:       &monitor.ScannerStats{BlocksScanned: 1200, BlockFetches: 1200, SkippedBlocks: 2, Subscribers: 10, Cursor: 68900000},
		Usage:         subscription.UsageStats{Queued: 5, Sent: 4, Failed: 1},
		Gaps:          subscription.GapStats{Recorded: 2, Pending: 1, PendingBlocks: []int64{68899990}, Recovered: 1},
	}}
	m.On("GetActiveMonitorsCount").Return(10)
	app.Get("/health", NewHealthHandler(m, "1.0.0-test").HealthCheck)

	resp, err := app.Test(httptest.NewRequest("GET", "/health", nil))
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &out), string(body))

	mon := out["monitoring"].(map[string]interface{})
	assert.Equal(t, true, mon["sharedScanner"])
	assert.Equal(t, float64(2), mon["scanner"].(map[string]interface{})["skippedBlocks"])
	assert.Equal(t, float64(1), mon["usage"].(map[string]interface{})["failed"])
	gaps := mon["gaps"].(map[string]interface{})
	assert.Equal(t, float64(1), gaps["pending"])
	assert.Equal(t, []interface{}{float64(68899990)}, gaps["pendingBlocks"])
}

func TestHealthCheck_WithoutMonitoringStatsKeepsTheOldShape(t *testing.T) {
	app := fiber.New()
	m := new(MockSubscriptionManager)
	m.On("GetActiveMonitorsCount").Return(1)
	app.Get("/health", NewHealthHandler(m, "1.0.0-test").HealthCheck)
	resp, err := app.Test(httptest.NewRequest("GET", "/health", nil))
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	assert.NotContains(t, string(body), "monitoring")
}
