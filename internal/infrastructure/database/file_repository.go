package database

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"

	"syncvault/internal/domain/entities"
	"syncvault/internal/domain/ports"
)

// FileRepository implements ports.FileRepository using PostgreSQL
type FileRepository struct {
	db *pgxpool.Pool
}

// NewFileRepository creates a new FileRepository
func NewFileRepository(db *pgxpool.Pool) *FileRepository {
	return &FileRepository{
		db: db,
	}
}

// Create creates a new file in the database
func (r *FileRepository) Create(ctx context.Context, file *entities.File) (int64, error) {
	log.Printf("FileRepository: Creating file %+v", file)

	query := `
		INSERT INTO files (user_id, file_name, file_path, file_size_bytes, file_hash, node_id, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		RETURNING id
	`

	var fileID int64
	err := r.db.QueryRow(ctx, query,
		file.UserID,
		"new_file.txt", // Temporary file name
		file.FilePath,
		file.FileSize,
		file.FileHash,
		file.StorageNodeID,
		file.FileStatus,
	).Scan(&fileID)

	if err != nil {
		return 0, fmt.Errorf("failed to create file: %w", err)
	}

	log.Printf("FileRepository: File created with ID: %d", fileID)
	return fileID, nil
}

// GetByID retrieves a file by ID
func (r *FileRepository) GetByID(ctx context.Context, id int64) (*entities.File, error) {
	log.Printf("FileRepository: Getting file by ID: %d", id)

	query := `
		SELECT id, user_id, file_name, file_path, file_size_bytes, file_hash, 
		       node_id, status, created_at, updated_at, version
		FROM files 
		WHERE id = $1 AND is_deleted = false
	`

	file := &entities.File{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&file.ID,
		&file.UserID,
		&file.FilePath,
		&file.FileSize,
		&file.FileHash,
		&file.StorageNodeID,
		&file.FileStatus,
		&file.CreatedAt,
		&file.ModifiedAt,
		&file.Version,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get file: %w", err)
	}

	log.Printf("FileRepository: Retrieved file: %+v", file)
	return file, nil
}

// GetByUserID retrieves files by user ID with pagination
func (r *FileRepository) GetByUserID(ctx context.Context, userID int64, limit, offset int) ([]entities.File, error) {
	log.Printf("FileRepository: Getting files for user %d, limit %d, offset %d", userID, limit, offset)

	query := `
		SELECT id, user_id, file_name, file_path, file_size_bytes, file_hash, 
		       node_id, status, created_at, updated_at, version
		FROM files 
		WHERE user_id = $1 AND is_deleted = false
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query files: %w", err)
	}
	defer rows.Close()

	var files []entities.File
	for rows.Next() {
		file := entities.File{}
		err := rows.Scan(
			&file.ID,
			&file.UserID,
			&file.FilePath,
			&file.FileSize,
			&file.FileHash,
			&file.StorageNodeID,
			&file.FileStatus,
			&file.CreatedAt,
			&file.ModifiedAt,
			&file.Version,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file: %w", err)
		}
		files = append(files, file)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating files: %w", err)
	}

	log.Printf("FileRepository: Retrieved %d files for user %d", len(files), userID)
	return files, nil
}

// Update updates an existing file
func (r *FileRepository) Update(ctx context.Context, file *entities.File) error {
	log.Printf("FileRepository: Updating file %+v", file)

	query := `
		UPDATE files 
		SET file_name = $2, file_path = $3, file_size_bytes = $4, file_hash = $5,
		    node_id = $6, status = $7, updated_at = NOW()
		WHERE id = $1
	`

	_, err := r.db.Exec(ctx, query,
		file.ID,
		"updated_file.txt", // Temporary file name
		file.FilePath,
		file.FileSize,
		file.FileHash,
		file.StorageNodeID,
		file.FileStatus,
	)

	if err != nil {
		return fmt.Errorf("failed to update file: %w", err)
	}

	log.Printf("FileRepository: File %d updated successfully", file.ID)
	return nil
}

// Delete soft deletes a file by ID
func (r *FileRepository) Delete(ctx context.Context, id int64) error {
	log.Printf("FileRepository: Deleting file ID: %d", id)

	query := `UPDATE files SET is_deleted = true, updated_at = NOW() WHERE id = $1`

	_, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	log.Printf("FileRepository: File %d deleted successfully", id)
	return nil
}

// List retrieves files with filtering
func (r *FileRepository) List(ctx context.Context, filter ports.FileFilter) ([]entities.File, error) {
	log.Printf("FileRepository: Listing files with filter %+v", filter)

	query := `
		SELECT id, user_id, file_name, file_path, file_size_bytes, file_hash, 
		       node_id, status, created_at, updated_at, version
		FROM files 
		WHERE is_deleted = false
	`

	args := []interface{}{}
	argIndex := 1

	if filter.UserID != nil {
		query += fmt.Sprintf(" AND user_id = $%d", argIndex)
		args = append(args, *filter.UserID)
		argIndex++
	}

	if filter.Status != nil {
		query += fmt.Sprintf(" AND status = $%d", argIndex)
		args = append(args, *filter.Status)
		argIndex++
	}

	if filter.PathLike != nil {
		query += fmt.Sprintf(" AND file_path ILIKE $%d", argIndex)
		args = append(args, "%"+*filter.PathLike+"%")
		argIndex++
	}

	query += " ORDER BY created_at DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIndex)
		args = append(args, filter.Limit)
		argIndex++
	}

	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argIndex)
		args = append(args, filter.Offset)
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query files: %w", err)
	}
	defer rows.Close()

	var files []entities.File
	for rows.Next() {
		file := entities.File{}
		err := rows.Scan(
			&file.ID,
			&file.UserID,
			&file.FilePath,
			&file.FileSize,
			&file.FileHash,
			&file.StorageNodeID,
			&file.FileStatus,
			&file.CreatedAt,
			&file.ModifiedAt,
			&file.Version,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file: %w", err)
		}
		files = append(files, file)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating files: %w", err)
	}

	log.Printf("FileRepository: Retrieved %d files", len(files))
	return files, nil
}
