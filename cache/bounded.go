package cache

import (
	"context"
	"strings"
	"sync"
	"time"
)

type BoundedConfig struct {
	MaxEntries int
}

// BoundedMemoryStore implements an in-memory Store with strict capacity limits to prevent OOM memory growth.
type BoundedMemoryStore struct {
	mu         sync.RWMutex
	store      map[string]cacheItem
	keysOrder  []string
	maxEntries int
}

// NewBoundedMemoryStore initializes a capacity-capped in-memory cache store.
func NewBoundedMemoryStore(maxEntries int) *BoundedMemoryStore {
	if maxEntries <= 0 {
		maxEntries = 10000
	}
	return &BoundedMemoryStore{
		store:      make(map[string]cacheItem),
		keysOrder:  make([]string, 0, maxEntries),
		maxEntries: maxEntries,
	}
}

func (s *BoundedMemoryStore) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// If key exists, update and keep position
	if _, ok := s.store[key]; ok {
		s.store[key] = cacheItem{val: val, expiresAt: time.Now().Add(ttl)}
		return nil
	}

	// Evict expired or oldest items if max capacity reached
	if len(s.store) >= s.maxEntries {
		s.evictOneLocked()
	}

	s.store[key] = cacheItem{val: val, expiresAt: time.Now().Add(ttl)}
	s.keysOrder = append(s.keysOrder, key)
	return nil
}

func (s *BoundedMemoryStore) Get(ctx context.Context, key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.store[key]
	if !ok || time.Now().After(item.expiresAt) {
		return nil, false
	}
	return item.val, true
}

func (s *BoundedMemoryStore) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.store, key)
	return nil
}

func (s *BoundedMemoryStore) GetOrSet(ctx context.Context, key string, ttl time.Duration, fetcher func() (any, error)) (any, error) {
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

func (s *BoundedMemoryStore) InvalidatePrefix(ctx context.Context, prefix string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.store {
		if strings.HasPrefix(k, prefix) {
			delete(s.store, k)
		}
	}
	return nil
}

func (s *BoundedMemoryStore) evictOneLocked() {
	now := time.Now()
	// First pass: remove an expired item if any
	for i, k := range s.keysOrder {
		if item, ok := s.store[k]; ok && now.After(item.expiresAt) {
			delete(s.store, k)
			s.keysOrder = append(s.keysOrder[:i], s.keysOrder[i+1:]...)
			return
		}
	}

	// Second pass: remove oldest item (FIFO)
	if len(s.keysOrder) > 0 {
		oldestKey := s.keysOrder[0]
		delete(s.store, oldestKey)
		s.keysOrder = s.keysOrder[1:]
	}
}
