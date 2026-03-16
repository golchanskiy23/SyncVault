package events

import "syncvault/internal/config"

func NewKafkaConfig(cfg *config.Config) *KafkaConfig {
	return &KafkaConfig{
		Brokers:         cfg.Kafka.Brokers,
		GroupID:         cfg.Kafka.GroupID,
		FileEventsTopic: cfg.Kafka.FileEventsTopic,
		SyncEventsTopic: cfg.Kafka.SyncEventsTopic,
		ConflictTopic:   cfg.Kafka.ConflictEventsTopic,
		DLQTopic:        cfg.Kafka.DLQTopic,
		Timeout:         cfg.Kafka.ProducerTimeout,
		MaxRetries:      cfg.Kafka.MaxRetries,
		RetryBackoff:    cfg.Kafka.RetryBackoff,
	}
}
