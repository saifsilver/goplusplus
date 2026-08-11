package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/saifsilver/goplusplus"
)

// Recovery returns a panic recovery middleware that catches unexpected runtime panics.
func Recovery() gpp.HandlerFunc {
	return func(c *gpp.Context) (err error) {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				slog.Error("CRITICAL: Panic Recovered in HTTP Handler",
					slog.Any("panic", r),
					slog.String("stack", string(stack)),
					slog.String("path", c.Request.URL.Path),
					slog.String("request_id", c.RequestID()),
				)
				c.Abort()
				_ = c.Problem(gpp.ProblemDetails{
					Type: "https://goplusplus.dev/errors/internal-error", Title: "Internal Server Error",
					Status: http.StatusInternalServerError, Detail: "An internal server error occurred",
					Instance: c.Request.URL.Path, TraceID: c.RequestID(),
				})
				err = gpp.ErrInternal("panic recovered")
			}
		}()
		return c.Next()
	}
}
