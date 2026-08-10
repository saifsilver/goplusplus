package middleware

import (
	"fmt"
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
				)
				c.Abort()
				_ = c.JSON(http.StatusInternalServerError, gpp.H{
					"code":    http.StatusInternalServerError,
					"message": "Internal Server Error",
					"error":   fmt.Sprintf("%v", r),
				})
				err = fmt.Errorf("panic: %v", r)
			}
		}()
		return c.Next()
	}
}
