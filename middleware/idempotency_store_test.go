package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/middleware"
)

func TestIdempotencyStoreContract(t *testing.T) {
	factories := map[string]func(*testing.T) middleware.IdempotencyStore{
		"memory": func(*testing.T) middleware.IdempotencyStore {
			return middleware.NewMemoryIdempotencyStore(8)
		},
		"redis": newTestRedisIdempotencyStore,
	}
	for name, factory := range factories {
		t.Run(name, func(t *testing.T) {
			store := factory(t)
			ctx := context.Background()
			const ttl = time.Minute

			claim, err := store.Claim(ctx, "key", "fingerprint", "owner-1", ttl)
			if err != nil || !claim.Acquired || claim.State != middleware.IdempotencyPending {
				t.Fatalf("initial claim = %#v, %v", claim, err)
			}
			pending, err := store.Claim(ctx, "key", "fingerprint", "owner-2", ttl)
			if err != nil || pending.Acquired || pending.State != middleware.IdempotencyPending {
				t.Fatalf("pending claim = %#v, %v", pending, err)
			}
			if err := store.Complete(ctx, "key", "fingerprint", "owner-2", middleware.IdempotencyResponse{}, ttl); !errors.Is(err, middleware.ErrIdempotencyOwnershipLost) {
				t.Fatalf("wrong-owner completion error = %v", err)
			}

			response := middleware.IdempotencyResponse{
				Status: http.StatusCreated, Header: http.Header{"X-Test": {"stored"}}, Body: []byte("created"),
			}
			if err := store.Complete(ctx, "key", "fingerprint", "owner-1", response, ttl); err != nil {
				t.Fatal(err)
			}
			response.Header.Set("X-Test", "mutated")
			response.Body[0] = 'X'
			replay, err := store.Claim(ctx, "key", "fingerprint", "owner-3", ttl)
			if err != nil || replay.State != middleware.IdempotencyComplete || replay.Response == nil {
				t.Fatalf("completed claim = %#v, %v", replay, err)
			}
			if replay.Response.Status != http.StatusCreated || replay.Response.Header.Get("X-Test") != "stored" || string(replay.Response.Body) != "created" {
				t.Fatalf("stored response was mutated: %#v", replay.Response)
			}

			other, err := store.Claim(ctx, "released", "fingerprint", "owner-1", ttl)
			if err != nil || !other.Acquired {
				t.Fatalf("release setup = %#v, %v", other, err)
			}
			if err := store.Release(ctx, "released", "owner-1"); err != nil {
				t.Fatal(err)
			}
			reacquired, err := store.Claim(ctx, "released", "fingerprint", "owner-2", ttl)
			if err != nil || !reacquired.Acquired {
				t.Fatalf("reacquired claim = %#v, %v", reacquired, err)
			}
		})
	}
}

func TestMemoryIdempotencyStoreDoesNotEvictPendingClaims(t *testing.T) {
	store := middleware.NewMemoryIdempotencyStore(1)
	ctx := context.Background()
	if _, err := store.Claim(ctx, "first", "fingerprint", "owner", time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(ctx, "second", "fingerprint", "owner", time.Minute); !errors.Is(err, middleware.ErrIdempotencyStoreCapacity) {
		t.Fatalf("capacity error = %v", err)
	}
}

func TestRedisIdempotencyCoordinatesApplicationInstances(t *testing.T) {
	server := miniredis.RunT(t)
	clientA := redis.NewClient(&redis.Options{Addr: server.Addr()})
	clientB := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = clientA.Close()
		_ = clientB.Close()
	})
	storeA, err := middleware.NewRedisIdempotencyStore(clientA, middleware.RedisIdempotencyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	storeB, err := middleware.NewRedisIdempotencyStore(clientB, middleware.RedisIdempotencyConfig{})
	if err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	var executions atomic.Int32
	newApp := func(store middleware.IdempotencyStore) *gpp.Engine {
		app := gpp.New()
		app.Use(middleware.Idempotency(middleware.IdempotencyConfig{Store: store, PendingPoll: time.Millisecond}))
		app.POST("/payments", func(c *gpp.Context) error {
			executions.Add(1)
			once.Do(func() { close(started) })
			<-release
			return c.String(http.StatusCreated, "payment-created")
		})
		return app
	}
	appA, appB := newApp(storeA), newApp(storeB)

	type result struct {
		status int
		body   string
		header http.Header
	}
	request := func(app *gpp.Engine) result {
		req := httptest.NewRequest(http.MethodPost, "/payments", strings.NewReader(`{"amount":10}`))
		req.Header.Set(middleware.IdempotencyHeader, "payment-key")
		recorder := httptest.NewRecorder()
		app.ServeHTTP(recorder, req)
		return result{status: recorder.Code, body: recorder.Body.String(), header: recorder.Header()}
	}
	results := make(chan result, 2)
	go func() { results <- request(appA) }()
	<-started
	go func() { results <- request(appB) }()
	time.Sleep(20 * time.Millisecond)
	if got := executions.Load(); got != 1 {
		t.Fatalf("concurrent handler executions = %d, want 1", got)
	}
	close(release)
	first, second := <-results, <-results
	for _, got := range []result{first, second} {
		if got.status != http.StatusCreated || got.body != "payment-created" {
			t.Fatalf("response = %d %q", got.status, got.body)
		}
	}
	if first.header.Get("X-Cache") != "HIT-IDEMPOTENT" && second.header.Get("X-Cache") != "HIT-IDEMPOTENT" {
		t.Fatal("neither response was identified as an idempotent replay")
	}
	if got := executions.Load(); got != 1 {
		t.Fatalf("handler executions = %d, want 1", got)
	}
}

func TestIdempotencyDoesNotSendSuccessWhenCompletionFails(t *testing.T) {
	store := &failingCompletionStore{}
	app := gpp.New()
	app.Use(middleware.Idempotency(middleware.IdempotencyConfig{Store: store}))
	app.POST("/payments", func(c *gpp.Context) error {
		return c.String(http.StatusCreated, "payment-created")
	})

	req := httptest.NewRequest(http.MethodPost, "/payments", nil)
	req.Header.Set(middleware.IdempotencyHeader, "payment-key")
	recorder := httptest.NewRecorder()
	app.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "payment-created") || strings.Contains(recorder.Body.String(), "redis unavailable") {
		t.Fatalf("response leaked success or internal cause: %q", recorder.Body.String())
	}
	if !store.released.Load() {
		t.Fatal("unpersisted claim was not released")
	}
}

func newTestRedisIdempotencyStore(t *testing.T) middleware.IdempotencyStore {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, err := middleware.NewRedisIdempotencyStore(client, middleware.RedisIdempotencyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

type failingCompletionStore struct {
	released atomic.Bool
}

func (*failingCompletionStore) Claim(_ context.Context, _, fingerprint, _ string, _ time.Duration) (middleware.IdempotencyClaim, error) {
	return middleware.IdempotencyClaim{
		Acquired: true, State: middleware.IdempotencyPending, Fingerprint: fingerprint,
	}, nil
}

func (*failingCompletionStore) Complete(context.Context, string, string, string, middleware.IdempotencyResponse, time.Duration) error {
	return errors.New("redis unavailable")
}

func (store *failingCompletionStore) Release(context.Context, string, string) error {
	store.released.Store(true)
	return nil
}
