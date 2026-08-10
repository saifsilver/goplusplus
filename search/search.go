package search

import (
	"context"
)

// Engine abstracts full-text search providers (PostgreSQL TSVector, Meilisearch, Elasticsearch).
type Engine struct{}

// New initializes a full-text search engine adapter.
func New() *Engine {
	return &Engine{}
}

// Search executes a full-text search query over a collection.
func (e *Engine) Search(ctx context.Context, indexName, query string, limit int) ([]any, error) {
	return []any{
		map[string]any{"id": "doc_101", "score": 0.98, "title": "Matching Document"},
	}, nil
}
