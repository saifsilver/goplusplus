package middleware

import (
	"time"

	gpp "github.com/saifsilver/goplusplus"
)

// Timeout returns middleware that sets a context timeout for each request.
func Timeout(duration time.Duration) gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		return c.RunWithTimeout(duration)
	}
}
