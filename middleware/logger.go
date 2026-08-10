package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"go++"
)

// responseWriterInterceptor intercepts status codes for logging.
type responseWriterInterceptor struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriterInterceptor) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Logger creates a structured request logger middleware using stdlib slog.
func Logger() gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		start := time.Now()
		path := c.Request.URL.Path
		rawQuery := c.Request.URL.RawQuery
		method := c.Request.Method
		clientIP := c.Request.RemoteAddr

		interceptor := &responseWriterInterceptor{
			ResponseWriter: c.Writer,
			statusCode:     http.StatusOK,
		}
		c.Writer = interceptor

		err := c.Next()

		latency := time.Since(start)
		status := interceptor.statusCode

		if rawQuery != "" {
			path = path + "?" + rawQuery
		}

		logAttrs := []any{
			slog.String("method", method),
			slog.String("path", path),
			slog.Int("status", status),
			slog.Duration("latency", latency),
			slog.String("ip", clientIP),
		}

		if err != nil {
			logAttrs = append(logAttrs, slog.String("error", err.Error()))
			slog.Error("HTTP Request Error", logAttrs...)
		} else if status >= 500 {
			slog.Error("HTTP Server Error", logAttrs...)
		} else if status >= 400 {
			slog.Warn("HTTP Client Warning", logAttrs...)
		} else {
			slog.Info("HTTP Request Handled", logAttrs...)
		}

		return err
	}
}
