package resilience_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/resilience"
)

func TestCircuitBreakerTripping(t *testing.T) {
	cb := resilience.NewCircuitBreaker(resilience.CircuitBreakerConfig{
		FailureThreshold: 2,
		ResetTimeout:     50 * time.Millisecond,
	})

	dummyErr := errors.New("downstream database timeout")

	// Attempt 1 fails
	_ = cb.Execute(func() error { return dummyErr })
	// Attempt 2 fails (trips breaker to OPEN)
	_ = cb.Execute(func() error { return dummyErr })

	// Attempt 3 should be rejected immediately because state is OPEN
	err := cb.Execute(func() error { return nil })
	if err == nil {
		t.Errorf("expected circuit breaker to reject execution when OPEN")
	}

	// Wait reset timeout to transition to HalfOpen
	time.Sleep(60 * time.Millisecond)

	// Attempt 4 in HalfOpen succeeds, closing the circuit breaker
	errHalf := cb.Execute(func() error { return nil })
	if errHalf != nil {
		t.Errorf("expected half-open execution to succeed and reset breaker, got %v", errHalf)
	}
}

func TestAdaptiveLimiter(t *testing.T) {
	limiter := resilience.NewAdaptiveLimiter(1)

	app := gpp.New()
	app.Use(limiter.Middleware())

	app.GET("/concurrency", func(c *gpp.Context) error {
		time.Sleep(20 * time.Millisecond)
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/concurrency", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 OK under limit, got %d", w.Code)
	}
}

func TestRetryWithBackoff(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	err := resilience.Retry(ctx, resilience.RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 5 * time.Millisecond,
		MaxDelay:     20 * time.Millisecond,
	}, func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("transient error")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("expected retry to succeed on 3rd attempt, got %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}
