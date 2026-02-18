package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

type Consumer interface {
	ConsumeFileEvents(ctx context.Context, handler FileEventHandler) error
	ConsumeSyncEvents(ctx context.Context, handler SyncEventHandler) error
	ConsumeConflictEvents(ctx context.Context, handler ConflictEventHandler) error
	Close() error
}

type FileEventHandler func(ctx context.Context, event *FileEvent) error
type SyncEventHandler func(ctx context.Context, event *SyncEvent) error
type ConflictEventHandler func(ctx context.Context, event *ConflictEvent) error

type KafkaConsumer struct {
	reader      *kafka.Reader
	config      *KafkaConfig
	dlqProducer *KafkaProducer // Bug 1.5 fix: single shared producer instead of per-message creation
}

func NewKafkaConsumer(config *KafkaConfig, topic string) *KafkaConsumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        config.Brokers,
		GroupID:        config.GroupID,
		Topic:          topic,
		MinBytes:       10e3,
		MaxBytes:       10e6,
		CommitInterval: time.Second,
		StartOffset:    kafka.LastOffset,
		ReadBackoffMin: 100 * time.Millisecond,
		ReadBackoffMax: 1 * time.Second,
		MaxWait:        500 * time.Millisecond,
	})

	return &KafkaConsumer{
		reader:      reader,
		config:      config,
		dlqProducer: NewKafkaProducer(config), // Bug 1.5 fix: initialize once
	}
}

func (c *KafkaConsumer) ConsumeFileEvents(ctx context.Context, handler FileEventHandler) error {
	return c.consume(ctx, handler, c.handleFileEvent)
}

func (c *KafkaConsumer) ConsumeSyncEvents(ctx context.Context, handler SyncEventHandler) error {
	return c.consume(ctx, handler, c.handleSyncEvent)
}

func (c *KafkaConsumer) ConsumeConflictEvents(ctx context.Context, handler ConflictEventHandler) error {
	return c.consume(ctx, handler, c.handleConflictEvent)
}

func (c *KafkaConsumer) consume(ctx context.Context, handler interface{}, processFunc func(context.Context, interface{}, []byte) error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msg, err := c.reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("Error reading message: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		if err := processFunc(ctx, handler, msg.Value); err != nil {
			log.Printf("Error processing message: %v", err)
			// Bug 1.5 fix: use shared dlqProducer instead of creating a new one per message
			// Bug 1.12 fix: publishToDLQ has its own scoped context with defer cancel()
			c.publishToDLQ(msg, err)
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			log.Printf("Error committing message: %v", err)
		}
	}
}

// publishToDLQ publishes a failed message to the dead-letter queue.
// Bug 1.12 fix: context.WithTimeout + defer cancel() are scoped to this function,
// not deferred inside the consume loop (which caused N contexts to accumulate).
func (c *KafkaConsumer) publishToDLQ(msg kafka.Message, processingErr error) {
	dlqEvent := &DLQEvent{
		ID:            fmt.Sprintf("dlq-%d", time.Now().UnixNano()),
		OriginalTopic: c.reader.Config().Topic,
		Event:         msg.Value,
		Error:         processingErr.Error(),
		Timestamp:     time.Now(),
		RetryCount:    0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.config.Timeout)
	defer cancel()

	if pubErr := c.dlqProducer.PublishDLQEvent(ctx, dlqEvent); pubErr != nil {
		log.Printf("Failed to publish to DLQ: %v", pubErr)
	}
}

func (c *KafkaConsumer) handleFileEvent(ctx context.Context, handler interface{}, data []byte) error {
	var event FileEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("failed to unmarshal file event: %w", err)
	}

	if h, ok := handler.(FileEventHandler); ok {
		return h(ctx, &event)
	}
	return fmt.Errorf("invalid handler type for file event")
}

func (c *KafkaConsumer) handleSyncEvent(ctx context.Context, handler interface{}, data []byte) error {
	var event SyncEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("failed to unmarshal sync event: %w", err)
	}

	if h, ok := handler.(SyncEventHandler); ok {
		return h(ctx, &event)
	}
	return fmt.Errorf("invalid handler type for sync event")
}

func (c *KafkaConsumer) handleConflictEvent(ctx context.Context, handler interface{}, data []byte) error {
	var event ConflictEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("failed to unmarshal conflict event: %w", err)
	}

	if h, ok := handler.(ConflictEventHandler); ok {
		return h(ctx, &event)
	}
	return fmt.Errorf("invalid handler type for conflict event")
}

func (c *KafkaConsumer) Close() error {
	// Bug 1.5 fix: close the shared DLQ producer on consumer shutdown
	if err := c.dlqProducer.Close(); err != nil {
		log.Printf("Error closing DLQ producer: %v", err)
	}
	return c.reader.Close()
}
