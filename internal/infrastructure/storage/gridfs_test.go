package storage_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"syncvault/internal/domain/valueobjects"
	"syncvault/internal/infrastructure/storage"
)

// TestGridFSAdapterWithMongoDB tests GridFS adapter with a real MongoDB instance
// This test requires MongoDB to be running locally on localhost:27017
func TestGridFSAdapterWithMongoDB(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Connect to MongoDB (assumes MongoDB is running locally)
	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Skipf("MongoDB not available locally: %v", err)
	}
	defer client.Disconnect(ctx)

	// Test database connection
	err = client.Ping(ctx, nil)
	require.NoError(t, err, "Failed to ping MongoDB")

	// Use a test database
	db := client.Database("syncvault_gridfs_test")
	log.Println("✓ Connected to MongoDB for integration test")

	// Clean up before test
	err = db.Drop(ctx)
	require.NoError(t, err, "Failed to clean up test database")

	// Create GridFS adapter
	gridfsAdapter := storage.NewGridFSAdapter(db, "test_files")

	// Test connection
	err = gridfsAdapter.Connect(ctx)
	require.NoError(t, err, "Failed to connect GridFS adapter")

	// Test 1: Store file
	t.Run("StoreFile", func(t *testing.T) {
		testContent := "Hello, GridFS! This is a test file for SyncVault."
		filePath := valueobjects.FilePathFromString("test/hello-gridfs.txt")

		err := gridfsAdapter.PutFile(ctx, filePath, strings.NewReader(testContent), int64(len(testContent)))
		require.NoError(t, err, "Failed to store test file")
		log.Printf("✓ Stored test file: %s", filePath.String())
	})

	// Test 2: Get file
	t.Run("GetFile", func(t *testing.T) {
		filePath := valueobjects.FilePathFromString("test/hello-gridfs.txt")

		reader, size, err := gridfsAdapter.GetFile(ctx, filePath)
		require.NoError(t, err, "Failed to retrieve test file")
		defer reader.Close()

		// Read the content
		content, err := io.ReadAll(reader)
		require.NoError(t, err, "Failed to read test file")

		expectedContent := "Hello, GridFS! This is a test file for SyncVault."
		assert.Equal(t, expectedContent, string(content), "Content mismatch")
		assert.Equal(t, int64(len(expectedContent)), size, "Size mismatch")

		log.Printf("✓ Retrieved file content: %s (%d bytes)", string(content), size)
	})

	// Test 3: Check file existence
	t.Run("FileExists", func(t *testing.T) {
		filePath := valueobjects.FilePathFromString("test/hello-gridfs.txt")

		exists, err := gridfsAdapter.FileExists(ctx, filePath)
		require.NoError(t, err, "Failed to check file existence")
		assert.True(t, exists, "File should exist")

		// Test non-existent file
		nonExistentPath := valueobjects.FilePathFromString("test/non-existent.txt")
		exists, err = gridfsAdapter.FileExists(ctx, nonExistentPath)
		require.NoError(t, err, "Failed to check non-existent file")
		assert.False(t, exists, "Non-existent file should not exist")

		log.Printf("✓ File existence check passed")
	})

	// Test 4: List files
	t.Run("ListFiles", func(t *testing.T) {
		// Create additional test files
		testFiles := []string{
			"test/file1.txt",
			"test/file2.txt",
			"test/subdir/file3.txt",
		}

		for _, filePath := range testFiles {
			path := valueobjects.FilePathFromString(filePath)
			content := "Test content for " + filePath
			err := gridfsAdapter.PutFile(ctx, path, strings.NewReader(content), int64(len(content)))
			require.NoError(t, err, "Failed to store test file %s", filePath)
		}

		// List files in test directory
		files, err := gridfsAdapter.ListDir(ctx, valueobjects.FilePathFromString("test"))
		require.NoError(t, err, "Failed to list files")
		assert.GreaterOrEqual(t, len(files), 3, "Expected at least 3 files")

		log.Printf("✓ Listed %d files from GridFS", len(files))
		for _, file := range files {
			log.Printf("  - %s (%d bytes)", file.Path.String(), file.Size)
		}
	})

	// Test 5: Get file info
	t.Run("GetFileInfo", func(t *testing.T) {
		filePath := valueobjects.FilePathFromString("test/hello-gridfs.txt")

		fileInfo, err := gridfsAdapter.GetFileInfo(ctx, filePath)
		require.NoError(t, err, "Failed to get file info")

		expectedSize := int64(len("Hello, GridFS! This is a test file for SyncVault."))
		assert.Equal(t, expectedSize, fileInfo.Size, "File size mismatch")
		assert.Equal(t, filePath.String(), fileInfo.Path.String(), "File path mismatch")

		log.Printf("✓ File info: %+v", fileInfo)
	})

	// Test 6: Get space info
	t.Run("GetSpaceInfo", func(t *testing.T) {
		spaceInfo, err := gridfsAdapter.GetSpaceInfo(ctx)
		require.NoError(t, err, "Failed to get space info")
		assert.Greater(t, spaceInfo.UsedSpace, int64(0), "Used space should be positive")

		log.Printf("✓ Space info: %+v", spaceInfo)
	})

	// Test 7: Create directory
	t.Run("CreateDir", func(t *testing.T) {
		dirPath := valueobjects.FilePathFromString("test/newdir")

		err := gridfsAdapter.CreateDir(ctx, dirPath)
		require.NoError(t, err, "Failed to create directory")

		log.Printf("✓ Created directory: %s", dirPath.String())
	})

	// Test 8: Delete file
	t.Run("DeleteFile", func(t *testing.T) {
		filePath := valueobjects.FilePathFromString("test/hello-gridfs.txt")

		// Verify file exists before deletion
		exists, err := gridfsAdapter.FileExists(ctx, filePath)
		require.NoError(t, err, "Failed to check file existence before deletion")
		require.True(t, exists, "File should exist before deletion")

		// Delete file
		err = gridfsAdapter.DeleteFile(ctx, filePath)
		require.NoError(t, err, "Failed to delete file")

		// Verify file doesn't exist after deletion
		exists, err = gridfsAdapter.FileExists(ctx, filePath)
		require.NoError(t, err, "Failed to check file existence after deletion")
		assert.False(t, exists, "File should not exist after deletion")

		log.Printf("✓ Deleted file: %s", filePath.String())
	})

	// Test 9: Disconnect
	t.Run("Disconnect", func(t *testing.T) {
		err := gridfsAdapter.Disconnect(ctx)
		require.NoError(t, err, "Failed to disconnect GridFS adapter")

		log.Printf("✓ GridFS adapter disconnected")
	})

	// Test 10: IsConnected
	t.Run("IsConnected", func(t *testing.T) {
		connected := gridfsAdapter.IsConnected(ctx)
		assert.True(t, connected, "GridFS adapter should be connected")

		log.Printf("✓ GridFS adapter connection status: %t", connected)
	})

	// Clean up after test
	err = db.Drop(ctx)
	require.NoError(t, err, "Failed to clean up test database")

	log.Println("🎉 All GridFS integration tests passed!")
}

// TestGridFSAdapterConcurrentIntegration tests concurrent operations
func TestGridFSAdapterConcurrentIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Connect to MongoDB
	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Skipf("MongoDB not available locally: %v", err)
	}
	defer client.Disconnect(ctx)

	err = client.Ping(ctx, nil)
	require.NoError(t, err, "Failed to ping MongoDB")

	db := client.Database("syncvault_gridfs_concurrent_test")
	defer db.Drop(ctx) // Clean up after test

	gridfsAdapter := storage.NewGridFSAdapter(db, "concurrent_test")

	// Test concurrent file operations
	t.Run("ConcurrentOperations", func(t *testing.T) {
		const numFiles = 10
		const numGoroutines = 5

		var wg sync.WaitGroup
		errors := make(chan error, numFiles*numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				for j := 0; j < numFiles; j++ {
					filePath := valueobjects.FilePathFromString(fmt.Sprintf("concurrent/g%d_f%d.txt", goroutineID, j))
					content := fmt.Sprintf("Content from goroutine %d, file %d", goroutineID, j)

					err := gridfsAdapter.PutFile(ctx, filePath, strings.NewReader(content), int64(len(content)))
					if err != nil {
						errors <- fmt.Errorf("goroutine %d, file %d: %v", goroutineID, j, err)
						continue
					}

					// Verify file exists
					exists, err := gridfsAdapter.FileExists(ctx, filePath)
					if err != nil || !exists {
						errors <- fmt.Errorf("goroutine %d, file %d: existence check failed", goroutineID, j)
						continue
					}
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		// Check for any errors
		for err := range errors {
			t.Errorf("Concurrent operation error: %v", err)
		}

		// Verify all files exist
		files, err := gridfsAdapter.ListDir(ctx, valueobjects.FilePathFromString("concurrent"))
		require.NoError(t, err, "Failed to list concurrent files")

		expectedFiles := numFiles * numGoroutines
		assert.Equal(t, expectedFiles, len(files), "Expected %d files, got %d", expectedFiles, len(files))

		log.Printf("✓ Concurrent operations test passed: %d files created", len(files))
	})
}

// TestGridFSAdapterMetadata tests metadata handling
func TestGridFSAdapterMetadata(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Connect to MongoDB
	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Skipf("MongoDB not available locally: %v", err)
	}
	defer client.Disconnect(ctx)

	err = client.Ping(ctx, nil)
	require.NoError(t, err, "Failed to ping MongoDB")

	db := client.Database("syncvault_gridfs_metadata_test")
	defer db.Drop(ctx) // Clean up after test

	gridfsAdapter := storage.NewGridFSAdapter(db, "metadata_test")

	t.Run("MetadataHandling", func(t *testing.T) {
		filePath := valueobjects.FilePathFromString("test/metadata.txt")
		content := "Test content with metadata"

		// Store file with metadata
		err := gridfsAdapter.PutFile(ctx, filePath, strings.NewReader(content), int64(len(content)))
		require.NoError(t, err, "Failed to store test file")

		// Retrieve file info
		fileInfo, err := gridfsAdapter.GetFileInfo(ctx, filePath)
		require.NoError(t, err, "Failed to get file info")

		// Check that metadata was stored correctly
		assert.Equal(t, int64(len(content)), fileInfo.Size, "File size should match")
		assert.Equal(t, filePath.String(), fileInfo.Path.String(), "File path should match")

		log.Printf("✓ Metadata handling test passed")
	})
}

// BenchmarkGridFSAdapter benchmarks GridFS operations
func BenchmarkGridFSAdapter(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	ctx := context.Background()

	// Connect to MongoDB
	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		b.Skipf("MongoDB not available locally: %v", err)
	}
	defer client.Disconnect(ctx)

	err = client.Ping(ctx, nil)
	require.NoError(b, err, "Failed to ping MongoDB")

	db := client.Database("syncvault_gridfs_benchmark_test")
	defer db.Drop(ctx) // Clean up after test

	gridfsAdapter := storage.NewGridFSAdapter(db, "benchmark_test")

	b.Run("PutFile", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			filePath := valueobjects.FilePathFromString(fmt.Sprintf("benchmark/file_%d.txt", i))
			content := fmt.Sprintf("Benchmark content %d", i)
			err := gridfsAdapter.PutFile(ctx, filePath, strings.NewReader(content), int64(len(content)))
			if err != nil {
				b.Fatalf("Failed to store file: %v", err)
			}
		}
	})

	b.Run("GetFile", func(b *testing.B) {
		// Pre-populate files
		for i := 0; i < b.N; i++ {
			filePath := valueobjects.FilePathFromString(fmt.Sprintf("benchmark/get_file_%d.txt", i))
			content := fmt.Sprintf("Benchmark content %d", i)
			err := gridfsAdapter.PutFile(ctx, filePath, strings.NewReader(content), int64(len(content)))
			if err != nil {
				b.Fatalf("Failed to pre-populate file: %v", err)
			}
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			filePath := valueobjects.FilePathFromString(fmt.Sprintf("benchmark/get_file_%d.txt", i))
			reader, _, err := gridfsAdapter.GetFile(ctx, filePath)
			if err != nil {
				b.Fatalf("Failed to retrieve file: %v", err)
			}
			reader.Close()
		}
	})

	b.Run("FileExists", func(b *testing.B) {
		// Pre-populate files
		for i := 0; i < b.N; i++ {
			filePath := valueobjects.FilePathFromString(fmt.Sprintf("benchmark/exists_file_%d.txt", i))
			content := fmt.Sprintf("Benchmark content %d", i)
			err := gridfsAdapter.PutFile(ctx, filePath, strings.NewReader(content), int64(len(content)))
			if err != nil {
				b.Fatalf("Failed to pre-populate file: %v", err)
			}
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			filePath := valueobjects.FilePathFromString(fmt.Sprintf("benchmark/exists_file_%d.txt", i))
			_, err := gridfsAdapter.FileExists(ctx, filePath)
			if err != nil {
				b.Fatalf("Failed to check file existence: %v", err)
			}
		}
	})
}
