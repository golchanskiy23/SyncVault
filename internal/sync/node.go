package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
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
		info, err := d.Info()
		if err != nil {
			return err
		}
		entries = append(entries, FileEntry{Path: filepath.ToSlash(rel), Hash: hash, Size: info.Size(), ModTime: info.ModTime()})
		return nil
	})
	return entries, err
}

// resolve переводит относительный путь в путь внутри rootPath; пути, выходящие за его пределы
// ("../x", абсолютные), отклоняются — их может прислать сервер или агент
func (s *SimpleStorage) resolve(path string) (string, error) {
	local := filepath.FromSlash(path)
	if !filepath.IsLocal(local) {
		return "", fmt.Errorf("path %q is outside of the sync folder", path)
	}
	return filepath.Join(s.rootPath, local), nil
}

func (s *SimpleStorage) ReadFile(_ context.Context, path string) (io.ReadCloser, error) {
	full, err := s.resolve(path)
	if err != nil {
		return nil, err
	}
	return os.Open(full)
}

func (s *SimpleStorage) WriteFile(_ context.Context, path string, r io.Reader) error {
	full, err := s.resolve(path)
	if err != nil {
		return err
	}
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
	full, err := s.resolve(path)
	if err != nil {
		return err
	}
	return os.Remove(full)
}

func (s *SimpleStorage) MkDir(_ context.Context, path string) error {
	full, err := s.resolve(path)
	if err != nil {
		return err
	}
	return os.MkdirAll(full, 0755)
}

// =============================================================================
// ComplexStorageAPI — интерфейс который должно реализовать любое
// специализированное хранилище (Google Drive, S3, Dropbox, SFTP и т.д.)
// folderID == "" везде означает корневую папку узла.
// =============================================================================

type ComplexStorageAPI interface {
	// ListFiles возвращает содержимое папки (только непосредственные потомки)
	ListFiles(ctx context.Context, folderID string) ([]RemoteFile, error)

	// Download скачивает файл по его ID в локальный путь
	Download(ctx context.Context, fileID, localPath string) error

	// Upload загружает локальный файл в папку folderID под именем filepath.Base(localPath),
	// возвращает ID нового файла
	Upload(ctx context.Context, localPath, folderID string) (string, error)

	// Update заменяет содержимое существующего файла
	Update(ctx context.Context, fileID, localPath string) error

	// Delete удаляет файл по ID
	Delete(ctx context.Context, fileID string) error

	// MkDir создаёт папку внутри parentID, возвращает ID новой папки
	MkDir(ctx context.Context, name, parentID string) (string, error)
}

// RemoteFile — описание файла в удалённом хранилище
type RemoteFile struct {
	ID       string // уникальный ID в системе хранилища
	Path     string // имя внутри родительской папки
	Hash     string // SHA-256 содержимого (если хранилище его предоставляет)
	Size     int64
	IsDir    bool
	MimeType string
	ModTime  time.Time
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
	seen := make(map[string]bool, len(items))
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
			continue
		}

		// В хранилище могут быть файлы с одинаковыми именами в одной папке — синхронизируем первый
		if seen[logicalPath] {
			log.Printf("ComplexStorage %s: duplicate file %s skipped", s.id, logicalPath)
			continue
		}
		seen[logicalPath] = true
		entries = append(entries, FileEntry{
			Path:    logicalPath,
			Hash:    item.Hash,
			Size:    item.Size,
			ModTime: item.ModTime,
		})
	}
	return entries, nil
}

func (s *ComplexStorage) ReadFile(ctx context.Context, path string) (io.ReadCloser, error) {
	file, err := s.resolve(ctx, path)
	if err != nil {
		return nil, err
	}

	dir, err := s.makeTmpDir()
	if err != nil {
		return nil, err
	}
	tmp := filepath.Join(dir, filepath.Base(path))
	if err := s.api.Download(ctx, file.ID, tmp); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}

	f, err := os.Open(tmp)
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	// Удаляем tmp после закрытия
	return &autoRemoveFile{File: f, path: dir}, nil
}

func (s *ComplexStorage) WriteFile(ctx context.Context, path string, r io.Reader) error {
	// Сохраняем во временный файл с именем как у целевого — Upload берёт имя из него
	dir, err := s.makeTmpDir()
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	tmp := filepath.Join(dir, filepath.Base(path))
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	// Файл уже есть — заменяем содержимое, а не создаём дубликат
	existing, err := s.resolve(ctx, path)
	if err == nil {
		return s.api.Update(ctx, existing.ID, tmp)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	folderID, err := s.ensureDir(ctx, parentPath(path))
	if err != nil {
		return err
	}
	_, err = s.api.Upload(ctx, tmp, folderID)
	return err
}

func (s *ComplexStorage) DeleteFile(ctx context.Context, path string) error {
	file, err := s.resolve(ctx, path)
	if err != nil {
		return err
	}
	return s.api.Delete(ctx, file.ID)
}

func (s *ComplexStorage) MkDir(ctx context.Context, path string) error {
	_, err := s.ensureDir(ctx, path)
	return err
}

// resolve находит файл по логическому пути, спускаясь по папкам
func (s *ComplexStorage) resolve(ctx context.Context, path string) (*RemoteFile, error) {
	folderID, err := s.findDir(ctx, parentPath(path), false)
	if err != nil {
		return nil, err
	}
	child, err := s.findChild(ctx, folderID, filepath.Base(path), false)
	if err != nil {
		return nil, err
	}
	if child == nil {
		return nil, fmt.Errorf("%s: %w", path, os.ErrNotExist)
	}
	return child, nil
}

// ensureDir возвращает ID папки по пути, создавая недостающие
func (s *ComplexStorage) ensureDir(ctx context.Context, path string) (string, error) {
	return s.findDir(ctx, path, true)
}

func (s *ComplexStorage) findDir(ctx context.Context, path string, create bool) (string, error) {
	parentID := ""
	for _, part := range splitPath(path) {
		dir, err := s.findChild(ctx, parentID, part, true)
		if err != nil {
			return "", err
		}
		if dir != nil {
			parentID = dir.ID
			continue
		}
		if !create {
			return "", fmt.Errorf("%s: %w", path, os.ErrNotExist)
		}
		id, err := s.api.MkDir(ctx, part, parentID)
		if err != nil {
			return "", err
		}
		parentID = id
	}
	return parentID, nil
}

// findChild ищет в папке элемент с именем name (папку, если isDir); nil — не найден
func (s *ComplexStorage) findChild(ctx context.Context, folderID, name string, isDir bool) (*RemoteFile, error) {
	items, err := s.api.ListFiles(ctx, folderID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].Path == name && items[i].IsDir == isDir {
			return &items[i], nil
		}
	}
	return nil, nil
}

func (s *ComplexStorage) makeTmpDir() (string, error) {
	if err := os.MkdirAll(s.tmpDir, 0755); err != nil {
		return "", err
	}
	return os.MkdirTemp(s.tmpDir, "syncvault-")
}

// =============================================================================
// autoRemoveFile — io.ReadCloser который удаляет tmp после закрытия
// =============================================================================

type autoRemoveFile struct {
	*os.File
	path string // файл или папка, удаляемые после закрытия
}

func (f *autoRemoveFile) Close() error {
	err := f.File.Close()
	os.RemoveAll(f.path)
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

// ResolveInside возвращает путь rel внутри base; rel может быть пустым (сама base),
// относительным или абсолютным, но не может выходить за пределы base
func ResolveInside(base, rel string) (string, error) {
	if rel == "" {
		return base, nil
	}
	outside := fmt.Errorf("path %q is outside of %s", rel, base)
	local := rel
	if filepath.IsAbs(local) {
		r, err := filepath.Rel(base, local)
		if err != nil {
			return "", outside
		}
		local = r
	}
	local = filepath.Clean(filepath.FromSlash(local))
	if local == "." {
		return base, nil
	}
	if !filepath.IsLocal(local) {
		return "", outside
	}
	return filepath.Join(base, local), nil
}

// ListSubdirs возвращает имена подпапок папки rel внутри base
func ListSubdirs(base, rel string) ([]string, error) {
	dir, err := ResolveInside(base, rel)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	dirs := []string{}
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	return dirs, nil
}

// parentPath возвращает папку логического пути ("" для файлов в корне)
func parentPath(path string) string {
	dir := filepath.ToSlash(filepath.Dir(path))
	if dir == "." {
		return ""
	}
	return dir
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
