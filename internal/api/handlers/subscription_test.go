package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/frstrtr/mongotron/internal/storage/models"
	"github.com/frstrtr/mongotron/internal/subscription"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockSubscriptionManager is a mock implementation of subscription.Manager
type MockSubscriptionManager struct {
	mock.Mock
}

func (m *MockSubscriptionManager) Subscribe(address string, webhookURL string, filters models.SubscriptionFilters, startBlock int64) (*models.Subscription, error) {
	args := m.Called(address, webhookURL, filters, startBlock)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Subscription), args.Error(1)
}

func (m *MockSubscriptionManager) Unsubscribe(subscriptionID string) error {
	args := m.Called(subscriptionID)
	return args.Error(0)
}

func (m *MockSubscriptionManager) Resubscribe(address string, webhookURL string, filters models.SubscriptionFilters, scanGap bool) (*subscription.ResubscribeResult, error) {
	args := m.Called(address, webhookURL, filters, scanGap)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*subscription.ResubscribeResult), args.Error(1)
}

func (m *MockSubscriptionManager) GetSubscription(subscriptionID string) (*models.Subscription, error) {
	args := m.Called(subscriptionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Subscription), args.Error(1)
}

func (m *MockSubscriptionManager) ListSubscriptions(limit, skip int64) ([]*models.Subscription, int64, error) {
	args := m.Called(limit, skip)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*models.Subscription), args.Get(1).(int64), args.Error(2)
}

func (m *MockSubscriptionManager) List(limit, skip int64) ([]*models.Subscription, int64, error) {
	args := m.Called(limit, skip)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*models.Subscription), args.Get(1).(int64), args.Error(2)
}

func (m *MockSubscriptionManager) GetByAddress(address string) (*models.Subscription, error) {
	args := m.Called(address)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Subscription), args.Error(1)
}

func (m *MockSubscriptionManager) ScanHistorical(subscriptionID string, fromBlock, toBlock int64) error {
	args := m.Called(subscriptionID, fromBlock, toBlock)
	return args.Error(0)
}

func (m *MockSubscriptionManager) GetActiveMonitorsCount() int {
	args := m.Called()
	return args.Int(0)
}

func (m *MockSubscriptionManager) Start() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockSubscriptionManager) Stop() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockSubscriptionManager) GetEventRouter() *subscription.EventRouter {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*subscription.EventRouter)
}

func TestCreateSubscription_Success(t *testing.T) {
	// Setup
	app := fiber.New()
	mockManager := new(MockSubscriptionManager)
	handler := NewSubscriptionHandler(mockManager)

	// Mock data
	now := time.Now()
	expectedSub := &models.Subscription{
		SubscriptionID: "sub_test123",
		Address:        "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
		Network:        "tron-nile",
		Status:         "active",
		EventsCount:    0,
		StartBlock:     0,
		CurrentBlock:   100,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	mockManager.On("Subscribe", "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf", "", mock.AnythingOfType("models.SubscriptionFilters"), int64(-1)).
		Return(expectedSub, nil)

	app.Post("/subscriptions", handler.CreateSubscription)

	// Prepare request
	reqBody := CreateSubscriptionRequest{
		Address:    "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
		WebhookURL: "",
		Filters:    models.SubscriptionFilters{},
		StartBlock: 0,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	// Execute request
	req := httptest.NewRequest("POST", "/subscriptions", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	// Parse response
	body, _ := io.ReadAll(resp.Body)
	var response SubscriptionResponse
	json.Unmarshal(body, &response)

	assert.Equal(t, "sub_test123", response.SubscriptionID)
	assert.Equal(t, "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf", response.Address)
	assert.Equal(t, "active", response.Status)

	mockManager.AssertExpectations(t)
}

func TestCreateSubscription_MissingAddress(t *testing.T) {
	// Setup
	app := fiber.New()
	mockManager := new(MockSubscriptionManager)
	handler := NewSubscriptionHandler(mockManager)

	app.Post("/subscriptions", handler.CreateSubscription)

	// Prepare request with missing address
	reqBody := CreateSubscriptionRequest{
		Address: "",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	// Execute request
	req := httptest.NewRequest("POST", "/subscriptions", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	// Parse response
	body, _ := io.ReadAll(resp.Body)
	var response ErrorResponse
	json.Unmarshal(body, &response)

	assert.Equal(t, "invalid_address", response.Error)
}

func TestGetSubscription_Success(t *testing.T) {
	// Setup
	app := fiber.New()
	mockManager := new(MockSubscriptionManager)
	handler := NewSubscriptionHandler(mockManager)

	// Mock data
	now := time.Now()
	expectedSub := &models.Subscription{
		SubscriptionID: "sub_test123",
		Address:        "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
		Network:        "tron-nile",
		Status:         "active",
		EventsCount:    5,
		StartBlock:     0,
		CurrentBlock:   100,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	mockManager.On("GetSubscription", "sub_test123").Return(expectedSub, nil)

	app.Get("/subscriptions/:id", handler.GetSubscription)

	// Execute request
	req := httptest.NewRequest("GET", "/subscriptions/sub_test123", nil)
	resp, err := app.Test(req)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Parse response
	body, _ := io.ReadAll(resp.Body)
	var response SubscriptionResponse
	json.Unmarshal(body, &response)

	assert.Equal(t, "sub_test123", response.SubscriptionID)
	assert.Equal(t, int64(5), response.EventsCount)

	mockManager.AssertExpectations(t)
}

func TestGetSubscription_NotFound(t *testing.T) {
	// Setup
	app := fiber.New()
	mockManager := new(MockSubscriptionManager)
	handler := NewSubscriptionHandler(mockManager)

	mockManager.On("GetSubscription", "sub_notfound").Return(nil, assert.AnError)

	app.Get("/subscriptions/:id", handler.GetSubscription)

	// Execute request
	req := httptest.NewRequest("GET", "/subscriptions/sub_notfound", nil)
	resp, err := app.Test(req)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)

	mockManager.AssertExpectations(t)
}

func TestListSubscriptions_Success(t *testing.T) {
	// Setup
	app := fiber.New()
	mockManager := new(MockSubscriptionManager)
	handler := NewSubscriptionHandler(mockManager)

	// Mock data
	now := time.Now()
	subscriptions := []*models.Subscription{
		{
			SubscriptionID: "sub_test1",
			Address:        "TAddress1",
			Status:         "active",
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		{
			SubscriptionID: "sub_test2",
			Address:        "TAddress2",
			Status:         "active",
			CreatedAt:      now,
			UpdatedAt:      now,
		},
	}

	mockManager.On("ListSubscriptions", int64(20), int64(0)).Return(subscriptions, int64(2), nil)

	app.Get("/subscriptions", handler.ListSubscriptions)

	// Execute request
	req := httptest.NewRequest("GET", "/subscriptions", nil)
	resp, err := app.Test(req)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Parse response
	body, _ := io.ReadAll(resp.Body)
	var response ListSubscriptionsResponse
	json.Unmarshal(body, &response)

	assert.Equal(t, int64(2), response.Total)
	assert.Len(t, response.Subscriptions, 2)

	mockManager.AssertExpectations(t)
}

func TestListSubscriptions_WithPagination(t *testing.T) {
	// Setup
	app := fiber.New()
	mockManager := new(MockSubscriptionManager)
	handler := NewSubscriptionHandler(mockManager)

	subscriptions := []*models.Subscription{}
	mockManager.On("ListSubscriptions", int64(10), int64(20)).Return(subscriptions, int64(50), nil)

	app.Get("/subscriptions", handler.ListSubscriptions)

	// Execute request with pagination
	req := httptest.NewRequest("GET", "/subscriptions?limit=10&skip=20", nil)
	resp, err := app.Test(req)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Parse response
	body, _ := io.ReadAll(resp.Body)
	var response ListSubscriptionsResponse
	json.Unmarshal(body, &response)

	assert.Equal(t, int64(50), response.Total)
	assert.Equal(t, int64(10), response.Limit)
	assert.Equal(t, int64(20), response.Skip)

	mockManager.AssertExpectations(t)
}

func TestDeleteSubscription_Success(t *testing.T) {
	// Setup
	app := fiber.New()
	mockManager := new(MockSubscriptionManager)
	handler := NewSubscriptionHandler(mockManager)

	mockManager.On("Unsubscribe", "sub_test123").Return(nil)

	app.Delete("/subscriptions/:id", handler.DeleteSubscription)

	// Execute request
	req := httptest.NewRequest("DELETE", "/subscriptions/sub_test123", nil)
	resp, err := app.Test(req)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	// Parse response
	body, _ := io.ReadAll(resp.Body)
	var response map[string]interface{}
	json.Unmarshal(body, &response)

	assert.Equal(t, true, response["success"])

	mockManager.AssertExpectations(t)
}

func TestDeleteSubscription_Error(t *testing.T) {
	// Setup
	app := fiber.New()
	mockManager := new(MockSubscriptionManager)
	handler := NewSubscriptionHandler(mockManager)

	mockManager.On("Unsubscribe", "sub_test123").Return(assert.AnError)

	app.Delete("/subscriptions/:id", handler.DeleteSubscription)

	// Execute request
	req := httptest.NewRequest("DELETE", "/subscriptions/sub_test123", nil)
	resp, err := app.Test(req)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)

	mockManager.AssertExpectations(t)
}

func TestCreateSubscription_WithFilters(t *testing.T) {
	// Setup
	app := fiber.New()
	mockManager := new(MockSubscriptionManager)
	handler := NewSubscriptionHandler(mockManager)

	// Mock data with filters
	now := time.Now()
	filters := models.SubscriptionFilters{
		ContractTypes: []string{"TransferContract", "TriggerSmartContract"},
		MinAmount:     1000000,
		MaxAmount:     10000000,
		OnlySuccess:   true,
	}

	expectedSub := &models.Subscription{
		SubscriptionID: "sub_test123",
		Address:        "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
		Network:        "tron-nile",
		Filters:        filters,
		Status:         "active",
		EventsCount:    0,
		StartBlock:     0,
		CurrentBlock:   100,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	mockManager.On("Subscribe", "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf", "https://webhook.example.com", filters, int64(1000)).
		Return(expectedSub, nil)

	app.Post("/subscriptions", handler.CreateSubscription)

	// Prepare request with filters
	reqBody := CreateSubscriptionRequest{
		Address:    "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
		WebhookURL: "https://webhook.example.com",
		Filters:    filters,
		StartBlock: 1000,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	// Execute request
	req := httptest.NewRequest("POST", "/subscriptions", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	// Parse response
	body, _ := io.ReadAll(resp.Body)
	var response SubscriptionResponse
	json.Unmarshal(body, &response)

	assert.Equal(t, "sub_test123", response.SubscriptionID)
	assert.Len(t, response.Filters.ContractTypes, 2)
	assert.Equal(t, int64(1000000), response.Filters.MinAmount)
	assert.True(t, response.Filters.OnlySuccess)

	mockManager.AssertExpectations(t)
}

func TestToSubscriptionResponse_WithLastEventAt(t *testing.T) {
	// Arrange
	now := time.Now()
	lastEvent := now.Add(-1 * time.Hour)
	sub := &models.Subscription{
		SubscriptionID: "sub-123",
		Address:        "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
		Network:        "mainnet",
		WebhookURL:     "https://example.com/webhook",
		Status:         "active",
		EventsCount:    10,
		StartBlock:     1000,
		CurrentBlock:   1100,
		CreatedAt:      now,
		UpdatedAt:      now,
		LastEventAt:    &lastEvent,
	}

	// Act
	response := toSubscriptionResponse(sub)

	// Assert
	assert.NotNil(t, response)
	assert.Equal(t, "sub-123", response.SubscriptionID)
	assert.Equal(t, "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf", response.Address)
	assert.NotNil(t, response.LastEventAt)
	assert.Contains(t, *response.LastEventAt, lastEvent.Format("2006-01-02"))
}

func TestToSubscriptionResponse_WithoutLastEventAt(t *testing.T) {
	// Arrange
	now := time.Now()
	sub := &models.Subscription{
		SubscriptionID: "sub-456",
		Address:        "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
		Network:        "mainnet",
		Status:         "active",
		EventsCount:    0,
		StartBlock:     1000,
		CurrentBlock:   1000,
		CreatedAt:      now,
		UpdatedAt:      now,
		LastEventAt:    nil,
	}

	// Act
	response := toSubscriptionResponse(sub)

	// Assert
	assert.NotNil(t, response)
	assert.Equal(t, "sub-456", response.SubscriptionID)
	assert.Nil(t, response.LastEventAt)
}

func TestCreateSubscription_DatabaseError(t *testing.T) {
	// Arrange
	mockManager := new(MockSubscriptionManager)
	// When StartBlock is 0, it defaults to -1 in the handler
	mockManager.On("Subscribe", "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf", "", mock.Anything, int64(-1)).
		Return(nil, fmt.Errorf("database connection failed"))

	handler := NewSubscriptionHandler(mockManager)

	app := fiber.New()
	app.Post("/subscriptions", handler.CreateSubscription)

	reqBody := CreateSubscriptionRequest{
		Address: "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	// Act
	req := httptest.NewRequest("POST", "/subscriptions", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	// Assert
	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var response ErrorResponse
	json.Unmarshal(body, &response)

	assert.Equal(t, "subscription_failed", response.Error)
	assert.Contains(t, response.Message, "database connection failed")

	mockManager.AssertExpectations(t)
}

func TestGetSubscription_InvalidID(t *testing.T) {
	// Arrange
	mockManager := new(MockSubscriptionManager)
	handler := NewSubscriptionHandler(mockManager)

	app := fiber.New()
	app.Get("/subscriptions/:id", handler.GetSubscription)

	// Act - empty ID
	req := httptest.NewRequest("GET", "/subscriptions/", nil)
	resp, _ := app.Test(req)

	// Assert - Fiber returns 404 for missing route param
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}

func TestListSubscriptions_ManagerError(t *testing.T) {
	// Arrange
	mockManager := new(MockSubscriptionManager)
	mockManager.On("ListSubscriptions", int64(20), int64(0)).
		Return(nil, int64(0), fmt.Errorf("manager error"))

	handler := NewSubscriptionHandler(mockManager)

	app := fiber.New()
	app.Get("/subscriptions", handler.ListSubscriptions)

	// Act
	req := httptest.NewRequest("GET", "/subscriptions", nil)
	resp, _ := app.Test(req)

	// Assert
	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var response ErrorResponse
	json.Unmarshal(body, &response)

	assert.Equal(t, "list_failed", response.Error)

	mockManager.AssertExpectations(t)
}
