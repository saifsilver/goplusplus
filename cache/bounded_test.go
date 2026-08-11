package cache_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/saifsilver/goplusplus/cache"
)

func TestBoundedMemoryStoreCapacity(t *testing.T) {
	ctx := context.Background()
	store := cache.NewBoundedMemoryStore(3)

	_ = store.Set(ctx, "k1", "v1", time.Minute)
	_ = store.Set(ctx, "k2", "v2", time.Minute)
	_ = store.Set(ctx, "k3", "v3", time.Minute)

	if val, ok := store.Get(ctx, "k1"); !ok || val != "v1" {
		t.Errorf("expected k1 = v1, got %v", val)
	}

	// Adding k4 should evict k1
	_ = store.Set(ctx, "k4", "v4", time.Minute)

	if _, ok := store.Get(ctx, "k1"); ok {
		t.Errorf("expected k1 to be evicted when max capacity exceeded")
	}

	if val, ok := store.Get(ctx, "k4"); !ok || val != "v4" {
		t.Errorf("expected k4 = v4, got %v", val)
	}
}

func TestBoundedMemoryStoreExpirationEviction(t *testing.T) {
	ctx := context.Background()
	store := cache.NewBoundedMemoryStore(2)

	_ = store.Set(ctx, "k1", "v1", 10*time.Millisecond)
	_ = store.Set(ctx, "k2", "v2", time.Minute)

	time.Sleep(20 * time.Millisecond)

	// k1 is expired, so adding k3 should clean k1
	_ = store.Set(ctx, "k3", "v3", time.Minute)

	if _, ok := store.Get(ctx, "k1"); ok {
		t.Errorf("expected k1 to be expired")
	}
	if val, ok := store.Get(ctx, "k2"); !ok || val != "v2" {
		t.Errorf("expected k2 to be preserved, got %v", val)
	}
}

func TestBoundedMemoryStorePrefix(t *testing.T) {
	ctx := context.Background()
	store := cache.NewBoundedMemoryStore(10)

	_ = store.Set(ctx, "user:1", "Alice", time.Minute)
	_ = store.Set(ctx, "user:2", "Bob", time.Minute)
	_ = store.Set(ctx, "post:1", "Hello World", time.Minute)

	_ = store.InvalidatePrefix(ctx, "user:")

	if _, ok := store.Get(ctx, "user:1"); ok {
		t.Errorf("user:1 should be invalidated")
	}
	if val, ok := store.Get(ctx, "post:1"); !ok || val != "Hello World" {
		t.Errorf("post:1 should still exist, got %v", val)
	}
}

func ExampleBoundedMemoryStore() {
	ctx := context.Background()
	store := cache.NewBoundedMemoryStore(100)
	_ = store.Set(ctx, "query:123", "result_data", 5*time.Minute)
	val, _ := store.Get(ctx, "query:123")
	fmt.Println(val)
	// Output: result_data
}
