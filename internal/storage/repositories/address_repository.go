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

// AddressRepository handles address storage operations
type AddressRepository struct {
	collection *mongo.Collection
}

// NewAddressRepository creates a new address repository
func NewAddressRepository(db *mongo.Database) *AddressRepository {
	return &AddressRepository{
		collection: db.Collection("addresses"),
	}
}

// Create creates a new address record
func (r *AddressRepository) Create(ctx context.Context, address *models.Address) error {
	if address.ID.IsZero() {
		address.ID = primitive.NewObjectID()
	}
	address.CreatedAt = time.Now()
	address.UpdatedAt = time.Now()

	_, err := r.collection.InsertOne(ctx, address)
	if err != nil {
		return fmt.Errorf("failed to create address: %w", err)
	}

	return nil
}

// FindByAddress finds an address by its address string
func (r *AddressRepository) FindByAddress(ctx context.Context, address string) (*models.Address, error) {
	var result models.Address
	err := r.collection.FindOne(ctx, bson.M{"address": address}).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find address: %w", err)
	}

	return &result, nil
}

// Update updates an existing address
func (r *AddressRepository) Update(ctx context.Context, address *models.Address) error {
	address.UpdatedAt = time.Now()

	filter := bson.M{"_id": address.ID}
	update := bson.M{"$set": address}

	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update address: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("address not found")
	}

	return nil
}

// UpdateActivity updates the last activity timestamp and increments tx count
func (r *AddressRepository) UpdateActivity(ctx context.Context, address string, timestamp time.Time) error {
	filter := bson.M{"address": address}
	update := bson.M{
		"$set": bson.M{
			"last_activity": timestamp,
			"updated_at":    time.Now(),
		},
		"$inc": bson.M{
			"tx_count": 1,
		},
	}

	opts := options.Update().SetUpsert(true)
	_, err := r.collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to update activity: %w", err)
	}

	return nil
}

// UpdateBalance updates the balance for an address
func (r *AddressRepository) UpdateBalance(ctx context.Context, address string, balance int64) error {
	filter := bson.M{"address": address}
	update := bson.M{
		"$set": bson.M{
			"balance":    balance,
			"updated_at": time.Now(),
		},
	}

	opts := options.Update().SetUpsert(true)
	_, err := r.collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}

	return nil
}

// List lists all addresses with pagination
func (r *AddressRepository) List(ctx context.Context, limit, skip int64) ([]*models.Address, error) {
	opts := options.Find().
		SetLimit(limit).
		SetSkip(skip).
		SetSort(bson.D{{Key: "last_activity", Value: -1}})

	cursor, err := r.collection.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list addresses: %w", err)
	}
	defer cursor.Close(ctx)

	var addresses []*models.Address
	if err := cursor.All(ctx, &addresses); err != nil {
		return nil, fmt.Errorf("failed to decode addresses: %w", err)
	}

	return addresses, nil
}

// Delete deletes an address by ID
func (r *AddressRepository) Delete(ctx context.Context, id primitive.ObjectID) error {
	result, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return fmt.Errorf("failed to delete address: %w", err)
	}

	if result.DeletedCount == 0 {
		return fmt.Errorf("address not found")
	}

	return nil
}

// Count returns the total number of addresses
func (r *AddressRepository) Count(ctx context.Context) (int64, error) {
	count, err := r.collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return 0, fmt.Errorf("failed to count addresses: %w", err)
	}

	return count, nil
}

// CreateIndexes creates necessary indexes for the addresses collection
func (r *AddressRepository) CreateIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "address", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "network", Value: 1}, {Key: "address", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "subscription_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "last_activity", Value: -1}},
		},
	}

	_, err := r.collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	return nil
}
