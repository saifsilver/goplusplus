package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/saifsilver/goplusplus"
)

// Timeout returns middleware that sets a context timeout for each request.
func Timeout(duration time.Duration) gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		ctx, cancel := context.WithTimeout(c.Request.Context(), duration)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)

		done := make(chan error, 1)
		go func() {
			done <- c.Next()
		}()

		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			c.Abort()
			return c.JSON(http.StatusGatewayTimeout, gpp.H{
				"code":    http.StatusGatewayTimeout,
				"message": "Request Timeout - Operation exceeded maximum allowed duration",
			})
		}
	}
}
