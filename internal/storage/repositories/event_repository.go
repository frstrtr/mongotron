package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/frstrtr/mongotron/internal/storage/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// EventRepository handles event storage operations
type EventRepository struct {
	collection *mongo.Collection
}

// NewEventRepository creates a new event repository
func NewEventRepository(db *mongo.Database) *EventRepository {
	return &EventRepository{
		collection: db.Collection("events"),
	}
}

// Create creates a new event record
func (r *EventRepository) Create(ctx context.Context, event *models.Event) error {
	if event.ID.IsZero() {
		event.ID = primitive.NewObjectID()
	}
	event.CreatedAt = time.Now()

	// Set TTL expiration (30 days from now)
	if event.ExpiresAt.IsZero() {
		event.ExpiresAt = time.Now().AddDate(0, 0, 30)
	}

	_, err := r.collection.InsertOne(ctx, event)
	if err != nil {
		return fmt.Errorf("failed to create event: %w", err)
	}

	return nil
}

// FindByID finds an event by its ID
func (r *EventRepository) FindByID(ctx context.Context, id primitive.ObjectID) (*models.Event, error) {
	var result models.Event
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find event: %w", err)
	}

	return &result, nil
}

// FindByEventID finds an event by its event ID
func (r *EventRepository) FindByEventID(ctx context.Context, eventID string) (*models.Event, error) {
	var result models.Event
	err := r.collection.FindOne(ctx, bson.M{"event_id": eventID}).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find event: %w", err)
	}

	return &result, nil
}

// FindByAddress finds all events for a specific address
func (r *EventRepository) FindByAddress(ctx context.Context, address string, limit, skip int64) ([]*models.Event, error) {
	filter := bson.M{"address": address}

	opts := options.Find().
		SetLimit(limit).
		SetSkip(skip).
		SetSort(bson.D{{Key: "block_timestamp", Value: -1}})

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find events by address: %w", err)
	}
	defer cursor.Close(ctx)

	var events []*models.Event
	if err := cursor.All(ctx, &events); err != nil {
		return nil, fmt.Errorf("failed to decode events: %w", err)
	}

	return events, nil
}

// FindByTxHash finds all events for a specific transaction
func (r *EventRepository) FindByTxHash(ctx context.Context, txHash string) ([]*models.Event, error) {
	filter := bson.M{"tx_hash": txHash}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to find events by tx hash: %w", err)
	}
	defer cursor.Close(ctx)

	var events []*models.Event
	if err := cursor.All(ctx, &events); err != nil {
		return nil, fmt.Errorf("failed to decode events: %w", err)
	}

	return events, nil
}

// FindByType finds all events of a specific type
func (r *EventRepository) FindByType(ctx context.Context, eventType string, limit, skip int64) ([]*models.Event, error) {
	filter := bson.M{"type": eventType}

	opts := options.Find().
		SetLimit(limit).
		SetSkip(skip).
		SetSort(bson.D{{Key: "block_timestamp", Value: -1}})

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find events by type: %w", err)
	}
	defer cursor.Close(ctx)

	var events []*models.Event
	if err := cursor.All(ctx, &events); err != nil {
		return nil, fmt.Errorf("failed to decode events: %w", err)
	}

	return events, nil
}

// FindUnprocessed finds all unprocessed events
func (r *EventRepository) FindUnprocessed(ctx context.Context, limit int64) ([]*models.Event, error) {
	filter := bson.M{"processed": false}

	opts := options.Find().
		SetLimit(limit).
		SetSort(bson.D{{Key: "created_at", Value: 1}})

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find unprocessed events: %w", err)
	}
	defer cursor.Close(ctx)

	var events []*models.Event
	if err := cursor.All(ctx, &events); err != nil {
		return nil, fmt.Errorf("failed to decode events: %w", err)
	}

	return events, nil
}

// MarkProcessed marks an event as processed
func (r *EventRepository) MarkProcessed(ctx context.Context, id primitive.ObjectID) error {
	filter := bson.M{"_id": id}
	update := bson.M{
		"$set": bson.M{
			"processed": true,
		},
	}

	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to mark event as processed: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("event not found")
	}

	return nil
}

// Update updates an existing event
func (r *EventRepository) Update(ctx context.Context, event *models.Event) error {
	filter := bson.M{"_id": event.ID}
	update := bson.M{"$set": event}

	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update event: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("event not found")
	}

	return nil
}

// Delete deletes an event by ID
func (r *EventRepository) Delete(ctx context.Context, id primitive.ObjectID) error {
	result, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return fmt.Errorf("failed to delete event: %w", err)
	}

	if result.DeletedCount == 0 {
		return fmt.Errorf("event not found")
	}

	return nil
}

// List lists all events with pagination
func (r *EventRepository) List(ctx context.Context, limit, skip int64) ([]*models.Event, error) {
	opts := options.Find().
		SetLimit(limit).
		SetSkip(skip).
		SetSort(bson.D{{Key: "block_timestamp", Value: -1}})

	cursor, err := r.collection.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}
	defer cursor.Close(ctx)

	var events []*models.Event
	if err := cursor.All(ctx, &events); err != nil {
		return nil, fmt.Errorf("failed to decode events: %w", err)
	}

	return events, nil
}

// Count returns the total number of events
func (r *EventRepository) Count(ctx context.Context) (int64, error) {
	count, err := r.collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return 0, fmt.Errorf("failed to count events: %w", err)
	}

	return count, nil
}

// CreateIndexes creates necessary indexes for the events collection
func (r *EventRepository) CreateIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "event_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "address", Value: 1}, {Key: "block_timestamp", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "type", Value: 1}, {Key: "block_timestamp", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "subscription_id", Value: 1}, {Key: "block_timestamp", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "tx_hash", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "processed", Value: 1}, {Key: "created_at", Value: 1}},
		},
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0), // TTL index
		},
	}

	_, err := r.collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	return nil
}
