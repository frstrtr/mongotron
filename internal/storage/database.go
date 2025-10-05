package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/frstrtr/mongotron/internal/storage/repositories"
	"github.com/frstrtr/mongotron/pkg/logger"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// Database holds database connection and repositories
type Database struct {
	client               *mongo.Client
	db                   *mongo.Database
	logger               *logger.Logger
	AddressRepo          *repositories.AddressRepository
	TransactionRepo      *repositories.TransactionRepository
	EventRepo            *repositories.EventRepository
}

// Config holds database configuration
type Config struct {
	URI            string
	Database       string
	MaxPoolSize    uint64
	MinPoolSize    uint64
	MaxIdleTime    time.Duration
	ConnectTimeout time.Duration
}

// NewDatabase creates a new database connection
func NewDatabase(cfg Config, log *logger.Logger) (*Database, error) {
	if log == nil {
		defaultLog := logger.NewDefault()
		log = &defaultLog
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout)
	defer cancel()

	// Set client options
	clientOptions := options.Client().
		ApplyURI(cfg.URI).
		SetMaxPoolSize(cfg.MaxPoolSize).
		SetMinPoolSize(cfg.MinPoolSize).
		SetMaxConnIdleTime(cfg.MaxIdleTime)

	// Connect to MongoDB
	log.Info().
		Str("database", cfg.Database).
		Msg("Connecting to MongoDB")

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Ping the database
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	log.Info().
		Str("database", cfg.Database).
		Msg("Successfully connected to MongoDB")

	db := client.Database(cfg.Database)

	// Initialize repositories
	database := &Database{
		client:          client,
		db:              db,
		logger:          log,
		AddressRepo:     repositories.NewAddressRepository(db),
		TransactionRepo: repositories.NewTransactionRepository(db),
		EventRepo:       repositories.NewEventRepository(db),
	}

	return database, nil
}

// InitializeIndexes creates all necessary indexes
func (d *Database) InitializeIndexes(ctx context.Context) error {
	d.logger.Info().Msg("Creating database indexes")

	if err := d.AddressRepo.CreateIndexes(ctx); err != nil {
		return fmt.Errorf("failed to create address indexes: %w", err)
	}

	if err := d.TransactionRepo.CreateIndexes(ctx); err != nil {
		return fmt.Errorf("failed to create transaction indexes: %w", err)
	}

	if err := d.EventRepo.CreateIndexes(ctx); err != nil {
		return fmt.Errorf("failed to create event indexes: %w", err)
	}

	d.logger.Info().Msg("Database indexes created successfully")

	return nil
}

// Ping checks the database connection
func (d *Database) Ping(ctx context.Context) error {
	return d.client.Ping(ctx, readpref.Primary())
}

// Close closes the database connection
func (d *Database) Close(ctx context.Context) error {
	d.logger.Info().Msg("Closing MongoDB connection")
	return d.client.Disconnect(ctx)
}

// GetClient returns the MongoDB client
func (d *Database) GetClient() *mongo.Client {
	return d.client
}

// GetDatabase returns the MongoDB database
func (d *Database) GetDatabase() *mongo.Database {
	return d.db
}
