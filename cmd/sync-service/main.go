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
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "syncvault/internal/grpc/proto/common"
	storagev1 "syncvault/internal/grpc/proto/storage"
	syncv1 "syncvault/internal/grpc/proto/sync"
	"syncvault/internal/storage"
)

// SyncService микросервис для синхронизации файлов
type SyncService struct {
	syncv1.UnimplementedSyncServiceServer
	deviceManager *storage.DeviceManager
}

func NewSyncService() *SyncService {
	return &SyncService{
		deviceManager: storage.NewDeviceManager(nil), // Здесь будет конфиг
	}
}

func (s *SyncService) StartSync(stream syncv1.SyncService_StartSyncServer) error {
	log.Printf("Starting sync stream")

	// Получаем первый запрос для регистрации устройства
	req, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("failed to receive initial request: %w", err)
	}

	// Извлекаем информацию об устройстве из SyncStart
	if syncStart := req.GetStart(); syncStart != nil {
		log.Printf("Device %s starting sync", syncStart.NodeId)

		storagePath := "/tmp/syncvault"
		if p, ok := syncStart.Options["storage_path"]; ok && p != "" {
			storagePath = p
		}

		// Регистрируем устройство если нужно
		device, err := s.deviceManager.RegisterDevice(
			stream.Context(),
			syncStart.NodeId, // Используем NodeId как UserID для простоты
			syncStart.NodeId, // Имя устройства
			storagePath,      // Временный путь, в реальности будет из конфига
		)
		if err != nil {
			return fmt.Errorf("failed to register device: %w", err)
		}

		log.Printf("Device registered: %s at %s", device.DeviceID, storagePath)

		// Отправляем подтверждение
		response := &syncv1.SyncResponse{
			Response: &syncv1.SyncResponse_Status{
				Status: &syncv1.SyncStatus{
					NodeId:            syncStart.NodeId,
					SessionId:         device.DeviceID,
					PendingEvents:     0,
					ProcessingEvents:  0,
					CompletedEvents:   1,
					FailedEvents:      0,
					LastSync:          timestamppb.Now(),
					AvgProcessingTime: &durationpb.Duration{Seconds: 1},
					SuccessRate:       1.0,
					State:             syncv1.SyncState_SYNC_STATE_CONNECTING,
					StartedAt:         timestamppb.Now(),
					ElapsedTime:       &durationpb.Duration{Seconds: 0},
					BytesTransferred:  0,
					TransferRateBps:   0.0,
					ActiveConnections: 1,
				},
			},
		}

		if err := stream.Send(response); err != nil {
			return fmt.Errorf("failed to send response: %w", err)
		}

		// Основной цикл синхронизации
		for {
			req, err := stream.Recv()
			if err != nil {
				log.Printf("Stream ended: %v", err)
				break
			}

			// Обрабатываем разные типы запросов
			switch req := req.Request.(type) {
			case *syncv1.SyncRequest_Status:
				// Запрос статуса
				status := s.getDeviceStatus(device.DeviceID)
				response := &syncv1.SyncResponse{
					Response: &syncv1.SyncResponse_Status{
						Status: status,
					},
				}
				if err := stream.Send(response); err != nil {
					return fmt.Errorf("failed to send status: %w", err)
				}

			case *syncv1.SyncRequest_Ping:
				// Ping-pong для проверки соединения
				response := &syncv1.SyncResponse{
					Response: &syncv1.SyncResponse_Pong{
						Pong: &syncv1.SyncPong{
							NodeId:         syncStart.NodeId,
							Timestamp:      timestamppb.Now(),
							SequenceNumber: 1,
							SessionId:      device.DeviceID,
						},
					},
				}
				if err := stream.Send(response); err != nil {
					return fmt.Errorf("failed to send pong: %w", err)
				}

			case *syncv1.SyncRequest_Complete:
				// Завершение синхронизации
				log.Printf("Sync completed for device %s", device.DeviceID)
				return nil

			default:
				log.Printf("Unknown request type: %T", req)
			}
		}
	}

	return nil
}

func (s *SyncService) GetSyncStatus(ctx context.Context, req *syncv1.GetSyncStatusRequest) (*syncv1.GetSyncStatusResponse, error) {
	log.Printf("Getting sync status for session: %s", req.SessionId)

	// Получаем устройство по session ID
	device, err := s.deviceManager.GetDevice(req.SessionId)
	if err != nil {
		log.Printf("Device not found: %v", err)
		// Возвращаем статус по умолчанию если устройство не найдено
		defaultStatus := s.getDefaultSyncStatus(req.SessionId)
		return &syncv1.GetSyncStatusResponse{
			Statuses: []*syncv1.SyncStatus{defaultStatus},
			Summary: &syncv1.SyncSummary{
				TotalSessions:         1,
				ActiveSessions:        0,
				TotalEvents:           0,
				PendingEvents:         0,
				FailedEvents:          0,
				OverallSuccessRate:    0.0,
				LastActivity:          timestamppb.Now(),
				TotalBytesTransferred: 0,
			},
		}, nil
	}

	// Получаем реальные данные от устройства
	status := s.getDeviceStatus(device.DeviceID)

	return &syncv1.GetSyncStatusResponse{
		Statuses: []*syncv1.SyncStatus{status},
		Summary: &syncv1.SyncSummary{
			TotalSessions:         1,
			ActiveSessions:        1,
			TotalEvents:           status.CompletedEvents + status.FailedEvents,
			PendingEvents:         status.PendingEvents,
			FailedEvents:          status.FailedEvents,
			OverallSuccessRate:    status.SuccessRate,
			LastActivity:          status.LastSync,
			TotalBytesTransferred: status.BytesTransferred,
		},
	}, nil
}

func (s *SyncService) CancelSync(ctx context.Context, req *syncv1.CancelSyncRequest) (*emptypb.Empty, error) {
	log.Printf("Cancelling sync session: %s", req.SessionId)

	return &emptypb.Empty{}, nil
}

func (s *SyncService) ForceSync(ctx context.Context, req *syncv1.ForceSyncRequest) (*syncv1.ForceSyncResponse, error) {
	log.Printf("Force sync from node: %s to node: %s", req.SourceNodeId, req.TargetNodeId)

	sessionID := fmt.Sprintf("force_sync_%d", time.Now().UnixNano())

	return &syncv1.ForceSyncResponse{
		SessionId: sessionID,
		Status: &syncv1.SyncStatus{
			NodeId:            req.SourceNodeId,
			SessionId:         sessionID,
			PendingEvents:     2,
			ProcessingEvents:  0,
			CompletedEvents:   0,
			FailedEvents:      0,
			LastSync:          timestamppb.Now(),
			AvgProcessingTime: &durationpb.Duration{Seconds: 1},
			SuccessRate:       0.0,
			State:             syncv1.SyncState_SYNC_STATE_CONNECTING,
			StartedAt:         timestamppb.Now(),
			ElapsedTime:       &durationpb.Duration{Seconds: 0},
			BytesTransferred:  0,
			TransferRateBps:   0.0,
			ActiveConnections: 1,
		},
		StartedAt: timestamppb.Now(),
	}, nil
}

func (s *SyncService) GetSyncHistory(ctx context.Context, req *syncv1.GetSyncHistoryRequest) (*syncv1.GetSyncHistoryResponse, error) {
	log.Printf("Getting sync history for node: %s", req.NodeId)

	// Имитация истории синхронизаций
	history := []*syncv1.SyncHistoryEntry{
		{
			EventId:      "sync_1",
			Type:         commonv1.SyncEventType_SYNC_EVENT_TYPE_SYNC_COMPLETED,
			SourceNodeId: "node_1",
			TargetNodeId: "node_2",
			FileMetadata: &storagev1.FileMetadata{
				FileId:     "file_1",
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
			Timestamp:  timestamppb.Now(),
			Result:     syncv1.SyncResult_SYNC_RESULT_SUCCESS,
			RetryCount: 0,
		},
	}

	return &syncv1.GetSyncHistoryResponse{
		Entries: history,
		Pagination: &commonv1.PaginationResponse{
			NextCursor: "",
			HasMore:    false,
			TotalCount: 1,
		},
	}, nil
}

func (s *SyncService) GetSyncConflicts(ctx context.Context, req *syncv1.GetSyncConflictsRequest) (*syncv1.GetSyncConflictsResponse, error) {
	log.Printf("Getting sync conflicts for node: %s", req.NodeId)

	// Имитация конфликтов
	conflicts := []*syncv1.SyncConflict{
		{
			ConflictId:   "conflict_1",
			FileId:       "file_conflict_1",
			FilePath:     "/documents/conflict.txt",
			SourceNodeId: "node_1",
			TargetNodeId: "node_2",
			Type:         syncv1.ConflictType_CONFLICT_TYPE_VERSION,
			Status:       syncv1.ConflictStatus_CONFLICT_STATUS_DETECTED,
			DetectedAt:   timestamppb.Now(),
			SourceMetadata: &storagev1.FileMetadata{
				FileId:     "file_conflict_1",
				FilePath:   "/documents/conflict.txt",
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
			TargetMetadata: &storagev1.FileMetadata{
				FileId:     "file_conflict_1",
				FilePath:   "/documents/conflict.txt",
				FileSize:   2048,
				MimeType:   "text/plain",
				CreatedAt:  timestamppb.Now(),
				ModifiedAt: timestamppb.Now(),
				SyncedAt:   timestamppb.Now(),
				Checksum:   "def456",
				Status:     commonv1.FileStatus_FILE_STATUS_SYNCED,
				Version:    2,
				ChunkCount: 2,
			},
			Context: map[string]string{
				"source_version": "v1.0",
				"target_version": "v2.0",
				"description":    "File version conflict",
			},
		},
	}

	return &syncv1.GetSyncConflictsResponse{
		Conflicts: conflicts,
		Pagination: &commonv1.PaginationResponse{
			NextCursor: "",
			HasMore:    false,
			TotalCount: 1,
		},
	}, nil
}

func (s *SyncService) ResolveConflict(ctx context.Context, req *syncv1.ResolveConflictRequest) (*syncv1.ResolveConflictResponse, error) {
	log.Printf("Resolving conflict: %s", req.ConflictId)

	return &syncv1.ResolveConflictResponse{
		Resolved:          true,
		ConflictId:        req.ConflictId,
		AppliedResolution: req.Resolution,
		ResolvedAt:        timestamppb.Now(),
	}, nil
}

func main() {
	log.Println("Starting Sync Service microservice...")

	// Создаем gRPC сервер
	grpcServer := grpc.NewServer()

	// Регистрируем сервисы
	syncService := NewSyncService()
	syncv1.RegisterSyncServiceServer(grpcServer, syncService)

	// Включаем reflection для разработки
	reflection.Register(grpcServer)

	// Настраиваем порт
	port := "50053"
	if envPort := os.Getenv("SYNC_SERVICE_PORT"); envPort != "" {
		port = envPort
	}

	// Создаем listener
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", port, err)
	}

	log.Printf("Sync Service listening on port %s", port)

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

	log.Println("Shutting down Sync Service...")

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
		log.Println("Sync Service stopped gracefully")
	}
}

// Вспомогательные методы для работы с устройствами

// getDeviceStatus получает статус устройства
func (s *SyncService) getDeviceStatus(deviceID string) *syncv1.SyncStatus {
	device, err := s.deviceManager.GetDevice(deviceID)
	if err != nil {
		log.Printf("Error getting device %s: %v", deviceID, err)
		return s.getDefaultSyncStatus(deviceID)
	}

	// Получаем файлы, ожидающие синхронизации
	pendingFiles := device.GetPendingFiles()
	pendingCount := int32(len(pendingFiles))

	// Получаем статистику по файлам
	totalFiles := int32(device.GetTotalFiles())
	completedCount := int32(device.GetCompletedFiles())

	log.Printf("Device %s: total=%d, pending=%d, completed=%d",
		deviceID, totalFiles, pendingCount, completedCount)

	return &syncv1.SyncStatus{
		NodeId:            device.DeviceID,
		SessionId:         device.DeviceID,
		PendingEvents:     pendingCount,
		ProcessingEvents:  0,
		CompletedEvents:   completedCount,
		FailedEvents:      0,
		LastSync:          timestamppb.New(device.LastSync),
		AvgProcessingTime: &durationpb.Duration{Seconds: 1},
		SuccessRate:       1.0,
		State:             syncv1.SyncState_SYNC_STATE_CONNECTING,
		StartedAt:         timestamppb.New(device.LastSync),
		ElapsedTime:       &durationpb.Duration{Seconds: int64(time.Since(device.LastSync).Seconds())},
		BytesTransferred:  1024,
		TransferRateBps:   102.4,
		ActiveConnections: 1,
	}
}

// getDefaultSyncStatus возвращает статус по умолчанию
func (s *SyncService) getDefaultSyncStatus(sessionID string) *syncv1.SyncStatus {
	return &syncv1.SyncStatus{
		NodeId:            "unknown",
		SessionId:         sessionID,
		PendingEvents:     0,
		ProcessingEvents:  0,
		CompletedEvents:   0,
		FailedEvents:      0,
		LastSync:          timestamppb.Now(),
		AvgProcessingTime: &durationpb.Duration{Seconds: 0},
		SuccessRate:       0.0,
		State:             syncv1.SyncState_SYNC_STATE_IDLE,
		StartedAt:         timestamppb.Now(),
		ElapsedTime:       &durationpb.Duration{Seconds: 0},
		BytesTransferred:  0,
		TransferRateBps:   0.0,
		ActiveConnections: 0,
	}
}

// RegisterDevice регистрирует новое устройство (HTTP/gRPC метод)
func (s *SyncService) RegisterDevice(ctx context.Context, userID, deviceName, storagePath string) (*storage.DeviceStorage, error) {
	return s.deviceManager.RegisterDevice(ctx, userID, deviceName, storagePath)
}

// ListUserDevices возвращает устройства пользователя
func (s *SyncService) ListUserDevices(userID string) []*storage.DeviceStorage {
	return s.deviceManager.ListUserDevices(userID)
}

// SyncDevice синхронизирует конкретное устройство
func (s *SyncService) SyncDevice(ctx context.Context, deviceID string) error {
	device, err := s.deviceManager.GetDevice(deviceID)
	if err != nil {
		return fmt.Errorf("device not found: %w", err)
	}

	// Получаем файлы для синхронизации
	pendingFiles := device.GetPendingFiles()
	log.Printf("Syncing %d files for device %s", len(pendingFiles), deviceID)

	// Синхронизируем каждый файл
	for _, file := range pendingFiles {
		if err := device.SyncFile(ctx, file.FilePath); err != nil {
			log.Printf("Failed to sync file %s: %v", file.FilePath, err)
		} else {
			log.Printf("Successfully synced file: %s", file.FilePath)
		}
	}

	return nil
}
