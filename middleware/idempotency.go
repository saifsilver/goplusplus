package middleware

import (
	"bytes"
	"crypto/sha256"
	"io"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/saifsilver/goplusplus"
)

const IdempotencyHeader = "Idempotency-Key"

type cachedResponse struct {
	Status      int
	Header      http.Header
	Body        []byte
	Fingerprint [sha256.Size]byte
	Created     time.Time
}

type IdempotencyConfig struct {
	TTL              time.Duration             // Cached response lifetime; defaults to 24 hours.
	MaxEntries       int                       // Maximum completed responses retained; defaults to 10,000.
	MaxKeyBytes      int                       // Maximum Idempotency-Key size; defaults to 256 bytes.
	MaxRequestBytes  int64                     // Maximum request body fingerprinted; defaults to 1 MiB.
	MaxResponseBytes int                       // Maximum response body cached; defaults to 1 MiB.
	Scope            func(*gpp.Context) string // Optional tenant/principal scope; defaults to tenant_id and sub.
}

type idempotencyCall struct {
	ready       chan struct{}
	fingerprint [sha256.Size]byte
}

// Idempotency returns middleware caching and replaying responses for requests with an Idempotency-Key header.
func Idempotency(cfg ...IdempotencyConfig) gpp.HandlerFunc {
	config := IdempotencyConfig{
		TTL:              24 * time.Hour,
		MaxEntries:       10000,
		MaxKeyBytes:      256,
		MaxRequestBytes:  1 << 20,
		MaxResponseBytes: 1 << 20,
	}
	if len(cfg) > 0 {
		if cfg[0].TTL > 0 {
			config.TTL = cfg[0].TTL
		}
		if cfg[0].MaxEntries > 0 {
			config.MaxEntries = cfg[0].MaxEntries
		}
		if cfg[0].MaxKeyBytes > 0 {
			config.MaxKeyBytes = cfg[0].MaxKeyBytes
		}
		if cfg[0].MaxRequestBytes > 0 {
			config.MaxRequestBytes = cfg[0].MaxRequestBytes
		}
		if cfg[0].MaxResponseBytes > 0 {
			config.MaxResponseBytes = cfg[0].MaxResponseBytes
		}
		config.Scope = cfg[0].Scope
	}

	var mu sync.RWMutex
	cache := make(map[string]cachedResponse)
	order := make([]string, 0, config.MaxEntries)
	inFlight := make(map[string]idempotencyCall)

	return func(c *gpp.Context) error {
		if c.Request.Method != http.MethodPost && c.Request.Method != http.MethodPut && c.Request.Method != http.MethodPatch && c.Request.Method != http.MethodDelete {
			return c.Next()
		}

		key := c.GetHeader(IdempotencyHeader)
		if key == "" {
			return c.Next()
		}
		if len(key) > config.MaxKeyBytes {
			return gpp.ErrBadRequest("Idempotency-Key exceeds the maximum allowed size")
		}
		fingerprint, err := idempotencyFingerprint(c.Request, config.MaxRequestBytes)
		if err != nil {
			return err
		}
		cacheKey := scopedIdempotencyKey(c, key, config.Scope)

		var ready chan struct{}
		for {
			mu.Lock()
			cached, found := cache[cacheKey]
			if found && time.Since(cached.Created) >= config.TTL {
				order = removeIdempotencyEntry(cache, order, cacheKey)
				found = false
			}
			if found {
				mu.Unlock()
				if cached.Fingerprint != fingerprint {
					return gpp.ErrConflict("Idempotency-Key was already used with a different request")
				}
				return replayIdempotentResponse(c, cached)
			}
			if pending, exists := inFlight[cacheKey]; exists {
				mu.Unlock()
				if pending.fingerprint != fingerprint {
					return gpp.ErrConflict("Idempotency-Key is already in use by a different request")
				}
				select {
				case <-c.Request.Context().Done():
					return c.Request.Context().Err()
				case <-pending.ready:
					continue
				}
			}
			ready = make(chan struct{})
			inFlight[cacheKey] = idempotencyCall{ready: ready, fingerprint: fingerprint}
			mu.Unlock()
			break
		}

		rec := &responseRecorder{
			ResponseWriter: c.Writer,
			header:         make(http.Header),
			body:           &bytes.Buffer{},
			statusCode:     http.StatusOK,
			maxBodyBytes:   config.MaxResponseBytes,
		}
		c.Writer = rec

		finished := false
		defer func() {
			if finished {
				return
			}
			mu.Lock()
			delete(inFlight, cacheKey)
			close(ready)
			mu.Unlock()
		}()

		err = c.Next()

		mu.Lock()
		if err == nil && !rec.overflow && rec.statusCode >= 200 && rec.statusCode < 300 {
			if _, exists := cache[cacheKey]; !exists {
				for len(cache) >= config.MaxEntries {
					order = evictOldestIdempotencyEntry(cache, order)
				}
				order = append(order, cacheKey)
			}
			cache[cacheKey] = cachedResponse{
				Status:      rec.statusCode,
				Header:      rec.header.Clone(),
				Body:        slices.Clone(rec.body.Bytes()),
				Fingerprint: fingerprint,
				Created:     time.Now(),
			}
		}
		delete(inFlight, cacheKey)
		close(ready)
		finished = true
		mu.Unlock()

		return err
	}
}

func idempotencyFingerprint(request *http.Request, maxBytes int64) ([sha256.Size]byte, error) {
	if request.ContentLength > maxBytes {
		return [sha256.Size]byte{}, idempotencyBodyTooLarge()
	}
	if request.Body == nil {
		request.Body = http.NoBody
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, maxBytes+1))
	_ = request.Body.Close()
	if err != nil {
		return [sha256.Size]byte{}, gpp.ErrBadRequest("Idempotency request body could not be read")
	}
	if int64(len(body)) > maxBytes {
		return [sha256.Size]byte{}, idempotencyBodyTooLarge()
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.ContentLength = int64(len(body))
	payload := append([]byte(request.Header.Get("Content-Type")+"\x00"), body...)
	return sha256.Sum256(payload), nil
}

func idempotencyBodyTooLarge() *gpp.ProblemDetails {
	return &gpp.ProblemDetails{
		Type:   "https://goplusplus.dev/errors/idempotency/body-too-large",
		Title:  "Request body too large",
		Status: http.StatusRequestEntityTooLarge,
		Detail: "Idempotency request body exceeds the maximum allowed size",
	}
}

func scopedIdempotencyKey(c *gpp.Context, key string, scope func(*gpp.Context) string) string {
	requestScope := c.GetString("tenant_id") + "\x00" + c.UserSubject()
	if scope != nil {
		requestScope = scope(c)
	}
	return c.Request.Method + "\x00" + c.Request.URL.RequestURI() + "\x00" + requestScope + "\x00" + key
}

func replayIdempotentResponse(c *gpp.Context, cached cachedResponse) error {
	for header, values := range cached.Header {
		c.Writer.Header()[header] = slices.Clone(values)
	}
	c.Writer.Header().Set("X-Cache", "HIT-IDEMPOTENT")
	c.Status(cached.Status)
	_, err := c.Writer.Write(cached.Body)
	c.Abort()
	return err
}

func removeIdempotencyEntry(cache map[string]cachedResponse, order []string, key string) []string {
	delete(cache, key)
	for index, candidate := range order {
		if candidate == key {
			return append(order[:index], order[index+1:]...)
		}
	}
	return order
}

func evictOldestIdempotencyEntry(cache map[string]cachedResponse, order []string) []string {
	if len(order) == 0 {
		return order
	}
	delete(cache, order[0])
	return order[1:]
}

type responseRecorder struct {
	http.ResponseWriter
	header       http.Header
	body         *bytes.Buffer
	statusCode   int
	wroteHeader  bool
	maxBodyBytes int
	overflow     bool
}

func (r *responseRecorder) Header() http.Header {
	return r.header
}

func (r *responseRecorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.statusCode = code
	r.commitHeader()
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.commitHeader()
	}
	if !r.overflow && (r.maxBodyBytes <= 0 || r.body.Len()+len(b) <= r.maxBodyBytes) {
		_, _ = r.body.Write(b)
	} else {
		r.overflow = true
	}
	return r.ResponseWriter.Write(b)
}

func (r *responseRecorder) commitHeader() {
	for header, values := range r.header {
		r.ResponseWriter.Header()[header] = slices.Clone(values)
	}
	r.wroteHeader = true
}
