package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

const (
	defaultRedisCachePrefix = "gpp:cache:"
	defaultRedisScanCount   = int64(100)
	defaultRedisPingTimeout = 2 * time.Second
	maxCacheKeyBytes        = 1024
)

// Codec serializes values stored by RedisStore.
type Codec interface {
	Marshal(value any) ([]byte, error)
	Unmarshal(data []byte) (any, error)
}

// JSONCodec is the safe default Redis cache codec. Decoded objects use the
// standard encoding/json representations such as map[string]any.
type JSONCodec struct{}

func (JSONCodec) Marshal(value any) ([]byte, error) { return json.Marshal(value) }

func (JSONCodec) Unmarshal(data []byte) (any, error) {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	return value, nil
}

// RedisConfig controls connection validation, namespacing, and serialization.
type RedisConfig struct {
	URL          string
	Prefix       string
	Codec        Codec
	PingTimeout  time.Duration
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	ScanCount    int64
}

// RedisStore implements Store using the official Redis Go client.
type RedisStore struct {
	client     redis.UniversalClient
	prefix     string
	codec      Codec
	scanCount  int64
	ownsClient bool
	loads      singleflight.Group
}

// NewRedisStore creates and verifies a Redis-backed cache. URL is required and
// may use redis:// or rediss:// syntax.
func NewRedisStore(ctx context.Context, config RedisConfig) (*RedisStore, error) {
	if strings.TrimSpace(config.URL) == "" {
		return nil, errors.New("cache: Redis URL is required")
	}
	options, err := redis.ParseURL(config.URL)
	if err != nil {
		return nil, fmt.Errorf("cache: parse Redis URL: %w", err)
	}
	applyRedisTimeouts(options, config)
	client := redis.NewClient(options)
	store, err := newRedisStore(ctx, client, config, true)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	return store, nil
}

// NewRedisStoreFromClient verifies and wraps an application-owned Redis
// client. Closing the store does not close the shared client.
func NewRedisStoreFromClient(ctx context.Context, client redis.UniversalClient, config RedisConfig) (*RedisStore, error) {
	return newRedisStore(ctx, client, config, false)
}

// NewRedisClient is a compatibility convenience for URL-based construction.
func NewRedisClient(ctx context.Context, redisURL string) (*RedisStore, error) {
	return NewRedisStore(ctx, RedisConfig{URL: redisURL})
}

func newRedisStore(ctx context.Context, client redis.UniversalClient, config RedisConfig, ownsClient bool) (*RedisStore, error) {
	if client == nil {
		return nil, errors.New("cache: Redis client is required")
	}
	config = normalizeRedisConfig(config)
	pingCtx, cancel := context.WithTimeout(ctx, config.PingTimeout)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		return nil, fmt.Errorf("cache: ping Redis: %w", err)
	}
	return &RedisStore{
		client: client, prefix: config.Prefix, codec: config.Codec,
		scanCount: config.ScanCount, ownsClient: ownsClient,
	}, nil
}

func normalizeRedisConfig(config RedisConfig) RedisConfig {
	if config.Prefix == "" {
		config.Prefix = defaultRedisCachePrefix
	}
	if config.Codec == nil {
		config.Codec = JSONCodec{}
	}
	if config.PingTimeout <= 0 {
		config.PingTimeout = defaultRedisPingTimeout
	}
	if config.ScanCount <= 0 {
		config.ScanCount = defaultRedisScanCount
	}
	return config
}

func applyRedisTimeouts(options *redis.Options, config RedisConfig) {
	if config.DialTimeout > 0 {
		options.DialTimeout = config.DialTimeout
	}
	if config.ReadTimeout > 0 {
		options.ReadTimeout = config.ReadTimeout
	}
	if config.WriteTimeout > 0 {
		options.WriteTimeout = config.WriteTimeout
	}
}

func (s *RedisStore) Get(ctx context.Context, key string) (any, bool, error) {
	redisKey, err := s.redisKey(key)
	if err != nil {
		return nil, false, err
	}
	data, err := s.client.Get(ctx, redisKey).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("cache: Redis get: %w", err)
	}
	value, err := s.codec.Unmarshal(data)
	if err != nil {
		return nil, false, fmt.Errorf("cache: decode Redis value: %w", err)
	}
	return value, true, nil
}

func (s *RedisStore) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	if ttl <= 0 {
		return errors.New("cache: Redis TTL must be positive")
	}
	redisKey, err := s.redisKey(key)
	if err != nil {
		return err
	}
	data, err := s.codec.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache: encode Redis value: %w", err)
	}
	if err := s.client.Set(ctx, redisKey, data, ttl).Err(); err != nil {
		return fmt.Errorf("cache: Redis set: %w", err)
	}
	return nil
}

func (s *RedisStore) Delete(ctx context.Context, key string) error {
	redisKey, err := s.redisKey(key)
	if err != nil {
		return err
	}
	if err := s.client.Unlink(ctx, redisKey).Err(); err != nil {
		return fmt.Errorf("cache: Redis delete: %w", err)
	}
	return nil
}

func (s *RedisStore) GetOrSet(ctx context.Context, key string, ttl time.Duration, fetcher func() (any, error)) (any, error) {
	if value, found, err := s.Get(ctx, key); err != nil {
		return nil, err
	} else if found {
		return value, nil
	}
	result := s.loads.DoChan(key, func() (any, error) {
		if value, found, err := s.Get(ctx, key); err != nil {
			return nil, err
		} else if found {
			return value, nil
		}
		value, err := fetcher()
		if err != nil {
			return nil, err
		}
		return value, s.Set(ctx, key, value, ttl)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case value := <-result:
		return value.Val, value.Err
	}
}

func (s *RedisStore) InvalidatePrefix(ctx context.Context, prefix string) error {
	pattern := s.prefix + escapeRedisPattern(prefix) + "*"
	var cursor uint64
	for {
		keys, next, err := s.client.Scan(ctx, cursor, pattern, s.scanCount).Result()
		if err != nil {
			return fmt.Errorf("cache: scan Redis prefix: %w", err)
		}
		if len(keys) > 0 {
			if err := s.client.Unlink(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("cache: invalidate Redis prefix: %w", err)
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}

// Client exposes the shared Redis client for other Redis-backed framework
// components such as distributed idempotency.
func (s *RedisStore) Client() redis.UniversalClient { return s.client }

func (s *RedisStore) Close() error {
	if !s.ownsClient {
		return nil
	}
	return s.client.Close()
}

func (s *RedisStore) redisKey(key string) (string, error) {
	if key == "" || len(key) > maxCacheKeyBytes {
		return "", errors.New("cache: key must contain 1 to 1024 bytes")
	}
	return s.prefix + key, nil
}

func escapeRedisPattern(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\", "*", "\\*", "?", "\\?", "[", "\\[", "]", "\\]",
	)
	return replacer.Replace(value)
}
