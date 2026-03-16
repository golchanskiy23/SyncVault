package server_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	"syncvault/internal/grpc/interceptors"
	commonv1 "syncvault/internal/grpc/proto/common"
	healthv1 "syncvault/internal/grpc/proto/health"
	notificationv1 "syncvault/internal/grpc/proto/notification"
	storagev1 "syncvault/internal/grpc/proto/storage"
	syncv1 "syncvault/internal/grpc/proto/sync"
	grpcsrv "syncvault/internal/grpc/server"
)

const testJWTSecret = "SyncVault@Test#2024!xK9mZ"
const testJWTIssuer = "syncvault"

// testToken генерирует валидный JWT токен для тестов
func testToken(t *testing.T) string {
	t.Helper()

	validator := interceptors.NewJWTValidator(
		testJWTSecret,
		testJWTIssuer,
		1*time.Hour,
		24*time.Hour,
	)

	claims := &interceptors.Claims{
		UserID:   "test-user",
		NodeID:   "test-node",
		Email:    "test@example.com",
		Roles:    []string{"admin", "user"},
		IsActive: true,
	}

	token, err := validator.GenerateToken(claims, 1*time.Hour)
	require.NoError(t, err)
	return token
}

// authCtx возвращает context с Bearer токеном в metadata
func authCtx(t *testing.T) context.Context {
	t.Helper()
	md := metadata.Pairs("authorization", "Bearer "+testToken(t))
	return metadata.NewOutgoingContext(context.Background(), md)
}

// newTestServer создаёт сервер и подключение через bufconn
func newTestServer(t *testing.T) *grpc.ClientConn {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)

	srv := grpcsrv.NewServer(
		grpcsrv.WithHost("localhost"),
		grpcsrv.WithPort(0),
		grpcsrv.WithJWTSecret(testJWTSecret),
		grpcsrv.WithEnableReflection(false),
		grpcsrv.WithEnableTLS(false),
	)

	go func() {
		if err := srv.StartWithListener(lis); err != nil {
			t.Logf("test server stopped: %v", err)
		}
	}()

	conn, err := grpc.DialContext(
		context.Background(),
		"bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		conn.Close()
		lis.Close()
	})

	return conn
}

func TestStorageService(t *testing.T) {
	conn := newTestServer(t)
	client := storagev1.NewStorageServiceClient(conn)

	t.Run("StoreFile", func(t *testing.T) {
		resp, err := client.StoreFile(authCtx(t), &storagev1.StoreFileRequest{
			FilePath:      "/test/file.txt",
			Content:       []byte("test content"),
			Size:          1024,
			StorageNodeId: "test-node",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.FileId)
		assert.Equal(t, "temp-hash", resp.FileHash)
	})

	t.Run("GetFile", func(t *testing.T) {
		resp, err := client.GetFile(authCtx(t), &storagev1.GetFileRequest{
			FilePath: "/test/file.txt",
		})
		require.NoError(t, err)
		assert.NotNil(t, resp.Metadata)
		assert.Equal(t, "/test/file.txt", resp.Metadata.FilePath)
	})

	t.Run("DeleteFile", func(t *testing.T) {
		resp, err := client.DeleteFile(authCtx(t), &storagev1.DeleteFileRequest{
			FilePath: "/test/file.txt",
		})
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("FileExists", func(t *testing.T) {
		resp, err := client.FileExists(authCtx(t), &storagev1.FileExistsRequest{
			FilePath: "/test/file.txt",
		})
		require.NoError(t, err)
		assert.True(t, resp.Exists)
	})
}

func TestSyncService(t *testing.T) {
	conn := newTestServer(t)
	client := syncv1.NewSyncServiceClient(conn)

	t.Run("GetSyncStatus", func(t *testing.T) {
		resp, err := client.GetSyncStatus(authCtx(t), &syncv1.GetSyncStatusRequest{
			SessionId: "test-session",
		})
		require.NoError(t, err)
		assert.NotNil(t, resp.Statuses)
	})

	t.Run("ForceSync", func(t *testing.T) {
		resp, err := client.ForceSync(authCtx(t), &syncv1.ForceSyncRequest{
			SourceNodeId: "test-node-1",
			TargetNodeId: "test-node-2",
			Priority:     commonv1.Priority_PRIORITY_HIGH,
		})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.SessionId)
		assert.NotNil(t, resp.Status)
	})
}

func TestNotificationService(t *testing.T) {
	conn := newTestServer(t)
	client := notificationv1.NewNotificationServiceClient(conn)

	t.Run("Subscribe", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(authCtx(t), 3*time.Second)
		defer cancel()

		stream, err := client.Subscribe(ctx, &notificationv1.SubscribeRequest{
			UserId: "test-user",
			Types: []notificationv1.NotificationType{
				notificationv1.NotificationType_NOTIFICATION_TYPE_FILE_CREATED,
				notificationv1.NotificationType_NOTIFICATION_TYPE_SYNC_COMPLETED,
			},
		})
		require.NoError(t, err)

		// Читаем первый ответ (subscription status)
		msg, err := stream.Recv()
		require.NoError(t, err)
		assert.NotNil(t, msg)
	})

	t.Run("SendNotification", func(t *testing.T) {
		resp, err := client.SendNotification(authCtx(t), &notificationv1.SendNotificationRequest{
			UserId:   "test-user",
			Type:     notificationv1.NotificationType_NOTIFICATION_TYPE_SYSTEM_MAINTENANCE,
			Title:    "Test Notification",
			Message:  "This is a test notification",
			Priority: notificationv1.NotificationPriority_NOTIFICATION_PRIORITY_NORMAL,
		})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.NotificationId)
		assert.True(t, resp.Sent)
	})
}

func TestHealthService(t *testing.T) {
	conn := newTestServer(t)
	client := healthv1.NewHealthServiceClient(conn)

	t.Run("Check", func(t *testing.T) {
		// Check — публичный метод, токен не нужен
		resp, err := client.Check(context.Background(), &healthv1.HealthCheckRequest{
			Service: "test-service",
		})
		require.NoError(t, err)
		assert.Equal(t, healthv1.HealthCheckResponse_SERVING, resp.Status)
	})

	t.Run("DetailedHealth", func(t *testing.T) {
		resp, err := client.DetailedHealth(authCtx(t), &healthv1.DetailedHealthRequest{
			Service:             "test-service",
			IncludeDependencies: true,
			IncludeMetrics:      true,
		})
		require.NoError(t, err)
		assert.Equal(t, "test-service", resp.Service)
		assert.NotNil(t, resp.Checks)
	})
}
