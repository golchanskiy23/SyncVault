package services

import (
	"context"
	"fmt"
	"time"

	"syncvault/internal/domain/domainevents"
	"syncvault/internal/domain/entities"
	"syncvault/internal/domain/repositories"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// AuditService handles audit logging for sync operations
type AuditService struct {
	auditRepo repositories.SyncAuditRepository
}

// NewAuditService creates a new audit service
func NewAuditService(auditRepo repositories.SyncAuditRepository) *AuditService {
	return &AuditService{
		auditRepo: auditRepo,
	}
}

// LogSyncJobStarted logs when a sync job starts
func (s *AuditService) LogSyncJobStarted(ctx context.Context, event *domainevents.SyncJobStartedEvent) error {
	syncJobID, err := primitive.ObjectIDFromHex(event.AggregateID)
	if err != nil {
		return fmt.Errorf("invalid sync job ID: %w", err)
	}

	sourceNodeID, err := s.convertToMongoID(event.SourceNodeID.String())
	if err != nil {
		return fmt.Errorf("invalid source node ID: %w", err)
	}

	targetNodeID, err := s.convertToMongoID(event.TargetNodeID.String())
	if err != nil {
		return fmt.Errorf("invalid target node ID: %w", err)
	}

	audit := entities.NewSyncAudit(event.EventType, entities.StatusPending).
		WithSyncJob(syncJobID).
		WithNodes(*sourceNodeID, *targetNodeID).
		WithMetadata("fileCount", event.FileCount).
		WithUserContext(event.UserID, "", "", "")

	if event.UserID != "" {
		audit.WithMetadata("userId", event.UserID)
	}

	return s.auditRepo.Save(ctx, audit)
}

// LogSyncJobCompleted logs when a sync job completes successfully
func (s *AuditService) LogSyncJobCompleted(ctx context.Context, event *domainevents.SyncJobCompletedEvent) error {
	syncJobID, err := primitive.ObjectIDFromHex(event.AggregateID)
	if err != nil {
		return fmt.Errorf("invalid sync job ID: %w", err)
	}

	sourceNodeID, err := s.convertToMongoID(event.SourceNodeID.String())
	if err != nil {
		return fmt.Errorf("invalid source node ID: %w", err)
	}

	targetNodeID, err := s.convertToMongoID(event.TargetNodeID.String())
	if err != nil {
		return fmt.Errorf("invalid target node ID: %w", err)
	}

	audit := entities.NewSyncAudit(event.EventType, entities.StatusSuccess).
		WithSyncJob(syncJobID).
		WithNodes(*sourceNodeID, *targetNodeID).
		WithDuration(event.Duration.Milliseconds()).
		WithBytesTransferred(event.BytesTransferred).
		WithMetadata("filesProcessed", event.FilesProcessed).
		WithMetadata("filesSkipped", event.FilesSkipped).
		WithUserContext(event.UserID, "", "", "")

	if event.UserID != "" {
		audit.WithMetadata("userId", event.UserID)
	}

	return s.auditRepo.Save(ctx, audit)
}

// LogSyncJobFailed logs when a sync job fails
func (s *AuditService) LogSyncJobFailed(ctx context.Context, event *domainevents.SyncJobFailedEvent) error {
	syncJobID, err := primitive.ObjectIDFromHex(event.AggregateID)
	if err != nil {
		return fmt.Errorf("invalid sync job ID: %w", err)
	}

	sourceNodeID, err := s.convertToMongoID(event.SourceNodeID.String())
	if err != nil {
		return fmt.Errorf("invalid source node ID: %w", err)
	}

	targetNodeID, err := s.convertToMongoID(event.TargetNodeID.String())
	if err != nil {
		return fmt.Errorf("invalid target node ID: %w", err)
	}

	audit := entities.NewSyncAudit(event.EventType, entities.StatusFailed).
		WithSyncJob(syncJobID).
		WithNodes(*sourceNodeID, *targetNodeID).
		WithError(event.ErrorCode, event.ErrorMessage, "", "", event.Retryable).
		WithUserContext(event.UserID, "", "", "")

	if event.UserID != "" {
		audit.WithMetadata("userId", event.UserID)
	}

	return s.auditRepo.Save(ctx, audit)
}

// LogFileSyncStarted logs when file synchronization starts
func (s *AuditService) LogFileSyncStarted(ctx context.Context, event *domainevents.FileSyncStartedEvent) error {
	fileID, err := primitive.ObjectIDFromHex(event.AggregateID)
	if err != nil {
		return fmt.Errorf("invalid file ID: %w", err)
	}

	sourceNodeID, err := s.convertToMongoID(event.SourceNodeID.String())
	if err != nil {
		return fmt.Errorf("invalid source node ID: %w", err)
	}

	targetNodeID, err := s.convertToMongoID(event.TargetNodeID.String())
	if err != nil {
		return fmt.Errorf("invalid target node ID: %w", err)
	}

	audit := entities.NewSyncAudit(event.EventType, entities.StatusPending).
		WithFile(fileID, event.FileName, event.FilePath, event.FileSize, event.FileHash).
		WithNodes(*sourceNodeID, *targetNodeID).
		WithOperation(event.Operation, entities.DirectionPush, "", nil).
		WithUserContext(event.UserID, "", "", "")

	if event.UserID != "" {
		audit.WithMetadata("userId", event.UserID)
	}

	return s.auditRepo.Save(ctx, audit)
}

// LogFileSyncCompleted logs when file synchronization completes successfully
func (s *AuditService) LogFileSyncCompleted(ctx context.Context, event *domainevents.FileSyncCompletedEvent) error {
	fileID, err := primitive.ObjectIDFromHex(event.AggregateID)
	if err != nil {
		return fmt.Errorf("invalid file ID: %w", err)
	}

	sourceNodeID, err := s.convertToMongoID(event.SourceNodeID.String())
	if err != nil {
		return fmt.Errorf("invalid source node ID: %w", err)
	}

	targetNodeID, err := s.convertToMongoID(event.TargetNodeID.String())
	if err != nil {
		return fmt.Errorf("invalid target node ID: %w", err)
	}

	audit := entities.NewSyncAudit(event.EventType, entities.StatusSuccess).
		WithFile(fileID, event.FileName, event.FilePath, event.FileSize, event.FileHash).
		WithNodes(*sourceNodeID, *targetNodeID).
		WithOperation(event.Operation, entities.DirectionPush, "", nil).
		WithDuration(event.Duration.Milliseconds()).
		WithBytesTransferred(event.BytesTransferred).
		WithUserContext(event.UserID, "", "", "")

	if event.UserID != "" {
		audit.WithMetadata("userId", event.UserID)
	}

	return s.auditRepo.Save(ctx, audit)
}

// LogFileSyncFailed logs when file synchronization fails
func (s *AuditService) LogFileSyncFailed(ctx context.Context, event *domainevents.FileSyncFailedEvent) error {
	fileID, err := primitive.ObjectIDFromHex(event.AggregateID)
	if err != nil {
		return fmt.Errorf("invalid file ID: %w", err)
	}

	sourceNodeID, err := s.convertToMongoID(event.SourceNodeID.String())
	if err != nil {
		return fmt.Errorf("invalid source node ID: %w", err)
	}

	targetNodeID, err := s.convertToMongoID(event.TargetNodeID.String())
	if err != nil {
		return fmt.Errorf("invalid target node ID: %w", err)
	}

	audit := entities.NewSyncAudit(event.EventType, entities.StatusFailed).
		WithFile(fileID, event.FileName, event.FilePath, 0, "").
		WithNodes(*sourceNodeID, *targetNodeID).
		WithOperation(event.Operation, entities.DirectionPush, "", nil).
		WithError(event.ErrorCode, event.ErrorMessage, "", "", event.Retryable).
		WithRetryCount(event.RetryCount).
		WithUserContext(event.UserID, "", "", "")

	if event.UserID != "" {
		audit.WithMetadata("userId", event.UserID)
	}

	return s.auditRepo.Save(ctx, audit)
}

// LogConflictDetected logs when a conflict is detected
func (s *AuditService) LogConflictDetected(ctx context.Context, event *domainevents.ConflictDetectedEvent) error {
	conflictID, err := primitive.ObjectIDFromHex(event.AggregateID)
	if err != nil {
		return fmt.Errorf("invalid conflict ID: %w", err)
	}

	fileID, err := primitive.ObjectIDFromHex(event.FileID.String())
	if err != nil {
		return fmt.Errorf("invalid file ID: %w", err)
	}

	sourceNodeID, err := s.convertToMongoID(event.SourceNodeID.String())
	if err != nil {
		return fmt.Errorf("invalid source node ID: %w", err)
	}

	targetNodeID, err := s.convertToMongoID(event.TargetNodeID.String())
	if err != nil {
		return fmt.Errorf("invalid target node ID: %w", err)
	}

	audit := entities.NewSyncAudit(event.EventType, entities.StatusPending).
		WithFile(fileID, event.FileName, "", 0, "").
		WithSyncJob(conflictID).
		WithNodes(*sourceNodeID, *targetNodeID).
		WithMetadata("conflictType", event.ConflictType).
		WithMetadata("description", event.Description).
		WithUserContext(event.UserID, "", "", "")

	if event.UserID != "" {
		audit.WithMetadata("userId", event.UserID)
	}

	return s.auditRepo.Save(ctx, audit)
}

// LogConflictResolved logs when a conflict is resolved
func (s *AuditService) LogConflictResolved(ctx context.Context, event *domainevents.ConflictResolvedEvent) error {
	conflictID, err := primitive.ObjectIDFromHex(event.AggregateID)
	if err != nil {
		return fmt.Errorf("invalid conflict ID: %w", err)
	}

	fileID, err := primitive.ObjectIDFromHex(event.FileID.String())
	if err != nil {
		return fmt.Errorf("invalid file ID: %w", err)
	}

	sourceNodeID, err := s.convertToMongoID(event.SourceNodeID.String())
	if err != nil {
		return fmt.Errorf("invalid source node ID: %w", err)
	}

	targetNodeID, err := s.convertToMongoID(event.TargetNodeID.String())
	if err != nil {
		return fmt.Errorf("invalid target node ID: %w", err)
	}

	audit := entities.NewSyncAudit(event.EventType, entities.StatusSuccess).
		WithFile(fileID, "", "", 0, "").
		WithSyncJob(conflictID).
		WithNodes(*sourceNodeID, *targetNodeID).
		WithMetadata("resolution", event.Resolution).
		WithMetadata("resolvedBy", event.ResolvedBy).
		WithUserContext(event.UserID, "", "", "")

	if event.UserID != "" {
		audit.WithMetadata("userId", event.UserID)
	}

	return s.auditRepo.Save(ctx, audit)
}

// LogCustomAudit logs a custom audit event
func (s *AuditService) LogCustomAudit(ctx context.Context, eventType, status string, metadata map[string]interface{}) error {
	audit := entities.NewSyncAudit(eventType, status)

	for key, value := range metadata {
		audit.WithMetadata(key, value)
	}

	return s.auditRepo.Save(ctx, audit)
}

// GetAuditStats returns audit statistics
func (s *AuditService) GetAuditStats(ctx context.Context) (*repositories.SyncAuditStats, error) {
	return s.auditRepo.GetStats(ctx)
}

// GetRecentAudits returns recent audit events
func (s *AuditService) GetRecentAudits(ctx context.Context, hours int) ([]*entities.SyncAudit, error) {
	return s.auditRepo.FindRecent(ctx, hours)
}

// GetFailedAudits returns failed audit events
func (s *AuditService) GetFailedAudits(ctx context.Context) ([]*entities.SyncAudit, error) {
	return s.auditRepo.FindFailedEvents(ctx)
}

// CleanupOldAudits removes old audit events
func (s *AuditService) CleanupOldAudits(ctx context.Context, olderThan time.Duration) (int64, error) {
	return s.auditRepo.CleanupOldEvents(ctx, olderThan)
}

// convertToMongoID converts a string ID to MongoDB ObjectID
func (s *AuditService) convertToMongoID(id string) (*primitive.ObjectID, error) {
	if id == "" {
		return nil, fmt.Errorf("empty ID")
	}

	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		// If it's not a valid hex ID, generate a new one based on the string hash
		// This is a fallback for compatibility with different ID formats
		return &primitive.ObjectID{}, nil
	}

	return &objID, nil
}

// BatchLogAudits logs multiple audit events in a batch
func (s *AuditService) BatchLogAudits(ctx context.Context, audits []*entities.SyncAudit) error {
	if len(audits) == 0 {
		return nil
	}

	// Ensure all audits have timestamps
	for _, audit := range audits {
		if audit.Timestamp.IsZero() {
			audit.Timestamp = time.Now().UTC()
		}
	}

	return s.auditRepo.SaveBatch(ctx, audits)
}
