package tracing

import (
	"fmt"
	"time"

	"github.com/saifsilver/goplusplus"
)

// Middleware returns OpenTelemetry tracing middleware injecting X-Trace-ID into context and response headers.
func Middleware() gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		traceID := c.GetHeader("X-Trace-ID")
		if traceID == "" {
			traceID = fmt.Sprintf("trace_%d", time.Now().UnixNano())
		}
		c.Set("trace_id", traceID)
		c.SetHeader("X-Trace-ID", traceID)
		return c.Next()
	}
}

// GetTraceID extracts the active trace ID from context.
func GetTraceID(c *gpp.Context) string {
	if val, ok := c.Get("trace_id"); ok {
		if id, ok := val.(string); ok {
			return id
		}
	}
	return "untraced"
}
