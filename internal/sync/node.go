package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Node — единый интерфейс для любого участника синхронизации.
// SyncEngine работает только с этим интерфейсом и не знает что за ним стоит.
type Node interface {
	ID() string
	Type() string
	ListFiles(ctx context.Context) ([]FileEntry, error)
	ReadFile(ctx context.Context, path string) (io.ReadCloser, error)
	WriteFile(ctx context.Context, path string, r io.Reader) error
	DeleteFile(ctx context.Context, path string) error
	MkDir(ctx context.Context, path string) error
}

// =============================================================================
// SimpleStorage — любая машина с ОС (Linux, macOS, Windows).
// Работает напрямую через файловую систему.
// Примеры: ноутбук, сервер, NAS, Raspberry Pi.
// =============================================================================

type SimpleStorage struct {
	id       string
	rootPath string // корневая папка синхронизации на этой машине
}

func NewSimpleStorage(id, rootPath string) *SimpleStorage {
	return &SimpleStorage{id: id, rootPath: rootPath}
}

func (s *SimpleStorage) ID() string   { return s.id }
func (s *SimpleStorage) Type() string { return "simple" }

func (s *SimpleStorage) ListFiles(_ context.Context) ([]FileEntry, error) {
	var entries []FileEntry
	err := filepath.WalkDir(s.rootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(s.rootPath, path)
		hash, err := hashLocalFile(path)
		if err != nil {
			return err
		}
		info, _ := d.Info()
		entries = append(entries, FileEntry{Path: rel, Hash: hash, Size: info.Size()})
		return nil
	})
	return entries, err
}

func (s *SimpleStorage) ReadFile(_ context.Context, path string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(s.rootPath, path))
}

func (s *SimpleStorage) WriteFile(_ context.Context, path string, r io.Reader) error {
	full := filepath.Join(s.rootPath, path)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return err
	}
	f, err := os.Create(full)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

func (s *SimpleStorage) DeleteFile(_ context.Context, path string) error {
	return os.Remove(filepath.Join(s.rootPath, path))
}

func (s *SimpleStorage) MkDir(_ context.Context, path string) error {
	return os.MkdirAll(filepath.Join(s.rootPath, path), 0755)
}

// =============================================================================
// ComplexStorageAPI — интерфейс который должно реализовать любое
// специализированное хранилище (Google Drive, S3, Dropbox, SFTP и т.д.)
// =============================================================================

type ComplexStorageAPI interface {
	// ListFiles возвращает список файлов в папке (folderID="" — корень)
	ListFiles(ctx context.Context, folderID string) ([]RemoteFile, error)

	// Download скачивает файл по его ID в локальный путь
	Download(ctx context.Context, fileID, localPath string) error

	// Upload загружает локальный файл в папку folderID, возвращает ID нового файла
	Upload(ctx context.Context, localPath, folderID string) (string, error)

	// Delete удаляет файл по ID
	Delete(ctx context.Context, fileID string) error

	// MkDir создаёт папку внутри parentID, возвращает ID новой папки
	MkDir(ctx context.Context, name, parentID string) (string, error)
}

// RemoteFile — описание файла в удалённом хранилище
type RemoteFile struct {
	ID       string // уникальный ID в системе хранилища
	Path     string // логический путь (относительный)
	Hash     string // хеш содержимого (если хранилище его предоставляет)
	Size     int64
	IsDir    bool
	MimeType string
}

// =============================================================================
// ComplexStorage — обёртка над любым специализированным хранилищем.
// Реализует Node через ComplexStorageAPI.
// Примеры: Google Drive, Amazon S3, Dropbox, SFTP-сервер.
// =============================================================================

type ComplexStorage struct {
	id     string
	kind   string // "google_drive", "s3", "dropbox", "sftp" — для логов
	api    ComplexStorageAPI
	tmpDir string // временная папка для промежуточных файлов
}

func NewComplexStorage(id, kind, tmpDir string, api ComplexStorageAPI) *ComplexStorage {
	return &ComplexStorage{id: id, kind: kind, api: api, tmpDir: tmpDir}
}

func (s *ComplexStorage) ID() string   { return s.id }
func (s *ComplexStorage) Type() string { return s.kind }

func (s *ComplexStorage) ListFiles(ctx context.Context) ([]FileEntry, error) {
	return s.listRecursive(ctx, "", "")
}

func (s *ComplexStorage) listRecursive(ctx context.Context, folderID, prefix string) ([]FileEntry, error) {
	items, err := s.api.ListFiles(ctx, folderID)
	if err != nil {
		return nil, err
	}

	var entries []FileEntry
	for _, item := range items {
		logicalPath := item.Path
		if prefix != "" {
			logicalPath = prefix + "/" + item.Path
		}

		if item.IsDir {
			sub, err := s.listRecursive(ctx, item.ID, logicalPath)
			if err != nil {
				return nil, err
			}
			entries = append(entries, sub...)
		} else {
			entries = append(entries, FileEntry{
				Path: logicalPath,
				Hash: item.Hash,
				Size: item.Size,
			})
		}
	}
	return entries, nil
}

func (s *ComplexStorage) ReadFile(ctx context.Context, path string) (io.ReadCloser, error) {
	// Находим ID файла по пути через листинг
	fileID, err := s.resolveID(ctx, path)
	if err != nil {
		return nil, err
	}

	tmp := filepath.Join(s.tmpDir, filepath.Base(path))
	if err := s.api.Download(ctx, fileID, tmp); err != nil {
		return nil, err
	}

	f, err := os.Open(tmp)
	if err != nil {
		return nil, err
	}
	// Удаляем tmp после закрытия
	return &autoRemoveFile{File: f, path: tmp}, nil
}

func (s *ComplexStorage) WriteFile(ctx context.Context, path string, r io.Reader) error {
	// Сохраняем во временный файл
	tmp := filepath.Join(s.tmpDir, filepath.Base(path))
	if err := os.MkdirAll(s.tmpDir, 0755); err != nil {
		return err
	}
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	defer os.Remove(tmp)

	// Определяем папку назначения
	dir := filepath.Dir(path)
	folderID := ""
	if dir != "." && dir != "" {
		folderID, err = s.ensureDir(ctx, dir)
		if err != nil {
			return err
		}
	}

	_, err = s.api.Upload(ctx, tmp, folderID)
	return err
}

func (s *ComplexStorage) DeleteFile(ctx context.Context, path string) error {
	fileID, err := s.resolveID(ctx, path)
	if err != nil {
		return err
	}
	return s.api.Delete(ctx, fileID)
}

func (s *ComplexStorage) MkDir(ctx context.Context, path string) error {
	_, err := s.ensureDir(ctx, path)
	return err
}

// resolveID находит ID файла по логическому пути через листинг
func (s *ComplexStorage) resolveID(ctx context.Context, path string) (string, error) {
	items, err := s.api.ListFiles(ctx, "")
	if err != nil {
		return "", err
	}
	for _, item := range items {
		if item.Path == path {
			return item.ID, nil
		}
	}
	return "", os.ErrNotExist
}

// ensureDir создаёт вложенные папки и возвращает ID конечной
func (s *ComplexStorage) ensureDir(ctx context.Context, path string) (string, error) {
	parts := splitPath(path)
	parentID := ""
	for _, part := range parts {
		id, err := s.api.MkDir(ctx, part, parentID)
		if err != nil {
			return "", err
		}
		parentID = id
	}
	return parentID, nil
}

// =============================================================================
// autoRemoveFile — io.ReadCloser который удаляет tmp файл после закрытия
// =============================================================================

type autoRemoveFile struct {
	*os.File
	path string
}

func (f *autoRemoveFile) Close() error {
	err := f.File.Close()
	os.Remove(f.path)
	return err
}

// =============================================================================
// helpers
// =============================================================================

func hashLocalFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func splitPath(path string) []string {
	var parts []string
	for _, p := range strings.Split(path, "/") {
		if p != "" && p != "." {
			parts = append(parts, p)
		}
	}
	return parts
}
