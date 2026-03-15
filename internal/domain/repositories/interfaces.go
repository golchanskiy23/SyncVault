package repositories

import (
	"context"
	"time"

	"syncvault/internal/domain/entities"
	"syncvault/internal/domain/valueobjects"
)

type FileRepository interface {
	Save(ctx context.Context, file *entities.FileObject) error
	FindByID(ctx context.Context, id valueobjects.FileID) (*entities.FileObject, error)
	FindByPath(ctx context.Context, nodeID valueobjects.StorageNodeID, path valueobjects.FilePath) (*entities.FileObject, error)
	FindByNode(ctx context.Context, nodeID valueobjects.StorageNodeID) ([]*entities.FileObject, error)
	FindModifiedSince(ctx context.Context, nodeID valueobjects.StorageNodeID, since time.Time) ([]*entities.FileObject, error)
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
