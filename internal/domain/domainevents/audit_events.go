package domainevents

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"syncvault/internal/domain/entities"
	"syncvault/internal/domain/valueobjects"
)

// AuditEvent represents a domain event for audit logging
type AuditEvent struct {
	ID          string                 `json:"id"`
	EventType   string                 `json:"eventType"`
	Timestamp   time.Time              `json:"timestamp"`
	AggregateID string                 `json:"aggregateId"`
	Data        map[string]interface{} `json:"data"`
	Version     int                    `json:"version"`
}

// SyncJobStartedEvent is raised when a sync job starts
type SyncJobStartedEvent struct {
	AuditEvent
	SyncJobID    valueobjects.SyncJobID     `json:"syncJobId"`
	SourceNodeID valueobjects.StorageNodeID `json:"sourceNodeId"`
	TargetNodeID valueobjects.StorageNodeID `json:"targetNodeId"`
	FileCount    int                       `json:"fileCount"`
	UserID       string                    `json:"userId,omitempty"`
}

// SyncJobCompletedEvent is raised when a sync job completes successfully
type SyncJobCompletedEvent struct {
	AuditEvent
	SyncJobID        valueobjects.SyncJobID     `json:"syncJobId"`
	SourceNodeID     valueobjects.StorageNodeID `json:"sourceNodeId"`
	TargetNodeID     valueobjects.StorageNodeID `json:"targetNodeId"`
	FilesProcessed    int                       `json:"filesProcessed"`
	FilesSkipped     int                       `json:"filesSkipped"`
	BytesTransferred int64                     `json:"bytesTransferred"`
	Duration         time.Duration             `json:"duration"`
	UserID           string                    `json:"userId,omitempty"`
}

// SyncJobFailedEvent is raised when a sync job fails
type SyncJobFailedEvent struct {
	AuditEvent
	SyncJobID    valueobjects.SyncJobID     `json:"syncJobId"`
	SourceNodeID valueobjects.StorageNodeID `json:"sourceNodeId"`
	TargetNodeID valueobjects.StorageNodeID `json:"targetNodeId"`
	ErrorCode    string                    `json:"errorCode"`
	ErrorMessage string                    `json:"errorMessage"`
	Retryable   bool                      `json:"retryable"`
	UserID      string                    `json:"userId,omitempty"`
}

// FileSyncStartedEvent is raised when file synchronization starts
type FileSyncStartedEvent struct {
	AuditEvent
	FileID       valueobjects.FileID         `json:"fileId"`
	FileName     string                     `json:"fileName"`
	FilePath     string                     `json:"filePath"`
	FileSize     int64                      `json:"fileSize"`
	FileHash     string                     `json:"fileHash"`
	SourceNodeID valueobjects.StorageNodeID  `json:"sourceNodeId"`
	TargetNodeID valueobjects.StorageNodeID  `json:"targetNodeId"`
	Operation    string                     `json:"operation"` // CREATE, UPDATE, DELETE, COPY, MOVE
	UserID       string                     `json:"userId,omitempty"`
}

// FileSyncCompletedEvent is raised when file synchronization completes successfully
type FileSyncCompletedEvent struct {
	AuditEvent
	FileID           valueobjects.FileID         `json:"fileId"`
	FileName         string                     `json:"fileName"`
	FilePath         string                     `json:"filePath"`
	FileSize         int64                      `json:"fileSize"`
	FileHash         string                     `json:"fileHash"`
	SourceNodeID     valueobjects.StorageNodeID  `json:"sourceNodeId"`
	TargetNodeID     valueobjects.StorageNodeID  `json:"targetNodeId"`
	Operation        string                     `json:"operation"`
	BytesTransferred int64                      `json:"bytesTransferred"`
	Duration         time.Duration             `json:"duration"`
	UserID           string                     `json:"userId,omitempty"`
}

// FileSyncFailedEvent is raised when file synchronization fails
type FileSyncFailedEvent struct {
	AuditEvent
	FileID       valueobjects.FileID         `json:"fileId"`
	FileName     string                     `json:"fileName"`
	FilePath     string                     `json:"filePath"`
	SourceNodeID valueobjects.StorageNodeID  `json:"sourceNodeId"`
	TargetNodeID valueobjects.StorageNodeID  `json:"targetNodeId"`
	Operation    string                     `json:"operation"`
	ErrorCode    string                     `json:"errorCode"`
	ErrorMessage string                     `json:"errorMessage"`
	Retryable   bool                       `json:"retryable"`
	RetryCount  int                        `json:"retryCount"`
	UserID      string                     `json:"userId,omitempty"`
}

// ConflictDetectedEvent is raised when a conflict is detected
type ConflictDetectedEvent struct {
	AuditEvent
	ConflictID   valueobjects.ConflictID     `json:"conflictId"`
	FileID       valueobjects.FileID         `json:"fileId"`
	FileName     string                     `json:"fileName"`
	SourceNodeID valueobjects.StorageNodeID  `json:"sourceNodeId"`
	TargetNodeID valueobjects.StorageNodeID  `json:"targetNodeId"`
	ConflictType string                     `json:"conflictType"`
	Description  string                     `json:"description"`
	UserID       string                     `json:"userId,omitempty"`
}

// ConflictResolvedEvent is raised when a conflict is resolved
type ConflictResolvedEvent struct {
	AuditEvent
	ConflictID   valueobjects.ConflictID     `json:"conflictId"`
	FileID       valueobjects.FileID         `json:"fileId"`
	SourceNodeID valueobjects.StorageNodeID  `json:"sourceNodeId"`
	TargetNodeID valueobjects.StorageNodeID  `json:"targetNodeId"`
	Resolution   string                     `json:"resolution"`
	ResolvedBy   string                     `json:"resolvedBy"`
	UserID       string                     `json:"userId,omitempty"`
}

// NewSyncJobStartedEvent creates a new SyncJobStartedEvent
func NewSyncJobStartedEvent(syncJobID valueobjects.SyncJobID, sourceNodeID, targetNodeID valueobjects.StorageNodeID, fileCount int, userID string) *SyncJobStartedEvent {
	return &SyncJobStartedEvent{
		AuditEvent: AuditEvent{
			ID:          primitive.NewObjectID().Hex(),
			EventType:   entities.AuditEventTypeSyncStarted,
			Timestamp:   time.Now().UTC(),
			AggregateID: syncJobID.String(),
			Data:        make(map[string]interface{}),
			Version:     1,
		},
		SyncJobID:    syncJobID,
		SourceNodeID: sourceNodeID,
		TargetNodeID: targetNodeID,
		FileCount:    fileCount,
		UserID:       userID,
	}
}

// NewSyncJobCompletedEvent creates a new SyncJobCompletedEvent
func NewSyncJobCompletedEvent(syncJobID valueobjects.SyncJobID, sourceNodeID, targetNodeID valueobjects.StorageNodeID, filesProcessed, filesSkipped int, bytesTransferred int64, duration time.Duration, userID string) *SyncJobCompletedEvent {
	return &SyncJobCompletedEvent{
		AuditEvent: AuditEvent{
			ID:          primitive.NewObjectID().Hex(),
			EventType:   entities.AuditEventTypeSyncCompleted,
			Timestamp:   time.Now().UTC(),
			AggregateID: syncJobID.String(),
			Data:        make(map[string]interface{}),
			Version:     1,
		},
		SyncJobID:        syncJobID,
		SourceNodeID:     sourceNodeID,
		TargetNodeID:     targetNodeID,
		FilesProcessed:    filesProcessed,
		FilesSkipped:     filesSkipped,
		BytesTransferred: bytesTransferred,
		Duration:         duration,
		UserID:           userID,
	}
}

// NewSyncJobFailedEvent creates a new SyncJobFailedEvent
func NewSyncJobFailedEvent(syncJobID valueobjects.SyncJobID, sourceNodeID, targetNodeID valueobjects.StorageNodeID, errorCode, errorMessage string, retryable bool, userID string) *SyncJobFailedEvent {
	return &SyncJobFailedEvent{
		AuditEvent: AuditEvent{
			ID:          primitive.NewObjectID().Hex(),
			EventType:   entities.AuditEventTypeSyncFailed,
			Timestamp:   time.Now().UTC(),
			AggregateID: syncJobID.String(),
			Data:        make(map[string]interface{}),
			Version:     1,
		},
		SyncJobID:    syncJobID,
		SourceNodeID: sourceNodeID,
		TargetNodeID: targetNodeID,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		Retryable:   retryable,
		UserID:      userID,
	}
}

// NewFileSyncStartedEvent creates a new FileSyncStartedEvent
func NewFileSyncStartedEvent(fileID valueobjects.FileID, fileName, filePath string, fileSize int64, fileHash string, sourceNodeID, targetNodeID valueobjects.StorageNodeID, operation, userID string) *FileSyncStartedEvent {
	return &FileSyncStartedEvent{
		AuditEvent: AuditEvent{
			ID:          primitive.NewObjectID().Hex(),
			EventType:   entities.AuditEventTypeFileCreated,
			Timestamp:   time.Now().UTC(),
			AggregateID: fileID.String(),
			Data:        make(map[string]interface{}),
			Version:     1,
		},
		FileID:       fileID,
		FileName:     fileName,
		FilePath:     filePath,
		FileSize:     fileSize,
		FileHash:     fileHash,
		SourceNodeID: sourceNodeID,
		TargetNodeID: targetNodeID,
		Operation:    operation,
		UserID:       userID,
	}
}

// NewFileSyncCompletedEvent creates a new FileSyncCompletedEvent
func NewFileSyncCompletedEvent(fileID valueobjects.FileID, fileName, filePath string, fileSize int64, fileHash string, sourceNodeID, targetNodeID valueobjects.StorageNodeID, operation string, bytesTransferred int64, duration time.Duration, userID string) *FileSyncCompletedEvent {
	return &FileSyncCompletedEvent{
		AuditEvent: AuditEvent{
			ID:          primitive.NewObjectID().Hex(),
			EventType:   entities.AuditEventTypeFileUpdated,
			Timestamp:   time.Now().UTC(),
			AggregateID: fileID.String(),
			Data:        make(map[string]interface{}),
			Version:     1,
		},
		FileID:           fileID,
		FileName:         fileName,
		FilePath:         filePath,
		FileSize:         fileSize,
		FileHash:         fileHash,
		SourceNodeID:     sourceNodeID,
		TargetNodeID:     targetNodeID,
		Operation:        operation,
		BytesTransferred: bytesTransferred,
		Duration:         duration,
		UserID:           userID,
	}
}

// NewFileSyncFailedEvent creates a new FileSyncFailedEvent
func NewFileSyncFailedEvent(fileID valueobjects.FileID, fileName, filePath string, sourceNodeID, targetNodeID valueobjects.StorageNodeID, operation, errorCode, errorMessage string, retryable bool, retryCount int, userID string) *FileSyncFailedEvent {
	return &FileSyncFailedEvent{
		AuditEvent: AuditEvent{
			ID:          primitive.NewObjectID().Hex(),
			EventType:   entities.AuditEventTypeSyncFailed,
			Timestamp:   time.Now().UTC(),
			AggregateID: fileID.String(),
			Data:        make(map[string]interface{}),
			Version:     1,
		},
		FileID:       fileID,
		FileName:     fileName,
		FilePath:     filePath,
		SourceNodeID: sourceNodeID,
		TargetNodeID: targetNodeID,
		Operation:    operation,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		Retryable:   retryable,
		RetryCount:  retryCount,
		UserID:      userID,
	}
}

// NewConflictDetectedEvent creates a new ConflictDetectedEvent
func NewConflictDetectedEvent(conflictID valueobjects.ConflictID, fileID valueobjects.FileID, fileName string, sourceNodeID, targetNodeID valueobjects.StorageNodeID, conflictType, description, userID string) *ConflictDetectedEvent {
	return &ConflictDetectedEvent{
		AuditEvent: AuditEvent{
			ID:          primitive.NewObjectID().Hex(),
			EventType:   entities.AuditEventTypeConflictDetected,
			Timestamp:   time.Now().UTC(),
			AggregateID: conflictID.String(),
			Data:        make(map[string]interface{}),
			Version:     1,
		},
		ConflictID:   conflictID,
		FileID:       fileID,
		FileName:     fileName,
		SourceNodeID: sourceNodeID,
		TargetNodeID: targetNodeID,
		ConflictType: conflictType,
		Description:  description,
		UserID:       userID,
	}
}

// NewConflictResolvedEvent creates a new ConflictResolvedEvent
func NewConflictResolvedEvent(conflictID valueobjects.ConflictID, fileID valueobjects.FileID, sourceNodeID, targetNodeID valueobjects.StorageNodeID, resolution, resolvedBy, userID string) *ConflictResolvedEvent {
	return &ConflictResolvedEvent{
		AuditEvent: AuditEvent{
			ID:          primitive.NewObjectID().Hex(),
			EventType:   entities.AuditEventTypeConflictResolved,
			Timestamp:   time.Now().UTC(),
			AggregateID: conflictID.String(),
			Data:        make(map[string]interface{}),
			Version:     1,
		},
		ConflictID:   conflictID,
		FileID:       fileID,
		SourceNodeID: sourceNodeID,
		TargetNodeID: targetNodeID,
		Resolution:   resolution,
		ResolvedBy:   resolvedBy,
		UserID:       userID,
	}
}
