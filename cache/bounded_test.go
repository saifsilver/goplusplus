package cache_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
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

	if val, ok := cacheGet(t, store, ctx, "k1"); !ok || val != "v1" {
		t.Errorf("expected k1 = v1, got %v", val)
	}

	// Adding k4 should evict k1
	_ = store.Set(ctx, "k4", "v4", time.Minute)

	if _, ok := cacheGet(t, store, ctx, "k1"); ok {
		t.Errorf("expected k1 to be evicted when max capacity exceeded")
	}

	if val, ok := cacheGet(t, store, ctx, "k4"); !ok || val != "v4" {
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

	if _, ok := cacheGet(t, store, ctx, "k1"); ok {
		t.Errorf("expected k1 to be expired")
	}
	if val, ok := cacheGet(t, store, ctx, "k2"); !ok || val != "v2" {
		t.Errorf("expected k2 to be preserved, got %v", val)
	}
}

func TestBoundedMemoryStoreDeletePreservesCapacity(t *testing.T) {
	ctx := context.Background()
	store := cache.NewBoundedMemoryStore(2)

	_ = store.Set(ctx, "k1", "v1", time.Minute)
	_ = store.Set(ctx, "k2", "v2", time.Minute)
	_ = store.Delete(ctx, "k1")
	_ = store.Set(ctx, "k3", "v3", time.Minute)
	_ = store.Set(ctx, "k4", "v4", time.Minute)

	if _, ok := cacheGet(t, store, ctx, "k2"); ok {
		t.Fatal("expected oldest live entry k2 to be evicted")
	}
	for _, key := range []string{"k3", "k4"} {
		if _, ok := cacheGet(t, store, ctx, key); !ok {
			t.Fatalf("expected %s to remain cached", key)
		}
	}
}

func TestCacheGetOrSetCoalescesConcurrentFetches(t *testing.T) {
	tests := map[string]cache.Store{
		"memory":     cache.NewMemoryStore(),
		"bounded":    cache.NewBoundedMemoryStore(10),
		"multilevel": cache.NewMultiLevelStore(cache.NewMemoryStore(), cache.NewMemoryStore()),
	}
	for name, store := range tests {
		t.Run(name, func(t *testing.T) {
			var fetches atomic.Int32
			started := make(chan struct{})
			release := make(chan struct{})
			var once sync.Once
			fetcher := func() (any, error) {
				fetches.Add(1)
				once.Do(func() { close(started) })
				<-release
				return "value", nil
			}

			const callers = 8
			var wg sync.WaitGroup
			wg.Add(callers)
			for range callers {
				go func() {
					defer wg.Done()
					value, err := store.GetOrSet(context.Background(), "shared", time.Minute, fetcher)
					if err != nil || value != "value" {
						t.Errorf("GetOrSet() = %v, %v", value, err)
					}
				}()
			}
			<-started
			time.Sleep(10 * time.Millisecond)
			close(release)
			wg.Wait()

			if got := fetches.Load(); got != 1 {
				t.Fatalf("fetcher called %d times, want 1", got)
			}
		})
	}
}

func TestBoundedMemoryStorePrefix(t *testing.T) {
	ctx := context.Background()
	store := cache.NewBoundedMemoryStore(10)

	_ = store.Set(ctx, "user:1", "Alice", time.Minute)
	_ = store.Set(ctx, "user:2", "Bob", time.Minute)
	_ = store.Set(ctx, "post:1", "Hello World", time.Minute)

	_ = store.InvalidatePrefix(ctx, "user:")

	if _, ok := cacheGet(t, store, ctx, "user:1"); ok {
		t.Errorf("user:1 should be invalidated")
	}
	if val, ok := cacheGet(t, store, ctx, "post:1"); !ok || val != "Hello World" {
		t.Errorf("post:1 should still exist, got %v", val)
	}

	_ = store.Delete(ctx, "post:1")
	if _, ok := cacheGet(t, store, ctx, "post:1"); ok {
		t.Errorf("post:1 should be deleted")
	}

	val, err := store.GetOrSet(ctx, "computed", time.Minute, func() (any, error) {
		return "computed_val", nil
	})
	if err != nil || val != "computed_val" {
		t.Errorf("GetOrSet failed: %v", err)
	}
}

func TestMemoryStoreAndMultiLevelStore(t *testing.T) {
	ctx := context.Background()
	memStore := cache.NewMemoryStore()
	l2Store := cache.NewMemoryStore()

	_ = memStore.Set(ctx, "key1", "val1", time.Minute)
	if val, ok := cacheGet(t, memStore, ctx, "key1"); !ok || val != "val1" {
		t.Errorf("expected key1 = val1")
	}

	_ = memStore.Delete(ctx, "key1")
	if _, ok := cacheGet(t, memStore, ctx, "key1"); ok {
		t.Errorf("key1 should be deleted")
	}

	val, err := memStore.GetOrSet(ctx, "fetch_key", time.Minute, func() (any, error) {
		return "fetched", nil
	})
	if err != nil || val != "fetched" {
		t.Errorf("GetOrSet failed")
	}

	_ = memStore.InvalidatePrefix(ctx, "fetch_")

	// MultiLevelStore
	multi := cache.NewMultiLevelStore(memStore, l2Store)
	_ = multi.Set(ctx, "multi_key", "multi_val", time.Minute)
	if val, ok := cacheGet(t, multi, ctx, "multi_key"); !ok || val != "multi_val" {
		t.Errorf("MultiLevelStore Get failed")
	}
	_ = multi.Delete(ctx, "multi_key")
	_ = multi.InvalidatePrefix(ctx, "multi_")

	_, _ = multi.GetOrSet(ctx, "multi_fetch", time.Minute, func() (any, error) {
		return "multi_fetched", nil
	})

	legacyClient := cache.NewClient()
	_ = legacyClient.Set(ctx, "legacy", "123", time.Minute)

}

func TestMultiLevelBackfillDoesNotOutliveL2(t *testing.T) {
	ctx := context.Background()
	l1 := cache.NewMemoryStore()
	l2 := cache.NewMemoryStore()
	multi := cache.NewMultiLevelStore(l1, l2)
	if err := l2.Set(ctx, "short-lived", "value", 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if value, found := cacheGet(t, multi, ctx, "short-lived"); !found || value != "value" {
		t.Fatalf("initial Get = %v, %v", value, found)
	}
	time.Sleep(30 * time.Millisecond)
	if _, found := cacheGet(t, multi, ctx, "short-lived"); found {
		t.Fatal("L1 served a value after its L2 entry expired")
	}
}

func ExampleBoundedMemoryStore() {
	ctx := context.Background()
	store := cache.NewBoundedMemoryStore(100)
	_ = store.Set(ctx, "query:123", "result_data", 5*time.Minute)
	val, _, _ := store.Get(ctx, "query:123")
	fmt.Println(val)
	// Output: result_data
}

func cacheGet(t *testing.T, store cache.Store, ctx context.Context, key string) (any, bool) {
	t.Helper()
	value, found, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get(%q): %v", key, err)
	}
	return value, found
}
