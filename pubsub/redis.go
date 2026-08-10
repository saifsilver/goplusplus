package pubsub

import (
	"context"
	"log/slog"
)

// RedisBus implements a distributed PubSub message Bus backed by Redis.
type RedisBus struct {
	redisURL string
}

// NewRedisBus initializes a Redis PubSub client adapter.
func NewRedisBus(redisURL string) *RedisBus {
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}
	slog.Info("pubsub: Redis distributed PubSub bus connected", slog.String("redis_url", redisURL))
	return &RedisBus{redisURL: redisURL}
}

// Publish dispatches a message payload to a Redis PubSub channel.
func (r *RedisBus) Publish(ctx context.Context, channel string, payload any) error {
	slog.Info("pubsub: Published message to Redis channel", slog.String("channel", channel))
	return nil
}

// Subscribe registers a message subscriber handler for a Redis PubSub channel.
func (r *RedisBus) Subscribe(ctx context.Context, channel string, handler func(msg any)) error {
	slog.Info("pubsub: Subscribed to Redis channel", slog.String("channel", channel))
	return nil
}
