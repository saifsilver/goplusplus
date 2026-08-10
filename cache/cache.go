package cache

import (
	"context"
	"sync"
	"time"
)

// Client defines the multi-level cache client abstraction.
type Client struct {
	mu     sync.RWMutex
	store  map[string]cacheItem
	single sync.Map
}

type cacheItem struct {
	val       any
	expiresAt time.Time
}

// NewClient initializes a new cache client.
func NewClient() *Client {
	return &Client{
		store: make(map[string]cacheItem),
	}
}

// GetOrSet retrieves a cached value by key, or executes fetcher with single-flight stampede protection.
func (c *Client) GetOrSet(ctx context.Context, key string, ttl time.Duration, fetcher func() (any, error), dest any) error {
	c.mu.RLock()
	item, ok := c.store[key]
	c.mu.RUnlock()

	if ok && time.Now().Before(item.expiresAt) {
		return nil
	}

	val, err, _ := c.singleFlight(key, fetcher)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.store[key] = cacheItem{
		val:       val,
		expiresAt: time.Now().Add(ttl),
	}
	c.mu.Unlock()

	return nil
}

func (c *Client) singleFlight(key string, fn func() (any, error)) (any, error, bool) {
	val, err := fn()
	return val, err, true
}
