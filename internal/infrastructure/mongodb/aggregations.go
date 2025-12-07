package mongodb

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	_ "go.mongodb.org/mongo-driver/mongo"

	"syncvault/internal/domain/entities"
)

// FileStatsByStorage represents file statistics grouped by storage node
type FileStatsByStorage struct {
	StorageNodeID string  `bson:"storageNodeId" json:"storageNodeId"`
	FileCount     int64   `bson:"fileCount" json:"fileCount"`
	AvgDuration   float64 `bson:"avgDuration" json:"avgDuration"`
	TotalBytes    int64   `bson:"totalBytes" json:"totalBytes"`
	SuccessCount  int64   `bson:"successCount" json:"successCount"`
	FailedCount   int64   `bson:"failedCount" json:"failedCount"`
	SuccessRate   float64 `bson:"successRate" json:"successRate"`
}

// FileStatsByPeriod represents file statistics grouped by time periods
type FileStatsByPeriod struct {
	Period       string  `bson:"period" json:"period"`
	FileCount    int64   `bson:"fileCount" json:"fileCount"`
	AvgDuration  float64 `bson:"avgDuration" json:"avgDuration"`
	TotalBytes   int64   `bson:"totalBytes" json:"totalBytes"`
	SuccessCount int64   `bson:"successCount" json:"successCount"`
	FailedCount  int64   `bson:"failedCount" json:"failedCount"`
}

// DetailedFileStats represents comprehensive file statistics
type DetailedFileStats struct {
	TotalFiles      int64                `json:"totalFiles"`
	SuccessfulFiles int64                `json:"successfulFiles"`
	FailedFiles     int64                `json:"failedFiles"`
	SuccessRate     float64              `json:"successRate"`
	AvgDuration     float64              `json:"avgDuration"`
	TotalBytes      int64                `json:"totalBytes"`
	ByStorage       []FileStatsByStorage `json:"byStorage"`
	ByPeriod        []FileStatsByPeriod  `json:"byPeriod"`
}

// GetFileStatsByPeriod returns file statistics for a given time period grouped by storage nodes
func (r *SyncAuditRepository) GetFileStatsByPeriod(ctx context.Context, startTime, endTime time.Time) ([]FileStatsByStorage, error) {
	pipeline := []bson.M{
		// Stage 1: Filter by time range and file-related events
		{
			"$match": bson.M{
				"timestamp": bson.M{
					"$gte": startTime,
					"$lte": endTime,
				},
				"eventType": bson.M{
					"$in": []string{
						entities.AuditEventTypeFileCreated,
						entities.AuditEventTypeFileUpdated,
						entities.AuditEventTypeFileDeleted,
						entities.AuditEventTypeFileCopied,
						entities.AuditEventTypeFileMoved,
					},
				},
			},
		},
		// Stage 2: Group by storage node (source or target)
		{
			"$group": bson.M{
				"_id": bson.M{
					"$switch": bson.M{
						"branches": []bson.M{
							{
								"case": bson.M{"$ne": []interface{}{"$sourceNodeId", nil}},
								"then": "$sourceNodeId",
							},
							{
								"case": bson.M{"$ne": []interface{}{"$targetNodeId", nil}},
								"then": "$targetNodeId",
							},
						},
						"default": "unknown",
					},
				},
				"fileCount":     bson.M{"$sum": 1},
				"totalDuration": bson.M{"$sum": "$duration"},
				"totalBytes":    bson.M{"$sum": "$bytesTransferred"},
				"successCount": bson.M{
					"$sum": bson.M{
						"$cond": bson.M{
							"if":   bson.M{"$eq": []string{"$status", entities.StatusSuccess}},
							"then": 1,
							"else": 0,
						},
					},
				},
				"failedCount": bson.M{
					"$sum": bson.M{
						"$cond": bson.M{
							"if":   bson.M{"$eq": []string{"$status", entities.StatusFailed}},
							"then": 1,
							"else": 0,
						},
					},
				},
			},
		},
		// Stage 3: Calculate average duration and success rate
		{
			"$addFields": bson.M{
				"storageNodeId": "$_id",
				"avgDuration": bson.M{
					"$cond": bson.M{
						"if": bson.M{"$gt": []interface{}{"$fileCount", 0}},
						"then": bson.M{
							"$divide": []interface{}{"$totalDuration", "$fileCount"},
						},
						"else": 0,
					},
				},
				"successRate": bson.M{
					"$cond": bson.M{
						"if": bson.M{"$gt": []interface{}{"$fileCount", 0}},
						"then": bson.M{
							"$multiply": []interface{}{
								bson.M{
									"$divide": []interface{}{"$successCount", "$fileCount"},
								},
								100,
							},
						},
						"else": 0,
					},
				},
			},
		},
		// Stage 4: Project final structure
		{
			"$project": bson.M{
				"_id":           0,
				"storageNodeId": 1,
				"fileCount":     1,
				"avgDuration":   bson.M{"$round": []interface{}{"$avgDuration", 2}},
				"totalBytes":    1,
				"successCount":  1,
				"failedCount":   1,
				"successRate":   bson.M{"$round": []interface{}{"$successRate", 2}},
			},
		},
		// Stage 5: Sort by file count descending
		{
			"$sort": bson.M{"fileCount": -1},
		},
	}

	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate file stats by storage: %w", err)
	}
	defer cursor.Close(ctx)

	var results []FileStatsByStorage
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode file stats results: %w", err)
	}

	return results, nil
}

// GetFileStatsByHourlyPeriod returns file statistics grouped by hour within the time period
func (r *SyncAuditRepository) GetFileStatsByHourlyPeriod(ctx context.Context, startTime, endTime time.Time) ([]FileStatsByPeriod, error) {
	pipeline := []bson.M{
		// Stage 1: Filter by time range and file-related events
		{
			"$match": bson.M{
				"timestamp": bson.M{
					"$gte": startTime,
					"$lte": endTime,
				},
				"eventType": bson.M{
					"$in": []string{
						entities.AuditEventTypeFileCreated,
						entities.AuditEventTypeFileUpdated,
						entities.AuditEventTypeFileDeleted,
						entities.AuditEventTypeFileCopied,
						entities.AuditEventTypeFileMoved,
					},
				},
			},
		},
		// Stage 2: Group by hour
		{
			"$group": bson.M{
				"_id": bson.M{
					"year":  bson.M{"$year": "$timestamp"},
					"month": bson.M{"$month": "$timestamp"},
					"day":   bson.M{"$dayOfMonth": "$timestamp"},
					"hour":  bson.M{"$hour": "$timestamp"},
				},
				"fileCount":     bson.M{"$sum": 1},
				"totalDuration": bson.M{"$sum": "$duration"},
				"totalBytes":    bson.M{"$sum": "$bytesTransferred"},
				"successCount": bson.M{
					"$sum": bson.M{
						"$cond": bson.M{
							"if":   bson.M{"$eq": []string{"$status", entities.StatusSuccess}},
							"then": 1,
							"else": 0,
						},
					},
				},
				"failedCount": bson.M{
					"$sum": bson.M{
						"$cond": bson.M{
							"if":   bson.M{"$eq": []string{"$status", entities.StatusFailed}},
							"then": 1,
							"else": 0,
						},
					},
				},
			},
		},
		// Stage 3: Format period string and calculate averages
		{
			"$addFields": bson.M{
				"period": bson.M{
					"$dateToString": bson.M{
						"format": "%Y-%m-%d %H:00",
						"date":   "$_id",
					},
				},
				"avgDuration": bson.M{
					"$cond": bson.M{
						"if": bson.M{"$gt": []interface{}{"$fileCount", 0}},
						"then": bson.M{
							"$divide": []interface{}{"$totalDuration", "$fileCount"},
						},
						"else": 0,
					},
				},
			},
		},
		// Stage 4: Project final structure
		{
			"$project": bson.M{
				"_id":          0,
				"period":       1,
				"fileCount":    1,
				"avgDuration":  bson.M{"$round": []interface{}{"$avgDuration", 2}},
				"totalBytes":   1,
				"successCount": 1,
				"failedCount":  1,
			},
		},
		// Stage 5: Sort by period
		{
			"$sort": bson.M{"period": 1},
		},
	}

	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate hourly file stats: %w", err)
	}
	defer cursor.Close(ctx)

	var results []FileStatsByPeriod
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode hourly file stats results: %w", err)
	}

	return results, nil
}

// GetFileStatsByDailyPeriod returns file statistics grouped by day within the time period
func (r *SyncAuditRepository) GetFileStatsByDailyPeriod(ctx context.Context, startTime, endTime time.Time) ([]FileStatsByPeriod, error) {
	pipeline := []bson.M{
		// Stage 1: Filter by time range and file-related events
		{
			"$match": bson.M{
				"timestamp": bson.M{
					"$gte": startTime,
					"$lte": endTime,
				},
				"eventType": bson.M{
					"$in": []string{
						entities.AuditEventTypeFileCreated,
						entities.AuditEventTypeFileUpdated,
						entities.AuditEventTypeFileDeleted,
						entities.AuditEventTypeFileCopied,
						entities.AuditEventTypeFileMoved,
					},
				},
			},
		},
		// Stage 2: Group by day
		{
			"$group": bson.M{
				"_id": bson.M{
					"year":  bson.M{"$year": "$timestamp"},
					"month": bson.M{"$month": "$timestamp"},
					"day":   bson.M{"$dayOfMonth": "$timestamp"},
				},
				"fileCount":     bson.M{"$sum": 1},
				"totalDuration": bson.M{"$sum": "$duration"},
				"totalBytes":    bson.M{"$sum": "$bytesTransferred"},
				"successCount": bson.M{
					"$sum": bson.M{
						"$cond": bson.M{
							"if":   bson.M{"$eq": []string{"$status", entities.StatusSuccess}},
							"then": 1,
							"else": 0,
						},
					},
				},
				"failedCount": bson.M{
					"$sum": bson.M{
						"$cond": bson.M{
							"if":   bson.M{"$eq": []string{"$status", entities.StatusFailed}},
							"then": 1,
							"else": 0,
						},
					},
				},
			},
		},
		// Stage 3: Format period string and calculate averages
		{
			"$addFields": bson.M{
				"period": bson.M{
					"$dateToString": bson.M{
						"format": "%Y-%m-%d",
						"date":   "$_id",
					},
				},
				"avgDuration": bson.M{
					"$cond": bson.M{
						"if": bson.M{"$gt": []interface{}{"$fileCount", 0}},
						"then": bson.M{
							"$divide": []interface{}{"$totalDuration", "$fileCount"},
						},
						"else": 0,
					},
				},
			},
		},
		// Stage 4: Project final structure
		{
			"$project": bson.M{
				"_id":          0,
				"period":       1,
				"fileCount":    1,
				"avgDuration":  bson.M{"$round": []interface{}{"$avgDuration", 2}},
				"totalBytes":   1,
				"successCount": 1,
				"failedCount":  1,
			},
		},
		// Stage 5: Sort by period
		{
			"$sort": bson.M{"period": 1},
		},
	}

	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate daily file stats: %w", err)
	}
	defer cursor.Close(ctx)

	var results []FileStatsByPeriod
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode daily file stats results: %w", err)
	}

	return results, nil
}

// GetDetailedFileStats returns comprehensive file statistics for a time period
func (r *SyncAuditRepository) GetDetailedFileStats(ctx context.Context, startTime, endTime time.Time) (*DetailedFileStats, error) {
	// Get stats by storage
	byStorage, err := r.GetFileStatsByPeriod(ctx, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get storage stats: %w", err)
	}

	// Get stats by day
	byPeriod, err := r.GetFileStatsByDailyPeriod(ctx, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get period stats: %w", err)
	}

	// Calculate overall totals
	var totalFiles, successfulFiles, failedFiles int64
	var totalDuration, totalBytes int64

	for _, storage := range byStorage {
		totalFiles += storage.FileCount
		successfulFiles += storage.SuccessCount
		failedFiles += storage.FailedCount
		totalBytes += storage.TotalBytes
	}

	// Get total duration for average calculation
	pipeline := []bson.M{
		{
			"$match": bson.M{
				"timestamp": bson.M{
					"$gte": startTime,
					"$lte": endTime,
				},
				"eventType": bson.M{
					"$in": []string{
						entities.AuditEventTypeFileCreated,
						entities.AuditEventTypeFileUpdated,
						entities.AuditEventTypeFileDeleted,
						entities.AuditEventTypeFileCopied,
						entities.AuditEventTypeFileMoved,
					},
				},
				"duration": bson.M{"$ne": nil},
			},
		},
		{
			"$group": bson.M{
				"_id":           nil,
				"totalDuration": bson.M{"$sum": "$duration"},
				"count":         bson.M{"$sum": 1},
			},
		},
	}

	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate total duration: %w", err)
	}
	defer cursor.Close(ctx)

	var durationResult []bson.M
	if err := cursor.All(ctx, &durationResult); err != nil {
		return nil, fmt.Errorf("failed to decode duration result: %w", err)
	}

	if len(durationResult) > 0 {
		if duration, ok := durationResult[0]["totalDuration"].(int64); ok {
			totalDuration = duration
		}
	}

	// Calculate averages
	var avgDuration float64
	if totalFiles > 0 {
		avgDuration = float64(totalDuration) / float64(totalFiles)
	}

	var successRate float64
	if totalFiles > 0 {
		successRate = float64(successfulFiles) / float64(totalFiles) * 100
	}

	return &DetailedFileStats{
		TotalFiles:      totalFiles,
		SuccessfulFiles: successfulFiles,
		FailedFiles:     failedFiles,
		SuccessRate:     successRate,
		AvgDuration:     avgDuration,
		TotalBytes:      totalBytes,
		ByStorage:       byStorage,
		ByPeriod:        byPeriod,
	}, nil
}

// GetTopFilesByOperation returns the most frequently operated files
func (r *SyncAuditRepository) GetTopFilesByOperation(ctx context.Context, startTime, endTime time.Time, limit int) ([]bson.M, error) {
	pipeline := []bson.M{
		// Stage 1: Filter by time range and file-related events
		{
			"$match": bson.M{
				"timestamp": bson.M{
					"$gte": startTime,
					"$lte": endTime,
				},
				"fileId": bson.M{"$ne": nil},
				"eventType": bson.M{
					"$in": []string{
						entities.AuditEventTypeFileCreated,
						entities.AuditEventTypeFileUpdated,
						entities.AuditEventTypeFileDeleted,
						entities.AuditEventTypeFileCopied,
						entities.AuditEventTypeFileMoved,
					},
				},
			},
		},
		// Stage 2: Group by file
		{
			"$group": bson.M{
				"_id":            "$fileId",
				"fileName":       bson.M{"$first": "$fileName"},
				"filePath":       bson.M{"$first": "$filePath"},
				"operationCount": bson.M{"$sum": 1},
				"totalDuration":  bson.M{"$sum": "$duration"},
				"totalBytes":     bson.M{"$sum": "$bytesTransferred"},
				"successCount": bson.M{
					"$sum": bson.M{
						"$cond": bson.M{
							"if":   bson.M{"$eq": []string{"$status", entities.StatusSuccess}},
							"then": 1,
							"else": 0,
						},
					},
				},
				"failedCount": bson.M{
					"$sum": bson.M{
						"$cond": bson.M{
							"if":   bson.M{"$eq": []string{"$status", entities.StatusFailed}},
							"then": 1,
							"else": 0,
						},
					},
				},
			},
		},
		// Stage 3: Calculate success rate and average duration
		{
			"$addFields": bson.M{
				"successRate": bson.M{
					"$cond": bson.M{
						"if": bson.M{"$gt": []interface{}{"$operationCount", 0}},
						"then": bson.M{
							"$multiply": []interface{}{
								bson.M{
									"$divide": []interface{}{"$successCount", "$operationCount"},
								},
								100,
							},
						},
						"else": 0,
					},
				},
				"avgDuration": bson.M{
					"$cond": bson.M{
						"if": bson.M{"$gt": []interface{}{"$operationCount", 0}},
						"then": bson.M{
							"$divide": []interface{}{"$totalDuration", "$operationCount"},
						},
						"else": 0,
					},
				},
			},
		},
		// Stage 4: Project final structure
		{
			"$project": bson.M{
				"_id":            0,
				"fileId":         "$_id",
				"fileName":       1,
				"filePath":       1,
				"operationCount": 1,
				"avgDuration":    bson.M{"$round": []interface{}{"$avgDuration", 2}},
				"totalBytes":     1,
				"successCount":   1,
				"failedCount":    1,
				"successRate":    bson.M{"$round": []interface{}{"$successRate", 2}},
			},
		},
		// Stage 5: Sort by operation count descending
		{
			"$sort": bson.M{"operationCount": -1},
		},
		// Stage 6: Limit results
		{
			"$limit": limit,
		},
	}

	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate top files: %w", err)
	}
	defer cursor.Close(ctx)

	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode top files results: %w", err)
	}

	return results, nil
}
