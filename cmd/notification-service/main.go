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
	notificationv1 "syncvault/internal/grpc/proto/notification"
)

// NotificationService микросервис для управления уведомлениями
type NotificationService struct {
	notificationv1.UnimplementedNotificationServiceServer
}

func (s *NotificationService) SendNotification(ctx context.Context, req *notificationv1.SendNotificationRequest) (*notificationv1.SendNotificationResponse, error) {
	log.Printf("Sending notification to user: %s, type: %v", req.UserId, req.Type)

	// Имитация отправки уведомления
	notificationID := fmt.Sprintf("notif_%d", time.Now().UnixNano())

	return &notificationv1.SendNotificationResponse{
		NotificationId: notificationID,
		Sent:           true,
		SentAt:         timestamppb.Now(),
	}, nil
}

func (s *NotificationService) GetNotifications(ctx context.Context, req *notificationv1.GetNotificationsRequest) (*notificationv1.GetNotificationsResponse, error) {
	log.Printf("Getting notifications for user: %s", req.UserId)

	// Имитация списка уведомлений
	notifications := []*notificationv1.Notification{
		{
			NotificationId: "notif_1",
			UserId:         req.UserId,
			Type:           notificationv1.NotificationType_NOTIFICATION_TYPE_FILE_CREATED,
			Title:          "File Created",
			Message:        "Your file has been created successfully",
			Priority:       notificationv1.NotificationPriority_NOTIFICATION_PRIORITY_NORMAL,
			Read:           false,
			CreatedAt:      timestamppb.Now(),
		},
		{
			NotificationId: "notif_2",
			UserId:         req.UserId,
			Type:           notificationv1.NotificationType_NOTIFICATION_TYPE_SYNC_COMPLETED,
			Title:          "Sync Completed",
			Message:        "File synchronization completed",
			Priority:       notificationv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW,
			Read:           true,
			CreatedAt:      timestamppb.Now(),
		},
	}

	return &notificationv1.GetNotificationsResponse{
		Notifications: notifications,
		Pagination: &commonv1.PaginationResponse{
			NextCursor: "",
			HasMore:    false,
			TotalCount: int32(len(notifications)),
		},
		Summary: &notificationv1.NotificationSummary{
			TotalCount:   int32(len(notifications)),
			UnreadCount:  1,
			ExpiredCount: 0,
			CountByType: map[string]int32{
				"FILE_CREATED":   1,
				"SYNC_COMPLETED": 1,
			},
			CountByPriority: map[string]int32{
				"NORMAL": 1,
				"LOW":    1,
			},
			OldestNotification: timestamppb.Now(),
			NewestNotification: timestamppb.Now(),
		},
	}, nil
}

func (s *NotificationService) MarkAsRead(ctx context.Context, req *notificationv1.MarkAsReadRequest) (*emptypb.Empty, error) {
	log.Printf("Marking notification as read: %s", req.NotificationId)

	return &emptypb.Empty{}, nil
}

func (s *NotificationService) MarkAllAsRead(ctx context.Context, req *notificationv1.MarkAllAsReadRequest) (*notificationv1.MarkAllAsReadResponse, error) {
	log.Printf("Marking all notifications as read for user: %s", req.UserId)

	return &notificationv1.MarkAllAsReadResponse{
		MarkedCount: 5, // Имитация
		MarkedAt:    timestamppb.Now(),
	}, nil
}

func (s *NotificationService) DeleteNotification(ctx context.Context, req *notificationv1.DeleteNotificationRequest) (*emptypb.Empty, error) {
	log.Printf("Deleting notification: %s", req.NotificationId)

	return &emptypb.Empty{}, nil
}

func (s *NotificationService) Subscribe(req *notificationv1.SubscribeRequest, stream notificationv1.NotificationService_SubscribeServer) error {
	log.Printf("User %s subscribing to notifications", req.UserId)

	// This is a streaming endpoint, so we need to implement streaming
	// For this simple example, we'll just return an error
	return fmt.Errorf("streaming not implemented in this simple example")
}

func (s *NotificationService) GetNotificationSettings(ctx context.Context, req *notificationv1.GetNotificationSettingsRequest) (*notificationv1.GetNotificationSettingsResponse, error) {
	log.Printf("Getting notification settings for user: %s", req.UserId)

	// Имитация настроек уведомлений
	settings := &notificationv1.NotificationSettings{
		UserId:  req.UserId,
		Enabled: true,
		TypeSettings: map[string]bool{
			"FILE_CREATED":       true,
			"SYNC_COMPLETED":     true,
			"ERROR_OCCURRED":     true,
			"SYSTEM_MAINTENANCE": false,
		},
		PrioritySettings: map[string]bool{
			"LOW":      true,
			"NORMAL":   true,
			"HIGH":     true,
			"CRITICAL": true,
		},
		ChannelSettings: map[string]*notificationv1.ChannelSettings{
			"email": {
				Enabled: true,
				TypeSettings: map[string]bool{
					"FILE_CREATED":   true,
					"SYNC_COMPLETED": true,
				},
				PrioritySettings: map[string]bool{
					"NORMAL": true,
					"HIGH":   true,
				},
				MaxFrequency: 10,
			},
			"push": {
				Enabled: true,
				TypeSettings: map[string]bool{
					"FILE_CREATED":   true,
					"SYNC_COMPLETED": true,
				},
				PrioritySettings: map[string]bool{
					"NORMAL": true,
					"HIGH":   true,
				},
				MaxFrequency: 50,
			},
			"sms": {
				Enabled: false,
			},
		},
		QuietHoursEnabled:     true,
		MaxDailyNotifications: 100,
		GroupSimilar:          true,
		PreferredLanguages:    []string{"en", "ru"},
		UpdatedAt:             timestamppb.Now(),
	}

	return &notificationv1.GetNotificationSettingsResponse{
		Settings: settings,
	}, nil
}

func (s *NotificationService) UpdateNotificationSettings(ctx context.Context, req *notificationv1.UpdateNotificationSettingsRequest) (*notificationv1.UpdateNotificationSettingsResponse, error) {
	log.Printf("Updating notification settings for user: %s", req.Settings.UserId)

	return &notificationv1.UpdateNotificationSettingsResponse{
		Updated:   true,
		Settings:  req.Settings,
		UpdatedAt: timestamppb.Now(),
	}, nil
}

func main() {
	log.Println("Starting Notification Service microservice...")

	// Создаем gRPC сервер
	grpcServer := grpc.NewServer()

	// Регистрируем сервисы
	notificationService := &NotificationService{}
	notificationv1.RegisterNotificationServiceServer(grpcServer, notificationService)

	// Включаем reflection для разработки
	reflection.Register(grpcServer)

	// Настраиваем порт
	port := "50055"
	if envPort := os.Getenv("NOTIFICATION_SERVICE_PORT"); envPort != "" {
		port = envPort
	}

	// Создаем listener
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", port, err)
	}

	log.Printf("Notification Service listening on port %s", port)

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

	log.Println("Shutting down Notification Service...")

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
		log.Println("Notification Service stopped gracefully")
	}
}
