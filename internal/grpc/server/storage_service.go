package server

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"syncvault/internal/grpc/interceptors"
	commonv1 "syncvault/internal/grpc/proto/common"
	storagev1 "syncvault/internal/grpc/proto/storage"
)

// StorageService реализация gRPC StorageService
type StorageService struct {
	storagev1.UnimplementedStorageServiceServer
}

// NewStorageService создает новый StorageService
func NewStorageService() *StorageService {
	return &StorageService{}
}

// StoreFile хранит файл
func (s *StorageService) StoreFile(ctx context.Context, req *storagev1.StoreFileRequest) (*storagev1.StoreFileResponse, error) {
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	fmt.Printf("StoreFile called by user %s, node %s\n", claims.UserID, claims.NodeID)

	if req.FilePath == "" {
		return nil, status.Error(codes.InvalidArgument, "file path is required")
	}

	return &storagev1.StoreFileResponse{
		FileId:        "temp-file-id-" + time.Now().Format("20060102150405"),
		FileHash:      "temp-hash",
		CreatedAt:     timestamppb.Now(),
		Size:          req.Size,
		StorageNodeId: req.StorageNodeId,
	}, nil
}

// GetFile получает файл
func (s *StorageService) GetFile(ctx context.Context, req *storagev1.GetFileRequest) (*storagev1.GetFileResponse, error) {
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	if err := interceptors.RequireUserID(ctx, claims.UserID); err != nil {
		return nil, err
	}

	fmt.Printf("GetFile called by user %s for file %s\n", claims.UserID, req.FilePath)

	if req.FilePath == "" {
		return nil, status.Error(codes.InvalidArgument, "file path is required")
	}

	return &storagev1.GetFileResponse{
		Metadata: &storagev1.FileMetadata{
			FileId:     "temp-file-id",
			FilePath:   req.FilePath,
			FileSize:   1024,
			CreatedAt:  timestamppb.Now(),
			ModifiedAt: timestamppb.Now(),
		},
		Content:   []byte("temp file content"),
		TotalSize: 1024,
		HasMore:   false,
	}, nil
}

// DeleteFile удаляет файл
func (s *StorageService) DeleteFile(ctx context.Context, req *storagev1.DeleteFileRequest) (*emptypb.Empty, error) {
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	if err := interceptors.RequireUserID(ctx, claims.UserID); err != nil {
		return nil, err
	}

	fmt.Printf("DeleteFile called by user %s for file %s\n", claims.UserID, req.FilePath)

	if req.FilePath == "" {
		return nil, status.Error(codes.InvalidArgument, "file path is required")
	}

	return &emptypb.Empty{}, nil
}

// FileExists проверяет существование файла
func (s *StorageService) FileExists(ctx context.Context, req *storagev1.FileExistsRequest) (*storagev1.FileExistsResponse, error) {
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	fmt.Printf("FileExists called by user %s for file %s\n", claims.UserID, req.FilePath)

	if req.FilePath == "" {
		return nil, status.Error(codes.InvalidArgument, "file path is required")
	}

	return &storagev1.FileExistsResponse{
		Exists: true,
		Metadata: &storagev1.FileMetadata{
			FileId:     "temp-file-id",
			FilePath:   req.FilePath,
			FileSize:   1024,
			CreatedAt:  timestamppb.Now(),
			ModifiedAt: timestamppb.Now(),
		},
	}, nil
}

// CreateDirectory создает директорию
func (s *StorageService) CreateDirectory(ctx context.Context, req *storagev1.CreateDirectoryRequest) (*emptypb.Empty, error) {
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	if err := interceptors.RequireUserID(ctx, claims.UserID); err != nil {
		return nil, err
	}

	fmt.Printf("CreateDirectory called by user %s for path %s\n", claims.UserID, req.DirectoryPath)

	if req.DirectoryPath == "" {
		return nil, status.Error(codes.InvalidArgument, "directory path is required")
	}

	return &emptypb.Empty{}, nil
}

// ListFiles получает список файлов
func (s *StorageService) ListFiles(ctx context.Context, req *storagev1.ListFilesRequest) (*storagev1.ListFilesResponse, error) {
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	fmt.Printf("ListFiles called by user %s in directory %s\n", claims.UserID, req.DirectoryPath)

	return &storagev1.ListFilesResponse{
		Files: []*storagev1.FileMetadata{
			{
				FileId:     "file1",
				FilePath:   "/tmp/file1.txt",
				FileSize:   1024,
				CreatedAt:  timestamppb.Now(),
				ModifiedAt: timestamppb.Now(),
			},
			{
				FileId:     "file2",
				FilePath:   "/tmp/file2.txt",
				FileSize:   2048,
				CreatedAt:  timestamppb.Now(),
				ModifiedAt: timestamppb.Now(),
			},
		},
		// proto: PaginationResponse — next_cursor, has_more, total_count
		Pagination: &commonv1.PaginationResponse{
			NextCursor: "",
			HasMore:    false,
			TotalCount: 2,
		},
	}, nil
}

// GetFileInfo получает информацию о файле
func (s *StorageService) GetFileInfo(ctx context.Context, req *storagev1.GetFileInfoRequest) (*storagev1.GetFileInfoResponse, error) {
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	fmt.Printf("GetFileInfo called by user %s for file %s\n", claims.UserID, req.FilePath)

	if req.FilePath == "" {
		return nil, status.Error(codes.InvalidArgument, "file path is required")
	}

	return &storagev1.GetFileInfoResponse{
		Metadata: &storagev1.FileMetadata{
			FileId:     "temp-file-id",
			FilePath:   req.FilePath,
			FileSize:   1024,
			CreatedAt:  timestamppb.Now(),
			ModifiedAt: timestamppb.Now(),
		},
	}, nil
}

// GetSpaceInfo получает информацию о пространстве
func (s *StorageService) GetSpaceInfo(ctx context.Context, req *storagev1.GetSpaceInfoRequest) (*storagev1.GetSpaceInfoResponse, error) {
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	fmt.Printf("GetSpaceInfo called by user %s\n", claims.UserID)

	return &storagev1.GetSpaceInfoResponse{
		SpaceInfo: &commonv1.SpaceInfo{
			TotalSpace: 10 * 1024 * 1024 * 1024,
			UsedSpace:  1024 * 1024,
			FreeSpace:  9 * 1024 * 1024 * 1024,
		},
		StorageNodeId: req.StorageNodeId,
		CalculatedAt:  timestamppb.Now(),
	}, nil
}

// Connect подключается к хранилищу
func (s *StorageService) Connect(ctx context.Context, req *storagev1.ConnectRequest) (*storagev1.ConnectResponse, error) {
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	if err := interceptors.RequireNodeID(ctx, claims.NodeID); err != nil {
		return nil, err
	}

	fmt.Printf("Connect called by node %s to storage %s\n", claims.NodeID, req.StorageNodeId)

	return &storagev1.ConnectResponse{
		Connected:   true,
		SessionId:   "session-" + time.Now().Format("20060102150405"),
		ConnectedAt: timestamppb.Now(),
		NodeInfo: &storagev1.StorageNode{
			NodeId:  req.StorageNodeId,
			Name:    "Storage Node",
			Address: "localhost:50051",
			Port:    50051,
		},
	}, nil
}

// Disconnect отключается от хранилища
func (s *StorageService) Disconnect(ctx context.Context, req *storagev1.DisconnectRequest) (*storagev1.DisconnectResponse, error) {
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	if err := interceptors.RequireNodeID(ctx, claims.NodeID); err != nil {
		return nil, err
	}

	fmt.Printf("Disconnect called by node %s from session %s\n", claims.NodeID, req.SessionId)

	return &storagev1.DisconnectResponse{
		Disconnected:   true,
		DisconnectedAt: timestamppb.Now(),
	}, nil
}

// IsConnected проверяет статус подключения
func (s *StorageService) IsConnected(ctx context.Context, req *storagev1.IsConnectedRequest) (*storagev1.IsConnectedResponse, error) {
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	fmt.Printf("IsConnected called by node %s for storage %s\n", claims.NodeID, req.StorageNodeId)

	return &storagev1.IsConnectedResponse{
		Connected:    true,
		LastActivity: timestamppb.Now(),
		SessionId:    "session-" + time.Now().Format("20060102150405"),
	}, nil
}
