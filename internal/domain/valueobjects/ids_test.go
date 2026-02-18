package valueobjects

import (
	"fmt"
	"testing"
)

// TestFileIDFromString_RejectsNumericIDs documents Bug 1.3:
// FileIDFromString rejects numeric IDs like "123" (not 32-char hex).
func TestFileIDFromString_RejectsNumericIDs(t *testing.T) {
	cases := []string{"42", "123", fmt.Sprintf("%d", int64(999))}
	for _, s := range cases {
		_, err := FileIDFromString(s)
		if err == nil {
			t.Errorf("FileIDFromString(%q) should return error for numeric ID", s)
		}
	}
}

// TestFileIDFromInt64_AcceptsNumericIDs verifies Bug 1.3 fix:
// FileIDFromInt64 creates a valid FileID from a PostgreSQL BIGINT.
func TestFileIDFromInt64_AcceptsNumericIDs(t *testing.T) {
	cases := []int64{1, 42, 999, 1000000}
	for _, id := range cases {
		fid := FileIDFromInt64(id)
		if fid.IsEmpty() {
			t.Errorf("FileIDFromInt64(%d) should not be empty", id)
		}
		expected := fmt.Sprintf("%d", id)
		if fid.String() != expected {
			t.Errorf("FileIDFromInt64(%d).String() = %q, want %q", id, fid.String(), expected)
		}
	}
}

// TestFileIDFromString_AcceptsValidHex verifies preservation:
// 32-char hex strings still work after the fix.
func TestFileIDFromString_AcceptsValidHex(t *testing.T) {
	valid := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"
	fid, err := FileIDFromString(valid)
	if err != nil {
		t.Errorf("FileIDFromString(%q) should succeed: %v", valid, err)
	}
	if fid.String() != valid {
		t.Errorf("expected %q, got %q", valid, fid.String())
	}
}

// TestNewFileID_GeneratesUniqueIDs verifies NewFileID produces unique values.
func TestNewFileID_GeneratesUniqueIDs(t *testing.T) {
	id1 := NewFileID()
	id2 := NewFileID()
	if id1.Equals(id2) {
		t.Error("NewFileID should generate unique IDs")
	}
}

// TestSyncEventID_NotEmpty verifies Bug 1.8 fix: SyncEventID type exists and generates IDs.
func TestSyncEventID_NotEmpty(t *testing.T) {
	id := NewSyncEventID()
	if id.IsEmpty() {
		t.Error("NewSyncEventID should not be empty")
	}
}

// TestSyncEventID_Unique verifies SyncEventID generates unique values.
func TestSyncEventID_Unique(t *testing.T) {
	id1 := NewSyncEventID()
	id2 := NewSyncEventID()
	if id1.String() == id2.String() {
		t.Error("NewSyncEventID should generate unique IDs")
	}
}
