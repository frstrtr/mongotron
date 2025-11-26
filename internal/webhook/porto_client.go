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

// PortoTransferEvent represents a TRC20 transfer event for Porto API
// This is a general-purpose event structure for all wallet types
type PortoTransferEvent struct {
	EventType string `json:"eventType"` // "trc20_transfer"
	EventID   string `json:"eventId"`   // Unique event identifier
	Timestamp int64  `json:"timestamp"` // Unix timestamp
	Network   string `json:"network"`   // "tron-mainnet" or "tron-nile"

	// Transaction details
	TxHash         string `json:"txHash"`
	BlockNumber    int64  `json:"blockNumber"`
	BlockTimestamp int64  `json:"blockTimestamp"`
	Success        bool   `json:"success"`

	// Transfer details
	ContractAddress string `json:"contractAddress"` // Token contract (e.g., USDT)
	TokenSymbol     string `json:"tokenSymbol"`     // "USDT", "USDC", etc.
	TokenDecimals   int    `json:"tokenDecimals"`   // 6 for USDT

	// Addresses
	From string `json:"from"` // Sender address (base58)
	To   string `json:"to"`   // Recipient address (base58)

	// Amount
	Amount        string `json:"amount"`        // Raw amount in smallest unit
	AmountDecimal string `json:"amountDecimal"` // Human-readable amount

	// Wallet classification
	WalletType     string `json:"walletType"`     // "nps", "portal", "exchange", "general"
	Direction      string `json:"direction"`      // "incoming" or "outgoing"
	WatchedAddress string `json:"watchedAddress"` // The wallet address that triggered this
	SubscriptionID string `json:"subscriptionId"` // MongoTron subscription ID

	// Additional metadata from subscription
	Metadata map[string]interface{} `json:"metadata,omitempty"`
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

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MongoTron-Event", "trc20_transfer")
	req.Header.Set("X-MongoTron-Signature", c.signPayload(payload))
	req.Header.Set("X-MongoTron-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	req.Header.Set("X-Subscription-ID", event.SubscriptionID)

	// Send request with retries
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
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

// CreateTransferEvent creates a PortoTransferEvent from a TRC20Transfer
func CreateTransferEvent(
	transfer *parser.TRC20Transfer,
	watchedAddress string,
	subscriptionID string,
	network string,
) *PortoTransferEvent {
	event := &PortoTransferEvent{
		EventType:       "trc20_transfer",
		EventID:         fmt.Sprintf("evt_%s_%d", transfer.TxHash[:16], time.Now().UnixNano()),
		Timestamp:       time.Now().Unix(),
		Network:         network,
		TxHash:          transfer.TxHash,
		BlockNumber:     transfer.BlockNumber,
		BlockTimestamp:  transfer.BlockTimestamp,
		Success:         transfer.Success,
		ContractAddress: transfer.ContractAddress,
		TokenSymbol:     transfer.TokenSymbol,
		TokenDecimals:   transfer.TokenDecimals,
		From:            transfer.From,
		To:              transfer.To,
		AmountDecimal:   transfer.AmountDecimal,
		WatchedAddress:  watchedAddress,
		SubscriptionID:  subscriptionID,
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

// Config holds Porto API client configuration
type Config struct {
	BaseURL       string `json:"baseUrl" yaml:"baseUrl"`
	WebhookSecret string `json:"webhookSecret" yaml:"webhookSecret"`
	Enabled       bool   `json:"enabled" yaml:"enabled"`
}
