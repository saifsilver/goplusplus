package middleware

import (
	"net"
	"net/http"
	"strings"
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
	MaxClients      int           // Maximum number of client buckets retained in memory
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
	if config.MaxClients <= 0 {
		config.MaxClients = 100000
	}

	var mu sync.Mutex
	buckets := make(map[string]*clientBucket)
	lastCleanup := time.Now()

	return func(c *gpp.Context) error {
		clientIP := remoteIP(c.Request.RemoteAddr)
		now := time.Now()

		mu.Lock()
		if now.Sub(lastCleanup) >= config.CleanupInterval {
			removeInactiveBuckets(buckets, now, config.CleanupInterval)
			lastCleanup = now
		}
		b, exists := buckets[clientIP]
		if !exists {
			if len(buckets) >= config.MaxClients {
				removeOldestBucket(buckets)
			}
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

func remoteIP(remoteAddr string) string {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	if remoteAddr == "" {
		return "unknown"
	}
	return remoteAddr
}

func removeInactiveBuckets(buckets map[string]*clientBucket, now time.Time, interval time.Duration) {
	for clientIP, bucket := range buckets {
		if now.Sub(bucket.lastUpdate) > interval {
			delete(buckets, clientIP)
		}
	}
}

func removeOldestBucket(buckets map[string]*clientBucket) {
	var oldestIP string
	var oldestUpdate time.Time
	for clientIP, bucket := range buckets {
		if oldestIP == "" || bucket.lastUpdate.Before(oldestUpdate) {
			oldestIP = clientIP
			oldestUpdate = bucket.lastUpdate
		}
	}
	delete(buckets, oldestIP)
}
