package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

type DLQHandler struct {
	producer Producer
	config   *KafkaConfig
}

func NewDLQHandler(producer Producer, config *KafkaConfig) *DLQHandler {
	return &DLQHandler{
		producer: producer,
		config:   config,
	}
}

func (h *DLQHandler) ProcessDLQEvents(ctx context.Context, handler DLQEventHandler) error {
	consumer := NewKafkaConsumer(h.config, h.config.DLQTopic)
	defer consumer.Close()

	return consumer.ConsumeDLQEvents(ctx, handler)
}

type DLQEventHandler func(ctx context.Context, event *DLQEvent) error

func (c *KafkaConsumer) ConsumeDLQEvents(ctx context.Context, handler DLQEventHandler) error {
	retryManager := NewRetryManager(nil, c.config)

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
			log.Printf("Error reading DLQ message: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		var dlqEvent DLQEvent
		if err := json.Unmarshal(msg.Value, &dlqEvent); err != nil {
			log.Printf("Failed to unmarshal DLQ event: %v", err)
			continue
		}

		if dlqEvent.RetryCount < c.config.MaxRetries {
			if err := retryManager.retryEventFromDLQ(ctx, &dlqEvent); err != nil {
				log.Printf("Failed to retry DLQ event %s: %v", dlqEvent.ID, err)
			}
		} else {
			if err := handler(ctx, &dlqEvent); err != nil {
				log.Printf("DLQ handler failed for event %s: %v", dlqEvent.ID, err)
			}
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			log.Printf("Error committing DLQ message: %v", err)
		}
	}
}

func (h *DLQHandler) retryEvent(ctx context.Context, dlqEvent *DLQEvent) error {
	time.Sleep(h.config.RetryBackoff)

	dlqEvent.RetryCount++
	dlqEvent.Timestamp = time.Now()

	switch dlqEvent.OriginalTopic {
	case h.config.FileEventsTopic:
		var fileEvent FileEvent
		if err := json.Unmarshal(dlqEvent.Event, &fileEvent); err != nil {
			return fmt.Errorf("failed to unmarshal file event for retry: %w", err)
		}
		return h.producer.PublishFileEvent(ctx, &fileEvent)

	case h.config.SyncEventsTopic:
		var syncEvent SyncEvent
		if err := json.Unmarshal(dlqEvent.Event, &syncEvent); err != nil {
			return fmt.Errorf("failed to unmarshal sync event for retry: %w", err)
		}
		return h.producer.PublishSyncEvent(ctx, &syncEvent)

	case h.config.ConflictTopic:
		var conflictEvent ConflictEvent
		if err := json.Unmarshal(dlqEvent.Event, &conflictEvent); err != nil {
			return fmt.Errorf("failed to unmarshal conflict event for retry: %w", err)
		}
		return h.producer.PublishConflictEvent(ctx, &conflictEvent)

	default:
		return fmt.Errorf("unknown original topic: %s", dlqEvent.OriginalTopic)
	}
}

type RetryManager struct {
	producer Producer
	config   *KafkaConfig
}

func NewRetryManager(producer Producer, config *KafkaConfig) *RetryManager {
	return &RetryManager{
		producer: producer,
		config:   config,
	}
}

func (rm *RetryManager) retryEventFromDLQ(ctx context.Context, dlqEvent *DLQEvent) error {
	time.Sleep(rm.config.RetryBackoff)

	dlqEvent.RetryCount++
	dlqEvent.Timestamp = time.Now()

	switch dlqEvent.OriginalTopic {
	case rm.config.FileEventsTopic:
		var fileEvent FileEvent
		if err := json.Unmarshal(dlqEvent.Event, &fileEvent); err != nil {
			return fmt.Errorf("failed to unmarshal file event for retry: %w", err)
		}
		return rm.producer.PublishFileEvent(ctx, &fileEvent)

	case rm.config.SyncEventsTopic:
		var syncEvent SyncEvent
		if err := json.Unmarshal(dlqEvent.Event, &syncEvent); err != nil {
			return fmt.Errorf("failed to unmarshal sync event for retry: %w", err)
		}
		return rm.producer.PublishSyncEvent(ctx, &syncEvent)

	case rm.config.ConflictTopic:
		var conflictEvent ConflictEvent
		if err := json.Unmarshal(dlqEvent.Event, &conflictEvent); err != nil {
			return fmt.Errorf("failed to unmarshal conflict event for retry: %w", err)
		}
		return rm.producer.PublishConflictEvent(ctx, &conflictEvent)

	default:
		return fmt.Errorf("unknown original topic: %s", dlqEvent.OriginalTopic)
	}
}

func (rm *RetryManager) PublishWithRetry(ctx context.Context, topic string, event interface{}) error {
	var lastErr error

	for attempt := 0; attempt <= rm.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(rm.config.RetryBackoff):
			}
		}

		var err error
		switch topic {
		case rm.config.FileEventsTopic:
			if e, ok := event.(*FileEvent); ok {
				err = rm.producer.PublishFileEvent(ctx, e)
			}
		case rm.config.SyncEventsTopic:
			if e, ok := event.(*SyncEvent); ok {
				err = rm.producer.PublishSyncEvent(ctx, e)
			}
		case rm.config.ConflictTopic:
			if e, ok := event.(*ConflictEvent); ok {
				err = rm.producer.PublishConflictEvent(ctx, e)
			}
		default:
			err = fmt.Errorf("unknown topic: %s", topic)
		}

		if err == nil {
			return nil
		}

		lastErr = err
		log.Printf("Attempt %d failed for topic %s: %v", attempt+1, topic, err)
	}

	eventData, _ := json.Marshal(event)
	dlqEvent := &DLQEvent{
		ID:            fmt.Sprintf("dlq-%d", time.Now().UnixNano()),
		OriginalTopic: topic,
		Event:         eventData,
		Error:         lastErr.Error(),
		Timestamp:     time.Now(),
		RetryCount:    rm.config.MaxRetries,
	}

	if dlqErr := rm.producer.PublishDLQEvent(ctx, dlqEvent); dlqErr != nil {
		log.Printf("Failed to publish to DLQ after retries: %v", dlqErr)
	}

	return fmt.Errorf("failed after %d attempts: %w", rm.config.MaxRetries+1, lastErr)
}
