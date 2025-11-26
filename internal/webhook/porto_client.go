package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/frstrtr/mongotron/internal/blockchain/parser"
	"github.com/frstrtr/mongotron/pkg/logger"
)

// PortoAPIClient handles webhook callbacks to Porto API
type PortoAPIClient struct {
	baseURL       string
	webhookPath   string
	webhookSecret string
	network       string
	httpClient    *http.Client
	logger        *logger.Logger
}

// TransferEvent represents any type of transfer event for Porto API
// Unified structure for TRX, TRC10, and TRC20 transfers
type TransferEvent struct {
	EventType string `json:"eventType"` // "trx_transfer", "trc10_transfer", "trc20_transfer"
	EventID   string `json:"eventId"`   // Unique event identifier
	Timestamp int64  `json:"timestamp"` // Unix timestamp
	Network   string `json:"network"`   // "tron-mainnet" or "tron-nile"

	// Transaction details
	TxHash         string `json:"txHash"`
	BlockNumber    int64  `json:"blockNumber"`
	BlockTimestamp int64  `json:"blockTimestamp"`
	Success        bool   `json:"success"`

	// Asset identification
	AssetType   string `json:"assetType"`         // "TRX", "TRC10", "TRC20"
	AssetID     string `json:"assetId,omitempty"` // Token ID for TRC10, contract address for TRC20
	AssetSymbol string `json:"assetSymbol"`       // "TRX", "BTT", "USDT", etc.
	Decimals    int    `json:"decimals"`          // 6 for TRX/USDT, varies for others

	// Addresses
	From string `json:"from"` // Sender address (base58)
	To   string `json:"to"`   // Recipient address (base58)

	// Amount
	Amount        string `json:"amount"`        // Raw amount in smallest unit
	AmountDecimal string `json:"amountDecimal"` // Human-readable amount

	// Wallet classification (from subscription registration)
	WalletType     string `json:"walletType"`       // "platform", "nps", "portal", "exchange", "general"
	Direction      string `json:"direction"`        // "incoming" or "outgoing"
	WatchedAddress string `json:"watchedAddress"`   // The wallet address that triggered this
	SubscriptionID string `json:"subscriptionId"`   // MongoTron subscription ID
	UserID         string `json:"userId,omitempty"` // User identifier (telegram_id, etc.)
	Label          string `json:"label,omitempty"`  // Address label

	// Additional metadata from subscription
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// PortoTransferEvent is an alias for backward compatibility
type PortoTransferEvent = TransferEvent

// OperationEvent represents a gas station operation event for Porto API
// Used for staking, delegation, voting, and permission changes
type OperationEvent struct {
	EventType string `json:"eventType"` // "freeze_balance", "delegate_resource", "vote_witness", "permission_update"
	EventID   string `json:"eventId"`   // Unique event identifier
	Timestamp int64  `json:"timestamp"` // Unix timestamp
	Network   string `json:"network"`   // "tron-mainnet" or "tron-nile"

	// Transaction details
	TxHash         string `json:"txHash"`
	BlockNumber    int64  `json:"blockNumber"`
	BlockTimestamp int64  `json:"blockTimestamp"`
	Success        bool   `json:"success"`

	// Operation identification
	OperationType string `json:"operationType"` // "STAKE", "UNSTAKE", "DELEGATE", "UNDELEGATE", "VOTE", "PERMISSION"
	OwnerAddress  string `json:"ownerAddress"`  // Who performed the operation

	// For delegation operations
	ReceiverAddress string `json:"receiverAddress,omitempty"` // Delegation target
	ResourceType    string `json:"resourceType,omitempty"`    // "ENERGY" or "BANDWIDTH"
	ResourceAmount  int64  `json:"resourceAmount,omitempty"`  // Amount in SUN
	Lock            bool   `json:"lock,omitempty"`            // Whether delegation is locked
	LockPeriod      int64  `json:"lockPeriod,omitempty"`      // Lock duration

	// For staking operations
	StakeAmount   int64 `json:"stakeAmount,omitempty"`   // Amount staked (SUN)
	UnstakeAmount int64 `json:"unstakeAmount,omitempty"` // Amount unstaked (SUN)

	// For voting operations
	Votes      []VoteEntry `json:"votes,omitempty"`
	TotalVotes int64       `json:"totalVotes,omitempty"`

	// For permission operations (CRITICAL)
	PermissionChanges *PermissionChangeInfo `json:"permissionChanges,omitempty"`
	Priority          string                `json:"priority,omitempty"` // "HIGH" for permission changes

	// Wallet classification (from subscription registration)
	WalletType     string `json:"walletType"`     // "gasstation", "nps", etc.
	WatchedAddress string `json:"watchedAddress"` // The wallet that triggered this
	SubscriptionID string `json:"subscriptionId"` // MongoTron subscription ID
	UserID         string `json:"userId,omitempty"`
	Label          string `json:"label,omitempty"`

	// Additional metadata from subscription
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// VoteEntry represents a single vote for an SR
type VoteEntry struct {
	SRAddress string `json:"srAddress"`
	VoteCount int64  `json:"voteCount"`
}

// PermissionChangeInfo contains permission change details for security alerts
type PermissionChangeInfo struct {
	OwnerPermission  *PermissionInfo   `json:"ownerPermission,omitempty"`
	ActivePermission []*PermissionInfo `json:"activePermission,omitempty"`
}

// PermissionInfo contains permission details
type PermissionInfo struct {
	Name      string    `json:"name"`
	Threshold int64     `json:"threshold"`
	Keys      []KeyInfo `json:"keys"`
}

// KeyInfo contains key/signer details
type KeyInfo struct {
	Address string `json:"address"`
	Weight  int64  `json:"weight"`
}

// NewPortoAPIClient creates a new Porto API webhook client
func NewPortoAPIClient(baseURL, webhookPath, webhookSecret, network string, log *logger.Logger) *PortoAPIClient {
	if log == nil {
		defaultLog := logger.NewDefault()
		log = &defaultLog
	}

	// Default webhook path if not specified
	if webhookPath == "" {
		webhookPath = "/v1/webhooks/mongotron/transfer"
	}

	return &PortoAPIClient{
		baseURL:       baseURL,
		webhookPath:   webhookPath,
		webhookSecret: webhookSecret,
		network:       network,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: log,
	}
}

// SendTransferNotification sends a TRC20 transfer notification to Porto API
func (c *PortoAPIClient) SendTransferNotification(ctx context.Context, event *PortoTransferEvent) error {
	if c.baseURL == "" {
		c.logger.Warn().Msg("Porto API URL not configured, skipping webhook")
		return nil
	}

	// Marshal event to JSON
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Create webhook URL using configured path
	webhookURL := c.baseURL + c.webhookPath

	// Pre-compute signature and headers (before retry loop)
	signature := c.signPayload(payload)
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	subscriptionID := event.SubscriptionID

	// Send request with retries
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		// Create fresh request for each attempt (body reader must be fresh)
		req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		// Set headers
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-MongoTron-Event", "trc20_transfer")
		req.Header.Set("X-MongoTron-Signature", signature)
		req.Header.Set("X-MongoTron-Timestamp", timestamp)
		req.Header.Set("X-Subscription-ID", subscriptionID)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			c.logger.Warn().
				Err(err).
				Int("attempt", attempt).
				Str("url", webhookURL).
				Msg("Webhook delivery failed, retrying...")
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			c.logger.Info().
				Str("eventId", event.EventID).
				Str("txHash", event.TxHash).
				Str("to", event.To).
				Str("amount", event.AmountDecimal).
				Msg("Transfer notification sent to Porto API")
			return nil
		}

		lastErr = fmt.Errorf("webhook returned status %d", resp.StatusCode)
		c.logger.Warn().
			Int("status", resp.StatusCode).
			Int("attempt", attempt).
			Msg("Webhook returned non-2xx status")

		if attempt < 3 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}

	return fmt.Errorf("failed to deliver webhook after 3 attempts: %w", lastErr)
}

// signPayload creates HMAC-SHA256 signature of the payload
func (c *PortoAPIClient) signPayload(payload []byte) string {
	if c.webhookSecret == "" {
		return ""
	}

	h := hmac.New(sha256.New, []byte(c.webhookSecret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// SendOperationNotification sends a gas station operation notification to Porto API
func (c *PortoAPIClient) SendOperationNotification(ctx context.Context, event *OperationEvent) error {
	if c.baseURL == "" {
		c.logger.Warn().Msg("Porto API URL not configured, skipping operation webhook")
		return nil
	}

	// Marshal event to JSON
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal operation event: %w", err)
	}

	// Use operation-specific webhook path
	webhookURL := c.baseURL + "/v1/webhooks/mongotron/operation"

	// Pre-compute signature and headers
	signature := c.signPayload(payload)
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	subscriptionID := event.SubscriptionID

	// Send request with retries
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		// Set headers
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-MongoTron-Event", event.EventType)
		req.Header.Set("X-MongoTron-Operation", event.OperationType)
		req.Header.Set("X-MongoTron-Signature", signature)
		req.Header.Set("X-MongoTron-Timestamp", timestamp)
		req.Header.Set("X-Subscription-ID", subscriptionID)
		if event.Priority != "" {
			req.Header.Set("X-MongoTron-Priority", event.Priority)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			c.logger.Warn().
				Err(err).
				Int("attempt", attempt).
				Str("url", webhookURL).
				Str("operation", event.OperationType).
				Msg("Operation webhook delivery failed, retrying...")
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			c.logger.Info().
				Str("eventId", event.EventID).
				Str("txHash", event.TxHash).
				Str("operation", event.OperationType).
				Str("owner", event.OwnerAddress).
				Msg("Operation notification sent to Porto API")
			return nil
		}

		lastErr = fmt.Errorf("webhook returned status %d", resp.StatusCode)
		c.logger.Warn().
			Int("status", resp.StatusCode).
			Int("attempt", attempt).
			Str("operation", event.OperationType).
			Msg("Operation webhook returned non-2xx status")

		if attempt < 3 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}

	return fmt.Errorf("failed to deliver operation webhook after 3 attempts: %w", lastErr)
}

// CreateTRC20TransferEvent creates a TransferEvent from a TRC20Transfer
func CreateTRC20TransferEvent(
	transfer *parser.TRC20Transfer,
	watchedAddress string,
	subscriptionID string,
	network string,
) *TransferEvent {
	event := &TransferEvent{
		EventType:      "trc20_transfer",
		EventID:        fmt.Sprintf("evt_%s_%d", transfer.TxHash[:16], time.Now().UnixNano()),
		Timestamp:      time.Now().Unix(),
		Network:        network,
		TxHash:         transfer.TxHash,
		BlockNumber:    transfer.BlockNumber,
		BlockTimestamp: transfer.BlockTimestamp,
		Success:        transfer.Success,
		AssetType:      "TRC20",
		AssetID:        transfer.ContractAddress, // Contract address for TRC20
		AssetSymbol:    transfer.TokenSymbol,
		Decimals:       transfer.TokenDecimals,
		From:           transfer.From,
		To:             transfer.To,
		AmountDecimal:  transfer.AmountDecimal,
		WatchedAddress: watchedAddress,
		SubscriptionID: subscriptionID,
	}

	// Set raw amount
	if transfer.Amount != nil {
		event.Amount = transfer.Amount.String()
	}

	// Determine direction based on watched address
	if transfer.To == watchedAddress {
		event.Direction = "incoming"
	} else if transfer.From == watchedAddress {
		event.Direction = "outgoing"
	} else {
		event.Direction = "related" // Address is involved but not sender/receiver
	}

	return event
}

// CreateTransferEvent is an alias for backward compatibility (TRC20)
func CreateTransferEvent(
	transfer *parser.TRC20Transfer,
	watchedAddress string,
	subscriptionID string,
	network string,
) *PortoTransferEvent {
	return CreateTRC20TransferEvent(transfer, watchedAddress, subscriptionID, network)
}

// CreateTRXTransferEvent creates a TransferEvent for native TRX transfers
func CreateTRXTransferEvent(
	txHash string,
	blockNumber int64,
	blockTimestamp int64,
	success bool,
	from string,
	to string,
	amount int64,
	watchedAddress string,
	subscriptionID string,
	network string,
) *TransferEvent {
	// TRX has 6 decimals (SUN)
	amountDecimal := formatTRXAmount(amount)

	event := &TransferEvent{
		EventType:      "trx_transfer",
		EventID:        fmt.Sprintf("evt_%s_%d", txHash[:min(16, len(txHash))], time.Now().UnixNano()),
		Timestamp:      time.Now().Unix(),
		Network:        network,
		TxHash:         txHash,
		BlockNumber:    blockNumber,
		BlockTimestamp: blockTimestamp,
		Success:        success,
		AssetType:      "TRX",
		AssetID:        "", // No asset ID for native TRX
		AssetSymbol:    "TRX",
		Decimals:       6,
		From:           from,
		To:             to,
		Amount:         fmt.Sprintf("%d", amount),
		AmountDecimal:  amountDecimal,
		WatchedAddress: watchedAddress,
		SubscriptionID: subscriptionID,
	}

	// Determine direction
	if to == watchedAddress {
		event.Direction = "incoming"
	} else if from == watchedAddress {
		event.Direction = "outgoing"
	} else {
		event.Direction = "related"
	}

	return event
}

// CreateTRC10TransferEvent creates a TransferEvent for TRC10 token transfers
func CreateTRC10TransferEvent(
	txHash string,
	blockNumber int64,
	blockTimestamp int64,
	success bool,
	from string,
	to string,
	amount int64,
	assetID string,
	assetSymbol string,
	decimals int,
	watchedAddress string,
	subscriptionID string,
	network string,
) *TransferEvent {
	amountDecimal := formatAmountWithDecimals(amount, decimals)

	event := &TransferEvent{
		EventType:      "trc10_transfer",
		EventID:        fmt.Sprintf("evt_%s_%d", txHash[:min(16, len(txHash))], time.Now().UnixNano()),
		Timestamp:      time.Now().Unix(),
		Network:        network,
		TxHash:         txHash,
		BlockNumber:    blockNumber,
		BlockTimestamp: blockTimestamp,
		Success:        success,
		AssetType:      "TRC10",
		AssetID:        assetID,
		AssetSymbol:    assetSymbol,
		Decimals:       decimals,
		From:           from,
		To:             to,
		Amount:         fmt.Sprintf("%d", amount),
		AmountDecimal:  amountDecimal,
		WatchedAddress: watchedAddress,
		SubscriptionID: subscriptionID,
	}

	// Determine direction
	if to == watchedAddress {
		event.Direction = "incoming"
	} else if from == watchedAddress {
		event.Direction = "outgoing"
	} else {
		event.Direction = "related"
	}

	return event
}

// formatTRXAmount formats TRX amount from SUN (6 decimals)
func formatTRXAmount(sunAmount int64) string {
	return formatAmountWithDecimals(sunAmount, 6)
}

// formatAmountWithDecimals formats an amount with the given decimal places
func formatAmountWithDecimals(amount int64, decimals int) string {
	if decimals == 0 {
		return fmt.Sprintf("%d", amount)
	}

	amountStr := fmt.Sprintf("%d", amount)
	if len(amountStr) <= decimals {
		return "0." + fmt.Sprintf("%0*s", decimals, amountStr)
	}
	pos := len(amountStr) - decimals
	return amountStr[:pos] + "." + amountStr[pos:]
}

// min returns the smaller of two ints
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Config holds Porto API client configuration
type Config struct {
	BaseURL       string `json:"baseUrl" yaml:"baseUrl"`
	WebhookSecret string `json:"webhookSecret" yaml:"webhookSecret"`
	Enabled       bool   `json:"enabled" yaml:"enabled"`
}
