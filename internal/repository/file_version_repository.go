package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FileVersion struct {
	ID             int64     `json:"id" db:"id"`
	FileID         int64     `json:"file_id" db:"file_id"`
	VersionNumber  int       `json:"version_number" db:"version_number"`
	StorageNodeID  *int64    `json:"storage_node_id" db:"storage_node_id"`
	FileSizeBytes  int64     `json:"file_size_bytes" db:"file_size_bytes"`
	ChecksumSHA256 string    `json:"checksum_sha256" db:"checksum_sha256"`
	StoragePath    string    `json:"storage_path" db:"storage_path"`
	IsCurrent      bool      `json:"is_current" db:"is_current"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

type FileVersionRepository struct {
	pool *pgxpool.Pool
}

func NewFileVersionRepository(pool *pgxpool.Pool) *FileVersionRepository {
	return &FileVersionRepository{
		pool: pool,
	}
}

func (r *FileVersionRepository) Save(ctx context.Context, version *FileVersion) error {
	query := `
		INSERT INTO file_versions (
			file_id, version_number, storage_node_id, file_size_bytes, 
			checksum_sha256, storage_path, is_current, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		) RETURNING id, created_at
	`

	err := r.pool.QueryRow(ctx, query,
		version.FileID,
		version.VersionNumber,
		version.StorageNodeID,
		version.FileSizeBytes,
		version.ChecksumSHA256,
		version.StoragePath,
		version.IsCurrent,
		time.Now(),
	).Scan(
		&version.ID,
		&version.CreatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save file version: %w", err)
	}

	return nil
}

func (r *FileVersionRepository) FindByID(ctx context.Context, id int64) (*FileVersion, error) {
	query := `
		SELECT id, file_id, version_number, storage_node_id, file_size_bytes,
			   checksum_sha256, storage_path, is_current, created_at
		FROM file_versions 
		WHERE id = $1
	`

	version := &FileVersion{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&version.ID,
		&version.FileID,
		&version.VersionNumber,
		&version.StorageNodeID,
		&version.FileSizeBytes,
		&version.ChecksumSHA256,
		&version.StoragePath,
		&version.IsCurrent,
		&version.CreatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("file version with id %d not found", id)
		}
		return nil, fmt.Errorf("failed to find file version by id: %w", err)
	}

	return version, nil
}

func (r *FileVersionRepository) FindByFileID(ctx context.Context, fileID int64) ([]*FileVersion, error) {
	query := `
		SELECT id, file_id, version_number, storage_node_id, file_size_bytes,
			   checksum_sha256, storage_path, is_current, created_at
		FROM file_versions 
		WHERE file_id = $1
		ORDER BY version_number DESC
	`

	rows, err := r.pool.Query(ctx, query, fileID)
	if err != nil {
		return nil, fmt.Errorf("failed to query file versions: %w", err)
	}
	defer rows.Close()

	var versions []*FileVersion
	for rows.Next() {
		version := &FileVersion{}
		err := rows.Scan(
			&version.ID,
			&version.FileID,
			&version.VersionNumber,
			&version.StorageNodeID,
			&version.FileSizeBytes,
			&version.ChecksumSHA256,
			&version.StoragePath,
			&version.IsCurrent,
			&version.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file version row: %w", err)
		}
		versions = append(versions, version)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating file version rows: %w", err)
	}

	return versions, nil
}

func (r *FileVersionRepository) GetVersionHistoryWithWindow(ctx context.Context, fileID int64) ([]*FileVersionWithRowNumber, error) {
	query := `
		WITH versioned_files AS (
			SELECT 
				id, file_id, version_number, storage_node_id, file_size_bytes,
				checksum_sha256, storage_path, is_current, created_at,
				ROW_NUMBER() OVER (PARTITION BY file_id ORDER BY version_number DESC) as row_num,
				COUNT(*) OVER (PARTITION BY file_id) as total_versions
			FROM file_versions 
			WHERE file_id = $1
		)
		SELECT 
				id, file_id, version_number, storage_node_id, file_size_bytes,
				checksum_sha256, storage_path, is_current, created_at,
				row_num, total_versions
		FROM versioned_files
		ORDER BY version_number DESC
	`

	rows, err := r.pool.Query(ctx, query, fileID)
	if err != nil {
		return nil, fmt.Errorf("failed to query version history with window: %w", err)
	}
	defer rows.Close()

	var versions []*FileVersionWithRowNumber
	for rows.Next() {
		version := &FileVersionWithRowNumber{}
		err := rows.Scan(
			&version.ID,
			&version.FileID,
			&version.VersionNumber,
			&version.StorageNodeID,
			&version.FileSizeBytes,
			&version.ChecksumSHA256,
			&version.StoragePath,
			&version.IsCurrent,
			&version.CreatedAt,
			&version.RowNumber,
			&version.TotalVersions,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan version with row number: %w", err)
		}
		versions = append(versions, version)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating version history rows: %w", err)
	}

	return versions, nil
}

func (r *FileVersionRepository) FindCurrent(ctx context.Context, fileID int64) (*FileVersion, error) {
	query := `
		SELECT id, file_id, version_number, storage_node_id, file_size_bytes,
			   checksum_sha256, storage_path, is_current, created_at
		FROM file_versions 
		WHERE file_id = $1 AND is_current = true
	`

	version := &FileVersion{}
	err := r.pool.QueryRow(ctx, query, fileID).Scan(
		&version.ID,
		&version.FileID,
		&version.VersionNumber,
		&version.StorageNodeID,
		&version.FileSizeBytes,
		&version.ChecksumSHA256,
		&version.StoragePath,
		&version.IsCurrent,
		&version.CreatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("no current version found for file %d", fileID)
		}
		return nil, fmt.Errorf("failed to find current version: %w", err)
	}

	return version, nil
}

func (r *FileVersionRepository) SetAsCurrent(ctx context.Context, versionID int64) error {
	query := `
		UPDATE file_versions 
		SET is_current = true, updated_at = $1
		WHERE id = $2
	`

	_, err := r.pool.Exec(ctx, query, time.Now(), versionID)
	if err != nil {
		return fmt.Errorf("failed to set version as current: %w", err)
	}

	return nil
}

func (r *FileVersionRepository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM file_versions WHERE id = $1`

	_, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete file version: %w", err)
	}

	return nil
}

func (r *FileVersionRepository) FindByChecksum(ctx context.Context, checksum string) ([]*FileVersion, error) {
	query := `
		SELECT id, file_id, version_number, storage_node_id, file_size_bytes,
			   checksum_sha256, storage_path, is_current, created_at
		FROM file_versions 
		WHERE checksum_sha256 = $1
		ORDER BY created_at DESC
	`

	rows, err := r.pool.Query(ctx, query, checksum)
	if err != nil {
		return nil, fmt.Errorf("failed to query file versions by checksum: %w", err)
	}
	defer rows.Close()

	var versions []*FileVersion
	for rows.Next() {
		version := &FileVersion{}
		err := rows.Scan(
			&version.ID,
			&version.FileID,
			&version.VersionNumber,
			&version.StorageNodeID,
			&version.FileSizeBytes,
			&version.ChecksumSHA256,
			&version.StoragePath,
			&version.IsCurrent,
			&version.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file version row: %w", err)
		}
		versions = append(versions, version)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating file version rows: %w", err)
	}

	return versions, nil
}

type FileVersionWithRowNumber struct {
	FileVersion
	RowNumber     int `json:"row_number" db:"row_num"`
	TotalVersions int `json:"total_versions" db:"total_versions"`
}

func (r *FileVersionRepository) GetVersionCount(ctx context.Context, fileID int64) (int64, error) {
	query := `SELECT COUNT(*) FROM file_versions WHERE file_id = $1`

	var count int64
	err := r.pool.QueryRow(ctx, query, fileID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count file versions: %w", err)
	}

	return count, nil
}
