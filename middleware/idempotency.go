package middleware

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"time"

	"github.com/saifsilver/goplusplus"
)

const IdempotencyHeader = "Idempotency-Key"

type IdempotencyConfig struct {
	TTL              time.Duration             // Cached response lifetime; defaults to 24 hours.
	PendingTTL       time.Duration             // In-flight claim lifetime; defaults to 30 seconds.
	PendingPoll      time.Duration             // Distributed claim poll interval; defaults to 25 milliseconds.
	MaxEntries       int                       // Maximum completed responses retained; defaults to 10,000.
	MaxKeyBytes      int                       // Maximum Idempotency-Key size; defaults to 256 bytes.
	MaxRequestBytes  int64                     // Maximum request body fingerprinted; defaults to 1 MiB.
	MaxResponseBytes int                       // Maximum response body cached; defaults to 1 MiB.
	Scope            func(*gpp.Context) string // Optional tenant/principal scope; defaults to tenant_id and sub.
	Store            IdempotencyStore          // Optional distributed store; defaults to bounded process memory.
}

// Idempotency returns middleware caching and replaying responses for requests with an Idempotency-Key header.
func Idempotency(cfg ...IdempotencyConfig) gpp.HandlerFunc {
	config := IdempotencyConfig{
		TTL:              24 * time.Hour,
		PendingTTL:       30 * time.Second,
		PendingPoll:      25 * time.Millisecond,
		MaxEntries:       10000,
		MaxKeyBytes:      256,
		MaxRequestBytes:  1 << 20,
		MaxResponseBytes: 1 << 20,
	}
	if len(cfg) > 0 {
		if cfg[0].TTL > 0 {
			config.TTL = cfg[0].TTL
		}
		if cfg[0].PendingTTL > 0 {
			config.PendingTTL = cfg[0].PendingTTL
		}
		if cfg[0].PendingPoll > 0 {
			config.PendingPoll = cfg[0].PendingPoll
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
		config.Store = cfg[0].Store
	}
	if config.Store == nil {
		config.Store = NewMemoryIdempotencyStore(config.MaxEntries)
	}

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
		fingerprintBytes, err := idempotencyFingerprint(c.Request, config.MaxRequestBytes)
		if err != nil {
			return err
		}
		fingerprint := hex.EncodeToString(fingerprintBytes[:])
		cacheKey := scopedIdempotencyKey(c, key, config.Scope)
		owner, err := newIdempotencyOwner()
		if err != nil {
			return gpp.NewInternalError("idempotency.owner", err)
		}
		for {
			claim, err := config.Store.Claim(c.Request.Context(), cacheKey, fingerprint, owner, config.PendingTTL)
			if err != nil {
				if errors.Is(err, ErrIdempotencyStoreCapacity) {
					return idempotencyUnavailable()
				}
				return idempotencyStoreError("idempotency.claim", err)
			}
			if claim.Fingerprint != fingerprint {
				return gpp.ErrConflict("Idempotency-Key was already used with a different request")
			}
			if claim.Acquired {
				break
			}
			if claim.State == IdempotencyComplete {
				if claim.Response == nil {
					return idempotencyStoreError("idempotency.replay", errors.New("completed claim has no response"))
				}
				return replayIdempotentResponse(c, *claim.Response)
			}
			if err := waitForIdempotencyClaim(c.Request.Context(), config.PendingPoll); err != nil {
				return err
			}
		}

		rec := newIdempotencyResponseBuffer(c.Writer, config.MaxResponseBytes)
		c.Writer = rec

		finished := false
		defer func() {
			if finished {
				return
			}
			if releaseErr := releaseIdempotencyClaim(config.Store, cacheKey, owner); releaseErr != nil {
				slog.Error("idempotency: failed to release interrupted claim", slog.Any("error", releaseErr))
			}
		}()

		err = c.Next()
		if err != nil || rec.overflow || rec.statusCode < 200 || rec.statusCode >= 300 {
			releaseErr := releaseIdempotencyClaim(config.Store, cacheKey, owner)
			finished = true
			if releaseErr != nil {
				slog.Error("idempotency: failed to release claim", slog.Any("error", releaseErr))
			}
			if flushErr := rec.FlushResponse(); flushErr != nil {
				return errors.Join(err, releaseErr, flushErr)
			}
			return errors.Join(err, releaseErr)
		}
		response := IdempotencyResponse{
			Status: rec.statusCode, Header: rec.header.Clone(), Body: slices.Clone(rec.body.Bytes()),
		}
		if err := config.Store.Complete(
			c.Request.Context(), cacheKey, fingerprint, owner, response, config.TTL,
		); err != nil {
			finished = true
			slog.Error("idempotency: failed to persist completed response", slog.Any("error", err))
			if releaseErr := releaseIdempotencyClaim(config.Store, cacheKey, owner); releaseErr != nil {
				slog.Error("idempotency: failed to release unpersisted claim", slog.Any("error", releaseErr))
			}
			c.Writer = rec.ResponseWriter
			return writeIdempotencyStoreFailure(c)
		}
		finished = true
		return rec.FlushResponse()
	}
}

func newIdempotencyOwner() (string, error) {
	var owner [16]byte
	if _, err := rand.Read(owner[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(owner[:]), nil
}

func waitForIdempotencyClaim(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func releaseIdempotencyClaim(store IdempotencyStore, key, owner string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return store.Release(ctx, key, owner)
}

func idempotencyStoreError(operation string, err error) error {
	return gpp.NewInternalError(operation, err, gpp.WithErrorCategory("idempotency_store"))
}

func idempotencyUnavailable() *gpp.ProblemDetails {
	return &gpp.ProblemDetails{
		Type: "https://goplusplus.dev/errors/idempotency/unavailable", Title: "Service Unavailable",
		Status: http.StatusServiceUnavailable, Detail: "Idempotency service is temporarily unavailable",
	}
}

func writeIdempotencyStoreFailure(c *gpp.Context) error {
	return c.Problem(gpp.ProblemDetails{
		Type: "https://goplusplus.dev/errors/idempotency_store", Title: "Internal Server Error",
		Status: http.StatusInternalServerError, Detail: "An internal server error occurred",
		Instance: c.Request.URL.Path, TraceID: c.RequestID(),
	})
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
	raw := c.Request.Method + "\x00" + c.Request.URL.RequestURI() + "\x00" + requestScope + "\x00" + key
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}

func replayIdempotentResponse(c *gpp.Context, cached IdempotencyResponse) error {
	for header, values := range cached.Header {
		c.Writer.Header()[header] = slices.Clone(values)
	}
	c.Writer.Header().Set("X-Cache", "HIT-IDEMPOTENT")
	c.Status(cached.Status)
	_, err := c.Writer.Write(cached.Body)
	c.Abort()
	return err
}

type idempotencyResponseBuffer struct {
	http.ResponseWriter
	header       http.Header
	body         bytes.Buffer
	statusCode   int
	wroteHeader  bool
	maxBodyBytes int
	overflow     bool
	flushed      bool
}

func newIdempotencyResponseBuffer(writer http.ResponseWriter, maxBodyBytes int) *idempotencyResponseBuffer {
	return &idempotencyResponseBuffer{
		ResponseWriter: writer,
		header:         make(http.Header),
		statusCode:     http.StatusOK,
		maxBodyBytes:   maxBodyBytes,
	}
}

func (r *idempotencyResponseBuffer) Header() http.Header { return r.header }

func (r *idempotencyResponseBuffer) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.statusCode = code
	r.wroteHeader = true
}

func (r *idempotencyResponseBuffer) Write(data []byte) (int, error) {
	if !r.wroteHeader {
		r.wroteHeader = true
	}
	if r.flushed {
		return r.ResponseWriter.Write(data)
	}
	if r.maxBodyBytes <= 0 || r.body.Len()+len(data) <= r.maxBodyBytes {
		return r.body.Write(data)
	}
	r.overflow = true
	if err := r.FlushResponse(); err != nil {
		return 0, err
	}
	return r.ResponseWriter.Write(data)
}

func (r *idempotencyResponseBuffer) FlushResponse() error {
	if r.flushed {
		return nil
	}
	for header, values := range r.header {
		r.ResponseWriter.Header()[header] = slices.Clone(values)
	}
	r.ResponseWriter.WriteHeader(r.statusCode)
	r.flushed = true
	if r.body.Len() == 0 {
		return nil
	}
	_, err := r.ResponseWriter.Write(r.body.Bytes())
	r.body.Reset()
	return err
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

func (r *responseRecorder) Flush() {
	r.overflow = true
	if !r.wroteHeader {
		r.WriteHeader(r.statusCode)
	}
	_ = http.NewResponseController(r.ResponseWriter).Flush()
}

func (r *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	r.overflow = true
	return http.NewResponseController(r.ResponseWriter).Hijack()
}

func (r *responseRecorder) Push(target string, options *http.PushOptions) error {
	if pusher, ok := r.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, options)
	}
	return http.ErrNotSupported
}

func (r *responseRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
