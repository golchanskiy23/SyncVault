package main

import (
	"context"
	"log"
	"time"

	"syncvault/internal/config"
	"syncvault/internal/infrastructure/mongodb"
)

func main() {
	log.Println("Starting MongoDB migration...")

	// Load configuration
	cfg, err := config.LoadFromFile("internal/config/config.yml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Connect to MongoDB
	db, err := mongodb.NewMongoConnection(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer func() {
		if err := mongodb.CloseMongoConnection(context.Background(), db); err != nil {
			log.Printf("Error closing MongoDB connection: %v", err)
		}
	}()

	log.Println("Connected to MongoDB successfully")

	// Create collection with validation
	if err := mongodb.CreateCollectionWithValidation(context.Background(), db); err != nil {
		log.Fatalf("Failed to create collection: %v", err)
	}

	// Create indexes
	if err := mongodb.CreateIndexes(context.Background(), db); err != nil {
		log.Fatalf("Failed to create indexes: %v", err)
	}

	// Get index stats
	stats, err := mongodb.GetIndexStats(context.Background(), db)
	if err != nil {
		log.Printf("Warning: Failed to get index stats: %v", err)
	} else {
		log.Printf("Index stats: %+v", stats)
	}

	// Test TTL index by inserting a test document
	collection := db.Collection("sync_audit")
	testDoc := map[string]interface{}{
		"timestamp": time.Now().Add(-35 * 24 * time.Hour), // 35 days ago
		"eventType": "test_event",
		"status":    "test_status",
	}

	result, err := collection.InsertOne(context.Background(), testDoc)
	if err != nil {
		log.Printf("Warning: Failed to insert test document: %v", err)
	} else {
		log.Printf("Inserted test document with ID: %v", result.InsertedID)

		// Clean up test document
		_, err := collection.DeleteOne(context.Background(), map[string]interface{}{
			"_id": result.InsertedID,
		})
		if err != nil {
			log.Printf("Warning: Failed to delete test document: %v", err)
		} else {
			log.Println("Test document cleaned up successfully")
		}
	}

	log.Println("✓ MongoDB migration completed successfully!")
	log.Printf("Collection: sync_audit")
	log.Printf("Database: %s", cfg.MongoDB.Database)
	log.Printf("TTL Index: 30 days for automatic cleanup")
}
