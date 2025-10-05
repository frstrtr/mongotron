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

// TransactionRepository handles transaction storage operations
type TransactionRepository struct {
	collection *mongo.Collection
}

// NewTransactionRepository creates a new transaction repository
func NewTransactionRepository(db *mongo.Database) *TransactionRepository {
	return &TransactionRepository{
		collection: db.Collection("transactions"),
	}
}

// Create creates a new transaction record
func (r *TransactionRepository) Create(ctx context.Context, tx *models.Transaction) error {
	if tx.ID.IsZero() {
		tx.ID = primitive.NewObjectID()
	}
	tx.CreatedAt = time.Now()
	tx.UpdatedAt = time.Now()

	_, err := r.collection.InsertOne(ctx, tx)
	if err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	return nil
}

// FindByHash finds a transaction by its hash
func (r *TransactionRepository) FindByHash(ctx context.Context, txHash string) (*models.Transaction, error) {
	var result models.Transaction
	err := r.collection.FindOne(ctx, bson.M{"tx_hash": txHash}).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find transaction: %w", err)
	}

	return &result, nil
}

// FindByID finds a transaction by its ID
func (r *TransactionRepository) FindByID(ctx context.Context, id primitive.ObjectID) (*models.Transaction, error) {
	var result models.Transaction
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find transaction: %w", err)
	}

	return &result, nil
}

// FindByAddress finds all transactions for a specific address
func (r *TransactionRepository) FindByAddress(ctx context.Context, address string, limit, skip int64) ([]*models.Transaction, error) {
	filter := bson.M{
		"$or": []bson.M{
			{"from_address": address},
			{"to_address": address},
		},
	}

	opts := options.Find().
		SetLimit(limit).
		SetSkip(skip).
		SetSort(bson.D{{Key: "block_timestamp", Value: -1}})

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find transactions by address: %w", err)
	}
	defer cursor.Close(ctx)

	var transactions []*models.Transaction
	if err := cursor.All(ctx, &transactions); err != nil {
		return nil, fmt.Errorf("failed to decode transactions: %w", err)
	}

	return transactions, nil
}

// FindByBlockNumber finds all transactions in a specific block
func (r *TransactionRepository) FindByBlockNumber(ctx context.Context, blockNumber int64) ([]*models.Transaction, error) {
	filter := bson.M{"block_number": blockNumber}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to find transactions by block: %w", err)
	}
	defer cursor.Close(ctx)

	var transactions []*models.Transaction
	if err := cursor.All(ctx, &transactions); err != nil {
		return nil, fmt.Errorf("failed to decode transactions: %w", err)
	}

	return transactions, nil
}

// Update updates an existing transaction
func (r *TransactionRepository) Update(ctx context.Context, tx *models.Transaction) error {
	tx.UpdatedAt = time.Now()

	filter := bson.M{"_id": tx.ID}
	update := bson.M{"$set": tx}

	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update transaction: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("transaction not found")
	}

	return nil
}

// Delete deletes a transaction by ID
func (r *TransactionRepository) Delete(ctx context.Context, id primitive.ObjectID) error {
	result, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return fmt.Errorf("failed to delete transaction: %w", err)
	}

	if result.DeletedCount == 0 {
		return fmt.Errorf("transaction not found")
	}

	return nil
}

// List lists all transactions with pagination
func (r *TransactionRepository) List(ctx context.Context, limit, skip int64) ([]*models.Transaction, error) {
	opts := options.Find().
		SetLimit(limit).
		SetSkip(skip).
		SetSort(bson.D{{Key: "block_timestamp", Value: -1}})

	cursor, err := r.collection.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list transactions: %w", err)
	}
	defer cursor.Close(ctx)

	var transactions []*models.Transaction
	if err := cursor.All(ctx, &transactions); err != nil {
		return nil, fmt.Errorf("failed to decode transactions: %w", err)
	}

	return transactions, nil
}

// Count returns the total number of transactions
func (r *TransactionRepository) Count(ctx context.Context) (int64, error) {
	count, err := r.collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return 0, fmt.Errorf("failed to count transactions: %w", err)
	}

	return count, nil
}

// CountByAddress returns the number of transactions for a specific address
func (r *TransactionRepository) CountByAddress(ctx context.Context, address string) (int64, error) {
	filter := bson.M{
		"$or": []bson.M{
			{"from_address": address},
			{"to_address": address},
		},
	}

	count, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to count transactions by address: %w", err)
	}

	return count, nil
}

// CreateIndexes creates necessary indexes for the transactions collection
func (r *TransactionRepository) CreateIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "tx_hash", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "from_address", Value: 1}, {Key: "block_timestamp", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "to_address", Value: 1}, {Key: "block_timestamp", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "network", Value: 1}, {Key: "block_timestamp", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "block_number", Value: -1}},
		},
	}

	_, err := r.collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	return nil
}
