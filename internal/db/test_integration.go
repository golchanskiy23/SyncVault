package db

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestDatabaseIntegration provides comprehensive testing for database integration
func TestDatabaseIntegration(t *testing.T) {
	ctx := context.Background()
	
	// Test database connection
	config := DefaultConfig()
	config.DBName = "syncvault_test" // Use test database
	
	db, err := NewDatabase(ctx, config)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	
	// Test health check
	if err := db.HealthCheck(ctx); err != nil {
		t.Fatalf("Health check failed: %v", err)
	}
	
	// Test schema validation
	if err := db.ValidateSchema(ctx); err != nil {
		t.Fatalf("Schema validation failed: %v", err)
	}
	
	// Test connection
	if err := db.TestConnection(ctx); err != nil {
		t.Fatalf("Connection test failed: %v", err)
	}
	
	// Get database info
	info, err := db.GetDatabaseInfo(ctx)
	if err != nil {
		t.Fatalf("Failed to get database info: %v", err)
	}
	
	if info.DatabaseVersion == "" {
		t.Error("Database version should not be empty")
	}
	
	t.Logf("Database integration test passed. Version: %s, Size: %s", 
		info.DatabaseVersion, info.DatabaseSize)
}

// TestRepositoryOperations tests all repository operations
func TestRepositoryOperations(t *testing.T) {
	ctx := context.Background()
	
	// Setup test database
	config := DefaultConfig()
	config.DBName = "syncvault_test"
	
	db, err := NewDatabase(ctx, config)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	
	// Test file repository operations
	testFileRepository(t, db.FileRepo, ctx)
	
	// Test file version repository operations
	testFileVersionRepository(t, db.FileVersionRepo, ctx)
	
	// Test directory repository operations
	testDirectoryRepository(t, db.DirectoryRepo, ctx)
	
	// Test batch repository operations
	testBatchRepository(t, db.BatchRepo, ctx)
	
	// Test query analyzer operations
	testQueryAnalyzer(t, db.QueryAnalyzer, ctx)
}

// testFileRepository tests file repository operations
func testFileRepository(t *testing.T, repo FileRepositoryInterface, ctx context.Context) {
	t.Helper()
	
	testUserID := int64(1)
	
	// Test count
	count, err := repo.Count(ctx, testUserID)
	if err != nil {
		t.Fatalf("Failed to count files: %v", err)
	}
	
	if count < 0 {
		t.Error("File count should not be negative")
	}
	
	// Test pagination
	files, err := repo.ListWithPagination(ctx, testUserID, 0, 10)
	if err != nil {
		t.Fatalf("Failed to list files: %v", err)
	}
	
	if len(files) > 10 {
		t.Error("Should not return more than 10 files")
	}
	
	t.Logf("File repository test passed. Total files: %d", count)
}

// testFileVersionRepository tests file version repository operations
func testFileVersionRepository(t *testing.T, repo FileVersionRepositoryInterface, ctx context.Context) {
	t.Helper()
	
	testFileID := int64(1)
	
	// Test version count
	count, err := repo.GetVersionCount(ctx, testFileID)
	if err != nil {
		t.Fatalf("Failed to get version count: %v", err)
	}
	
	if count < 0 {
		t.Error("Version count should not be negative")
	}
	
	// Test find by file ID
	versions, err := repo.FindByFileID(ctx, testFileID)
	if err != nil {
		t.Fatalf("Failed to find versions: %v", err)
	}
	
	if len(versions) != int(count) {
		t.Errorf("Version count mismatch: expected %d, got %d", count, len(versions))
	}
	
	t.Logf("File version repository test passed. Total versions: %d", count)
}

// testDirectoryRepository tests directory repository operations
func testDirectoryRepository(t *testing.T, repo DirectoryRepositoryInterface, ctx context.Context) {
	t.Helper()
	
	testUserID := int64(1)
	
	// Test directory tree
	tree, err := repo.GetDirectoryTree(ctx, testUserID)
	if err != nil {
		t.Fatalf("Failed to get directory tree: %v", err)
	}
	
	// Tree should not be nil
	if tree == nil {
		t.Error("Directory tree should not be nil")
	}
	
	// Test directory stats
	stats, err := repo.GetDirectoryStats(ctx, testUserID)
	if err != nil {
		t.Fatalf("Failed to get directory stats: %v", err)
	}
	
	if stats == nil {
		t.Error("Directory stats should not be nil")
	}
	
	t.Logf("Directory repository test passed. Tree nodes: %d, Stats: %+v", 
		len(tree), stats)
}

// testBatchRepository tests batch repository operations
func testBatchRepository(t *testing.T, repo BatchRepositoryInterface, ctx context.Context) {
	t.Helper()
	
	// Create test batch files
	files := []*BatchFile{
		{
			ID:            9999,
			UserID:        1,
			FilePath:      "/test/batch1.txt",
			FileName:      "batch1.txt",
			FileSizeBytes: 1024,
			MimeType:      "text/plain",
			ChecksumMD5:   "testmd5",
			ChecksumSHA256: "testsha256",
			IsDeleted:     false,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		},
		{
			ID:            10000,
			UserID:        1,
			FilePath:      "/test/batch2.txt",
			FileName:      "batch2.txt",
			FileSizeBytes: 2048,
			MimeType:      "text/plain",
			ChecksumMD5:   "testmd52",
			ChecksumSHA256: "testsha2562",
			IsDeleted:     false,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		},
	}
	
	// Test batch insert with stats
	stats, err := repo.BatchInsertWithStats(ctx, files)
	if err != nil {
		t.Fatalf("Failed to batch insert files: %v", err)
	}
	
	if stats == nil {
		t.Error("Batch stats should not be nil")
	}
	
	if stats.TotalRows != int64(len(files)) {
		t.Errorf("Expected %d total rows, got %d", len(files), stats.TotalRows)
	}
	
	t.Logf("Batch repository test passed. Stats: %+v", stats)
}

// testQueryAnalyzer tests query analyzer operations
func testQueryAnalyzer(t *testing.T, analyzer QueryAnalyzerInterface, ctx context.Context) {
	t.Helper()
	
	// Test table stats
	stats, err := analyzer.GetTableStats(ctx, "files")
	if err != nil {
		t.Fatalf("Failed to get table stats: %v", err)
	}
	
	if stats == nil {
		t.Error("Table stats should not be nil")
	}
	
	// Test analyze file list query
	plan, err := analyzer.AnalyzeFileListQuery(ctx, 1, 0, 10)
	if err != nil {
		t.Fatalf("Failed to analyze file list query: %v", err)
	}
	
	if plan == nil {
		t.Error("Query plan should not be nil")
	}
	
	// Check for sequential scans
	warnings := analyzer.CheckForSeqScans(plan)
	if len(warnings) > 0 {
		t.Logf("Query warnings: %v", warnings)
	}
	
	t.Logf("Query analyzer test passed. Plan cost: %.2f, Warnings: %d", 
		plan.TotalCost, len(warnings))
}

// BenchmarkDatabaseOperations benchmarks database operations
func BenchmarkDatabaseOperations(b *testing.B) {
	ctx := context.Background()
	config := DefaultConfig()
	config.DBName = "syncvault_test"
	
	db, err := NewDatabase(ctx, config)
	if err != nil {
		b.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	
	// Benchmark file operations
	b.Run("FileList", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := db.FileRepo.ListWithPagination(ctx, 1, 0, 10)
			if err != nil {
				b.Fatalf("Failed to list files: %v", err)
			}
		}
	})
	
	// Benchmark query analysis
	b.Run("QueryAnalysis", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := db.QueryAnalyzer.AnalyzeFileListQuery(ctx, 1, 0, 10)
			if err != nil {
				b.Fatalf("Failed to analyze query: %v", err)
			}
		}
	})
}

// IntegrationTestRunner provides a complete integration test suite
type IntegrationTestRunner struct {
	db *Database
}

// NewIntegrationTestRunner creates a new integration test runner
func NewIntegrationTestRunner(config *Config) (*IntegrationTestRunner, error) {
	if config == nil {
		config = DefaultConfig()
		config.DBName = "syncvault_test"
	}
	
	ctx := context.Background()
	db, err := NewDatabase(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create test database: %w", err)
	}
	
	return &IntegrationTestRunner{
		db: db,
	}, nil
}

// RunAllTests runs all integration tests
func (itr *IntegrationTestRunner) RunAllTests() error {
	ctx := context.Background()
	
	log.Println("Starting database integration tests...")
	
	// Test basic connectivity
	if err := itr.db.HealthCheck(ctx); err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	log.Println("✓ Health check passed")
	
	// Test schema validation
	if err := itr.db.ValidateSchema(ctx); err != nil {
		return fmt.Errorf("schema validation failed: %w", err)
	}
	log.Println("✓ Schema validation passed")
	
	// Test repository operations
	if err := testRepositoryOperations(nil, itr.db); err != nil {
		return fmt.Errorf("repository operations failed: %w", err)
	}
	log.Println("✓ Repository operations passed")
	
	// Test query analyzer
	if err := testQueryAnalyzer(nil, itr.db.QueryAnalyzer, ctx); err != nil {
		return fmt.Errorf("query analyzer test failed: %w", err)
	}
	log.Println("✓ Query analyzer test passed")
	
	// Get final database info
	info, err := itr.db.GetDatabaseInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get database info: %w", err)
	}
	
	log.Printf("✓ All integration tests passed!")
	log.Printf("Database: %s, Size: %s, Connections: %d/%d", 
		info.DatabaseVersion, info.DatabaseSize, 
		info.AcquiredConnections, info.TotalConnections)
	
	return nil
}

// Close closes the integration test runner
func (itr *IntegrationTestRunner) Close() {
	if itr.db != nil {
		itr.db.Close()
	}
}

// testRepositoryOperations is a helper function for testing repository operations
func testRepositoryOperations(t interface{}, db *Database) error {
	ctx := context.Background()
	
	// Test file repository
	_, err := db.FileRepo.Count(ctx, 1)
	if err != nil {
		return fmt.Errorf("file repository test failed: %w", err)
	}
	
	// Test file version repository
	_, err = db.FileVersionRepo.GetVersionCount(ctx, 1)
	if err != nil {
		return fmt.Errorf("file version repository test failed: %w", err)
	}
	
	// Test directory repository
	_, err = db.DirectoryRepo.GetDirectoryTree(ctx, 1)
	if err != nil {
		return fmt.Errorf("directory repository test failed: %w", err)
	}
	
	// Test batch repository
	testFiles := []*BatchFile{
		{
			ID:            99999,
			UserID:        1,
			FilePath:      "/test/integration.txt",
			FileName:      "integration.txt",
			FileSizeBytes: 512,
			MimeType:      "text/plain",
			ChecksumMD5:   "integrationmd5",
			ChecksumSHA256: "integrationsha256",
			IsDeleted:     false,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		},
	}
	
	_, err = db.BatchRepo.BatchInsertWithStats(ctx, testFiles)
	if err != nil {
		return fmt.Errorf("batch repository test failed: %w", err)
	}
	
	return nil
}
