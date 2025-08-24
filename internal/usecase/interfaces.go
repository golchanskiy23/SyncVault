package usecase

import (
	"context"

	"syncvault/internal/domain/aggregates"
	"syncvault/internal/domain/entities"
	"syncvault/internal/domain/valueobjects"
)

type FileUseCase interface {
	SyncFile(ctx context.Context, fileID valueobjects.FileID, sourceNode, targetNode valueobjects.StorageNodeID) error
	GetFileStatus(ctx context.Context, fileID valueobjects.FileID) (*entities.FileObject, error)
	ListFiles(ctx context.Context, nodeID valueobjects.StorageNodeID) ([]*entities.FileObject, error)
}

type ConflictResolutionUseCase interface {
	DetectConflicts(ctx context.Context, syncJobID valueobjects.SyncJobID) ([]*entities.Conflict, error)
	ResolveConflict(ctx context.Context, conflictID valueobjects.ConflictID, resolution entities.ConflictResolution) error
}

type SyncUseCase interface {
	CreateSyncJob(ctx context.Context, sourceNode, targetNode valueobjects.StorageNodeID, fileIDs []valueobjects.FileID) (*aggregates.SyncJob, error)
	StartSyncJob(ctx context.Context, jobID valueobjects.SyncJobID) error
	GetSyncJobStatus(ctx context.Context, jobID valueobjects.SyncJobID) (*aggregates.SyncJob, error)
	CancelSyncJob(ctx context.Context, jobID valueobjects.SyncJobID) error
}

type StorageNodeUseCase interface {
	RegisterNode(ctx context.Context, node *entities.StorageNode) error
	GetNode(ctx context.Context, nodeID valueobjects.StorageNodeID) (*entities.StorageNode, error)
	ListNodes(ctx context.Context) ([]*entities.StorageNode, error)
	UpdateNodeStatus(ctx context.Context, nodeID valueobjects.StorageNodeID, status entities.NodeStatus) error
}
