package cache

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// Store defines the universal caching contract for all in-memory and distributed cache providers.
type Store interface {
	Get(ctx context.Context, key string) (any, bool, error)
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
	loads singleflight.Group
}

// NewMemoryStore initializes a local in-memory cache store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		store: make(map[string]cacheItem),
	}
}

// Set stores a process-local value until its TTL expires.
func (s *MemoryStore) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	s.mu.Lock()
	s.store[key] = cacheItem{val: val, expiresAt: time.Now().Add(ttl)}
	s.mu.Unlock()
	return nil
}

// Get returns a live process-local value when present.
func (s *MemoryStore) Get(ctx context.Context, key string) (any, bool, error) {
	value, found, _, err := s.getWithTTL(ctx, key)
	return value, found, err
}

func (s *MemoryStore) getWithTTL(ctx context.Context, key string) (any, bool, time.Duration, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, 0, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.store[key]
	remaining := time.Until(item.expiresAt)
	if !ok || remaining <= 0 {
		return nil, false, 0, nil
	}
	return item.val, true, remaining, nil
}

// Delete removes a process-local cache entry.
func (s *MemoryStore) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	delete(s.store, key)
	s.mu.Unlock()
	return nil
}

// GetOrSet coalesces concurrent process-local misses and caches the fetched value.
func (s *MemoryStore) GetOrSet(ctx context.Context, key string, ttl time.Duration, fetcher func() (any, error)) (any, error) {
	if val, ok, err := s.Get(ctx, key); err != nil {
		return nil, err
	} else if ok {
		return val, nil
	}
	result := s.loads.DoChan(key, func() (any, error) {
		if val, ok, err := s.Get(ctx, key); err != nil {
			return nil, err
		} else if ok {
			return val, nil
		}
		val, err := fetcher()
		if err != nil {
			return nil, err
		}
		return val, s.Set(ctx, key, val, ttl)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case value := <-result:
		return value.Val, value.Err
	}
}

// InvalidatePrefix removes process-local keys that begin with prefix.
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

// MultiLevelStore provides L1 Memory + L2 Redis tiered caching.
type MultiLevelStore struct {
	l1    Store
	l2    Store
	loads singleflight.Group
}

// NewMultiLevelStore creates a multi-level tiered cache store.
func NewMultiLevelStore(l1, l2 Store) *MultiLevelStore {
	return &MultiLevelStore{l1: l1, l2: l2}
}

// Get reads L1 before L2 and repopulates L1 with the remaining lifetime.
func (m *MultiLevelStore) Get(ctx context.Context, key string) (any, bool, error) {
	if val, ok, err := m.l1.Get(ctx, key); err != nil {
		return nil, false, fmt.Errorf("cache: read L1: %w", err)
	} else if ok {
		return val, true, nil
	}
	if val, ok, remaining, err := getWithRemainingTTL(ctx, m.l2, key); err != nil {
		return nil, false, fmt.Errorf("cache: read L2: %w", err)
	} else if ok {
		if remaining > 0 {
			if err := m.l1.Set(ctx, key, val, min(remaining, 5*time.Minute)); err != nil {
				return nil, false, fmt.Errorf("cache: populate L1: %w", err)
			}
		}
		return val, true, nil
	}
	return nil, false, nil
}

type remainingTTLStore interface {
	getWithTTL(context.Context, string) (any, bool, time.Duration, error)
}

func getWithRemainingTTL(ctx context.Context, store Store, key string) (any, bool, time.Duration, error) {
	if source, ok := store.(remainingTTLStore); ok {
		return source.getWithTTL(ctx, key)
	}
	value, found, err := store.Get(ctx, key)
	return value, found, 0, err
}

// Set writes the authoritative L2 before L1.
func (m *MultiLevelStore) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	if err := m.l2.Set(ctx, key, val, ttl); err != nil {
		return fmt.Errorf("cache: write L2: %w", err)
	}
	if err := m.l1.Set(ctx, key, val, ttl); err != nil {
		return fmt.Errorf("cache: write L1: %w", err)
	}
	return nil
}

// Delete removes key from both cache levels and joins failures.
func (m *MultiLevelStore) Delete(ctx context.Context, key string) error {
	return errors.Join(m.l1.Delete(ctx, key), m.l2.Delete(ctx, key))
}

// GetOrSet coalesces misses and populates both cache levels.
func (m *MultiLevelStore) GetOrSet(ctx context.Context, key string, ttl time.Duration, fetcher func() (any, error)) (any, error) {
	if val, ok, err := m.Get(ctx, key); err != nil {
		return nil, err
	} else if ok {
		return val, nil
	}
	result := m.loads.DoChan(key, func() (any, error) {
		if val, ok, err := m.Get(ctx, key); err != nil {
			return nil, err
		} else if ok {
			return val, nil
		}
		val, err := m.l2.GetOrSet(ctx, key, ttl, fetcher)
		if err != nil {
			return nil, err
		}
		if err := m.l1.Set(ctx, key, val, ttl); err != nil {
			return nil, fmt.Errorf("cache: populate L1: %w", err)
		}
		return val, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case value := <-result:
		return value.Val, value.Err
	}
}

// InvalidatePrefix removes matching keys from both levels and joins failures.
func (m *MultiLevelStore) InvalidatePrefix(ctx context.Context, prefix string) error {
	return errors.Join(m.l1.InvalidatePrefix(ctx, prefix), m.l2.InvalidatePrefix(ctx, prefix))
}

// Legacy Client alias for backwards compatibility
type Client = MemoryStore

// NewClient returns the legacy process-local cache client.
func NewClient() *Client { return NewMemoryStore() }
