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
)

// SyncService микросервис для синхронизации файлов
type SyncService struct {
	syncv1.UnimplementedSyncServiceServer
}

func (s *SyncService) StartSync(stream syncv1.SyncService_StartSyncServer) error {
	log.Printf("Starting sync stream")

	// This is a streaming endpoint, so we need to implement streaming
	// For this simple example, we'll just return an error
	return fmt.Errorf("streaming not implemented in this simple example")
}

func (s *SyncService) GetSyncStatus(ctx context.Context, req *syncv1.GetSyncStatusRequest) (*syncv1.GetSyncStatusResponse, error) {
	log.Printf("Getting sync status for session: %s", req.SessionId)

	// Имитация статуса синхронизации
	statuses := []*syncv1.SyncStatus{
		{
			NodeId:            "node_1",
			SessionId:         req.SessionId,
			PendingEvents:     0,
			ProcessingEvents:  0,
			CompletedEvents:   5,
			FailedEvents:      0,
			LastSync:          timestamppb.Now(),
			AvgProcessingTime: &durationpb.Duration{Seconds: 1},
			SuccessRate:       1.0,
			State:             syncv1.SyncState_SYNC_STATE_COMPLETED,
			StartedAt:         timestamppb.Now(),
			ElapsedTime:       &durationpb.Duration{Seconds: 10},
			BytesTransferred:  1024,
			TransferRateBps:   102.4,
			ActiveConnections: 1,
		},
	}

	return &syncv1.GetSyncStatusResponse{
		Statuses: statuses,
		Summary: &syncv1.SyncSummary{
			TotalSessions:         1,
			ActiveSessions:        0,
			TotalEvents:           5,
			PendingEvents:         0,
			FailedEvents:          0,
			OverallSuccessRate:    1.0,
			LastActivity:          timestamppb.Now(),
			TotalBytesTransferred: 1024,
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
	syncService := &SyncService{}
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
