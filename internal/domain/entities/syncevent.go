package entities

import (
	"time"

	"syncvault/internal/domain/valueobjects"
)

type EventType string

const (
	EventTypeFileCreated      EventType = "file_created"
	EventTypeFileModified     EventType = "file_modified"
	EventTypeFileDeleted      EventType = "file_deleted"
	EventTypeFileMoved        EventType = "file_moved"
	EventTypeSyncStarted      EventType = "sync_started"
	EventTypeSyncCompleted    EventType = "sync_completed"
	EventTypeSyncFailed       EventType = "sync_failed"
	EventTypeConflictDetected EventType = "conflict_detected"
	EventTypeConflictResolved EventType = "conflict_resolved"
)

type SyncEvent struct {
	id          valueobjects.FileID
	eventType   EventType
	nodeID      valueobjects.StorageNodeID
	filePath    valueobjects.FilePath
	description string
	timestamp   time.Time
	metadata    map[string]string
	jobID       *valueobjects.SyncJobID
}

func NewSyncEvent(
	eventType EventType,
	nodeID valueobjects.StorageNodeID,
	filePath valueobjects.FilePath,
	description string,
) *SyncEvent {
	return &SyncEvent{
		id:          valueobjects.NewFileID(),
		eventType:   eventType,
		nodeID:      nodeID,
		filePath:    filePath,
		description: description,
		timestamp:   time.Now(),
		metadata:    make(map[string]string),
	}
}

func (e *SyncEvent) ID() valueobjects.FileID {
	return e.id
}

func (e *SyncEvent) EventType() EventType {
	return e.eventType
}

func (e *SyncEvent) NodeID() valueobjects.StorageNodeID {
	return e.nodeID
}

func (e *SyncEvent) FilePath() valueobjects.FilePath {
	return e.filePath
}

func (e *SyncEvent) Description() string {
	return e.description
}

func (e *SyncEvent) Timestamp() time.Time {
	return e.timestamp
}

func (e *SyncEvent) Metadata() map[string]string {
	return e.metadata
}

func (e *SyncEvent) JobID() *valueobjects.SyncJobID {
	return e.jobID
}

func (e *SyncEvent) SetJobID(jobID valueobjects.SyncJobID) {
	e.jobID = &jobID
}

func (e *SyncEvent) SetMetadata(key, value string) {
	if e.metadata == nil {
		e.metadata = make(map[string]string)
	}
	e.metadata[key] = value
}

func (e *SyncEvent) GetMetadata(key string) string {
	if e.metadata == nil {
		return ""
	}
	return e.metadata[key]
}

func (e *SyncEvent) IsFileEvent() bool {
	return e.eventType == EventTypeFileCreated ||
		e.eventType == EventTypeFileModified ||
		e.eventType == EventTypeFileDeleted ||
		e.eventType == EventTypeFileMoved
}

func (e *SyncEvent) IsSyncEvent() bool {
	return e.eventType == EventTypeSyncStarted ||
		e.eventType == EventTypeSyncCompleted ||
		e.eventType == EventTypeSyncFailed
}

func (e *SyncEvent) IsConflictEvent() bool {
	return e.eventType == EventTypeConflictDetected ||
		e.eventType == EventTypeConflictResolved
}

func (e *SyncEvent) IsFailureEvent() bool {
	return e.eventType == EventTypeSyncFailed
}

func (e *SyncEvent) IsSuccessEvent() bool {
	return e.eventType == EventTypeSyncCompleted ||
		e.eventType == EventTypeConflictResolved
}
