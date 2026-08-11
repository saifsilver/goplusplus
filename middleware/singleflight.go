package middleware

import (
	"bytes"
	"net/http"
	"sync"

	"github.com/saifsilver/goplusplus"
)

type call struct {
	wg     sync.WaitGroup
	status int
	header http.Header
	body   []byte
	err    error
}

// Singleflight returns middleware that deduplicates concurrent identical GET HTTP requests, executing the handler ONCE.
func Singleflight() gpp.HandlerFunc {
	var mu sync.Mutex
	inFlight := make(map[string]*call)

	return func(c *gpp.Context) error {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			return c.Next()
		}

		key := c.Request.Method + ":" + c.Request.URL.RequestURI()

		mu.Lock()
		if cCall, exists := inFlight[key]; exists {
			mu.Unlock()
			cCall.wg.Wait()

			for h, vals := range cCall.header {
				for _, v := range vals {
					c.Writer.Header().Add(h, v)
				}
			}
			c.Writer.Header().Set("X-Singleflight", "HIT")
			c.Status(cCall.status)
			_, err := c.Writer.Write(cCall.body)
			c.Abort()
			return err
		}

		cCall := new(call)
		cCall.wg.Add(1)
		inFlight[key] = cCall
		mu.Unlock()

		rec := &responseRecorder{
			ResponseWriter: c.Writer,
			header:         make(http.Header),
			body:           &bytes.Buffer{},
			statusCode:     http.StatusOK,
		}
		c.Writer = rec

		err := c.Next()

		cCall.status = rec.statusCode
		cCall.header = rec.header.Clone()
		cCall.body = rec.body.Bytes()
		cCall.err = err
		cCall.wg.Done()

		mu.Lock()
		delete(inFlight, key)
		mu.Unlock()

		return err
	}
}
