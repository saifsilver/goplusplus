package health_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/health"
)

func TestHealthCheckerLivenessAndReadiness(t *testing.T) {
	checker := health.NewChecker()

	checker.AddReadinessCheck("db", func(ctx context.Context) error {
		return nil
	})

	app := gpp.New()
	app.GET("/healthz/liveness", checker.Liveness())
	app.GET("/healthz/readiness", checker.Readiness())

	// Test Liveness
	reqLive := httptest.NewRequest(http.MethodGet, "/healthz/liveness", nil)
	wLive := httptest.NewRecorder()
	app.ServeHTTP(wLive, reqLive)

	if wLive.Code != http.StatusOK {
		t.Errorf("expected liveness 200 OK, got %d", wLive.Code)
	}

	// Test Healthy Readiness
	reqReady := httptest.NewRequest(http.MethodGet, "/healthz/readiness", nil)
	wReady := httptest.NewRecorder()
	app.ServeHTTP(wReady, reqReady)

	if wReady.Code != http.StatusOK {
		t.Errorf("expected readiness 200 OK, got %d", wReady.Code)
	}

	// Add failing readiness check
	checker.AddReadinessCheck("cache", func(ctx context.Context) error {
		return errors.New("connection failed")
	})

	reqReady2 := httptest.NewRequest(http.MethodGet, "/healthz/readiness", nil)
	wReady2 := httptest.NewRecorder()
	app.ServeHTTP(wReady2, reqReady2)

	if wReady2.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 Service Unavailable when dependency is down, got %d", wReady2.Code)
	}
}
