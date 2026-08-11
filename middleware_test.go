package gpp_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/middleware"
)

func TestSecurityMiddleware(t *testing.T) {
	app := gpp.New()
	app.Use(middleware.Security())

	app.GET("/secure", func(c *gpp.Context) error {
		return c.String(http.StatusOK, "secure")
	})

	req := httptest.NewRequest(http.MethodGet, "/secure", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("expected X-Frame-Options DENY, got %s", w.Header().Get("X-Frame-Options"))
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("expected X-Content-Type-Options nosniff, got %s", w.Header().Get("X-Content-Type-Options"))
	}
}

func TestCORSMiddleware(t *testing.T) {
	app := gpp.New()
	app.Use(middleware.CORS())

	app.GET("/cors", func(c *gpp.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodOptions, "/cors", nil)
	req.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for preflight, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("expected Access-Control-Allow-Origin *, got %s", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	app := gpp.New()
	app.Use(middleware.Recovery())

	app.GET("/panic", func(c *gpp.Context) error {
		panic("simulated fatal error")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500 on panic recovery, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "simulated fatal error") || strings.Contains(w.Body.String(), "stack") {
		t.Fatalf("panic details leaked to client: %s", w.Body.String())
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	app := gpp.New()
	// Rate limit: capacity 2, rate 1/sec
	app.Use(middleware.RateLimit(middleware.RateLimiterConfig{
		Rate:     1,
		Capacity: 2,
	}))

	app.GET("/limited", func(c *gpp.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	// Request 1: allowed
	req1 := httptest.NewRequest(http.MethodGet, "/limited", nil)
	req1.RemoteAddr = "192.168.1.1:1234"
	w1 := httptest.NewRecorder()
	app.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected request 1 status 200, got %d", w1.Code)
	}

	// Request 2: allowed
	req2 := httptest.NewRequest(http.MethodGet, "/limited", nil)
	req2.RemoteAddr = "192.168.1.1:1234"
	w2 := httptest.NewRecorder()
	app.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected request 2 status 200, got %d", w2.Code)
	}

	// Request 3: rate limited
	req3 := httptest.NewRequest(http.MethodGet, "/limited", nil)
	req3.RemoteAddr = "192.168.1.1:1234"
	w3 := httptest.NewRecorder()
	app.ServeHTTP(w3, req3)
	if w3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected request 3 status 429 Too Many Requests, got %d", w3.Code)
	}
}

func TestRateLimitUsesClientIPWithoutSourcePort(t *testing.T) {
	app := gpp.New()
	app.Use(middleware.RateLimit(middleware.RateLimiterConfig{Rate: 0.0001, Capacity: 2}))
	app.GET("/limited", func(c *gpp.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	for index, remoteAddr := range []string{
		"192.168.1.10:1001",
		"192.168.1.10:1002",
		"192.168.1.10:1003",
	} {
		request := httptest.NewRequest(http.MethodGet, "/limited", nil)
		request.RemoteAddr = remoteAddr
		response := httptest.NewRecorder()
		app.ServeHTTP(response, request)
		want := http.StatusOK
		if index == 2 {
			want = http.StatusTooManyRequests
		}
		if response.Code != want {
			t.Fatalf("request %d status = %d, want %d", index+1, response.Code, want)
		}
	}
}

func TestTimeoutMiddleware(t *testing.T) {
	app := gpp.New()
	app.Use(middleware.Timeout(50 * time.Millisecond))

	app.GET("/slow", func(c *gpp.Context) error {
		time.Sleep(100 * time.Millisecond)
		return c.String(http.StatusOK, "done")
	})

	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504 Gateway Timeout, got %d", w.Code)
	}
}

func TestTimeoutMiddlewarePreservesHandlerErrors(t *testing.T) {
	app := gpp.New()
	app.Use(middleware.Timeout(time.Second))
	app.GET("/error", func(c *gpp.Context) error {
		return gpp.ErrInternal("database detail")
	})
	request := httptest.NewRequest(http.MethodGet, "/error", nil)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "database detail") {
		t.Fatalf("timeout middleware corrupted error response: %d %s", response.Code, response.Body.String())
	}
}
