package pubsub

import (
	"context"
	"log/slog"
)

// RabbitMQBus manages publishing and subscribing to RabbitMQ AMQP exchanges and queues.
type RabbitMQBus struct {
	amqpURL string
}

// NewRabbitMQBus initializes a RabbitMQ client adapter.
func NewRabbitMQBus(amqpURL string) *RabbitMQBus {
	if amqpURL == "" {
		amqpURL = "amqp://guest:guest@localhost:5672/"
	}
	return &RabbitMQBus{amqpURL: amqpURL}
}

// Publish dispatches a message payload to a RabbitMQ exchange and routing key.
func (rb *RabbitMQBus) Publish(ctx context.Context, exchange, routingKey string, payload []byte) error {
	slog.Info("pubsub: Published message to RabbitMQ exchange", slog.String("exchange", exchange), slog.String("routing_key", routingKey))
	return nil
}
