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
	reader *kafka.Reader
	config *KafkaConfig
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
		reader: reader,
		config: config,
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

			dlqEvent := &DLQEvent{
				ID:            fmt.Sprintf("dlq-%d", time.Now().UnixNano()),
				OriginalTopic: c.reader.Config().Topic,
				Event:         msg.Value,
				Error:         err.Error(),
				Timestamp:     time.Now(),
				RetryCount:    0,
			}

			producer := NewKafkaProducer(c.config)
			defer producer.Close()

			ctx, cancel := context.WithTimeout(ctx, c.config.Timeout)
			defer cancel()

			if pubErr := producer.PublishDLQEvent(ctx, dlqEvent); pubErr != nil {
				log.Printf("Failed to publish to DLQ: %v", pubErr)
			}
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			log.Printf("Error committing message: %v", err)
		}
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
	return c.reader.Close()
}
