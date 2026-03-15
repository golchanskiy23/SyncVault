// database/file_repository.go
package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"syncvault/internal/domain/entities"
	"syncvault/internal/domain/ports"
	"syncvault/internal/domain/valueobjects"
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

// Create saves a file and returns ID
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

// FindByID retrieves a file by ID (alias for GetByID)
func (r *FileRepository) FindByID(ctx context.Context, id valueobjects.FileID) (*entities.File, error) {
	// Convert valueobjects.FileID to int64 for database query
	idInt, err := id.Int64()
	if err != nil {
		return nil, fmt.Errorf("invalid FileID: %w", err)
	}
	return r.GetByID(ctx, idInt)
}

// Create saves a file and returns ID (alias for Create)
func (r *FileRepository) Save(ctx context.Context, file *entities.File) error {
	id, err := r.Create(ctx, file)
	if err != nil {
		return err
	}

	// Update the file's ID with the generated ID
	file.ID = id
	return nil
}

// FindByID retrieves a file by ID
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
func (r *FileRepository) Update_old(ctx context.Context, file *entities.File) error {
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
		file.FilePath.String(),
		file.FileSize,
		file.FileHash.String(),
		file.StorageNodeID.String(),
		file.FileStatus,
	)

	if err != nil {
		return fmt.Errorf("failed to update file: %w", err)
	}

	log.Printf("FileRepository: File %d updated successfully", file.ID)
	return nil
}

// Save updates a file in the database
func (r *FileRepository) Update(ctx context.Context, file *entities.File) error {
	log.Printf("FileRepository: Saving file %+v", file)

	query := `
		UPDATE files 
		SET file_path = $2, file_hash = $3, file_size = $4, 
		    file_status = $5, storage_node_id = $6, user_id = $7, 
		    updated_at = NOW()
		WHERE id = $1
	`

	_, err := r.db.Exec(ctx, query,
		file.ID,
		file.FilePath.String(),
		file.FileHash.String(),
		file.FileSize,
		file.FileStatus,
		file.StorageNodeID.String(),
		file.UserID,
	)
	if err != nil {
		return fmt.Errorf("failed to update file: %w", err)
	}

	log.Printf("FileRepository: File %d updated successfully", file.ID)
	return nil
}

// FindByPath retrieves a file by node ID and path
func (r *FileRepository) FindByPath(ctx context.Context, nodeID valueobjects.StorageNodeID, path valueobjects.FilePath) (*entities.File, error) {
	log.Printf("FileRepository: Getting file by node ID %s and path %s", nodeID.String(), path.String())

	query := `
		SELECT id, user_id, file_name, file_path, file_size_bytes, file_hash, 
		       node_id, status, created_at, updated_at, version
		FROM files 
		WHERE node_id = $1 AND file_path = $2 AND is_deleted = false
	`

	file := &entities.File{}
	err := r.db.QueryRow(ctx, query, nodeID, path).Scan(
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

	return file, nil
}

// FindByNode retrieves files by storage node ID
func (r *FileRepository) FindByNode(ctx context.Context, nodeID valueobjects.StorageNodeID) ([]*entities.File, error) {
	log.Printf("FileRepository: Getting files for node %s", nodeID.String())

	query := `
		SELECT id, user_id, file_name, file_path, file_size_bytes, file_hash, 
		       node_id, status, created_at, updated_at, version
		FROM files 
		WHERE node_id = $1 AND is_deleted = false
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query, nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get files: %w", err)
	}
	defer rows.Close()

	var files []*entities.File
	for rows.Next() {
		file := &entities.File{}
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

	return files, nil
}

// FindModifiedSince retrieves files modified since a specific time
func (r *FileRepository) FindModifiedSince(ctx context.Context, nodeID valueobjects.StorageNodeID, since time.Time) ([]*entities.File, error) {
	log.Printf("FileRepository: Getting files modified since %v for node %s", since, nodeID.String())

	query := `
		SELECT id, user_id, file_name, file_path, file_size_bytes, file_hash, 
		       node_id, status, created_at, updated_at, version
		FROM files 
		WHERE node_id = $1 AND updated_at > $2 AND is_deleted = false
		ORDER BY updated_at DESC
	`

	rows, err := r.db.Query(ctx, query, nodeID, since)
	if err != nil {
		return nil, fmt.Errorf("failed to get files: %w", err)
	}
	defer rows.Close()

	var files []*entities.File
	for rows.Next() {
		file := &entities.File{}
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

	return files, nil
}

// Exists checks if a file exists
func (r *FileRepository) Exists(ctx context.Context, nodeID valueobjects.StorageNodeID, path valueobjects.FilePath) (bool, error) {
	log.Printf("FileRepository: Checking if file exists at node %s, path %s", nodeID.String(), path.String())

	query := `
		SELECT COUNT(*) FROM files 
		WHERE node_id = $1 AND file_path = $2 AND is_deleted = false
	`

	var count int
	err := r.db.QueryRow(ctx, query, nodeID, path).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check file existence: %w", err)
	}

	return count > 0, nil
}

func (r *FileRepository) Delete(ctx context.Context, id valueobjects.FileID) error {
	log.Printf("FileRepository: Deleting file ID: %s", id.String())

	query := `UPDATE files SET is_deleted = true, updated_at = NOW() WHERE id = $1`

	_, err := r.db.Exec(ctx, query, id.String())
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	log.Printf("FileRepository: File %s deleted successfully", id.String())
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
