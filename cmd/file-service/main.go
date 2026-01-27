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
)

// FileService микросервис для управления файлами
type FileService struct {
	storagev1.UnimplementedStorageServiceServer
}

func (s *FileService) StoreFile(ctx context.Context, req *storagev1.StoreFileRequest) (*storagev1.StoreFileResponse, error) {
	log.Printf("Storing file: %s, size: %d bytes", req.FilePath, req.Size)

	// Имитация хранения файла
	fileID := fmt.Sprintf("file_%d", time.Now().UnixNano())
	fileHash := "temp-hash" // В реальном приложении здесь был бы SHA256

	return &storagev1.StoreFileResponse{
		FileId:        fileID,
		FileHash:      fileHash,
		CreatedAt:     timestamppb.Now(),
		Size:          req.Size,
		StorageNodeId: req.StorageNodeId,
	}, nil
}

func (s *FileService) GetFile(ctx context.Context, req *storagev1.GetFileRequest) (*storagev1.GetFileResponse, error) {
	log.Printf("Getting file: %s", req.FilePath)

	// Имитация получения файла
	metadata := &storagev1.FileMetadata{
		FileId:     "file_1",
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
		Metadata:  metadata,
		Content:   []byte("This is file content"),
		TotalSize: 1024,
		HasMore:   false,
	}, nil
}

func (s *FileService) DeleteFile(ctx context.Context, req *storagev1.DeleteFileRequest) (*emptypb.Empty, error) {
	log.Printf("Deleting file: %s", req.FilePath)

	return &emptypb.Empty{}, nil
}

func (s *FileService) ListFiles(ctx context.Context, req *storagev1.ListFilesRequest) (*storagev1.ListFilesResponse, error) {
	log.Printf("Listing files in path: %s", req.DirectoryPath)

	// Имитация списка файлов
	files := []*storagev1.FileMetadata{
		{
			FileId:     "file_1",
			FilePath:   "/documents/document.pdf",
			FileSize:   1024000,
			MimeType:   "application/pdf",
			CreatedAt:  timestamppb.Now(),
			ModifiedAt: timestamppb.Now(),
			SyncedAt:   timestamppb.Now(),
			Checksum:   "abc123",
			Status:     commonv1.FileStatus_FILE_STATUS_SYNCED,
			Version:    1,
			ChunkCount: 10,
		},
		{
			FileId:     "file_2",
			FilePath:   "/images/image.jpg",
			FileSize:   512000,
			MimeType:   "image/jpeg",
			CreatedAt:  timestamppb.Now(),
			ModifiedAt: timestamppb.Now(),
			SyncedAt:   timestamppb.Now(),
			Checksum:   "def456",
			Status:     commonv1.FileStatus_FILE_STATUS_SYNCED,
			Version:    1,
			ChunkCount: 5,
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

func main() {
	log.Println("Starting File Service microservice...")

	// Создаем gRPC сервер
	grpcServer := grpc.NewServer()

	// Регистрируем сервисы
	fileService := &FileService{}
	storagev1.RegisterStorageServiceServer(grpcServer, fileService)

	// Включаем reflection для разработки
	reflection.Register(grpcServer)

	// Настраиваем порт
	port := "50052"
	if envPort := os.Getenv("FILE_SERVICE_PORT"); envPort != "" {
		port = envPort
	}

	// Создаем listener
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", port, err)
	}

	log.Printf("File Service listening on port %s", port)

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

	log.Println("Shutting down File Service...")

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
		log.Println("File Service stopped gracefully")
	}
}
