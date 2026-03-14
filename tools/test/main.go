package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Simple test program to verify database integration
func main() {
	var (
		host     = flag.String("host", "localhost", "Database host")
		port     = flag.String("port", "5432", "Database port")
		user     = flag.String("user", "postgres", "Database user")
		password = flag.String("password", "postgres", "Database password")
		dbname   = flag.String("dbname", "syncvault", "Database name")
		testType = flag.String("test", "all", "Test type: all, basic, repositories, batch, query")
	)
	flag.Parse()

	ctx := context.Background()

	// Build connection string
	connString := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		*user, *password, *host, *port, *dbname,
	)

	log.Printf("Connecting to database: %s:%s/%s", *host, *port, *dbname)

	// Create connection pool
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		log.Fatalf("Failed to create connection pool: %v", err)
	}
	defer pool.Close()

	// Test basic connectivity
	log.Println("Testing basic connectivity...")
	if err := testBasicConnectivity(ctx, pool); err != nil {
		log.Fatalf("Basic connectivity test failed: %v", err)
	}
	log.Println("✓ Basic connectivity test passed")

	// Test schema
	log.Println("Testing database schema...")
	if err := testSchema(ctx, pool); err != nil {
		log.Fatalf("Schema test failed: %v", err)
	}
	log.Println("✓ Schema test passed")

	// Run specific tests based on test type
	switch *testType {
	case "basic":
		log.Println("Basic tests completed successfully")
	case "repositories":
		if err := testRepositoryOperations(ctx, pool); err != nil {
			log.Fatalf("Repository operations test failed: %v", err)
		}
		log.Println("✓ Repository operations test passed")
	case "batch":
		if err := testBatchOperations(ctx, pool); err != nil {
			log.Fatalf("Batch operations test failed: %v", err)
		}
		log.Println("✓ Batch operations test passed")
	case "query":
		if err := testQueryOperations(ctx, pool); err != nil {
			log.Fatalf("Query operations test failed: %v", err)
		}
		log.Println("✓ Query operations test passed")
	case "all":
		if err := testRepositoryOperations(ctx, pool); err != nil {
			log.Fatalf("Repository operations test failed: %v", err)
		}
		log.Println("✓ Repository operations test passed")

		if err := testBatchOperations(ctx, pool); err != nil {
			log.Fatalf("Batch operations test failed: %v", err)
		}
		log.Println("✓ Batch operations test passed")

		if err := testQueryOperations(ctx, pool); err != nil {
			log.Fatalf("Query operations test failed: %v", err)
		}
		log.Println("✓ Query operations test passed")
	default:
		log.Fatalf("Unknown test type: %s", *testType)
	}

	// Get final statistics
	if err := printDatabaseStats(ctx, pool); err != nil {
		log.Printf("Warning: Failed to get database stats: %v", err)
	}

	log.Println("All tests completed successfully!")
}

// testBasicConnectivity tests basic database connectivity
func testBasicConnectivity(ctx context.Context, pool *pgxpool.Pool) error {
	// Test ping
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	// Test basic query
	var result int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&result); err != nil {
		return fmt.Errorf("basic query failed: %w", err)
	}

	if result != 1 {
		return fmt.Errorf("unexpected query result: got %d, expected 1", result)
	}

	return nil
}

// testSchema tests database schema
func testSchema(ctx context.Context, pool *pgxpool.Pool) error {
	// Check required tables
	requiredTables := []string{
		"users", "sessions", "files", "file_versions",
		"storage_nodes", "sync_jobs", "sync_events",
	}

	for _, table := range requiredTables {
		query := `
			SELECT COUNT(*) 
			FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = $1
		`

		var count int
		if err := pool.QueryRow(ctx, query, table).Scan(&count); err != nil {
			return fmt.Errorf("failed to check table %s: %w", table, err)
		}

		if count == 0 {
			return fmt.Errorf("required table %s not found", table)
		}
	}

	// Check indexes
	requiredIndexes := []string{
		"files_user_id_idx", "files_file_path_idx", "files_checksum_sha256_idx",
		"file_versions_file_id_idx", "file_versions_checksum_sha256_idx",
		"sync_events_job_id_idx", "sync_events_created_at_idx",
	}

	for _, index := range requiredIndexes {
		query := `
			SELECT COUNT(*) 
			FROM pg_indexes 
			WHERE schemaname = 'public' 
			AND indexname = $1
		`

		var count int
		if err := pool.QueryRow(ctx, query, index).Scan(&count); err != nil {
			return fmt.Errorf("failed to check index %s: %w", index, err)
		}

		if count == 0 {
			log.Printf("Warning: index %s not found", index)
		}
	}

	return nil
}

// testRepositoryOperations tests repository operations
func testRepositoryOperations(ctx context.Context, pool *pgxpool.Pool) error {
	// Test file operations
	testUserID := int64(1)

	// Test count files
	var fileCount int64
	err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM files WHERE user_id = $1 AND is_deleted = false", testUserID).Scan(&fileCount)
	if err != nil {
		return fmt.Errorf("failed to count files: %w", err)
	}

	// Test list files with pagination
	rows, err := pool.Query(ctx, `
		SELECT id, file_name, file_size_bytes, created_at 
		FROM files 
		WHERE user_id = $1 AND is_deleted = false 
		ORDER BY updated_at DESC 
		LIMIT 10
	`, testUserID)
	if err != nil {
		return fmt.Errorf("failed to list files: %w", err)
	}
	defer rows.Close()

	var listedFiles int
	for rows.Next() {
		var id int64
		var fileName string
		var fileSize int64
		var createdAt time.Time

		if err := rows.Scan(&id, &fileName, &fileSize, &createdAt); err != nil {
			return fmt.Errorf("failed to scan file: %w", err)
		}
		listedFiles++
	}

	log.Printf("Repository test: %d total files, %d listed", fileCount, listedFiles)

	// Test file version operations
	var versionCount int64
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM file_versions").Scan(&versionCount)
	if err != nil {
		return fmt.Errorf("failed to count file versions: %w", err)
	}

	log.Printf("Repository test: %d total file versions", versionCount)

	return nil
}

// testBatchOperations tests batch operations
func testBatchOperations(ctx context.Context, pool *pgxpool.Pool) error {
	// Create a temporary table for testing
	_, err := pool.Exec(ctx, `
		CREATE TEMP TABLE test_batch_files (
			id BIGINT PRIMARY KEY,
			user_id BIGINT NOT NULL,
			file_path TEXT NOT NULL,
			file_name TEXT NOT NULL,
			file_size_bytes BIGINT NOT NULL,
			created_at TIMESTAMP NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create temp table: %w", err)
	}
	defer pool.Exec(ctx, "DROP TABLE IF EXISTS test_batch_files")

	// Test batch insert using COPY
	startTime := time.Now()

	_, err = pool.CopyFrom(ctx, []string{"test_batch_files"}, []string{
		"id", "user_id", "file_path", "file_name", "file_size_bytes", "created_at",
	}, pgx.CopyFromSlice(1000, func(i int) ([]interface{}, error) {
		return []interface{}{
			int64(i + 1),
			int64(1),
			fmt.Sprintf("/test/batch/file_%d.txt", i),
			fmt.Sprintf("file_%d.txt", i),
			int64(1024 * (i + 1)),
			time.Now(),
		}, nil
	}))

	if err != nil {
		return fmt.Errorf("failed to batch insert: %w", err)
	}

	duration := time.Since(startTime)

	// Verify batch insert
	var count int64
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM test_batch_files").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to count batch files: %w", err)
	}

	if count != 1000 {
		return fmt.Errorf("batch insert failed: expected 1000 rows, got %d", count)
	}

	log.Printf("Batch test: inserted %d rows in %v (%.0f rows/sec)",
		count, duration, float64(count)/duration.Seconds())

	return nil
}

// testQueryOperations tests query analysis
func testQueryOperations(ctx context.Context, pool *pgxpool.Pool) error {
	// Test EXPLAIN ANALYZE on a complex query
	query := `
		EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) 
		SELECT f.id, f.file_name, f.file_size_bytes, 
		       COUNT(fv.id) as version_count
		FROM files f
		LEFT JOIN file_versions fv ON f.id = fv.file_id
		WHERE f.user_id = $1 AND f.is_deleted = false
		GROUP BY f.id, f.file_name, f.file_size_bytes
		ORDER BY f.updated_at DESC
		LIMIT 10
	`

	rows, err := pool.Query(ctx, query, int64(1))
	if err != nil {
		return fmt.Errorf("failed to execute explain analyze: %w", err)
	}
	defer rows.Close()

	var plans []string
	for rows.Next() {
		var plan string
		if err := rows.Scan(&plan); err != nil {
			return fmt.Errorf("failed to scan plan: %w", err)
		}
		plans = append(plans, plan)
	}

	if len(plans) == 0 {
		return fmt.Errorf("no query plans returned")
	}

	log.Printf("Query test: generated %d execution plans", len(plans))

	// Test table statistics
	var tableSize string
	err = pool.QueryRow(ctx, `
		SELECT pg_size_pretty(pg_total_relation_size('files'))
	`).Scan(&tableSize)
	if err != nil {
		return fmt.Errorf("failed to get table size: %w", err)
	}

	log.Printf("Query test: files table size: %s", tableSize)

	return nil
}

// printDatabaseStats prints database statistics
func printDatabaseStats(ctx context.Context, pool *pgxpool.Pool) error {
	// Get database version
	var version string
	err := pool.QueryRow(ctx, "SELECT version()").Scan(&version)
	if err != nil {
		return fmt.Errorf("failed to get database version: %w", err)
	}

	// Get database size
	var size string
	err = pool.QueryRow(ctx, `
		SELECT pg_size_pretty(pg_database_size(current_database()))
	`).Scan(&size)
	if err != nil {
		return fmt.Errorf("failed to get database size: %w", err)
	}

	// Get connection pool stats
	stats := pool.Stat()

	fmt.Printf("\n=== Database Statistics ===\n")
	fmt.Printf("Version: %s\n", version)
	fmt.Printf("Size: %s\n", size)
	fmt.Printf("Total Connections: %d\n", stats.TotalConns())
	fmt.Printf("Idle Connections: %d\n", stats.IdleConns())
	fmt.Printf("Acquired Connections: %d\n", stats.AcquiredConns())
	fmt.Printf("Max Connections: %d\n", stats.MaxConns())
	fmt.Printf("========================\n")

	return nil
}
