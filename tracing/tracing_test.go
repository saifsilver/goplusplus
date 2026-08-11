package tracing_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/tracing"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTracingMiddlewareCreatesAndExportsServerSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider, err := tracing.NewProvider(context.Background(), tracing.Config{
		ServiceName: "test-api", Exporter: exporter,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Shutdown(context.Background())
	app := gpp.New()
	app.Use(provider.Middleware())

	var extractedTraceID string
	app.GET("/traced", func(c *gpp.Context) error {
		extractedTraceID = tracing.GetTraceID(c)
		return c.String(http.StatusOK, "ok")
	})

	request := httptest.NewRequest(http.MethodGet, "/traced", nil)
	request.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if err := provider.ForceFlush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if extractedTraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace ID = %s", extractedTraceID)
	}
	if response.Header().Get("X-Trace-ID") != extractedTraceID {
		t.Fatalf("response trace ID = %s", response.Header().Get("X-Trace-ID"))
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Name != "GET /traced" {
		t.Fatalf("exported spans = %#v", spans)
	}
	if spans[0].Parent.SpanID().String() != "00f067aa0ba902b7" {
		t.Fatalf("parent span ID = %s", spans[0].Parent.SpanID())
	}
}

func TestTracingProviderConfigurationFailsClosed(t *testing.T) {
	if _, err := tracing.NewProvider(context.Background(), tracing.Config{}); err == nil {
		t.Fatal("expected missing service name to fail")
	}
	if _, err := tracing.NewProvider(context.Background(), tracing.Config{ServiceName: "api"}); err == nil {
		t.Fatal("expected missing exporter to fail")
	}
	app := gpp.New()
	var provider *tracing.Provider
	app.Use(provider.Middleware())
	app.GET("/", func(c *gpp.Context) error { return c.NoContent() })
	response := httptest.NewRecorder()
	app.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code < 500 {
		t.Fatalf("status = %d", response.Code)
	}
}
