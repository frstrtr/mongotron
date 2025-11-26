package subscription

import "github.com/frstrtr/mongotron/internal/storage/models"

// SubscribeOptions contains options for creating a subscription
type SubscribeOptions struct {
	Address    string
	WebhookURL string
	Filters    models.SubscriptionFilters
	StartBlock int64
	WalletType string                 // "platform", "nps", "portal", "exchange", "general"
	UserID     string                 // telegram_id or user identifier
	Label      string                 // Optional label
	Metadata   map[string]interface{} // Extra data
}

// BatchSubscribeResult contains the result of a batch subscription operation
type BatchSubscribeResult struct {
	Success []*models.Subscription
	Failed  []BatchSubscribeFailure
}

// BatchSubscribeFailure represents a failed subscription in batch operation
type BatchSubscribeFailure struct {
	Address string
	Error   string
}

// ManagerInterface defines the interface for subscription manager operations
// This allows for easier testing with mock implementations
type ManagerInterface interface {
	Start() error
	Stop() error
	Subscribe(address string, webhookURL string, filters models.SubscriptionFilters, startBlock int64) (*models.Subscription, error)
	SubscribeWithOptions(opts SubscribeOptions) (*models.Subscription, error)
	BatchSubscribe(opts []SubscribeOptions) (*BatchSubscribeResult, error)
	Resubscribe(address string, webhookURL string, filters models.SubscriptionFilters, scanGap bool) (*ResubscribeResult, error)
	Unsubscribe(subscriptionID string) error
	GetSubscription(subscriptionID string) (*models.Subscription, error)
	GetByAddress(address string) (*models.Subscription, error)
	List(limit, skip int64) ([]*models.Subscription, int64, error)
	ListSubscriptions(limit, skip int64) ([]*models.Subscription, int64, error)
	GetActiveMonitorsCount() int
	GetEventRouter() *EventRouter
	ScanHistorical(subscriptionID string, fromBlock, toBlock int64) error
}

// Ensure Manager implements ManagerInterface
var _ ManagerInterface = (*Manager)(nil)
