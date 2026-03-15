package db

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Simple database operations wrapper
type SimpleDB struct {
	Pool *pgxpool.Pool
}

// NewSimpleDB creates a simple database wrapper
func NewSimpleDB(pool *pgxpool.Pool) *SimpleDB {
	return &SimpleDB{
		Pool: pool,
	}
}

// HealthCheck checks database connection
func (db *SimpleDB) HealthCheck(ctx context.Context) error {
	var result int
	return db.Pool.QueryRow(ctx, "SELECT 1").Scan(&result)
}

// GetStats returns database statistics
func (db *SimpleDB) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})

	// Get user count
	var userCount int64
	db.Pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM users").Scan(&userCount)
	stats["users"] = userCount

	// Get file count
	var fileCount int64
	db.Pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM files WHERE is_deleted = false").Scan(&fileCount)
	stats["files"] = fileCount

	// Get version count
	var versionCount int64
	db.Pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM file_versions").Scan(&versionCount)
	stats["versions"] = versionCount

	// Get pool stats
	poolStats := db.Pool.Stat()
	stats["pool_connections"] = map[string]interface{}{
		"acquired": poolStats.AcquiredConns(),
		"total":    poolStats.TotalConns(),
	}

	return stats
}

// CreateUser creates a new user
func (db *SimpleDB) CreateUser(ctx context.Context, username, email string) (int64, error) {
	var userID int64
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO users (username, email, password_hash, storage_quota_bytes, used_storage_bytes, created_at, updated_at)
		VALUES ($1, $2, 'hashed_password', $3, $4, NOW(), NOW())
		RETURNING id
	`, username, email, int64(10)*1024*1024*1024, int64(0)).Scan(&userID)

	if err != nil {
		return 0, fmt.Errorf("failed to create user: %w", err)
	}

	log.Printf("Created user %s with ID %d", username, userID)
	return userID, nil
}

// CreateFile creates a new file
func (db *SimpleDB) CreateFile(ctx context.Context, userID int64, fileName, filePath string, fileSize int64) (int64, error) {
	var fileID int64
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO files (user_id, file_path, file_name, file_size_bytes, mime_type, 
		                 checksum_md5, checksum_sha256, is_deleted, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'application/octet-stream', 
		        'md5_hash', 'sha256_hash', false, NOW(), NOW())
		RETURNING id
	`, userID, filePath, fileName, fileSize).Scan(&fileID)

	if err != nil {
		return 0, fmt.Errorf("failed to create file: %w", err)
	}

	log.Printf("Created file %s with ID %d", fileName, fileID)
	return fileID, nil
}

// CreateFileVersion creates a new file version
func (db *SimpleDB) CreateFileVersion(ctx context.Context, fileID int64, versionNumber int, fileSize int64) (int64, error) {
	var versionID int64
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO file_versions (file_id, version_number, storage_node_id, file_size_bytes, 
		                          checksum_sha256, storage_path, is_current, created_at)
		VALUES ($1, $2, 1, $3, 'sha256_hash', '/storage/path', true, NOW())
		RETURNING id
	`, fileID, versionNumber, fileSize).Scan(&versionID)

	if err != nil {
		return 0, fmt.Errorf("failed to create file version: %w", err)
	}

	log.Printf("Created file version %d with ID %d", versionNumber, versionID)
	return versionID, nil
}

// ListFiles returns list of files with window function
func (db *SimpleDB) ListFiles(ctx context.Context, userID int64, limit, offset int) ([]map[string]interface{}, error) {
	rows, err := db.Pool.Query(ctx, `
		WITH file_list AS (
			SELECT 
				f.id,
				f.user_id,
				f.file_name,
				f.file_size_bytes,
				f.created_at,
				f.updated_at,
				ROW_NUMBER() OVER (PARTITION BY f.user_id ORDER BY f.updated_at DESC) as rn
			FROM files f
			WHERE f.user_id = $1 AND f.is_deleted = false
		)
		SELECT id, user_id, file_name, file_size_bytes, created_at, updated_at
		FROM file_list
		WHERE rn BETWEEN $2 + 1 AND $2 + $3
		ORDER BY updated_at DESC
	`, userID, offset, limit)

	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}
	defer rows.Close()

	var files []map[string]interface{}
	for rows.Next() {
		var id, userID int64
		var fileName string
		var fileSize int64
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &userID, &fileName, &fileSize, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan file: %w", err)
		}

		files = append(files, map[string]interface{}{
			"id":         id,
			"user_id":    userID,
			"file_name":  fileName,
			"file_size":  fileSize,
			"created_at": createdAt,
			"updated_at": updatedAt,
		})
	}

	return files, nil
}

// GetFileVersions returns file versions with window function
func (db *SimpleDB) GetFileVersions(ctx context.Context, fileID int64) ([]map[string]interface{}, error) {
	rows, err := db.Pool.Query(ctx, `
		WITH versioned_files AS (
			SELECT 
				fv.id,
				fv.file_id,
				fv.version_number,
				fv.file_size_bytes,
				fv.created_at,
				ROW_NUMBER() OVER (PARTITION BY fv.file_id ORDER BY fv.version_number DESC) as rn
			FROM file_versions fv
			WHERE fv.file_id = $1
		)
		SELECT id, file_id, version_number, file_size_bytes, created_at
		FROM versioned_files
		ORDER BY version_number DESC
	`, fileID)

	if err != nil {
		return nil, fmt.Errorf("failed to get file versions: %w", err)
	}
	defer rows.Close()

	var versions []map[string]interface{}
	for rows.Next() {
		var id, fileID int64
		var versionNumber int
		var fileSize int64
		var createdAt time.Time

		if err := rows.Scan(&id, &fileID, &versionNumber, &fileSize, &createdAt); err != nil {
			return nil, fmt.Errorf("failed to scan file version: %w", err)
		}

		versions = append(versions, map[string]interface{}{
			"id":             id,
			"file_id":        fileID,
			"version_number": versionNumber,
			"file_size":      fileSize,
			"created_at":     createdAt,
		})
	}

	return versions, nil
}

// ExecuteInTransaction executes a function within a database transaction
func (db *SimpleDB) ExecuteInTransaction(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback(ctx)
			panic(p) // re-throw panic after rollback
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("transaction failed: %v, rollback failed: %w", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
