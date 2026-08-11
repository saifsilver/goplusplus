package pubsub

import (
	"context"
	"errors"
	"testing"
)

func TestUnconfiguredExternalBusesFailClosed(t *testing.T) {
	ctx := context.Background()
	var redisBus *RedisBus
	if err := redisBus.Publish(ctx, "events", "payload"); !errors.Is(err, ErrRedisNotConfigured) {
		t.Fatalf("Redis Publish error = %v", err)
	}
	if err := redisBus.Subscribe(ctx, "events", func(any) {}); !errors.Is(err, ErrRedisNotConfigured) {
		t.Fatalf("Redis Subscribe error = %v", err)
	}
	var rabbitBus *RabbitMQBus
	if err := rabbitBus.Publish(ctx, "events", "created", nil); !errors.Is(err, ErrRabbitMQNotConfigured) {
		t.Fatalf("RabbitMQ Publish error = %v", err)
	}
	if err := rabbitBus.Subscribe(ctx, "events", "created", func(context.Context, []byte) error { return nil }); !errors.Is(err, ErrRabbitMQNotConfigured) {
		t.Fatalf("RabbitMQ Subscribe error = %v", err)
	}
}
