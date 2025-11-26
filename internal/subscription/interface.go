package subscription

import "github.com/frstrtr/mongotron/internal/storage/models"

// ManagerInterface defines the interface for subscription manager operations
// This allows for easier testing with mock implementations
type ManagerInterface interface {
	Start() error
	Stop() error
	Subscribe(address string, webhookURL string, filters models.SubscriptionFilters, startBlock int64) (*models.Subscription, error)
	Unsubscribe(subscriptionID string) error
	GetSubscription(subscriptionID string) (*models.Subscription, error)
	GetByAddress(address string) (*models.Subscription, error)
	List(limit, skip int64) ([]*models.Subscription, int64, error)
	ListSubscriptions(limit, skip int64) ([]*models.Subscription, int64, error)
	GetActiveMonitorsCount() int
	GetEventRouter() *EventRouter
}

// Ensure Manager implements ManagerInterface
var _ ManagerInterface = (*Manager)(nil)
