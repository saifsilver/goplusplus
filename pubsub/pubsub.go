package pubsub

import (
	"context"
	"log/slog"
	"sync"
)

// Bus represents an in-memory or Redis event message bus.
type Bus struct {
	mu     sync.RWMutex
	topics map[string][]func(ctx context.Context, payload any)
}

// New initializes an event bus instance.
func New() *Bus {
	return &Bus{
		topics: make(map[string][]func(ctx context.Context, payload any)),
	}
}

// Publish broadcasts an event payload onto a named topic.
func (b *Bus) Publish(ctx context.Context, topic string, payload any) error {
	slog.Info("pubsub: Published event", slog.String("topic", topic))
	b.mu.RLock()
	handlers := b.topics[topic]
	b.mu.RUnlock()

	for _, h := range handlers {
		go h(ctx, payload)
	}
	return nil
}

// Subscribe attaches an event handler function to a named topic.
func (b *Bus) Subscribe(topic string, handler func(ctx context.Context, payload any)) {
	b.mu.Lock()
	b.topics[topic] = append(b.topics[topic], handler)
	b.mu.Unlock()
}
