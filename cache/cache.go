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

func (s *MemoryStore) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	s.mu.Lock()
	s.store[key] = cacheItem{val: val, expiresAt: time.Now().Add(ttl)}
	s.mu.Unlock()
	return nil
}

func (s *MemoryStore) Get(ctx context.Context, key string) (any, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.store[key]
	if !ok || time.Now().After(item.expiresAt) {
		return nil, false, nil
	}
	return item.val, true, nil
}

func (s *MemoryStore) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	delete(s.store, key)
	s.mu.Unlock()
	return nil
}

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
	l1 Store
	l2 Store
}

// NewMultiLevelStore creates a multi-level tiered cache store.
func NewMultiLevelStore(l1, l2 Store) *MultiLevelStore {
	return &MultiLevelStore{l1: l1, l2: l2}
}

func (m *MultiLevelStore) Get(ctx context.Context, key string) (any, bool, error) {
	if val, ok, err := m.l1.Get(ctx, key); err != nil {
		return nil, false, fmt.Errorf("cache: read L1: %w", err)
	} else if ok {
		return val, true, nil
	}
	if val, ok, err := m.l2.Get(ctx, key); err != nil {
		return nil, false, fmt.Errorf("cache: read L2: %w", err)
	} else if ok {
		if err := m.l1.Set(ctx, key, val, 5*time.Minute); err != nil {
			return nil, false, fmt.Errorf("cache: populate L1: %w", err)
		}
		return val, true, nil
	}
	return nil, false, nil
}

func (m *MultiLevelStore) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	if err := m.l2.Set(ctx, key, val, ttl); err != nil {
		return fmt.Errorf("cache: write L2: %w", err)
	}
	if err := m.l1.Set(ctx, key, val, ttl); err != nil {
		return fmt.Errorf("cache: write L1: %w", err)
	}
	return nil
}

func (m *MultiLevelStore) Delete(ctx context.Context, key string) error {
	return errors.Join(m.l1.Delete(ctx, key), m.l2.Delete(ctx, key))
}

func (m *MultiLevelStore) GetOrSet(ctx context.Context, key string, ttl time.Duration, fetcher func() (any, error)) (any, error) {
	if val, ok, err := m.Get(ctx, key); err != nil {
		return nil, err
	} else if ok {
		return val, nil
	}
	val, err := fetcher()
	if err != nil {
		return nil, err
	}
	return val, m.Set(ctx, key, val, ttl)
}

func (m *MultiLevelStore) InvalidatePrefix(ctx context.Context, prefix string) error {
	return errors.Join(m.l1.InvalidatePrefix(ctx, prefix), m.l2.InvalidatePrefix(ctx, prefix))
}

// Legacy Client alias for backwards compatibility
type Client = MemoryStore

func NewClient() *Client { return NewMemoryStore() }
