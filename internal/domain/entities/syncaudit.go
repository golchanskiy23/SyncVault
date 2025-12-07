package entities

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// SyncAudit represents a synchronization audit event
type SyncAudit struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Timestamp time.Time          `bson:"timestamp" json:"timestamp"`
	EventType string             `bson:"eventType" json:"eventType"`
	Status    string             `bson:"status" json:"status"`

	// Core entities
	SyncJobID    *primitive.ObjectID `bson:"syncJobId,omitempty" json:"syncJobId,omitempty"`
	FileID       *primitive.ObjectID `bson:"fileId,omitempty" json:"fileId,omitempty"`
	SourceNodeID *primitive.ObjectID `bson:"sourceNodeId,omitempty" json:"sourceNodeId,omitempty"`
	TargetNodeID *primitive.ObjectID `bson:"targetNodeId,omitempty" json:"targetNodeId,omitempty"`

	// File information (optional, flexible schema)
	FileName *string `bson:"fileName,omitempty" json:"fileName,omitempty"`
	FilePath *string `bson:"filePath,omitempty" json:"filePath,omitempty"`
	FileSize *int64  `bson:"fileSize,omitempty" json:"fileSize,omitempty"`
	FileHash *string `bson:"fileHash,omitempty" json:"fileHash,omitempty"`

	// Operation details
	Operation        *SyncOperation `bson:"operation,omitempty" json:"operation,omitempty"`
	Duration         *int64         `bson:"duration,omitempty" json:"duration,omitempty"` // milliseconds
	BytesTransferred *int64         `bson:"bytesTransferred,omitempty" json:"bytesTransferred,omitempty"`

	// Error information
	Error      *ErrorInfo `bson:"error,omitempty" json:"error,omitempty"`
	RetryCount *int       `bson:"retryCount,omitempty" json:"retryCount,omitempty"`

	// Context metadata (flexible)
	Metadata map[string]interface{} `bson:"metadata,omitempty" json:"metadata,omitempty"`

	// User and session context
	UserID    *string `bson:"userId,omitempty" json:"userId,omitempty"`
	SessionID *string `bson:"sessionId,omitempty" json:"sessionId,omitempty"`
	ClientIP  *string `bson:"clientIp,omitempty" json:"clientIp,omitempty"`
	UserAgent *string `bson:"userAgent,omitempty" json:"userAgent,omitempty"`

	// System information
	NodeID      *string `bson:"nodeId,omitempty" json:"nodeId,omitempty"`
	Version     *string `bson:"version,omitempty" json:"version,omitempty"`
	Environment *string `bson:"environment,omitempty" json:"environment,omitempty"`
}

// SyncOperation represents the type of sync operation
type SyncOperation struct {
	Type      string                 `bson:"type" json:"type"`                             // CREATE, UPDATE, DELETE, COPY, MOVE
	Direction string                 `bson:"direction" json:"direction"`                   // PUSH, PULL, BIDIRECTIONAL
	Protocol  string                 `bson:"protocol,omitempty" json:"protocol,omitempty"` // HTTP, SSH, SMB, etc.
	Options   map[string]interface{} `bson:"options,omitempty" json:"options,omitempty"`
}

// ErrorInfo represents error details
type ErrorInfo struct {
	Code       string `bson:"code" json:"code"`
	Message    string `bson:"message" json:"message"`
	StackTrace string `bson:"stackTrace,omitempty" json:"stackTrace,omitempty"`
	Retryable  bool   `bson:"retryable" json:"retryable"`
	Category   string `bson:"category,omitempty" json:"category,omitempty"` // NETWORK, PERMISSION, CONFLICT, SYSTEM
}

// Constants for audit event types (different from domain events)
const (
	AuditEventTypeSyncStarted      = "audit_sync_started"
	AuditEventTypeSyncCompleted    = "audit_sync_completed"
	AuditEventTypeSyncFailed       = "audit_sync_failed"
	AuditEventTypeSyncRetried      = "audit_sync_retried"
	AuditEventTypeConflictDetected = "audit_conflict_detected"
	AuditEventTypeConflictResolved = "audit_conflict_resolved"
	AuditEventTypeFileCreated      = "audit_file_created"
	AuditEventTypeFileUpdated      = "audit_file_updated"
	AuditEventTypeFileDeleted      = "audit_file_deleted"
	AuditEventTypeFileCopied       = "audit_file_copied"
	AuditEventTypeFileMoved        = "audit_file_moved"
)

// Constants for status
const (
	StatusSuccess   = "success"
	StatusFailed    = "failed"
	StatusPending   = "pending"
	StatusRetrying  = "retrying"
	StatusCancelled = "cancelled"
	StatusPartial   = "partial"
)

// Constants for operation types
const (
	OperationCreate = "CREATE"
	OperationUpdate = "UPDATE"
	OperationDelete = "DELETE"
	OperationCopy   = "COPY"
	OperationMove   = "MOVE"
)

// Constants for direction
const (
	DirectionPush          = "PUSH"
	DirectionPull          = "PULL"
	DirectionBidirectional = "BIDIRECTIONAL"
)

// Constants for error categories
const (
	ErrorCategoryNetwork    = "NETWORK"
	ErrorCategoryPermission = "PERMISSION"
	ErrorCategoryConflict   = "CONFLICT"
	ErrorCategorySystem     = "SYSTEM"
	ErrorCategoryValidation = "VALIDATION"
	ErrorCategoryTimeout    = "TIMEOUT"
)

// NewSyncAudit creates a new SyncAudit entity
func NewSyncAudit(eventType, status string) *SyncAudit {
	return &SyncAudit{
		ID:        primitive.NewObjectID(),
		Timestamp: time.Now().UTC(),
		EventType: eventType,
		Status:    status,
		Metadata:  make(map[string]interface{}),
	}
}

// WithSyncJob sets the sync job ID
func (sa *SyncAudit) WithSyncJob(syncJobID primitive.ObjectID) *SyncAudit {
	sa.SyncJobID = &syncJobID
	return sa
}

// WithFile sets file information
func (sa *SyncAudit) WithFile(fileID primitive.ObjectID, fileName, filePath string, fileSize int64, fileHash string) *SyncAudit {
	sa.FileID = &fileID
	sa.FileName = &fileName
	sa.FilePath = &filePath
	sa.FileSize = &fileSize
	sa.FileHash = &fileHash
	return sa
}

// WithNodes sets source and target node IDs
func (sa *SyncAudit) WithNodes(sourceNodeID, targetNodeID primitive.ObjectID) *SyncAudit {
	sa.SourceNodeID = &sourceNodeID
	sa.TargetNodeID = &targetNodeID
	return sa
}

// WithOperation sets operation details
func (sa *SyncAudit) WithOperation(opType, direction, protocol string, options map[string]interface{}) *SyncAudit {
	sa.Operation = &SyncOperation{
		Type:      opType,
		Direction: direction,
		Protocol:  protocol,
		Options:   options,
	}
	return sa
}

// WithDuration sets operation duration in milliseconds
func (sa *SyncAudit) WithDuration(duration int64) *SyncAudit {
	sa.Duration = &duration
	return sa
}

// WithBytesTransferred sets bytes transferred
func (sa *SyncAudit) WithBytesTransferred(bytes int64) *SyncAudit {
	sa.BytesTransferred = &bytes
	return sa
}

// WithError sets error information
func (sa *SyncAudit) WithError(code, message, stackTrace, category string, retryable bool) *SyncAudit {
	sa.Error = &ErrorInfo{
		Code:       code,
		Message:    message,
		StackTrace: stackTrace,
		Retryable:  retryable,
		Category:   category,
	}
	return sa
}

// WithRetryCount sets retry count
func (sa *SyncAudit) WithRetryCount(count int) *SyncAudit {
	sa.RetryCount = &count
	return sa
}

// WithMetadata adds metadata key-value pair
func (sa *SyncAudit) WithMetadata(key string, value interface{}) *SyncAudit {
	if sa.Metadata == nil {
		sa.Metadata = make(map[string]interface{})
	}
	sa.Metadata[key] = value
	return sa
}

// WithUserContext sets user and session context
func (sa *SyncAudit) WithUserContext(userID, sessionID, clientIP, userAgent string) *SyncAudit {
	sa.UserID = &userID
	sa.SessionID = &sessionID
	sa.ClientIP = &clientIP
	sa.UserAgent = &userAgent
	return sa
}

// WithSystemContext sets system information
func (sa *SyncAudit) WithSystemContext(nodeID, version, environment string) *SyncAudit {
	sa.NodeID = &nodeID
	sa.Version = &version
	sa.Environment = &environment
	return sa
}

// IsSuccess returns true if the audit event represents a successful operation
func (sa *SyncAudit) IsSuccess() bool {
	return sa.Status == StatusSuccess
}

// IsFailed returns true if the audit event represents a failed operation
func (sa *SyncAudit) IsFailed() bool {
	return sa.Status == StatusFailed
}

// HasError returns true if the audit event has error information
func (sa *SyncAudit) HasError() bool {
	return sa.Error != nil
}

// GetDurationMs returns duration in milliseconds, returns 0 if nil
func (sa *SyncAudit) GetDurationMs() int64 {
	if sa.Duration != nil {
		return *sa.Duration
	}
	return 0
}

// GetBytesTransferred returns bytes transferred, returns 0 if nil
func (sa *SyncAudit) GetBytesTransferred() int64 {
	if sa.BytesTransferred != nil {
		return *sa.BytesTransferred
	}
	return 0
}

// GetRetryCount returns retry count, returns 0 if nil
func (sa *SyncAudit) GetRetryCount() int {
	if sa.RetryCount != nil {
		return *sa.RetryCount
	}
	return 0
}
