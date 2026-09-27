package handlers

import (
	"sync"

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
	// WalletTypePlatform represents platform deposit wallets (user-specific)
	WalletTypePlatform WalletType = "platform"
	// WalletTypeGeneral represents general/unspecified wallets
	WalletTypeGeneral WalletType = "general"
	// WalletTypeGasStation represents gas station pool wallets
	WalletTypeGasStation WalletType = "gasstation"
	// WalletTypeInvoice represents invoice payment wallets
	WalletTypeInvoice WalletType = "invoice"
)

// WatchListHandler handles watch list management for USDT/TRC20 monitoring
// Supports multiple wallet types: NPS custodial, portal non-custodial, exchange, etc.
type WatchListHandler struct {
	manager subscription.ManagerInterface
	// upsertMu serialises the lookup-then-create-or-update of watchlist writes so
	// two concurrent posts for the same address cannot both create a subscription.
	upsertMu sync.Mutex
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
	WalletType  WalletType             `json:"walletType,omitempty"`  // "platform", "nps", "portal", "exchange", "general"
	UserID      string                 `json:"userId,omitempty"`      // User identifier (telegram_id, etc.)
	Label       string                 `json:"label,omitempty"`       // Optional label (e.g., "User Wallet #123")
	WebhookURL  string                 `json:"webhookUrl,omitempty"`  // Webhook for this specific address
	TokenFilter []string               `json:"tokenFilter,omitempty"` // e.g., ["USDT", "USDC"]
	AssetTypes  []string               `json:"assetTypes,omitempty"`  // e.g., ["TRX", "TRC10", "TRC20"] - empty means all
	StartBlock  int64                  `json:"startBlock,omitempty"`  // Start monitoring from specific block (0 = current)
	Metadata    map[string]interface{} `json:"metadata,omitempty"`    // Extra data (e.g., account_id, portal_user_id)
}

// BulkWatchRequest represents a bulk add request
type BulkWatchRequest struct {
	Addresses  []WatchAddressRequest `json:"addresses" validate:"required,min=1,max=100"`
	WebhookURL string                `json:"webhookUrl,omitempty"` // Default webhook for all addresses
}

// WatchListResponse represents a watched address in responses
type WatchListResponse struct {
	SubscriptionID string     `json:"subscriptionId"`
	Address        string     `json:"address"`
	WalletType     WalletType `json:"walletType"`
	UserID         string     `json:"userId,omitempty"`
	Label          string     `json:"label,omitempty"`
	WebhookURL     string     `json:"webhookUrl,omitempty"`
	TokenFilter    []string   `json:"tokenFilter,omitempty"`
	// AssetTypes are the asset types the subscription was asked to watch, e.g.
	// ["TRX", "TRC20"]. Always present (an empty list means all transfer types),
	// so its presence also tells a client that this server supports upsert.
	AssetTypes []string `json:"assetTypes"`
	// Filters are the effective filters the running monitor applies. TRX is
	// watched when filters.contractTypes is empty or contains "TransferContract".
	Filters      models.SubscriptionFilters `json:"filters"`
	Status       string                     `json:"status"`
	EventsCount  int64                      `json:"eventsCount"`
	StartBlock   int64                      `json:"startBlock"`
	CurrentBlock int64                      `json:"currentBlock"`
	Metadata     map[string]interface{}     `json:"metadata,omitempty"`
	CreatedAt    string                     `json:"createdAt"`
}

// BulkWatchResponse represents bulk add response. Added counts every address that
// succeeded (created or updated in place), Created and Updated split it.
type BulkWatchResponse struct {
	Success []WatchListResponse `json:"success"`
	Failed  []BulkFailure       `json:"failed,omitempty"`
	Total   int                 `json:"total"`
	Added   int                 `json:"added"`
	Created int                 `json:"created"`
	Updated int                 `json:"updated"`
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
//
// Upsert: when the address has no active subscription, a new one is created and
// the answer is 201. When an active subscription exists, it is updated in place
// and the answer is 200 with the row: the running monitor keeps going (no gap)
// and no second subscription is created. See upsertWatch for which fields change.
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

	// Validate wallet type (empty means general on create, unchanged on update)
	if req.WalletType != "" && !isValidWalletType(req.WalletType) {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_wallet_type",
			Message: "Invalid wallet type. Must be one of: platform, nps, portal, exchange, gasstation, invoice, general",
		})
	}

	sub, created, err := h.upsertWatch(req, req.WebhookURL)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "subscription_failed",
			Message: err.Error(),
		})
	}

	status := fiber.StatusOK
	if created {
		status = fiber.StatusCreated
	}
	return c.Status(status).JSON(toWatchListResponse(sub))
}

// upsertWatch creates the subscription for req.Address, or updates the active one
// in place. It reports whether a new subscription was created.
//
// Update semantics (fields omitted from the request keep their stored value):
//   - assetTypes present: the contract type filter is rebuilt from it (an empty
//     list means all transfer types) and onlySuccess is set, as on create.
//   - tokenFilter present: replaces the stored token filter.
//   - walletType, userId, label: replace the stored value when non-empty.
//   - metadata present: replaces the stored metadata as a whole (no merge).
//   - webhookUrl and startBlock are creation-only and never change an existing row.
func (h *WatchListHandler) upsertWatch(req WatchAddressRequest, webhookURL string) (*models.Subscription, bool, error) {
	h.upsertMu.Lock()
	defer h.upsertMu.Unlock()

	if existing, err := h.manager.GetByAddress(req.Address); err == nil && existing != nil && existing.Status == "active" {
		sub, err := h.manager.UpdateSubscription(existing.SubscriptionID, resolveWatchUpdate(existing, req))
		return sub, false, err
	}

	walletType := req.WalletType
	if walletType == "" {
		walletType = WalletTypeGeneral
	}

	// Use startBlock from request, or -1 for current block
	startBlock := req.StartBlock
	if startBlock == 0 {
		startBlock = -1 // Will use latest block
	}

	sub, err := h.manager.SubscribeWithOptions(subscription.SubscribeOptions{
		Address:    req.Address,
		WebhookURL: webhookURL,
		Filters:    subscription.FiltersForAssets(req.AssetTypes, req.TokenFilter),
		StartBlock: startBlock,
		WalletType: string(walletType),
		UserID:     req.UserID,
		Label:      req.Label,
		Metadata:   req.Metadata,
	})
	if err != nil {
		return nil, false, err
	}
	return sub, true, nil
}

// resolveWatchUpdate computes the desired state of an existing subscription from a
// watchlist request, keeping stored values for omitted fields (see upsertWatch).
func resolveWatchUpdate(existing *models.Subscription, req WatchAddressRequest) subscription.SubscriptionUpdate {
	filters := existing.Filters
	if req.AssetTypes != nil {
		filters.ContractTypes = subscription.ContractTypesForAssets(req.AssetTypes)
		filters.AssetTypes = req.AssetTypes
		filters.OnlySuccess = true
	}
	if req.TokenFilter != nil {
		filters.TokenFilter = req.TokenFilter
	}

	upd := subscription.SubscriptionUpdate{
		Filters:    filters,
		WalletType: existing.WalletType,
		UserID:     existing.UserID,
		Label:      existing.Label,
		Metadata:   existing.Metadata,
	}
	if req.WalletType != "" {
		upd.WalletType = string(req.WalletType)
	}
	if req.UserID != "" {
		upd.UserID = req.UserID
	}
	if req.Label != "" {
		upd.Label = req.Label
	}
	if req.Metadata != nil {
		upd.Metadata = req.Metadata
	}
	return upd
}

// toWatchListResponse renders a subscription as a watchlist row.
func toWatchListResponse(sub *models.Subscription) WatchListResponse {
	// Get wallet type from subscription (default to general if empty)
	walletType := WalletType(sub.WalletType)
	if walletType == "" {
		walletType = WalletTypeGeneral
	}

	assetTypes := sub.Filters.AssetTypes
	if assetTypes == nil {
		assetTypes = []string{}
	}

	return WatchListResponse{
		SubscriptionID: sub.SubscriptionID,
		Address:        sub.Address,
		WalletType:     walletType,
		UserID:         sub.UserID,
		Label:          sub.Label,
		WebhookURL:     sub.WebhookURL,
		TokenFilter:    sub.Filters.TokenFilter,
		AssetTypes:     assetTypes,
		Filters:        sub.Filters,
		Status:         sub.Status,
		EventsCount:    sub.EventsCount,
		StartBlock:     sub.StartBlock,
		CurrentBlock:   sub.CurrentBlock,
		Metadata:       sub.Metadata,
		CreatedAt:      sub.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// buildContractTypes returns the contract types to monitor based on asset types.
// Kept as a thin alias: the mapping lives in the subscription package.
func buildContractTypes(assetTypes []string) []string {
	return subscription.ContractTypesForAssets(assetTypes)
}

// isValidWalletType checks if the wallet type is valid
func isValidWalletType(wt WalletType) bool {
	switch wt {
	case WalletTypeNPS, WalletTypePortal, WalletTypeExchange, WalletTypePlatform, WalletTypeGeneral, WalletTypeGasStation, WalletTypeInvoice:
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

	if len(req.Addresses) > 1000 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "invalid_request",
			Message: "Maximum 1000 addresses per request",
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

		if addr.WalletType != "" && !isValidWalletType(addr.WalletType) {
			response.Failed = append(response.Failed, BulkFailure{
				Address: addr.Address,
				Error:   "Invalid wallet type",
			})
			continue
		}

		// Use request-level webhook or address-specific
		webhookURL := addr.WebhookURL
		if webhookURL == "" {
			webhookURL = req.WebhookURL
		}

		// Same upsert as the single endpoint: an active subscription is updated in place
		sub, created, err := h.upsertWatch(addr, webhookURL)
		if err != nil {
			response.Failed = append(response.Failed, BulkFailure{
				Address: addr.Address,
				Error:   err.Error(),
			})
			continue
		}
		if created {
			response.Created++
		} else {
			response.Updated++
		}

		response.Success = append(response.Success, toWatchListResponse(sub))
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
		row := toWatchListResponse(sub)

		// Apply wallet type filter if specified
		if walletTypeFilter != "" && string(row.WalletType) != walletTypeFilter {
			continue
		}

		watchList = append(watchList, row)
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

	response := toWatchListResponse(sub)

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
