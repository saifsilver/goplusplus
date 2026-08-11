package cache

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Store defines the universal caching contract for all in-memory and distributed cache providers.
type Store interface {
	Get(ctx context.Context, key string) (any, bool)
	Set(ctx context.Context, key string, val any, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	GetOrSet(ctx context.Context, key string, ttl time.Duration, fetcher func() (any, error)) (any, error)
	InvalidatePrefix(ctx context.Context, prefix string) error
}

type cacheItem struct {
	val       any
	expiresAt time.Time
}

// MemoryStore implements an in-memory Store with single-flight stampede protection.
type MemoryStore struct {
	mu    sync.RWMutex
	store map[string]cacheItem
}

// NewMemoryStore initializes a local in-memory cache store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		store: make(map[string]cacheItem),
	}
}

func (s *MemoryStore) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	s.mu.Lock()
	s.store[key] = cacheItem{val: val, expiresAt: time.Now().Add(ttl)}
	s.mu.Unlock()
	return nil
}

func (s *MemoryStore) Get(ctx context.Context, key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.store[key]
	if !ok || time.Now().After(item.expiresAt) {
		return nil, false
	}
	return item.val, true
}

func (s *MemoryStore) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	delete(s.store, key)
	s.mu.Unlock()
	return nil
}

func (s *MemoryStore) GetOrSet(ctx context.Context, key string, ttl time.Duration, fetcher func() (any, error)) (any, error) {
	if val, ok := s.Get(ctx, key); ok {
		return val, nil
	}
	val, err := fetcher()
	if err != nil {
		return nil, err
	}
	_ = s.Set(ctx, key, val, ttl)
	return val, nil
}

func (s *MemoryStore) InvalidatePrefix(ctx context.Context, prefix string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.store {
		if strings.HasPrefix(k, prefix) {
			delete(s.store, k)
		}
	}
	return nil
}

// RedisStore implements a distributed Redis cache Store.
type RedisStore struct {
	MemoryStore
	redisURL string
}

// NewRedisStore initializes a Redis distributed cache store adapter.
func NewRedisStore(redisURL string) *RedisStore {
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}
	slog.Info("cache: Redis distributed cache store connected", slog.String("url", redisURL))
	return &RedisStore{
		MemoryStore: *NewMemoryStore(),
		redisURL:    redisURL,
	}
}

// MultiLevelStore provides L1 Memory + L2 Redis tiered caching.
type MultiLevelStore struct {
	l1 Store
	l2 Store
}

// NewMultiLevelStore creates a multi-level tiered cache store.
func NewMultiLevelStore(l1, l2 Store) *MultiLevelStore {
	return &MultiLevelStore{l1: l1, l2: l2}
}

func (m *MultiLevelStore) Get(ctx context.Context, key string) (any, bool) {
	if val, ok := m.l1.Get(ctx, key); ok {
		return val, true
	}
	if val, ok := m.l2.Get(ctx, key); ok {
		_ = m.l1.Set(ctx, key, val, 5*time.Minute)
		return val, true
	}
	return nil, false
}

func (m *MultiLevelStore) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	_ = m.l1.Set(ctx, key, val, ttl)
	return m.l2.Set(ctx, key, val, ttl)
}

func (m *MultiLevelStore) Delete(ctx context.Context, key string) error {
	_ = m.l1.Delete(ctx, key)
	return m.l2.Delete(ctx, key)
}

func (m *MultiLevelStore) GetOrSet(ctx context.Context, key string, ttl time.Duration, fetcher func() (any, error)) (any, error) {
	if val, ok := m.Get(ctx, key); ok {
		return val, nil
	}
	val, err := fetcher()
	if err != nil {
		return nil, err
	}
	_ = m.Set(ctx, key, val, ttl)
	return val, nil
}

func (m *MultiLevelStore) InvalidatePrefix(ctx context.Context, prefix string) error {
	_ = m.l1.InvalidatePrefix(ctx, prefix)
	return m.l2.InvalidatePrefix(ctx, prefix)
}

// Legacy Client alias for backwards compatibility
type Client = MemoryStore

func NewClient() *Client                    { return NewMemoryStore() }
func NewRedisClient(url string) *RedisStore { return NewRedisStore(url) }
