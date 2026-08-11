package health_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/health"
)

func TestReadinessRedactsFailuresAndBoundsChecks(t *testing.T) {
	checker := health.NewChecker(health.WithTimeout(10 * time.Millisecond))
	if err := checker.RegisterReadinessCheck("database", func(context.Context) error {
		return errors.New("postgres://secret@private-host/app")
	}); err != nil {
		t.Fatal(err)
	}
	if err := checker.RegisterReadinessCheck("slow", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	if err := checker.RegisterReadinessCheck("panic", func(context.Context) error { panic("secret") }); err != nil {
		t.Fatal(err)
	}
	if err := checker.RegisterReadinessCheck("database", func(context.Context) error { return nil }); err == nil {
		t.Fatal("expected duplicate registration error")
	}
	app := gpp.New()
	app.GET("/ready", checker.Readiness())
	start := time.Now()
	recorder := httptest.NewRecorder()
	app.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if recorder.Code != http.StatusServiceUnavailable || time.Since(start) > time.Second {
		t.Fatalf("unexpected readiness result: %d duration=%s", recorder.Code, time.Since(start))
	}
	if strings.Contains(recorder.Body.String(), "secret") || strings.Contains(recorder.Body.String(), "private-host") {
		t.Fatalf("readiness leaked dependency error: %s", recorder.Body.String())
	}
}
