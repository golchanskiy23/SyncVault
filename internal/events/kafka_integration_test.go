package events

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/kafka"
)

func TestKafkaIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	kafkaContainer, err := kafka.RunContainer(ctx,
		testcontainers.WithImage("confluentinc/cp-kafka:7.4.0"),
		kafka.WithClusterID("test-cluster"),
	)
	require.NoError(t, err)
	defer func() {
		if err := kafkaContainer.Terminate(ctx); err != nil {
			t.Fatalf("Failed to terminate Kafka container: %v", err)
		}
	}()

	brokers, err := kafkaContainer.Brokers(ctx)
	require.NoError(t, err)
	require.Len(t, brokers, 1)

	config := &KafkaConfig{
		Brokers:         brokers,
		GroupID:         "test-group",
		FileEventsTopic: "test-file-events",
		SyncEventsTopic: "test-sync-events",
		ConflictTopic:   "test-conflict-events",
		DLQTopic:        "test-dlq",
		Timeout:         10 * time.Second,
		MaxRetries:      3,
		RetryBackoff:    100 * time.Millisecond,
	}

	t.Run("ProducerConsumer", func(t *testing.T) {
		producer := NewKafkaProducer(config)
		defer producer.Close()

		consumer := NewKafkaConsumer(config, config.FileEventsTopic)
		defer consumer.Close()

		event := &FileEvent{
			ID:        "test-file-1",
			Type:      FileCreated,
			UserID:    "user123",
			FilePath:  "/test/file.txt",
			FileHash:  "abc123",
			Size:      1024,
			Timestamp: time.Now(),
		}

		err := producer.PublishFileEvent(ctx, event)
		require.NoError(t, err)

		receivedEvent := make(chan *FileEvent, 1)
		handler := func(ctx context.Context, e *FileEvent) error {
			receivedEvent <- e
			return nil
		}

		go func() {
			if err := consumer.ConsumeFileEvents(ctx, handler); err != nil {
				t.Errorf("Consumer error: %v", err)
			}
		}()

		select {
		case received := <-receivedEvent:
			if received.ID != event.ID {
				t.Errorf("Expected ID %s, got %s", event.ID, received.ID)
			}
			if received.Type != event.Type {
				t.Errorf("Expected type %s, got %s", event.Type, received.Type)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for event")
		}
	})

	t.Run("DLQRetry", func(t *testing.T) {
		producer := NewKafkaProducer(config)
		defer producer.Close()

		retryManager := NewRetryManager(producer, config)

		event := &FileEvent{
			ID:        "test-file-2",
			Type:      FileUpdated,
			UserID:    "user456",
			FilePath:  "/test/file2.txt",
			FileHash:  "def456",
			Size:      2048,
			Timestamp: time.Now(),
		}

		err := retryManager.PublishWithRetry(ctx, config.FileEventsTopic, event)
		require.NoError(t, err)

		dlqConsumer := NewKafkaConsumer(config, config.DLQTopic)
		defer dlqConsumer.Close()

		dlqEvent := make(chan *DLQEvent, 1)
		dlqHandler := func(ctx context.Context, e *DLQEvent) error {
			dlqEvent <- e
			return nil
		}

		go func() {
			if err := dlqConsumer.ConsumeDLQEvents(ctx, dlqHandler); err != nil {
				t.Errorf("DLQ consumer error: %v", err)
			}
		}()

		select {
		case received := <-dlqEvent:
			if received.OriginalTopic != config.FileEventsTopic {
				t.Errorf("Expected original topic %s, got %s", config.FileEventsTopic, received.OriginalTopic)
			}
			if received.RetryCount != config.MaxRetries {
				t.Errorf("Expected retry count %d, got %d", config.MaxRetries, received.RetryCount)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for DLQ event")
		}
	})

	t.Run("SyncEvents", func(t *testing.T) {
		producer := NewKafkaProducer(config)
		defer producer.Close()

		consumer := NewKafkaConsumer(config, config.SyncEventsTopic)
		defer consumer.Close()

		event := &SyncEvent{
			ID:         "test-sync-1",
			Type:       SyncStarted,
			UserID:     "user789",
			SourceNode: "node1",
			TargetNode: "node2",
			FilePaths:  []string{"/test/sync.txt"},
			Timestamp:  time.Now(),
			Status:     "started",
		}

		err := producer.PublishSyncEvent(ctx, event)
		require.NoError(t, err)

		receivedEvent := make(chan *SyncEvent, 1)
		handler := func(ctx context.Context, e *SyncEvent) error {
			receivedEvent <- e
			return nil
		}

		go func() {
			if err := consumer.ConsumeSyncEvents(ctx, handler); err != nil {
				t.Errorf("Sync consumer error: %v", err)
			}
		}()

		select {
		case received := <-receivedEvent:
			if received.ID != event.ID {
				t.Errorf("Expected ID %s, got %s", event.ID, received.ID)
			}
			if received.Type != event.Type {
				t.Errorf("Expected type %s, got %s", event.Type, received.Type)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for sync event")
		}
	})

	t.Run("ConflictEvents", func(t *testing.T) {
		producer := NewKafkaProducer(config)
		defer producer.Close()

		consumer := NewKafkaConsumer(config, config.ConflictTopic)
		defer consumer.Close()

		event := &ConflictEvent{
			ID:           "test-conflict-1",
			Type:         ConflictDetected,
			UserID:       "user101",
			FilePath:     "/test/conflict.txt",
			SourceNode:   "node1",
			TargetNode:   "node2",
			ConflictType: "version",
			Timestamp:    time.Now(),
		}

		err := producer.PublishConflictEvent(ctx, event)
		require.NoError(t, err)

		receivedEvent := make(chan *ConflictEvent, 1)
		handler := func(ctx context.Context, e *ConflictEvent) error {
			receivedEvent <- e
			return nil
		}

		go func() {
			if err := consumer.ConsumeConflictEvents(ctx, handler); err != nil {
				t.Errorf("Conflict consumer error: %v", err)
			}
		}()

		select {
		case received := <-receivedEvent:
			if received.ID != event.ID {
				t.Errorf("Expected ID %s, got %s", event.ID, received.ID)
			}
			if received.Type != event.Type {
				t.Errorf("Expected type %s, got %s", event.Type, received.Type)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for conflict event")
		}
	})
}
