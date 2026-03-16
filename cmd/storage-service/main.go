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
		StorageNodeId: "storage-node-1",
	}, nil
}

func (s *StorageService) GetFile(ctx context.Context, req *storagev1.GetFileRequest) (*storagev1.GetFileResponse, error) {
	log.Printf("Getting file: %s", req.FilePath)

	// Имитация получения файла
	fileContent := "This is a test file content"
	fileSize := int64(len(fileContent))

	return &storagev1.GetFileResponse{
		Metadata: &storagev1.FileMetadata{
			FileId:    "file_123",
			FilePath:  req.FilePath,
			FileSize:  fileSize,
			FileHash:  "test-hash",
			CreatedAt: timestamppb.Now(),
			MimeType:  "text/plain",
		},
		Content:   []byte(fileContent),
		TotalSize: fileSize,
		HasMore:   false,
	}, nil
}

func (s *StorageService) DeleteFile(ctx context.Context, req *storagev1.DeleteFileRequest) (*emptypb.Empty, error) {
	log.Printf("Deleting file: %s", req.FilePath)

	// Имитация удаления файла
	return &emptypb.Empty{}, nil
}

func (s *StorageService) ListFiles(ctx context.Context, req *storagev1.ListFilesRequest) (*storagev1.ListFilesResponse, error) {
	log.Printf("Listing files in directory: %s", req.DirectoryPath)

	// Имитация списка файлов
	files := []*storagev1.FileMetadata{
		{
			FileId:    "file_1",
			FilePath:  "/documents/report.pdf",
			FileSize:  1024000,
			FileHash:  "hash_1",
			CreatedAt: timestamppb.Now(),
			MimeType:  "application/pdf",
		},
		{
			FileId:    "file_2",
			FilePath:  "/documents/presentation.pptx",
			FileSize:  2048000,
			FileHash:  "hash_2",
			CreatedAt: timestamppb.Now(),
			MimeType:  "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		},
	}

	return &storagev1.ListFilesResponse{
		Files: files,
		Pagination: &commonv1.PaginationResponse{
			NextCursor: "",
			HasMore:    false,
		},
	}, nil
}

func (s *StorageService) GetSpaceInfo(ctx context.Context, req *storagev1.GetSpaceInfoRequest) (*storagev1.GetSpaceInfoResponse, error) {
	log.Printf("Getting space info for node: %s", req.StorageNodeId)

	// Имитация информации о пространстве
	spaceInfo := &commonv1.SpaceInfo{
		TotalSpace:     10 * 1024 * 1024 * 1024, // 10GB
		UsedSpace:      5 * 1024 * 1024 * 1024,  // 5GB
		AvailableSpace: 5 * 1024 * 1024 * 1024,  // 5GB
	}

	return &storagev1.GetSpaceInfoResponse{
		SpaceInfo: spaceInfo,
	}, nil
}

// Методы для работы с устройствами

// RegisterDevice регистрирует новое устройство через Storage Service
func (s *StorageService) RegisterDevice(ctx context.Context, userID, deviceName, deviceType string) error {
	log.Printf("Registering device %s for user %s via Storage Service", deviceName, userID)

	// Здесь должна быть логика регистрации устройства
	// Например, сохранение в базу данных метаданных

	log.Printf("Device %s registered successfully", deviceName)
	return nil
}

// GetUserDevices получает все устройства пользователя
func (s *StorageService) GetUserDevices(ctx context.Context, userID string) error {
	log.Printf("Getting devices for user %s via Storage Service", userID)

	// Здесь должна быть логика получения устройств
	// Например, запрос к базе данных

	log.Printf("Found devices for user %s", userID)
	return nil
}

// SyncDevice синхронизирует устройство
func (s *StorageService) SyncDevice(ctx context.Context, deviceID string) error {
	log.Printf("Syncing device %s via Storage Service", deviceID)

	// Здесь должна быть логика синхронизации
	// Например, отправка команды в Sync Service

	log.Printf("Device %s synced successfully", deviceID)
	return nil
}

// GetDeviceStatus получает статус устройства
func (s *StorageService) GetDeviceStatus(ctx context.Context, userID, deviceID string) error {
	log.Printf("Getting device status %s for user %s via Storage Service", deviceID, userID)

	// Здесь должна быть логика получения статуса
	// Например, запрос к Device Manager

	log.Printf("Device %s status: online", deviceID)
	return nil
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

	// Запускаем сервер
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", port, err)
	}

	log.Printf("Storage Service listening on port %s", port)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

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
