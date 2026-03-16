package services

import (
	"context"
	"fmt"

	"syncvault/internal/domain/aggregates"
	"syncvault/internal/domain/domainevents"
	"syncvault/internal/domain/entities"
	"syncvault/internal/domain/repositories"
	"syncvault/internal/domain/valueobjects"
)

type SyncDomainService struct {
	fileRepo     repositories.FileRepository
	nodeRepo     repositories.StorageNodeRepository
	conflictRepo repositories.ConflictRepository
	eventBus     EventBus
}

type EventBus interface {
	Publish(ctx context.Context, event domainevents.DomainEvent) error
}

func NewSyncDomainService(
	fileRepo repositories.FileRepository,
	nodeRepo repositories.StorageNodeRepository,
	conflictRepo repositories.ConflictRepository,
	eventBus EventBus,
) *SyncDomainService {
	return &SyncDomainService{
		fileRepo:     fileRepo,
		nodeRepo:     nodeRepo,
		conflictRepo: conflictRepo,
		eventBus:     eventBus,
	}
}

func (s *SyncDomainService) DetectConflicts(
	ctx context.Context,
	sourceNode, targetNode valueobjects.StorageNodeID,
) ([]*entities.Conflict, error) {
	sourceFiles, err := s.fileRepo.FindByNode(ctx, sourceNode)
	if err != nil {
		return nil, fmt.Errorf("failed to get source files: %w", err)
	}

	targetFiles, err := s.fileRepo.FindByNode(ctx, targetNode)
	if err != nil {
		return nil, fmt.Errorf("failed to get target files: %w", err)
	}

	targetFileMap := make(map[string]*entities.File)
	for _, file := range targetFiles {
		targetFileMap[file.FilePath.String()] = file
	}

	var conflicts []*entities.Conflict

	for _, sourceFile := range sourceFiles {
		targetFile, exists := targetFileMap[sourceFile.FilePath.String()]

		if !exists {
			continue
		}

		conflict := s.analyzeFileConflict(sourceFile, targetFile, sourceNode, targetNode)
		if conflict != nil {
			conflicts = append(conflicts, conflict)
		}
	}

	for _, conflict := range conflicts {
		event := domainevents.NewConflictDetected(
			conflict.ID(),
			conflict.FileID(),
			conflict.SourceNode(),
			conflict.TargetNode(),
			conflict.ConflictType(),
			conflict.Description(),
			conflict.SourceFile(),
			conflict.TargetFile(),
		)

		if err := s.eventBus.Publish(ctx, event); err != nil {
			fmt.Printf("Failed to publish conflict event: %v\n", err)
		}
	}

	return conflicts, nil
}

func (s *SyncDomainService) analyzeFileConflict(
	sourceFile, targetFile *entities.File,
	sourceNode, targetNode valueobjects.StorageNodeID,
) *entities.Conflict {
	if !sourceFile.HasSameContent(targetFile) {
		if sourceFile.Status() == entities.FileStatusModified &&
			targetFile.Status() == entities.FileStatusModified {

			return entities.NewConflict(
				valueobjects.FileID{},
				sourceNode,
				targetNode,
				entities.ConflictTypeContent,
				fmt.Sprintf("Content conflict: both files modified on %s and %s",
					sourceFile.UpdatedAt().Format("2006-01-02 15:04:05"),
					targetFile.UpdatedAt().Format("2006-01-02 15:04:05")),
				sourceFile,
				targetFile,
			)
		}
	}
	fileID, _ := valueobjects.FileIDFromString(fmt.Sprintf("%d", sourceFile.ID))

	if sourceFile.Status() == entities.FileStatusDeleted &&
		targetFile.Status() == entities.FileStatusModified {
		return entities.NewConflict(
			fileID,
			sourceNode,
			targetNode,
			entities.ConflictTypeDeletion,
			"File deleted on source but modified on target",
			sourceFile,
			targetFile,
		)
	}

	if targetFile.Status() == entities.FileStatusDeleted &&
		sourceFile.Status() == entities.FileStatusModified {
		return entities.NewConflict(
			fileID,
			sourceNode,
			targetNode,
			entities.ConflictTypeDeletion,
			"File deleted on target but modified on source",
			sourceFile,
			targetFile,
		)
	}

	return nil
}

func (s *SyncDomainService) CreateSyncJob(
	ctx context.Context,
	sourceNode, targetNode valueobjects.StorageNodeID,
	fileIDs []valueobjects.FileID,
) (*aggregates.SyncJob, error) {
	source, err := s.nodeRepo.FindByID(ctx, sourceNode)
	if err != nil {
		return nil, fmt.Errorf("source node not found: %w", err)
	}

	target, err := s.nodeRepo.FindByID(ctx, targetNode)
	if err != nil {
		return nil, fmt.Errorf("target node not found: %w", err)
	}

	if !source.IsOnline() {
		return nil, fmt.Errorf("source node is not online")
	}

	if !target.IsOnline() {
		return nil, fmt.Errorf("target node is not online")
	}

	if len(fileIDs) > 0 {
		files, err := s.getFileDetails(ctx, fileIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to get file details: %w", err)
		}

		totalSize := int64(0)
		for _, file := range files {
			totalSize += file.FileSize
		}

		if !target.HasEnoughSpace(totalSize) {
			return nil, fmt.Errorf("target node doesn't have enough space. Required: %d, Available: %d",
				totalSize, target.FreeSpace())
		}
	}

	job := aggregates.NewSyncJob(sourceNode, targetNode, fileIDs)
	return job, nil
}

func (s *SyncDomainService) getFileDetails(ctx context.Context, fileIDs []valueobjects.FileID) ([]*entities.File, error) {
	files := make([]*entities.File, len(fileIDs))

	for i, fileID := range fileIDs {
		file, err := s.fileRepo.FindByID(ctx, fileID)
		if err != nil {
			return nil, fmt.Errorf("failed to find file %s: %w", fileID.String(), err)
		}
		files[i] = file
	}

	return files, nil
}

func (s *SyncDomainService) ResolveConflict(
	ctx context.Context,
	conflictID valueobjects.ConflictID,
	resolution entities.ConflictResolution,
) error {
	conflict, err := s.conflictRepo.FindByID(ctx, conflictID)
	if err != nil {
		return fmt.Errorf("conflict not found: %w", err)
	}

	switch resolution {
	case entities.ConflictResolutionKeepSource:
		err = s.keepSourceFile(ctx, conflict)
	case entities.ConflictResolutionKeepTarget:
		err = s.keepTargetFile(ctx, conflict)
	case entities.ConflictResolutionKeepBoth:
		err = s.keepBothFiles(ctx, conflict)
	case entities.ConflictResolutionManualMerge:
		conflict.Resolve(resolution)
		err = s.conflictRepo.Save(ctx, conflict)
	}

	if err != nil {
		return fmt.Errorf("failed to apply resolution: %w", err)
	}

	return nil
}

func (s *SyncDomainService) keepSourceFile(ctx context.Context, conflict *entities.Conflict) error {
	sourceFile := conflict.SourceFile()
	sourceFile.MarkAsSynced()

	if err := s.fileRepo.Save(ctx, sourceFile); err != nil {
		return fmt.Errorf("failed to save source file: %w", err)
	}

	conflict.Resolve(entities.ConflictResolutionKeepSource)
	return s.conflictRepo.Save(ctx, conflict)
}

func (s *SyncDomainService) keepTargetFile(ctx context.Context, conflict *entities.Conflict) error {
	targetFile := conflict.TargetFile()
	targetFile.MarkAsSynced()

	if err := s.fileRepo.Save(ctx, targetFile); err != nil {
		return fmt.Errorf("failed to save target file: %w", err)
	}

	conflict.Resolve(entities.ConflictResolutionKeepTarget)
	return s.conflictRepo.Save(ctx, conflict)
}

func (s *SyncDomainService) keepBothFiles(ctx context.Context, conflict *entities.Conflict) error {
	sourceFile := conflict.SourceFile()
	targetFile := conflict.TargetFile()

	sourceFile.MarkAsSynced()
	targetFile.MarkAsSynced()

	if err := s.fileRepo.Save(ctx, sourceFile); err != nil {
		return fmt.Errorf("failed to save source file: %w", err)
	}

	if err := s.fileRepo.Save(ctx, targetFile); err != nil {
		return fmt.Errorf("failed to save target file: %w", err)
	}

	conflict.Resolve(entities.ConflictResolutionKeepBoth)
	return s.conflictRepo.Save(ctx, conflict)
}
