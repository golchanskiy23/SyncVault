package usecase

import (
	"context"
	"fmt"

	"syncvault/internal/domain/entities"
	"syncvault/internal/domain/repositories"
	"syncvault/internal/domain/valueobjects"
)

type SyncUseCaseImpl struct {
	fileRepo     repositories.FileRepository
	nodeRepo     repositories.StorageNodeRepository
	conflictRepo repositories.ConflictRepository
	eventBus     EventBus
	strategies   []SyncStrategy
}

func NewSyncUseCase(
	fileRepo repositories.FileRepository,
	nodeRepo repositories.StorageNodeRepository,
	conflictRepo repositories.ConflictRepository,
	eventBus EventBus,
) *SyncUseCaseImpl {
	return &SyncUseCaseImpl{
		fileRepo:     fileRepo,
		nodeRepo:     nodeRepo,
		conflictRepo: conflictRepo,
		eventBus:     eventBus,
		strategies:   make([]SyncStrategy, 0),
	}
}

type EventBus interface {
	Publish(ctx context.Context, event interface{}) error
}

type SyncStrategy interface {
	Name() string
	CanHandle(ctx context.Context, source, target valueobjects.StorageNodeID) bool
	Sync(ctx context.Context, source, target valueobjects.StorageNodeID, files []*entities.File) error
}

type IncrementalSyncStrategy struct {
	fileRepo repositories.FileRepository
}

func (s *IncrementalSyncStrategy) Name() string {
	return "incremental"
}

func (s *IncrementalSyncStrategy) CanHandle(ctx context.Context, source, target valueobjects.StorageNodeID) bool {
	return true
}

func (s *IncrementalSyncStrategy) Sync(ctx context.Context, source, target valueobjects.StorageNodeID, files []*entities.File) error {
	for _, file := range files {
		if file.Status() == entities.FileStatusModified {
			if err := s.syncFile(ctx, source, target, file); err != nil {
				return fmt.Errorf("failed to sync file %s: %w", file.Path().String(), err)
			}
		}
	}
	return nil
}

func (s *IncrementalSyncStrategy) syncFile(ctx context.Context, source, target valueobjects.StorageNodeID, file *entities.File) error {
	file.MarkAsSynced()
	return s.fileRepo.Save(ctx, file)
}

type FullSyncStrategy struct {
	fileRepo repositories.FileRepository
}

func (s *FullSyncStrategy) Name() string {
	return "full"
}

func (s *FullSyncStrategy) CanHandle(ctx context.Context, source, target valueobjects.StorageNodeID) bool {
	return true
}

func (s *FullSyncStrategy) Sync(ctx context.Context, source, target valueobjects.StorageNodeID, files []*entities.File) error {
	for _, file := range files {
		if err := s.syncFile(ctx, source, target, file); err != nil {
			return fmt.Errorf("failed to sync file %s: %w", file.Path().String(), err)
		}
	}
	return nil
}

func (s *FullSyncStrategy) syncFile(ctx context.Context, source, target valueobjects.StorageNodeID, file *entities.File) error {
	file.MarkAsSynced()
	return s.fileRepo.Save(ctx, file)
}

func (uc *SyncUseCaseImpl) AddStrategy(strategy SyncStrategy) {
	uc.strategies = append(uc.strategies, strategy)
}

func (uc *SyncUseCaseImpl) SyncFiles(ctx context.Context, source, target valueobjects.StorageNodeID, fileIDs []valueobjects.FileID) error {
	sourceNode, err := uc.nodeRepo.FindByID(ctx, source)
	if err != nil {
		return fmt.Errorf("source node not found: %w", err)
	}

	targetNode, err := uc.nodeRepo.FindByID(ctx, target)
	if err != nil {
		return fmt.Errorf("target node not found: %w", err)
	}

	if !sourceNode.IsOnline() || !targetNode.IsOnline() {
		return fmt.Errorf("both nodes must be online for sync")
	}

	files, err := uc.getFiles(ctx, fileIDs)
	if err != nil {
		return fmt.Errorf("failed to get files: %w", err)
	}

	strategy := uc.selectStrategy(ctx, source, target)
	if strategy == nil {
		return fmt.Errorf("no suitable sync strategy found")
	}

	if err := strategy.Sync(ctx, source, target, files); err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	event := map[string]interface{}{
		"type":       "sync_completed",
		"source":     source.String(),
		"target":     target.String(),
		"file_count": len(files),
		"strategy":   strategy.Name(),
	}

	return uc.eventBus.Publish(ctx, event)
}

func (uc *SyncUseCaseImpl) selectStrategy(ctx context.Context, source, target valueobjects.StorageNodeID) SyncStrategy {
	for _, strategy := range uc.strategies {
		if strategy.CanHandle(ctx, source, target) {
			return strategy
		}
	}
	return nil
}

func (uc *SyncUseCaseImpl) getFiles(ctx context.Context, fileIDs []valueobjects.FileID) ([]*entities.File, error) {
	files := make([]*entities.File, len(fileIDs))
	for i, fileID := range fileIDs {
		file, err := uc.fileRepo.FindByID(ctx, fileID)
		if err != nil {
			return nil, fmt.Errorf("failed to find file %s: %w", fileID.String(), err)
		}
		files[i] = file
	}
	return files, nil
}
