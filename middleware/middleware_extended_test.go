package middleware_test

import (
	"net/http"
	"net/http/httptest"
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
