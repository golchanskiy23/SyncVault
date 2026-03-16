package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"syncvault/internal/config"
	"syncvault/internal/events"
)

type SyncServiceWithKafka struct {
	*SyncService
	kafkaConfig *events.KafkaConfig
	producer    events.Producer
	consumer    events.Consumer
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

func NewSyncServiceWithKafka(cfg *config.Config) *SyncServiceWithKafka {
	ctx, cancel := context.WithCancel(context.Background())

	kafkaConfig := events.NewKafkaConfig(cfg)
	producer := events.NewKafkaProducer(kafkaConfig)
	consumer := events.NewKafkaConsumer(kafkaConfig, cfg.Kafka.FileEventsTopic)

	return &SyncServiceWithKafka{
		SyncService: NewSyncService(cfg),
		kafkaConfig: kafkaConfig,
		producer:    producer,
		consumer:    consumer,
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (s *SyncServiceWithKafka) StartKafkaConsumer() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		log.Println("Starting Kafka consumer for file events...")

		handler := func(ctx context.Context, event *events.FileEvent) error {
			return s.handleFileEvent(ctx, event)
		}

		if err := s.consumer.ConsumeFileEvents(s.ctx, handler); err != nil {
			log.Printf("Kafka consumer error: %v", err)
		}
	}()
}

func (s *SyncServiceWithKafka) handleFileEvent(ctx context.Context, event *events.FileEvent) error {
	log.Printf("Processing file event: %s for user %s", event.Type, event.UserID)

	switch event.Type {
	case events.FileCreated:
		return s.handleFileCreated(ctx, event)
	case events.FileUpdated:
		return s.handleFileUpdated(ctx, event)
	case events.FileDeleted:
		return s.handleFileDeleted(ctx, event)
	default:
		log.Printf("Unknown file event type: %s", event.Type)
		return nil
	}
}

func (s *SyncServiceWithKafka) handleFileCreated(ctx context.Context, event *events.FileEvent) error {
	log.Printf("File created: %s (size: %d)", event.FilePath, event.Size)

	syncEvent := &events.SyncEvent{
		ID:         generateSyncEventID(),
		Type:       events.SyncStarted,
		UserID:     event.UserID,
		SourceNode: "file-service",
		TargetNode: "sync-service",
		FilePaths:  []string{event.FilePath},
		Timestamp:  time.Now(),
		Status:     "started",
	}

	if err := s.producer.PublishSyncEvent(ctx, syncEvent); err != nil {
		log.Printf("Failed to publish sync event: %v", err)
		return err
	}

	return s.syncFileToDevice(ctx, event.UserID, event.FilePath)
}

func (s *SyncServiceWithKafka) handleFileUpdated(ctx context.Context, event *events.FileEvent) error {
	log.Printf("File updated: %s (hash: %s)", event.FilePath, event.FileHash)

	syncEvent := &events.SyncEvent{
		ID:         generateSyncEventID(),
		Type:       events.SyncStarted,
		UserID:     event.UserID,
		SourceNode: "file-service",
		TargetNode: "sync-service",
		FilePaths:  []string{event.FilePath},
		Timestamp:  time.Now(),
		Status:     "started",
	}

	if err := s.producer.PublishSyncEvent(ctx, syncEvent); err != nil {
		log.Printf("Failed to publish sync event: %v", err)
		return err
	}

	return s.syncFileToDevice(ctx, event.UserID, event.FilePath)
}

func (s *SyncServiceWithKafka) handleFileDeleted(ctx context.Context, event *events.FileEvent) error {
	log.Printf("File deleted: %s", event.FilePath)

	devices := s.ListUserDevices(event.UserID)
	for _, device := range devices {
		if err := device.DeleteFile(ctx, event.FilePath); err != nil {
			log.Printf("Failed to delete file %s from device %s: %v", event.FilePath, device.DeviceID, err)
		} else {
			log.Printf("Successfully deleted file %s from device %s", event.FilePath, device.DeviceID)
		}
	}

	syncEvent := &events.SyncEvent{
		ID:         generateSyncEventID(),
		Type:       events.SyncCompleted,
		UserID:     event.UserID,
		SourceNode: "file-service",
		TargetNode: "sync-service",
		FilePaths:  []string{event.FilePath},
		Timestamp:  time.Now(),
		Status:     "completed",
	}

	return s.producer.PublishSyncEvent(ctx, syncEvent)
}

func (s *SyncServiceWithKafka) syncFileToDevice(ctx context.Context, userID, filePath string) error {
	devices := s.ListUserDevices(userID)
	var lastError error

	for _, device := range devices {
		if err := s.SyncDevice(ctx, device.DeviceID); err != nil {
			log.Printf("Failed to sync device %s: %v", device.DeviceID, err)
			lastError = err

			conflictEvent := &events.ConflictEvent{
				ID:           generateConflictEventID(),
				Type:         events.ConflictDetected,
				UserID:       userID,
				FilePath:     filePath,
				SourceNode:   "file-service",
				TargetNode:   device.DeviceID,
				ConflictType: "sync_error",
				Timestamp:    time.Now(),
			}

			if pubErr := s.producer.PublishConflictEvent(ctx, conflictEvent); pubErr != nil {
				log.Printf("Failed to publish conflict event: %v", pubErr)
			}
		} else {
			log.Printf("Successfully synced file %s to device %s", filePath, device.DeviceID)
		}
	}

	status := "completed"
	if lastError != nil {
		status = "failed"
	}

	syncEvent := &events.SyncEvent{
		ID:         generateSyncEventID(),
		Type:       events.SyncCompleted,
		UserID:     userID,
		SourceNode: "file-service",
		TargetNode: "sync-service",
		FilePaths:  []string{filePath},
		Timestamp:  time.Now(),
		Status:     status,
	}

	if lastError != nil {
		syncEvent.Error = lastError.Error()
	}

	if err := s.producer.PublishSyncEvent(ctx, syncEvent); err != nil {
		log.Printf("Failed to publish sync completion event: %v", err)
	}

	return lastError
}

func (s *SyncServiceWithKafka) Shutdown() {
	log.Println("Shutting down Kafka integration...")

	s.cancel()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("Kafka consumer stopped gracefully")
	case <-time.After(10 * time.Second):
		log.Println("Kafka consumer shutdown timeout")
	}

	if err := s.consumer.Close(); err != nil {
		log.Printf("Error closing Kafka consumer: %v", err)
	}

	if err := s.producer.Close(); err != nil {
		log.Printf("Error closing Kafka producer: %v", err)
	}
}

func generateSyncEventID() string {
	return fmt.Sprintf("sync_%d", time.Now().UnixNano())
}

func generateConflictEventID() string {
	return fmt.Sprintf("conflict_%d", time.Now().UnixNano())
}
