package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	var (
		host     = flag.String("host", "localhost", "Database host")
		port     = flag.String("port", "5432", "Database port")
		user     = flag.String("user", "postgres", "Database user")
		password = flag.String("password", "postgres", "Database password")
		dbname   = flag.String("dbname", "syncvault", "Database name")
		users    = flag.Int("users", 5, "Number of users to create")
		files    = flag.Int("files", 20, "Number of files per user")
		versions = flag.Int("versions", 3, "Number of versions per file")
		clear    = flag.Bool("clear", false, "Clear existing data before seeding")
	)
	flag.Parse()

	ctx := context.Background()

	// Build connection string
	connString := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		*user, *password, *host, *port, *dbname,
	)

	// Create connection pool
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	log.Printf("Connected to database: %s:%s/%s", *host, *port, *dbname)

	// Clear existing data if requested
	if *clear {
		log.Println("Clearing existing data...")
		clearData(ctx, pool)
	}

	// Seed data
	log.Printf("Seeding data: %d users, %d files per user, %d versions per file", *users, *files, *versions)

	err = seedData(ctx, pool, *users, *files, *versions)
	if err != nil {
		log.Fatalf("Failed to seed data: %v", err)
	}

	log.Println("Data seeding completed successfully!")

	// Show summary
	showSummary(ctx, pool)
}

// clearData removes all existing data
func clearData(ctx context.Context, pool *pgxpool.Pool) {
	tables := []string{"sync_events", "file_versions", "files", "storage_nodes", "sync_jobs", "sessions", "users"}

	for _, table := range tables {
		_, err := pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s", table))
		if err != nil {
			log.Printf("Failed to clear table %s: %v", table, err)
		}
	}

	// Reset sequences
	sequences := []string{"users_id_seq", "files_id_seq", "file_versions_id_seq", "sync_events_id_seq"}
	for _, seq := range sequences {
		_, err := pool.Exec(ctx, fmt.Sprintf("ALTER SEQUENCE %s RESTART WITH 1", seq))
		if err != nil {
			log.Printf("Failed to reset sequence %s: %v", seq, err)
		}
	}
}

// seedData creates test data
func seedData(ctx context.Context, pool *pgxpool.Pool, numUsers, numFiles, numVersions int) error {
	// Create users
	userIDs := make([]int64, numUsers)
	for i := 0; i < numUsers; i++ {
		userID, err := createUser(ctx, pool, i+1)
		if err != nil {
			return fmt.Errorf("failed to create user %d: %w", i+1, err)
		}
		userIDs[i] = userID
		log.Printf("Created user %d with ID %d", i+1, userID)
	}

	// Create storage nodes
	storageNodeID, err := createStorageNode(ctx, pool)
	if err != nil {
		return fmt.Errorf("failed to create storage node: %w", err)
	}
	log.Printf("Created storage node with ID %d", storageNodeID)

	// Create files and versions for each user
	totalFiles := 0
	totalVersions := 0

	for userIDIdx, userID := range userIDs {
		for fileIdx := 0; fileIdx < numFiles; fileIdx++ {
			fileID, err := createFile(ctx, pool, userID, userIDIdx+1, fileIdx+1)
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
			totalFiles++

			// Create versions for this file
			for versionIdx := 0; versionIdx < numVersions; versionIdx++ {
				_, err := createFileVersion(ctx, pool, fileID, storageNodeID, versionIdx+1)
				if err != nil {
					return fmt.Errorf("failed to create file version: %w", err)
				}
				totalVersions++
			}
		}
	}

	// Create some sync events
	err = createSyncEvents(ctx, pool, totalFiles)
	if err != nil {
		return fmt.Errorf("failed to create sync events: %w", err)
	}

	log.Printf("Created %d files and %d versions for %d users", totalFiles, totalVersions, numUsers)
	return nil
}

// createUser creates a test user
func createUser(ctx context.Context, pool *pgxpool.Pool, userNum int) (int64, error) {
	email := fmt.Sprintf("user%d@test.com", userNum)
	username := fmt.Sprintf("testuser%d", userNum)

	var userID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO users (username, email, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		RETURNING id
	`, username, email, "hashed_password").Scan(&userID)

	return userID, err
}

// createStorageNode creates a test storage node
func createStorageNode(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var nodeID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO storage_nodes (node_name, node_type, endpoint_url, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		RETURNING id
	`, "test-storage", "local", "http://localhost:8080", true).Scan(&nodeID)

	return nodeID, err
}

// createFile creates a test file
func createFile(ctx context.Context, pool *pgxpool.Pool, userID int64, userNum, fileNum int) (int64, error) {
	fileName := fmt.Sprintf("test_file_%d_%d.txt", userNum, fileNum)
	filePath := fmt.Sprintf("/home/user%d/documents/%s", userNum, fileName)
	fileSize := rand.Int63n(10*1024*1024) + 1024 // 1KB to 10MB
	mimeType := "text/plain"
	checksumMD5 := fmt.Sprintf("md5_%d_%d", userNum, fileNum)
	checksumSHA256 := fmt.Sprintf("sha256_%d_%d", userNum, fileNum)

	var fileID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO files (user_id, file_path, file_name, file_size_bytes, mime_type, 
		                  checksum_md5, checksum_sha256, is_deleted, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		RETURNING id
	`, userID, filePath, fileName, fileSize, mimeType, checksumMD5, checksumSHA256, false).Scan(&fileID)

	return fileID, err
}

// createFileVersion creates a test file version
func createFileVersion(ctx context.Context, pool *pgxpool.Pool, fileID, storageNodeID int64, versionNum int) (int64, error) {
	fileSize := rand.Int63n(5*1024*1024) + 512 // 512B to 5MB
	checksumSHA256 := fmt.Sprintf("sha256_v%d_f%d", versionNum, fileID)
	storagePath := fmt.Sprintf("/storage/files/%d/versions/%d", fileID, versionNum)
	isCurrent := versionNum == 1 // First version is current

	var versionID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO file_versions (file_id, version_number, storage_node_id, file_size_bytes,
		                         checksum_sha256, storage_path, is_current, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		RETURNING id
	`, fileID, versionNum, storageNodeID, fileSize, checksumSHA256, storagePath, isCurrent).Scan(&versionID)

	return versionID, err
}

// createSyncEvents creates test sync events
func createSyncEvents(ctx context.Context, pool *pgxpool.Pool, numFiles int) error {
	// Create sync events for some files
	// Skip sync events and jobs for now - they require job_type field
	log.Println("Skipping sync events and jobs creation for now")

	return nil
}

// showSummary displays a summary of the seeded data
func showSummary(ctx context.Context, pool *pgxpool.Pool) {
	fmt.Println("\n=== DATA SUMMARY ===")

	// Count users
	var userCount int64
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&userCount)
	fmt.Printf("Users: %d\n", userCount)

	// Count files
	var fileCount int64
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM files WHERE is_deleted = false").Scan(&fileCount)
	fmt.Printf("Files: %d\n", fileCount)

	// Count file versions
	var versionCount int64
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM file_versions").Scan(&versionCount)
	fmt.Printf("File Versions: %d\n", versionCount)

	// Count storage nodes
	var nodeCount int64
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM storage_nodes").Scan(&nodeCount)
	fmt.Printf("Storage Nodes: %d\n", nodeCount)

	// Count sync events
	var eventCount int64
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM sync_events").Scan(&eventCount)
	fmt.Printf("Sync Events: %d\n", eventCount)

	// Show some sample data
	fmt.Println("\n=== SAMPLE DATA ===")

	// Show recent files
	rows, err := pool.Query(ctx, `
		SELECT id, user_id, file_name, file_size_bytes, created_at
		FROM files 
		WHERE is_deleted = false
		ORDER BY created_at DESC 
		LIMIT 5
	`)
	if err == nil {
		defer rows.Close()
		fmt.Println("\nRecent Files:")
		for rows.Next() {
			var id, userID int64
			var fileName string
			var fileSize int64
			var createdAt time.Time

			if err := rows.Scan(&id, &userID, &fileName, &fileSize, &createdAt); err == nil {
				fmt.Printf("  ID: %d, User: %d, File: %s, Size: %.2f MB, Created: %s\n",
					id, userID, fileName, float64(fileSize)/1024/1024,
					createdAt.Format("2006-01-02 15:04"))
			}
		}
	}

	// Show current versions
	rows, err = pool.Query(ctx, `
		SELECT fv.id, fv.file_id, fv.version_number, fv.file_size_bytes, f.file_name
		FROM file_versions fv
		JOIN files f ON fv.file_id = f.id
		WHERE fv.is_current = true
		ORDER BY fv.created_at DESC
		LIMIT 5
	`)
	if err == nil {
		defer rows.Close()
		fmt.Println("\nCurrent File Versions:")
		for rows.Next() {
			var id, fileID, versionNumber int64
			var fileSize int64
			var fileName string

			if err := rows.Scan(&id, &fileID, &versionNumber, &fileSize, &fileName); err == nil {
				fmt.Printf("  Version ID: %d, File ID: %d, Version: %d, File: %s, Size: %.2f MB\n",
					id, fileID, versionNumber, fileName, float64(fileSize)/1024/1024)
			}
		}
	}
}
