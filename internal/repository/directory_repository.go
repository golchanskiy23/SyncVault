package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DirectoryNode struct {
	ID        int64     `json:"id" db:"id"`
	UserID    int64     `json:"user_id" db:"user_id"`
	ParentID  *int64    `json:"parent_id" db:"parent_id"`
	Name      string    `json:"name" db:"name"`
	Path      string    `json:"path" db:"path"`
	IsDir     bool      `json:"is_dir" db:"is_dir"`
	FileSize  int64     `json:"file_size" db:"file_size"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
	Level     int       `json:"level" db:"level"` // Tree level from CTE
}

type DirectoryRepository struct {
	pool *pgxpool.Pool
}

func NewDirectoryRepository(pool *pgxpool.Pool) *DirectoryRepository {
	return &DirectoryRepository{
		pool: pool,
	}
}

func (r *DirectoryRepository) GetDirectoryTree(ctx context.Context, userID int64) ([]*DirectoryNode, error) {
	query := `
		WITH RECURSIVE directory_tree AS (
			-- Base case: root directories for the user
			SELECT 
					f.id,
					f.user_id,
					f.parent_id,
					f.file_name as name,
					f.file_path as path,
					true as is_dir,
					f.file_size_bytes as file_size,
					f.created_at,
					f.updated_at,
					0 as level
			FROM files f
			WHERE f.user_id = $1 
			AND f.is_deleted = false 
			AND f.parent_id IS NULL
			
			UNION ALL
			
			-- Recursive case: child directories and files
			SELECT 
					f.id,
					f.user_id,
					f.parent_id,
					f.file_name as name,
					f.file_path as path,
					f.file_size_bytes as file_size,
					f.created_at,
					f.updated_at,
					dt.level + 1 as level
			FROM files f
			INNER JOIN directory_tree dt ON f.parent_id = dt.id
			WHERE f.user_id = $1 
			AND f.is_deleted = false
		)
		SELECT 
				id, user_id, parent_id, name, path, is_dir, 
				file_size, created_at, updated_at, level
		FROM directory_tree
		ORDER BY path, level, name
	`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query directory tree: %w", err)
	}
	defer rows.Close()

	var directories []*DirectoryNode
	for rows.Next() {
		dir := &DirectoryNode{}
		err := rows.Scan(
			&dir.ID,
			&dir.UserID,
			&dir.ParentID,
			&dir.Name,
			&dir.Path,
			&dir.IsDir,
			&dir.FileSize,
			&dir.CreatedAt,
			&dir.UpdatedAt,
			&dir.Level,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan directory node: %w", err)
		}
		directories = append(directories, dir)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating directory tree rows: %w", err)
	}

	return directories, nil
}

func (r *DirectoryRepository) GetDirectoryTreeWithPath(ctx context.Context, userID int64, rootPath string) ([]*DirectoryNode, error) {
	query := `
		WITH RECURSIVE directory_tree AS (
			-- Base case: find the root directory
			SELECT 
					f.id,
					f.user_id,
					f.parent_id,
					f.file_name as name,
					f.file_path as path,
					true as is_dir,
					f.file_size_bytes as file_size,
					f.created_at,
					f.updated_at,
					0 as level
			FROM files f
			WHERE f.user_id = $1 
			AND f.file_path = $2
			AND f.is_deleted = false
			
			UNION ALL
			
			-- Recursive case: find all children
			SELECT 
					f.id,
					f.user_id,
					f.parent_id,
					f.file_name as name,
					f.file_path as path,
					f.file_size_bytes as file_size,
					f.created_at,
					f.updated_at,
					dt.level + 1 as level
			FROM files f
			INNER JOIN directory_tree dt ON f.parent_id = dt.id
			WHERE f.user_id = $1 
			AND f.is_deleted = false
		)
		SELECT 
				id, user_id, parent_id, name, path, is_dir, 
				file_size, created_at, updated_at, level
		FROM directory_tree
		ORDER BY level, path, name
	`

	rows, err := r.pool.Query(ctx, query, userID, rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to query directory tree from path: %w", err)
	}
	defer rows.Close()

	var directories []*DirectoryNode
	for rows.Next() {
		dir := &DirectoryNode{}
		err := rows.Scan(
			&dir.ID,
			&dir.UserID,
			&dir.ParentID,
			&dir.Name,
			&dir.Path,
			&dir.IsDir,
			&dir.FileSize,
			&dir.CreatedAt,
			&dir.UpdatedAt,
			&dir.Level,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan directory node: %w", err)
		}
		directories = append(directories, dir)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating directory tree rows: %w", err)
	}

	return directories, nil
}

func (r *DirectoryRepository) GetSubdirectories(ctx context.Context, userID int64, parentPath string) ([]*DirectoryNode, error) {
	query := `
		SELECT 
				f.id,
				f.user_id,
				f.parent_id,
				f.file_name as name,
				f.file_path as path,
				true as is_dir,
				f.file_size_bytes as file_size,
				f.created_at,
				f.updated_at,
				1 as level
		FROM files f
		WHERE f.user_id = $1 
		AND f.parent_id = (
			SELECT id FROM files 
			WHERE user_id = $1 AND file_path = $2 AND is_deleted = false
		)
		AND f.is_deleted = false
		ORDER BY f.file_name
	`

	rows, err := r.pool.Query(ctx, query, userID, parentPath)
	if err != nil {
		return nil, fmt.Errorf("failed to query subdirectories: %w", err)
	}
	defer rows.Close()

	var directories []*DirectoryNode
	for rows.Next() {
		dir := &DirectoryNode{}
		err := rows.Scan(
			&dir.ID,
			&dir.UserID,
			&dir.ParentID,
			&dir.Name,
			&dir.Path,
			&dir.IsDir,
			&dir.FileSize,
			&dir.CreatedAt,
			&dir.UpdatedAt,
			&dir.Level,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan subdirectory: %w", err)
		}
		directories = append(directories, dir)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating subdirectory rows: %w", err)
	}

	return directories, nil
}

func (r *DirectoryRepository) GetDirectoryStats(ctx context.Context, userID int64) (*DirectoryStats, error) {
	query := `
		WITH RECURSIVE directory_tree AS (
			SELECT 
					f.id,
					f.user_id,
					f.parent_id,
					f.file_name,
					f.file_path,
					f.file_size_bytes,
					f.created_at,
					0 as level
			FROM files f
			WHERE f.user_id = $1 
			AND f.is_deleted = false 
			AND f.parent_id IS NULL
			
			UNION ALL
			
			SELECT 
					f.id,
					f.user_id,
					f.parent_id,
					f.file_name,
					f.file_path,
					f.file_size_bytes,
					f.created_at,
					dt.level + 1 as level
			FROM files f
			INNER JOIN directory_tree dt ON f.parent_id = dt.id
			WHERE f.user_id = $1 
			AND f.is_deleted = false
		)
		SELECT 
				COUNT(*) as total_items,
				COUNT(CASE WHEN f.file_size_bytes IS NOT NULL AND f.file_size_bytes > 0 THEN 1 END) as file_count,
				COUNT(CASE WHEN f.file_size_bytes IS NULL OR f.file_size_bytes = 0 THEN 1 END) as directory_count,
				COALESCE(SUM(f.file_size_bytes), 0) as total_size,
				MAX(f.file_size_bytes) as max_file_size,
				MIN(f.created_at) as oldest_created,
				MAX(f.created_at) as newest_created
		FROM directory_tree dt
		JOIN files f ON dt.id = f.id
	`

	stats := &DirectoryStats{}
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&stats.TotalItems,
		&stats.FileCount,
		&stats.DirectoryCount,
		&stats.TotalSize,
		&stats.MaxFileSize,
		&stats.OldestCreated,
		&stats.NewestCreated,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get directory stats: %w", err)
	}

	return stats, nil
}

type DirectoryStats struct {
	TotalItems     int64     `json:"total_items"`
	FileCount      int64     `json:"file_count"`
	DirectoryCount int64     `json:"directory_count"`
	TotalSize      int64     `json:"total_size"`
	MaxFileSize    int64     `json:"max_file_size"`
	OldestCreated  time.Time `json:"oldest_created"`
	NewestCreated  time.Time `json:"newest_created"`
}

func (r *DirectoryRepository) FindPath(ctx context.Context, userID int64, targetPath string) ([]*DirectoryNode, error) {
	query := `
		WITH RECURSIVE path_up AS (
			-- Base case: the target item
			SELECT 
					f.id,
					f.user_id,
					f.parent_id,
					f.file_name as name,
					f.file_path as path,
					true as is_dir,
					f.file_size_bytes as file_size,
					f.created_at,
					f.updated_at,
					0 as level
			FROM files f
			WHERE f.user_id = $1 
			AND f.file_path = $2
			AND f.is_deleted = false
			
			UNION ALL
			
			-- Recursive case: go up the tree
			SELECT 
					f.id,
					f.user_id,
					f.parent_id,
					f.file_name as name,
					f.file_path as path,
					true as is_dir,
					f.file_size_bytes as file_size,
					f.created_at,
					f.updated_at,
					pu.level - 1 as level
			FROM files f
			INNER JOIN path_up pu ON f.id = pu.parent_id
			WHERE f.user_id = $1 
			AND f.is_deleted = false
		)
		SELECT 
				id, user_id, parent_id, name, path, is_dir, 
				file_size, created_at, updated_at, level
		FROM path_up
		ORDER BY level DESC
	`

	rows, err := r.pool.Query(ctx, query, userID, targetPath)
	if err != nil {
		return nil, fmt.Errorf("failed to find path: %w", err)
	}
	defer rows.Close()

	var path []*DirectoryNode
	for rows.Next() {
		node := &DirectoryNode{}
		err := rows.Scan(
			&node.ID,
			&node.UserID,
			&node.ParentID,
			&node.Name,
			&node.Path,
			&node.IsDir,
			&node.FileSize,
			&node.CreatedAt,
			&node.UpdatedAt,
			&node.Level,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan path node: %w", err)
		}
		path = append(path, node)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating path rows: %w", err)
	}

	return path, nil
}

func (r *DirectoryRepository) SearchInTree(ctx context.Context, userID int64, searchTerm string) ([]*DirectoryNode, error) {
	query := `
		WITH RECURSIVE directory_tree AS (
			SELECT 
					f.id,
					f.user_id,
					f.parent_id,
					f.file_name as name,
					f.file_path as path,
					true as is_dir,
					f.file_size_bytes as file_size,
					f.created_at,
					f.updated_at,
					0 as level
			FROM files f
			WHERE f.user_id = $1 
			AND f.is_deleted = false 
			AND f.parent_id IS NULL
			
			UNION ALL
			
			SELECT 
					f.id,
					f.user_id,
					f.parent_id,
					f.file_name as name,
					f.file_path as path,
					true as is_dir,
					f.file_size_bytes as file_size,
					f.created_at,
					f.updated_at,
					dt.level + 1 as level
			FROM files f
			INNER JOIN directory_tree dt ON f.parent_id = dt.id
			WHERE f.user_id = $1 
			AND f.is_deleted = false
		)
		SELECT 
				id, user_id, parent_id, name, path, is_dir, 
				file_size, created_at, updated_at, level
		FROM directory_tree
		WHERE f.file_name ILIKE $2
		ORDER BY level, path, name
	`

	rows, err := r.pool.Query(ctx, query, userID, "%"+searchTerm+"%")
	if err != nil {
		return nil, fmt.Errorf("failed to search directory tree: %w", err)
	}
	defer rows.Close()

	var results []*DirectoryNode
	for rows.Next() {
		node := &DirectoryNode{}
		err := rows.Scan(
			&node.ID,
			&node.UserID,
			&node.ParentID,
			&node.Name,
			&node.Path,
			&node.IsDir,
			&node.FileSize,
			&node.CreatedAt,
			&node.UpdatedAt,
			&node.Level,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}
		results = append(results, node)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating search results: %w", err)
	}

	return results, nil
}
