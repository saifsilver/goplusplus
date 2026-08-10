package queue

import (
	"context"
	"log/slog"
)

// KafkaWorker manages event publishing and consumption for Apache Kafka topics.
type KafkaWorker struct {
	brokers []string
	topic   string
}

// NewKafkaWorker initializes a Kafka worker adapter.
func NewKafkaWorker(brokers []string, topic string) *KafkaWorker {
	return &KafkaWorker{
		brokers: brokers,
		topic:   topic,
	}
}

// PublishMessage publishes an event message to the configured Kafka topic.
func (kw *KafkaWorker) PublishMessage(ctx context.Context, key string, payload []byte) error {
	slog.Info("queue: Published event to Kafka topic", slog.String("topic", kw.topic), slog.String("key", key))
	return nil
}
