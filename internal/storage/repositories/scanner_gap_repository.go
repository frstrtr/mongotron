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

// ScannerGapCollection is the collection skipped blocks are recorded in.
const ScannerGapCollection = "scanner_gaps"

// ScannerGapRepository stores the blocks the block scanner skipped.
type ScannerGapRepository struct {
	collection *mongo.Collection
}

// NewScannerGapRepository creates a scanner gap repository.
func NewScannerGapRepository(db *mongo.Database) *ScannerGapRepository {
	return &ScannerGapRepository{collection: db.Collection(ScannerGapCollection)}
}

// Create records a skipped block.
func (r *ScannerGapRepository) Create(ctx context.Context, gap *models.ScannerGap) error {
	if gap.ID.IsZero() {
		gap.ID = primitive.NewObjectID()
	}
	if gap.SkippedAt.IsZero() {
		gap.SkippedAt = time.Now()
	}
	if _, err := r.collection.InsertOne(ctx, gap); err != nil {
		return fmt.Errorf("failed to record scanner gap: %w", err)
	}
	return nil
}

// ListPending returns the gaps neither retried nor abandoned, oldest block first.
func (r *ScannerGapRepository) ListPending(ctx context.Context, limit int64) ([]*models.ScannerGap, error) {
	filter := bson.M{"retried_at": bson.M{"$exists": false}, "abandoned_at": bson.M{"$exists": false}}
	opts := options.Find().SetSort(bson.D{{Key: "block_number", Value: 1}}).SetLimit(limit)
	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list scanner gaps: %w", err)
	}
	defer cursor.Close(ctx)
	var gaps []*models.ScannerGap
	if err := cursor.All(ctx, &gaps); err != nil {
		return nil, fmt.Errorf("failed to decode scanner gaps: %w", err)
	}
	return gaps, nil
}

// MarkRetried records that the gap's block was read again and its events routed.
func (r *ScannerGapRepository) MarkRetried(ctx context.Context, id primitive.ObjectID, attempts int, at time.Time) error {
	_, err := r.collection.UpdateOne(ctx, bson.M{"_id": id},
		bson.M{"$set": bson.M{"retried_at": at, "attempts": attempts}})
	if err != nil {
		return fmt.Errorf("failed to mark scanner gap retried: %w", err)
	}
	return nil
}

// RecordFailure records a failed retry; abandoned marks the gap given up.
func (r *ScannerGapRepository) RecordFailure(ctx context.Context, id primitive.ObjectID, attempts int, lastErr string, abandoned bool, at time.Time) error {
	set := bson.M{"attempts": attempts, "last_error": lastErr}
	if abandoned {
		set["abandoned_at"] = at
	}
	if _, err := r.collection.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set}); err != nil {
		return fmt.Errorf("failed to record scanner gap retry: %w", err)
	}
	return nil
}

// CreateIndexes creates the indexes of the scanner gap collection.
func (r *ScannerGapRepository) CreateIndexes(ctx context.Context) error {
	_, err := r.collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "block_number", Value: 1}}},
		{Keys: bson.D{{Key: "skipped_at", Value: -1}}},
	})
	if err != nil {
		return fmt.Errorf("failed to create scanner gap indexes: %w", err)
	}
	return nil
}
