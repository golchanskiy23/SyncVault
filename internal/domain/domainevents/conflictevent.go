package domainevents

import (
	"time"

	"syncvault/internal/domain/entities"
	"syncvault/internal/domain/valueobjects"
)

type ConflictEvent struct {
	eventID      string
	occurredAt   time.Time
	conflictID   valueobjects.ConflictID
	fileID       valueobjects.FileID
	sourceNode   valueobjects.StorageNodeID
	targetNode   valueobjects.StorageNodeID
	conflictType entities.ConflictType
	description  string
	resolved     bool
}

func NewConflictEvent(
	conflictID valueobjects.ConflictID,
	fileID valueobjects.FileID,
	sourceNode, targetNode valueobjects.StorageNodeID,
	conflictType entities.ConflictType,
	description string,
) *ConflictEvent {
	return &ConflictEvent{
		eventID:      valueobjects.NewFileID().String(),
		occurredAt:   time.Now(),
		conflictID:   conflictID,
		fileID:       fileID,
		sourceNode:   sourceNode,
		targetNode:   targetNode,
		conflictType: conflictType,
		description:  description,
		resolved:     false,
	}
}

func (e *ConflictEvent) EventID() string {
	return e.eventID
}

func (e *ConflictEvent) EventType() string {
	return "ConflictEvent"
}

func (e *ConflictEvent) OccurredAt() time.Time {
	return e.occurredAt
}

func (e *ConflictEvent) AggregateID() string {
	return e.conflictID.String()
}

func (e *ConflictEvent) ConflictID() valueobjects.ConflictID {
	return e.conflictID
}

func (e *ConflictEvent) FileID() valueobjects.FileID {
	return e.fileID
}

func (e *ConflictEvent) SourceNode() valueobjects.StorageNodeID {
	return e.sourceNode
}

func (e *ConflictEvent) TargetNode() valueobjects.StorageNodeID {
	return e.targetNode
}

func (e *ConflictEvent) ConflictType() entities.ConflictType {
	return e.conflictType
}

func (e *ConflictEvent) Description() string {
	return e.description
}

func (e *ConflictEvent) IsResolved() bool {
	return e.resolved
}

func (e *ConflictEvent) MarkAsResolved() {
	e.resolved = true
	e.occurredAt = time.Now()
}
