package valueobjects

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

type FileHash struct {
	value string
}

func NewFileHash(data []byte) FileHash {
	hash := sha256.Sum256(data)
	return FileHash{value: hex.EncodeToString(hash[:])}
}

func FileHashFromString(s string) (FileHash, error) {
	if len(s) != 64 {
		return FileHash{}, fmt.Errorf("invalid hash length: expected 64 characters, got %d", len(s))
	}

	if _, err := hex.DecodeString(s); err != nil {
		return FileHash{}, fmt.Errorf("invalid hex format: %w", err)
	}

	return FileHash{value: s}, nil
}

func (f FileHash) String() string {
	return f.value
}

func (f FileHash) IsEmpty() bool {
	return f.value == ""
}

func (f FileHash) Equals(other FileHash) bool {
	return f.value == other.value
}

func (f FileHash) IsValid() bool {
	if len(f.value) != 64 {
		return false
	}
	_, err := hex.DecodeString(f.value)
	return err == nil
}

func (f *FileHash) MarshalJSON() ([]byte, error) {
	return []byte(`"` + f.value + `"`), nil
}

func (f *FileHash) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	f.value = s
	return nil
}
