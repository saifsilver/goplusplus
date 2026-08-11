package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/middleware"
)

func TestRequestIDMiddleware(t *testing.T) {
	app := gpp.New()
	app.Use(middleware.RequestID())

	var capturedID string
	app.GET("/test", func(c *gpp.Context) error {
		capturedID = c.RequestID()
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if capturedID == "" {
		t.Error("expected non-empty captured request_id in context")
	}
	if w.Header().Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID response header")
	}
}

func TestIdempotencyMiddleware(t *testing.T) {
	app := gpp.New()
	app.Use(middleware.Idempotency())

	var executions int32
	app.POST("/pay", func(c *gpp.Context) error {
		atomic.AddInt32(&executions, 1)
		return c.JSON(http.StatusOK, gpp.H{"status": "paid"})
	})

	req1 := httptest.NewRequest(http.MethodPost, "/pay", nil)
	req1.Header.Set("Idempotency-Key", "idemp_key_100")
	w1 := httptest.NewRecorder()
	app.ServeHTTP(w1, req1)

	req2 := httptest.NewRequest(http.MethodPost, "/pay", nil)
	req2.Header.Set("Idempotency-Key", "idemp_key_100")
	w2 := httptest.NewRecorder()
	app.ServeHTTP(w2, req2)

	if atomic.LoadInt32(&executions) != 1 {
		t.Fatalf("expected handler execution count to be 1, got %d", executions)
	}
	if w2.Header().Get("X-Cache") != "HIT-IDEMPOTENT" {
		t.Errorf("expected X-Cache HIT-IDEMPOTENT header on replayed request")
	}
}

func TestIdempotencyScopesKeysByRoute(t *testing.T) {
	app := gpp.New()
	app.Use(middleware.Idempotency())

	var payments atomic.Int32
	var refunds atomic.Int32
	app.POST("/pay", func(c *gpp.Context) error {
		payments.Add(1)
		return c.String(http.StatusOK, "paid")
	})
	app.POST("/refund", func(c *gpp.Context) error {
		refunds.Add(1)
		return c.String(http.StatusOK, "refunded")
	})

	for path, want := range map[string]string{"/pay": "paid", "/refund": "refunded"} {
		request := httptest.NewRequest(http.MethodPost, path, nil)
		request.Header.Set(middleware.IdempotencyHeader, "shared-key")
		response := httptest.NewRecorder()
		app.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Body.String() != want {
			t.Fatalf("POST %s = %d %q, want 200 %q", path, response.Code, response.Body.String(), want)
		}
	}
	if payments.Load() != 1 || refunds.Load() != 1 {
		t.Fatalf("handler executions = pay:%d refund:%d, want 1 each", payments.Load(), refunds.Load())
	}
}

func TestIdempotencyCoalescesConcurrentRequests(t *testing.T) {
	app := gpp.New()
	app.Use(middleware.Idempotency())

	var executions atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	app.POST("/pay", func(c *gpp.Context) error {
		if executions.Add(1) == 1 {
			close(started)
		}
		<-release
		return c.String(http.StatusOK, "paid")
	})

	const callers = 8
	var wg sync.WaitGroup
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			request := httptest.NewRequest(http.MethodPost, "/pay", strings.NewReader("amount=10"))
			request.Header.Set(middleware.IdempotencyHeader, "concurrent-key")
			response := httptest.NewRecorder()
			app.ServeHTTP(response, request)
			if response.Code != http.StatusOK || response.Body.String() != "paid" {
				t.Errorf("response = %d %q", response.Code, response.Body.String())
			}
		}()
	}
	<-started
	time.Sleep(10 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := executions.Load(); got != 1 {
		t.Fatalf("handler executed %d times, want 1", got)
	}
}

func TestIdempotencyCacheIsBounded(t *testing.T) {
	app := gpp.New()
	app.Use(middleware.Idempotency(middleware.IdempotencyConfig{MaxEntries: 1}))

	var executions atomic.Int32
	app.POST("/pay", func(c *gpp.Context) error {
		executions.Add(1)
		return c.String(http.StatusOK, "paid")
	})

	for _, key := range []string{"first", "second", "first"} {
		request := httptest.NewRequest(http.MethodPost, "/pay", nil)
		request.Header.Set(middleware.IdempotencyHeader, key)
		app.ServeHTTP(httptest.NewRecorder(), request)
	}
	if got := executions.Load(); got != 3 {
		t.Fatalf("handler executed %d times, want 3 after oldest entry eviction", got)
	}
}

func TestIdempotencyRejectsKeyReuseWithDifferentRequest(t *testing.T) {
	app := gpp.New()
	app.Use(middleware.Idempotency())

	var executions atomic.Int32
	app.POST("/pay", func(c *gpp.Context) error {
		executions.Add(1)
		return c.String(http.StatusOK, "paid")
	})

	for index, body := range []string{"amount=10", "amount=20"} {
		request := httptest.NewRequest(http.MethodPost, "/pay", strings.NewReader(body))
		request.Header.Set(middleware.IdempotencyHeader, "reused-key")
		response := httptest.NewRecorder()
		app.ServeHTTP(response, request)
		want := http.StatusOK
		if index == 1 {
			want = http.StatusConflict
		}
		if response.Code != want {
			t.Fatalf("request %d status = %d, want %d", index+1, response.Code, want)
		}
	}
	if got := executions.Load(); got != 1 {
		t.Fatalf("handler executed %d times, want 1", got)
	}
}

func TestIdempotencyScopesKeysByPrincipal(t *testing.T) {
	app := gpp.New()
	app.Use(func(c *gpp.Context) error {
		c.Set("sub", c.GetHeader("X-Test-Subject"))
		return c.Next()
	}, middleware.Idempotency())

	var executions atomic.Int32
	app.POST("/pay", func(c *gpp.Context) error {
		executions.Add(1)
		return c.String(http.StatusOK, "%s", c.UserSubject())
	})

	for _, subject := range []string{"user-a", "user-b"} {
		request := httptest.NewRequest(http.MethodPost, "/pay", nil)
		request.Header.Set(middleware.IdempotencyHeader, "shared-key")
		request.Header.Set("X-Test-Subject", subject)
		response := httptest.NewRecorder()
		app.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Body.String() != subject {
			t.Fatalf("subject %s response = %d %q", subject, response.Code, response.Body.String())
		}
	}
	if got := executions.Load(); got != 2 {
		t.Fatalf("handler executed %d times, want 2", got)
	}
}

func TestSingleflightMiddleware(t *testing.T) {
	app := gpp.New()
	app.Use(middleware.Singleflight())

	var handlerExecutions int32
	app.GET("/heavy", func(c *gpp.Context) error {
		atomic.AddInt32(&handlerExecutions, 1)
		time.Sleep(50 * time.Millisecond)
		return c.JSON(http.StatusOK, gpp.H{"data": "heavy_result"})
	})

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/heavy", nil)
			w := httptest.NewRecorder()
			app.ServeHTTP(w, req)
		}()
	}
	wg.Wait()

	if atomic.LoadInt32(&handlerExecutions) != 1 {
		t.Fatalf("expected singleflight to execute handler exactly 1 time, got %d", handlerExecutions)
	}
}
