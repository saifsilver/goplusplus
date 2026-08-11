package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/middleware"
)

func TestRequestIDAndObservability(t *testing.T) {
	app := gpp.New()
	app.Use(
		middleware.RequestID(),
		middleware.Observability(),
		middleware.Logger(),
	)

	app.GET("/metrics", middleware.Prometheus())

	app.GET("/ping", func(c *gpp.Context) error {
		if c.RequestID() == "" {
			t.Errorf("expected RequestID to be populated")
		}
		return c.String(http.StatusOK, "pong")
	})

	reqPing := httptest.NewRequest(http.MethodGet, "/ping", nil)
	wPing := httptest.NewRecorder()
	app.ServeHTTP(wPing, reqPing)

	if wPing.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", wPing.Code)
	}
	if wPing.Header().Get("X-Request-ID") == "" {
		t.Errorf("expected X-Request-ID header to be populated")
	}

	reqMetrics := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	wMetrics := httptest.NewRecorder()
	app.ServeHTTP(wMetrics, reqMetrics)

	if wMetrics.Code != http.StatusOK {
		t.Errorf("expected /metrics status 200 OK, got %d", wMetrics.Code)
	}
}

func TestIdempotencyAndSingleflight(t *testing.T) {
	app := gpp.New()
	app.Use(
		middleware.Idempotency(),
		middleware.Singleflight(),
	)

	counter := 0
	app.POST("/charge", func(c *gpp.Context) error {
		counter++
		return c.JSON(http.StatusOK, gpp.H{"charge_count": counter})
	})

	// Request 1 with Idempotency-Key
	req1 := httptest.NewRequest(http.MethodPost, "/charge", nil)
	req1.Header.Set("Idempotency-Key", "key_abc123")
	w1 := httptest.NewRecorder()
	app.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", w1.Code)
	}

	// Request 2 with SAME Idempotency-Key (should return cached response, counter stays 1)
	req2 := httptest.NewRequest(http.MethodPost, "/charge", nil)
	req2.Header.Set("Idempotency-Key", "key_abc123")
	w2 := httptest.NewRecorder()
	app.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("expected 200 OK for cached idempotency request, got %d", w2.Code)
	}
	if counter != 1 {
		t.Errorf("expected counter to be 1 due to idempotency, got %d", counter)
	}
}
