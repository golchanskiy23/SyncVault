package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"syncvault/internal/config"
	"syncvault/internal/events"
)

func mainFileProducerDemo() {
	log.Println("Starting File Service with Kafka integration...")

	// Load configuration
	cfg, err := config.LoadFromFile("internal/config/config.yml")
	if err != nil {
		log.Printf("Failed to load config, using defaults: %v", err)
		cfg = config.Default()
	}

	// Create Kafka producer
	kafkaConfig := events.NewKafkaConfig(cfg)
	producer := events.NewKafkaProducer(kafkaConfig)
	defer producer.Close()

	// Simulate file operations
	ctx := context.Background()

	// File Created
	fileCreatedEvent := &events.FileEvent{
		ID:        fmt.Sprintf("file_%d", time.Now().UnixNano()),
		Type:      events.FileCreated,
		UserID:    "user123",
		FilePath:  "/documents/example.txt",
		FileHash:  "abc123def456",
		Size:      1024,
		Timestamp: time.Now(),
		Metadata:  map[string]interface{}{"created_by": "user123"},
	}

	log.Printf("Publishing FileCreated event: %s", fileCreatedEvent.FilePath)
	if err := producer.PublishFileEvent(ctx, fileCreatedEvent); err != nil {
		log.Printf("Failed to publish FileCreated event: %v", err)
	} else {
		log.Printf("Successfully published FileCreated event")
	}

	// File Updated
	fileUpdatedEvent := &events.FileEvent{
		ID:        fmt.Sprintf("file_%d", time.Now().UnixNano()),
		Type:      events.FileUpdated,
		UserID:    "user123",
		FilePath:  "/documents/example.txt",
		FileHash:  "def456abc123",
		Size:      2048,
		Timestamp: time.Now(),
		Metadata:  map[string]interface{}{"updated_by": "user123", "version": 2},
	}

	log.Printf("Publishing FileUpdated event: %s", fileUpdatedEvent.FilePath)
	if err := producer.PublishFileEvent(ctx, fileUpdatedEvent); err != nil {
		log.Printf("Failed to publish FileUpdated event: %v", err)
	} else {
		log.Printf("Successfully published FileUpdated event")
	}

	// File Deleted
	fileDeletedEvent := &events.FileEvent{
		ID:        fmt.Sprintf("file_%d", time.Now().UnixNano()),
		Type:      events.FileDeleted,
		UserID:    "user123",
		FilePath:  "/documents/old_file.txt",
		FileHash:  "oldhash123",
		Size:      512,
		Timestamp: time.Now(),
		Metadata:  map[string]interface{}{"deleted_by": "user123"},
	}

	log.Printf("Publishing FileDeleted event: %s", fileDeletedEvent.FilePath)
	if err := producer.PublishFileEvent(ctx, fileDeletedEvent); err != nil {
		log.Printf("Failed to publish FileDeleted event: %v", err)
	} else {
		log.Printf("Successfully published FileDeleted event")
	}

	log.Println("File Service with Kafka integration completed")
}
