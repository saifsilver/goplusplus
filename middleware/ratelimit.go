package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/saifsilver/goplusplus"
)

type clientBucket struct {
	tokens     float64
	lastUpdate time.Time
}

// RateLimiterConfig defines rate limiting capacity and refill parameters.
type RateLimiterConfig struct {
	Rate            float64       // Tokens added per second
	Capacity        float64       // Maximum token bucket capacity
	CleanupInterval time.Duration // Interval to clean inactive client buckets
}

// RateLimit returns a thread-safe token bucket rate limiting middleware by client IP address.
func RateLimit(config RateLimiterConfig) gpp.HandlerFunc {
	if config.Rate <= 0 {
		config.Rate = 10
	}
	if config.Capacity <= 0 {
		config.Capacity = 20
	}
	if config.CleanupInterval <= 0 {
		config.CleanupInterval = 5 * time.Minute
	}

	var mu sync.Mutex
	buckets := make(map[string]*clientBucket)

	// Periodic cleanup routine for stale IP buckets
	go func() {
		ticker := time.NewTicker(config.CleanupInterval)
		for range ticker.C {
			mu.Lock()
			now := time.Now()
			for ip, b := range buckets {
				if now.Sub(b.lastUpdate) > config.CleanupInterval {
					delete(buckets, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(c *gpp.Context) error {
		clientIP := c.Request.RemoteAddr
		now := time.Now()

		mu.Lock()
		b, exists := buckets[clientIP]
		if !exists {
			b = &clientBucket{
				tokens:     config.Capacity - 1, // Consume 1 token immediately
				lastUpdate: now,
			}
			buckets[clientIP] = b
			mu.Unlock()
			return c.Next()
		}

		// Calculate refilled tokens
		elapsed := now.Sub(b.lastUpdate).Seconds()
		b.tokens += elapsed * config.Rate
		if b.tokens > config.Capacity {
			b.tokens = config.Capacity
		}
		b.lastUpdate = now

		if b.tokens >= 1 {
			b.tokens -= 1
			mu.Unlock()
			return c.Next()
		}
		mu.Unlock()

		c.Abort()
		return c.JSON(http.StatusTooManyRequests, gpp.H{
			"code":    http.StatusTooManyRequests,
			"message": "Too Many Requests - Rate limit exceeded. Please try again later.",
		})
	}
}
