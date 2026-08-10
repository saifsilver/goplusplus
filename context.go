package gpp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"reflect"
	"strings"
	"sync"

	"github.com/saifsilver/goplusplus/queue"
)

const abortIndex int8 = math.MaxInt8 / 2

// HandlerFunc defines the function signature for framework handlers and middleware.
type HandlerFunc func(c *Context) error

// HandlersChain is a slice of HandlerFunc executed in sequence.
type HandlersChain []HandlerFunc

// Context represents the request-scoped context containing Request, ResponseWriter, and parameter metadata.
type Context struct {
	Writer  http.ResponseWriter
	Request *http.Request
	Params  Params

	handlers HandlersChain
	index    int8
	keys     map[string]any
	mu       sync.RWMutex
	aborted  bool

	// engine back-reference
	engine *Engine
}

func newContext() *Context {
	return &Context{
		index: -1,
		keys:  make(map[string]any),
	}
}

func (c *Context) reset(w http.ResponseWriter, req *http.Request) {
	c.Writer = w
	c.Request = req
	c.Params = c.Params[:0]
	c.index = -1
	c.aborted = false

	// Clear keys map without reallocation
	for k := range c.keys {
		delete(c.keys, k)
	}
}

// Next executes the remaining handlers in the chain.
func (c *Context) Next() error {
	c.index++
	for c.index < int8(len(c.handlers)) {
		if c.aborted {
			break
		}
		if err := c.handlers[c.index](c); err != nil {
			return err
		}
		c.index++
	}
	return nil
}

// Abort stops the execution of remaining handlers in the current request chain.
func (c *Context) Abort() {
	c.index = abortIndex
	c.aborted = true
}

// IsAborted returns true if the current context request chain has been aborted.
func (c *Context) IsAborted() bool {
	return c.aborted
}

// IsCancelled returns true if the client closed the browser connection or cancelled the request.
func (c *Context) IsCancelled() bool {
	if c.Request == nil || c.Request.Context() == nil {
		return false
	}
	select {
	case <-c.Request.Context().Done():
		return true
	default:
		return false
	}
}

// AbortWithStatusJSON aborts execution and writes a JSON error payload immediately.
func (c *Context) AbortWithStatusJSON(statusCode int, payload any) error {
	c.Abort()
	return c.JSON(statusCode, payload)
}

// Param retrieves a path parameter by name.
func (c *Context) Param(key string) string {
	return c.Params.Get(key)
}

// Query retrieves a URL query parameter by name.
func (c *Context) Query(key string) string {
	return c.Request.URL.Query().Get(key)
}

// QueryDefault retrieves a URL query parameter by name, returning defaultValue if empty.
func (c *Context) QueryDefault(key, defaultValue string) string {
	if val := c.Query(key); val != "" {
		return val
	}
	return defaultValue
}

// Set stores a key-value pair in the request context storage.
func (c *Context) Set(key string, value any) {
	c.mu.Lock()
	if c.keys == nil {
		c.keys = make(map[string]any)
	}
	c.keys[key] = value
	c.mu.Unlock()
}

// Get retrieves a stored value by key from the request context storage.
func (c *Context) Get(key string) (any, bool) {
	c.mu.RLock()
	val, ok := c.keys[key]
	c.mu.RUnlock()
	return val, ok
}

// MustGet retrieves a value by key or panics if not found.
func (c *Context) MustGet(key string) any {
	val, ok := c.Get(key)
	if !ok {
		panic(fmt.Sprintf("gpp.Context: key '%s' does not exist", key))
	}
	return val
}

// GetString retrieves a stored string value from context storage.
func (c *Context) GetString(key string) string {
	if val, ok := c.Get(key); ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

// SetHeader sets an HTTP response header.
func (c *Context) SetHeader(key, value string) {
	c.Writer.Header().Set(key, value)
}

// GetHeader gets an HTTP request header.
func (c *Context) GetHeader(key string) string {
	return c.Request.Header.Get(key)
}

// Status sets the HTTP response status code.
func (c *Context) Status(code int) {
	c.Writer.WriteHeader(code)
}

// BindJSON parses the request body as JSON into the provided struct pointer.
func (c *Context) BindJSON(v any) error {
	if c.Request.Body == nil {
		return errors.New("gpp: request body is nil")
	}
	defer c.Request.Body.Close()
	decoder := json.NewDecoder(c.Request.Body)
	return decoder.Decode(v)
}

// Validate executes struct tag validation (e.g. validate:"required", validate:"email") and returns ErrBadRequest on failure.
func (c *Context) Validate(v any) error {
	if v == nil {
		return ErrBadRequest("Validation target cannot be nil")
	}
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil
	}

	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)
		tag := field.Tag.Get("validate")
		if tag == "" {
			continue
		}

		rules := strings.Split(tag, ",")
		for _, rule := range rules {
			rule = strings.TrimSpace(rule)
			switch rule {
			case "required":
				if fieldVal.IsZero() {
					return ErrBadRequest(fmt.Sprintf("Field '%s' is required", field.Name))
				}
			case "email":
				str := fmt.Sprintf("%v", fieldVal.Interface())
				if str != "" && (!strings.Contains(str, "@") || !strings.Contains(str, ".")) {
					return ErrBadRequest(fmt.Sprintf("Field '%s' must be a valid email address", field.Name))
				}
			}
		}
	}
	return nil
}

// BindAndValidate decodes the JSON request body and validates struct fields in a single atomic step.
func (c *Context) BindAndValidate(v any) error {
	if err := c.BindJSON(v); err != nil {
		return ErrBadRequest("Invalid request body: " + err.Error())
	}
	return c.Validate(v)
}

// Parallel executes multiple task functions concurrently in parallel goroutines.
func (c *Context) Parallel(tasks ...func(c *Context) error) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(tasks))

	for _, task := range tasks {
		wg.Add(1)
		go func(fn func(c *Context) error) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errChan <- fmt.Errorf("parallel task panic: %v", r)
				}
			}()
			if err := fn(c); err != nil {
				errChan <- err
			}
		}(task)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			return err
		}
	}
	return nil
}

// Async dispatches a non-blocking background task in a detached goroutine with panic safety.
func (c *Context) Async(fn func(c *Context) error) {
	cCopy := &Context{
		Request: c.Request.Clone(context.Background()),
		Writer:  c.Writer,
		index:   c.index,
		keys:    c.keys,
	}

	go func(ctx *Context) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("gpp: background async task panic", slog.Any("panic", r))
			}
		}()
		if err := fn(ctx); err != nil {
			slog.Error("gpp: background async task error", slog.String("error", err.Error()))
		}
	}(cCopy)
}

// AsyncTask dispatches a background task with automatic retries on failure and persistent status tracking, returning a task ID.
func (c *Context) AsyncTask(name string, fn func(c *Context) error) string {
	cCopy := &Context{
		Request: c.Request.Clone(context.Background()),
		Writer:  c.Writer,
		index:   c.index,
		keys:    c.keys,
	}
	return queue.AsyncTask(name, 3, func(ctx context.Context) error {
		return fn(cCopy)
	})
}

// GetTaskStatus retrieves the status and retry details of a background task by ID.
func (c *Context) GetTaskStatus(taskID string) (*queue.TaskInfo, bool) {
	return queue.GetTaskStatus(taskID)
}

// SSE streams Server-Sent Events from a channel continuously.
func (c *Context) SSE(eventChan <-chan any) error {
	c.SetHeader("Content-Type", "text/event-stream")
	c.SetHeader("Cache-Control", "no-cache")
	c.SetHeader("Connection", "keep-alive")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if ok {
		flusher.Flush()
	}

	for {
		select {
		case <-c.Request.Context().Done():
			return nil
		case item, open := <-eventChan:
			if !open {
				return nil
			}
			dataBytes, _ := json.Marshal(item)
			payload := fmt.Sprintf("data: %s\n\n", string(dataBytes))
			_, _ = c.Writer.Write([]byte(payload))
			if ok && flusher != nil {
				flusher.Flush()
			}
		}
	}
}

// Body reads the raw bytes from the request body.
func (c *Context) Body() ([]byte, error) {
	if c.Request.Body == nil {
		return nil, nil
	}
	defer c.Request.Body.Close()
	return io.ReadAll(c.Request.Body)
}

// JSON serializes the given data struct into JSON and writes it with the status code.
func (c *Context) JSON(statusCode int, data any) error {
	c.SetHeader("Content-Type", "application/json; charset=utf-8")
	c.Status(statusCode)
	return json.NewEncoder(c.Writer).Encode(data)
}

// String formats and writes a plain text string response.
func (c *Context) String(statusCode int, format string, values ...any) error {
	c.SetHeader("Content-Type", "text/plain; charset=utf-8")
	c.Status(statusCode)
	if len(values) == 0 {
		_, err := c.Writer.Write([]byte(format))
		return err
	}
	_, err := fmt.Fprintf(c.Writer, format, values...)
	return err
}

// Data writes raw binary data with a specified content type.
func (c *Context) Data(statusCode int, contentType string, data []byte) error {
	c.SetHeader("Content-Type", contentType)
	c.Status(statusCode)
	_, err := c.Writer.Write(data)
	return err
}

// File serves a static file from disk.
func (c *Context) File(filepath string) error {
	http.ServeFile(c.Writer, c.Request, filepath)
	return nil
}

// Redirect sends an HTTP redirect response.
func (c *Context) Redirect(code int, location string) error {
	if code < 300 || code > 308 {
		return errors.New("gpp: invalid redirect status code")
	}
	http.Redirect(c.Writer, c.Request, location, code)
	return nil
}
