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
	SubscriptionID string                 `json:"subscriptionId"`
	Address        string                 `json:"address"`
	WalletType     WalletType             `json:"walletType"`
	UserID         string                 `json:"userId,omitempty"`
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
			Message: "Invalid wallet type. Must be one of: platform, nps, portal, exchange, general",
		})
	}

	// Build contract types based on asset filter
	contractTypes := buildContractTypes(req.AssetTypes)

	// Create filters
	filters := models.SubscriptionFilters{
		ContractTypes: contractTypes,
		AssetTypes:    req.AssetTypes,
		TokenFilter:   req.TokenFilter,
		OnlySuccess:   true,
	}

	// Use startBlock from request, or -1 for current block
	startBlock := req.StartBlock
	if startBlock == 0 {
		startBlock = -1 // Will use latest block
	}

	// Create subscription with full options
	sub, err := h.manager.SubscribeWithOptions(subscription.SubscribeOptions{
		Address:    req.Address,
		WebhookURL: req.WebhookURL,
		Filters:    filters,
		StartBlock: startBlock,
		WalletType: string(req.WalletType),
		UserID:     req.UserID,
		Label:      req.Label,
		Metadata:   req.Metadata,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "subscription_failed",
			Message: err.Error(),
		})
	}

	response := WatchListResponse{
		SubscriptionID: sub.SubscriptionID,
		Address:        sub.Address,
		WalletType:     WalletType(sub.WalletType),
		Label:          sub.Label,
		WebhookURL:     sub.WebhookURL,
		TokenFilter:    req.TokenFilter,
		Status:         sub.Status,
		EventsCount:    sub.EventsCount,
		StartBlock:     sub.StartBlock,
		CurrentBlock:   sub.CurrentBlock,
		Metadata:       sub.Metadata,
		CreatedAt:      sub.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	return c.Status(fiber.StatusCreated).JSON(response)
}

// buildContractTypes returns the contract types to monitor based on asset types
func buildContractTypes(assetTypes []string) []string {
	if len(assetTypes) == 0 {
		// Default: monitor all transfer types
		return []string{"TransferContract", "TransferAssetContract", "TriggerSmartContract"}
	}

	contractTypes := make([]string, 0, 10)
	for _, asset := range assetTypes {
		switch asset {
		case "TRX":
			if !contains(contractTypes, "TransferContract") {
				contractTypes = append(contractTypes, "TransferContract")
			}
		case "TRC10":
			if !contains(contractTypes, "TransferAssetContract") {
				contractTypes = append(contractTypes, "TransferAssetContract")
			}
		case "TRC20":
			if !contains(contractTypes, "TriggerSmartContract") {
				contractTypes = append(contractTypes, "TriggerSmartContract")
			}
		case "*":
			// All transfer types (legacy)
			return []string{"TransferContract", "TransferAssetContract", "TriggerSmartContract"}

		// Staking operations
		case "STAKE", "FREEZE":
			if !contains(contractTypes, "FreezeBalanceV2Contract") {
				contractTypes = append(contractTypes, "FreezeBalanceV2Contract")
			}
		case "UNSTAKE", "UNFREEZE":
			if !contains(contractTypes, "UnfreezeBalanceV2Contract") {
				contractTypes = append(contractTypes, "UnfreezeBalanceV2Contract")
			}
		case "WITHDRAW_UNSTAKE":
			if !contains(contractTypes, "WithdrawExpireUnfreezeContract") {
				contractTypes = append(contractTypes, "WithdrawExpireUnfreezeContract")
			}

		// Delegation operations
		case "DELEGATE":
			if !contains(contractTypes, "DelegateResourceContract") {
				contractTypes = append(contractTypes, "DelegateResourceContract")
			}
		case "UNDELEGATE":
			if !contains(contractTypes, "UnDelegateResourceContract") {
				contractTypes = append(contractTypes, "UnDelegateResourceContract")
			}

		// Voting operations
		case "VOTE":
			if !contains(contractTypes, "VoteWitnessContract") {
				contractTypes = append(contractTypes, "VoteWitnessContract")
			}

		// Permission operations (CRITICAL for security)
		case "PERMISSION":
			if !contains(contractTypes, "AccountPermissionUpdateContract") {
				contractTypes = append(contractTypes, "AccountPermissionUpdateContract")
			}

		// Claim voting rewards
		case "CLAIM":
			if !contains(contractTypes, "WithdrawBalanceContract") {
				contractTypes = append(contractTypes, "WithdrawBalanceContract")
			}

		// All operations for full gas station monitoring
		case "ALL_OPERATIONS", "FULL":
			return []string{
				"TransferContract",
				"TransferAssetContract",
				"TriggerSmartContract",
				"FreezeBalanceV2Contract",
				"UnfreezeBalanceV2Contract",
				"WithdrawExpireUnfreezeContract",
				"DelegateResourceContract",
				"UnDelegateResourceContract",
				"VoteWitnessContract",
				"AccountPermissionUpdateContract",
				"WithdrawBalanceContract",
			}
		}
	}

	if len(contractTypes) == 0 {
		// Fallback to all transfer types
		return []string{"TransferContract", "TransferAssetContract", "TriggerSmartContract"}
	}

	return contractTypes
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
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

		// Build contract types based on asset filter
		contractTypes := buildContractTypes(addr.AssetTypes)

		// Create filters
		filters := models.SubscriptionFilters{
			ContractTypes: contractTypes,
			AssetTypes:    addr.AssetTypes,
			TokenFilter:   addr.TokenFilter,
			OnlySuccess:   true,
		}

		// Create subscription with full options
		sub, err := h.manager.SubscribeWithOptions(subscription.SubscribeOptions{
			Address:    addr.Address,
			WebhookURL: webhookURL,
			Filters:    filters,
			StartBlock: startBlock,
			WalletType: string(walletType),
			UserID:     addr.UserID,
			Label:      addr.Label,
			Metadata:   addr.Metadata,
		})
		if err != nil {
			response.Failed = append(response.Failed, BulkFailure{
				Address: addr.Address,
				Error:   err.Error(),
			})
			continue
		}

		response.Success = append(response.Success, WatchListResponse{
			SubscriptionID: sub.SubscriptionID,
			Address:        sub.Address,
			WalletType:     WalletType(sub.WalletType),
			UserID:         sub.UserID,
			Label:          sub.Label,
			WebhookURL:     sub.WebhookURL,
			TokenFilter:    addr.TokenFilter,
			Status:         sub.Status,
			EventsCount:    sub.EventsCount,
			StartBlock:     sub.StartBlock,
			CurrentBlock:   sub.CurrentBlock,
			Metadata:       sub.Metadata,
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
		// Get wallet type from subscription (default to general if empty)
		walletType := WalletType(sub.WalletType)
		if walletType == "" {
			walletType = WalletTypeGeneral
		}

		// Apply wallet type filter if specified
		if walletTypeFilter != "" && string(walletType) != walletTypeFilter {
			continue
		}

		watchList = append(watchList, WatchListResponse{
			SubscriptionID: sub.SubscriptionID,
			Address:        sub.Address,
			WalletType:     walletType,
			UserID:         sub.UserID,
			Label:          sub.Label,
			WebhookURL:     sub.WebhookURL,
			Status:         sub.Status,
			EventsCount:    sub.EventsCount,
			StartBlock:     sub.StartBlock,
			CurrentBlock:   sub.CurrentBlock,
			Metadata:       sub.Metadata,
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

	// Get wallet type from subscription (default to general if empty)
	walletType := WalletType(sub.WalletType)
	if walletType == "" {
		walletType = WalletTypeGeneral
	}

	response := WatchListResponse{
		SubscriptionID: sub.SubscriptionID,
		Address:        sub.Address,
		WalletType:     walletType,
		UserID:         sub.UserID,
		Label:          sub.Label,
		WebhookURL:     sub.WebhookURL,
		Status:         sub.Status,
		EventsCount:    sub.EventsCount,
		StartBlock:     sub.StartBlock,
		CurrentBlock:   sub.CurrentBlock,
		Metadata:       sub.Metadata,
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
