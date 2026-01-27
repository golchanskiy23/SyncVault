package events

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockProducer implements Producer interface for testing
type MockProducer struct {
	events map[string][][]byte
}

func NewMockProducer() *MockProducer {
	return &MockProducer{
		events: make(map[string][][]byte),
	}
}

func (m *MockProducer) PublishFileEvent(ctx context.Context, event *FileEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	m.events["file.events"] = append(m.events["file.events"], data)
	return nil
}

func (m *MockProducer) PublishSyncEvent(ctx context.Context, event *SyncEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	m.events["sync.events"] = append(m.events["sync.events"], data)
	return nil
}

func (m *MockProducer) PublishConflictEvent(ctx context.Context, event *ConflictEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	m.events["conflict.events"] = append(m.events["conflict.events"], data)
	return nil
}

func (m *MockProducer) PublishDLQEvent(ctx context.Context, event *DLQEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	m.events["dlq.file.events"] = append(m.events["dlq.file.events"], data)
	return nil
}

func (m *MockProducer) Close() error {
	return nil
}

func (m *MockProducer) GetEvents(topic string) [][]byte {
	return m.events[topic]
}

func (m *MockProducer) ClearEvents() {
	m.events = make(map[string][][]byte)
}

// Test lightweight producer functionality without Kafka
func TestMockProducerFileEvents(t *testing.T) {
	producer := NewMockProducer()
	defer producer.Close()

	ctx := context.Background()

	// Test FileCreated event
	fileEvent := &FileEvent{
		ID:        "test-file-1",
		Type:      FileCreated,
		UserID:    "user123",
		FilePath:  "/test/file.txt",
		FileHash:  "abc123",
		Size:      1024,
		Timestamp: time.Now(),
		Metadata:  map[string]interface{}{"test": true},
	}

	err := producer.PublishFileEvent(ctx, fileEvent)
	require.NoError(t, err)

	events := producer.GetEvents("file.events")
	require.Len(t, events, 1)

	var received FileEvent
	err = json.Unmarshal(events[0], &received)
	require.NoError(t, err)

	assert.Equal(t, fileEvent.ID, received.ID)
	assert.Equal(t, fileEvent.Type, received.Type)
	assert.Equal(t, fileEvent.UserID, received.UserID)
	assert.Equal(t, fileEvent.FilePath, received.FilePath)
	assert.Equal(t, fileEvent.FileHash, received.FileHash)
	assert.Equal(t, fileEvent.Size, received.Size)
}

func TestMockProducerSyncEvents(t *testing.T) {
	producer := NewMockProducer()
	defer producer.Close()

	ctx := context.Background()

	syncEvent := &SyncEvent{
		ID:         "test-sync-1",
		Type:       SyncStarted,
		UserID:     "user123",
		SourceNode: "node1",
		TargetNode: "node2",
		FilePaths:  []string{"/test/file1.txt", "/test/file2.txt"},
		Timestamp:  time.Now(),
		Status:     "started",
	}

	err := producer.PublishSyncEvent(ctx, syncEvent)
	require.NoError(t, err)

	events := producer.GetEvents("sync.events")
	require.Len(t, events, 1)

	var received SyncEvent
	err = json.Unmarshal(events[0], &received)
	require.NoError(t, err)

	assert.Equal(t, syncEvent.ID, received.ID)
	assert.Equal(t, syncEvent.Type, received.Type)
	assert.Equal(t, syncEvent.UserID, received.UserID)
	assert.Equal(t, syncEvent.SourceNode, received.SourceNode)
	assert.Equal(t, syncEvent.TargetNode, received.TargetNode)
	assert.Equal(t, syncEvent.FilePaths, received.FilePaths)
	assert.Equal(t, syncEvent.Status, received.Status)
}

func TestMockProducerConflictEvents(t *testing.T) {
	producer := NewMockProducer()
	defer producer.Close()

	ctx := context.Background()

	conflictEvent := &ConflictEvent{
		ID:           "test-conflict-1",
		Type:         ConflictDetected,
		UserID:       "user123",
		FilePath:     "/test/conflict.txt",
		SourceNode:   "node1",
		TargetNode:   "node2",
		ConflictType: "version",
		Timestamp:    time.Now(),
	}

	err := producer.PublishConflictEvent(ctx, conflictEvent)
	require.NoError(t, err)

	events := producer.GetEvents("conflict.events")
	require.Len(t, events, 1)

	var received ConflictEvent
	err = json.Unmarshal(events[0], &received)
	require.NoError(t, err)

	assert.Equal(t, conflictEvent.ID, received.ID)
	assert.Equal(t, conflictEvent.Type, received.Type)
	assert.Equal(t, conflictEvent.UserID, received.UserID)
	assert.Equal(t, conflictEvent.FilePath, received.FilePath)
	assert.Equal(t, conflictEvent.SourceNode, received.SourceNode)
	assert.Equal(t, conflictEvent.TargetNode, received.TargetNode)
	assert.Equal(t, conflictEvent.ConflictType, received.ConflictType)
}

func TestMockProducerDLQEvents(t *testing.T) {
	producer := NewMockProducer()
	defer producer.Close()

	ctx := context.Background()

	dlqEvent := &DLQEvent{
		ID:            "test-dlq-1",
		OriginalTopic: "file.events",
		Event:         []byte(`{"id":"test","type":"FileCreated"}`),
		Error:         "processing failed",
		Timestamp:     time.Now(),
		RetryCount:    3,
	}

	err := producer.PublishDLQEvent(ctx, dlqEvent)
	require.NoError(t, err)

	events := producer.GetEvents("dlq.file.events")
	require.Len(t, events, 1)

	var received DLQEvent
	err = json.Unmarshal(events[0], &received)
	require.NoError(t, err)

	assert.Equal(t, dlqEvent.ID, received.ID)
	assert.Equal(t, dlqEvent.OriginalTopic, received.OriginalTopic)
	assert.Equal(t, dlqEvent.Event, received.Event)
	assert.Equal(t, dlqEvent.Error, received.Error)
	assert.Equal(t, dlqEvent.RetryCount, received.RetryCount)
}

func TestEventFlowWithMockProducer(t *testing.T) {
	producer := NewMockProducer()
	defer producer.Close()

	ctx := context.Background()

	// Simulate complete event flow
	fileEvent := &FileEvent{
		ID:        "test-flow-1",
		Type:      FileCreated,
		UserID:    "user123",
		FilePath:  "/test/flow.txt",
		FileHash:  "flow123",
		Size:      1024,
		Timestamp: time.Now(),
	}

	// Publish file event
	err := producer.PublishFileEvent(ctx, fileEvent)
	require.NoError(t, err)

	// Create sync event in response
	syncEvent := &SyncEvent{
		ID:         "sync-" + fileEvent.ID,
		Type:       SyncStarted,
		UserID:     fileEvent.UserID,
		SourceNode: "file-service",
		TargetNode: "sync-service",
		FilePaths:  []string{fileEvent.FilePath},
		Timestamp:  time.Now(),
		Status:     "initiated",
	}

	err = producer.PublishSyncEvent(ctx, syncEvent)
	require.NoError(t, err)

	// Create conflict event if needed
	conflictEvent := &ConflictEvent{
		ID:           "conflict-" + fileEvent.ID,
		Type:         ConflictDetected,
		UserID:       fileEvent.UserID,
		FilePath:     fileEvent.FilePath,
		SourceNode:   "node1",
		TargetNode:   "node2",
		ConflictType: "version",
		Timestamp:    time.Now(),
	}

	err = producer.PublishConflictEvent(ctx, conflictEvent)
	require.NoError(t, err)

	// Verify all events were published
	fileEvents := producer.GetEvents("file.events")
	syncEvents := producer.GetEvents("sync.events")
	conflictEvents := producer.GetEvents("conflict.events")

	assert.Len(t, fileEvents, 1)
	assert.Len(t, syncEvents, 1)
	assert.Len(t, conflictEvents, 1)
}

func TestRetryManagerWithMockProducer(t *testing.T) {
	producer := NewMockProducer()
	defer producer.Close()

	config := &KafkaConfig{
		Brokers:         []string{"localhost:9092"},
		GroupID:         "test-group",
		FileEventsTopic: "file.events",
		SyncEventsTopic: "sync.events",
		ConflictTopic:   "conflict.events",
		DLQTopic:        "dlq.file.events",
		Timeout:         1 * time.Second,
		MaxRetries:      2,
		RetryBackoff:    10 * time.Millisecond,
	}

	retryManager := NewRetryManager(producer, config)

	ctx := context.Background()
	fileEvent := &FileEvent{
		ID:        "test-retry-1",
		Type:      FileCreated,
		UserID:    "user123",
		FilePath:  "/test/retry.txt",
		FileHash:  "retry123",
		Size:      1024,
		Timestamp: time.Now(),
	}

	// This should succeed with mock producer
	err := retryManager.PublishWithRetry(ctx, config.FileEventsTopic, fileEvent)
	require.NoError(t, err)

	// Verify event was published
	events := producer.GetEvents("file.events")
	assert.Len(t, events, 1)
}

func TestEventSerializationValidation(t *testing.T) {
	// Test FileEvent serialization
	fileEvent := &FileEvent{
		ID:        "test-serialization-1",
		Type:      FileCreated,
		UserID:    "user123",
		FilePath:  "/test/serialization.txt",
		FileHash:  "serial123",
		Size:      2048,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"created_by": "user123",
			"version":    1,
			"tags":       []string{"test", "serialization"},
		},
	}

	data, err := json.Marshal(fileEvent)
	require.NoError(t, err)

	var deserialized FileEvent
	err = json.Unmarshal(data, &deserialized)
	require.NoError(t, err)

	assert.Equal(t, fileEvent.ID, deserialized.ID)
	assert.Equal(t, fileEvent.Type, deserialized.Type)
	assert.Equal(t, fileEvent.UserID, deserialized.UserID)
	assert.Equal(t, fileEvent.FilePath, deserialized.FilePath)
	assert.Equal(t, fileEvent.FileHash, deserialized.FileHash)
	assert.Equal(t, fileEvent.Size, deserialized.Size)
}

func TestKafkaConfigDefaults(t *testing.T) {
	config := &KafkaConfig{
		Brokers:         []string{"localhost:9092"},
		GroupID:         "test-group",
		FileEventsTopic: "file.events",
		SyncEventsTopic: "sync.events",
		ConflictTopic:   "conflict.events",
		DLQTopic:        "dlq.file.events",
		Timeout:         5 * time.Second,
		MaxRetries:      3,
		RetryBackoff:    500 * time.Millisecond,
	}

	// Test that all required fields are set
	assert.NotEmpty(t, config.Brokers)
	assert.NotEmpty(t, config.GroupID)
	assert.NotEmpty(t, config.FileEventsTopic)
	assert.NotEmpty(t, config.SyncEventsTopic)
	assert.NotEmpty(t, config.ConflictTopic)
	assert.NotEmpty(t, config.DLQTopic)
	assert.Greater(t, config.Timeout, time.Duration(0))
	assert.Greater(t, config.MaxRetries, 0)
	assert.Greater(t, config.RetryBackoff, time.Duration(0))
}

func TestEventIDGeneration(t *testing.T) {
	// Test that all event types have GetID method
	fileEvent := &FileEvent{ID: "file-123"}
	assert.Equal(t, "file-123", fileEvent.GetID())

	syncEvent := &SyncEvent{ID: "sync-123"}
	assert.Equal(t, "sync-123", syncEvent.GetID())

	conflictEvent := &ConflictEvent{ID: "conflict-123"}
	assert.Equal(t, "conflict-123", conflictEvent.GetID())

	dlqEvent := &DLQEvent{ID: "dlq-123"}
	assert.Equal(t, "dlq-123", dlqEvent.GetID())
}

func TestEventTypeConstants(t *testing.T) {
	// Test FileEvent types
	assert.Equal(t, FileEventType("FileCreated"), FileCreated)
	assert.Equal(t, FileEventType("FileUpdated"), FileUpdated)
	assert.Equal(t, FileEventType("FileDeleted"), FileDeleted)

	// Test SyncEvent types
	assert.Equal(t, SyncEventType("SyncStarted"), SyncStarted)
	assert.Equal(t, SyncEventType("SyncCompleted"), SyncCompleted)
	assert.Equal(t, SyncEventType("SyncFailed"), SyncFailed)

	// Test ConflictEvent types
	assert.Equal(t, ConflictEventType("ConflictDetected"), ConflictDetected)
	assert.Equal(t, ConflictEventType("ConflictResolved"), ConflictResolved)
}
