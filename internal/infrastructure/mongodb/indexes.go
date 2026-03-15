package mongodb

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"syncvault/internal/domain/entities"
)

// CreateIndexes creates all necessary indexes for the sync_audit collection
func CreateIndexes(ctx context.Context, db *mongo.Database) error {
	collection := db.Collection("sync_audit")

	// Index models to be created
	indexModels := []mongo.IndexModel{
		// Single field indexes
		{
			Keys:    bson.D{{"timestamp", -1}}, // Descending for recent queries
			Options: options.Index().SetName("timestamp_desc"),
		},
		{
			Keys:    bson.D{{"eventType", 1}},
			Options: options.Index().SetName("event_type"),
		},
		{
			Keys:    bson.D{{"status", 1}},
			Options: options.Index().SetName("status"),
		},
		{
			Keys:    bson.D{{"syncJobId", 1}},
			Options: options.Index().SetName("sync_job_id"),
		},
		{
			Keys:    bson.D{{"fileId", 1}},
			Options: options.Index().SetName("file_id"),
		},
		{
			Keys:    bson.D{{"sourceNodeId", 1}},
			Options: options.Index().SetName("source_node_id"),
		},
		{
			Keys:    bson.D{{"targetNodeId", 1}},
			Options: options.Index().SetName("target_node_id"),
		},
		{
			Keys:    bson.D{{"userId", 1}},
			Options: options.Index().SetName("user_id"),
		},
		{
			Keys:    bson.D{{"error.code", 1}},
			Options: options.Index().SetName("error_code"),
		},

		// Compound indexes for common query patterns
		{
			Keys: bson.D{
				{"eventType", 1},
				{"status", 1},
				{"timestamp", -1},
			},
			Options: options.Index().SetName("event_type_status_timestamp"),
		},
		{
			Keys: bson.D{
				{"syncJobId", 1},
				{"timestamp", -1},
			},
			Options: options.Index().SetName("sync_job_timestamp"),
		},
		{
			Keys: bson.D{
				{"fileId", 1},
				{"timestamp", -1},
			},
			Options: options.Index().SetName("file_id_timestamp"),
		},
		{
			Keys: bson.D{
				{"sourceNodeId", 1},
				{"targetNodeId", 1},
				{"timestamp", -1},
			},
			Options: options.Index().SetName("source_target_timestamp"),
		},
		{
			Keys: bson.D{
				{"userId", 1},
				{"timestamp", -1},
			},
			Options: options.Index().SetName("user_timestamp"),
		},
		{
			Keys: bson.D{
				{"status", 1},
				{"error.retryable", 1},
				{"timestamp", -1},
			},
			Options: options.Index().SetName("status_retryable_timestamp"),
		},

		// TTL index for automatic cleanup of old audit logs
		{
			Keys: bson.D{{"timestamp", 1}},
			Options: options.Index().
				SetName("timestamp_ttl").
				SetExpireAfterSeconds(int32((90 * 24 * time.Hour).Seconds())), // 90 days
		},
	}

	// Create all indexes
	log.Println("Creating indexes for sync_audit collection...")
	
	for i, indexModel := range indexModels {
		name := indexModel.Options.GetName()
		if name == "" {
			// Generate name if not provided
			name = fmt.Sprintf("index_%d", i)
			indexModel.Options.SetName(name)
		}

		log.Printf("Creating index: %s", name)
		
		_, err := collection.Indexes().CreateOne(ctx, indexModel)
		if err != nil {
			// Check if index already exists
			if mongoErr, ok := err.(mongo.CommandError); ok && mongoErr.Code == 48 {
				log.Printf("Index %s already exists, skipping", name)
				continue
			}
			return fmt.Errorf("failed to create index %s: %w", name, err)
		}
		
		log.Printf("✓ Created index: %s", name)
	}

	// Verify indexes were created
	return verifyIndexes(ctx, collection)
}

// verifyIndexes checks that all expected indexes exist
func verifyIndexes(ctx context.Context, collection *mongo.Collection) error {
	log.Println("Verifying created indexes...")
	
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list indexes: %w", err)
	}
	defer cursor.Close(ctx)

	var indexNames []string
	for cursor.Next(ctx) {
		var index bson.M
		if err := cursor.Decode(&index); err != nil {
			log.Printf("Error decoding index: %v", err)
			continue
		}
		
		if name, ok := index["name"].(string); ok {
			indexNames = append(indexNames, name)
		}
	}

	if err := cursor.Err(); err != nil {
		return fmt.Errorf("cursor error: %w", err)
	}

	expectedIndexes := []string{
		"_id_", // Default MongoDB index
		"timestamp_desc",
		"event_type",
		"status",
		"sync_job_id",
		"file_id",
		"source_node_id",
		"target_node_id",
		"user_id",
		"error_code",
		"event_type_status_timestamp",
		"sync_job_timestamp",
		"file_id_timestamp",
		"source_target_timestamp",
		"user_timestamp",
		"status_retryable_timestamp",
		"timestamp_ttl",
	}

	log.Printf("Found %d indexes: %v", len(indexNames), indexNames)
	
	for _, expected := range expectedIndexes {
		found := false
		for _, existing := range indexNames {
			if existing == expected {
				found = true
				break
			}
		}
		if !found {
			log.Printf("⚠️  Expected index not found: %s", expected)
		} else {
			log.Printf("✓ Index verified: %s", expected)
		}
	}

	return nil
}

// DropIndexes drops all custom indexes (keeps only _id_)
func DropIndexes(ctx context.Context, db *mongo.Database) error {
	collection := db.Collection("sync_audit")
	
	log.Println("Dropping custom indexes from sync_audit collection...")
	
	// List all indexes
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list indexes: %w", err)
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var index bson.M
		if err := cursor.Decode(&index); err != nil {
			log.Printf("Error decoding index: %v", err)
			continue
		}
		
		name, ok := index["name"].(string)
		if !ok || name == "_id_" {
			// Skip _id_ index (default MongoDB index)
			continue
		}
		
		log.Printf("Dropping index: %s", name)
		_, err := collection.Indexes().DropOne(ctx, name)
		if err != nil {
			log.Printf("Failed to drop index %s: %v", name, err)
		} else {
			log.Printf("✓ Dropped index: %s", name)
		}
	}

	return cursor.Err()
}

// GetIndexStats returns statistics about the sync_audit collection indexes
func GetIndexStats(ctx context.Context, db *mongo.Database) (*IndexStats, error) {
	collection := db.Collection("sync_audit")
	
	// Get collection stats
	stats := collection.Database().RunCommand(ctx, bson.D{
		{"collStats", "sync_audit"},
		{"indexDetails", true},
	})

	var result bson.M
	if err := stats.Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to get collection stats: %w", err)
	}

	indexStats := &IndexStats{
		CollectionName: "sync_audit",
		IndexCount:    0,
		TotalIndexSize: 0,
		Indexes:       make([]IndexDetail, 0),
	}

	if indexDetails, ok := result["indexDetails"].(bson.M); ok {
		for name, detail := range indexDetails {
			if detailMap, ok := detail.(bson.M); ok {
				indexDetail := IndexDetail{
					Name: name,
				}
				
				if size, ok := detailMap["size"].(int64); ok {
					indexDetail.Size = size
					indexStats.TotalIndexSize += size
				}
				
				indexStats.IndexCount++
				indexStats.Indexes = append(indexStats.Indexes, indexDetail)
			}
		}
	}

	return indexStats, nil
}

// IndexStats represents statistics about collection indexes
type IndexStats struct {
	CollectionName string        `json:"collectionName"`
	IndexCount    int           `json:"indexCount"`
	TotalIndexSize int64         `json:"totalIndexSize"`
	Indexes       []IndexDetail `json:"indexes"`
}

// IndexDetail represents details of a single index
type IndexDetail struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// CreateCollectionWithValidation creates the sync_audit collection with schema validation
func CreateCollectionWithValidation(ctx context.Context, db *mongo.Database) error {
	collectionName := "sync_audit"
	
	// Check if collection already exists
	collections, err := db.ListCollectionNames(ctx, bson.M{"name": collectionName})
	if err != nil {
		return fmt.Errorf("failed to list collections: %w", err)
	}
	
	if len(collections) > 0 {
		log.Printf("Collection %s already exists", collectionName)
		return nil
	}

	// Define JSON Schema for validation
	validator := bson.M{
		"$jsonSchema": bson.M{
			"bsonType": "object",
			"required": []string{
				"timestamp",
				"eventType",
				"status",
			},
			"properties": bson.M{
				"timestamp": bson.M{
					"bsonType": "date",
					"description": "When the audit event occurred",
				},
				"eventType": bson.M{
					"bsonType": "string",
					"enum": []string{
						entities.AuditEventTypeSyncStarted,
						entities.AuditEventTypeSyncCompleted,
						entities.AuditEventTypeSyncFailed,
						entities.AuditEventTypeSyncRetried,
						entities.AuditEventTypeConflictDetected,
						entities.AuditEventTypeConflictResolved,
						entities.AuditEventTypeFileCreated,
						entities.AuditEventTypeFileUpdated,
						entities.AuditEventTypeFileDeleted,
						entities.AuditEventTypeFileCopied,
						entities.AuditEventTypeFileMoved,
					},
					"description": "Type of audit event",
				},
				"status": bson.M{
					"bsonType": "string",
					"enum": []string{
						entities.StatusSuccess,
						entities.StatusFailed,
						entities.StatusPending,
						entities.StatusRetrying,
						entities.StatusCancelled,
						entities.StatusPartial,
					},
					"description": "Status of the operation",
				},
				"syncJobId": bson.M{
					"bsonType": "objectId",
					"description": "ID of the sync job (optional)",
				},
				"fileId": bson.M{
					"bsonType": "objectId",
					"description": "ID of the file (optional)",
				},
				"sourceNodeId": bson.M{
					"bsonType": "objectId",
					"description": "ID of the source node (optional)",
				},
				"targetNodeId": bson.M{
					"bsonType": "objectId",
					"description": "ID of the target node (optional)",
				},
				"duration": bson.M{
					"bsonType": "long",
					"minimum": 0,
					"description": "Duration in milliseconds (optional)",
				},
				"bytesTransferred": bson.M{
					"bsonType": "long",
					"minimum": 0,
					"description": "Number of bytes transferred (optional)",
				},
				"retryCount": bson.M{
					"bsonType": "int",
					"minimum": 0,
					"description": "Number of retry attempts (optional)",
				},
			},
		},
	}

	// Create collection with validation
	opts := options.CreateCollection().
		SetValidator(validator).
		SetValidationLevel("strict").
		SetValidationAction("error")

	err = db.CreateCollection(ctx, collectionName, opts)
	if err != nil {
		return fmt.Errorf("failed to create collection %s: %w", collectionName, err)
	}

	log.Printf("✓ Created collection %s with schema validation", collectionName)
	return nil
}
