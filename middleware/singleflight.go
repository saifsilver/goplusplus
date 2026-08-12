package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"net/http"
	"slices"
	"strings"
	"sync"

	gpp "github.com/saifsilver/goplusplus"
)

const defaultSingleflightMaxResponseBytes = 1 << 20

var singleflightSecurityHeaders = []string{
	"Authorization", "Cookie", "Accept", "Accept-Encoding", "Accept-Language",
}

// SingleflightConfig defines response bounds and request-key variation.
type SingleflightConfig struct {
	MaxResponseBytes int                       // Maximum replayable body; defaults to 1 MiB.
	Scope            func(*gpp.Context) string // Optional additional application-specific scope.
	VaryHeaders      []string                  // Additional request headers included in the flight key.
}

type singleflightCall struct {
	done          chan struct{}
	status        int
	header        http.Header
	body          []byte
	requestHeader http.Header
	shareable     bool
}

// Singleflight deduplicates concurrent equivalent GET and HEAD requests within
// one application process. Unsafe, failed, private, and oversized responses are
// never replayed.
func Singleflight(configs ...SingleflightConfig) gpp.HandlerFunc {
	config := normalizeSingleflightConfig(configs)
	var mu sync.Mutex
	inFlight := make(map[string]*singleflightCall)

	return func(c *gpp.Context) error {
		if !singleflightRequestEligible(c.Request) {
			return c.Next()
		}
		key := singleflightKey(c, config)

		mu.Lock()
		if existing := inFlight[key]; existing != nil {
			mu.Unlock()
			select {
			case <-c.Request.Context().Done():
				return c.Request.Context().Err()
			case <-existing.done:
			}
			if !existing.shareable || !singleflightVaryMatches(c.Request.Header, existing) {
				return c.Next()
			}
			return replaySingleflightResponse(c, existing)
		}

		call := &singleflightCall{done: make(chan struct{}), requestHeader: c.Request.Header.Clone()}
		inFlight[key] = call
		mu.Unlock()
		defer func() {
			mu.Lock()
			if inFlight[key] == call {
				delete(inFlight, key)
			}
			mu.Unlock()
			close(call.done)
		}()

		recorder := &responseRecorder{
			ResponseWriter: c.Writer,
			header:         make(http.Header),
			body:           &bytes.Buffer{},
			statusCode:     http.StatusOK,
			maxBodyBytes:   config.MaxResponseBytes,
		}
		c.Writer = recorder
		err := c.Next()
		call.status = recorder.statusCode
		call.header = recorder.header.Clone()
		call.body = slices.Clone(recorder.body.Bytes())
		call.shareable = singleflightResponseShareable(err, recorder)
		return err
	}
}

func normalizeSingleflightConfig(configs []SingleflightConfig) SingleflightConfig {
	config := SingleflightConfig{MaxResponseBytes: defaultSingleflightMaxResponseBytes}
	if len(configs) == 0 {
		return config
	}
	if configs[0].MaxResponseBytes > 0 {
		config.MaxResponseBytes = configs[0].MaxResponseBytes
	}
	config.Scope = configs[0].Scope
	config.VaryHeaders = append([]string(nil), configs[0].VaryHeaders...)
	return config
}

func singleflightRequestEligible(request *http.Request) bool {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		return false
	}
	if request.Header.Get("Upgrade") != "" {
		return false
	}
	return !strings.Contains(strings.ToLower(request.Header.Get("Accept")), "text/event-stream")
}

func singleflightKey(c *gpp.Context, config SingleflightConfig) string {
	hash := sha256.New()
	writeSingleflightKeyPart(hash, c.Request.Method)
	writeSingleflightKeyPart(hash, c.Request.Host)
	writeSingleflightKeyPart(hash, c.Request.URL.RequestURI())
	writeSingleflightKeyPart(hash, c.GetString("tenant_id"))
	writeSingleflightKeyPart(hash, c.GetString("user_id"))
	writeSingleflightKeyPart(hash, c.UserSubject())
	if config.Scope != nil {
		writeSingleflightKeyPart(hash, config.Scope(c))
	}
	headers := append(append([]string(nil), singleflightSecurityHeaders...), config.VaryHeaders...)
	for _, header := range headers {
		canonical := http.CanonicalHeaderKey(header)
		writeSingleflightKeyPart(hash, canonical)
		writeSingleflightKeyPart(hash, strings.Join(c.Request.Header.Values(canonical), "\x00"))
	}
	return string(hash.Sum(nil))
}

type byteWriter interface {
	Write([]byte) (int, error)
}

func writeSingleflightKeyPart(writer byteWriter, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = writer.Write(size[:])
	_, _ = writer.Write([]byte(value))
}

func singleflightResponseShareable(err error, recorder *responseRecorder) bool {
	if err != nil || recorder.overflow || recorder.statusCode < 200 || recorder.statusCode >= 300 {
		return false
	}
	if len(recorder.header.Values("Set-Cookie")) > 0 || varyContainsWildcard(recorder.header) {
		return false
	}
	cacheControl := strings.ToLower(strings.Join(recorder.header.Values("Cache-Control"), ","))
	for _, directive := range strings.Split(cacheControl, ",") {
		switch strings.TrimSpace(strings.SplitN(directive, "=", 2)[0]) {
		case "private", "no-store":
			return false
		}
	}
	return true
}

func varyContainsWildcard(header http.Header) bool {
	for _, line := range header.Values("Vary") {
		for _, field := range strings.Split(line, ",") {
			if strings.TrimSpace(field) == "*" {
				return true
			}
		}
	}
	return false
}

func singleflightVaryMatches(requestHeader http.Header, call *singleflightCall) bool {
	for _, line := range call.header.Values("Vary") {
		for _, field := range strings.Split(line, ",") {
			field = http.CanonicalHeaderKey(strings.TrimSpace(field))
			if field != "" && !slices.Equal(requestHeader.Values(field), call.requestHeader.Values(field)) {
				return false
			}
		}
	}
	return true
}

func replaySingleflightResponse(c *gpp.Context, call *singleflightCall) error {
	copySingleflightHeaders(c.Writer.Header(), call.header)
	c.Writer.Header().Set("X-Singleflight", "HIT")
	c.Status(call.status)
	var err error
	if c.Request.Method != http.MethodHead {
		_, err = c.Writer.Write(call.body)
	}
	c.Abort()
	return err
}

func copySingleflightHeaders(destination, source http.Header) {
	connectionHeaders := make(map[string]struct{})
	for _, line := range source.Values("Connection") {
		for _, field := range strings.Split(line, ",") {
			connectionHeaders[http.CanonicalHeaderKey(strings.TrimSpace(field))] = struct{}{}
		}
	}
	for header, values := range source {
		canonical := http.CanonicalHeaderKey(header)
		if _, hopByHop := connectionHeaders[canonical]; hopByHop ||
			isHopByHopHeader(canonical) || isRequestSpecificResponseHeader(canonical) {
			continue
		}
		destination[canonical] = slices.Clone(values)
	}
}

func isRequestSpecificResponseHeader(header string) bool {
	switch header {
	case "X-Request-Id", "Traceparent", "Tracestate", "Server-Timing":
		return true
	default:
		return false
	}
}

func isHopByHopHeader(header string) bool {
	switch header {
	case "Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization",
		"Te", "Trailer", "Transfer-Encoding", "Upgrade":
		return true
	default:
		return false
	}
}
