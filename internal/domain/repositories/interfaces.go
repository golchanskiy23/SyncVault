package repositories

import (
	"context"
	"time"

	"syncvault/internal/domain/entities"
	"syncvault/internal/domain/valueobjects"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type FileRepository interface {
	Save(ctx context.Context, file *entities.File) error
	FindByID(ctx context.Context, id valueobjects.FileID) (*entities.File, error)
	FindByPath(ctx context.Context, nodeID valueobjects.StorageNodeID, path valueobjects.FilePath) (*entities.File, error)
	FindByNode(ctx context.Context, nodeID valueobjects.StorageNodeID) ([]*entities.File, error)
	FindModifiedSince(ctx context.Context, nodeID valueobjects.StorageNodeID, since time.Time) ([]*entities.File, error)
	Delete(ctx context.Context, id valueobjects.FileID) error
	Exists(ctx context.Context, nodeID valueobjects.StorageNodeID, path valueobjects.FilePath) (bool, error)
}

type StorageNodeRepository interface {
	Save(ctx context.Context, node *entities.StorageNode) error
	FindByID(ctx context.Context, id valueobjects.StorageNodeID) (*entities.StorageNode, error)
	FindAll(ctx context.Context) ([]*entities.StorageNode, error)
	FindByType(ctx context.Context, nodeType entities.NodeType) ([]*entities.StorageNode, error)
	FindOnline(ctx context.Context) ([]*entities.StorageNode, error)
	Delete(ctx context.Context, id valueobjects.StorageNodeID) error
	UpdateStatus(ctx context.Context, id valueobjects.StorageNodeID, status entities.NodeStatus) error
}

type ConflictRepository interface {
	Save(ctx context.Context, conflict *entities.Conflict) error
	FindByID(ctx context.Context, id valueobjects.ConflictID) (*entities.Conflict, error)
	FindByFileID(ctx context.Context, fileID valueobjects.FileID) ([]*entities.Conflict, error)
	FindByNodes(ctx context.Context, sourceNode, targetNode valueobjects.StorageNodeID) ([]*entities.Conflict, error)
	FindUnresolved(ctx context.Context) ([]*entities.Conflict, error)
	Delete(ctx context.Context, id valueobjects.ConflictID) error
}

type SyncEventRepository interface {
	Save(ctx context.Context, event *entities.SyncEvent) error
	FindByID(ctx context.Context, id valueobjects.FileID) (*entities.SyncEvent, error)
	FindByNode(ctx context.Context, nodeID valueobjects.StorageNodeID, limit int) ([]*entities.SyncEvent, error)
	FindByJobID(ctx context.Context, jobID valueobjects.SyncJobID) ([]*entities.SyncEvent, error)
	FindByType(ctx context.Context, eventType entities.EventType, limit int) ([]*entities.SyncEvent, error)
	FindRecent(ctx context.Context, limit int) ([]*entities.SyncEvent, error)
	Delete(ctx context.Context, id valueobjects.FileID) error
	DeleteOldEvents(ctx context.Context, olderThan time.Time) error
}

type SyncAuditRepository interface {
	Save(ctx context.Context, audit *entities.SyncAudit) error
	SaveBatch(ctx context.Context, audits []*entities.SyncAudit) error
	FindByID(ctx context.Context, id primitive.ObjectID) (*entities.SyncAudit, error)
	FindBySyncJobID(ctx context.Context, syncJobID primitive.ObjectID) ([]*entities.SyncAudit, error)
	FindByFileID(ctx context.Context, fileID primitive.ObjectID) ([]*entities.SyncAudit, error)
	FindByTimeRange(ctx context.Context, startTime, endTime time.Time) ([]*entities.SyncAudit, error)
	FindByEventType(ctx context.Context, eventType string) ([]*entities.SyncAudit, error)
	FindByStatus(ctx context.Context, status string) ([]*entities.SyncAudit, error)
	FindFailedEvents(ctx context.Context) ([]*entities.SyncAudit, error)
	FindByNode(ctx context.Context, nodeID primitive.ObjectID) ([]*entities.SyncAudit, error)
	FindByUser(ctx context.Context, userID string) ([]*entities.SyncAudit, error)
	FindWithError(ctx context.Context) ([]*entities.SyncAudit, error)
	FindRecent(ctx context.Context, hours int) ([]*entities.SyncAudit, error)
	FindWithPagination(ctx context.Context, filter interface{}, page, limit int) ([]*entities.SyncAudit, int64, error)
	GetStats(ctx context.Context) (*SyncAuditStats, error)
	CleanupOldEvents(ctx context.Context, olderThan time.Duration) (int64, error)
}

type SyncAuditStats struct {
	TotalEvents           int   `json:"totalEvents"`
	SuccessfulEvents      int   `json:"successfulEvents"`
	FailedEvents          int   `json:"failedEvents"`
	TotalBytesTransferred int64 `json:"totalBytesTransferred"`
	AverageDuration       int64 `json:"averageDuration"` // milliseconds
}
