package search_test

import (
	"context"
	"testing"

	"github.com/saifsilver/goplusplus/search"
)

func TestInMemSearchEngine(t *testing.T) {
	engine := search.NewInMemSearchEngine()
	ctx := context.Background()

	_ = engine.IndexDocument(ctx, "products", "laptop_macbook", map[string]any{"title": "MacBook Pro"})
	_ = engine.IndexDocument(ctx, "products", "phone_iphone", map[string]any{"title": "iPhone 15"})

	res, err := engine.SearchKeyword(ctx, "products", "macbook")
	if err != nil {
		t.Fatalf("SearchKeyword failed: %v", err)
	}
	if len(res) != 1 {
		t.Errorf("expected 1 result, got %d", len(res))
	}

	legacyClient := search.New()
	_ = legacyClient.IndexDocument(ctx, "default", "doc_1", "test")
	resLegacy, _ := legacyClient.Search(ctx, "doc")
	if len(resLegacy) != 1 {
		t.Errorf("expected 1 legacy search result, got %d", len(resLegacy))
	}
}
