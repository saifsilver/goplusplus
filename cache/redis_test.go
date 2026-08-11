package cache_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/saifsilver/goplusplus/cache"
)

func TestRedisStoreContract(t *testing.T) {
	server, store := newRedisStore(t, "contract:")
	ctx := context.Background()

	if err := store.Set(ctx, "string", "value", time.Minute); err != nil {
		t.Fatalf("Set string: %v", err)
	}
	if value, found := cacheGet(t, store, ctx, "string"); !found || value != "value" {
		t.Fatalf("Get string = %v, %v", value, found)
	}

	if err := store.Set(ctx, "object", map[string]any{"count": 2}, time.Minute); err != nil {
		t.Fatalf("Set object: %v", err)
	}
	value, found := cacheGet(t, store, ctx, "object")
	object, ok := value.(map[string]any)
	if !found || !ok || object["count"] != float64(2) {
		t.Fatalf("Get object = %#v, %v", value, found)
	}

	server.FastForward(time.Minute + time.Second)
	if _, found := cacheGet(t, store, ctx, "string"); found {
		t.Fatal("expired value remained in Redis")
	}

	if err := store.Set(ctx, "delete", "value", time.Minute); err != nil {
		t.Fatalf("Set delete: %v", err)
	}
	if err := store.Delete(ctx, "delete"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, found := cacheGet(t, store, ctx, "delete"); found {
		t.Fatal("deleted value remained in Redis")
	}
}

func TestRedisStorePrefixInvalidationEscapesPatterns(t *testing.T) {
	_, store := newRedisStore(t, "prefix:")
	ctx := context.Background()
	for _, key := range []string{"user:*:one", "user:*:two", "user:x:three", "other"} {
		if err := store.Set(ctx, key, key, time.Minute); err != nil {
			t.Fatalf("Set(%q): %v", key, err)
		}
	}

	if err := store.InvalidatePrefix(ctx, "user:*"); err != nil {
		t.Fatalf("InvalidatePrefix: %v", err)
	}
	for _, key := range []string{"user:*:one", "user:*:two"} {
		if _, found := cacheGet(t, store, ctx, key); found {
			t.Fatalf("literal wildcard prefix did not remove %q", key)
		}
	}
	for _, key := range []string{"user:x:three", "other"} {
		if _, found := cacheGet(t, store, ctx, key); !found {
			t.Fatalf("prefix invalidation removed unrelated key %q", key)
		}
	}
}

func TestRedisStoreGetOrSetCoalescesLocalFetches(t *testing.T) {
	_, store := newRedisStore(t, "singleflight:")
	var fetches atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once

	const callers = 8
	var wait sync.WaitGroup
	wait.Add(callers)
	for range callers {
		go func() {
			defer wait.Done()
			value, err := store.GetOrSet(context.Background(), "shared", time.Minute, func() (any, error) {
				fetches.Add(1)
				once.Do(func() { close(started) })
				<-release
				return "value", nil
			})
			if err != nil || value != "value" {
				t.Errorf("GetOrSet = %v, %v", value, err)
			}
		}()
	}
	<-started
	close(release)
	wait.Wait()
	if got := fetches.Load(); got != 1 {
		t.Fatalf("fetcher called %d times, want 1", got)
	}
}

func TestRedisStoreSurfacesConnectionFailures(t *testing.T) {
	server, store := newRedisStore(t, "failure:")
	server.Close()

	if _, _, err := store.Get(context.Background(), "key"); err == nil {
		t.Fatal("Redis connection failure was reported as a cache miss")
	}
}

func TestRedisStoreRequiresExplicitURL(t *testing.T) {
	if _, err := cache.NewRedisStore(context.Background(), cache.RedisConfig{}); err == nil {
		t.Fatal("empty Redis URL was accepted")
	}
}

func newRedisStore(t *testing.T, prefix string) (*miniredis.Miniredis, *cache.RedisStore) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	store, err := cache.NewRedisStoreFromClient(context.Background(), client, cache.RedisConfig{Prefix: prefix})
	if err != nil {
		t.Fatalf("NewRedisStoreFromClient: %v", err)
	}
	return server, store
}
