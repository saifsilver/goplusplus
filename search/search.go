package search

import (
	"context"
	"strings"
	"sync"
)

// Engine defines the contract for full-text search indexing and keyword search providers.
type Engine interface {
	IndexDocument(ctx context.Context, indexName, docID string, payload any) error
	SearchKeyword(ctx context.Context, indexName, query string) ([]map[string]any, error)
}

// InMemSearchEngine provides a lightweight in-memory search Engine implementation.
type InMemSearchEngine struct {
	mu    sync.RWMutex
	store map[string]map[string]any
}

// NewInMemSearchEngine initializes an in-memory search engine.
func NewInMemSearchEngine() *InMemSearchEngine {
	return &InMemSearchEngine{
		store: make(map[string]map[string]any),
	}
}

func (e *InMemSearchEngine) IndexDocument(ctx context.Context, indexName, docID string, payload any) error {
	e.mu.Lock()
	if e.store[indexName] == nil {
		e.store[indexName] = make(map[string]any)
	}
	e.store[indexName][docID] = payload
	e.mu.Unlock()
	return nil
}

func (e *InMemSearchEngine) SearchKeyword(ctx context.Context, indexName, query string) ([]map[string]any, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var results []map[string]any
	idx, ok := e.store[indexName]
	if !ok {
		return results, nil
	}
	for id, doc := range idx {
		if strings.Contains(strings.ToLower(id), strings.ToLower(query)) {
			results = append(results, map[string]any{"_id": id, "doc": doc})
		}
	}
	return results, nil
}

// Legacy Engine alias for backwards compatibility
type Client = InMemSearchEngine

func New() *Client { return NewInMemSearchEngine() }
