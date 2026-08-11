package cache

import (
	"container/list"
	"context"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type BoundedConfig struct {
	MaxEntries int
}

// BoundedMemoryStore implements an in-memory Store with strict capacity limits to prevent OOM memory growth.
type BoundedMemoryStore struct {
	mu         sync.RWMutex
	store      map[string]*boundedEntry
	keysOrder  list.List
	maxEntries int
	loads      singleflight.Group
}

type boundedEntry struct {
	item  cacheItem
	order *list.Element
}

// NewBoundedMemoryStore initializes a capacity-capped in-memory cache store.
func NewBoundedMemoryStore(maxEntries int) *BoundedMemoryStore {
	if maxEntries <= 0 {
		maxEntries = 10000
	}
	return &BoundedMemoryStore{
		store:      make(map[string]*boundedEntry),
		maxEntries: maxEntries,
	}
}

func (s *BoundedMemoryStore) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// If key exists, update and keep position
	if entry, ok := s.store[key]; ok {
		entry.item = cacheItem{val: val, expiresAt: time.Now().Add(ttl)}
		return nil
	}

	// Evict expired or oldest items if max capacity reached
	if len(s.store) >= s.maxEntries {
		s.evictOneLocked()
	}

	order := s.keysOrder.PushBack(key)
	s.store[key] = &boundedEntry{
		item:  cacheItem{val: val, expiresAt: time.Now().Add(ttl)},
		order: order,
	}
	return nil
}

func (s *BoundedMemoryStore) Get(ctx context.Context, key string) (any, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.store[key]
	if !ok || time.Now().After(entry.item.expiresAt) {
		return nil, false, nil
	}
	return entry.item.val, true, nil
}

func (s *BoundedMemoryStore) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteLocked(key)
	return nil
}

func (s *BoundedMemoryStore) GetOrSet(ctx context.Context, key string, ttl time.Duration, fetcher func() (any, error)) (any, error) {
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

func (s *BoundedMemoryStore) InvalidatePrefix(ctx context.Context, prefix string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.store {
		if strings.HasPrefix(k, prefix) {
			s.deleteLocked(k)
		}
	}
	return nil
}

func (s *BoundedMemoryStore) evictOneLocked() {
	now := time.Now()
	// First pass: remove an expired item if any
	for element := s.keysOrder.Front(); element != nil; element = element.Next() {
		key := element.Value.(string)
		if entry := s.store[key]; now.After(entry.item.expiresAt) {
			s.deleteLocked(key)
			return
		}
	}

	// Second pass: remove oldest item (FIFO)
	if oldest := s.keysOrder.Front(); oldest != nil {
		s.deleteLocked(oldest.Value.(string))
	}
}

func (s *BoundedMemoryStore) deleteLocked(key string) {
	entry, ok := s.store[key]
	if !ok {
		return
	}
	s.keysOrder.Remove(entry.order)
	delete(s.store, key)
}
