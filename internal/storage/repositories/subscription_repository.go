package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/frstrtr/mongotron/internal/storage/models"
	"github.com/frstrtr/mongotron/pkg/logger"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SubscriptionRepository handles subscription data operations
type SubscriptionRepository struct {
	collection *mongo.Collection
	logger     *logger.Logger
}

// NewSubscriptionRepository creates a new subscription repository
func NewSubscriptionRepository(db *mongo.Database, log *logger.Logger) *SubscriptionRepository {
	if log == nil {
		defaultLog := logger.NewDefault()
		log = &defaultLog
	}

	collection := db.Collection("subscriptions")

	return &SubscriptionRepository{
		collection: collection,
		logger:     log,
	}
}

// Create creates a new subscription
func (r *SubscriptionRepository) Create(ctx context.Context, subscription *models.Subscription) error {
	now := time.Now()
	subscription.CreatedAt = now
	subscription.UpdatedAt = now

	result, err := r.collection.InsertOne(ctx, subscription)
	if err != nil {
		return fmt.Errorf("failed to create subscription: %w", err)
	}

	subscription.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

// FindByID finds a subscription by ID
func (r *SubscriptionRepository) FindByID(ctx context.Context, id primitive.ObjectID) (*models.Subscription, error) {
	var subscription models.Subscription
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&subscription)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("subscription not found")
		}
		return nil, fmt.Errorf("failed to find subscription: %w", err)
	}
	return &subscription, nil
}

// FindBySubscriptionID finds a subscription by subscription ID
func (r *SubscriptionRepository) FindBySubscriptionID(ctx context.Context, subscriptionID string) (*models.Subscription, error) {
	var subscription models.Subscription
	err := r.collection.FindOne(ctx, bson.M{"subscription_id": subscriptionID}).Decode(&subscription)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("subscription not found")
		}
		return nil, fmt.Errorf("failed to find subscription: %w", err)
	}
	return &subscription, nil
}

// FindByAddress finds subscriptions by address
func (r *SubscriptionRepository) FindByAddress(ctx context.Context, address string) ([]*models.Subscription, error) {
	cursor, err := r.collection.Find(ctx, bson.M{"address": address})
	if err != nil {
		return nil, fmt.Errorf("failed to find subscriptions: %w", err)
	}
	defer cursor.Close(ctx)

	var subscriptions []*models.Subscription
	if err := cursor.All(ctx, &subscriptions); err != nil {
		return nil, fmt.Errorf("failed to decode subscriptions: %w", err)
	}

	return subscriptions, nil
}

// FindActive finds all active subscriptions
func (r *SubscriptionRepository) FindActive(ctx context.Context) ([]*models.Subscription, error) {
	cursor, err := r.collection.Find(ctx, bson.M{"status": "active"})
	if err != nil {
		return nil, fmt.Errorf("failed to find active subscriptions: %w", err)
	}
	defer cursor.Close(ctx)

	var subscriptions []*models.Subscription
	if err := cursor.All(ctx, &subscriptions); err != nil {
		return nil, fmt.Errorf("failed to decode subscriptions: %w", err)
	}

	return subscriptions, nil
}

// List lists all subscriptions with pagination
func (r *SubscriptionRepository) List(ctx context.Context, limit, skip int64) ([]*models.Subscription, int64, error) {
	// Get total count
	total, err := r.collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count subscriptions: %w", err)
	}

	// Get paginated results
	opts := options.Find().
		SetLimit(limit).
		SetSkip(skip).
		SetSort(bson.D{{"created_at", -1}})

	cursor, err := r.collection.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to find subscriptions: %w", err)
	}
	defer cursor.Close(ctx)

	var subscriptions []*models.Subscription
	if err := cursor.All(ctx, &subscriptions); err != nil {
		return nil, 0, fmt.Errorf("failed to decode subscriptions: %w", err)
	}

	return subscriptions, total, nil
}

// Update updates a subscription
func (r *SubscriptionRepository) Update(ctx context.Context, subscription *models.Subscription) error {
	subscription.UpdatedAt = time.Now()

	update := bson.M{
		"$set": subscription,
	}

	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": subscription.ID}, update)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("subscription not found")
	}

	return nil
}

// UpdateStatus updates subscription status
func (r *SubscriptionRepository) UpdateStatus(ctx context.Context, id primitive.ObjectID, status string) error {
	update := bson.M{
		"$set": bson.M{
			"status":     status,
			"updated_at": time.Now(),
		},
	}

	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": id}, update)
	if err != nil {
		return fmt.Errorf("failed to update subscription status: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("subscription not found")
	}

	return nil
}

// IncrementEventsCount increments the events counter
func (r *SubscriptionRepository) IncrementEventsCount(ctx context.Context, subscriptionID string) error {
	now := time.Now()
	update := bson.M{
		"$inc": bson.M{"events_count": 1},
		"$set": bson.M{
			"last_event_at": now,
			"updated_at":    now,
		},
	}

	result, err := r.collection.UpdateOne(ctx, bson.M{"subscription_id": subscriptionID}, update)
	if err != nil {
		return fmt.Errorf("failed to increment events count: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("subscription not found")
	}

	return nil
}

// UpdateCurrentBlock updates the current block number
func (r *SubscriptionRepository) UpdateCurrentBlock(ctx context.Context, subscriptionID string, blockNumber int64) error {
	update := bson.M{
		"$set": bson.M{
			"current_block": blockNumber,
			"updated_at":    time.Now(),
		},
	}

	_, err := r.collection.UpdateOne(ctx, bson.M{"subscription_id": subscriptionID}, update)
	if err != nil {
		return fmt.Errorf("failed to update current block: %w", err)
	}

	return nil
}

// Delete deletes a subscription
func (r *SubscriptionRepository) Delete(ctx context.Context, id primitive.ObjectID) error {
	result, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return fmt.Errorf("failed to delete subscription: %w", err)
	}

	if result.DeletedCount == 0 {
		return fmt.Errorf("subscription not found")
	}

	return nil
}

// CreateIndexes creates necessary indexes for subscriptions collection
func (r *SubscriptionRepository) CreateIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{"subscription_id", 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{"address", 1}},
		},
		{
			Keys: bson.D{{"status", 1}},
		},
		{
			Keys: bson.D{{"created_at", -1}},
		},
	}

	_, err := r.collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	r.logger.Info().Msg("Subscription indexes created successfully")
	return nil
}
