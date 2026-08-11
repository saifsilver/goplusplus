package middleware

import (
	"bytes"
	"net/http"
	"sync"
	"time"

	"github.com/saifsilver/goplusplus"
)

const IdempotencyHeader = "Idempotency-Key"

type cachedResponse struct {
	Status  int
	Header  http.Header
	Body    []byte
	Created time.Time
}

type IdempotencyConfig struct {
	TTL time.Duration
}

// Idempotency returns middleware caching and replaying responses for requests with an Idempotency-Key header.
func Idempotency(cfg ...IdempotencyConfig) gpp.HandlerFunc {
	ttl := 24 * time.Hour
	if len(cfg) > 0 && cfg[0].TTL > 0 {
		ttl = cfg[0].TTL
	}

	var mu sync.RWMutex
	cache := make(map[string]cachedResponse)

	return func(c *gpp.Context) error {
		if c.Request.Method != http.MethodPost && c.Request.Method != http.MethodPut && c.Request.Method != http.MethodPatch && c.Request.Method != http.MethodDelete {
			return c.Next()
		}

		key := c.GetHeader(IdempotencyHeader)
		if key == "" {
			return c.Next()
		}

		mu.RLock()
		cached, found := cache[key]
		mu.RUnlock()

		if found && time.Since(cached.Created) < ttl {
			for h, vals := range cached.Header {
				for _, v := range vals {
					c.Writer.Header().Add(h, v)
				}
			}
			c.Writer.Header().Set("X-Cache", "HIT-IDEMPOTENT")
			c.Status(cached.Status)
			_, err := c.Writer.Write(cached.Body)
			c.Abort()
			return err
		}

		rec := &responseRecorder{
			ResponseWriter: c.Writer,
			header:         make(http.Header),
			body:           &bytes.Buffer{},
			statusCode:     http.StatusOK,
		}
		c.Writer = rec

		err := c.Next()

		if rec.statusCode >= 200 && rec.statusCode < 300 {
			mu.Lock()
			cache[key] = cachedResponse{
				Status:  rec.statusCode,
				Header:  rec.header.Clone(),
				Body:    rec.body.Bytes(),
				Created: time.Now(),
			}
			mu.Unlock()
		}

		return err
	}
}

type responseRecorder struct {
	http.ResponseWriter
	header     http.Header
	body       *bytes.Buffer
	statusCode int
}

func (r *responseRecorder) Header() http.Header {
	return r.header
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}
