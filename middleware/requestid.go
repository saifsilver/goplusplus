package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/saifsilver/goplusplus"
)

const RequestIDHeader = "X-Request-ID"
const ContextRequestIDKey = "request_id"

// RequestID returns middleware that generates or propagates X-Request-ID HTTP headers across requests and context.
func RequestID() gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		reqID := c.GetHeader(RequestIDHeader)
		if reqID == "" {
			reqID = generateRequestID()
		}

		c.Set(ContextRequestIDKey, reqID)
		c.SetHeader(RequestIDHeader, reqID)
		return c.Next()
	}
}

func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("req_%d", SystemTimeNowNano())
	}
	return hex.EncodeToString(b)
}

func SystemTimeNowNano() int64 {
	return 1700000000000000000
}
