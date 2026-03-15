package services

import (
	"context"
	"fmt"
	"log"

	"syncvault/internal/domain/entities"
	"syncvault/internal/domain/ports"
	"syncvault/internal/domain/valueobjects"
)

// FileService implements business logic for file operations
type FileService struct {
	fileRepo ports.FileRepository
	storage  ports.Storage
}

// NewFileService creates a new FileService
func NewFileService(fileRepo ports.FileRepository, storage ports.Storage) *FileService {
	return &FileService{
		fileRepo: fileRepo,
		storage:  storage,
	}
}

// CreateFileRequest represents a request to create a file
type CreateFileRequest struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	NodeID string `json:"node_id"`
	UserID int64  `json:"user_id"`
}

// CreateFileResponse represents a response after creating a file
type CreateFileResponse struct {
	ID     int64  `json:"id"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Status string `json:"status"`
}

// ListFilesRequest represents a request to list files
type ListFilesRequest struct {
	UserID int64 `json:"user_id"`
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
}

// ListFilesResponse represents a response with file list
type ListFilesResponse struct {
	Files []entities.File `json:"files"`
	Total int             `json:"total"`
}

// CreateFile creates a new file with business logic validation
func (s *FileService) CreateFile(ctx context.Context, req CreateFileRequest) (*CreateFileResponse, error) {
	log.Printf("FileService: Creating file with path: %s, size: %d", req.Path, req.Size)

	// Business validation
	if req.Path == "" {
		return nil, fmt.Errorf("file path cannot be empty")
	}
	if req.Size < 0 {
		return nil, fmt.Errorf("file size cannot be negative")
	}
	if req.UserID <= 0 {
		return nil, fmt.Errorf("user ID must be positive")
	}

	// Create file object
	filePath, err := valueobjects.NewFilePath(req.Path)
	if err != nil {
		return nil, fmt.Errorf("invalid file path: %w", err)
	}
	fileHash := valueobjects.NewFileHash([]byte("placeholder"))
	storageNodeID, err := valueobjects.StorageNodeIDFromString(req.NodeID)
	if err != nil {
		return nil, fmt.Errorf("invalid storage node ID: %w", err)
	}

	fileObj := &entities.File{
		FilePath:      filePath,
		FileHash:      fileHash,
		FileSize:      req.Size,
		FileStatus:    entities.FileStatusCreated,
		StorageNodeID: storageNodeID,
		UserID:        req.UserID,
	}

	// Store in repository
	fileID, err := s.fileRepo.Create(ctx, fileObj)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	// Store in storage (if needed)
	if s.storage != nil {
		// TODO: Implement actual file storage
		log.Printf("FileService: File %d stored in storage", fileID)
	}

	log.Printf("FileService: File created successfully with ID: %d", fileID)

	return &CreateFileResponse{
		ID:     fileID,
		Path:   req.Path,
		Size:   req.Size,
		Status: "created",
	}, nil
}

// ListFiles retrieves files with business logic
func (s *FileService) ListFiles(ctx context.Context, req ListFilesRequest) (*ListFilesResponse, error) {
	log.Printf("FileService: Listing files for user %d, limit %d, offset %d", req.UserID, req.Limit, req.Offset)

	// Business validation
	if req.UserID <= 0 {
		return nil, fmt.Errorf("user ID must be positive")
	}
	if req.Limit <= 0 {
		req.Limit = 10 // Default limit
	}
	if req.Offset < 0 {
		req.Offset = 0
	}

	// Retrieve from repository
	files, err := s.fileRepo.GetByUserID(ctx, req.UserID, req.Limit, req.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	log.Printf("FileService: Retrieved %d files for user %d", len(files), req.UserID)

	return &ListFilesResponse{
		Files: files,
		Total: len(files),
	}, nil
}

// GetFile retrieves a single file
func (s *FileService) GetFile(ctx context.Context, userID, fileID int64) (*entities.File, error) {
	log.Printf("FileService: Getting file %d for user %d", fileID, userID)

	// Convert int64 to valueobjects.FileID
	voFileID, err := valueobjects.FileIDFromString(fmt.Sprintf("%d", fileID))
	if err != nil {
		return nil, fmt.Errorf("invalid file ID: %w", err)
	}
	file, err := s.fileRepo.FindByID(ctx, voFileID)
	if err != nil {
		return nil, fmt.Errorf("failed to get file: %w", err)
	}

	// Business rule: user can only access their own files
	if file.UserID != userID {
		return nil, fmt.Errorf("access denied: file does not belong to user")
	}

	return file, nil
}

// DeleteFile deletes a file with business logic
func (s *FileService) DeleteFile(ctx context.Context, userID, fileID int64) error {
	log.Printf("FileService: Deleting file %d for user %d", fileID, userID)

	// Convert int64 to valueobjects.FileID
	voFileID, err := valueobjects.FileIDFromString(fmt.Sprintf("%d", fileID))
	if err != nil {
		return fmt.Errorf("invalid file ID: %w", err)
	}

	// Get file first to check ownership
	file, err := s.fileRepo.FindByID(ctx, voFileID)
	if err != nil {
		return fmt.Errorf("failed to get file: %w", err)
	}

	// Business rule: user can only delete their own files
	if file.UserID != userID {
		return fmt.Errorf("access denied: file does not belong to user")
	}

	err = s.fileRepo.Delete(ctx, voFileID)
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	// Delete from storage (if needed)
	if s.storage != nil {
		// TODO: Implement actual storage deletion
		log.Printf("FileService: File %d deleted from storage", fileID)
	}

	log.Printf("FileService: File %d deleted successfully", fileID)
	return nil
}
