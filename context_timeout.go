package gpp

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"sync"
	"time"
)

// RunWithTimeout executes the remaining handler chain against an isolated,
// buffered response. Only the request goroutine commits a response.
func (c *Context) RunWithTimeout(duration time.Duration) error {
	if duration <= 0 {
		return errors.New("gpp: timeout duration must be positive")
	}
	requestContext, cancel := context.WithTimeout(c.Request.Context(), duration)
	defer cancel()

	buffer := newBufferedResponse(c.Writer.Header())
	child := c.timeoutChild(buffer, c.Request.WithContext(requestContext))
	done := make(chan error, 1)
	go func() { done <- child.Next() }()

	select {
	case err := <-done:
		c.adoptTimeoutChild(child)
		if err == nil {
			buffer.commit(c.Writer)
		}
		return err
	case <-requestContext.Done():
		c.index = abortIndex
		return ErrRequestTimeout("Request exceeded the configured deadline")
	}
}

func (c *Context) timeoutChild(writer http.ResponseWriter, request *http.Request) *Context {
	c.mu.RLock()
	keys := make(map[string]any, len(c.keys))
	for key, value := range c.keys {
		keys[key] = value
	}
	c.mu.RUnlock()
	return &Context{
		Writer: writer, Request: request, Params: append(Params(nil), c.Params...),
		handlers: c.handlers, index: c.index, keys: keys, engine: c.engine,
	}
}

func (c *Context) adoptTimeoutChild(child *Context) {
	c.index = child.index
	c.aborted = child.aborted
	c.mu.Lock()
	c.keys = child.keys
	c.mu.Unlock()
}

type bufferedResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
	mu     sync.Mutex
}

func newBufferedResponse(initial http.Header) *bufferedResponse {
	return &bufferedResponse{header: initial.Clone()}
}

// Header returns the response headers buffered for a timed handler.
func (response *bufferedResponse) Header() http.Header { return response.header }

// WriteHeader records the first status written by a timed handler.
func (response *bufferedResponse) WriteHeader(status int) {
	response.mu.Lock()
	defer response.mu.Unlock()
	if response.status == 0 {
		response.status = status
	}
}

// Write buffers response bytes until the timed handler completes.
func (response *bufferedResponse) Write(data []byte) (int, error) {
	response.mu.Lock()
	defer response.mu.Unlock()
	if response.status == 0 {
		response.status = http.StatusOK
	}
	return response.body.Write(data)
}

func (response *bufferedResponse) commit(writer http.ResponseWriter) {
	response.mu.Lock()
	defer response.mu.Unlock()
	for key := range writer.Header() {
		writer.Header().Del(key)
	}
	for key, values := range response.header {
		writer.Header()[key] = append([]string(nil), values...)
	}
	status := response.status
	if status == 0 {
		status = http.StatusOK
	}
	writer.WriteHeader(status)
	_, _ = writer.Write(response.body.Bytes())
}
