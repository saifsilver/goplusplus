package middleware

import (
	"container/list"
	"context"
	"errors"
	"net/http"
	"slices"
	"sync"
	"time"
)

var (
	ErrIdempotencyStoreCapacity = errors.New("idempotency store capacity reached")
	ErrIdempotencyOwnershipLost = errors.New("idempotency claim ownership lost")
)

type IdempotencyState string

const (
	IdempotencyPending  IdempotencyState = "pending"
	IdempotencyComplete IdempotencyState = "complete"
)

// IdempotencyResponse is the replayable HTTP response persisted by a store.
type IdempotencyResponse struct {
	Status int         `json:"status"`
	Header http.Header `json:"header"`
	Body   []byte      `json:"body"`
}

// IdempotencyClaim describes either a newly acquired claim or existing state.
type IdempotencyClaim struct {
	Acquired    bool
	State       IdempotencyState
	Fingerprint string
	Response    *IdempotencyResponse
}

// IdempotencyStore atomically coordinates claims across middleware instances.
type IdempotencyStore interface {
	Claim(ctx context.Context, key, fingerprint, owner string, ttl time.Duration) (IdempotencyClaim, error)
	Complete(ctx context.Context, key, fingerprint, owner string, response IdempotencyResponse, ttl time.Duration) error
	Release(ctx context.Context, key, owner string) error
}

type memoryIdempotencyEntry struct {
	fingerprint string
	owner       string
	state       IdempotencyState
	response    *IdempotencyResponse
	expiresAt   time.Time
	order       *list.Element
}

// MemoryIdempotencyStore is a bounded process-local implementation.
type MemoryIdempotencyStore struct {
	mu         sync.Mutex
	entries    map[string]*memoryIdempotencyEntry
	order      list.List
	maxEntries int
}

func NewMemoryIdempotencyStore(maxEntries int) *MemoryIdempotencyStore {
	if maxEntries <= 0 {
		maxEntries = 10000
	}
	return &MemoryIdempotencyStore{
		entries: make(map[string]*memoryIdempotencyEntry, maxEntries), maxEntries: maxEntries,
	}
}

func (s *MemoryIdempotencyStore) Claim(
	ctx context.Context, key, fingerprint, owner string, ttl time.Duration,
) (IdempotencyClaim, error) {
	if err := validateIdempotencyStoreInput(ctx, key, fingerprint, owner, ttl); err != nil {
		return IdempotencyClaim{}, err
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeExpiredLocked(now)
	if entry, exists := s.entries[key]; exists {
		return memoryClaim(entry), nil
	}
	if len(s.entries) >= s.maxEntries && !s.evictCompletedLocked() {
		return IdempotencyClaim{}, ErrIdempotencyStoreCapacity
	}
	entry := &memoryIdempotencyEntry{
		fingerprint: fingerprint, owner: owner, state: IdempotencyPending, expiresAt: now.Add(ttl),
	}
	entry.order = s.order.PushBack(key)
	s.entries[key] = entry
	return IdempotencyClaim{Acquired: true, State: IdempotencyPending, Fingerprint: fingerprint}, nil
}

func (s *MemoryIdempotencyStore) Complete(
	ctx context.Context, key, fingerprint, owner string, response IdempotencyResponse, ttl time.Duration,
) error {
	if err := validateIdempotencyStoreInput(ctx, key, fingerprint, owner, ttl); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entries[key]
	if !exists || entry.state != IdempotencyPending || entry.owner != owner || entry.fingerprint != fingerprint {
		return ErrIdempotencyOwnershipLost
	}
	cloned := cloneIdempotencyResponse(response)
	entry.state = IdempotencyComplete
	entry.response = &cloned
	entry.expiresAt = time.Now().Add(ttl)
	return nil
}

func (s *MemoryIdempotencyStore) Release(ctx context.Context, key, owner string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if key == "" || owner == "" {
		return errors.New("idempotency store key and owner are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entries[key]
	if !exists {
		return nil
	}
	if entry.state != IdempotencyPending || entry.owner != owner {
		return ErrIdempotencyOwnershipLost
	}
	s.deleteLocked(key)
	return nil
}

func validateIdempotencyStoreInput(ctx context.Context, key, fingerprint, owner string, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if key == "" || fingerprint == "" || owner == "" {
		return errors.New("idempotency store key, fingerprint, and owner are required")
	}
	if ttl <= 0 {
		return errors.New("idempotency store TTL must be positive")
	}
	return nil
}

func memoryClaim(entry *memoryIdempotencyEntry) IdempotencyClaim {
	claim := IdempotencyClaim{State: entry.state, Fingerprint: entry.fingerprint}
	if entry.response != nil {
		response := cloneIdempotencyResponse(*entry.response)
		claim.Response = &response
	}
	return claim
}

func cloneIdempotencyResponse(response IdempotencyResponse) IdempotencyResponse {
	return IdempotencyResponse{
		Status: response.Status, Header: response.Header.Clone(), Body: slices.Clone(response.Body),
	}
}

func (s *MemoryIdempotencyStore) removeExpiredLocked(now time.Time) {
	for element := s.order.Front(); element != nil; {
		next := element.Next()
		key := element.Value.(string)
		if !now.Before(s.entries[key].expiresAt) {
			s.deleteLocked(key)
		}
		element = next
	}
}

func (s *MemoryIdempotencyStore) evictCompletedLocked() bool {
	for element := s.order.Front(); element != nil; element = element.Next() {
		key := element.Value.(string)
		if s.entries[key].state == IdempotencyComplete {
			s.deleteLocked(key)
			return true
		}
	}
	return false
}

func (s *MemoryIdempotencyStore) deleteLocked(key string) {
	entry, exists := s.entries[key]
	if !exists {
		return
	}
	s.order.Remove(entry.order)
	delete(s.entries, key)
}
