package entities

import (
	"time"

	"syncvault/internal/domain/valueobjects"
)

type ConflictType string

const (
	ConflictTypeContent    ConflictType = "content"
	ConflictTypeDeletion   ConflictType = "deletion"
	ConflictTypeRename     ConflictType = "rename"
	ConflictTypePermission ConflictType = "permission"
)

type ConflictResolution string

const (
	ConflictResolutionKeepSource  ConflictResolution = "keep_source"
	ConflictResolutionKeepTarget  ConflictResolution = "keep_target"
	ConflictResolutionKeepBoth    ConflictResolution = "keep_both"
	ConflictResolutionManualMerge ConflictResolution = "manual_merge"
)

type Conflict struct {
	id           valueobjects.ConflictID
	fileID       valueobjects.FileID
	sourceNode   valueobjects.StorageNodeID
	targetNode   valueobjects.StorageNodeID
	conflictType ConflictType
	description  string
	resolution   *ConflictResolution
	resolvedAt   *time.Time
	createdAt    time.Time
	UpdatedAt    time.Time
	sourceFile   *File
	targetFile   *File
}

func NewConflict(
	fileID valueobjects.FileID,
	sourceNode, targetNode valueobjects.StorageNodeID,
	conflictType ConflictType,
	description string,
	sourceFile, targetFile *File,
) *Conflict {
	now := time.Now()
	return &Conflict{
		id:           valueobjects.NewConflictID(),
		fileID:       fileID,
		sourceNode:   sourceNode,
		targetNode:   targetNode,
		conflictType: conflictType,
		description:  description,
		createdAt:    now,
		UpdatedAt:    now,
		sourceFile:   sourceFile,
		targetFile:   targetFile,
	}
}

func (c *Conflict) ID() valueobjects.ConflictID {
	return c.id
}

func (c *Conflict) FileID() valueobjects.FileID {
	return c.fileID
}

func (c *Conflict) SourceNode() valueobjects.StorageNodeID {
	return c.sourceNode
}

func (c *Conflict) TargetNode() valueobjects.StorageNodeID {
	return c.targetNode
}

func (c *Conflict) ConflictType() ConflictType {
	return c.conflictType
}

func (c *Conflict) Description() string {
	return c.description
}

func (c *Conflict) Resolution() *ConflictResolution {
	return c.resolution
}

func (c *Conflict) ResolvedAt() *time.Time {
	return c.resolvedAt
}

func (c *Conflict) CreatedAt() time.Time {
	return c.createdAt
}

func (c *Conflict) SourceFile() *File {
	return c.sourceFile
}

func (c *Conflict) TargetFile() *File {
	return c.targetFile
}

func (c *Conflict) IsResolved() bool {
	return c.resolution != nil
}

func (c *Conflict) Resolve(resolution ConflictResolution) {
	c.resolution = &resolution
	now := time.Now()
	c.resolvedAt = &now
}

func (c *Conflict) IsContentConflict() bool {
	return c.conflictType == ConflictTypeContent
}

func (c *Conflict) IsDeletionConflict() bool {
	return c.conflictType == ConflictTypeDeletion
}

func (c *Conflict) IsRenameConflict() bool {
	return c.conflictType == ConflictTypeRename
}

func (c *Conflict) IsPermissionConflict() bool {
	return c.conflictType == ConflictTypePermission
}

func (c *Conflict) GetSeverity() ConflictSeverity {
	if c.conflictType == ConflictTypeContent {
		return ConflictSeverityHigh
	}
	if c.conflictType == ConflictTypeDeletion {
		return ConflictSeverityMedium
	}
	return ConflictSeverityLow
}

type ConflictSeverity string

const (
	ConflictSeverityLow    ConflictSeverity = "low"
	ConflictSeverityMedium ConflictSeverity = "medium"
	ConflictSeverityHigh   ConflictSeverity = "high"
)
