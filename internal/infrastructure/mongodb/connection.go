package mongodb

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"syncvault/internal/config"
)

// NewMongoConnection creates a new MongoDB connection
func NewMongoConnection(cfg *config.Config) (*mongo.Database, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.MongoDB.Timeout)
	defer cancel()

	// Configure client options
	clientOptions := options.Client().
		ApplyURI(cfg.MongoDB.URI).
		SetMaxPoolSize(cfg.MongoDB.MaxPoolSize).
		SetMinPoolSize(cfg.MongoDB.MinPoolSize).
		SetConnectTimeout(cfg.MongoDB.Timeout).
		SetServerSelectionTimeout(cfg.MongoDB.Timeout)

	// Connect to MongoDB
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Ping the database to verify connection
	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	// Return database instance
	db := client.Database(cfg.MongoDB.Database)

	return db, nil
}

// CloseMongoConnection gracefully closes MongoDB connection
func CloseMongoConnection(ctx context.Context, db *mongo.Database) error {
	if db == nil {
		return nil
	}

	// Get the client from the database
	client := db.Client()

	// Disconnect with timeout
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := client.Disconnect(ctx); err != nil {
		return fmt.Errorf("failed to disconnect from MongoDB: %w", err)
	}

	return nil
}
