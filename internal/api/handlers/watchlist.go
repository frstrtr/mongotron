package handlers

import (
	"github.com/frstrtr/mongotron/internal/storage/models"
	"github.com/frstrtr/mongotron/internal/subscription"
	"github.com/gofiber/fiber/v2"
)

// WalletType defines the type of wallet being monitored
type WalletType string

const (
	// WalletTypeNPS represents NPS custodial wallets
	WalletTypeNPS WalletType = "nps"
	// WalletTypePortal represents non-custodial portal wallets
	WalletTypePortal WalletType = "portal"
	// WalletTypeExchange represents exchange wallets
	WalletTypeExchange WalletType = "exchange"
	// WalletTypeGeneral represents general/unspecified wallets
	WalletTypeGeneral WalletType = "general"
)

// WatchListHandler handles watch list management for USDT/TRC20 monitoring
// Supports multiple wallet types: NPS custodial, portal non-custodial, exchange, etc.
type WatchListHandler struct {
	manager subscription.ManagerInterface
}

// NewWatchListHandler creates a new watch list handler
func NewWatchListHandler(manager subscription.ManagerInterface) *WatchListHandler {
	return &WatchListHandler{
		manager: manager,
	}
}

// WatchAddressRequest represents a request to add an address to watch list
type WatchAddressRequest struct {
	Address     string                 `json:"address" validate:"required"`
	WalletType  WalletType             `json:"walletType,omitempty"`  // "nps", "portal", "exchange", "general"
	Label       string                 `json:"label,omitempty"`       // Optional label (e.g., "User Wallet #123")
	WebhookURL  string                 `json:"webhookUrl,omitempty"`  // Webhook for this specific address
	TokenFilter []string               `json:"tokenFilter,omitempty"` // e.g., ["USDT", "USDC"]
	StartBlock  int64                  `json:"startBlock,omitempty"`  // Start monitoring from specific block (0 = current)
	Metadata    map[string]interface{} `json:"metadata,omitempty"`    // Extra data (e.g., user_id, account_id, portal_user_id)
}

// BulkWatchRequest represents a bulk add request
type BulkWatchRequest struct {
	Addresses  []WatchAddressRequest `json:"addresses" validate:"required,min=1,max=100"`
	WebhookURL string                `json:"webhookUrl,omitempty"` // Default webhook for all addresses
}

// WatchListResponse represents a watched address in responses
type WatchListResponse struct {
	SubscriptionID string                 `json:"subscriptionId"`
	Address        string                 `json:"address"`
	WalletType     WalletType             `json:"walletType"`
	Label          string                 `json:"label,omitempty"`
	WebhookURL     string                 `json:"webhookUrl,omitempty"`
	TokenFilter    []string               `json:"tokenFilter,omitempty"`
	Status         string                 `json:"status"`
	EventsCount    int64                  `json:"eventsCount"`
	StartBlock     int64                  `json:"startBlock"`
	CurrentBlock   int64                  `json:"currentBlock"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt      string                 `json:"createdAt"`
}

// BulkWatchResponse represents bulk add response
type BulkWatchResponse struct {
	Success []WatchListResponse `json:"success"`
	Failed  []BulkFailure       `json:"failed,omitempty"`
	Total   int                 `json:"total"`
	Added   int                 `json:"added"`
}

// BulkFailure represents a failed bulk operation item
type BulkFailure struct {
	Address string `json:"address"`
	Error   string `json:"error"`
}

// ResubscribeRequest represents a request to resubscribe an address
type ResubscribeRequest struct {
	Address    string     `json:"address" validate:"required"`
	WalletType WalletType `json:"walletType,omitempty"`
	WebhookURL string     `json:"webhookUrl,omitempty"`
	ScanGap    bool       `json:"scanGap"` // Whether to scan for missed transactions during unsubscribed period
}

// ResubscribeResponse represents the response for a resubscription
type ResubscribeResponse struct {
	SubscriptionID string `json:"subscriptionId"`
	Address        string `json:"address"`
	Status         string `json:"status"`
	GapDetected    bool   `json:"gapDetected"`
	GapStart       int64  `json:"gapStart,omitempty"`
	GapEnd         int64  `json:"gapEnd,omitempty"`
	GapBlocks      int64  `json:"gapBlocks,omitempty"`
	GapScanning    bool   `json:"gapScanning"` // True if background gap scan was started
	Message        string `json:"message"`
}

// AddToWatchList handles POST /api/v1/watchlist
// Adds a single address to the watch list for TRC20 transfer monitoring
// Supports all wallet types: NPS custodial, portal non-custodial, exchange, etc.
func (h *WatchListHandler) AddToWatchList(c *fiber.Ctx) error {
	var req WatchAddressRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_request",
			Message: "Failed to parse request body",
		})
	}

	// Validate address format
	if !isValidTronAddress(req.Address) {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_address",
			Message: "Invalid Tron address format. Address should start with 'T' and be 34 characters",
		})
	}

	// Default wallet type to general if not specified
	if req.WalletType == "" {
		req.WalletType = WalletTypeGeneral
	}

	// Validate wallet type
	if !isValidWalletType(req.WalletType) {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_wallet_type",
			Message: "Invalid wallet type. Must be one of: nps, portal, exchange, general",
		})
	}

	// Create filters for TRC20 monitoring (TriggerSmartContract type)
	filters := models.SubscriptionFilters{
		ContractTypes: []string{"TriggerSmartContract"},
		OnlySuccess:   true,
	}

	// Use startBlock from request, or -1 for current block
	startBlock := req.StartBlock
	if startBlock == 0 {
		startBlock = -1 // Will use latest block
	}

	// Create subscription
	sub, err := h.manager.Subscribe(req.Address, req.WebhookURL, filters, startBlock)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "subscription_failed",
			Message: err.Error(),
		})
	}

	// Add wallet type to metadata
	if req.Metadata == nil {
		req.Metadata = make(map[string]interface{})
	}
	req.Metadata["walletType"] = string(req.WalletType)

	response := WatchListResponse{
		SubscriptionID: sub.SubscriptionID,
		Address:        sub.Address,
		WalletType:     req.WalletType,
		Label:          req.Label,
		WebhookURL:     sub.WebhookURL,
		TokenFilter:    req.TokenFilter,
		Status:         sub.Status,
		EventsCount:    sub.EventsCount,
		StartBlock:     sub.StartBlock,
		CurrentBlock:   sub.CurrentBlock,
		Metadata:       req.Metadata,
		CreatedAt:      sub.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	return c.Status(fiber.StatusCreated).JSON(response)
}

// isValidWalletType checks if the wallet type is valid
func isValidWalletType(wt WalletType) bool {
	switch wt {
	case WalletTypeNPS, WalletTypePortal, WalletTypeExchange, WalletTypeGeneral:
		return true
	}
	return false
}

// BulkAddToWatchList handles POST /api/v1/watchlist/bulk
// Adds multiple addresses to the watch list at once
func (h *WatchListHandler) BulkAddToWatchList(c *fiber.Ctx) error {
	var req BulkWatchRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_request",
			Message: "Failed to parse request body",
		})
	}

	if len(req.Addresses) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_request",
			Message: "At least one address is required",
		})
	}

	if len(req.Addresses) > 100 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_request",
			Message: "Maximum 100 addresses per request",
		})
	}

	response := BulkWatchResponse{
		Success: make([]WatchListResponse, 0),
		Failed:  make([]BulkFailure, 0),
		Total:   len(req.Addresses),
	}

	for _, addr := range req.Addresses {
		// Validate address
		if !isValidTronAddress(addr.Address) {
			response.Failed = append(response.Failed, BulkFailure{
				Address: addr.Address,
				Error:   "Invalid Tron address format",
			})
			continue
		}

		// Default wallet type
		walletType := addr.WalletType
		if walletType == "" {
			walletType = WalletTypeGeneral
		}

		// Use request-level webhook or address-specific
		webhookURL := addr.WebhookURL
		if webhookURL == "" {
			webhookURL = req.WebhookURL
		}

		// Use startBlock from address or default to current
		startBlock := addr.StartBlock
		if startBlock == 0 {
			startBlock = -1
		}

		// Create subscription
		filters := models.SubscriptionFilters{
			ContractTypes: []string{"TriggerSmartContract"},
			OnlySuccess:   true,
		}

		sub, err := h.manager.Subscribe(addr.Address, webhookURL, filters, startBlock)
		if err != nil {
			response.Failed = append(response.Failed, BulkFailure{
				Address: addr.Address,
				Error:   err.Error(),
			})
			continue
		}

		// Add wallet type to metadata
		metadata := addr.Metadata
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
		metadata["walletType"] = string(walletType)

		response.Success = append(response.Success, WatchListResponse{
			SubscriptionID: sub.SubscriptionID,
			Address:        sub.Address,
			WalletType:     walletType,
			Label:          addr.Label,
			WebhookURL:     sub.WebhookURL,
			TokenFilter:    addr.TokenFilter,
			Status:         sub.Status,
			EventsCount:    sub.EventsCount,
			StartBlock:     sub.StartBlock,
			CurrentBlock:   sub.CurrentBlock,
			Metadata:       metadata,
			CreatedAt:      sub.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	response.Added = len(response.Success)

	return c.Status(fiber.StatusOK).JSON(response)
}

// RemoveFromWatchList handles DELETE /api/v1/watchlist/:address
// Removes an address from the watch list
func (h *WatchListHandler) RemoveFromWatchList(c *fiber.Ctx) error {
	address := c.Params("address")
	if address == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_request",
			Message: "Address is required",
		})
	}

	// Find subscription by address
	sub, err := h.manager.GetByAddress(address)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "not_found",
			Message: "Address not found in watch list",
		})
	}

	// Unsubscribe
	if err := h.manager.Unsubscribe(sub.SubscriptionID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "unsubscribe_failed",
			Message: err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Address removed from watch list",
		"address": address,
	})
}

// ResubscribeToWatchList handles POST /api/v1/watchlist/:address/resubscribe
// Resubscribes a previously unsubscribed address and optionally scans for missed transactions
func (h *WatchListHandler) ResubscribeToWatchList(c *fiber.Ctx) error {
	address := c.Params("address")
	if address == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_request",
			Message: "Address is required",
		})
	}

	var req ResubscribeRequest
	if err := c.BodyParser(&req); err != nil {
		// If no body, use defaults
		req = ResubscribeRequest{
			Address: address,
			ScanGap: true, // Default to scanning for gap
		}
	}

	// Override address from URL
	req.Address = address

	// Validate address format
	if !isValidTronAddress(req.Address) {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_address",
			Message: "Invalid Tron address format. Address should start with 'T' and be 34 characters",
		})
	}

	// Create filters for TRC20 monitoring
	filters := models.SubscriptionFilters{
		ContractTypes: []string{"TriggerSmartContract"},
		OnlySuccess:   true,
	}

	// Call resubscribe which handles gap detection and scanning
	result, err := h.manager.Resubscribe(req.Address, req.WebhookURL, filters, req.ScanGap)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "resubscribe_failed",
			Message: err.Error(),
		})
	}

	response := ResubscribeResponse{
		SubscriptionID: result.Subscription.SubscriptionID,
		Address:        result.Subscription.Address,
		Status:         result.Subscription.Status,
		GapDetected:    result.GapDetected,
		GapStart:       result.GapStart,
		GapEnd:         result.GapEnd,
		GapBlocks:      result.GapEnd - result.GapStart,
		GapScanning:    result.GapScanning,
	}

	if result.GapDetected {
		if result.GapScanning {
			response.Message = "Resubscribed successfully. Background scan started to recover missed transactions."
		} else {
			response.Message = "Resubscribed successfully. Gap detected but scan not requested."
		}
	} else {
		response.Message = "New subscription created (no previous subscription found)."
	}

	return c.Status(fiber.StatusOK).JSON(response)
}

// GetWatchList handles GET /api/v1/watchlist
// Returns all watched addresses with optional filtering by wallet type
func (h *WatchListHandler) GetWatchList(c *fiber.Ctx) error {
	// Get pagination params
	limit := c.QueryInt("limit", 50)
	skip := c.QueryInt("skip", 0)
	walletTypeFilter := c.Query("walletType", "") // Optional filter

	if limit > 100 {
		limit = 100
	}

	// Get all active subscriptions
	subs, total, err := h.manager.List(int64(limit), int64(skip))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "list_failed",
			Message: err.Error(),
		})
	}

	// Convert to watch list response
	watchList := make([]WatchListResponse, 0, len(subs))
	for _, sub := range subs {
		// Extract wallet type from metadata or default to general
		walletType := WalletTypeGeneral
		// TODO: Read from subscription metadata when stored

		// Apply wallet type filter if specified
		if walletTypeFilter != "" && string(walletType) != walletTypeFilter {
			continue
		}

		watchList = append(watchList, WatchListResponse{
			SubscriptionID: sub.SubscriptionID,
			Address:        sub.Address,
			WalletType:     walletType,
			WebhookURL:     sub.WebhookURL,
			Status:         sub.Status,
			EventsCount:    sub.EventsCount,
			StartBlock:     sub.StartBlock,
			CurrentBlock:   sub.CurrentBlock,
			CreatedAt:      sub.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"watchList": watchList,
		"total":     total,
		"limit":     limit,
		"skip":      skip,
	})
}

// GetWatchedAddress handles GET /api/v1/watchlist/:address
// Returns details for a specific watched address
func (h *WatchListHandler) GetWatchedAddress(c *fiber.Ctx) error {
	address := c.Params("address")
	if address == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_request",
			Message: "Address is required",
		})
	}

	sub, err := h.manager.GetByAddress(address)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "not_found",
			Message: "Address not found in watch list",
		})
	}

	response := WatchListResponse{
		SubscriptionID: sub.SubscriptionID,
		Address:        sub.Address,
		WebhookURL:     sub.WebhookURL,
		Status:         sub.Status,
		EventsCount:    sub.EventsCount,
		CreatedAt:      sub.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	return c.Status(fiber.StatusOK).JSON(response)
}

// ScanHistoricalRequest represents a request to scan historical blocks
type ScanHistoricalRequest struct {
	FromBlock int64 `json:"fromBlock" validate:"required"` // Start block number
	ToBlock   int64 `json:"toBlock,omitempty"`             // End block number (0 = current)
}

// ScanHistorical handles POST /api/v1/watchlist/:address/scan
// Triggers a historical scan for an address from a specific block range
// Useful when adding a new wallet and need to catch up on past transactions
func (h *WatchListHandler) ScanHistorical(c *fiber.Ctx) error {
	address := c.Params("address")
	if address == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_request",
			Message: "Address is required",
		})
	}

	var req ScanHistoricalRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_request",
			Message: "Failed to parse request body",
		})
	}

	if req.FromBlock <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_request",
			Message: "fromBlock must be a positive block number",
		})
	}

	// Find the subscription for this address
	sub, err := h.manager.GetByAddress(address)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "not_found",
			Message: "Address not found in watch list. Add it first with POST /api/v1/watchlist",
		})
	}

	// Start the historical scan in a goroutine so we can return immediately
	go func() {
		if err := h.manager.ScanHistorical(sub.SubscriptionID, req.FromBlock, req.ToBlock); err != nil {
			// Log the error - we can't return it to the client since we're async
			// In a production system, you might want to store scan status in DB
			_ = err // Error is logged in the manager
		}
	}()

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"message":        "Historical scan initiated",
		"address":        address,
		"subscriptionId": sub.SubscriptionID,
		"fromBlock":      req.FromBlock,
		"toBlock":        req.ToBlock,
		"status":         "scanning",
		"note":           "Scan is running in background. Events will be processed through normal pipeline including Porto webhooks.",
	})
}

// isValidTronAddress validates Tron address format
func isValidTronAddress(address string) bool {
	if len(address) != 34 {
		return false
	}
	if address[0] != 'T' {
		return false
	}
	// Basic base58 character check
	for _, c := range address {
		if !isBase58Char(c) {
			return false
		}
	}
	return true
}

// isBase58Char checks if character is valid base58
func isBase58Char(c rune) bool {
	// Base58 alphabet (no 0, O, I, l)
	return (c >= '1' && c <= '9') ||
		(c >= 'A' && c <= 'H') ||
		(c >= 'J' && c <= 'N') ||
		(c >= 'P' && c <= 'Z') ||
		(c >= 'a' && c <= 'k') ||
		(c >= 'm' && c <= 'z')
}
