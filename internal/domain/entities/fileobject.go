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

type FileObject struct {
	id         valueobjects.FileID
	path       valueobjects.FilePath
	hash       valueobjects.FileHash
	size       int64
	status     FileStatus
	nodeID     valueobjects.StorageNodeID
	createdAt  time.Time
	modifiedAt time.Time
	syncedAt   *time.Time
	version    int64
}

func NewFileObject(
	path valueobjects.FilePath,
	hash valueobjects.FileHash,
	size int64,
	nodeID valueobjects.StorageNodeID,
) *FileObject {
	now := time.Now()
	return &FileObject{
		id:         valueobjects.NewFileID(),
		path:       path,
		hash:       hash,
		size:       size,
		status:     FileStatusCreated,
		nodeID:     nodeID,
		createdAt:  now,
		modifiedAt: now,
		version:    1,
	}
}

func (f *FileObject) ID() valueobjects.FileID {
	return f.id
}

func (f *FileObject) Path() valueobjects.FilePath {
	return f.path
}

func (f *FileObject) Hash() valueobjects.FileHash {
	return f.hash
}

func (f *FileObject) Size() int64 {
	return f.size
}

func (f *FileObject) Status() FileStatus {
	return f.status
}

func (f *FileObject) NodeID() valueobjects.StorageNodeID {
	return f.nodeID
}

func (f *FileObject) CreatedAt() time.Time {
	return f.createdAt
}

func (f *FileObject) ModifiedAt() time.Time {
	return f.modifiedAt
}

func (f *FileObject) SyncedAt() *time.Time {
	return f.syncedAt
}

func (f *FileObject) Version() int64 {
	return f.version
}

func (f *FileObject) UpdateContent(newHash valueobjects.FileHash, newSize int64) {
	if !f.hash.Equals(newHash) || f.size != newSize {
		f.hash = newHash
		f.size = newSize
		f.status = FileStatusModified
		f.modifiedAt = time.Now()
		f.version++
	}
}

func (f *FileObject) MarkAsSynced() {
	f.status = FileStatusSynced
	now := time.Now()
	f.syncedAt = &now
}

func (f *FileObject) MarkAsConflicted() {
	f.status = FileStatusConflict
}

func (f *FileObject) MarkAsDeleted() {
	f.status = FileStatusDeleted
	f.modifiedAt = time.Now()
}

func (f *FileObject) MoveToNode(newNodeID valueobjects.StorageNodeID) {
	f.nodeID = newNodeID
	f.modifiedAt = time.Now()
}

func (f *FileObject) HasSameContent(other *FileObject) bool {
	return f.hash.Equals(other.hash) && f.size == other.size
}

func (f *FileObject) IsNewerThan(other *FileObject) bool {
	return f.modifiedAt.After(other.modifiedAt)
}
