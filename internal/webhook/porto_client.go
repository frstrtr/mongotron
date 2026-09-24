package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/frstrtr/mongotron/internal/blockchain/parser"
	"github.com/frstrtr/mongotron/pkg/logger"
)

// PortoAPIClient handles webhook callbacks to Porto API
type PortoAPIClient struct {
	baseURL       string
	webhookPath   string
	operationPath string
	webhookSecret string
	network       string
	httpClient    *http.Client
	logger        *logger.Logger
	retryDelay    time.Duration // base backoff between attempts (attempt n waits n*retryDelay)
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
	OperationType string `json:"operationType"` // "STAKE", "UNSTAKE", "DELEGATE", "UNDELEGATE", "VOTE", "PERMISSION", "CLAIM"
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

// Webhook path and header constants shared by the Porto client and the
// per-subscription webhook sender.
const (
	// DefaultTransferPath is the Porto API endpoint for transfer events.
	DefaultTransferPath = "/v1/webhooks/mongotron/transfer"
	// DefaultOperationPath is the Porto API endpoint for gas station operation events.
	DefaultOperationPath = "/v1/webhooks/mongotron/operation"
	// PortoWebhookPathPrefix identifies Porto API webhook endpoints. These only accept
	// Porto-shaped, signed payloads (TransferEvent/OperationEvent) delivered by this client.
	PortoWebhookPathPrefix = "/v1/webhooks/mongotron/"

	// HeaderSignature carries hex HMAC-SHA256(secret, body) (v1, kept for compatibility).
	HeaderSignature = "X-MongoTron-Signature"
	// HeaderSignatureV2 carries hex HMAC-SHA256(secret, timestamp + "." + body), which binds
	// the timestamp so a receiver can reject replays of old deliveries.
	HeaderSignatureV2 = "X-MongoTron-Signature-V2"
	// HeaderTimestamp carries the Unix time (seconds) the delivery attempt was signed.
	HeaderTimestamp = "X-MongoTron-Timestamp"

	maxDeliveryAttempts = 3
)

// Sign returns the v1 signature: hex HMAC-SHA256(secret, body).
// It returns "" when secret is empty.
func Sign(secret string, body []byte) string {
	if secret == "" {
		return ""
	}
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// SignV2 returns the v2 signature: hex HMAC-SHA256(secret, timestamp + "." + body).
// It returns "" when secret is empty.
func SignV2(secret, timestamp string, body []byte) string {
	if secret == "" {
		return ""
	}
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(timestamp))
	h.Write([]byte("."))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// SetSignatureHeaders signs body with secret and sets the v1, v2 and timestamp headers.
// With an empty secret no signature headers are set (receivers must reject the request).
func SetSignatureHeaders(h http.Header, secret string, body []byte, now time.Time) {
	if secret == "" {
		return
	}
	ts := strconv.FormatInt(now.Unix(), 10)
	h.Set(HeaderSignature, Sign(secret, body))
	h.Set(HeaderSignatureV2, SignV2(secret, ts, body))
	h.Set(HeaderTimestamp, ts)
}

// IsPortoWebhookURL reports whether rawURL points at a Porto API MongoTron webhook
// endpoint (any path containing /v1/webhooks/mongotron/). Such endpoints are served by
// the signed Porto client, so a raw per-subscription post to them is never accepted.
func IsPortoWebhookURL(rawURL string) bool {
	p := rawURL
	if u, err := url.Parse(strings.TrimSpace(rawURL)); err == nil && u.Path != "" {
		p = u.Path
	}
	return strings.Contains(p, PortoWebhookPathPrefix)
}

// NewPortoAPIClient creates a new Porto API webhook client
func NewPortoAPIClient(baseURL, webhookPath, webhookSecret, network string, log *logger.Logger) *PortoAPIClient {
	if log == nil {
		defaultLog := logger.NewDefault()
		log = &defaultLog
	}

	// Default webhook path if not specified
	if webhookPath == "" {
		webhookPath = DefaultTransferPath
	}

	if webhookSecret == "" {
		log.Error().Msg("Porto webhook secret is empty: deliveries will be UNSIGNED and Porto API will reject them (set webhooks.porto.webhookSecret / MONGOTRON_WEBHOOK_SECRET)")
	}

	return &PortoAPIClient{
		baseURL:       strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		webhookPath:   ensureLeadingSlash(webhookPath),
		operationPath: DefaultOperationPath,
		webhookSecret: webhookSecret,
		network:       network,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger:     log,
		retryDelay: time.Second,
	}
}

// SetOperationPath overrides the path operation events are posted to
// (default /v1/webhooks/mongotron/operation). An empty path keeps the default.
func (c *PortoAPIClient) SetOperationPath(p string) {
	if strings.TrimSpace(p) != "" {
		c.operationPath = ensureLeadingSlash(strings.TrimSpace(p))
	}
}

// TransferURL returns the full URL transfer events are posted to.
func (c *PortoAPIClient) TransferURL() string { return c.baseURL + c.webhookPath }

// OperationURL returns the full URL operation events are posted to.
func (c *PortoAPIClient) OperationURL() string { return c.baseURL + c.operationPath }

func ensureLeadingSlash(p string) string {
	if p != "" && !strings.HasPrefix(p, "/") {
		return "/" + p
	}
	return p
}

// deliver POSTs payload to webhookURL with up to maxDeliveryAttempts attempts.
// Every attempt is signed with a fresh timestamp. 4xx responses other than 408/429
// are not retried: the receiver rejected the request itself and will do so again.
func (c *PortoAPIClient) deliver(ctx context.Context, webhookURL string, payload []byte, headers map[string]string) error {
	var lastErr error
	for attempt := 1; attempt <= maxDeliveryAttempts; attempt++ {
		// Create fresh request for each attempt (body reader must be fresh)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			if v != "" {
				req.Header.Set(k, v)
			}
		}
		SetSignatureHeaders(req.Header, c.webhookSecret, payload, time.Now())

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			c.logger.Warn().
				Err(err).
				Int("attempt", attempt).
				Str("url", webhookURL).
				Msg("Webhook delivery failed, retrying...")
		} else {
			// Drain and close inside the loop (a deferred close would hold every
			// attempt's connection until the whole delivery returns).
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
			resp.Body.Close()

			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			lastErr = fmt.Errorf("webhook returned status %d", resp.StatusCode)
			c.logger.Warn().
				Int("status", resp.StatusCode).
				Int("attempt", attempt).
				Str("url", webhookURL).
				Msg("Webhook returned non-2xx status")
			if resp.StatusCode >= 400 && resp.StatusCode < 500 &&
				resp.StatusCode != http.StatusRequestTimeout && resp.StatusCode != http.StatusTooManyRequests {
				return fmt.Errorf("webhook rejected (not retried): %w", lastErr)
			}
		}

		if attempt < maxDeliveryAttempts {
			select {
			case <-ctx.Done():
				return fmt.Errorf("webhook delivery cancelled: %w (last error: %v)", ctx.Err(), lastErr)
			case <-time.After(time.Duration(attempt) * c.retryDelay):
			}
		}
	}
	return fmt.Errorf("failed to deliver webhook after %d attempts: %w", maxDeliveryAttempts, lastErr)
}

// SendTransferNotification sends a transfer notification (TRX/TRC10/TRC20) to Porto API
func (c *PortoAPIClient) SendTransferNotification(ctx context.Context, event *PortoTransferEvent) error {
	if c.baseURL == "" {
		c.logger.Warn().Msg("Porto API URL not configured, skipping webhook")
		return nil
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	eventHeader := event.EventType
	if eventHeader == "" {
		eventHeader = "trc20_transfer"
	}
	if err := c.deliver(ctx, c.TransferURL(), payload, map[string]string{
		"X-MongoTron-Event": eventHeader,
		"X-Subscription-ID": event.SubscriptionID,
	}); err != nil {
		return err
	}

	c.logger.Info().
		Str("eventId", event.EventID).
		Str("txHash", event.TxHash).
		Str("to", event.To).
		Str("amount", event.AmountDecimal).
		Msg("Transfer notification sent to Porto API")
	return nil
}

// SendOperationNotification sends a gas station operation notification to Porto API
func (c *PortoAPIClient) SendOperationNotification(ctx context.Context, event *OperationEvent) error {
	if c.baseURL == "" {
		c.logger.Warn().Msg("Porto API URL not configured, skipping operation webhook")
		return nil
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal operation event: %w", err)
	}

	if err := c.deliver(ctx, c.OperationURL(), payload, map[string]string{
		"X-MongoTron-Event":     event.EventType,
		"X-MongoTron-Operation": event.OperationType,
		"X-Subscription-ID":     event.SubscriptionID,
		"X-MongoTron-Priority":  event.Priority,
	}); err != nil {
		return fmt.Errorf("operation %s: %w", event.OperationType, err)
	}

	c.logger.Info().
		Str("eventId", event.EventID).
		Str("txHash", event.TxHash).
		Str("operation", event.OperationType).
		Str("owner", event.OwnerAddress).
		Msg("Operation notification sent to Porto API")
	return nil
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
