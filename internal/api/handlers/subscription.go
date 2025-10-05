package handlers

import (
	"github.com/frstrtr/mongotron/internal/storage/models"
	"github.com/frstrtr/mongotron/internal/subscription"
	"github.com/gofiber/fiber/v2"
)

// SubscriptionHandler handles subscription-related HTTP requests
type SubscriptionHandler struct {
	manager subscription.ManagerInterface
}

// NewSubscriptionHandler creates a new subscription handler
func NewSubscriptionHandler(manager subscription.ManagerInterface) *SubscriptionHandler {
	return &SubscriptionHandler{
		manager: manager,
	}
}

// CreateSubscriptionRequest represents a subscription creation request
type CreateSubscriptionRequest struct {
	Address    string                     `json:"address" validate:"required"`
	WebhookURL string                     `json:"webhookUrl,omitempty"`
	Filters    models.SubscriptionFilters `json:"filters"`
	StartBlock int64                      `json:"startBlock,omitempty"`
}

// SubscriptionResponse represents a subscription in API responses
type SubscriptionResponse struct {
	SubscriptionID string                     `json:"subscriptionId"`
	Address        string                     `json:"address"`
	Network        string                     `json:"network"`
	WebhookURL     string                     `json:"webhookUrl,omitempty"`
	Filters        models.SubscriptionFilters `json:"filters"`
	Status         string                     `json:"status"`
	EventsCount    int64                      `json:"eventsCount"`
	LastEventAt    *string                    `json:"lastEventAt,omitempty"`
	StartBlock     int64                      `json:"startBlock"`
	CurrentBlock   int64                      `json:"currentBlock"`
	CreatedAt      string                     `json:"createdAt"`
	UpdatedAt      string                     `json:"updatedAt"`
}

// ListSubscriptionsResponse represents paginated subscription list
type ListSubscriptionsResponse struct {
	Subscriptions []*SubscriptionResponse `json:"subscriptions"`
	Total         int64                   `json:"total"`
	Limit         int64                   `json:"limit"`
	Skip          int64                   `json:"skip"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// CreateSubscription handles POST /api/v1/subscriptions
func (h *SubscriptionHandler) CreateSubscription(c *fiber.Ctx) error {
	var req CreateSubscriptionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_request",
			Message: "Failed to parse request body",
		})
	}

	// Validate address
	if req.Address == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_address",
			Message: "Address is required",
		})
	}

	// Use current block if startBlock not specified
	if req.StartBlock == 0 {
		req.StartBlock = -1 // Will use latest block
	}

	// Create subscription
	sub, err := h.manager.Subscribe(req.Address, req.WebhookURL, req.Filters, req.StartBlock)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "subscription_failed",
			Message: err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(toSubscriptionResponse(sub))
}

// GetSubscription handles GET /api/v1/subscriptions/:id
func (h *SubscriptionHandler) GetSubscription(c *fiber.Ctx) error {
	subscriptionID := c.Params("id")
	if subscriptionID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_request",
			Message: "Subscription ID is required",
		})
	}

	sub, err := h.manager.GetSubscription(subscriptionID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "subscription_not_found",
			Message: "Subscription not found",
		})
	}

	return c.JSON(toSubscriptionResponse(sub))
}

// ListSubscriptions handles GET /api/v1/subscriptions
func (h *SubscriptionHandler) ListSubscriptions(c *fiber.Ctx) error {
	// Parse pagination parameters
	limit := c.QueryInt("limit", 20)
	skip := c.QueryInt("skip", 0)

	if limit < 1 || limit > 100 {
		limit = 20
	}

	subs, total, err := h.manager.ListSubscriptions(int64(limit), int64(skip))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "list_failed",
			Message: err.Error(),
		})
	}

	responses := make([]*SubscriptionResponse, len(subs))
	for i, sub := range subs {
		responses[i] = toSubscriptionResponse(sub)
	}

	return c.JSON(ListSubscriptionsResponse{
		Subscriptions: responses,
		Total:         total,
		Limit:         int64(limit),
		Skip:          int64(skip),
	})
}

// DeleteSubscription handles DELETE /api/v1/subscriptions/:id
func (h *SubscriptionHandler) DeleteSubscription(c *fiber.Ctx) error {
	subscriptionID := c.Params("id")
	if subscriptionID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_request",
			Message: "Subscription ID is required",
		})
	}

	if err := h.manager.Unsubscribe(subscriptionID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "unsubscribe_failed",
			Message: err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Subscription stopped successfully",
	})
}

// toSubscriptionResponse converts a subscription model to response format
func toSubscriptionResponse(sub *models.Subscription) *SubscriptionResponse {
	resp := &SubscriptionResponse{
		SubscriptionID: sub.SubscriptionID,
		Address:        sub.Address,
		Network:        sub.Network,
		WebhookURL:     sub.WebhookURL,
		Filters:        sub.Filters,
		Status:         sub.Status,
		EventsCount:    sub.EventsCount,
		StartBlock:     sub.StartBlock,
		CurrentBlock:   sub.CurrentBlock,
		CreatedAt:      sub.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:      sub.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if sub.LastEventAt != nil {
		lastEventStr := sub.LastEventAt.Format("2006-01-02T15:04:05Z07:00")
		resp.LastEventAt = &lastEventStr
	}

	return resp
}
