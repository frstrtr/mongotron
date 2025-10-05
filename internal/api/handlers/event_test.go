package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/frstrtr/mongotron/internal/storage/models"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockEventRepository is a mock implementation of EventRepositoryInterface
type MockEventRepository struct {
	mock.Mock
}

func (m *MockEventRepository) Create(ctx context.Context, event *models.Event) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func (m *MockEventRepository) FindByEventID(ctx context.Context, eventID string) (*models.Event, error) {
	args := m.Called(ctx, eventID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Event), args.Error(1)
}

func (m *MockEventRepository) FindByAddress(ctx context.Context, address string, limit, skip int64) ([]*models.Event, error) {
	args := m.Called(ctx, address, limit, skip)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Event), args.Error(1)
}

func (m *MockEventRepository) FindByTxHash(ctx context.Context, txHash string) ([]*models.Event, error) {
	args := m.Called(ctx, txHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Event), args.Error(1)
}

func (m *MockEventRepository) List(ctx context.Context, limit, skip int64) ([]*models.Event, error) {
	args := m.Called(ctx, limit, skip)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Event), args.Error(1)
}

func (m *MockEventRepository) Count(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockEventRepository) Delete(ctx context.Context, eventID string) error {
	args := m.Called(ctx, eventID)
	return args.Error(0)
}

func (m *MockEventRepository) UpdateProcessed(ctx context.Context, eventID string, processed bool) error {
	args := m.Called(ctx, eventID, processed)
	return args.Error(0)
}

func TestNewEventHandler(t *testing.T) {
	// Arrange
	mockEventRepo := new(MockEventRepository)

	// Act
	handler := NewEventHandler(mockEventRepo)

	// Assert
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.eventRepo)
}

func TestListEvents_Success(t *testing.T) {
	// Arrange
	mockEventRepo := new(MockEventRepository)
	handler := NewEventHandler(mockEventRepo)

	now := time.Now()
	mockEvents := []*models.Event{
		{
			EventID:        "event-1",
			SubscriptionID: "sub-1",
			Network:        "mainnet",
			Type:           "transfer",
			Address:        "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
			TxHash:         "abc123",
			BlockNumber:    1000,
			BlockTimestamp: now.Unix(),
			Data:           map[string]interface{}{"amount": "100"},
			Processed:      false,
			CreatedAt:      now,
		},
		{
			EventID:        "event-2",
			SubscriptionID: "sub-1",
			Network:        "mainnet",
			Type:           "approval",
			Address:        "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
			TxHash:         "def456",
			BlockNumber:    1001,
			BlockTimestamp: now.Unix(),
			Data:           map[string]interface{}{"spender": "address"},
			Processed:      true,
			CreatedAt:      now,
		},
	}

	mockEventRepo.On("List", mock.Anything, int64(50), int64(0)).Return(mockEvents, nil)
	mockEventRepo.On("Count", mock.Anything).Return(int64(2), nil)

	app := fiber.New()
	app.Get("/events", handler.ListEvents)

	// Act
	req := httptest.NewRequest("GET", "/events", nil)
	resp, _ := app.Test(req)

	// Assert
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var response ListEventsResponse
	json.Unmarshal(body, &response)

	assert.Equal(t, 2, len(response.Events))
	assert.Equal(t, int64(2), response.Total)
	assert.Equal(t, int64(50), response.Limit)
	assert.Equal(t, int64(0), response.Skip)
	assert.Equal(t, "event-1", response.Events[0].EventID)
	assert.Equal(t, "transfer", response.Events[0].Type)

	mockEventRepo.AssertExpectations(t)
}

func TestListEvents_WithPagination(t *testing.T) {
	// Arrange
	mockEventRepo := new(MockEventRepository)
	// removed
	handler := NewEventHandler(mockEventRepo)

	mockEvents := []*models.Event{}
	mockEventRepo.On("List", mock.Anything, int64(10), int64(20)).Return(mockEvents, nil)
	mockEventRepo.On("Count", mock.Anything).Return(int64(100), nil)

	app := fiber.New()
	app.Get("/events", handler.ListEvents)

	// Act
	req := httptest.NewRequest("GET", "/events?limit=10&skip=20", nil)
	resp, _ := app.Test(req)

	// Assert
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var response ListEventsResponse
	json.Unmarshal(body, &response)

	assert.Equal(t, int64(10), response.Limit)
	assert.Equal(t, int64(20), response.Skip)
	assert.Equal(t, int64(100), response.Total)

	mockEventRepo.AssertExpectations(t)
}

func TestListEvents_InvalidLimit(t *testing.T) {
	// Arrange
	mockEventRepo := new(MockEventRepository)
	// removed
	handler := NewEventHandler(mockEventRepo)

	mockEvents := []*models.Event{}
	// Should default to 50 when limit > 100
	mockEventRepo.On("List", mock.Anything, int64(50), int64(0)).Return(mockEvents, nil)
	mockEventRepo.On("Count", mock.Anything).Return(int64(0), nil)

	app := fiber.New()
	app.Get("/events", handler.ListEvents)

	// Act - limit > 100 should be capped
	req := httptest.NewRequest("GET", "/events?limit=200", nil)
	resp, _ := app.Test(req)

	// Assert
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var response ListEventsResponse
	json.Unmarshal(body, &response)

	assert.Equal(t, int64(50), response.Limit) // Should be capped to 50

	mockEventRepo.AssertExpectations(t)
}

func TestListEvents_ByAddress(t *testing.T) {
	// Arrange
	mockEventRepo := new(MockEventRepository)
	// removed
	handler := NewEventHandler(mockEventRepo)

	address := "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf"
	now := time.Now()
	mockEvents := []*models.Event{
		{
			EventID:        "event-1",
			SubscriptionID: "sub-1",
			Address:        address,
			Type:           "transfer",
			CreatedAt:      now,
		},
	}

	mockEventRepo.On("FindByAddress", mock.Anything, address, int64(50), int64(0)).Return(mockEvents, nil)
	mockEventRepo.On("Count", mock.Anything).Return(int64(1), nil)

	app := fiber.New()
	app.Get("/events", handler.ListEvents)

	// Act
	req := httptest.NewRequest("GET", "/events?address="+address, nil)
	resp, _ := app.Test(req)

	// Assert
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var response ListEventsResponse
	json.Unmarshal(body, &response)

	assert.Equal(t, 1, len(response.Events))
	assert.Equal(t, address, response.Events[0].Address)

	mockEventRepo.AssertExpectations(t)
}

func TestListEvents_DatabaseError(t *testing.T) {
	// Arrange
	mockEventRepo := new(MockEventRepository)
	// removed
	handler := NewEventHandler(mockEventRepo)

	mockEventRepo.On("List", mock.Anything, int64(50), int64(0)).Return(nil, fmt.Errorf("database error"))

	app := fiber.New()
	app.Get("/events", handler.ListEvents)

	// Act
	req := httptest.NewRequest("GET", "/events", nil)
	resp, _ := app.Test(req)

	// Assert
	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var response ErrorResponse
	json.Unmarshal(body, &response)

	assert.Equal(t, "list_failed", response.Error)
	assert.Contains(t, response.Message, "database error")

	mockEventRepo.AssertExpectations(t)
}

func TestListEvents_CountError(t *testing.T) {
	// Arrange
	mockEventRepo := new(MockEventRepository)
	// removed
	handler := NewEventHandler(mockEventRepo)

	mockEvents := []*models.Event{}
	mockEventRepo.On("List", mock.Anything, int64(50), int64(0)).Return(mockEvents, nil)
	mockEventRepo.On("Count", mock.Anything).Return(int64(0), fmt.Errorf("count error"))

	app := fiber.New()
	app.Get("/events", handler.ListEvents)

	// Act
	req := httptest.NewRequest("GET", "/events", nil)
	resp, _ := app.Test(req)

	// Assert
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var response ListEventsResponse
	json.Unmarshal(body, &response)

	// Count error should default to 0
	assert.Equal(t, int64(0), response.Total)

	mockEventRepo.AssertExpectations(t)
}

func TestGetEvent_Success(t *testing.T) {
	// Arrange
	mockEventRepo := new(MockEventRepository)
	// removed
	handler := NewEventHandler(mockEventRepo)

	now := time.Now()
	mockEvent := &models.Event{
		EventID:        "event-123",
		SubscriptionID: "sub-1",
		Network:        "mainnet",
		Type:           "transfer",
		Address:        "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
		TxHash:         "abc123",
		BlockNumber:    1000,
		BlockTimestamp: now.Unix(),
		Data:           map[string]interface{}{"amount": "100"},
		Processed:      false,
		CreatedAt:      now,
	}

	mockEventRepo.On("FindByEventID", mock.Anything, "event-123").Return(mockEvent, nil)

	app := fiber.New()
	app.Get("/events/:id", handler.GetEvent)

	// Act
	req := httptest.NewRequest("GET", "/events/event-123", nil)
	resp, _ := app.Test(req)

	// Assert
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var response EventResponse
	json.Unmarshal(body, &response)

	assert.Equal(t, "event-123", response.EventID)
	assert.Equal(t, "transfer", response.Type)
	assert.Equal(t, "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf", response.Address)

	mockEventRepo.AssertExpectations(t)
}

func TestGetEvent_NotFound(t *testing.T) {
	// Arrange
	mockEventRepo := new(MockEventRepository)
	// removed
	handler := NewEventHandler(mockEventRepo)

	mockEventRepo.On("FindByEventID", mock.Anything, "nonexistent").Return(nil, fmt.Errorf("not found"))

	app := fiber.New()
	app.Get("/events/:id", handler.GetEvent)

	// Act
	req := httptest.NewRequest("GET", "/events/nonexistent", nil)
	resp, _ := app.Test(req)

	// Assert
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var response ErrorResponse
	json.Unmarshal(body, &response)

	assert.Equal(t, "event_not_found", response.Error)

	mockEventRepo.AssertExpectations(t)
}

func TestGetEvent_MissingID(t *testing.T) {
	// Arrange
	mockEventRepo := new(MockEventRepository)
	// removed
	handler := NewEventHandler(mockEventRepo)

	app := fiber.New()
	app.Get("/events/:id", handler.GetEvent)

	// Act
	req := httptest.NewRequest("GET", "/events/", nil)
	resp, _ := app.Test(req)

	// Assert
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode) // Fiber returns 404 for missing params
}

func TestGetEventByTransactionHash_Success(t *testing.T) {
	// Arrange
	mockEventRepo := new(MockEventRepository)
	// removed
	handler := NewEventHandler(mockEventRepo)

	now := time.Now()
	txHash := "abc123def456"
	mockEvents := []*models.Event{
		{
			EventID:        "event-1",
			SubscriptionID: "sub-1",
			Network:        "mainnet",
			Type:           "transfer",
			Address:        "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
			TxHash:         txHash,
			BlockNumber:    1000,
			BlockTimestamp: now.Unix(),
			Data:           map[string]interface{}{"amount": "100"},
			Processed:      false,
			CreatedAt:      now,
		},
	}

	mockEventRepo.On("FindByTxHash", mock.Anything, txHash).Return(mockEvents, nil)

	app := fiber.New()
	app.Get("/events/tx/:hash", handler.GetEventByTransactionHash)

	// Act
	req := httptest.NewRequest("GET", "/events/tx/"+txHash, nil)
	resp, _ := app.Test(req)

	// Assert
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var response []*EventResponse
	json.Unmarshal(body, &response)

	assert.Equal(t, 1, len(response))
	assert.Equal(t, "event-1", response[0].EventID)
	assert.Equal(t, txHash, response[0].TxHash)

	mockEventRepo.AssertExpectations(t)
}

func TestGetEventByTransactionHash_MultipleEvents(t *testing.T) {
	// Arrange
	mockEventRepo := new(MockEventRepository)
	// removed
	handler := NewEventHandler(mockEventRepo)

	now := time.Now()
	txHash := "multi-event-hash"
	mockEvents := []*models.Event{
		{
			EventID:   "event-1",
			TxHash:    txHash,
			Type:      "transfer",
			CreatedAt: now,
		},
		{
			EventID:   "event-2",
			TxHash:    txHash,
			Type:      "approval",
			CreatedAt: now,
		},
	}

	mockEventRepo.On("FindByTxHash", mock.Anything, txHash).Return(mockEvents, nil)

	app := fiber.New()
	app.Get("/events/tx/:hash", handler.GetEventByTransactionHash)

	// Act
	req := httptest.NewRequest("GET", "/events/tx/"+txHash, nil)
	resp, _ := app.Test(req)

	// Assert
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var response []*EventResponse
	json.Unmarshal(body, &response)

	assert.Equal(t, 2, len(response))
	assert.Equal(t, "event-1", response[0].EventID)
	assert.Equal(t, "event-2", response[1].EventID)

	mockEventRepo.AssertExpectations(t)
}

func TestGetEventByTransactionHash_NotFound(t *testing.T) {
	// Arrange
	mockEventRepo := new(MockEventRepository)
	// removed
	handler := NewEventHandler(mockEventRepo)

	txHash := "nonexistent"
	mockEventRepo.On("FindByTxHash", mock.Anything, txHash).Return([]*models.Event{}, nil)

	app := fiber.New()
	app.Get("/events/tx/:hash", handler.GetEventByTransactionHash)

	// Act
	req := httptest.NewRequest("GET", "/events/tx/"+txHash, nil)
	resp, _ := app.Test(req)

	// Assert
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var response ErrorResponse
	json.Unmarshal(body, &response)

	assert.Equal(t, "event_not_found", response.Error)
	assert.Contains(t, response.Message, "No events found")

	mockEventRepo.AssertExpectations(t)
}

func TestGetEventByTransactionHash_DatabaseError(t *testing.T) {
	// Arrange
	mockEventRepo := new(MockEventRepository)
	// removed
	handler := NewEventHandler(mockEventRepo)

	txHash := "error-hash"
	mockEventRepo.On("FindByTxHash", mock.Anything, txHash).Return(nil, fmt.Errorf("database error"))

	app := fiber.New()
	app.Get("/events/tx/:hash", handler.GetEventByTransactionHash)

	// Act
	req := httptest.NewRequest("GET", "/events/tx/"+txHash, nil)
	resp, _ := app.Test(req)

	// Assert
	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var response ErrorResponse
	json.Unmarshal(body, &response)

	assert.Equal(t, "query_failed", response.Error)

	mockEventRepo.AssertExpectations(t)
}

func TestGetEventByTransactionHash_MissingHash(t *testing.T) {
	// Arrange
	mockEventRepo := new(MockEventRepository)
	// removed
	handler := NewEventHandler(mockEventRepo)

	app := fiber.New()
	app.Get("/events/tx/:hash", handler.GetEventByTransactionHash)

	// Act
	req := httptest.NewRequest("GET", "/events/tx/", nil)
	resp, _ := app.Test(req)

	// Assert
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode) // Fiber returns 404 for missing params
}

func TestToEventResponse(t *testing.T) {
	// Arrange
	now := time.Now()
	event := &models.Event{
		EventID:        "event-123",
		SubscriptionID: "sub-456",
		Network:        "mainnet",
		Type:           "transfer",
		Address:        "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
		TxHash:         "abc123",
		BlockNumber:    1000,
		BlockTimestamp: now.Unix(),
		Data:           map[string]interface{}{"amount": "100", "from": "addr1"},
		Processed:      true,
		CreatedAt:      now,
	}

	// Act
	response := toEventResponse(event)

	// Assert
	assert.NotNil(t, response)
	assert.Equal(t, "event-123", response.EventID)
	assert.Equal(t, "sub-456", response.SubscriptionID)
	assert.Equal(t, "mainnet", response.Network)
	assert.Equal(t, "transfer", response.Type)
	assert.Equal(t, "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf", response.Address)
	assert.Equal(t, "abc123", response.TxHash)
	assert.Equal(t, int64(1000), response.BlockNumber)
	assert.Equal(t, now.Unix(), response.BlockTimestamp)
	assert.Equal(t, true, response.Processed)
	assert.Equal(t, "100", response.Data["amount"])
	assert.Equal(t, "addr1", response.Data["from"])
	assert.NotEmpty(t, response.CreatedAt)
}
