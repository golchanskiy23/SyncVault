package events

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFileEventTypes(t *testing.T) {
	tests := []struct {
		name     string
		event    FileEvent
		expected FileEventType
	}{
		{
			name: "FileCreated",
			event: FileEvent{
				Type: FileCreated,
			},
			expected: FileCreated,
		},
		{
			name: "FileUpdated",
			event: FileEvent{
				Type: FileUpdated,
			},
			expected: FileUpdated,
		},
		{
			name: "FileDeleted",
			event: FileEvent{
				Type: FileDeleted,
			},
			expected: FileDeleted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.event.Type)
		})
	}
}

func TestSyncEventTypes(t *testing.T) {
	tests := []struct {
		name     string
		event    SyncEvent
		expected SyncEventType
	}{
		{
			name: "SyncStarted",
			event: SyncEvent{
				Type: SyncStarted,
			},
			expected: SyncStarted,
		},
		{
			name: "SyncCompleted",
			event: SyncEvent{
				Type: SyncCompleted,
			},
			expected: SyncCompleted,
		},
		{
			name: "SyncFailed",
			event: SyncEvent{
				Type: SyncFailed,
			},
			expected: SyncFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.event.Type)
		})
	}
}

func TestConflictEventTypes(t *testing.T) {
	tests := []struct {
		name     string
		event    ConflictEvent
		expected ConflictEventType
	}{
		{
			name: "ConflictDetected",
			event: ConflictEvent{
				Type: ConflictDetected,
			},
			expected: ConflictDetected,
		},
		{
			name: "ConflictResolved",
			event: ConflictEvent{
				Type: ConflictResolved,
			},
			expected: ConflictResolved,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.event.Type)
		})
	}
}

func TestKafkaConfig(t *testing.T) {
	config := &KafkaConfig{
		Brokers:         []string{"localhost:9092"},
		GroupID:         "test-group",
		FileEventsTopic: "test-file-events",
		SyncEventsTopic: "test-sync-events",
		ConflictTopic:   "test-conflict-events",
		DLQTopic:        "test-dlq",
		Timeout:         10 * time.Second,
		MaxRetries:      3,
		RetryBackoff:    100 * time.Millisecond,
	}

	assert.Equal(t, []string{"localhost:9092"}, config.Brokers)
	assert.Equal(t, "test-group", config.GroupID)
	assert.Equal(t, "test-file-events", config.FileEventsTopic)
	assert.Equal(t, "test-sync-events", config.SyncEventsTopic)
	assert.Equal(t, "test-conflict-events", config.ConflictTopic)
	assert.Equal(t, "test-dlq", config.DLQTopic)
	assert.Equal(t, 10*time.Second, config.Timeout)
	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, 100*time.Millisecond, config.RetryBackoff)
}

func TestFileEventGetID(t *testing.T) {
	event := &FileEvent{
		ID: "test-file-1",
	}
	assert.Equal(t, "test-file-1", event.GetID())
}

func TestSyncEventGetID(t *testing.T) {
	event := &SyncEvent{
		ID: "test-sync-1",
	}
	assert.Equal(t, "test-sync-1", event.GetID())
}

func TestConflictEventGetID(t *testing.T) {
	event := &ConflictEvent{
		ID: "test-conflict-1",
	}
	assert.Equal(t, "test-conflict-1", event.GetID())
}

func TestDLQEventGetID(t *testing.T) {
	event := &DLQEvent{
		ID: "test-dlq-1",
	}
	assert.Equal(t, "test-dlq-1", event.GetID())
}
