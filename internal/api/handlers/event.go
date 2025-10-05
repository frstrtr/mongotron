package handlers

import (
	"github.com/frstrtr/mongotron/internal/storage"
	"github.com/frstrtr/mongotron/internal/storage/models"
	"github.com/gofiber/fiber/v2"
)

// EventHandler handles event-related HTTP requests
type EventHandler struct {
	db *storage.Database
}

// NewEventHandler creates a new event handler
func NewEventHandler(db *storage.Database) *EventHandler {
	return &EventHandler{
		db: db,
	}
}

// EventResponse represents an event in API responses
type EventResponse struct {
	EventID        string                 `json:"eventId"`
	SubscriptionID string                 `json:"subscriptionId"`
	Network        string                 `json:"network"`
	Type           string                 `json:"type"`
	Address        string                 `json:"address"`
	TxHash         string                 `json:"txHash"`
	BlockNumber    int64                  `json:"blockNumber"`
	BlockTimestamp int64                  `json:"blockTimestamp"`
	Data           map[string]interface{} `json:"data"`
	Processed      bool                   `json:"processed"`
	CreatedAt      string                 `json:"createdAt"`
}

// ListEventsResponse represents paginated event list
type ListEventsResponse struct {
	Events []*EventResponse `json:"events"`
	Total  int64            `json:"total"`
	Limit  int64            `json:"limit"`
	Skip   int64            `json:"skip"`
}

// ListEvents handles GET /api/v1/events
func (h *EventHandler) ListEvents(c *fiber.Ctx) error {
	// Parse pagination parameters
	limit := c.QueryInt("limit", 50)
	skip := c.QueryInt("skip", 0)

	if limit < 1 || limit > 100 {
		limit = 50
	}

	// Parse filter parameters
	address := c.Query("address")

	var events []*models.Event
	var err error

	if address != "" {
		events, err = h.db.EventRepo.FindByAddress(c.Context(), address, int64(limit), int64(skip))
	} else {
		events, err = h.db.EventRepo.List(c.Context(), int64(limit), int64(skip))
	}

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "list_failed",
			Message: err.Error(),
		})
	}

	// Get total count
	total, err := h.db.EventRepo.Count(c.Context())
	if err != nil {
		total = 0
	}

	responses := make([]*EventResponse, len(events))
	for i, event := range events {
		responses[i] = toEventResponse(event)
	}

	return c.JSON(ListEventsResponse{
		Events: responses,
		Total:  total,
		Limit:  int64(limit),
		Skip:   int64(skip),
	})
}

// GetEvent handles GET /api/v1/events/:id
func (h *EventHandler) GetEvent(c *fiber.Ctx) error {
	eventID := c.Params("id")
	if eventID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_request",
			Message: "Event ID is required",
		})
	}

	event, err := h.db.EventRepo.FindByEventID(c.Context(), eventID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "event_not_found",
			Message: "Event not found",
		})
	}

	return c.JSON(toEventResponse(event))
}

// GetEventByTransactionHash handles GET /api/v1/events/tx/:hash
func (h *EventHandler) GetEventByTransactionHash(c *fiber.Ctx) error {
	txHash := c.Params("hash")
	if txHash == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_request",
			Message: "Transaction hash is required",
		})
	}

	events, err := h.db.EventRepo.FindByTxHash(c.Context(), txHash)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "query_failed",
			Message: err.Error(),
		})
	}

	if len(events) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "event_not_found",
			Message: "No events found for transaction hash",
		})
	}

	responses := make([]*EventResponse, len(events))
	for i, event := range events {
		responses[i] = toEventResponse(event)
	}

	return c.JSON(responses)
}

// toEventResponse converts an event model to response format
func toEventResponse(event *models.Event) *EventResponse {
	return &EventResponse{
		EventID:        event.EventID,
		SubscriptionID: event.SubscriptionID,
		Network:        event.Network,
		Type:           event.Type,
		Address:        event.Address,
		TxHash:         event.TxHash,
		BlockNumber:    event.BlockNumber,
		BlockTimestamp: event.BlockTimestamp,
		Data:           event.Data,
		Processed:      event.Processed,
		CreatedAt:      event.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
