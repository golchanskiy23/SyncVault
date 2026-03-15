package domainevents

import (
	"time"

	"syncvault/internal/domain/entities"
	"syncvault/internal/domain/valueobjects"
)

type DomainEvent interface {
	EventID() string
	EventType() string
	OccurredAt() time.Time
	AggregateID() string
}

type FileCreated struct {
	eventID     string
	occurredAt  time.Time
	fileID      valueobjects.FileID
	nodeID      valueobjects.StorageNodeID
	filePath    valueobjects.FilePath
	fileHash    valueobjects.FileHash
	fileSize    int64
}

func NewFileCreated(
	fileID valueobjects.FileID,
	nodeID valueobjects.StorageNodeID,
	filePath valueobjects.FilePath,
	fileHash valueobjects.FileHash,
	fileSize int64,
) *FileCreated {
	return &FileCreated{
		eventID:    valueobjects.NewFileID().String(),
		occurredAt: time.Now(),
		fileID:     fileID,
		nodeID:     nodeID,
		filePath:   filePath,
		fileHash:   fileHash,
		fileSize:   fileSize,
	}
}

func (e *FileCreated) EventID() string {
	return e.eventID
}

func (e *FileCreated) EventType() string {
	return "FileCreated"
}

func (e *FileCreated) OccurredAt() time.Time {
	return e.occurredAt
}

func (e *FileCreated) AggregateID() string {
	return e.fileID.String()
}

func (e *FileCreated) FileID() valueobjects.FileID {
	return e.fileID
}

func (e *FileCreated) NodeID() valueobjects.StorageNodeID {
	return e.nodeID
}

func (e *FileCreated) FilePath() valueobjects.FilePath {
	return e.filePath
}

func (e *FileCreated) FileHash() valueobjects.FileHash {
	return e.fileHash
}

func (e *FileCreated) FileSize() int64 {
	return e.fileSize
}

type FileSynced struct {
	eventID     string
	occurredAt  time.Time
	fileID      valueobjects.FileID
	sourceNode  valueobjects.StorageNodeID
	targetNode  valueobjects.StorageNodeID
	syncJobID   valueobjects.SyncJobID
	filePath    valueobjects.FilePath
}

func NewFileSynced(
	fileID valueobjects.FileID,
	sourceNode, targetNode valueobjects.StorageNodeID,
	syncJobID valueobjects.SyncJobID,
	filePath valueobjects.FilePath,
) *FileSynced {
	return &FileSynced{
		eventID:    valueobjects.NewFileID().String(),
		occurredAt: time.Now(),
		fileID:     fileID,
		sourceNode: sourceNode,
		targetNode: targetNode,
		syncJobID:  syncJobID,
		filePath:   filePath,
	}
}

func (e *FileSynced) EventID() string {
	return e.eventID
}

func (e *FileSynced) EventType() string {
	return "FileSynced"
}

func (e *FileSynced) OccurredAt() time.Time {
	return e.occurredAt
}

func (e *FileSynced) AggregateID() string {
	return e.syncJobID.String()
}

func (e *FileSynced) FileID() valueobjects.FileID {
	return e.fileID
}

func (e *FileSynced) SourceNode() valueobjects.StorageNodeID {
	return e.sourceNode
}

func (e *FileSynced) TargetNode() valueobjects.StorageNodeID {
	return e.targetNode
}

func (e *FileSynced) SyncJobID() valueobjects.SyncJobID {
	return e.syncJobID
}

func (e *FileSynced) FilePath() valueobjects.FilePath {
	return e.filePath
}

type ConflictDetected struct {
	eventID      string
	occurredAt   time.Time
	conflictID   valueobjects.ConflictID
	fileID       valueobjects.FileID
	sourceNode   valueobjects.StorageNodeID
	targetNode   valueobjects.StorageNodeID
	conflictType entities.ConflictType
	description  string
	sourceFile   *entities.FileObject
	targetFile   *entities.FileObject
}

func NewConflictDetected(
	conflictID valueobjects.ConflictID,
	fileID valueobjects.FileID,
	sourceNode, targetNode valueobjects.StorageNodeID,
	conflictType entities.ConflictType,
	description string,
	sourceFile, targetFile *entities.FileObject,
) *ConflictDetected {
	return &ConflictDetected{
		eventID:      valueobjects.NewFileID().String(),
		occurredAt:   time.Now(),
		conflictID:   conflictID,
		fileID:       fileID,
		sourceNode:   sourceNode,
		targetNode:   targetNode,
		conflictType: conflictType,
		description:  description,
		sourceFile:   sourceFile,
		targetFile:   targetFile,
	}
}

func (e *ConflictDetected) EventID() string {
	return e.eventID
}

func (e *ConflictDetected) EventType() string {
	return "ConflictDetected"
}

func (e *ConflictDetected) OccurredAt() time.Time {
	return e.occurredAt
}

func (e *ConflictDetected) AggregateID() string {
	return e.conflictID.String()
}

func (e *ConflictDetected) ConflictID() valueobjects.ConflictID {
	return e.conflictID
}

func (e *ConflictDetected) FileID() valueobjects.FileID {
	return e.fileID
}

func (e *ConflictDetected) SourceNode() valueobjects.StorageNodeID {
	return e.sourceNode
}

func (e *ConflictDetected) TargetNode() valueobjects.StorageNodeID {
	return e.targetNode
}

func (e *ConflictDetected) ConflictType() entities.ConflictType {
	return e.conflictType
}

func (e *ConflictDetected) Description() string {
	return e.description
}

func (e *ConflictDetected) SourceFile() *entities.FileObject {
	return e.sourceFile
}

func (e *ConflictDetected) TargetFile() *entities.FileObject {
	return e.targetFile
}

type SyncJobCompleted struct {
	eventID     string
	occurredAt  time.Time
	syncJobID   valueobjects.SyncJobID
	sourceNode  valueobjects.StorageNodeID
	targetNode  valueobjects.StorageNodeID
	status      string
	totalFiles  int
	successful  int
	failed      int
}

func NewSyncJobCompleted(
	syncJobID valueobjects.SyncJobID,
	sourceNode, targetNode valueobjects.StorageNodeID,
	status string,
	totalFiles, successful, failed int,
) *SyncJobCompleted {
	return &SyncJobCompleted{
		eventID:    valueobjects.NewFileID().String(),
		occurredAt: time.Now(),
		syncJobID:  syncJobID,
		sourceNode: sourceNode,
		targetNode: targetNode,
		status:     status,
		totalFiles: totalFiles,
		successful: successful,
		failed:     failed,
	}
}

func (e *SyncJobCompleted) EventID() string {
	return e.eventID
}

func (e *SyncJobCompleted) EventType() string {
	return "SyncJobCompleted"
}

func (e *SyncJobCompleted) OccurredAt() time.Time {
	return e.occurredAt
}

func (e *SyncJobCompleted) AggregateID() string {
	return e.syncJobID.String()
}

func (e *SyncJobCompleted) SyncJobID() valueobjects.SyncJobID {
	return e.syncJobID
}

func (e *SyncJobCompleted) SourceNode() valueobjects.StorageNodeID {
	return e.sourceNode
}

func (e *SyncJobCompleted) TargetNode() valueobjects.StorageNodeID {
	return e.targetNode
}

func (e *SyncJobCompleted) Status() string {
	return e.status
}

func (e *SyncJobCompleted) TotalFiles() int {
	return e.totalFiles
}

func (e *SyncJobCompleted) Successful() int {
	return e.successful
}

func (e *SyncJobCompleted) Failed() int {
	return e.failed
}
