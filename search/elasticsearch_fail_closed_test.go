package search

import (
	"context"
	"errors"
	"testing"
)

func TestUnconfiguredElasticsearchFailsClosed(t *testing.T) {
	client := (*ESClient)(nil)
	if err := client.IndexDocument(context.Background(), "index", "id", map[string]any{}); !errors.Is(err, ErrElasticsearchNotConfigured) {
		t.Fatalf("IndexDocument error = %v", err)
	}
	if _, err := client.SearchKeyword(context.Background(), "index", "query"); !errors.Is(err, ErrElasticsearchNotConfigured) {
		t.Fatalf("SearchKeyword error = %v", err)
	}
	if _, err := NewElasticsearchClient(context.Background(), ESConfig{}); !errors.Is(err, ErrElasticsearchNotConfigured) {
		t.Fatalf("constructor error = %v", err)
	}
}
