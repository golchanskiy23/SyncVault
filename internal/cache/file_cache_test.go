package cache

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"syncvault/internal/domain/entities"
	"syncvault/internal/domain/ports"
	"syncvault/internal/domain/valueobjects"
)

// MockFileRepository for testing
type MockFileRepository struct {
	mock.Mock
}

func (m *MockFileRepository) Create(ctx context.Context, file *entities.File) (int64, error) {
	args := m.Called(ctx, file)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockFileRepository) GetByUserID(ctx context.Context, userID int64, limit, offset int) ([]entities.File, error) {
	args := m.Called(ctx, userID, limit, offset)
	return args.Get(0).([]entities.File), args.Error(1)
}

func (m *MockFileRepository) Update(ctx context.Context, file *entities.File) error {
	args := m.Called(ctx, file)
	return args.Error(0)
}

func (m *MockFileRepository) List(ctx context.Context, filter ports.FileFilter) ([]entities.File, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]entities.File), args.Error(1)
}

func (m *MockFileRepository) Save(ctx context.Context, file *entities.File) error {
	args := m.Called(ctx, file)
	return args.Error(0)
}

func (m *MockFileRepository) FindByID(ctx context.Context, id valueobjects.FileID) (*entities.File, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(*entities.File), args.Error(1)
}

func (m *MockFileRepository) FindByPath(ctx context.Context, nodeID valueobjects.StorageNodeID, path valueobjects.FilePath) (*entities.File, error) {
	args := m.Called(ctx, nodeID, path)
	return args.Get(0).(*entities.File), args.Error(1)
}

func (m *MockFileRepository) FindByNode(ctx context.Context, nodeID valueobjects.StorageNodeID) ([]*entities.File, error) {
	args := m.Called(ctx, nodeID)
	return args.Get(0).([]*entities.File), args.Error(1)
}

func (m *MockFileRepository) FindModifiedSince(ctx context.Context, nodeID valueobjects.StorageNodeID, since time.Time) ([]*entities.File, error) {
	args := m.Called(ctx, nodeID, since)
	return args.Get(0).([]*entities.File), args.Error(1)
}

func (m *MockFileRepository) Delete(ctx context.Context, id valueobjects.FileID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockFileRepository) Exists(ctx context.Context, nodeID valueobjects.StorageNodeID, path valueobjects.FilePath) (bool, error) {
	args := m.Called(ctx, nodeID, path)
	return args.Bool(0), args.Error(1)
}

func setupTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	return mr, client
}

// createTestFile создаёт тестовый файл с автоматически сгенерированным FileID.
func createTestFile(id int64, path string) *entities.File {
	filePath, _ := valueobjects.NewFilePath(path)
	fileHash, _ := valueobjects.FileHashFromString("abc123abc123abc123abc123abc123abc123abc123abc123abc123abc123abc1")
	nodeID := valueobjects.NewStorageNodeID()

	return &entities.File{
		FileID:        valueobjects.NewFileID(), // генерируем UUID
		ID:            id,
		FilePath:      filePath,
		FileHash:      fileHash,
		FileSize:      1024,
		FileStatus:    entities.FileStatusCreated,
		StorageNodeID: nodeID,
		UserID:        1,
		CreatedAt:     time.Now(),
		ModifiedAt:    time.Now(),
		Version:       1,
	}
}

// createTestFileWithID создаёт тестовый файл с заданным FileID (UUID).
// FileID используется как ключ кэша, поэтому его нужно явно передавать
// в тестах, где FileID известен заранее (например, при проверке cache hit).
func createTestFileWithID(fileID valueobjects.FileID, id int64, path string) *entities.File {
	filePath, _ := valueobjects.NewFilePath(path)
	fileHash, _ := valueobjects.FileHashFromString("abc123abc123abc123abc123abc123abc123abc123abc123abc123abc123abc1")
	nodeID := valueobjects.NewStorageNodeID()

	return &entities.File{
		FileID:        fileID, // присваиваем переданный UUID
		ID:            id,
		FilePath:      filePath,
		FileHash:      fileHash,
		FileSize:      1024,
		FileStatus:    entities.FileStatusCreated,
		StorageNodeID: nodeID,
		UserID:        1,
		CreatedAt:     time.Now(),
		ModifiedAt:    time.Now(),
		Version:       1,
	}
}

func TestFileCache_Get(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()

	cache := NewFileCache(client, 5*time.Minute)
	ctx := context.Background()

	file := createTestFile(1, "/test/file.txt")
	fileID := file.FileID.String() // ключ кэша = FileID UUID

	// Test cache miss
	result, err := cache.Get(ctx, fileID)
	assert.NoError(t, err)
	assert.Nil(t, result)

	// Set file in cache
	err = cache.Set(ctx, file)
	assert.NoError(t, err)

	// Test cache hit
	result, err = cache.Get(ctx, fileID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, file.ID, result.ID)
	assert.Equal(t, file.FilePath, result.FilePath)
}

func TestFileCache_Set(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()

	cache := NewFileCache(client, 5*time.Minute)
	ctx := context.Background()

	file := createTestFile(1, "/test/file.txt")

	// Set file in cache
	err := cache.Set(ctx, file)
	assert.NoError(t, err)

	// Verify file is in Redis under FileID key
	redisKey := "file_metadata:" + file.FileID.String()
	data, err := client.Get(ctx, redisKey).Result()
	assert.NoError(t, err)

	var cachedFile entities.File
	err = json.Unmarshal([]byte(data), &cachedFile)
	assert.NoError(t, err)
	assert.Equal(t, file.ID, cachedFile.ID)
}

func TestFileCache_Delete(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()

	cache := NewFileCache(client, 5*time.Minute)
	ctx := context.Background()

	file := createTestFile(1, "/test/file.txt")
	fileID := file.FileID.String()

	// Set file in cache
	err := cache.Set(ctx, file)
	assert.NoError(t, err)

	// Delete from cache
	err = cache.Delete(ctx, fileID)
	assert.NoError(t, err)

	// Verify file is deleted
	result, err := cache.Get(ctx, fileID)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestFileCache_InvalidateByPattern(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()

	cache := NewFileCache(client, 5*time.Minute)
	ctx := context.Background()

	// Set multiple files
	file1 := createTestFile(1, "/test/file1.txt")
	file2 := createTestFile(2, "/test/file2.txt")
	file3 := createTestFile(3, "/other/file3.txt")

	err := cache.Set(ctx, file1)
	assert.NoError(t, err)
	err = cache.Set(ctx, file2)
	assert.NoError(t, err)
	err = cache.Set(ctx, file3)
	assert.NoError(t, err)

	// Invalidate по паттерну пути файла (filepath.Match)
	// "/test/*" совпадёт с /test/file1.txt и /test/file2.txt, но не с /other/file3.txt
	err = cache.InvalidateByPattern(ctx, "/test/*")
	assert.NoError(t, err)

	// file1 и file2 должны быть удалены
	result1, _ := cache.Get(ctx, file1.FileID.String())
	assert.Nil(t, result1)

	result2, _ := cache.Get(ctx, file2.FileID.String())
	assert.Nil(t, result2)

	// file3 должен остаться
	result3, _ := cache.Get(ctx, file3.FileID.String())
	assert.NotNil(t, result3)
}

func TestCachedFileRepository_FindByID_CacheHit(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()

	mockRepo := &MockFileRepository{}
	cache := NewFileCache(client, 5*time.Minute)
	cachedRepo := NewCachedFileRepository(mockRepo, cache)

	ctx := context.Background()
	fileID := valueobjects.NewFileID()
	file := createTestFileWithID(fileID, 1, "/test/file.txt")

	// Кладём файл в кэш под его FileID
	err := cache.Set(ctx, file)
	require.NoError(t, err)

	// FindByID должен вернуть данные из кэша, не обращаясь к репозиторию
	result, err := cachedRepo.FindByID(ctx, fileID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, file.ID, result.ID)

	// Убеждаемся, что к БД не обращались
	mockRepo.AssertNotCalled(t, "FindByID")
}

func TestCachedFileRepository_FindByID_CacheMiss(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()

	mockRepo := &MockFileRepository{}
	cache := NewFileCache(client, 5*time.Minute)
	cachedRepo := NewCachedFileRepository(mockRepo, cache)

	ctx := context.Background()
	fileID := valueobjects.NewFileID()
	file := createTestFileWithID(fileID, 1, "/test/file.txt")

	// Setup mock
	mockRepo.On("FindByID", ctx, fileID).Return(file, nil)

	// Cache miss — должен обратиться к БД
	result, err := cachedRepo.FindByID(ctx, fileID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, file.ID, result.ID)

	mockRepo.AssertExpectations(t)

	// После обращения к БД файл должен быть помещён в кэш
	cachedResult, err := cache.Get(ctx, fileID.String())
	assert.NoError(t, err)
	assert.NotNil(t, cachedResult)
	assert.Equal(t, file.ID, cachedResult.ID)
}

func TestCachedFileRepository_Save(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()

	mockRepo := &MockFileRepository{}
	cache := NewFileCache(client, 5*time.Minute)
	cachedRepo := NewCachedFileRepository(mockRepo, cache)

	ctx := context.Background()
	fileID := valueobjects.NewFileID()
	file := createTestFileWithID(fileID, 1, "/test/file.txt")

	mockRepo.On("Save", ctx, file).Return(nil)

	err := cachedRepo.Save(ctx, file)
	assert.NoError(t, err)

	mockRepo.AssertExpectations(t)

	// Файл должен быть в кэше
	cachedFile, err := cache.Get(ctx, fileID.String())
	assert.NoError(t, err)
	assert.NotNil(t, cachedFile)
	assert.Equal(t, file.ID, cachedFile.ID)
}

func TestCachedFileRepository_Delete(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()

	mockRepo := &MockFileRepository{}
	cache := NewFileCache(client, 5*time.Minute)
	cachedRepo := NewCachedFileRepository(mockRepo, cache)

	ctx := context.Background()
	fileID := valueobjects.NewFileID()
	file := createTestFileWithID(fileID, 1, "/test/file.txt")

	// Кладём в кэш
	err := cache.Set(ctx, file)
	require.NoError(t, err)

	mockRepo.On("Delete", ctx, fileID).Return(nil)

	err = cachedRepo.Delete(ctx, fileID)
	assert.NoError(t, err)

	mockRepo.AssertExpectations(t)

	// Файл должен быть удалён из кэша
	cachedFile, err := cache.Get(ctx, fileID.String())
	assert.NoError(t, err)
	assert.Nil(t, cachedFile)
}

func TestCachedFileRepository_Save_UnderlyingError(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()

	mockRepo := &MockFileRepository{}
	cache := NewFileCache(client, 5*time.Minute)
	cachedRepo := NewCachedFileRepository(mockRepo, cache)

	ctx := context.Background()
	fileID := valueobjects.NewFileID()
	file := createTestFileWithID(fileID, 1, "/test/file.txt")

	// БД возвращает ошибку
	mockRepo.On("Save", ctx, file).Return(assert.AnError)

	err := cachedRepo.Save(ctx, file)
	assert.Error(t, err)

	mockRepo.AssertExpectations(t)

	// При ошибке БД файл не должен попасть в кэш
	cachedFile, err := cache.Get(ctx, fileID.String())
	assert.NoError(t, err)
	assert.Nil(t, cachedFile)
}

func TestFileCache_TTLExpiration(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()

	cache := NewFileCache(client, 100*time.Millisecond)
	ctx := context.Background()

	fileID := valueobjects.NewFileID()
	file := createTestFileWithID(fileID, 1, "/test/file.txt")

	err := cache.Set(ctx, file)
	assert.NoError(t, err)

	// Файл должен быть в кэше
	result, err := cache.Get(ctx, fileID.String())
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Ждём истечения TTL
	mr.FastForward(150 * time.Millisecond)

	// Файл должен быть удалён по TTL
	result, err = cache.Get(ctx, fileID.String())
	assert.NoError(t, err)
	assert.Nil(t, result)
}
