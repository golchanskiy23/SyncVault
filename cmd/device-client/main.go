package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/durationpb"

	commonv1 "syncvault/internal/grpc/proto/common"
	syncv1 "syncvault/internal/grpc/proto/sync"
)

// DeviceClient клиент для синхронизации устройств
type DeviceClient struct {
	conn   *grpc.ClientConn
	client syncv1.SyncServiceClient
}

func NewDeviceClient(address string) (*DeviceClient, error) {
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	client := syncv1.NewSyncServiceClient(conn)

	return &DeviceClient{
		conn:   conn,
		client: client,
	}, nil
}

func (dc *DeviceClient) Close() error {
	return dc.conn.Close()
}

// StartSyncSession начинает сессию синхронизации
func (dc *DeviceClient) StartSyncSession(deviceID, storagePath string) error {
	log.Printf("Starting sync session for device: %s", deviceID)

	// Создаем стрим для синхронизации
	stream, err := dc.client.StartSync(context.Background())
	if err != nil {
		return fmt.Errorf("failed to start sync stream: %w", err)
	}

	// Отправляем начальный запрос
	startReq := &syncv1.SyncRequest{
		Request: &syncv1.SyncRequest_Start{
			Start: &syncv1.SyncStart{
				NodeId:   deviceID,
				Since:    nil, // Начинаем с текущего времени
				FileIds:  []string{},
				Priority: commonv1.Priority_PRIORITY_NORMAL,
				Mode:     syncv1.SyncMode_SYNC_MODE_INCREMENTAL,
				Timeout:  &durationpb.Duration{Seconds: 300},
				Options: map[string]string{
					"storage_path": storagePath,
				},
			},
		},
	}

	if err := stream.Send(startReq); err != nil {
		return fmt.Errorf("failed to send start request: %w", err)
	}

	// Получаем ответ
	resp, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("failed to receive response: %w", err)
	}

	if statusResp := resp.GetStatus(); statusResp != nil {
		log.Printf("Sync session started. Session ID: %s", statusResp.SessionId)
		log.Printf("Device state: %s", statusResp.State.String())
	}

	// Запускаем пинг для проверки соединения
	go dc.pingLoop(stream, deviceID)

	// Основной цикл обработки событий
	return dc.handleSyncEvents(stream, deviceID)
}

// pingLoop отправляет периодические пинги
func (dc *DeviceClient) pingLoop(stream syncv1.SyncService_StartSyncClient, deviceID string) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	sequence := int32(1)

	for {
		select {
		case <-ticker.C:
			pingReq := &syncv1.SyncRequest{
				Request: &syncv1.SyncRequest_Ping{
					Ping: &syncv1.SyncPing{
						NodeId:         deviceID,
						Timestamp:      nil, // Будет заполнено сервером
						SequenceNumber: sequence,
					},
				},
			}

			if err := stream.Send(pingReq); err != nil {
				log.Printf("Failed to send ping: %v", err)
				return
			}

			sequence++
		}
	}
}

// handleSyncEvents обрабатывает события синхронизации
func (dc *DeviceClient) handleSyncEvents(stream syncv1.SyncService_StartSyncClient, deviceID string) error {
	for {
		resp, err := stream.Recv()
		if err != nil {
			log.Printf("Stream ended: %v", err)
			return err
		}

		switch resp := resp.Response.(type) {
		case *syncv1.SyncResponse_Status:
			log.Printf("Status update: %s", resp.Status.State.String())
			log.Printf("Pending events: %d", resp.Status.PendingEvents)
			log.Printf("Completed events: %d", resp.Status.CompletedEvents)

		case *syncv1.SyncResponse_Pong:
			log.Printf("Received pong: sequence=%d", resp.Pong.SequenceNumber)

		case *syncv1.SyncResponse_Event:
			log.Printf("Sync event: %s", resp.Event.Type.String())
			log.Printf("File: %s", resp.Event.FileMetadata.FilePath)

		case *syncv1.SyncResponse_Complete:
			log.Printf("Sync completed")
			return nil

		case *syncv1.SyncResponse_Error:
			log.Printf("Sync error: %s", resp.Error.ErrorMessage)
			return fmt.Errorf("sync error: %s", resp.Error.ErrorMessage)

		default:
			log.Printf("Unknown response type: %T", resp)
		}
	}
}

// GetSyncStatus получает статус синхронизации
func (dc *DeviceClient) GetSyncStatus(sessionID string) error {
	req := &syncv1.GetSyncStatusRequest{
		NodeId:    "test-device",
		SessionId: sessionID,
	}

	resp, err := dc.client.GetSyncStatus(context.Background(), req)
	if err != nil {
		return fmt.Errorf("failed to get sync status: %w", err)
	}

	log.Printf("Sync status for session %s:", sessionID)
	for _, status := range resp.Statuses {
		log.Printf("  Device: %s", status.NodeId)
		log.Printf("  State: %s", status.State.String())
		log.Printf("  Pending: %d", status.PendingEvents)
		log.Printf("  Completed: %d", status.CompletedEvents)
		log.Printf("  Success rate: %.2f%%", status.SuccessRate*100)
	}

	if resp.Summary != nil {
		log.Printf("Summary:")
		log.Printf("  Total sessions: %d", resp.Summary.TotalSessions)
		log.Printf("  Active sessions: %d", resp.Summary.ActiveSessions)
		log.Printf("  Total events: %d", resp.Summary.TotalEvents)
		log.Printf("  Failed events: %d", resp.Summary.FailedEvents)
		log.Printf("  Overall success rate: %.2f%%", resp.Summary.OverallSuccessRate*100)
	}

	return nil
}

func main() {
	var (
		syncServiceAddr = flag.String("sync", "localhost:50053", "Sync service address")
		deviceID        = flag.String("device", "test-device", "Device ID")
		storagePath     = flag.String("path", "./sync-data", "Local storage path")
		command         = flag.String("cmd", "sync", "Command: sync, status")
		sessionID       = flag.String("session", "", "Session ID for status")
	)
	flag.Parse()

	// Создаем директорию для хранения если не существует
	if err := os.MkdirAll(*storagePath, 0755); err != nil {
		log.Fatalf("Failed to create storage directory: %v", err)
	}

	// Получаем абсолютный путь
	absPath, err := filepath.Abs(*storagePath)
	if err != nil {
		log.Fatalf("Failed to get absolute path: %v", err)
	}

	log.Printf("Device client starting...")
	log.Printf("Device ID: %s", *deviceID)
	log.Printf("Storage path: %s", absPath)
	log.Printf("Sync service: %s", *syncServiceAddr)

	// Подключаемся к сервису синхронизации
	client, err := NewDeviceClient(*syncServiceAddr)
	if err != nil {
		log.Fatalf("Failed to create device client: %v", err)
	}
	defer client.Close()

	// Выполняем команду
	switch *command {
	case "sync":
		log.Printf("Starting sync session...")
		if err := client.StartSyncSession(*deviceID, absPath); err != nil {
			log.Fatalf("Sync session failed: %v", err)
		}

	case "status":
		if *sessionID == "" {
			log.Fatal("Session ID is required for status command")
		}
		log.Printf("Getting sync status...")
		if err := client.GetSyncStatus(*sessionID); err != nil {
			log.Fatalf("Failed to get sync status: %v", err)
		}

	default:
		log.Fatalf("Unknown command: %s", *command)
	}
}
