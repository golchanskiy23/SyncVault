package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

type FileEventType string

const (
	FileCreated FileEventType = "FileCreated"
	FileUpdated FileEventType = "FileUpdated"
	FileDeleted FileEventType = "FileDeleted"
)

type FileEvent struct {
	ID        string        `json:"id"`
	Type      FileEventType `json:"type"`
	UserID    string        `json:"userId"`
	FilePath  string        `json:"filePath"`
	FileHash  string        `json:"fileHash"`
	Size      int64         `json:"size"`
	Timestamp time.Time     `json:"timestamp"`
	Metadata  interface{}   `json:"metadata,omitempty"`
}

type Producer interface {
	PublishFileEvent(ctx context.Context, event *FileEvent) error
	PublishSyncEvent(ctx context.Context, event *SyncEvent) error
	PublishConflictEvent(ctx context.Context, event *ConflictEvent) error
	PublishDLQEvent(ctx context.Context, event *DLQEvent) error
	Close() error
}

type KafkaProducer struct {
	writer *kafka.Writer
	config *KafkaConfig
}

type KafkaConfig struct {
	Brokers         []string
	GroupID         string
	FileEventsTopic string
	SyncEventsTopic string
	ConflictTopic   string
	DLQTopic        string
	Timeout         time.Duration
	MaxRetries      int
	RetryBackoff    time.Duration
}

func NewKafkaProducer(config *KafkaConfig) *KafkaProducer {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(config.Brokers...),
		Balancer:     &kafka.LeastBytes{},
		BatchSize:    100,
		BatchTimeout: 1 * time.Second,
		RequiredAcks: kafka.RequireOne,
		Async:        false,
	}

	return &KafkaProducer{
		writer: writer,
		config: config,
	}
}

func (p *KafkaProducer) PublishFileEvent(ctx context.Context, event *FileEvent) error {
	return p.publishWithRetry(ctx, p.config.FileEventsTopic, event)
}

func (p *KafkaProducer) PublishSyncEvent(ctx context.Context, event *SyncEvent) error {
	return p.publishWithRetry(ctx, p.config.SyncEventsTopic, event)
}

func (p *KafkaProducer) PublishConflictEvent(ctx context.Context, event *ConflictEvent) error {
	return p.publishWithRetry(ctx, p.config.ConflictTopic, event)
}

func (p *KafkaProducer) PublishDLQEvent(ctx context.Context, event *DLQEvent) error {
	return p.publishWithRetry(ctx, p.config.DLQTopic, event)
}

func (p *KafkaProducer) publishWithRetry(ctx context.Context, topic string, event interface{}) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Bug 1.1 fix: safe type assertion with fallback instead of direct assertion
	var eventID string
	switch e := event.(type) {
	case interface{ GetID() string }:
		eventID = e.GetID()
	default:
		eventID = fmt.Sprintf("unknown-%d", time.Now().UnixNano())
	}

	var lastErr error
	for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(p.config.RetryBackoff):
			}
		}

		var key string
		if getter, ok := event.(interface{ GetID() string }); ok {
			key = getter.GetID()
		}

		msg := kafka.Message{
			Topic: topic,
			Key:   []byte(key),
			Value: data,
			Time:  time.Now(),
		}

		// Bug 1.1 fix: call cancel() explicitly after WriteMessages, not via defer inside loop
		timeoutCtx, cancel := context.WithTimeout(ctx, p.config.Timeout)
		err = p.writer.WriteMessages(timeoutCtx, msg)
		cancel()

		if err == nil {
			return nil
		}

		lastErr = err

		if attempt == p.config.MaxRetries {
			dlqEvent := &DLQEvent{
				ID:            fmt.Sprintf("dlq-%d", time.Now().UnixNano()),
				OriginalTopic: topic,
				Event:         data,
				Error:         err.Error(),
				Timestamp:     time.Now(),
				RetryCount:    attempt,
			}
			p.PublishDLQEvent(ctx, dlqEvent)
		}
	}

	return fmt.Errorf("failed to publish after %d attempts: %w", p.config.MaxRetries+1, lastErr)
}

func (p *KafkaProducer) Close() error {
	return p.writer.Close()
}

func (e *FileEvent) GetID() string {
	return e.ID
}

type SyncEventType string

const (
	SyncStarted   SyncEventType = "SyncStarted"
	SyncCompleted SyncEventType = "SyncCompleted"
	SyncFailed    SyncEventType = "SyncFailed"
)

type SyncEvent struct {
	ID            string        `json:"id"`
	Type          SyncEventType `json:"type"`
	UserID        string        `json:"userId"`
	SourceNode    string        `json:"sourceNode"`
	TargetNode    string        `json:"targetNode"`
	FilePaths     []string      `json:"filePaths"`
	Timestamp     time.Time     `json:"timestamp"`
	Status        string        `json:"status"`
	OperationType string        `json:"operationType,omitempty"`
	Error         string        `json:"error,omitempty"`
}

func (e *SyncEvent) GetID() string {
	return e.ID
}

type ConflictEventType string

const (
	ConflictDetected ConflictEventType = "ConflictDetected"
	ConflictResolved ConflictEventType = "ConflictResolved"
)

type ConflictEvent struct {
	ID           string            `json:"id"`
	Type         ConflictEventType `json:"type"`
	UserID       string            `json:"userId"`
	FilePath     string            `json:"filePath"`
	SourceNode   string            `json:"sourceNode"`
	TargetNode   string            `json:"targetNode"`
	ConflictType string            `json:"conflictType"`
	Timestamp    time.Time         `json:"timestamp"`
	Resolution   string            `json:"resolution,omitempty"`
}

func (e *ConflictEvent) GetID() string {
	return e.ID
}

type DLQEvent struct {
	ID            string    `json:"id"`
	OriginalTopic string    `json:"originalTopic"`
	Event         []byte    `json:"event"`
	Error         string    `json:"error"`
	Timestamp     time.Time `json:"timestamp"`
	RetryCount    int       `json:"retryCount"`
}

func (e *DLQEvent) GetID() string {
	return e.ID
}
