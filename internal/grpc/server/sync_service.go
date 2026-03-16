package server

import (
	"context"
	"fmt"
	"io"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"syncvault/internal/grpc/interceptors"
	commonv1 "syncvault/internal/grpc/proto/common"
	storagev1 "syncvault/internal/grpc/proto/storage"
	syncv1 "syncvault/internal/grpc/proto/sync"
)

// SyncService реализация gRPC SyncService с bidirectional streaming
type SyncService struct {
	syncv1.UnimplementedSyncServiceServer
}

// NewSyncService создает новый SyncService
func NewSyncService() *SyncService {
	return &SyncService{}
}

// StartSync реализует bidirectional streaming для синхронизации
func (s *SyncService) StartSync(stream syncv1.SyncService_StartSyncServer) error {
	ctx := stream.Context()
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "authentication required")
	}

	fmt.Printf("StartSync stream started for user %s, node %s\n", claims.UserID, claims.NodeID)

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			fmt.Printf("Client closed stream for user %s\n", claims.UserID)
			return nil
		}
		if err != nil {
			return status.Errorf(codes.Internal, "stream error: %v", err)
		}

		switch request := req.Request.(type) {
		case *syncv1.SyncRequest_Start:
			if err := s.handleSyncStart(ctx, stream, request.Start, claims); err != nil {
				return err
			}
		case *syncv1.SyncRequest_Ack:
			if err := s.handleSyncAck(ctx, stream, request.Ack, claims); err != nil {
				return err
			}
		case *syncv1.SyncRequest_Complete:
			if err := s.handleSyncComplete(ctx, stream, request.Complete, claims); err != nil {
				return err
			}
		case *syncv1.SyncRequest_Error:
			if err := s.handleSyncError(ctx, stream, request.Error, claims); err != nil {
				return err
			}
		case *syncv1.SyncRequest_Ping:
			if err := s.handleSyncPing(ctx, stream, request.Ping, claims); err != nil {
				return err
			}
		default:
			fmt.Printf("Unknown request type: %T\n", request)
		}
	}
}

// handleSyncStart обрабатывает начало синхронизации
func (s *SyncService) handleSyncStart(ctx context.Context, stream syncv1.SyncService_StartSyncServer, req *syncv1.SyncStart, claims *interceptors.Claims) error {
	fmt.Printf("Sync start: node=%s, mode=%v\n", req.NodeId, req.Mode)

	// proto SyncStatus: pending_events, processing_events, completed_events, failed_events,
	// last_sync, avg_processing_time, success_rate, state, started_at, elapsed_time,
	// bytes_transferred, transfer_rate_bps, active_connections
	return stream.Send(&syncv1.SyncResponse{
		Response: &syncv1.SyncResponse_Status{
			Status: &syncv1.SyncStatus{
				NodeId:            req.NodeId,
				SessionId:         "session-" + time.Now().Format("20060102150405"),
				State:             syncv1.SyncState_SYNC_STATE_SYNCING,
				LastSync:          timestamppb.Now(),
				PendingEvents:     0,
				ProcessingEvents:  0,
				CompletedEvents:   0,
				FailedEvents:      0,
				SuccessRate:       0.0,
				StartedAt:         timestamppb.Now(),
				ElapsedTime:       durationpb.New(0),
				BytesTransferred:  0,
				TransferRateBps:   0.0,
				ActiveConnections: 1,
			},
		},
	})
}

// handleSyncAck обрабатывает подтверждение
func (s *SyncService) handleSyncAck(ctx context.Context, stream syncv1.SyncService_StartSyncServer, req *syncv1.SyncAck, claims *interceptors.Claims) error {
	fmt.Printf("Sync ack: event=%s, success=%v\n", req.EventId, req.Success)
	return nil
}

// handleSyncComplete обрабатывает завершение синхронизации
func (s *SyncService) handleSyncComplete(ctx context.Context, stream syncv1.SyncService_StartSyncServer, req *syncv1.SyncComplete, claims *interceptors.Claims) error {
	fmt.Printf("Sync complete: session=%s, events=%d, failed=%d\n", req.SessionId, req.EventsProcessed, req.EventsFailed)

	return stream.Send(&syncv1.SyncResponse{
		Response: &syncv1.SyncResponse_Complete{
			Complete: req,
		},
	})
}

// handleSyncError обрабатывает ошибку синхронизации
func (s *SyncService) handleSyncError(ctx context.Context, stream syncv1.SyncService_StartSyncServer, req *syncv1.SyncError, claims *interceptors.Claims) error {
	fmt.Printf("Sync error: event=%s, code=%s, message=%s\n", req.EventId, req.ErrorCode, req.ErrorMessage)

	return stream.Send(&syncv1.SyncResponse{
		Response: &syncv1.SyncResponse_Error{
			Error: req,
		},
	})
}

// handleSyncPing обрабатывает ping
func (s *SyncService) handleSyncPing(ctx context.Context, stream syncv1.SyncService_StartSyncServer, req *syncv1.SyncPing, claims *interceptors.Claims) error {
	fmt.Printf("Sync ping: node=%s, seq=%d\n", req.NodeId, req.SequenceNumber)

	// proto SyncPong: node_id, timestamp, sequence_number, status, session_id
	return stream.Send(&syncv1.SyncResponse{
		Response: &syncv1.SyncResponse_Pong{
			Pong: &syncv1.SyncPong{
				NodeId:         req.NodeId,
				Timestamp:      timestamppb.Now(),
				SequenceNumber: req.SequenceNumber,
				SessionId:      req.SessionId,
				Status: &syncv1.SyncStatus{
					NodeId:            req.NodeId,
					SessionId:         req.SessionId,
					State:             syncv1.SyncState_SYNC_STATE_SYNCING,
					LastSync:          timestamppb.Now(),
					PendingEvents:     0,
					ProcessingEvents:  0,
					CompletedEvents:   0,
					FailedEvents:      0,
					SuccessRate:       1.0,
					StartedAt:         timestamppb.Now(),
					ElapsedTime:       durationpb.New(0),
					BytesTransferred:  0,
					TransferRateBps:   0.0,
					ActiveConnections: 1,
				},
			},
		},
	})
}

// SubscribeEvents реализует server streaming для подписки на события
func (s *SyncService) SubscribeEvents(req *syncv1.SubscribeEventsRequest, stream syncv1.SyncService_SubscribeEventsServer) error {
	ctx := stream.Context()
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "authentication required")
	}

	fmt.Printf("SubscribeEvents: user=%s, types=%v\n", claims.UserID, req.EventTypes)

	subscriptionId := "sub-" + time.Now().Format("20060102150405")

	// proto SubscriptionStatus (sync): subscription_id, active, queued_events,
	// processed_events, subscribed_at, last_activity, subscribed_types
	// НЕТ полей: avg_delivery_time, delivery_rate, channels, failed_notifications
	if err := stream.Send(&syncv1.SyncEventResponse{
		Response: &syncv1.SyncEventResponse_Status{
			Status: &syncv1.SubscriptionStatus{
				SubscriptionId:  subscriptionId,
				Active:          true,
				SubscribedAt:    timestamppb.Now(),
				LastActivity:    timestamppb.Now(),
				QueuedEvents:    0,
				ProcessedEvents: 0,
				SubscribedTypes: req.EventTypes,
			},
		},
	}); err != nil {
		return status.Errorf(codes.Internal, "failed to send status: %v", err)
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Printf("Client disconnected from events stream\n")
			return nil
		case <-ticker.C:
			// proto Heartbeat (sync): timestamp, sequence_number, status
			// НЕТ поля subscription_id
			if err := stream.Send(&syncv1.SyncEventResponse{
				Response: &syncv1.SyncEventResponse_Heartbeat{
					Heartbeat: &syncv1.Heartbeat{
						Timestamp:      timestamppb.Now(),
						SequenceNumber: 1,
						Status: &syncv1.SyncStatus{
							NodeId:            claims.NodeID,
							SessionId:         subscriptionId,
							State:             syncv1.SyncState_SYNC_STATE_SYNCING,
							LastSync:          timestamppb.Now(),
							PendingEvents:     0,
							ProcessingEvents:  0,
							CompletedEvents:   10,
							FailedEvents:      0,
							SuccessRate:       1.0,
							StartedAt:         timestamppb.Now(),
							ElapsedTime:       durationpb.New(5 * time.Minute),
							BytesTransferred:  10240,
							TransferRateBps:   34.13,
							ActiveConnections: 1,
						},
					},
				},
			}); err != nil {
				return status.Errorf(codes.Internal, "failed to send heartbeat: %v", err)
			}
		}
	}
}

// GetSyncStatus получает статус синхронизации
func (s *SyncService) GetSyncStatus(ctx context.Context, req *syncv1.GetSyncStatusRequest) (*syncv1.GetSyncStatusResponse, error) {
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	fmt.Printf("GetSyncStatus: user=%s, session=%s\n", claims.UserID, req.SessionId)

	return &syncv1.GetSyncStatusResponse{
		Statuses: []*syncv1.SyncStatus{
			{
				NodeId:            claims.NodeID,
				SessionId:         req.SessionId,
				State:             syncv1.SyncState_SYNC_STATE_SYNCING,
				LastSync:          timestamppb.Now(),
				PendingEvents:     0,
				ProcessingEvents:  0,
				CompletedEvents:   50,
				FailedEvents:      2,
				SuccessRate:       0.96,
				StartedAt:         timestamppb.New(time.Now().Add(-5 * time.Minute)),
				ElapsedTime:       durationpb.New(5 * time.Minute),
				BytesTransferred:  102400,
				TransferRateBps:   341.33,
				ActiveConnections: 1,
			},
		},
		// proto SyncSummary: last_activity — Timestamp
		Summary: &syncv1.SyncSummary{
			TotalSessions:         1,
			ActiveSessions:        1,
			TotalEvents:           52,
			PendingEvents:         0,
			FailedEvents:          2,
			OverallSuccessRate:    0.96,
			LastActivity:          timestamppb.Now(),
			TotalBytesTransferred: 102400,
		},
	}, nil
}

// ForceSync принудительная синхронизация
func (s *SyncService) ForceSync(ctx context.Context, req *syncv1.ForceSyncRequest) (*syncv1.ForceSyncResponse, error) {
	_, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	if err := interceptors.RequireRole(ctx, "admin"); err != nil {
		return nil, err
	}

	fmt.Printf("ForceSync: source=%s, target=%s, priority=%v\n", req.SourceNodeId, req.TargetNodeId, req.Priority)

	sessionId := "force-" + time.Now().Format("20060102150405")

	return &syncv1.ForceSyncResponse{
		SessionId: sessionId,
		StartedAt: timestamppb.Now(),
		Status: &syncv1.SyncStatus{
			NodeId:            req.SourceNodeId,
			SessionId:         sessionId,
			State:             syncv1.SyncState_SYNC_STATE_SYNCING,
			LastSync:          timestamppb.Now(),
			PendingEvents:     0,
			ProcessingEvents:  0,
			CompletedEvents:   0,
			FailedEvents:      0,
			SuccessRate:       0.0,
			StartedAt:         timestamppb.Now(),
			ElapsedTime:       durationpb.New(0),
			BytesTransferred:  0,
			TransferRateBps:   0.0,
			ActiveConnections: 1,
		},
	}, nil
}

// CancelSync отмена синхронизации
func (s *SyncService) CancelSync(ctx context.Context, req *syncv1.CancelSyncRequest) (*emptypb.Empty, error) {
	_, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	fmt.Printf("CancelSync: session=%s, reason=%s\n", req.SessionId, req.Reason)

	return &emptypb.Empty{}, nil
}

// GetSyncHistory получает историю синхронизации
func (s *SyncService) GetSyncHistory(ctx context.Context, req *syncv1.GetSyncHistoryRequest) (*syncv1.GetSyncHistoryResponse, error) {
	_, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	fmt.Printf("GetSyncHistory: from=%v, to=%v\n", req.From, req.To)

	return &syncv1.GetSyncHistoryResponse{
		Entries: []*syncv1.SyncHistoryEntry{
			{
				EventId:      "event-1",
				Type:         commonv1.SyncEventType_SYNC_EVENT_TYPE_FILE_CREATED,
				SourceNodeId: "local",
				TargetNodeId: "node-2",
				FileMetadata: &storagev1.FileMetadata{
					FileId:     "file-1",
					FilePath:   "/path/to/file1.txt",
					FileSize:   1024,
					CreatedAt:  timestamppb.Now(),
					ModifiedAt: timestamppb.Now(),
				},
				Timestamp:  timestamppb.Now(),
				Result:     syncv1.SyncResult_SYNC_RESULT_SUCCESS,
				Duration:   durationpb.New(5 * time.Second),
				RetryCount: 0,
			},
		},
		// proto: PaginationResponse — next_cursor, has_more, total_count
		Pagination: &commonv1.PaginationResponse{
			NextCursor: "",
			HasMore:    false,
			TotalCount: 1,
		},
	}, nil
}

// GetSyncConflicts получает конфликты синхронизации
func (s *SyncService) GetSyncConflicts(ctx context.Context, req *syncv1.GetSyncConflictsRequest) (*syncv1.GetSyncConflictsResponse, error) {
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	fmt.Printf("GetSyncConflicts: user=%s, status=%v\n", claims.UserID, req.Status)

	return &syncv1.GetSyncConflictsResponse{
		Conflicts: []*syncv1.SyncConflict{
			{
				ConflictId:   "conflict-1",
				FileId:       "file-1",
				FilePath:     "/path/to/file1.txt",
				SourceNodeId: claims.NodeID,
				TargetNodeId: "node-2",
				Type:         syncv1.ConflictType_CONFLICT_TYPE_CONTENT,
				Status:       syncv1.ConflictStatus_CONFLICT_STATUS_DETECTED,
				DetectedAt:   timestamppb.Now(),
				SourceMetadata: &storagev1.FileMetadata{
					FileId:     "file-1",
					FilePath:   "/path/to/file1.txt",
					FileSize:   1024,
					CreatedAt:  timestamppb.Now(),
					ModifiedAt: timestamppb.Now(),
				},
				TargetMetadata: &storagev1.FileMetadata{
					FileId:     "file-1",
					FilePath:   "/path/to/file1.txt",
					FileSize:   2048,
					CreatedAt:  timestamppb.Now(),
					ModifiedAt: timestamppb.Now(),
				},
				ResolutionStrategy: "manual",
			},
		},
		Pagination: &commonv1.PaginationResponse{
			NextCursor: "",
			HasMore:    false,
			TotalCount: 1,
		},
	}, nil
}

// ResolveConflict разрешает конфликт синхронизации
func (s *SyncService) ResolveConflict(ctx context.Context, req *syncv1.ResolveConflictRequest) (*syncv1.ResolveConflictResponse, error) {
	_, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	fmt.Printf("ResolveConflict: conflict=%s, resolution=%v, resolved_by=%s\n", req.ConflictId, req.Resolution, req.ResolvedBy)

	return &syncv1.ResolveConflictResponse{
		Resolved:          true,
		ConflictId:        req.ConflictId,
		AppliedResolution: req.Resolution,
		ResolvedAt:        timestamppb.Now(),
	}, nil
}

// HealthCheck проверяет здоровье сервиса синхронизации
func (s *SyncService) HealthCheck(ctx context.Context, req *emptypb.Empty) (*syncv1.HealthCheckResponse, error) {
	fmt.Printf("HealthCheck called\n")

	return &syncv1.HealthCheckResponse{
		Healthy:   true,
		Version:   "1.0.0",
		Timestamp: timestamppb.Now(),
		Details: map[string]string{
			"active_sessions": "5",
			"pending_events":  "10",
			"failed_events":   "2",
		},
		Dependencies: []string{
			"storage-service",
			"notification-service",
			"database",
		},
	}, nil
}
