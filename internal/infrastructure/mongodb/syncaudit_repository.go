package mongodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"syncvault/internal/domain/entities"
	"syncvault/internal/domain/repositories"
)

// SyncAuditRepository implements the SyncAudit repository interface using MongoDB
type SyncAuditRepository struct {
	collection *mongo.Collection
}

// NewSyncAuditRepository creates a new MongoDB sync audit repository
func NewSyncAuditRepository(db *mongo.Database) repositories.SyncAuditRepository {
	return &SyncAuditRepository{
		collection: db.Collection("sync_audit"),
	}
}

// Save saves a sync audit event to MongoDB
func (r *SyncAuditRepository) Save(ctx context.Context, audit *entities.SyncAudit) error {
	if audit == nil {
		return errors.New("audit cannot be nil")
	}

	// Set timestamp if not already set
	if audit.Timestamp.IsZero() {
		audit.Timestamp = time.Now().UTC()
	}

	_, err := r.collection.InsertOne(ctx, audit)
	if err != nil {
		return fmt.Errorf("failed to insert sync audit: %w", err)
	}

	return nil
}

// SaveBatch saves multiple sync audit events in bulk
func (r *SyncAuditRepository) SaveBatch(ctx context.Context, audits []*entities.SyncAudit) error {
	if len(audits) == 0 {
		return nil
	}

	var documents []interface{}
	for _, audit := range audits {
		if audit == nil {
			continue
		}
		// Set timestamp if not already set
		if audit.Timestamp.IsZero() {
			audit.Timestamp = time.Now().UTC()
		}
		documents = append(documents, audit)
	}

	if len(documents) == 0 {
		return nil
	}

	_, err := r.collection.InsertMany(ctx, documents)
	if err != nil {
		return fmt.Errorf("failed to insert sync audit batch: %w", err)
	}

	return nil
}

// FindByID finds a sync audit event by ID
func (r *SyncAuditRepository) FindByID(ctx context.Context, id primitive.ObjectID) (*entities.SyncAudit, error) {
	var audit entities.SyncAudit
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&audit)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("sync audit not found: %s", id.Hex())
		}
		return nil, fmt.Errorf("failed to find sync audit: %w", err)
	}

	return &audit, nil
}

// FindBySyncJobID finds all sync audit events for a specific sync job
func (r *SyncAuditRepository) FindBySyncJobID(ctx context.Context, syncJobID primitive.ObjectID) ([]*entities.SyncAudit, error) {
	filter := bson.M{"syncJobId": syncJobID}
	return r.findWithFilter(ctx, filter)
}

// FindByFileID finds all sync audit events for a specific file
func (r *SyncAuditRepository) FindByFileID(ctx context.Context, fileID primitive.ObjectID) ([]*entities.SyncAudit, error) {
	filter := bson.M{"fileId": fileID}
	return r.findWithFilter(ctx, filter)
}

// FindByTimeRange finds sync audit events within a time range
func (r *SyncAuditRepository) FindByTimeRange(ctx context.Context, startTime, endTime time.Time) ([]*entities.SyncAudit, error) {
	filter := bson.M{
		"timestamp": bson.M{
			"$gte": startTime,
			"$lte": endTime,
		},
	}
	return r.findWithFilter(ctx, filter)
}

// FindByEventType finds sync audit events by event type
func (r *SyncAuditRepository) FindByEventType(ctx context.Context, eventType string) ([]*entities.SyncAudit, error) {
	filter := bson.M{"eventType": eventType}
	return r.findWithFilter(ctx, filter)
}

// FindByStatus finds sync audit events by status
func (r *SyncAuditRepository) FindByStatus(ctx context.Context, status string) ([]*entities.SyncAudit, error) {
	filter := bson.M{"status": status}
	return r.findWithFilter(ctx, filter)
}

// FindFailedEvents finds all failed sync audit events
func (r *SyncAuditRepository) FindFailedEvents(ctx context.Context) ([]*entities.SyncAudit, error) {
	filter := bson.M{
		"status": entities.StatusFailed,
	}
	return r.findWithFilter(ctx, filter)
}

// FindByNode finds sync audit events for a specific node (source or target)
func (r *SyncAuditRepository) FindByNode(ctx context.Context, nodeID primitive.ObjectID) ([]*entities.SyncAudit, error) {
	filter := bson.M{
		"$or": []bson.M{
			{"sourceNodeId": nodeID},
			{"targetNodeId": nodeID},
		},
	}
	return r.findWithFilter(ctx, filter)
}

// FindByUser finds sync audit events for a specific user
func (r *SyncAuditRepository) FindByUser(ctx context.Context, userID string) ([]*entities.SyncAudit, error) {
	filter := bson.M{"userId": userID}
	return r.findWithFilter(ctx, filter)
}

// FindWithError finds sync audit events that have errors
func (r *SyncAuditRepository) FindWithError(ctx context.Context) ([]*entities.SyncAudit, error) {
	filter := bson.M{
		"error": bson.M{"$exists": true},
	}
	return r.findWithFilter(ctx, filter)
}

// FindRecent finds recent sync audit events within the last N hours
func (r *SyncAuditRepository) FindRecent(ctx context.Context, hours int) ([]*entities.SyncAudit, error) {
	since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
	filter := bson.M{
		"timestamp": bson.M{"$gte": since},
	}
	return r.findWithFilter(ctx, filter)
}

// FindWithPagination finds sync audit events with pagination
func (r *SyncAuditRepository) FindWithPagination(ctx context.Context, filter interface{}, page, limit int) ([]*entities.SyncAudit, int64, error) {
	// Count total documents
	var filterDoc bson.M
	if filter != nil {
		// Convert interface{} to bson.M if needed
		if filterMap, ok := filter.(map[string]interface{}); ok {
			filterDoc = bson.M(filterMap)
		} else if filterBson, ok := filter.(bson.M); ok {
			filterDoc = filterBson
		} else {
			filterDoc = bson.M{}
		}
	} else {
		filterDoc = bson.M{}
	}

	total, err := r.collection.CountDocuments(ctx, filterDoc)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count documents: %w", err)
	}

	// Calculate skip
	skip := (page - 1) * limit
	if skip < 0 {
		skip = 0
	}

	// Find with filter and options
	opts := options.Find().
		SetSort(bson.D{{"timestamp", -1}}). // Sort by timestamp descending
		SetSkip(int64(skip)).
		SetLimit(int64(limit))

	cursor, err := r.collection.Find(ctx, filterDoc, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to find documents: %w", err)
	}
	defer cursor.Close(ctx)

	var audits []*entities.SyncAudit
	for cursor.Next(ctx) {
		var audit entities.SyncAudit
		if err := cursor.Decode(&audit); err != nil {
			return nil, 0, fmt.Errorf("failed to decode audit: %w", err)
		}
		audits = append(audits, &audit)
	}

	if err := cursor.Err(); err != nil {
		return nil, 0, fmt.Errorf("cursor error: %w", err)
	}

	return audits, total, nil
}

// GetStats returns statistics about sync audit events
func (r *SyncAuditRepository) GetStats(ctx context.Context) (*repositories.SyncAuditStats, error) {
	pipeline := []bson.M{
		{
			"$group": bson.M{
				"_id":         nil,
				"totalEvents": bson.M{"$sum": 1},
				"successfulEvents": bson.M{
					"$sum": bson.M{
						"$cond": bson.M{
							"if":   bson.M{"$eq": []string{"$status", entities.StatusSuccess}},
							"then": 1,
							"else": 0,
						},
					},
				},
				"failedEvents": bson.M{
					"$sum": bson.M{
						"$cond": bson.M{
							"if":   bson.M{"$eq": []string{"$status", entities.StatusFailed}},
							"then": 1,
							"else": 0,
						},
					},
				},
				"totalBytesTransferred": bson.M{"$sum": "$bytesTransferred"},
				"avgDuration":           bson.M{"$avg": "$duration"},
			},
		},
	}

	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate stats: %w", err)
	}
	defer cursor.Close(ctx)

	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode stats: %w", err)
	}

	stats := &repositories.SyncAuditStats{}
	if len(results) > 0 {
		result := results[0]
		stats.TotalEvents = int(result["totalEvents"].(int64))
		stats.SuccessfulEvents = int(result["successfulEvents"].(int64))
		stats.FailedEvents = int(result["failedEvents"].(int64))

		if totalBytes, ok := result["totalBytesTransferred"]; ok && totalBytes != nil {
			stats.TotalBytesTransferred = totalBytes.(int64)
		}

		if avgDur, ok := result["avgDuration"]; ok && avgDur != nil {
			stats.AverageDuration = avgDur.(int64)
		}
	}

	return stats, nil
}

// CleanupOldEvents removes sync audit events older than the specified duration
func (r *SyncAuditRepository) CleanupOldEvents(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-olderThan)
	filter := bson.M{
		"timestamp": bson.M{"$lt": cutoff},
	}

	result, err := r.collection.DeleteMany(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old events: %w", err)
	}

	return result.DeletedCount, nil
}

// findWithFilter is a helper method to find audits with a filter
func (r *SyncAuditRepository) findWithFilter(ctx context.Context, filter bson.M) ([]*entities.SyncAudit, error) {
	opts := options.Find().SetSort(bson.D{{"timestamp", -1}})
	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find audits: %w", err)
	}
	defer cursor.Close(ctx)

	var audits []*entities.SyncAudit
	for cursor.Next(ctx) {
		var audit entities.SyncAudit
		if err := cursor.Decode(&audit); err != nil {
			return nil, fmt.Errorf("failed to decode audit: %w", err)
		}
		audits = append(audits, &audit)
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	return audits, nil
}
