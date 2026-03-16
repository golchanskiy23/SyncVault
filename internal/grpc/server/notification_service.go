package server

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"syncvault/internal/grpc/interceptors"
	commonv1 "syncvault/internal/grpc/proto/common"
	notificationv1 "syncvault/internal/grpc/proto/notification"
)

// NotificationService реализация gRPC NotificationService с server streaming
type NotificationService struct {
	notificationv1.UnimplementedNotificationServiceServer
}

// NewNotificationService создает новый NotificationService
func NewNotificationService() *NotificationService {
	return &NotificationService{}
}

// Subscribe реализует server streaming для подписки на уведомления
func (n *NotificationService) Subscribe(req *notificationv1.SubscribeRequest, stream notificationv1.NotificationService_SubscribeServer) error {
	ctx := stream.Context()
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "authentication required")
	}

	fmt.Printf("Subscribe: user=%s, node=%s, types=%v\n", claims.UserID, claims.NodeID, req.Types)

	if req.UserId == "" {
		return status.Error(codes.InvalidArgument, "user ID is required")
	}
	if req.UserId != claims.UserID {
		return status.Error(codes.PermissionDenied, "cannot subscribe to notifications for another user")
	}

	subscriptionId := "sub-" + time.Now().Format("20060102150405")

	// proto: SubscriptionStatus — queued_notifications, delivered_notifications,
	// failed_notifications имеют тип int32
	statusResponse := &notificationv1.NotificationResponse{
		Response: &notificationv1.NotificationResponse_Status{
			Status: &notificationv1.SubscriptionStatus{
				SubscriptionId:         subscriptionId,
				Active:                 true,
				SubscribedAt:           timestamppb.Now(),
				LastActivity:           timestamppb.Now(),
				QueuedNotifications:    0,
				DeliveredNotifications: 0,
				FailedNotifications:    0,
				SubscribedTypes:        req.Types,
				Channels:               req.Channels,
				AvgDeliveryTime:        durationpb.New(50 * time.Millisecond),
				DeliveryRate:           0.98,
			},
		},
	}

	if err := stream.Send(statusResponse); err != nil {
		return status.Errorf(codes.Internal, "failed to send subscription status: %v", err)
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	eventCounter := int32(0)
	for {
		select {
		case <-ctx.Done():
			fmt.Printf("Client %s disconnected from notification stream\n", claims.UserID)
			return nil
		case <-ticker.C:
			eventCounter++

			notification := &notificationv1.Notification{
				NotificationId: fmt.Sprintf("notif-%d", eventCounter),
				UserId:         claims.UserID,
				NodeId:         claims.NodeID,
				Type:           notificationv1.NotificationType_NOTIFICATION_TYPE_SYNC_COMPLETED,
				Priority:       notificationv1.NotificationPriority_NOTIFICATION_PRIORITY_NORMAL,
				Title:          fmt.Sprintf("Sync Event %d", eventCounter),
				Message:        fmt.Sprintf("Sync operation completed for user %s", claims.UserID),
				CreatedAt:      timestamppb.Now(),
				Read:           false,
				Source:         "sync-service",
				Category:       "sync",
				Tags:           []string{"sync", "completed"},
				Actions: []*notificationv1.Action{
					{
						ActionId: "view-details",
						Label:    "View Details",
						Type:     notificationv1.ActionType_ACTION_TYPE_VIEW_DETAILS,
						Primary:  true,
					},
					{
						ActionId: "dismiss",
						Label:    "Dismiss",
						Type:     notificationv1.ActionType_ACTION_TYPE_DISMISS,
						Primary:  false,
					},
				},
			}

			if err := stream.Send(&notificationv1.NotificationResponse{
				Response: &notificationv1.NotificationResponse_Notification{
					Notification: notification,
				},
			}); err != nil {
				return status.Errorf(codes.Internal, "failed to send notification: %v", err)
			}

			// proto: Heartbeat.sequence_number — int32
			if eventCounter%5 == 0 {
				heartbeat := &notificationv1.NotificationResponse{
					Response: &notificationv1.NotificationResponse_Heartbeat{
						Heartbeat: &notificationv1.Heartbeat{
							Timestamp:      timestamppb.Now(),
							SequenceNumber: eventCounter / 5, // int32
							Status: &notificationv1.SubscriptionStatus{
								SubscriptionId:         subscriptionId,
								Active:                 true,
								QueuedNotifications:    0,
								DeliveredNotifications: eventCounter, // int32
								FailedNotifications:    0,
								AvgDeliveryTime:        durationpb.New(45 * time.Millisecond),
								DeliveryRate:           0.99,
							},
							SubscriptionId: subscriptionId,
						},
					},
				}

				if err := stream.Send(heartbeat); err != nil {
					return status.Errorf(codes.Internal, "failed to send heartbeat: %v", err)
				}
			}
		}
	}
}

// SendNotification отправляет уведомление
func (n *NotificationService) SendNotification(ctx context.Context, req *notificationv1.SendNotificationRequest) (*notificationv1.SendNotificationResponse, error) {
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	if err := interceptors.RequireRole(ctx, "admin"); err != nil {
		return nil, err
	}

	fmt.Printf("SendNotification: from=%s, to=%s, type=%v\n", claims.UserID, req.UserId, req.Type)

	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user ID is required")
	}
	if req.Title == "" {
		return nil, status.Error(codes.InvalidArgument, "title is required")
	}
	if req.Message == "" {
		return nil, status.Error(codes.InvalidArgument, "message is required")
	}

	return &notificationv1.SendNotificationResponse{
		NotificationId: fmt.Sprintf("notif-%d", time.Now().Unix()),
		Sent:           true,
		SentAt:         timestamppb.Now(),
		FailedChannels: []string{},
		DeliveryStatus: map[string]string{
			"grpc":  "delivered",
			"email": "skipped",
			"push":  "skipped",
		},
	}, nil
}

// GetNotifications получает уведомления
func (n *NotificationService) GetNotifications(ctx context.Context, req *notificationv1.GetNotificationsRequest) (*notificationv1.GetNotificationsResponse, error) {
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	if err := interceptors.RequireUserID(ctx, claims.UserID); err != nil {
		return nil, err
	}

	fmt.Printf("GetNotifications: user=%s, types=%v, include_read=%v\n", claims.UserID, req.Types, req.IncludeRead)

	return &notificationv1.GetNotificationsResponse{
		Notifications: []*notificationv1.Notification{
			{
				NotificationId: "notif-1",
				UserId:         claims.UserID,
				NodeId:         claims.NodeID,
				Type:           notificationv1.NotificationType_NOTIFICATION_TYPE_FILE_CREATED,
				Priority:       notificationv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH,
				Title:          "New File Created",
				Message:        "File 'document.txt' was created",
				CreatedAt:      timestamppb.New(time.Now().Add(-1 * time.Hour)),
				Read:           false,
				Source:         "storage-service",
				Category:       "files",
				Tags:           []string{"file", "created"},
			},
			{
				NotificationId: "notif-2",
				UserId:         claims.UserID,
				NodeId:         claims.NodeID,
				Type:           notificationv1.NotificationType_NOTIFICATION_TYPE_SYNC_COMPLETED,
				Priority:       notificationv1.NotificationPriority_NOTIFICATION_PRIORITY_NORMAL,
				Title:          "Sync Completed",
				Message:        "All files synchronized successfully",
				CreatedAt:      timestamppb.New(time.Now().Add(-2 * time.Hour)),
				Read:           true,
				ReadAt:         timestamppb.New(time.Now().Add(-90 * time.Minute)),
				Source:         "sync-service",
				Category:       "sync",
				Tags:           []string{"sync", "completed"},
			},
		},
		// proto: PaginationResponse имеет поля next_cursor, has_more, total_count
		Pagination: &commonv1.PaginationResponse{
			NextCursor: "",
			HasMore:    false,
			TotalCount: 2,
		},
		Summary: &notificationv1.NotificationSummary{
			TotalCount:   50,
			UnreadCount:  25,
			ExpiredCount: 5,
			CountByType: map[string]int32{
				"NOTIFICATION_TYPE_FILE_CREATED":      20,
				"NOTIFICATION_TYPE_SYNC_COMPLETED":    15,
				"NOTIFICATION_TYPE_FILE_DELETED":      10,
				"NOTIFICATION_TYPE_CONFLICT_DETECTED": 5,
			},
			CountByPriority: map[string]int32{
				"NOTIFICATION_PRIORITY_HIGH":   10,
				"NOTIFICATION_PRIORITY_NORMAL": 30,
				"NOTIFICATION_PRIORITY_LOW":    10,
			},
			OldestNotification: timestamppb.New(time.Now().Add(-24 * time.Hour)),
			NewestNotification: timestamppb.New(time.Now().Add(-5 * time.Minute)),
		},
	}, nil
}

// MarkAsRead помечает уведомление как прочитанное
func (n *NotificationService) MarkAsRead(ctx context.Context, req *notificationv1.MarkAsReadRequest) (*emptypb.Empty, error) {
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	if err := interceptors.RequireUserID(ctx, claims.UserID); err != nil {
		return nil, err
	}

	fmt.Printf("MarkAsRead: user=%s, notification=%s\n", claims.UserID, req.NotificationId)

	return &emptypb.Empty{}, nil
}

// MarkAllAsRead помечает все уведомления как прочитанные
func (n *NotificationService) MarkAllAsRead(ctx context.Context, req *notificationv1.MarkAllAsReadRequest) (*notificationv1.MarkAllAsReadResponse, error) {
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	if err := interceptors.RequireUserID(ctx, claims.UserID); err != nil {
		return nil, err
	}

	fmt.Printf("MarkAllAsRead: user=%s, types=%v\n", claims.UserID, req.Types)

	return &notificationv1.MarkAllAsReadResponse{
		MarkedCount: 25,
		MarkedAt:    timestamppb.Now(),
	}, nil
}

// DeleteNotification удаляет уведомление
func (n *NotificationService) DeleteNotification(ctx context.Context, req *notificationv1.DeleteNotificationRequest) (*emptypb.Empty, error) {
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	if err := interceptors.RequireUserID(ctx, claims.UserID); err != nil {
		return nil, err
	}

	fmt.Printf("DeleteNotification: user=%s, notification=%s\n", claims.UserID, req.NotificationId)

	return &emptypb.Empty{}, nil
}

// GetNotificationSettings получает настройки уведомлений
func (n *NotificationService) GetNotificationSettings(ctx context.Context, req *notificationv1.GetNotificationSettingsRequest) (*notificationv1.GetNotificationSettingsResponse, error) {
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	if err := interceptors.RequireUserID(ctx, claims.UserID); err != nil {
		return nil, err
	}

	fmt.Printf("GetNotificationSettings: user=%s\n", claims.UserID)

	return &notificationv1.GetNotificationSettingsResponse{
		Settings: &notificationv1.NotificationSettings{
			UserId:  claims.UserID,
			NodeId:  claims.NodeID,
			Enabled: true,
			TypeSettings: map[string]bool{
				"NOTIFICATION_TYPE_FILE_CREATED":      true,
				"NOTIFICATION_TYPE_SYNC_COMPLETED":    true,
				"NOTIFICATION_TYPE_FILE_DELETED":      true,
				"NOTIFICATION_TYPE_CONFLICT_DETECTED": true,
			},
			PrioritySettings: map[string]bool{
				"NOTIFICATION_PRIORITY_HIGH":     true,
				"NOTIFICATION_PRIORITY_NORMAL":   true,
				"NOTIFICATION_PRIORITY_LOW":      false,
				"NOTIFICATION_PRIORITY_CRITICAL": true,
			},
			ChannelSettings: map[string]*notificationv1.ChannelSettings{
				"grpc": {
					Enabled: true,
					TypeSettings: map[string]bool{
						"NOTIFICATION_TYPE_FILE_CREATED":   true,
						"NOTIFICATION_TYPE_SYNC_COMPLETED": true,
					},
					PrioritySettings: map[string]bool{
						"NOTIFICATION_PRIORITY_HIGH":   true,
						"NOTIFICATION_PRIORITY_NORMAL": true,
					},
					MaxFrequency:      100,
					QuietHoursStart:   durationpb.New(1 * time.Hour),
					QuietHoursEnd:     durationpb.New(80 * time.Minute),
					QuietHoursEnabled: false,
					ChannelSpecific:   map[string]string{"format": "json"},
				},
				"email": {
					Enabled:           false,
					TypeSettings:      map[string]bool{},
					PrioritySettings:  map[string]bool{},
					MaxFrequency:      10,
					QuietHoursStart:   durationpb.New(1 * time.Hour),
					QuietHoursEnd:     durationpb.New(80 * time.Minute),
					QuietHoursEnabled: true,
					ChannelSpecific:   map[string]string{},
				},
			},
			QuietHoursStart:       durationpb.New(1 * time.Hour),
			QuietHoursEnd:         durationpb.New(80 * time.Minute),
			QuietHoursEnabled:     false,
			MaxDailyNotifications: 100,
			BlockedSources:        []string{"spam-source"},
			AllowedSources:        []string{"sync-service", "storage-service"},
			GroupSimilar:          true,
			AutoReadDelay:         durationpb.New(5 * time.Minute),
			PreferredLanguages:    []string{"en", "ru"},
			CustomSettings: map[string]string{
				"theme": "dark",
				"sound": "enabled",
			},
			UpdatedAt: timestamppb.Now(),
		},
	}, nil
}

// UpdateNotificationSettings обновляет настройки уведомлений
func (n *NotificationService) UpdateNotificationSettings(ctx context.Context, req *notificationv1.UpdateNotificationSettingsRequest) (*notificationv1.UpdateNotificationSettingsResponse, error) {
	claims, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	if err := interceptors.RequireUserID(ctx, claims.UserID); err != nil {
		return nil, err
	}

	fmt.Printf("UpdateNotificationSettings: user=%s\n", claims.UserID)

	return &notificationv1.UpdateNotificationSettingsResponse{
		Updated:   true,
		Settings:  req.Settings,
		UpdatedAt: timestamppb.Now(),
	}, nil
}

// CreateNotificationTemplate создает шаблон уведомления
func (n *NotificationService) CreateNotificationTemplate(ctx context.Context, req *notificationv1.CreateNotificationTemplateRequest) (*notificationv1.CreateNotificationTemplateResponse, error) {
	_, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	if err := interceptors.RequireRole(ctx, "admin"); err != nil {
		return nil, err
	}

	fmt.Printf("CreateNotificationTemplate: type=%v, name=%s\n", req.Type, req.Name)

	return &notificationv1.CreateNotificationTemplateResponse{
		TemplateId: req.TemplateId,
		Created:    true,
		CreatedAt:  timestamppb.Now(),
	}, nil
}

// GetNotificationTemplates получает шаблоны уведомлений
func (n *NotificationService) GetNotificationTemplates(ctx context.Context, req *notificationv1.GetNotificationTemplatesRequest) (*notificationv1.GetNotificationTemplatesResponse, error) {
	_, ok := interceptors.GetClaimsFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authentication required")
	}

	fmt.Printf("GetNotificationTemplates: types=%v, active_only=%v\n", req.Types, req.ActiveOnly)

	return &notificationv1.GetNotificationTemplatesResponse{
		Templates: []*notificationv1.NotificationTemplate{
			{
				TemplateId:      "sync-completed",
				Type:            notificationv1.NotificationType_NOTIFICATION_TYPE_SYNC_COMPLETED,
				Name:            "Sync Completed Template",
				Description:     "Template for sync completion notifications",
				TitleTemplate:   "Sync Completed: {{.FileCount}} files processed",
				MessageTemplate: "Successfully synchronized {{.FileCount}} files from {{.SourceNode}} to {{.TargetNode}}",
				DefaultData: map[string]string{
					"FileCount":  "0",
					"SourceNode": "",
					"TargetNode": "",
				},
				SupportedLanguages: []string{"en", "ru"},
				LocalizedTemplates: map[string]*notificationv1.LocalizedTemplate{
					"en": {
						Language:        "en",
						TitleTemplate:   "Sync Completed: {{.FileCount}} files",
						MessageTemplate: "Successfully synchronized {{.FileCount}} files",
						LocalizedData:   map[string]string{"files": "files", "successfully": "successfully"},
					},
					"ru": {
						Language:        "ru",
						TitleTemplate:   "Синхронизация завершена: {{.FileCount}} файлов",
						MessageTemplate: "Успешно синхронизировано {{.FileCount}} файлов",
						LocalizedData:   map[string]string{"files": "файлов", "successfully": "успешно"},
					},
				},
				DefaultActions: []*notificationv1.Action{
					{
						ActionId: "view-details",
						Label:    "View Details",
						Type:     notificationv1.ActionType_ACTION_TYPE_VIEW_DETAILS,
						Primary:  true,
					},
				},
				Active:    true,
				CreatedAt: timestamppb.New(time.Now().Add(-7 * 24 * time.Hour)),
				UpdatedAt: timestamppb.New(time.Now().Add(-1 * time.Hour)),
				ExpiresAt: timestamppb.New(time.Now().Add(365 * 24 * time.Hour)),
				Metadata:  map[string]string{"category": "sync", "version": "1.0"},
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

// HealthCheck проверяет здоровье сервиса уведомлений
func (n *NotificationService) HealthCheck(ctx context.Context, req *emptypb.Empty) (*notificationv1.HealthCheckResponse, error) {
	fmt.Printf("HealthCheck called\n")

	return &notificationv1.HealthCheckResponse{
		Healthy:   true,
		Version:   "1.0.0",
		Timestamp: timestamppb.Now(),
		Details: map[string]string{
			"active_subscriptions": "150",
			"queued_notifications": "25",
			"delivery_rate":        "0.98",
		},
		Dependencies:        []string{"database", "template-service", "email-service"},
		ActiveSubscriptions: 150,
		QueuedNotifications: 25,
		AvgDeliveryTime:     durationpb.New(45 * time.Millisecond),
		DeliveryRate:        0.98,
	}, nil
}
