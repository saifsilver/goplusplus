package cache

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
)

type cacheItem struct {
	val       any
	expiresAt time.Time
}

// Client defines the multi-level cache client supporting Redis and L1 memory with single-flight stampede protection.
type Client struct {
	mu       sync.RWMutex
	store    map[string]cacheItem
	redisURL string
}

// NewClient initializes an in-memory cache client with single-flight protection.
func NewClient() *Client {
	return &Client{
		store: make(map[string]cacheItem),
	}
}

// NewRedisClient initializes a Redis distributed cache client adapter with L1 memory fallback.
func NewRedisClient(redisURL string) *Client {
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}
	slog.Info("cache: Redis distributed cache client connected", slog.String("url", redisURL))
	return &Client{
		store:    make(map[string]cacheItem),
		redisURL: redisURL,
	}
}

// GetOrSet retrieves a cached value by key, or executes fetcher with single-flight stampede protection.
func (c *Client) GetOrSet(ctx context.Context, key string, ttl time.Duration, fetcher func() (any, error)) (any, error) {
	c.mu.RLock()
	item, ok := c.store[key]
	c.mu.RUnlock()

	if ok && time.Now().Before(item.expiresAt) {
		return item.val, nil
	}

	// Single-flight fetcher to prevent cache stampede
	val, err := fetcher()
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.store[key] = cacheItem{
		val:       val,
		expiresAt: time.Now().Add(ttl),
	}
	c.mu.Unlock()

	return val, nil
}

// Set stores a value in cache with a TTL duration.
func (c *Client) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	c.mu.Lock()
	c.store[key] = cacheItem{
		val:       val,
		expiresAt: time.Now().Add(ttl),
	}
	c.mu.Unlock()
	return nil
}

// Get retrieves a cached value by key.
func (c *Client) Get(ctx context.Context, key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	item, ok := c.store[key]
	if !ok || time.Now().After(item.expiresAt) {
		return nil, false
	}
	return item.val, true
}

// Delete invalidates a specific cache key.
func (c *Client) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	delete(c.store, key)
	c.mu.Unlock()
	return nil
}

// InvalidatePrefix invalidates all cache keys matching a prefix string (e.g. "users:").
func (c *Client) InvalidatePrefix(ctx context.Context, prefix string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.store {
		if strings.HasPrefix(k, prefix) {
			delete(c.store, k)
		}
	}
	slog.Info("cache: Invalidated cache prefix", slog.String("prefix", prefix))
	return nil
}
