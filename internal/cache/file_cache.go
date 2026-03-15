package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"syncvault/internal/domain/entities"
	"syncvault/internal/domain/ports"
	"syncvault/internal/domain/valueobjects"

	"github.com/redis/go-redis/v9"
)

type FileCache struct {
	rdb    *redis.Client
	ttl    time.Duration
	prefix string
}

func NewFileCache(rdb *redis.Client, ttl time.Duration) *FileCache {
	return &FileCache{
		rdb:    rdb,
		ttl:    ttl,
		prefix: "file_metadata:",
	}
}

func (fc *FileCache) key(fileID string) string {
	return fc.prefix + fileID
}

func (fc *FileCache) Get(ctx context.Context, fileID string) (*entities.File, error) {
	data, err := fc.rdb.Get(ctx, fc.key(fileID)).Result()
	if err == redis.Nil {
		return nil, nil // Cache miss
	}
	if err != nil {
		return nil, fmt.Errorf("redis get error: %w", err)
	}

	var file entities.File
	if err := json.Unmarshal([]byte(data), &file); err != nil {
		return nil, fmt.Errorf("json unmarshal error: %w", err)
	}

	return &file, nil
}

// Set сохраняет файл в кэш, используя FileID (UUID) как ключ.
// Все методы Get/Delete также используют FileID.String(), поэтому ключи согласованы.
func (fc *FileCache) Set(ctx context.Context, file *entities.File) error {
	data, err := json.Marshal(file)
	if err != nil {
		return fmt.Errorf("json marshal error: %w", err)
	}

	key := fc.key(file.FileID.String())
	if err := fc.rdb.Set(ctx, key, data, fc.ttl).Err(); err != nil {
		return fmt.Errorf("redis set error: %w", err)
	}

	return nil
}

func (fc *FileCache) Delete(ctx context.Context, fileID string) error {
	if err := fc.rdb.Del(ctx, fc.key(fileID)).Err(); err != nil {
		return fmt.Errorf("redis delete error: %w", err)
	}
	return nil
}

// InvalidateByPattern удаляет из кэша все записи, у которых FilePath соответствует
// паттерну в стиле filepath.Match (например, "*test*", "/docs/*").
// Метод сканирует все ключи с префиксом file_metadata:, десериализует каждый файл
// и удаляет те, чей путь совпадает с паттерном.
func (fc *FileCache) InvalidateByPattern(ctx context.Context, pattern string) error {
	keys, err := fc.rdb.Keys(ctx, fc.prefix+"*").Result()
	if err != nil {
		return fmt.Errorf("redis keys error: %w", err)
	}

	var toDelete []string
	for _, key := range keys {
		data, err := fc.rdb.Get(ctx, key).Result()
		if err != nil {
			continue // ключ мог истечь между Keys и Get — пропускаем
		}

		var file entities.File
		if err := json.Unmarshal([]byte(data), &file); err != nil {
			continue
		}

		matched, err := filepath.Match(pattern, file.FilePath.String())
		if err != nil {
			return fmt.Errorf("invalid pattern %q: %w", pattern, err)
		}
		if matched {
			toDelete = append(toDelete, key)
		}
	}

	if len(toDelete) > 0 {
		if err := fc.rdb.Del(ctx, toDelete...).Err(); err != nil {
			return fmt.Errorf("redis delete multiple error: %w", err)
		}
	}

	return nil
}

// CachedFileRepository — декоратор над FileRepository с кэшированием через Redis.
// Ключ кэша = FileID.String() (UUID). Операции Save/Delete инвалидируют кэш автоматически.
type CachedFileRepository struct {
	underlying ports.FileRepository
	cache      *FileCache
}

func NewCachedFileRepository(underlying ports.FileRepository, cache *FileCache) *CachedFileRepository {
	return &CachedFileRepository{
		underlying: underlying,
		cache:      cache,
	}
}

func (c *CachedFileRepository) FindByID(ctx context.Context, fileID valueobjects.FileID) (*entities.File, error) {
	// Сначала проверяем кэш
	fileIDStr := fileID.String()
	file, err := c.cache.Get(ctx, fileIDStr)
	if err != nil {
		return nil, err
	}
	if file != nil {
		return file, nil // Cache hit
	}

	// Cache miss — идём в БД
	file, err = c.underlying.FindByID(ctx, fileID)
	if err != nil {
		return nil, err
	}

	if file != nil {
		if cacheErr := c.cache.Set(ctx, file); cacheErr != nil {
			fmt.Printf("cache set error: %v\n", cacheErr)
		}
	}

	return file, nil
}

func (c *CachedFileRepository) Save(ctx context.Context, file *entities.File) error {
	// Сначала сохраняем в БД
	if err := c.underlying.Save(ctx, file); err != nil {
		return err
	}

	// Обновляем кэш только при успехе
	if err := c.cache.Set(ctx, file); err != nil {
		fmt.Printf("cache update error: %v\n", err)
	}

	return nil
}

func (c *CachedFileRepository) Delete(ctx context.Context, fileID valueobjects.FileID) error {
	if err := c.underlying.Delete(ctx, fileID); err != nil {
		return err
	}

	if err := c.cache.Delete(ctx, fileID.String()); err != nil {
		fmt.Printf("cache delete error: %v\n", err)
	}

	return nil
}

func (c *CachedFileRepository) FindByNode(ctx context.Context, nodeID valueobjects.StorageNodeID) ([]*entities.File, error) {
	return c.underlying.FindByNode(ctx, nodeID)
}

func (c *CachedFileRepository) FindByPath(ctx context.Context, nodeID valueobjects.StorageNodeID, path valueobjects.FilePath) (*entities.File, error) {
	return c.underlying.FindByPath(ctx, nodeID, path)
}

func (c *CachedFileRepository) FindModifiedSince(ctx context.Context, nodeID valueobjects.StorageNodeID, since time.Time) ([]*entities.File, error) {
	return c.underlying.FindModifiedSince(ctx, nodeID, since)
}

func (c *CachedFileRepository) Exists(ctx context.Context, nodeID valueobjects.StorageNodeID, path valueobjects.FilePath) (bool, error) {
	return c.underlying.Exists(ctx, nodeID, path)
}

func (c *CachedFileRepository) Create(ctx context.Context, file *entities.File) (int64, error) {
	return c.underlying.Create(ctx, file)
}

func (c *CachedFileRepository) GetByUserID(ctx context.Context, userID int64, limit, offset int) ([]entities.File, error) {
	return c.underlying.GetByUserID(ctx, userID, limit, offset)
}

func (c *CachedFileRepository) Update(ctx context.Context, file *entities.File) error {
	return c.underlying.Update(ctx, file)
}

func (c *CachedFileRepository) List(ctx context.Context, filter ports.FileFilter) ([]entities.File, error) {
	return c.underlying.List(ctx, filter)
}
