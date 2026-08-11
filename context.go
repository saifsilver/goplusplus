package gpp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
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
	written  bool
	response *responseTracker

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
	tracker := &responseTracker{ResponseWriter: w}
	c.Writer = tracker
	c.response = tracker
	c.Request = req
	c.Params = c.Params[:0]
	c.index = -1
	c.aborted = false
	c.written = false

	// Clear keys map without reallocation
	for k := range c.keys {
		delete(c.keys, k)
	}
}

type responseTracker struct {
	http.ResponseWriter
	written bool
}

func (writer *responseTracker) WriteHeader(status int) {
	writer.written = true
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *responseTracker) Write(data []byte) (int, error) {
	writer.written = true
	return writer.ResponseWriter.Write(data)
}

func (writer *responseTracker) Flush() {
	_ = http.NewResponseController(writer.ResponseWriter).Flush()
}

func (writer *responseTracker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(writer.ResponseWriter).Hijack()
}

func (writer *responseTracker) Push(target string, options *http.PushOptions) error {
	if pusher, ok := writer.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, options)
	}
	return http.ErrNotSupported
}

func (writer *responseTracker) Unwrap() http.ResponseWriter { return writer.ResponseWriter }

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

// Param retrieves a path parameter by name (e.g. /users/:id -> c.Param("id")).
func (c *Context) Param(key string) string {
	return c.Params.Get(key)
}

// ParamInt64 retrieves a path parameter converted directly to int64 with optional default fallback.
func (c *Context) ParamInt64(key string, defaultValue ...int64) int64 {
	valStr := c.Param(key)
	if valStr == "" {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	if val, err := strconv.ParseInt(valStr, 10, 64); err == nil {
		return val
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return 0
}

// ParamInt retrieves a path parameter converted directly to int with optional default fallback.
func (c *Context) ParamInt(key string, defaultValue ...int) int {
	valStr := c.Param(key)
	if valStr == "" {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	if val, err := strconv.Atoi(valStr); err == nil {
		return val
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return 0
}

// UserID extracts the active authenticated user ID (int64) from request context storage ("user_id" or "sub").
func (c *Context) UserID() int64 {
	if id := c.GetInt64("user_id"); id > 0 {
		return id
	}
	if id := c.GetInt64("sub"); id > 0 {
		return id
	}
	return 0
}

// RequireUserID returns the active authenticated user ID, or returns a 401 Unauthorized error if unauthenticated.
func (c *Context) RequireUserID() (int64, error) {
	id := c.UserID()
	if id <= 0 {
		return 0, ErrUnauthorized("Authentication required")
	}
	return id, nil
}

// UserSubject returns the canonical authenticated string identity. Unlike
// UserID, it supports UUIDs and other bounded opaque identifiers.
func (c *Context) UserSubject() string {
	return c.GetString("sub")
}

// RequireUserSubject returns the canonical string identity or a 401 error.
func (c *Context) RequireUserSubject() (string, error) {
	subject := c.UserSubject()
	if subject == "" {
		return "", ErrUnauthorized("Authentication required")
	}
	return subject, nil
}

// OK writes an HTTP 200 OK JSON response.
func (c *Context) OK(data any) error {
	return c.JSON(http.StatusOK, data)
}

// Created writes an HTTP 201 Created JSON response.
func (c *Context) Created(data any) error {
	return c.JSON(http.StatusCreated, data)
}

// Accepted writes an HTTP 202 Accepted JSON response for asynchronous operations.
func (c *Context) Accepted(data any) error {
	return c.JSON(http.StatusAccepted, data)
}

// NoContent writes an HTTP 204 No Content response with zero body.
func (c *Context) NoContent() error {
	c.Status(http.StatusNoContent)
	return nil
}

// BadRequest writes an HTTP 400 Bad Request error response.
func (c *Context) BadRequest(message string) error {
	return ErrBadRequest(message)
}

// Unauthorized writes an HTTP 401 Unauthorized error response.
func (c *Context) Unauthorized(message string) error {
	return ErrUnauthorized(message)
}

// Forbidden writes an HTTP 403 Forbidden error response.
func (c *Context) Forbidden(message string) error {
	return ErrForbidden(message)
}

// NotFound writes an HTTP 404 Not Found error response.
func (c *Context) NotFound(message string) error {
	return ErrNotFound(message)
}

// Conflict writes an HTTP 409 Conflict error response.
func (c *Context) Conflict(message string) error {
	return ErrConflict(message)
}

// InternalError writes an HTTP 500 Internal Server Error response.
func (c *Context) InternalError(message string) error {
	return ErrInternal(message)
}

// PathValue is an alias for Param, providing Go 1.22+ r.PathValue() compatibility.
func (c *Context) PathValue(key string) string {
	return c.Param(key)
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

// QueryInt retrieves a query parameter converted to integer with optional default fallback.
func (c *Context) QueryInt(key string, defaultValue ...int) int {
	valStr := c.Query(key)
	if valStr == "" {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return 0
	}
	if intVal, err := strconv.Atoi(valStr); err == nil {
		return intVal
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return 0
}

// QueryBool retrieves a query parameter converted to boolean with optional default fallback.
func (c *Context) QueryBool(key string, defaultValue ...bool) bool {
	valStr := c.Query(key)
	if valStr == "" {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return false
	}
	if boolVal, err := strconv.ParseBool(valStr); err == nil {
		return boolVal
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return false
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

// Delete removes a key from request context storage.
func (c *Context) Delete(key string) {
	c.mu.Lock()
	delete(c.keys, key)
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

// GetAny retrieves a stored value by key as a single return value (nil if not found).
func (c *Context) GetAny(key string) any {
	val, _ := c.Get(key)
	return val
}

// Value retrieves a stored value by key as a single return value (nil if not found), compatible with context.Context interface style.
func (c *Context) Value(key string) any {
	return c.GetAny(key)
}

// GetString retrieves a stored string value from context storage, converting numeric/boolean values if necessary.
func (c *Context) GetString(key string) string {
	val, ok := c.Get(key)
	if !ok || val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// GetInt64 retrieves a stored value converted to int64. Returns 0 if not found or unconvertible.
func (c *Context) GetInt64(key string) int64 {
	val, ok := c.Get(key)
	if !ok || val == nil {
		return 0
	}
	switch v := val.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case int16:
		return int64(v)
	case int8:
		return int64(v)
	case uint:
		return int64(v)
	case uint64:
		return int64(v)
	case uint32:
		return int64(v)
	case uint16:
		return int64(v)
	case uint8:
		return int64(v)
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	case string:
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i
		}
	}
	return 0
}

// GetInt retrieves a stored value converted to int. Returns 0 if not found or unconvertible.
func (c *Context) GetInt(key string) int {
	return int(c.GetInt64(key))
}

// GetFloat64 retrieves a stored value converted to float64. Returns 0.0 if not found or unconvertible.
func (c *Context) GetFloat64(key string) float64 {
	val, ok := c.Get(key)
	if !ok || val == nil {
		return 0
	}
	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case int32:
		return float64(v)
	case uint:
		return float64(v)
	case uint64:
		return float64(v)
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return 0
}

// GetBool retrieves a stored value converted to bool. Returns false if not found or unconvertible.
func (c *Context) GetBool(key string) bool {
	val, ok := c.Get(key)
	if !ok || val == nil {
		return false
	}
	switch v := val.(type) {
	case bool:
		return v
	case int:
		return v != 0
	case int64:
		return v != 0
	case string:
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return false
}

// GetAs retrieves a context value typed as T if present and matching type T.
func GetAs[T any](c *Context, key string) (T, bool) {
	var zero T
	val, ok := c.Get(key)
	if !ok || val == nil {
		return zero, false
	}
	typed, ok := val.(T)
	if !ok {
		return zero, false
	}
	return typed, true
}

// GetOrDefault retrieves a context value typed as T, or returns defaultValue if not set or wrong type.
func GetOrDefault[T any](c *Context, key string, defaultValue T) T {
	if val, ok := GetAs[T](c, key); ok {
		return val
	}
	return defaultValue
}

// MustGetAs retrieves a context value typed as T or panics if not set or wrong type.
func MustGetAs[T any](c *Context, key string) T {
	val, ok := GetAs[T](c, key)
	if !ok {
		panic(fmt.Sprintf("gpp.Context: key '%s' does not exist or type mismatch", key))
	}
	return val
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
	c.written = true
	c.Writer.WriteHeader(code)
}

// BindJSON securely parses one JSON document into a non-nil pointer.
func (c *Context) BindJSON(v any) error {
	return bindJSON(c, v)
}

// Validate executes validation rules declared in validate struct tags and returns
// RFC 7807 problem details on failure.
func (c *Context) Validate(v any) error {
	return validateStruct(v)
}

// BindAndValidate decodes the JSON request body and validates struct fields in a single atomic step.
func (c *Context) BindAndValidate(v any) error {
	if err := c.BindJSON(v); err != nil {
		return err
	}
	return c.Validate(v)
}

// NormalizeFunc applies application-owned normalization after binding and
// before validation. A nil callback is a documented no-op.
type NormalizeFunc func(context.Context) error

// BindNormalizeAndValidate executes the explicit request processing pipeline.
func (c *Context) BindNormalizeAndValidate(v any, normalize NormalizeFunc) error {
	if !isValidBindingTarget(v) {
		return bindingProblem(http.StatusBadRequest, "invalid-target", "Invalid request target")
	}
	if err := c.BindJSON(v); err != nil {
		return err
	}
	if normalize != nil {
		if err := normalize(c.Request.Context()); err != nil {
			if c.Request.Context().Err() != nil {
				return c.Request.Context().Err()
			}
			return ErrBadRequest("Request normalization failed")
		}
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

// Problem writes an RFC 7807/9457 Problem Details response.
func (c *Context) Problem(problem ProblemDetails) error {
	c.SetHeader("Content-Type", "application/problem+json; charset=utf-8")
	c.Status(problem.Status)
	return json.NewEncoder(c.Writer).Encode(problem)
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

// HTML renders an HTML string response with text/html content type.
func (c *Context) HTML(statusCode int, html string) error {
	return c.Data(statusCode, "text/html; charset=utf-8", []byte(html))
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

// RequestID returns the active request ID from context or HTTP header.
func (c *Context) RequestID() string {
	if val := c.GetString("request_id"); val != "" {
		return val
	}
	return c.GetHeader("X-Request-ID")
}

// GetPageAndLimit parses query parameters 'page' and 'limit' with safe fallback defaults.
func (c *Context) GetPageAndLimit(defaultLimit ...int) (int, int) {
	dLimit := 20
	if len(defaultLimit) > 0 && defaultLimit[0] > 0 {
		dLimit = defaultLimit[0]
	}
	page := c.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}
	limit := c.QueryInt("limit", dLimit)
	if limit < 1 {
		limit = dLimit
	}
	if limit > 1000 {
		limit = 1000
	}
	return page, limit
}

// GetCursorAndLimit parses query parameters 'cursor' and 'limit' with safe fallback defaults.
func (c *Context) GetCursorAndLimit(defaultLimit ...int) (string, int) {
	dLimit := 20
	if len(defaultLimit) > 0 && defaultLimit[0] > 0 {
		dLimit = defaultLimit[0]
	}
	cursor := c.Query("cursor")
	limit := c.QueryInt("limit", dLimit)
	if limit < 1 {
		limit = dLimit
	}
	if limit > 1000 {
		limit = 1000
	}
	return cursor, limit
}

// Paginate writes a standardized offset-based paginated JSON response.
func (c *Context) Paginate(statusCode int, items any, page, limit, total int) error {
	totalPages, err := TotalPages(total, limit)
	if err != nil {
		return err
	}
	return c.JSON(statusCode, H{
		"data": items,
		"pagination": H{
			"page":        page,
			"limit":       limit,
			"total_items": total,
			"total_pages": totalPages,
		},
	})
}

// PaginateCursor writes a standardized O(1) high-performance cursor-based paginated JSON response.
func (c *Context) PaginateCursor(statusCode int, items any, nextCursor string, hasMore bool, limit int) error {
	return c.JSON(statusCode, H{
		"data": items,
		"pagination": H{
			"next_cursor": nextCursor,
			"has_more":    hasMore,
			"limit":       limit,
		},
	})
}

// Retry executes a function with up to maxAttempts automatic retries on error.
func (c *Context) Retry(maxAttempts int, fn func(c *Context) error) error {
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	var lastErr error
	for i := 1; i <= maxAttempts; i++ {
		if err := fn(c); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	return lastErr
}
