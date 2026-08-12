package pubsub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultRedisPubSubMaxPayload = 1 << 20

// ErrRedisNotConfigured indicates an uninitialized Redis bus.
var ErrRedisNotConfigured = errors.New("pubsub: Redis provider is not configured")

// RedisConfig defines connection, channel namespace, and payload limits.
type RedisConfig struct {
	URL             string
	ChannelPrefix   string
	MaxPayloadBytes int
	PingTimeout     time.Duration
}

// RedisBus provides at-most-once distributed notifications. Use the durable
// Kafka or RabbitMQ adapters when messages must survive subscriber downtime.
type RedisBus struct {
	client          redis.UniversalClient
	prefix          string
	maxPayloadBytes int
	ownsClient      bool
}

// NewRedisBus creates and verifies an owned Redis connection.
func NewRedisBus(ctx context.Context, config RedisConfig) (*RedisBus, error) {
	if strings.TrimSpace(config.URL) == "" {
		return nil, errors.New("pubsub: Redis URL is required")
	}
	options, err := redis.ParseURL(config.URL)
	if err != nil {
		return nil, fmt.Errorf("pubsub: parse Redis URL: %w", err)
	}
	return newRedisBus(ctx, redis.NewClient(options), config, true)
}

// NewRedisBusFromClient verifies and wraps an application-owned Redis client.
func NewRedisBusFromClient(
	ctx context.Context, client redis.UniversalClient, config RedisConfig,
) (*RedisBus, error) {
	return newRedisBus(ctx, client, config, false)
}

func newRedisBus(
	ctx context.Context, client redis.UniversalClient, config RedisConfig, ownsClient bool,
) (*RedisBus, error) {
	if client == nil {
		return nil, errors.New("pubsub: Redis client is required")
	}
	if strings.ContainsAny(config.ChannelPrefix, "\r\n") {
		return nil, errors.New("pubsub: invalid Redis channel prefix")
	}
	if config.MaxPayloadBytes <= 0 {
		config.MaxPayloadBytes = defaultRedisPubSubMaxPayload
	}
	if config.PingTimeout <= 0 {
		config.PingTimeout = 2 * time.Second
	}
	pingCtx, cancel := context.WithTimeout(ctx, config.PingTimeout)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		if ownsClient {
			_ = client.Close()
		}
		return nil, fmt.Errorf("pubsub: connect Redis: %w", err)
	}
	return &RedisBus{
		client: client, prefix: config.ChannelPrefix, maxPayloadBytes: config.MaxPayloadBytes, ownsClient: ownsClient,
	}, nil
}

// Publish sends one bounded JSON notification with at-most-once semantics.
func (bus *RedisBus) Publish(ctx context.Context, channel string, payload any) error {
	if bus == nil || bus.client == nil {
		return ErrRedisNotConfigured
	}
	channel, err := bus.channel(channel)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("pubsub: encode Redis payload: %w", err)
	}
	if len(encoded) > bus.maxPayloadBytes {
		return fmt.Errorf("pubsub: Redis payload exceeds %d bytes", bus.maxPayloadBytes)
	}
	if err := bus.client.Publish(ctx, channel, encoded).Err(); err != nil {
		return fmt.Errorf("pubsub: publish Redis message: %w", err)
	}
	return nil
}

// Subscribe blocks until ctx is cancelled or the Redis subscription fails.
func (bus *RedisBus) Subscribe(ctx context.Context, channel string, handler func(msg any)) error {
	if bus == nil || bus.client == nil {
		return ErrRedisNotConfigured
	}
	if handler == nil {
		return errors.New("pubsub: Redis subscriber handler is required")
	}
	channel, err := bus.channel(channel)
	if err != nil {
		return err
	}
	subscription := bus.client.Subscribe(ctx, channel)
	defer subscription.Close()
	if _, err := subscription.Receive(ctx); err != nil {
		return fmt.Errorf("pubsub: establish Redis subscription: %w", err)
	}
	for {
		message, err := subscription.ReceiveMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("pubsub: receive Redis message: %w", err)
		}
		if len(message.Payload) > bus.maxPayloadBytes {
			return fmt.Errorf("pubsub: received Redis payload exceeds %d bytes", bus.maxPayloadBytes)
		}
		var payload any
		if err := json.Unmarshal([]byte(message.Payload), &payload); err != nil {
			return fmt.Errorf("pubsub: decode Redis payload: %w", err)
		}
		if err := invokeRedisHandler(handler, payload); err != nil {
			return err
		}
	}
}

// Close releases the Redis client only when the bus owns it.
func (bus *RedisBus) Close() error {
	if bus == nil || bus.client == nil || !bus.ownsClient {
		return nil
	}
	return bus.client.Close()
}

func (bus *RedisBus) channel(channel string) (string, error) {
	channel = strings.TrimSpace(channel)
	if channel == "" || len(channel) > 256 || strings.ContainsAny(channel, "\r\n\t ") {
		return "", errors.New("pubsub: invalid Redis channel")
	}
	return bus.prefix + channel, nil
}

func invokeRedisHandler(handler func(any), payload any) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("pubsub: Redis subscriber handler panic: %v", recovered)
		}
	}()
	handler(payload)
	return nil
}
