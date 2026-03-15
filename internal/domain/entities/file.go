package entities

import (
	"time"

	"syncvault/internal/domain/valueobjects"
)

type FileStatus string

const (
	FileStatusCreated  FileStatus = "created"
	FileStatusModified FileStatus = "modified"
	FileStatusDeleted  FileStatus = "deleted"
	FileStatusSynced   FileStatus = "synced"
	FileStatusConflict FileStatus = "conflict"
)

type File struct {
	ID            int64
	FilePath      valueobjects.FilePath
	FileHash      valueobjects.FileHash
	FileSize      int64
	FileStatus    FileStatus
	StorageNodeID valueobjects.StorageNodeID
	UserID        int64
	CreatedAt     time.Time
	ModifiedAt    time.Time
	SyncedAt      *time.Time
	Version       int64
}

func NewFile(
	path valueobjects.FilePath,
	hash valueobjects.FileHash,
	size int64,
	nodeID valueobjects.StorageNodeID,
) *File {
	now := time.Now()
	return &File{
		FilePath:      path,
		FileHash:      hash,
		FileSize:      size,
		FileStatus:    FileStatusCreated,
		StorageNodeID: nodeID,
		CreatedAt:     now,
		ModifiedAt:    now,
		Version:       1,
	}
}

// HasSameContent checks if two files have the same content
func (f *File) HasSameContent(other *File) bool {
	return f.FileHash.String() == other.FileHash.String()
}

// Status returns the file status
func (f *File) Status() FileStatus {
	return f.FileStatus
}

// UpdatedAt returns the modification time
func (f *File) UpdatedAt() time.Time {
	return f.ModifiedAt
}

// MarkAsSynced marks the file as synced
func (f *File) MarkAsSynced() {
	f.FileStatus = FileStatusSynced
	now := time.Now()
	f.ModifiedAt = now
}
