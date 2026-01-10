package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "syncvault/internal/grpc/proto/common"
	storagev1 "syncvault/internal/grpc/proto/storage"
	"syncvault/internal/storage"
)

// StorageService микросервис для хранения данных
type StorageService struct {
	storagev1.UnimplementedStorageServiceServer
	deviceManager *storage.DeviceManager
}

func NewStorageService() *StorageService {
	return &StorageService{
		deviceManager: storage.NewDeviceManager(nil), // Здесь будет конфиг
	}
}

func (s *StorageService) StoreFile(ctx context.Context, req *storagev1.StoreFileRequest) (*storagev1.StoreFileResponse, error) {
	log.Printf("Storing file: %s, size: %d bytes", req.FilePath, req.Size)

	// Имитация хранения файла
	fileID := fmt.Sprintf("storage_%d", time.Now().UnixNano())
	fileHash := "temp-hash" // В реальном приложении здесь был бы SHA256

	return &storagev1.StoreFileResponse{
		FileId:        fileID,
		FileHash:      fileHash,
		CreatedAt:     timestamppb.Now(),
		Size:          req.Size,
		StorageNodeId: req.StorageNodeId,
	}, nil
}

func (s *StorageService) GetFile(ctx context.Context, req *storagev1.GetFileRequest) (*storagev1.GetFileResponse, error) {
	log.Printf("Getting file: %s", req.FilePath)

	// Имитация получения файла
	metadata := &storagev1.FileMetadata{
		FileId:     "storage_1",
		FilePath:   req.FilePath,
		FileSize:   1024,
		MimeType:   "text/plain",
		CreatedAt:  timestamppb.Now(),
		ModifiedAt: timestamppb.Now(),
		SyncedAt:   timestamppb.Now(),
		Checksum:   "abc123",
		Status:     commonv1.FileStatus_FILE_STATUS_SYNCED,
		Version:    1,
		ChunkCount: 1,
	}

	return &storagev1.GetFileResponse{
		Metadata: metadata,
		Content:  []byte("This is file content"),
	}, nil
}

func (s *StorageService) DeleteFile(ctx context.Context, req *storagev1.DeleteFileRequest) (*emptypb.Empty, error) {
	log.Printf("Deleting file: %s", req.FilePath)

	return &emptypb.Empty{}, nil
}

func (s *StorageService) FileExists(ctx context.Context, req *storagev1.FileExistsRequest) (*storagev1.FileExistsResponse, error) {
	log.Printf("Checking if file exists: %s", req.FilePath)

	// Имитация проверки существования файла
	exists := true // В реальном приложении здесь была бы проверка в storage
	var metadata *storagev1.FileMetadata
	if exists {
		metadata = &storagev1.FileMetadata{
			FileId:     "storage_1",
			FilePath:   req.FilePath,
			FileSize:   1024,
			MimeType:   "text/plain",
			CreatedAt:  timestamppb.Now(),
			ModifiedAt: timestamppb.Now(),
			SyncedAt:   timestamppb.Now(),
			Checksum:   "abc123",
			Status:     commonv1.FileStatus_FILE_STATUS_SYNCED,
			Version:    1,
			ChunkCount: 1,
		}
	}

	return &storagev1.FileExistsResponse{
		Exists:   exists,
		Metadata: metadata,
	}, nil
}

func (s *StorageService) ListFiles(ctx context.Context, req *storagev1.ListFilesRequest) (*storagev1.ListFilesResponse, error) {
	log.Printf("Listing files in path: %s", req.DirectoryPath)

	// Имитация списка файлов
	files := []*storagev1.FileMetadata{
		{
			FileId:     "storage_1",
			FilePath:   "/documents/file1.txt",
			FileSize:   1024,
			MimeType:   "text/plain",
			CreatedAt:  timestamppb.Now(),
			ModifiedAt: timestamppb.Now(),
			SyncedAt:   timestamppb.Now(),
			Checksum:   "abc123",
			Status:     commonv1.FileStatus_FILE_STATUS_SYNCED,
			Version:    1,
			ChunkCount: 1,
		},
		{
			FileId:     "storage_2",
			FilePath:   "/images/image1.jpg",
			FileSize:   2048,
			MimeType:   "image/jpeg",
			CreatedAt:  timestamppb.Now(),
			ModifiedAt: timestamppb.Now(),
			SyncedAt:   timestamppb.Now(),
			Checksum:   "def456",
			Status:     commonv1.FileStatus_FILE_STATUS_SYNCED,
			Version:    1,
			ChunkCount: 1,
		},
	}

	return &storagev1.ListFilesResponse{
		Files: files,
		Pagination: &commonv1.PaginationResponse{
			NextCursor: "",
			HasMore:    false,
			TotalCount: 2,
		},
	}, nil
}

func (s *StorageService) GetSpaceInfo(ctx context.Context, req *storagev1.GetSpaceInfoRequest) (*storagev1.GetSpaceInfoResponse, error) {
	log.Printf("Getting storage info for node: %s", req.StorageNodeId)

	// Имитация информации о хранилище
	return &storagev1.GetSpaceInfoResponse{
		StorageNodeId: req.StorageNodeId,
		SpaceInfo: &commonv1.SpaceInfo{
			TotalSpace:     10737418240, // 10GB
			UsedSpace:      1073741824,  // 1GB
			FreeSpace:      9663676416,  // 9GB
			AvailableSpace: 9663676416,
		},
		CalculatedAt: timestamppb.Now(),
	}, nil
}

func main() {
	log.Println("Starting Storage Service microservice...")

	// Создаем gRPC сервер
	grpcServer := grpc.NewServer()

	// Регистрируем сервисы
	storageService := NewStorageService()
	storagev1.RegisterStorageServiceServer(grpcServer, storageService)

	// Включаем reflection для разработки
	reflection.Register(grpcServer)

	// Настраиваем порт
	port := "50054"
	if envPort := os.Getenv("STORAGE_SERVICE_PORT"); envPort != "" {
		port = envPort
	}

	// Создаем listener
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", port, err)
	}

	log.Printf("Storage Service listening on port %s", port)

	// Запускаем сервер в горутине
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down Storage Service...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-ctx.Done():
		log.Println("Shutdown timeout, forcing stop...")
		grpcServer.Stop()
	case <-stopped:
		log.Println("Storage Service stopped gracefully")
	}
}

// Методы для работы с метаданными файлов

// SyncFileMetadata синхронизирует метаданные файла между устройствами
func (s *StorageService) SyncFileMetadata(ctx context.Context, req *storagev1.SyncFileMetadataRequest) (*storagev1.SyncFileMetadataResponse, error) {
	log.Printf("Syncing file metadata: %s from device %s to device %s",
		req.FilePath, req.SourceDeviceId, req.TargetDeviceId)

	// Получаем исходное устройство
	sourceDevice, err := s.deviceManager.GetDevice(req.SourceDeviceId)
	if err != nil {
		return nil, fmt.Errorf("source device not found: %w", err)
	}

	// Получаем целевое устройство
	targetDevice, err := s.deviceManager.GetDevice(req.TargetDeviceId)
	if err != nil {
		return nil, fmt.Errorf("target device not found: %w", err)
	}

	// Здесь должна быть логика синхронизации метаданных
	// Например, обновление версии файла, статуса синхронизации

	log.Printf("Metadata synced successfully for file: %s", req.FilePath)

	return &storagev1.SyncFileMetadataResponse{
		Success:  true,
		Message:  "File metadata synced successfully",
		FileId:   fmt.Sprintf("file_%d", time.Now().UnixNano()),
		Version:  1,
		SyncedAt: timestamppb.Now(),
	}, nil
}

// GetFileHistory получает историю изменений файла
func (s *StorageService) GetFileHistory(ctx context.Context, req *storagev1.GetFileHistoryRequest) (*storagev1.GetFileHistoryResponse, error) {
	log.Printf("Getting file history for: %s", req.FilePath)

	// Здесь должна быть логика получения истории из базы данных
	// Например, из PostgreSQL или MongoDB

	history := []*storagev1.FileVersion{
		{
			Version:    1,
			DeviceId:   "device_1",
			ModifiedAt: timestamppb.Now(),
			Size:       1024,
			Hash:       "hash_1",
		},
		{
			Version:    2,
			DeviceId:   "device_2",
			ModifiedAt: timestamppb.Now(),
			Size:       2048,
			Hash:       "hash_2",
		},
	}

	return &storagev1.GetFileHistoryResponse{
		History:       history,
		TotalVersions: int32(len(history)),
	}, nil
}

// ResolveConflict разрешает конфликт файлов
func (s *StorageService) ResolveConflict(ctx context.Context, req *storagev1.ResolveConflictRequest) (*storagev1.ResolveConflictResponse, error) {
	log.Printf("Resolving conflict for file: %s", req.FilePath)

	// Здесь должна быть логика разрешения конфликтов
	// Например, выбор версии пользователя, слияние изменений

	log.Printf("Conflict resolved for file: %s, selected version: %d", req.FilePath, req.SelectedVersion)

	return &storagev1.ResolveConflictResponse{
		Success:         true,
		Message:         "Conflict resolved successfully",
		ResolvedVersion: req.SelectedVersion,
		ResolvedAt:      timestamppb.Now(),
	}, nil
}
