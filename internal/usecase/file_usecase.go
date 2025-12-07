package usecase

import (
	"context"
	"fmt"
	"time"

	"syncvault/internal/domain/aggregates"
	"syncvault/internal/domain/entities"
	"syncvault/internal/domain/repositories"
	"syncvault/internal/domain/valueobjects"
)

type SyncJobBuilder struct {
	sourceNode valueobjects.StorageNodeID
	targetNode valueobjects.StorageNodeID
	fileIDs    []valueobjects.FileID
	priority   aggregates.JobPriority
	retryCount int
	timeout    time.Duration
	metadata   map[string]string
	errors     []error
}

func NewSyncJobBuilder() *SyncJobBuilder {
	return &SyncJobBuilder{
		priority:   aggregates.PriorityNormal,
		retryCount: 3,
		timeout:    30 * time.Minute,
		metadata:   make(map[string]string),
		errors:     make([]error, 0),
	}
}

func (b *SyncJobBuilder) SourceNode(nodeID valueobjects.StorageNodeID) *SyncJobBuilder {
	b.sourceNode = nodeID
	return b
}

func (b *SyncJobBuilder) TargetNode(nodeID valueobjects.StorageNodeID) *SyncJobBuilder {
	b.targetNode = nodeID
	return b
}

func (b *SyncJobBuilder) Files(fileIDs []valueobjects.FileID) *SyncJobBuilder {
	b.fileIDs = fileIDs
	return b
}

func (b *SyncJobBuilder) HighPriority() *SyncJobBuilder {
	b.priority = aggregates.PriorityHigh
	return b
}

func (b *SyncJobBuilder) WithRetries(count int) *SyncJobBuilder {
	if count < 0 {
		b.errors = append(b.errors, fmt.Errorf("retry count must be non-negative"))
		return b
	}
	b.retryCount = count
	return b
}

func (b *SyncJobBuilder) Timeout(duration time.Duration) *SyncJobBuilder {
	if duration <= 0 {
		b.errors = append(b.errors, fmt.Errorf("timeout must be positive"))
		return b
	}
	b.timeout = duration
	return b
}

func (b *SyncJobBuilder) Metadata(key, value string) *SyncJobBuilder {
	b.metadata[key] = value
	return b
}

func (b *SyncJobBuilder) Build() (*aggregates.SyncJob, error) {
	if len(b.errors) > 0 {
		return nil, fmt.Errorf("build errors: %v", b.errors)
	}

	if b.sourceNode.IsEmpty() {
		return nil, fmt.Errorf("source node is required")
	}

	if b.targetNode.IsEmpty() {
		return nil, fmt.Errorf("target node is required")
	}

	if len(b.fileIDs) == 0 {
		return nil, fmt.Errorf("at least one file is required")
	}

	job := aggregates.NewSyncJob(b.sourceNode, b.targetNode, b.fileIDs)

	if b.priority != aggregates.PriorityNormal {
		job.SetPriority(b.priority)
	}

	if b.retryCount != 3 {
		job.SetRetryCount(b.retryCount)
	}

	if b.timeout != 30*time.Minute {
		job.SetTimeout(b.timeout)
	}

	for k, v := range b.metadata {
		job.SetMetadata(k, v)
	}

	return job, nil
}

type FileUseCaseConfig struct {
	maxConcurrentSyncs int
	syncTimeout        time.Duration
	enableCompression  bool
	retryAttempts      int
}

type FileUseCaseOption func(*FileUseCaseConfig)

func WithMaxConcurrentSyncs(max int) FileUseCaseOption {
	return func(config *FileUseCaseConfig) {
		config.maxConcurrentSyncs = max
	}
}

func WithSyncTimeout(timeout time.Duration) FileUseCaseOption {
	return func(config *FileUseCaseConfig) {
		config.syncTimeout = timeout
	}
}

func WithCompression(enabled bool) FileUseCaseOption {
	return func(config *FileUseCaseConfig) {
		config.enableCompression = enabled
	}
}

func WithRetryAttempts(attempts int) FileUseCaseOption {
	return func(config *FileUseCaseConfig) {
		config.retryAttempts = attempts
	}
}

type FileUseCase interface {
	SyncFile(ctx context.Context, fileID valueobjects.FileID, sourceNode, targetNode valueobjects.StorageNodeID) error
	GetFileStatus(ctx context.Context, fileID valueobjects.FileID) (*entities.File, error)
	ListFiles(ctx context.Context, nodeID valueobjects.StorageNodeID) ([]*entities.File, error)
}

type FileUseCaseImpl struct {
	fileRepo repositories.FileRepository
	nodeRepo repositories.StorageNodeRepository
	eventBus EventBus
	config   FileUseCaseConfig
}

func NewFileUseCase(
	fileRepo repositories.FileRepository,
	nodeRepo repositories.StorageNodeRepository,
	eventBus EventBus,
	opts ...FileUseCaseOption,
) *FileUseCaseImpl {
	config := FileUseCaseConfig{
		maxConcurrentSyncs: 5,
		syncTimeout:        30 * time.Minute,
		enableCompression:  false,
		retryAttempts:      3,
	}

	for _, opt := range opts {
		opt(&config)
	}

	return &FileUseCaseImpl{
		fileRepo: fileRepo,
		nodeRepo: nodeRepo,
		eventBus: eventBus,
		config:   config,
	}
}

func (uc *FileUseCaseImpl) SyncFile(ctx context.Context, fileID valueobjects.FileID, sourceNode, targetNode valueobjects.StorageNodeID) error {
	file, err := uc.fileRepo.FindByID(ctx, fileID)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}

	srcNode, err := uc.nodeRepo.FindByID(ctx, sourceNode)
	if err != nil {
		return fmt.Errorf("source node not found: %w", err)
	}

	tgtNode, err := uc.nodeRepo.FindByID(ctx, targetNode)
	if err != nil {
		return fmt.Errorf("target node not found: %w", err)
	}

	if !srcNode.IsOnline() || !tgtNode.IsOnline() {
		return fmt.Errorf("nodes must be online")
	}

	if !tgtNode.HasEnoughSpace(file.Size()) {
		return fmt.Errorf("insufficient space on target node")
	}

	if err := uc.syncFile(ctx, file, sourceNode, targetNode); err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	event := map[string]interface{}{
		"type":    "file_synced",
		"file_id": fileID.String(),
		"source":  sourceNode.String(),
		"target":  targetNode.String(),
		"size":    file.Size(),
	}

	return uc.eventBus.Publish(ctx, event)
}

func (uc *FileUseCaseImpl) GetFileStatus(ctx context.Context, fileID valueobjects.FileID) (*entities.File, error) {
	return uc.fileRepo.FindByID(ctx, fileID)
}

func (uc *FileUseCaseImpl) ListFiles(ctx context.Context, nodeID valueobjects.StorageNodeID) ([]*entities.File, error) {
	return uc.fileRepo.FindByNode(ctx, nodeID)
}

func (uc *FileUseCaseImpl) syncFile(ctx context.Context, file *entities.File, sourceNode, targetNode valueobjects.StorageNodeID) error {
	file.MarkAsSynced()
	return uc.fileRepo.Save(ctx, file)
}
