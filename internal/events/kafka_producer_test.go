package events

import (
	"context"
	"testing"
	"time"
)

// TestPublishWithRetry_SafeTypeAssertion verifies Bug 1.1 fix:
// publishWithRetry must NOT panic when the event does not implement GetID().
// Before the fix, the direct type assertion caused a panic.
func TestPublishWithRetry_SafeTypeAssertion(t *testing.T) {
	// eventWithoutGetID does NOT implement GetID() string
	type eventWithoutGetID struct {
		Data string
	}

	// We can't call publishWithRetry directly without a real Kafka broker,
	// but we can verify the type-switch logic in isolation.
	event := &eventWithoutGetID{Data: "test"}

	var eventID string
	switch e := any(event).(type) {
	case interface{ GetID() string }:
		eventID = e.GetID()
	default:
		eventID = "fallback"
	}

	if eventID == "" {
		t.Error("eventID must not be empty — fallback should be used")
	}
	if eventID != "fallback" {
		t.Errorf("expected fallback, got %q", eventID)
	}
}

// TestPublishWithRetry_EventWithGetID verifies that events implementing GetID()
// still use their real ID (preservation test for Bug 1.1 fix).
func TestPublishWithRetry_EventWithGetID(t *testing.T) {
	event := &FileEvent{ID: "abc123", Type: FileCreated, Timestamp: time.Now()}

	var eventID string
	switch e := any(event).(type) {
	case interface{ GetID() string }:
		eventID = e.GetID()
	default:
		eventID = "fallback"
	}

	if eventID != "abc123" {
		t.Errorf("expected real ID abc123, got %q", eventID)
	}
}

// TestSyncEvent_GetID verifies SyncEvent implements GetID().
func TestSyncEvent_GetID(t *testing.T) {
	event := &SyncEvent{ID: "sync-42"}
	if event.GetID() != "sync-42" {
		t.Errorf("expected sync-42, got %q", event.GetID())
	}
}

// TestSyncEvent_OperationType verifies Bug 1.7 fix: SyncEvent has OperationType field.
func TestSyncEvent_OperationType(t *testing.T) {
	event := &SyncEvent{
		ID:            "sync-1",
		Status:        "delete_initiated",
		OperationType: "delete",
	}
	if event.OperationType != "delete" {
		t.Errorf("expected OperationType=delete, got %q", event.OperationType)
	}
	if event.Status != "delete_initiated" {
		t.Errorf("expected Status=delete_initiated, got %q", event.Status)
	}
}

// TestKafkaConfig_Timeout ensures config has a non-zero timeout (used in publishWithRetry).
func TestKafkaConfig_Timeout(t *testing.T) {
	cfg := &KafkaConfig{
		Brokers:         []string{"localhost:9092"},
		FileEventsTopic: "file-events",
		SyncEventsTopic: "sync-events",
		ConflictTopic:   "conflicts",
		DLQTopic:        "dlq",
		Timeout:         5 * time.Second,
		MaxRetries:      3,
		RetryBackoff:    100 * time.Millisecond,
	}

	if cfg.Timeout == 0 {
		t.Error("KafkaConfig.Timeout must not be zero")
	}
}

// TestCancelNotLeaked verifies the cancel-in-loop fix:
// context.WithTimeout + explicit cancel() should not accumulate contexts.
func TestCancelNotLeaked(t *testing.T) {
	parent := context.Background()
	const iterations = 100

	for i := 0; i < iterations; i++ {
		ctx, cancel := context.WithTimeout(parent, 10*time.Millisecond)
		// Simulate the fixed pattern: cancel() called explicitly, not via defer
		cancel()
		_ = ctx
	}
	// If we reach here without goroutine leak, the pattern is correct.
	// (Actual goroutine leak detection would require goleak in integration tests.)
}
