package subscription

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/fbsobreira/gotron-sdk/pkg/proto/core"
	"github.com/frstrtr/mongotron/internal/blockchain/monitor"
	"github.com/frstrtr/mongotron/internal/blockchain/parser"
	"github.com/frstrtr/mongotron/internal/storage"
	"github.com/frstrtr/mongotron/internal/storage/models"
	"github.com/frstrtr/mongotron/internal/webhook"
	"github.com/frstrtr/mongotron/pkg/logger"
)

// EventRouter routes events to clients (WebSocket and webhook)
type EventRouter struct {
	db            *storage.Database
	logger        *logger.Logger
	wsClients     map[string][]*WebSocketClient // key: subscription_id
	eventQueue    chan *RouteEventRequest
	webhookClient *http.Client
	portoClient   *webhook.PortoAPIClient
	trc20Parser   *parser.TRC20Parser
	tronParser    *parser.TronParser
	network       string // "tron-mainnet" or "tron-nile"
	mu            sync.RWMutex
}

// RouteEventRequest contains event routing information
type RouteEventRequest struct {
	Subscription *models.Subscription
	Event        *monitor.AddressEvent
}

// WebSocketClient represents a connected WebSocket client
type WebSocketClient struct {
	ID       string
	SendChan chan []byte
	mu       sync.RWMutex
	closed   bool // Track if channel has been closed
}

// NewEventRouter creates a new event router
func NewEventRouter(db *storage.Database, log *logger.Logger) *EventRouter {
	return &EventRouter{
		db:          db,
		logger:      log,
		wsClients:   make(map[string][]*WebSocketClient),
		eventQueue:  make(chan *RouteEventRequest, 1000),
		trc20Parser: parser.NewTRC20Parser(),
		tronParser:  parser.NewTronParser(log),
		network:     "tron-nile", // Default to testnet
		webhookClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SetPortoClient sets the Porto API client for webhook notifications
func (r *EventRouter) SetPortoClient(client *webhook.PortoAPIClient) {
	r.portoClient = client
}

// SetNetwork sets the network name (tron-mainnet or tron-nile)
func (r *EventRouter) SetNetwork(network string) {
	r.network = network
}

// Run starts the event router
func (r *EventRouter) Run(ctx context.Context) {
	r.logger.Info().Msg("Event router started")

	for {
		select {
		case <-ctx.Done():
			r.logger.Info().Msg("Event router stopped")
			return

		case req := <-r.eventQueue:
			r.routeEvent(req)
		}
	}
}

// RouteEvent queues an event for routing
func (r *EventRouter) RouteEvent(sub *models.Subscription, event *monitor.AddressEvent) error {
	req := &RouteEventRequest{
		Subscription: sub,
		Event:        event,
	}

	select {
	case r.eventQueue <- req:
		return nil
	default:
		return fmt.Errorf("event queue full")
	}
}

// routeEvent routes an event to all registered destinations
func (r *EventRouter) routeEvent(req *RouteEventRequest) {
	// Convert event to JSON
	eventData, err := json.Marshal(req.Event)
	if err != nil {
		r.logger.Error().
			Err(err).
			Str("subscriptionId", req.Subscription.SubscriptionID).
			Msg("Failed to marshal event")
		return
	}

	// Route to WebSocket clients
	r.sendToWebSocketClients(req.Subscription.SubscriptionID, eventData)

	// Route to webhook if configured
	if req.Subscription.WebhookURL != "" {
		go r.sendToWebhook(req.Subscription, eventData)
	}

	// Route to Porto API based on contract type
	if r.portoClient != nil {
		switch req.Event.ContractType {
		case "TransferContract":
			// Native TRX transfer
			go r.handleTRXTransfer(req)
		case "TransferAssetContract":
			// TRC10 token transfer
			go r.handleTRC10Transfer(req)
		case "TriggerSmartContract":
			// TRC20 token transfer (smart contract call)
			go r.handleTRC20Transfer(req)
		// Gas station operations
		case "FreezeBalanceV2Contract":
			go r.handleFreezeOperation(req)
		case "UnfreezeBalanceV2Contract":
			go r.handleUnfreezeOperation(req)
		case "WithdrawExpireUnfreezeContract":
			go r.handleWithdrawOperation(req)
		case "DelegateResourceContract":
			go r.handleDelegateOperation(req)
		case "UnDelegateResourceContract":
			go r.handleUnDelegateOperation(req)
		case "VoteWitnessContract":
			go r.handleVoteOperation(req)
		case "AccountPermissionUpdateContract":
			// CRITICAL: Permission changes - immediate processing
			go r.handlePermissionOperation(req)
		case "WithdrawBalanceContract":
			// Claim voting rewards
			go r.handleClaimRewardsOperation(req)
		}
	}

	// Store in database (events collection)
	go r.storeEvent(req)
}

// handleTRC20Transfer checks if the event is a TRC20 transfer and sends to Porto API
func (r *EventRouter) handleTRC20Transfer(req *RouteEventRequest) {
	// Get smart contract data from EventData
	scData, ok := req.Event.EventData["smartContract"].(map[string]interface{})
	if !ok {
		r.logger.Debug().
			Str("txHash", req.Event.TransactionID).
			Msg("TRC20 check: No smartContract data in event")
		return
	}

	// Check if this is a transfer method
	methodSig, _ := scData["methodSignature"].(string)
	if methodSig != "a9059cbb" && methodSig != "23b872dd" {
		r.logger.Debug().
			Str("txHash", req.Event.TransactionID).
			Str("methodSig", methodSig).
			Msg("TRC20 check: Not a transfer method")
		return // Not a transfer
	}

	// Get the contract address (the token contract being called)
	contractAddress := req.Event.To

	// Check if this is a USDT contract
	if !r.trc20Parser.IsUSDTContract(contractAddress) {
		r.logger.Debug().
			Str("txHash", req.Event.TransactionID).
			Str("contractAddress", contractAddress).
			Msg("TRC20 check: Not a USDT contract")
		return
	}

	r.logger.Info().
		Str("txHash", req.Event.TransactionID).
		Str("contractAddress", contractAddress).
		Str("methodSig", methodSig).
		Str("watchedAddress", req.Subscription.Address).
		Msg("USDT transfer detected, processing...")

	// Parse the transfer details
	transfer := &parser.TRC20Transfer{
		ContractAddress: contractAddress,
		TxHash:          req.Event.TransactionID,
		BlockNumber:     req.Event.BlockNumber,
		BlockTimestamp:  req.Event.BlockTimestamp,
		Success:         req.Event.Success,
	}

	// Get token info
	transfer.TokenSymbol, transfer.TokenDecimals = r.getTokenInfo(contractAddress)

	// Extract parameters from the smart contract call
	params, hasParams := scData["parameters"].(map[string]interface{})
	if !hasParams {
		r.logger.Warn().
			Str("txHash", req.Event.TransactionID).
			Msg("TRC20 transfer: No parameters found in smart contract data")
		return
	}

	// Extract recipient address
	if to, ok := params["to"].(string); ok {
		transfer.ToHex = to
		transfer.To = parser.HexToBase58(to)
	}

	// Extract sender address (for transferFrom)
	if from, ok := params["from"].(string); ok {
		transfer.FromHex = from
		transfer.From = parser.HexToBase58(from)
	}

	// Extract amount - handle both string and numeric types
	if amountVal := params["amount"]; amountVal != nil {
		switch v := amountVal.(type) {
		case string:
			transfer.AmountDecimal = r.formatAmount(v, transfer.TokenDecimals)
		case float64:
			transfer.AmountDecimal = r.formatAmount(fmt.Sprintf("%.0f", v), transfer.TokenDecimals)
		case int64:
			transfer.AmountDecimal = r.formatAmount(fmt.Sprintf("%d", v), transfer.TokenDecimals)
		case int:
			transfer.AmountDecimal = r.formatAmount(fmt.Sprintf("%d", v), transfer.TokenDecimals)
		}
	}

	// For transfer() method, From is the transaction sender (owner_address)
	if methodSig == "a9059cbb" {
		transfer.From = parser.HexToBase58(req.Event.From)
		transfer.FromHex = req.Event.From
		transfer.MethodType = "transfer"
	} else {
		transfer.MethodType = "transferFrom"
	}

	r.logger.Info().
		Str("txHash", transfer.TxHash).
		Str("token", transfer.TokenSymbol).
		Str("from", transfer.From).
		Str("to", transfer.To).
		Str("amount", transfer.AmountDecimal).
		Str("watchedAddress", req.Subscription.Address).
		Msg("Parsed TRC20 transfer details")

	// Check if this transfer involves our watched address
	watchedAddr := req.Subscription.Address
	watchedAddrHex := parser.Base58ToHex(watchedAddr)

	isIncoming := transfer.To == watchedAddr || transfer.ToHex == watchedAddrHex
	isOutgoing := transfer.From == watchedAddr || transfer.FromHex == watchedAddrHex

	if !isIncoming && !isOutgoing {
		r.logger.Debug().
			Str("txHash", transfer.TxHash).
			Str("transferTo", transfer.To).
			Str("transferFrom", transfer.From).
			Str("watchedAddress", watchedAddr).
			Msg("TRC20 transfer does not involve watched address")
		return
	}

	// Create Porto event
	portoEvent := webhook.CreateTransferEvent(
		transfer,
		watchedAddr,
		req.Subscription.SubscriptionID,
		r.network,
	)

	// Set direction based on our analysis
	if isIncoming {
		portoEvent.Direction = "incoming"
	} else {
		portoEvent.Direction = "outgoing"
	}

	// Add subscription metadata to event
	portoEvent.WalletType = req.Subscription.WalletType
	portoEvent.UserID = req.Subscription.UserID
	portoEvent.Label = req.Subscription.Label
	portoEvent.Metadata = req.Subscription.Metadata

	r.logger.Info().
		Str("txHash", transfer.TxHash).
		Str("token", transfer.TokenSymbol).
		Str("to", transfer.To).
		Str("amount", transfer.AmountDecimal).
		Str("direction", portoEvent.Direction).
		Msg("Sending TRC20 transfer notification to Porto API")

	// Send to Porto API
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := r.portoClient.SendTransferNotification(ctx, portoEvent); err != nil {
		r.logger.Error().
			Err(err).
			Str("txHash", transfer.TxHash).
			Str("to", transfer.To).
			Msg("Failed to send transfer notification to Porto API")
	} else {
		r.logger.Info().
			Str("txHash", transfer.TxHash).
			Str("token", transfer.TokenSymbol).
			Str("to", transfer.To).
			Str("amount", transfer.AmountDecimal).
			Str("direction", portoEvent.Direction).
			Msg("TRC20 transfer notification sent to Porto API successfully")
	}
}

// handleTRXTransfer handles native TRX transfer events
func (r *EventRouter) handleTRXTransfer(req *RouteEventRequest) {
	// Extract addresses
	from := req.Event.From
	to := req.Event.To
	amount := req.Event.Amount

	// Convert hex addresses to base58 if needed
	if len(from) == 42 && from[:2] == "41" {
		from = parser.HexToBase58(from)
	}
	if len(to) == 42 && to[:2] == "41" {
		to = parser.HexToBase58(to)
	}

	// Check if this transfer involves our watched address
	watchedAddr := req.Subscription.Address
	isIncoming := to == watchedAddr
	isOutgoing := from == watchedAddr

	if !isIncoming && !isOutgoing {
		r.logger.Debug().
			Str("txHash", req.Event.TransactionID).
			Str("from", from).
			Str("to", to).
			Str("watchedAddress", watchedAddr).
			Msg("TRX transfer does not involve watched address")
		return
	}

	r.logger.Info().
		Str("txHash", req.Event.TransactionID).
		Str("from", from).
		Str("to", to).
		Int64("amount", amount).
		Str("watchedAddress", watchedAddr).
		Msg("TRX transfer detected, processing...")

	// Create Porto event
	portoEvent := webhook.CreateTRXTransferEvent(
		req.Event.TransactionID,
		req.Event.BlockNumber,
		req.Event.BlockTimestamp,
		req.Event.Success,
		from,
		to,
		amount,
		watchedAddr,
		req.Subscription.SubscriptionID,
		r.network,
	)

	// Add subscription metadata to event
	portoEvent.WalletType = req.Subscription.WalletType
	portoEvent.UserID = req.Subscription.UserID
	portoEvent.Label = req.Subscription.Label
	portoEvent.Metadata = req.Subscription.Metadata

	r.logger.Info().
		Str("txHash", portoEvent.TxHash).
		Str("from", from).
		Str("to", to).
		Str("amount", portoEvent.AmountDecimal).
		Str("direction", portoEvent.Direction).
		Msg("Sending TRX transfer notification to Porto API")

	// Send to Porto API
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := r.portoClient.SendTransferNotification(ctx, portoEvent); err != nil {
		r.logger.Error().
			Err(err).
			Str("txHash", portoEvent.TxHash).
			Str("to", to).
			Msg("Failed to send TRX transfer notification to Porto API")
	} else {
		r.logger.Info().
			Str("txHash", portoEvent.TxHash).
			Str("to", to).
			Str("amount", portoEvent.AmountDecimal).
			Str("direction", portoEvent.Direction).
			Msg("TRX transfer notification sent to Porto API successfully")
	}
}

// handleTRC10Transfer handles TRC10 token transfer events
func (r *EventRouter) handleTRC10Transfer(req *RouteEventRequest) {
	// Extract addresses and asset info
	from := req.Event.From
	to := req.Event.To
	amount := req.Event.Amount
	assetName := req.Event.AssetName // TRC10 asset ID/name

	// Convert hex addresses to base58 if needed
	if len(from) == 42 && from[:2] == "41" {
		from = parser.HexToBase58(from)
	}
	if len(to) == 42 && to[:2] == "41" {
		to = parser.HexToBase58(to)
	}

	// Check if this transfer involves our watched address
	watchedAddr := req.Subscription.Address
	isIncoming := to == watchedAddr
	isOutgoing := from == watchedAddr

	if !isIncoming && !isOutgoing {
		r.logger.Debug().
			Str("txHash", req.Event.TransactionID).
			Str("from", from).
			Str("to", to).
			Str("watchedAddress", watchedAddr).
			Msg("TRC10 transfer does not involve watched address")
		return
	}

	// Get TRC10 token info
	assetSymbol, decimals := r.getTRC10TokenInfo(assetName)

	r.logger.Info().
		Str("txHash", req.Event.TransactionID).
		Str("from", from).
		Str("to", to).
		Int64("amount", amount).
		Str("assetId", assetName).
		Str("assetSymbol", assetSymbol).
		Str("watchedAddress", watchedAddr).
		Msg("TRC10 transfer detected, processing...")

	// Create Porto event
	portoEvent := webhook.CreateTRC10TransferEvent(
		req.Event.TransactionID,
		req.Event.BlockNumber,
		req.Event.BlockTimestamp,
		req.Event.Success,
		from,
		to,
		amount,
		assetName,
		assetSymbol,
		decimals,
		watchedAddr,
		req.Subscription.SubscriptionID,
		r.network,
	)

	// Add subscription metadata to event
	portoEvent.WalletType = req.Subscription.WalletType
	portoEvent.UserID = req.Subscription.UserID
	portoEvent.Label = req.Subscription.Label
	portoEvent.Metadata = req.Subscription.Metadata

	r.logger.Info().
		Str("txHash", portoEvent.TxHash).
		Str("from", from).
		Str("to", to).
		Str("amount", portoEvent.AmountDecimal).
		Str("assetSymbol", assetSymbol).
		Str("direction", portoEvent.Direction).
		Msg("Sending TRC10 transfer notification to Porto API")

	// Send to Porto API
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := r.portoClient.SendTransferNotification(ctx, portoEvent); err != nil {
		r.logger.Error().
			Err(err).
			Str("txHash", portoEvent.TxHash).
			Str("to", to).
			Msg("Failed to send TRC10 transfer notification to Porto API")
	} else {
		r.logger.Info().
			Str("txHash", portoEvent.TxHash).
			Str("to", to).
			Str("amount", portoEvent.AmountDecimal).
			Str("assetSymbol", assetSymbol).
			Str("direction", portoEvent.Direction).
			Msg("TRC10 transfer notification sent to Porto API successfully")
	}
}

// getTRC10TokenInfo returns token symbol and decimals for known TRC10 tokens
func (r *EventRouter) getTRC10TokenInfo(assetID string) (string, int) {
	// Known TRC10 tokens on mainnet
	// BTT (BitTorrent) - ID: 1002000
	// WIN (WINk) - ID: 1002534
	// Common TRC10s vary - return the ID as symbol if unknown
	knownTokens := map[string]struct {
		Symbol   string
		Decimals int
	}{
		"1002000": {"BTT", 6},
		"1002534": {"WIN", 6},
		"_":       {"TRX", 6}, // TRX bandwidth (rare)
	}

	if info, ok := knownTokens[assetID]; ok {
		return info.Symbol, info.Decimals
	}
	// Unknown token - return asset ID as symbol with default decimals
	return assetID, 0
}

// ============================================================================
// Gas Station Operation Handlers
// ============================================================================

// getContractFromEvent extracts the first contract from an event's raw transaction
func (r *EventRouter) getContractFromEvent(event *monitor.AddressEvent) *core.Transaction_Contract {
	if event == nil || event.RawTransaction == nil {
		return nil
	}
	rawData := event.RawTransaction.GetRawData()
	if rawData == nil {
		return nil
	}
	contracts := rawData.GetContract()
	if len(contracts) == 0 {
		return nil
	}
	return contracts[0]
}

// handleFreezeOperation handles FreezeBalanceV2Contract (staking)
func (r *EventRouter) handleFreezeOperation(req *RouteEventRequest) {
	// Get contract from raw transaction
	contract := r.getContractFromEvent(req.Event)
	if contract == nil {
		r.logger.Debug().Str("txHash", req.Event.TransactionID).Msg("Freeze operation: no contract in event")
		return
	}

	owner, resourceType, amount := r.tronParser.ParseFreezeDetails(contract)
	if owner == "" {
		r.logger.Debug().
			Str("txHash", req.Event.TransactionID).
			Msg("Freeze operation: could not parse details")
		return
	}

	// Convert hex to base58
	ownerBase58 := parser.HexToBase58(owner)
	watchedAddr := req.Subscription.Address

	// Verify this involves our watched address
	if ownerBase58 != watchedAddr {
		return
	}

	r.logger.Info().
		Str("txHash", req.Event.TransactionID).
		Str("owner", ownerBase58).
		Str("resourceType", resourceType).
		Int64("amount", amount).
		Msg("Stake operation detected")

	event := &webhook.OperationEvent{
		EventType:      "freeze_balance",
		EventID:        fmt.Sprintf("evt_%s_%d", req.Event.TransactionID[:16], time.Now().UnixNano()),
		Timestamp:      time.Now().Unix(),
		Network:        r.network,
		TxHash:         req.Event.TransactionID,
		BlockNumber:    req.Event.BlockNumber,
		BlockTimestamp: req.Event.BlockTimestamp,
		Success:        req.Event.Success,
		OperationType:  "STAKE",
		OwnerAddress:   ownerBase58,
		ResourceType:   resourceType,
		StakeAmount:    amount,
		WalletType:     req.Subscription.WalletType,
		WatchedAddress: watchedAddr,
		SubscriptionID: req.Subscription.SubscriptionID,
		UserID:         req.Subscription.UserID,
		Label:          req.Subscription.Label,
		Metadata:       req.Subscription.Metadata,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := r.portoClient.SendOperationNotification(ctx, event); err != nil {
		r.logger.Error().Err(err).Str("txHash", event.TxHash).Msg("Failed to send stake notification")
	}
}

// handleUnfreezeOperation handles UnfreezeBalanceV2Contract (unstaking)
func (r *EventRouter) handleUnfreezeOperation(req *RouteEventRequest) {
	contract := r.getContractFromEvent(req.Event)
	if contract == nil {
		return
	}

	owner, resourceType, amount := r.tronParser.ParseUnfreezeDetails(contract)
	if owner == "" {
		return
	}

	ownerBase58 := parser.HexToBase58(owner)
	watchedAddr := req.Subscription.Address

	if ownerBase58 != watchedAddr {
		return
	}

	r.logger.Info().
		Str("txHash", req.Event.TransactionID).
		Str("owner", ownerBase58).
		Str("resourceType", resourceType).
		Int64("amount", amount).
		Msg("Unstake operation detected")

	event := &webhook.OperationEvent{
		EventType:      "unfreeze_balance",
		EventID:        fmt.Sprintf("evt_%s_%d", req.Event.TransactionID[:16], time.Now().UnixNano()),
		Timestamp:      time.Now().Unix(),
		Network:        r.network,
		TxHash:         req.Event.TransactionID,
		BlockNumber:    req.Event.BlockNumber,
		BlockTimestamp: req.Event.BlockTimestamp,
		Success:        req.Event.Success,
		OperationType:  "UNSTAKE",
		OwnerAddress:   ownerBase58,
		ResourceType:   resourceType,
		UnstakeAmount:  amount,
		WalletType:     req.Subscription.WalletType,
		WatchedAddress: watchedAddr,
		SubscriptionID: req.Subscription.SubscriptionID,
		UserID:         req.Subscription.UserID,
		Label:          req.Subscription.Label,
		Metadata:       req.Subscription.Metadata,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := r.portoClient.SendOperationNotification(ctx, event); err != nil {
		r.logger.Error().Err(err).Str("txHash", event.TxHash).Msg("Failed to send unstake notification")
	}
}

// handleWithdrawOperation handles WithdrawExpireUnfreezeContract
func (r *EventRouter) handleWithdrawOperation(req *RouteEventRequest) {
	contract := r.getContractFromEvent(req.Event)
	if contract == nil {
		return
	}

	owner := r.tronParser.ParseWithdrawDetails(contract)
	if owner == "" {
		return
	}

	ownerBase58 := parser.HexToBase58(owner)
	watchedAddr := req.Subscription.Address

	if ownerBase58 != watchedAddr {
		return
	}

	r.logger.Info().
		Str("txHash", req.Event.TransactionID).
		Str("owner", ownerBase58).
		Msg("Withdraw unstaked TRX operation detected")

	event := &webhook.OperationEvent{
		EventType:      "withdraw_unstake",
		EventID:        fmt.Sprintf("evt_%s_%d", req.Event.TransactionID[:16], time.Now().UnixNano()),
		Timestamp:      time.Now().Unix(),
		Network:        r.network,
		TxHash:         req.Event.TransactionID,
		BlockNumber:    req.Event.BlockNumber,
		BlockTimestamp: req.Event.BlockTimestamp,
		Success:        req.Event.Success,
		OperationType:  "WITHDRAW",
		OwnerAddress:   ownerBase58,
		WalletType:     req.Subscription.WalletType,
		WatchedAddress: watchedAddr,
		SubscriptionID: req.Subscription.SubscriptionID,
		UserID:         req.Subscription.UserID,
		Label:          req.Subscription.Label,
		Metadata:       req.Subscription.Metadata,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := r.portoClient.SendOperationNotification(ctx, event); err != nil {
		r.logger.Error().Err(err).Str("txHash", event.TxHash).Msg("Failed to send withdraw notification")
	}
}

// handleDelegateOperation handles DelegateResourceContract
func (r *EventRouter) handleDelegateOperation(req *RouteEventRequest) {
	contract := r.getContractFromEvent(req.Event)
	if contract == nil {
		return
	}

	details := r.tronParser.ParseDelegateDetails(contract)
	if details == nil {
		return
	}

	ownerBase58 := parser.HexToBase58(details.Owner)
	receiverBase58 := parser.HexToBase58(details.Receiver)
	watchedAddr := req.Subscription.Address

	// Check if watched address is involved (as delegator or receiver)
	if ownerBase58 != watchedAddr && receiverBase58 != watchedAddr {
		return
	}

	r.logger.Info().
		Str("txHash", req.Event.TransactionID).
		Str("owner", ownerBase58).
		Str("receiver", receiverBase58).
		Str("resourceType", details.ResourceType).
		Int64("amount", details.Amount).
		Msg("Delegate operation detected")

	event := &webhook.OperationEvent{
		EventType:       "delegate_resource",
		EventID:         fmt.Sprintf("evt_%s_%d", req.Event.TransactionID[:16], time.Now().UnixNano()),
		Timestamp:       time.Now().Unix(),
		Network:         r.network,
		TxHash:          req.Event.TransactionID,
		BlockNumber:     req.Event.BlockNumber,
		BlockTimestamp:  req.Event.BlockTimestamp,
		Success:         req.Event.Success,
		OperationType:   "DELEGATE",
		OwnerAddress:    ownerBase58,
		ReceiverAddress: receiverBase58,
		ResourceType:    details.ResourceType,
		ResourceAmount:  details.Amount,
		Lock:            details.Lock,
		LockPeriod:      details.LockPeriod,
		WalletType:      req.Subscription.WalletType,
		WatchedAddress:  watchedAddr,
		SubscriptionID:  req.Subscription.SubscriptionID,
		UserID:          req.Subscription.UserID,
		Label:           req.Subscription.Label,
		Metadata:        req.Subscription.Metadata,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := r.portoClient.SendOperationNotification(ctx, event); err != nil {
		r.logger.Error().Err(err).Str("txHash", event.TxHash).Msg("Failed to send delegate notification")
	}
}

// handleUnDelegateOperation handles UnDelegateResourceContract
func (r *EventRouter) handleUnDelegateOperation(req *RouteEventRequest) {
	contract := r.getContractFromEvent(req.Event)
	if contract == nil {
		return
	}

	details := r.tronParser.ParseUnDelegateDetails(contract)
	if details == nil {
		return
	}

	ownerBase58 := parser.HexToBase58(details.Owner)
	receiverBase58 := parser.HexToBase58(details.Receiver)
	watchedAddr := req.Subscription.Address

	// Check if watched address is involved
	if ownerBase58 != watchedAddr && receiverBase58 != watchedAddr {
		return
	}

	r.logger.Info().
		Str("txHash", req.Event.TransactionID).
		Str("owner", ownerBase58).
		Str("receiver", receiverBase58).
		Str("resourceType", details.ResourceType).
		Int64("amount", details.Amount).
		Msg("Undelegate operation detected")

	event := &webhook.OperationEvent{
		EventType:       "undelegate_resource",
		EventID:         fmt.Sprintf("evt_%s_%d", req.Event.TransactionID[:16], time.Now().UnixNano()),
		Timestamp:       time.Now().Unix(),
		Network:         r.network,
		TxHash:          req.Event.TransactionID,
		BlockNumber:     req.Event.BlockNumber,
		BlockTimestamp:  req.Event.BlockTimestamp,
		Success:         req.Event.Success,
		OperationType:   "UNDELEGATE",
		OwnerAddress:    ownerBase58,
		ReceiverAddress: receiverBase58,
		ResourceType:    details.ResourceType,
		ResourceAmount:  details.Amount,
		WalletType:      req.Subscription.WalletType,
		WatchedAddress:  watchedAddr,
		SubscriptionID:  req.Subscription.SubscriptionID,
		UserID:          req.Subscription.UserID,
		Label:           req.Subscription.Label,
		Metadata:        req.Subscription.Metadata,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := r.portoClient.SendOperationNotification(ctx, event); err != nil {
		r.logger.Error().Err(err).Str("txHash", event.TxHash).Msg("Failed to send undelegate notification")
	}
}

// handleVoteOperation handles VoteWitnessContract
func (r *EventRouter) handleVoteOperation(req *RouteEventRequest) {
	contract := r.getContractFromEvent(req.Event)
	if contract == nil {
		return
	}

	details := r.tronParser.ParseVoteDetails(contract)
	if details == nil {
		return
	}

	ownerBase58 := parser.HexToBase58(details.Owner)
	watchedAddr := req.Subscription.Address

	if ownerBase58 != watchedAddr {
		return
	}

	r.logger.Info().
		Str("txHash", req.Event.TransactionID).
		Str("owner", ownerBase58).
		Int64("totalVotes", details.TotalVotes).
		Int("voteCount", len(details.Votes)).
		Msg("Vote operation detected")

	// Convert vote entries
	votes := make([]webhook.VoteEntry, 0, len(details.Votes))
	for _, v := range details.Votes {
		votes = append(votes, webhook.VoteEntry{
			SRAddress: parser.HexToBase58(v.SRAddress),
			VoteCount: v.VoteCount,
		})
	}

	event := &webhook.OperationEvent{
		EventType:      "vote_witness",
		EventID:        fmt.Sprintf("evt_%s_%d", req.Event.TransactionID[:16], time.Now().UnixNano()),
		Timestamp:      time.Now().Unix(),
		Network:        r.network,
		TxHash:         req.Event.TransactionID,
		BlockNumber:    req.Event.BlockNumber,
		BlockTimestamp: req.Event.BlockTimestamp,
		Success:        req.Event.Success,
		OperationType:  "VOTE",
		OwnerAddress:   ownerBase58,
		Votes:          votes,
		TotalVotes:     details.TotalVotes,
		WalletType:     req.Subscription.WalletType,
		WatchedAddress: watchedAddr,
		SubscriptionID: req.Subscription.SubscriptionID,
		UserID:         req.Subscription.UserID,
		Label:          req.Subscription.Label,
		Metadata:       req.Subscription.Metadata,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := r.portoClient.SendOperationNotification(ctx, event); err != nil {
		r.logger.Error().Err(err).Str("txHash", event.TxHash).Msg("Failed to send vote notification")
	}
}

// handlePermissionOperation handles AccountPermissionUpdateContract
// CRITICAL: This triggers security alerts
func (r *EventRouter) handlePermissionOperation(req *RouteEventRequest) {
	contract := r.getContractFromEvent(req.Event)
	if contract == nil {
		return
	}

	details := r.tronParser.ParsePermissionDetails(contract)
	if details == nil {
		return
	}

	ownerBase58 := parser.HexToBase58(details.Owner)
	watchedAddr := req.Subscription.Address

	if ownerBase58 != watchedAddr {
		return
	}

	// CRITICAL LOG - Permission change on watched wallet
	r.logger.Warn().
		Str("txHash", req.Event.TransactionID).
		Str("owner", ownerBase58).
		Str("walletType", req.Subscription.WalletType).
		Msg("⚠️ CRITICAL: Permission change detected on watched wallet!")

	// Convert permission details
	var permChanges *webhook.PermissionChangeInfo
	if details.OwnerPermission != nil || len(details.ActivePermission) > 0 {
		permChanges = &webhook.PermissionChangeInfo{}

		if details.OwnerPermission != nil {
			permChanges.OwnerPermission = &webhook.PermissionInfo{
				Name:      details.OwnerPermission.Name,
				Threshold: details.OwnerPermission.Threshold,
				Keys:      make([]webhook.KeyInfo, 0, len(details.OwnerPermission.Keys)),
			}
			for _, k := range details.OwnerPermission.Keys {
				permChanges.OwnerPermission.Keys = append(permChanges.OwnerPermission.Keys, webhook.KeyInfo{
					Address: parser.HexToBase58(k.Address),
					Weight:  k.Weight,
				})
			}
		}

		for _, active := range details.ActivePermission {
			permInfo := &webhook.PermissionInfo{
				Name:      active.Name,
				Threshold: active.Threshold,
				Keys:      make([]webhook.KeyInfo, 0, len(active.Keys)),
			}
			for _, k := range active.Keys {
				permInfo.Keys = append(permInfo.Keys, webhook.KeyInfo{
					Address: parser.HexToBase58(k.Address),
					Weight:  k.Weight,
				})
			}
			permChanges.ActivePermission = append(permChanges.ActivePermission, permInfo)
		}
	}

	event := &webhook.OperationEvent{
		EventType:         "permission_update",
		EventID:           fmt.Sprintf("evt_%s_%d", req.Event.TransactionID[:16], time.Now().UnixNano()),
		Timestamp:         time.Now().Unix(),
		Network:           r.network,
		TxHash:            req.Event.TransactionID,
		BlockNumber:       req.Event.BlockNumber,
		BlockTimestamp:    req.Event.BlockTimestamp,
		Success:           req.Event.Success,
		OperationType:     "PERMISSION",
		OwnerAddress:      ownerBase58,
		PermissionChanges: permChanges,
		Priority:          "HIGH", // Mark as high priority for immediate alerting
		WalletType:        req.Subscription.WalletType,
		WatchedAddress:    watchedAddr,
		SubscriptionID:    req.Subscription.SubscriptionID,
		UserID:            req.Subscription.UserID,
		Label:             req.Subscription.Label,
		Metadata:          req.Subscription.Metadata,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := r.portoClient.SendOperationNotification(ctx, event); err != nil {
		r.logger.Error().Err(err).Str("txHash", event.TxHash).Msg("CRITICAL: Failed to send permission change notification!")
	} else {
		r.logger.Info().Str("txHash", event.TxHash).Msg("Permission change notification sent to Porto API")
	}
}

// handleClaimRewardsOperation handles WithdrawBalanceContract (claim voting rewards)
func (r *EventRouter) handleClaimRewardsOperation(req *RouteEventRequest) {
	contract := r.getContractFromEvent(req.Event)
	if contract == nil {
		return
	}

	owner := r.tronParser.ParseClaimRewardsDetails(contract)
	if owner == "" {
		return
	}

	ownerBase58 := parser.HexToBase58(owner)
	watchedAddr := req.Subscription.Address

	if ownerBase58 != watchedAddr {
		return
	}

	// Extract claimed amount from transaction result (not in contract params)
	var claimedAmount int64
	if req.Event.RawTxInfo != nil {
		claimedAmount = req.Event.RawTxInfo.GetWithdrawAmount()
	}

	r.logger.Info().
		Str("txHash", req.Event.TransactionID).
		Str("owner", ownerBase58).
		Int64("claimedAmount", claimedAmount).
		Msg("Claim rewards operation detected")

	event := &webhook.OperationEvent{
		EventType:      "claim_rewards",
		EventID:        fmt.Sprintf("evt_%s_%d", req.Event.TransactionID[:16], time.Now().UnixNano()),
		Timestamp:      time.Now().Unix(),
		Network:        r.network,
		TxHash:         req.Event.TransactionID,
		BlockNumber:    req.Event.BlockNumber,
		BlockTimestamp: req.Event.BlockTimestamp,
		Success:        req.Event.Success,
		OperationType:  "CLAIM",
		OwnerAddress:   ownerBase58,
		ResourceAmount: claimedAmount, // Claimed TRX amount in SUN
		WalletType:     req.Subscription.WalletType,
		WatchedAddress: watchedAddr,
		SubscriptionID: req.Subscription.SubscriptionID,
		UserID:         req.Subscription.UserID,
		Label:          req.Subscription.Label,
		Metadata:       req.Subscription.Metadata,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := r.portoClient.SendOperationNotification(ctx, event); err != nil {
		r.logger.Error().Err(err).Str("txHash", event.TxHash).Msg("Failed to send claim rewards notification")
	} else {
		r.logger.Info().
			Str("txHash", event.TxHash).
			Int64("claimedAmount", claimedAmount).
			Msg("Claim rewards notification sent to Porto API")
	}
}

// getTokenInfo returns token symbol and decimals for known contracts
func (r *EventRouter) getTokenInfo(contractAddress string) (string, int) {
	if r.trc20Parser.IsUSDTContract(contractAddress) {
		return "USDT", 6
	}
	return "TRC20", 18 // Default
}

// formatAmount formats raw amount string with decimals
func (r *EventRouter) formatAmount(amountStr string, decimals int) string {
	// Simple formatting - for production use big.Int
	if amountStr == "" {
		return "0"
	}
	// Add decimal point
	if len(amountStr) <= decimals {
		return "0." + fmt.Sprintf("%0*s", decimals, amountStr)
	}
	pos := len(amountStr) - decimals
	return amountStr[:pos] + "." + amountStr[pos:]
}

// sendToWebSocketClients sends event to all WebSocket clients subscribed to this subscription
func (r *EventRouter) sendToWebSocketClients(subscriptionID string, eventData []byte) {
	r.mu.RLock()
	clients := r.wsClients[subscriptionID]
	r.mu.RUnlock()

	if len(clients) == 0 {
		return
	}

	r.logger.Debug().
		Str("subscriptionId", subscriptionID).
		Int("clientCount", len(clients)).
		Msg("Sending event to WebSocket clients")

	for _, client := range clients {
		select {
		case client.SendChan <- eventData:
			// Successfully queued
		default:
			// Client's send buffer is full, skip
			r.logger.Warn().
				Str("clientId", client.ID).
				Str("subscriptionId", subscriptionID).
				Msg("Client send buffer full, dropping event")
		}
	}
}

// sendToWebhook sends event to webhook URL
func (r *EventRouter) sendToWebhook(sub *models.Subscription, eventData []byte) {
	maxRetries := 3
	retryDelay := time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest("POST", sub.WebhookURL, bytes.NewReader(eventData))
		if err != nil {
			r.logger.Error().
				Err(err).
				Str("subscriptionId", sub.SubscriptionID).
				Msg("Failed to create webhook request")
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Subscription-ID", sub.SubscriptionID)
		req.Header.Set("X-MongoTron-Event", "address.transaction")

		resp, err := r.webhookClient.Do(req)
		if err != nil {
			r.logger.Warn().
				Err(err).
				Str("subscriptionId", sub.SubscriptionID).
				Int("attempt", attempt).
				Msg("Webhook delivery failed")

			if attempt < maxRetries {
				time.Sleep(retryDelay)
				retryDelay *= 2 // Exponential backoff
				continue
			}
			return
		}

		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			r.logger.Debug().
				Str("subscriptionId", sub.SubscriptionID).
				Int("statusCode", resp.StatusCode).
				Msg("Webhook delivered successfully")
			return
		}

		r.logger.Warn().
			Str("subscriptionId", sub.SubscriptionID).
			Int("statusCode", resp.StatusCode).
			Int("attempt", attempt).
			Msg("Webhook returned non-2xx status")

		if attempt < maxRetries {
			time.Sleep(retryDelay)
			retryDelay *= 2
		}
	}
}

// storeEvent stores event in database
func (r *EventRouter) storeEvent(req *RouteEventRequest) {
	event := &models.Event{
		EventID:        fmt.Sprintf("evt_%s_%d", req.Event.TransactionID[:16], time.Now().UnixNano()),
		Network:        "tron-nile", // TODO: Make configurable
		Type:           req.Event.ContractType,
		Address:        req.Subscription.Address,
		TxHash:         req.Event.TransactionID,
		BlockNumber:    req.Event.BlockNumber,
		BlockTimestamp: req.Event.BlockTimestamp,
		Data: map[string]interface{}{
			"from":      req.Event.From,
			"to":        req.Event.To,
			"amount":    req.Event.Amount,
			"asset":     req.Event.AssetName,
			"success":   req.Event.Success,
			"eventType": req.Event.EventType,
			"eventData": req.Event.EventData,
		},
		SubscriptionID: req.Subscription.SubscriptionID,
		Processed:      false,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := r.db.EventRepo.Create(ctx, event); err != nil {
		r.logger.Error().
			Err(err).
			Str("subscriptionId", req.Subscription.SubscriptionID).
			Str("txHash", req.Event.TransactionID).
			Msg("Failed to store event")
	}
}

// RegisterClient registers a WebSocket client for a subscription
func (r *EventRouter) RegisterClient(subscriptionID string, client *WebSocketClient) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.wsClients[subscriptionID] = append(r.wsClients[subscriptionID], client)

	r.logger.Info().
		Str("subscriptionId", subscriptionID).
		Str("clientId", client.ID).
		Int("totalClients", len(r.wsClients[subscriptionID])).
		Msg("WebSocket client registered")
}

// UnregisterClient unregisters a WebSocket client
func (r *EventRouter) UnregisterClient(subscriptionID string, clientID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	clients := r.wsClients[subscriptionID]
	for i, client := range clients {
		if client.ID == clientID {
			// Remove client from slice
			r.wsClients[subscriptionID] = append(clients[:i], clients[i+1:]...)

			// Safely close the channel only if not already closed
			client.mu.Lock()
			if !client.closed {
				close(client.SendChan)
				client.closed = true
			}
			client.mu.Unlock()

			r.logger.Info().
				Str("subscriptionId", subscriptionID).
				Str("clientId", clientID).
				Int("remainingClients", len(r.wsClients[subscriptionID])).
				Msg("WebSocket client unregistered")

			// Clean up empty subscription entries
			if len(r.wsClients[subscriptionID]) == 0 {
				delete(r.wsClients, subscriptionID)
			}
			break
		}
	}
}

// GetClientCount returns the number of connected clients for a subscription
func (r *EventRouter) GetClientCount(subscriptionID string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.wsClients[subscriptionID])
}

// GetTotalClientCount returns the total number of connected clients
func (r *EventRouter) GetTotalClientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	total := 0
	for _, clients := range r.wsClients {
		total += len(clients)
	}
	return total
}
