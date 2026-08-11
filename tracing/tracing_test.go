package tracing_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/tracing"
)

func TestTracingMiddleware(t *testing.T) {
	app := gpp.New()
	app.Use(tracing.Middleware())

	var extractedTraceID string
	app.GET("/traced", func(c *gpp.Context) error {
		extractedTraceID = tracing.GetTraceID(c)
		return c.String(http.StatusOK, "ok")
	})

	// Case 1: Provided X-Trace-ID
	req1 := httptest.NewRequest(http.MethodGet, "/traced", nil)
	req1.Header.Set("X-Trace-ID", "trace_abc123")
	w1 := httptest.NewRecorder()
	app.ServeHTTP(w1, req1)

	if extractedTraceID != "trace_abc123" {
		t.Errorf("expected trace_abc123, got %s", extractedTraceID)
	}
	if w1.Header().Get("X-Trace-ID") != "trace_abc123" {
		t.Errorf("expected header X-Trace-ID trace_abc123")
	}

	// Case 2: Generated X-Trace-ID
	req2 := httptest.NewRequest(http.MethodGet, "/traced", nil)
	w2 := httptest.NewRecorder()
	app.ServeHTTP(w2, req2)

	if extractedTraceID == "" || extractedTraceID == "untraced" {
		t.Errorf("expected generated trace ID, got %s", extractedTraceID)
	}
}
