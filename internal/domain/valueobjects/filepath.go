package valueobjects

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type FilePath struct {
	value string
}

func NewFilePath(path string) (FilePath, error) {
	if path == "" {
		return FilePath{}, fmt.Errorf("file path cannot be empty")
	}

	cleanPath := filepath.Clean(path)

	if !filepath.IsAbs(cleanPath) {
		return FilePath{}, fmt.Errorf("file path must be absolute: %s", path)
	}

	return FilePath{value: cleanPath}, nil
}

func FilePathFromString(s string) FilePath {
	return FilePath{value: s}
}

func (f FilePath) String() string {
	return f.value
}

func (f FilePath) IsEmpty() bool {
	return f.value == ""
}

func (f FilePath) Equals(other FilePath) bool {
	return f.value == other.value
}

func (f FilePath) BaseName() string {
	return filepath.Base(f.value)
}

func (f FilePath) Dir() string {
	return filepath.Dir(f.value)
}

func (f FilePath) Ext() string {
	return filepath.Ext(f.value)
}

func (f FilePath) HasExtension(ext string) bool {
	return strings.EqualFold(f.Ext(), ext)
}

func (f FilePath) Join(parts ...string) FilePath {
	joined := filepath.Join(append([]string{f.value}, parts...)...)
	return FilePath{value: joined}
}

func (f FilePath) RelativeTo(base FilePath) (string, error) {
	rel, err := filepath.Rel(base.value, f.value)
	if err != nil {
		return "", fmt.Errorf("cannot get relative path: %w", err)
	}
	return rel, nil
}

func (f *FilePath) MarshalJSON() ([]byte, error) {
	return []byte(`"` + f.value + `"`), nil
}

func (f *FilePath) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	f.value = s
	return nil
}
