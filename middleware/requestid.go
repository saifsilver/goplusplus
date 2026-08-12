package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/saifsilver/goplusplus"
)

// RequestIDHeader is the HTTP request correlation header.
const RequestIDHeader = "X-Request-ID"

// ContextRequestIDKey is the gpp context key containing the validated request ID.
const ContextRequestIDKey = "request_id"

// RequestID returns middleware that generates or propagates X-Request-ID HTTP headers across requests and context.
func RequestID() gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		reqID := c.GetHeader(RequestIDHeader)
		if reqID == "" {
			reqID = generateRequestID()
		}
		if !validRequestID(reqID) {
			reqID = generateRequestID()
		}

		c.Set(ContextRequestIDKey, reqID)
		c.SetHeader(RequestIDHeader, reqID)
		return c.Next()
	}
}

var fallbackRequestIDCounter atomic.Uint64

func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), fallbackRequestIDCounter.Add(1))
	}
	return hex.EncodeToString(b)
}

func validRequestID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || character == '-' || character == '_' || character == '.' || character == ':' {
			continue
		}
		return false
	}
	return true
}

// SystemTimeNowNano is retained for source compatibility.
func SystemTimeNowNano() int64 {
	return time.Now().UnixNano()
}
