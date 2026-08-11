package tracing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/saifsilver/goplusplus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "github.com/saifsilver/goplusplus/tracing"

var ErrExporterNotConfigured = errors.New("tracing: OTLP exporter endpoint is not configured")

type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	Endpoint       string
	Headers        map[string]string
	Insecure       bool
	SampleRatio    float64
	BatchTimeout   time.Duration
	ExportTimeout  time.Duration
	Exporter       sdktrace.SpanExporter
	Propagator     propagation.TextMapPropagator
}

// Provider owns the OpenTelemetry SDK lifecycle. Applications should call
// Shutdown during graceful termination to flush queued spans.
type Provider struct {
	tracerProvider *sdktrace.TracerProvider
	propagator     propagation.TextMapPropagator
}

func NewProvider(ctx context.Context, config Config) (*Provider, error) {
	if strings.TrimSpace(config.ServiceName) == "" {
		return nil, errors.New("tracing: service name is required")
	}
	exporter := config.Exporter
	if exporter == nil {
		var err error
		exporter, err = newOTLPExporter(ctx, config)
		if err != nil {
			return nil, err
		}
	}
	if config.SampleRatio == 0 {
		config.SampleRatio = 1
	}
	if config.SampleRatio < 0 || config.SampleRatio > 1 {
		return nil, errors.New("tracing: sample ratio must be between 0 and 1")
	}
	if config.BatchTimeout <= 0 {
		config.BatchTimeout = 5 * time.Second
	}
	if config.ExportTimeout <= 0 {
		config.ExportTimeout = 10 * time.Second
	}
	if config.Propagator == nil {
		config.Propagator = propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{}, propagation.Baggage{},
		)
	}
	attributes := []attribute.KeyValue{attribute.String("service.name", config.ServiceName)}
	if config.ServiceVersion != "" {
		attributes = append(attributes, attribute.String("service.version", config.ServiceVersion))
	}
	if config.Environment != "" {
		attributes = append(attributes, attribute.String("deployment.environment.name", config.Environment))
	}
	serviceResource, err := resource.New(ctx, resource.WithAttributes(attributes...))
	if err != nil {
		return nil, fmt.Errorf("tracing: create OpenTelemetry resource: %w", err)
	}
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(serviceResource),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(config.SampleRatio))),
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(config.BatchTimeout),
			sdktrace.WithExportTimeout(config.ExportTimeout),
		),
	)
	return &Provider{tracerProvider: tracerProvider, propagator: config.Propagator}, nil
}

func newOTLPExporter(ctx context.Context, config Config) (sdktrace.SpanExporter, error) {
	endpoint, err := url.Parse(config.Endpoint)
	if err != nil || endpoint.Host == "" || endpoint.User != nil {
		return nil, ErrExporterNotConfigured
	}
	if endpoint.Scheme != "https" && !(config.Insecure && endpoint.Scheme == "http") {
		return nil, errors.New("tracing: OTLP endpoint must use HTTPS")
	}
	options := []otlptracehttp.Option{
		otlptracehttp.WithEndpointURL(endpoint.String()),
		otlptracehttp.WithHeaders(config.Headers),
	}
	if config.Insecure {
		options = append(options, otlptracehttp.WithInsecure())
	}
	exporter, err := otlptracehttp.New(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("tracing: create OTLP exporter: %w", err)
	}
	return exporter, nil
}

func (provider *Provider) Middleware() gpp.HandlerFunc {
	if provider == nil || provider.tracerProvider == nil {
		return failClosedMiddleware()
	}
	return middleware(provider.tracerProvider, provider.propagator)
}

// Middleware instruments requests with the process-wide OpenTelemetry provider.
// Prefer Provider.Middleware when this package owns exporter configuration.
func Middleware() gpp.HandlerFunc {
	return middleware(otel.GetTracerProvider(), otel.GetTextMapPropagator())
}

func middleware(tracerProvider trace.TracerProvider, propagator propagation.TextMapPropagator) gpp.HandlerFunc {
	tracer := tracerProvider.Tracer(instrumentationName)
	return func(c *gpp.Context) error {
		parent := propagator.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		spanName := c.Request.Method + " " + c.Request.URL.Path
		ctx, span := tracer.Start(parent, spanName, trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(
			attribute.String("http.request.method", c.Request.Method),
			attribute.String("url.path", c.Request.URL.Path),
			attribute.String("server.address", c.Request.Host),
		))
		defer span.End()
		c.Request = c.Request.WithContext(ctx)
		traceID := span.SpanContext().TraceID().String()
		if span.SpanContext().TraceID().IsValid() {
			c.Set("trace_id", traceID)
			c.SetHeader("X-Trace-ID", traceID)
		}
		err := c.Next()
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "request failed")
		}
		return err
	}
}

func failClosedMiddleware() gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		return errors.New("tracing: provider is not configured")
	}
}

func (provider *Provider) ForceFlush(ctx context.Context) error {
	if provider == nil || provider.tracerProvider == nil {
		return nil
	}
	return provider.tracerProvider.ForceFlush(ctx)
}

func (provider *Provider) Shutdown(ctx context.Context) error {
	if provider == nil || provider.tracerProvider == nil {
		return nil
	}
	return provider.tracerProvider.Shutdown(ctx)
}

// GetTraceID extracts the standards-compliant active OpenTelemetry trace ID.
func GetTraceID(c *gpp.Context) string {
	if c != nil && c.Request != nil {
		spanContext := trace.SpanContextFromContext(c.Request.Context())
		if spanContext.TraceID().IsValid() {
			return spanContext.TraceID().String()
		}
	}
	if c != nil {
		if value, ok := c.Get("trace_id"); ok {
			if traceID, ok := value.(string); ok && traceID != "" {
				return traceID
			}
		}
	}
	return "untraced"
}

// Inject writes W3C trace context and baggage headers for an outgoing request.
func (provider *Provider) Inject(ctx context.Context, header http.Header) {
	if provider == nil || provider.propagator == nil {
		return
	}
	provider.propagator.Inject(ctx, propagation.HeaderCarrier(header))
}
