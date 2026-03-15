// file_repository.go
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type File struct {
	ID               int64     `json:"id" db:"id"`
	UserID           int64     `json:"user_id" db:"user_id"`
	FilePath         string    `json:"file_path" db:"file_path"`
	FileName         string    `json:"file_name" db:"file_name"`
	FileSizeBytes    int64     `json:"file_size_bytes" db:"file_size_bytes"`
	MimeType         *string   `json:"mime_type" db:"mime_type"`
	ChecksumMD5      *string   `json:"checksum_md5" db:"checksum_md5"`
	ChecksumSHA256   *string   `json:"checksum_sha256" db:"checksum_sha256"`
	IsDeleted        bool      `json:"is_deleted" db:"is_deleted"`
	CurrentVersionID *int64    `json:"current_version_id" db:"current_version_id"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

type FileRepository struct {
	pool *pgxpool.Pool
}

func NewFileRepository(pool *pgxpool.Pool) *FileRepository {
	return &FileRepository{
		pool: pool,
	}
}

func (r *FileRepository) Save(ctx context.Context, file *File) error {
	query := `
		INSERT INTO files (
			user_id, file_path, file_name, file_size_bytes, 
			mime_type, checksum_md5, checksum_sha256, 
			is_deleted, current_version_id, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		) RETURNING id, created_at, updated_at
	`

	err := r.pool.QueryRow(ctx, query,
		file.UserID,
		file.FilePath,
		file.FileName,
		file.FileSizeBytes,
		file.MimeType,
		file.ChecksumMD5,
		file.ChecksumSHA256,
		file.IsDeleted,
		file.CurrentVersionID,
		time.Now(),
		time.Now(),
	).Scan(
		&file.ID,
		&file.CreatedAt,
		&file.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save file: %w", err)
	}

	return nil
}

func (r *FileRepository) FindByID(ctx context.Context, id int64) (*File, error) {
	query := `
		SELECT id, user_id, file_path, file_name, file_size_bytes,
			   mime_type, checksum_md5, checksum_sha256, is_deleted,
			   current_version_id, created_at, updated_at
		FROM files 
		WHERE id = $1 AND is_deleted = false
	`

	file := &File{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&file.ID,
		&file.UserID,
		&file.FilePath,
		&file.FileName,
		&file.FileSizeBytes,
		&file.MimeType,
		&file.ChecksumMD5,
		&file.ChecksumSHA256,
		&file.IsDeleted,
		&file.CurrentVersionID,
		&file.CreatedAt,
		&file.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("file with id %d not found", id)
		}
		return nil, fmt.Errorf("failed to find file by id: %w", err)
	}

	return file, nil
}

func (r *FileRepository) FindByHash(ctx context.Context, hash string) ([]*File, error) {
	query := `
		SELECT id, user_id, file_path, file_name, file_size_bytes,
			   mime_type, checksum_md5, checksum_sha256, is_deleted,
			   current_version_id, created_at, updated_at
		FROM files 
		WHERE checksum_sha256 = $1 AND is_deleted = false
		ORDER BY created_at DESC
	`

	rows, err := r.pool.Query(ctx, query, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to query files by hash: %w", err)
	}
	defer rows.Close()

	var files []*File
	for rows.Next() {
		file := &File{}
		err := rows.Scan(
			&file.ID,
			&file.UserID,
			&file.FilePath,
			&file.FileName,
			&file.FileSizeBytes,
			&file.MimeType,
			&file.ChecksumMD5,
			&file.ChecksumSHA256,
			&file.IsDeleted,
			&file.CurrentVersionID,
			&file.CreatedAt,
			&file.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file row: %w", err)
		}
		files = append(files, file)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating file rows: %w", err)
	}

	return files, nil
}

func (r *FileRepository) ListWithPagination(ctx context.Context, userID int64, offset, limit int) ([]*File, error) {
	query := `
		SELECT id, user_id, file_path, file_name, file_size_bytes,
			   mime_type, checksum_md5, checksum_sha256, is_deleted,
			   current_version_id, created_at, updated_at
		FROM files 
		WHERE user_id = $1 AND is_deleted = false
		ORDER BY updated_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.pool.Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query files with pagination: %w", err)
	}
	defer rows.Close()

	var files []*File
	for rows.Next() {
		file := &File{}
		err := rows.Scan(
			&file.ID,
			&file.UserID,
			&file.FilePath,
			&file.FileName,
			&file.FileSizeBytes,
			&file.MimeType,
			&file.ChecksumMD5,
			&file.ChecksumSHA256,
			&file.IsDeleted,
			&file.CurrentVersionID,
			&file.CreatedAt,
			&file.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file row: %w", err)
		}
		files = append(files, file)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating file rows: %w", err)
	}

	return files, nil
}

func (r *FileRepository) ListWithPaginationAndFilter(ctx context.Context, userID int64, offset, limit int, filter *FileFilter) ([]*File, error) {
	query := `
		SELECT id, user_id, file_path, file_name, file_size_bytes,
			   mime_type, checksum_md5, checksum_sha256, is_deleted,
			   current_version_id, created_at, updated_at
		FROM files 
		WHERE user_id = $1 AND is_deleted = false
	`

	args := []interface{}{userID}
	argIndex := 2

	if filter != nil {
		if filter.FileName != "" {
			query += fmt.Sprintf(" AND file_name ILIKE $%d", argIndex)
			args = append(args, "%"+filter.FileName+"%")
			argIndex++
		}
		if filter.MimeType != "" {
			query += fmt.Sprintf(" AND mime_type = $%d", argIndex)
			args = append(args, filter.MimeType)
			argIndex++
		}
		if filter.MinSize != nil {
			query += fmt.Sprintf(" AND file_size_bytes >= $%d", argIndex)
			args = append(args, *filter.MinSize)
			argIndex++
		}
		if filter.MaxSize != nil {
			query += fmt.Sprintf(" AND file_size_bytes <= $%d", argIndex)
			args = append(args, *filter.MaxSize)
			argIndex++
		}
		if filter.CreatedAfter != nil {
			query += fmt.Sprintf(" AND created_at >= $%d", argIndex)
			args = append(args, *filter.CreatedAfter)
			argIndex++
		}
		if filter.CreatedBefore != nil {
			query += fmt.Sprintf(" AND created_at <= $%d", argIndex)
			args = append(args, *filter.CreatedBefore)
			argIndex++
		}
	}

	query += " ORDER BY updated_at DESC LIMIT $" + fmt.Sprintf("%d", argIndex) + " OFFSET $" + fmt.Sprintf("%d", argIndex+1)
	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query files with pagination and filter: %w", err)
	}
	defer rows.Close()

	var files []*File
	for rows.Next() {
		file := &File{}
		err := rows.Scan(
			&file.ID,
			&file.UserID,
			&file.FilePath,
			&file.FileName,
			&file.FileSizeBytes,
			&file.MimeType,
			&file.ChecksumMD5,
			&file.ChecksumSHA256,
			&file.IsDeleted,
			&file.CurrentVersionID,
			&file.CreatedAt,
			&file.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file row: %w", err)
		}
		files = append(files, file)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating file rows: %w", err)
	}

	return files, nil
}

func (r *FileRepository) Count(ctx context.Context, userID int64) (int64, error) {
	query := `SELECT COUNT(*) FROM files WHERE user_id = $1 AND is_deleted = false`

	var count int64
	err := r.pool.QueryRow(ctx, query, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count files: %w", err)
	}

	return count, nil
}

func (r *FileRepository) CountWithFilter(ctx context.Context, userID int64, filter *FileFilter) (int64, error) {
	query := `SELECT COUNT(*) FROM files WHERE user_id = $1 AND is_deleted = false`

	args := []interface{}{userID}
	argIndex := 2

	if filter != nil {
		if filter.FileName != "" {
			query += fmt.Sprintf(" AND file_name ILIKE $%d", argIndex)
			args = append(args, "%"+filter.FileName+"%")
			argIndex++
		}
		if filter.MimeType != "" {
			query += fmt.Sprintf(" AND mime_type = $%d", argIndex)
			args = append(args, filter.MimeType)
			argIndex++
		}
		if filter.MinSize != nil {
			query += fmt.Sprintf(" AND file_size_bytes >= $%d", argIndex)
			args = append(args, *filter.MinSize)
			argIndex++
		}
		if filter.MaxSize != nil {
			query += fmt.Sprintf(" AND file_size_bytes <= $%d", argIndex)
			args = append(args, *filter.MaxSize)
			argIndex++
		}
		if filter.CreatedAfter != nil {
			query += fmt.Sprintf(" AND created_at >= $%d", argIndex)
			args = append(args, *filter.CreatedAfter)
			argIndex++
		}
		if filter.CreatedBefore != nil {
			query += fmt.Sprintf(" AND created_at <= $%d", argIndex)
			args = append(args, *filter.CreatedBefore)
			argIndex++
		}
	}

	var count int64
	err := r.pool.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count files with filter: %w", err)
	}

	return count, nil
}

func (r *FileRepository) Update(ctx context.Context, file *File) error {
	query := `
		UPDATE files SET 
			file_path = $2, file_name = $3, file_size_bytes = $4,
			mime_type = $5, checksum_md5 = $6, checksum_sha256 = $7,
			is_deleted = $8, current_version_id = $9, updated_at = $10
		WHERE id = $1
	`

	_, err := r.pool.Exec(ctx, query,
		file.ID,
		file.FilePath,
		file.FileName,
		file.FileSizeBytes,
		file.MimeType,
		file.ChecksumMD5,
		file.ChecksumSHA256,
		file.IsDeleted,
		file.CurrentVersionID,
		time.Now(),
	)

	if err != nil {
		return fmt.Errorf("failed to update file: %w", err)
	}

	return nil
}

func (r *FileRepository) SoftDelete(ctx context.Context, id int64) error {
	query := `UPDATE files SET is_deleted = true, updated_at = $1 WHERE id = $2`

	_, err := r.pool.Exec(ctx, query, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to soft delete file: %w", err)
	}

	return nil
}

func (r *FileRepository) FindByPath(ctx context.Context, userID int64, filePath string) (*File, error) {
	query := `
		SELECT id, user_id, file_path, file_name, file_size_bytes,
			   mime_type, checksum_md5, checksum_sha256, is_deleted,
			   current_version_id, created_at, updated_at
		FROM files 
		WHERE user_id = $1 AND file_path = $2 AND is_deleted = false
	`

	file := &File{}
	err := r.pool.QueryRow(ctx, query, userID, filePath).Scan(
		&file.ID,
		&file.UserID,
		&file.FilePath,
		&file.FileName,
		&file.FileSizeBytes,
		&file.MimeType,
		&file.ChecksumMD5,
		&file.ChecksumSHA256,
		&file.IsDeleted,
		&file.CurrentVersionID,
		&file.CreatedAt,
		&file.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("file with path %s not found for user %d", filePath, userID)
		}
		return nil, fmt.Errorf("failed to find file by path: %w", err)
	}

	return file, nil
}

func (r *FileRepository) UpdateCurrentVersion(ctx context.Context, fileID, versionID int64) error {
	query := `UPDATE files SET current_version_id = $1, updated_at = $2 WHERE id = $3`

	_, err := r.pool.Exec(ctx, query, versionID, time.Now(), fileID)
	if err != nil {
		return fmt.Errorf("failed to update current version: %w", err)
	}

	return nil
}

func (r *FileRepository) GetStorageUsage(ctx context.Context, userID int64) (int64, error) {
	query := `SELECT COALESCE(SUM(file_size_bytes), 0) FROM files WHERE user_id = $1 AND is_deleted = false`

	var usage int64
	err := r.pool.QueryRow(ctx, query, userID).Scan(&usage)
	if err != nil {
		return 0, fmt.Errorf("failed to get storage usage: %w", err)
	}

	return usage, nil
}

type FileFilter struct {
	FileName      string     `json:"file_name"`
	MimeType      string     `json:"mime_type"`
	MinSize       *int64     `json:"min_size"`
	MaxSize       *int64     `json:"max_size"`
	CreatedAfter  *time.Time `json:"created_after"`
	CreatedBefore *time.Time `json:"created_before"`
}

func (r *FileRepository) FindDuplicates(ctx context.Context, userID int64) ([]*File, error) {
	query := `
		SELECT id, user_id, file_path, file_name, file_size_bytes,
			   mime_type, checksum_md5, checksum_sha256, is_deleted,
			   current_version_id, created_at, updated_at
		FROM files 
		WHERE user_id = $1 AND checksum_sha256 IN (
			SELECT checksum_sha256 
			FROM files 
			WHERE user_id = $1 AND is_deleted = false
			GROUP BY checksum_sha256 
			HAVING COUNT(*) > 1
		) AND is_deleted = false
		ORDER BY created_at DESC
	`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to find duplicate files: %w", err)
	}
	defer rows.Close()

	var files []*File
	for rows.Next() {
		file := &File{}
		err := rows.Scan(
			&file.ID,
			&file.UserID,
			&file.FilePath,
			&file.FileName,
			&file.FileSizeBytes,
			&file.MimeType,
			&file.ChecksumMD5,
			&file.ChecksumSHA256,
			&file.IsDeleted,
			&file.CurrentVersionID,
			&file.CreatedAt,
			&file.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan duplicate file row: %w", err)
		}
		files = append(files, file)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating duplicate file rows: %w", err)
	}

	return files, nil
}

func (r *FileRepository) Exists(ctx context.Context, userID int64, filePath string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM files WHERE user_id = $1 AND file_path = $2 AND is_deleted = false)`

	var exists bool
	err := r.pool.QueryRow(ctx, query, userID, filePath).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check file existence: %w", err)
	}

	return exists, nil
}

func (r *FileRepository) BatchSave(ctx context.Context, files []*File) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO files (
			user_id, file_path, file_name, file_size_bytes, 
			mime_type, checksum_md5, checksum_sha256, 
			is_deleted, current_version_id, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		) RETURNING id, created_at, updated_at
	`

	for _, file := range files {
		err := tx.QueryRow(ctx, query,
			file.UserID,
			file.FilePath,
			file.FileName,
			file.FileSizeBytes,
			file.MimeType,
			file.ChecksumMD5,
			file.ChecksumSHA256,
			file.IsDeleted,
			file.CurrentVersionID,
			time.Now(),
			time.Now(),
		).Scan(
			&file.ID,
			&file.CreatedAt,
			&file.UpdatedAt,
		)

		if err != nil {
			return fmt.Errorf("failed to save file in batch: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit batch save: %w", err)
	}

	return nil
}
