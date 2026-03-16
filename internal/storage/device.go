package storage

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"syncvault/internal/config"
)

// DeviceStorage представляет хранилище на устройстве пользователя
type DeviceStorage struct {
	DeviceID    string
	StoragePath string
	UserID      string

	// Состояние синхронизации
	LastSync   time.Time
	SyncStatus string // "syncing", "synced", "error"

	// Локальная база данных метаданных
	metadataDB *MetadataDB

	// Мьютекс для блокировки файлов
	fileLocks sync.Map
}

// MetadataDB локальная база метаданных файлов
type MetadataDB struct {
	// Здесь может быть SQLite, BoltDB или другая встраиваемая БД
	// Для простоты используем map в памяти
	files map[string]*FileMetadata
	mutex sync.RWMutex
}

// FileMetadata метаданные файла на устройстве
type FileMetadata struct {
	FilePath     string    `json:"file_path"`
	FileHash     string    `json:"file_hash"`
	FileSize     int64     `json:"file_size"`
	ModifiedTime time.Time `json:"modified_time"`
	LastSync     time.Time `json:"last_sync"`
	SyncStatus   string    `json:"sync_status"` // "pending", "synced", "conflict"
	Version      int64     `json:"version"`
	DeviceID     string    `json:"device_id"`
}

// DeviceManager управляет всеми устройствами пользователя
type DeviceManager struct {
	devices map[string]*DeviceStorage
	mutex   sync.RWMutex
	config  *config.Config
}

func NewDeviceManager(cfg *config.Config) *DeviceManager {
	return &DeviceManager{
		devices: make(map[string]*DeviceStorage),
		config:  cfg,
	}
}

// RegisterDevice регистрирует новое устройство
func (dm *DeviceManager) RegisterDevice(ctx context.Context, userID, deviceName, storagePath string) (*DeviceStorage, error) {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	deviceID := fmt.Sprintf("%s_%s_%d", userID, deviceName, time.Now().Unix())

	// Проверяем существует ли директория, создаем если нужно
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		// Создаем директорию если не существует
		if err := os.MkdirAll(storagePath, 0755); err != nil {
			return nil, fmt.Errorf("failed to create storage directory: %s: %w", storagePath, err)
		}
		log.Printf("Created storage directory: %s", storagePath)
	} else if err != nil {
		return nil, fmt.Errorf("failed to access storage path: %s: %w", storagePath, err)
	}

	device := &DeviceStorage{
		DeviceID:    deviceID,
		UserID:      userID,
		StoragePath: storagePath,
		LastSync:    time.Now(),
		SyncStatus:  "online",
		metadataDB: &MetadataDB{
			files: make(map[string]*FileMetadata),
		},
	}

	// Сканируем файлы в хранилище
	if err := device.scanFiles(); err != nil {
		return nil, fmt.Errorf("failed to scan files: %w", err)
	}

	dm.devices[deviceID] = device

	return device, nil
}

// GetDevice получает устройство по ID
func (dm *DeviceManager) GetDevice(deviceID string) (*DeviceStorage, error) {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()

	device, exists := dm.devices[deviceID]
	if !exists {
		return nil, fmt.Errorf("device not found: %s", deviceID)
	}

	return device, nil
}

// ListUserDevices возвращает все устройства пользователя
func (dm *DeviceManager) ListUserDevices(userID string) []*DeviceStorage {
	dm.mutex.RLock()
	defer dm.mutex.RUnlock()

	var userDevices []*DeviceStorage
	for _, device := range dm.devices {
		if device.UserID == userID {
			userDevices = append(userDevices, device)
		}
	}

	return userDevices
}

// scanFiles сканирует файлы в хранилище устройства
func (ds *DeviceStorage) scanFiles() error {
	return filepath.Walk(ds.StoragePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			relPath, err := filepath.Rel(ds.StoragePath, path)
			if err != nil {
				return err
			}

			metadata := &FileMetadata{
				FilePath:     relPath,
				FileSize:     info.Size(),
				ModifiedTime: info.ModTime(),
				SyncStatus:   "pending",
				Version:      1,
				DeviceID:     ds.DeviceID,
			}

			// Вычисляем хеш файла
			if hash, err := ds.calculateFileHash(path); err == nil {
				metadata.FileHash = hash
			}

			// Проверяем, нужно ли синхронизировать файл
			// Для нового файла всегда ставим pending
			metadata.SyncStatus = "pending"
			metadata.Version = 1

			ds.metadataDB.files[relPath] = metadata
		}

		return nil
	})
}

// calculateFileHash вычисляет хеш файла
func (ds *DeviceStorage) calculateFileHash(filePath string) (string, error) {
	// Здесь должна быть реальная реализация хеширования
	// Например, SHA256
	return fmt.Sprintf("hash_%s", filepath.Base(filePath)), nil
}

// GetFile получает файл с устройства
func (ds *DeviceStorage) GetFile(relativePath string) ([]byte, error) {
	fullPath := filepath.Join(ds.StoragePath, relativePath)
	return os.ReadFile(fullPath)
}

// PutFile сохраняет файл на устройство
func (ds *DeviceStorage) PutFile(relativePath string, content []byte) error {
	fullPath := filepath.Join(ds.StoragePath, relativePath)

	// Создаем директорию если нужно
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(fullPath, content, 0644)
}

// SyncFile синхронизирует файл с центральным хранилищем
func (ds *DeviceStorage) SyncFile(ctx context.Context, relativePath string) error {
	// Блокируем файл для предотвращения конфликтов
	ds.fileLocks.Store(relativePath, true)
	defer ds.fileLocks.Delete(relativePath)

	// Получаем метаданные файла
	ds.metadataDB.mutex.RLock()
	metadata, exists := ds.metadataDB.files[relativePath]
	ds.metadataDB.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("file not found: %s", relativePath)
	}

	// Здесь должна быть логика синхронизации с центральным хранилищем
	// через gRPC вызовы Storage Service

	// Обновляем статус синхронизации
	ds.metadataDB.mutex.Lock()
	metadata.SyncStatus = "syncing"
	ds.metadataDB.mutex.Unlock()

	// Имитация синхронизации
	time.Sleep(100 * time.Millisecond)

	ds.metadataDB.mutex.Lock()
	metadata.SyncStatus = "synced"
	metadata.LastSync = time.Now()
	ds.metadataDB.mutex.Unlock()

	return nil
}

// GetPendingFiles возвращает файлы, ожидающие синхронизации
func (ds *DeviceStorage) GetPendingFiles() []*FileMetadata {
	ds.metadataDB.mutex.RLock()
	defer ds.metadataDB.mutex.RUnlock()

	var pending []*FileMetadata
	for _, metadata := range ds.metadataDB.files {
		if metadata.SyncStatus == "pending" {
			pending = append(pending, metadata)
		}
	}

	return pending
}

// GetTotalFiles возвращает общее количество файлов
func (ds *DeviceStorage) GetTotalFiles() int {
	ds.metadataDB.mutex.RLock()
	defer ds.metadataDB.mutex.RUnlock()
	return len(ds.metadataDB.files)
}

// GetCompletedFiles возвращает количество синхронизированных файлов
func (ds *DeviceStorage) GetCompletedFiles() int {
	ds.metadataDB.mutex.RLock()
	defer ds.metadataDB.mutex.RUnlock()

	completed := 0
	for _, metadata := range ds.metadataDB.files {
		if metadata.SyncStatus == "synced" {
			completed++
		}
	}
	return completed
}

// WatchFiles отслеживает изменения файлов
func (ds *DeviceStorage) WatchFiles(ctx context.Context) error {
	// Здесь должна быть реализация отслеживания изменений файлов
	// Используя fsnotify или similar
	return nil
}

// DeleteFile удаляет файл с устройства
func (ds *DeviceStorage) DeleteFile(ctx context.Context, relativePath string) error {
	fullPath := filepath.Join(ds.StoragePath, relativePath)

	// Удаляем файл из файловой системы
	if err := os.Remove(fullPath); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	// Удаляем метаданные
	ds.metadataDB.mutex.Lock()
	delete(ds.metadataDB.files, relativePath)
	ds.metadataDB.mutex.Unlock()

	return nil
}
