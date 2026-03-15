package valueobjects

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

type FileID struct {
	value string
}

func generateID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func NewFileID() FileID {
	return FileID{value: generateID()}
}

func FileIDFromString(s string) (FileID, error) {
	if len(s) != 32 {
		return FileID{}, fmt.Errorf("invalid file ID format: expected 32 characters, got %d", len(s))
	}
	if _, err := hex.DecodeString(s); err != nil {
		return FileID{}, fmt.Errorf("invalid file ID format: %w", err)
	}
	return FileID{value: s}, nil
}

func (f FileID) String() string {
	return f.value
}

func (f FileID) IsEmpty() bool {
	return f.value == ""
}

func (f FileID) Equals(other FileID) bool {
	return f.value == other.value
}

type StorageNodeID struct {
	value string
}

func NewStorageNodeID() StorageNodeID {
	return StorageNodeID{value: generateID()}
}

func StorageNodeIDFromString(s string) (StorageNodeID, error) {
	if len(s) != 32 {
		return StorageNodeID{}, fmt.Errorf("invalid storage node ID format: expected 32 characters, got %d", len(s))
	}
	if _, err := hex.DecodeString(s); err != nil {
		return StorageNodeID{}, fmt.Errorf("invalid storage node ID format: %w", err)
	}
	return StorageNodeID{value: s}, nil
}

func (s StorageNodeID) String() string {
	return s.value
}

func (s StorageNodeID) IsEmpty() bool {
	return s.value == ""
}

func (s StorageNodeID) Equals(other StorageNodeID) bool {
	return s.value == other.value
}

type SyncJobID struct {
	value string
}

func NewSyncJobID() SyncJobID {
	return SyncJobID{value: generateID()}
}

func SyncJobIDFromString(s string) (SyncJobID, error) {
	if len(s) != 32 {
		return SyncJobID{}, fmt.Errorf("invalid sync job ID format: expected 32 characters, got %d", len(s))
	}
	if _, err := hex.DecodeString(s); err != nil {
		return SyncJobID{}, fmt.Errorf("invalid sync job ID format: %w", err)
	}
	return SyncJobID{value: s}, nil
}

func (s SyncJobID) String() string {
	return s.value
}

func (s SyncJobID) IsEmpty() bool {
	return s.value == ""
}

func (s SyncJobID) Equals(other SyncJobID) bool {
	return s.value == other.value
}

type ConflictID struct {
	value string
}

func NewConflictID() ConflictID {
	return ConflictID{value: generateID()}
}

func ConflictIDFromString(s string) (ConflictID, error) {
	if len(s) != 32 {
		return ConflictID{}, fmt.Errorf("invalid conflict ID format: expected 32 characters, got %d", len(s))
	}
	if _, err := hex.DecodeString(s); err != nil {
		return ConflictID{}, fmt.Errorf("invalid conflict ID format: %w", err)
	}
	return ConflictID{value: s}, nil
}

func (c ConflictID) String() string {
	return c.value
}

func (c ConflictID) IsEmpty() bool {
	return c.value == ""
}

func (c ConflictID) Equals(other ConflictID) bool {
	return c.value == other.value
}
