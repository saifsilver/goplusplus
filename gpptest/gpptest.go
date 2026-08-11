package gpptest

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/saifsilver/goplusplus"
)

// ResponseWrapper wraps httptest.ResponseRecorder with ergonomic assertion helpers.
type ResponseWrapper struct {
	t        testing.TB
	Recorder *httptest.ResponseRecorder
	method   string
	target   string
}

// Tester provides simple 3-line E2E API integration testing for goplusplus applications.
type Tester struct {
	t   testing.TB
	app *gpp.Engine
}

// New initializes an API tester for a goplusplus application engine.
func New(t testing.TB, app *gpp.Engine) *Tester {
	t.Helper()
	return &Tester{t: t, app: app}
}

// Request performs a simulated HTTP request against the application engine without opening network sockets.
func (st *Tester) Request(method, target string, body any, headers map[string]string) *ResponseWrapper {
	st.t.Helper()
	var bodyReader io.Reader
	if body != nil {
		if b, ok := body.([]byte); ok {
			bodyReader = bytes.NewReader(b)
		} else if s, ok := body.(string); ok {
			bodyReader = strings.NewReader(s)
		} else {
			jsonBytes, err := json.Marshal(body)
			if err != nil {
				st.t.Fatalf("gpptest: encode %s %s request body: %v", method, target, err)
			}
			bodyReader = bytes.NewReader(jsonBytes)
		}
	}

	req := httptest.NewRequest(method, target, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	st.app.ServeHTTP(rec, req)

	return &ResponseWrapper{t: st.t, Recorder: rec, method: method, target: target}
}

// GET performs a simulated GET request.
func (st *Tester) GET(target string) *ResponseWrapper {
	return st.Request(http.MethodGet, target, nil, nil)
}

// POST performs a simulated POST request with a JSON body.
func (st *Tester) POST(target string, body any) *ResponseWrapper {
	return st.Request(http.MethodPost, target, body, nil)
}

// PUT performs a simulated PUT request with a JSON body.
func (st *Tester) PUT(target string, body any) *ResponseWrapper {
	return st.Request(http.MethodPut, target, body, nil)
}

// DELETE performs a simulated DELETE request.
func (st *Tester) DELETE(target string) *ResponseWrapper {
	return st.Request(http.MethodDelete, target, nil, nil)
}

// AssertStatus asserts that the response HTTP status code matches expected.
func (r *ResponseWrapper) AssertStatus(expected int) *ResponseWrapper {
	r.t.Helper()
	if r.Recorder.Code != expected {
		r.t.Fatalf("gpptest: %s %s expected HTTP status %d, got %d; body: %s", r.method, r.target, expected, r.Recorder.Code, r.Recorder.Body.String())
	}
	return r
}

// AssertJSON asserts that a JSON field in the response body equals the expected value.
func (r *ResponseWrapper) AssertJSON(key string, expected any) *ResponseWrapper {
	r.t.Helper()
	var data map[string]any
	if err := json.Unmarshal(r.Recorder.Body.Bytes(), &data); err != nil {
		r.t.Fatalf("gpptest: Failed to parse response JSON: %v", err)
	}
	val, ok := data[key]
	if !ok {
		r.t.Fatalf("gpptest: Expected JSON key '%s' not found in response", key)
	}
	if fmtVal(val) != fmtVal(expected) {
		r.t.Fatalf("gpptest: Expected JSON key '%s' to be '%v', got '%v'", key, expected, val)
	}
	return r
}

// AssertContains asserts that the response body contains a expected substring.
func (r *ResponseWrapper) AssertContains(substring string) *ResponseWrapper {
	r.t.Helper()
	if !strings.Contains(r.Recorder.Body.String(), substring) {
		r.t.Fatalf("gpptest: Expected body to contain '%s', got: %s", substring, r.Recorder.Body.String())
	}
	return r
}

// DecodeInto strictly decodes a response body. Empty 204 responses leave the
// target unchanged; empty bodies for other statuses fail the test.
func (r *ResponseWrapper) DecodeInto(target any) *ResponseWrapper {
	r.t.Helper()
	if target == nil || reflect.ValueOf(target).Kind() != reflect.Ptr || reflect.ValueOf(target).IsNil() {
		r.t.Fatalf("gpptest: %s %s decode target must be a non-nil pointer", r.method, r.target)
	}
	body := r.Recorder.Body.Bytes()
	if len(bytes.TrimSpace(body)) == 0 {
		if r.Recorder.Code == http.StatusNoContent {
			return r
		}
		r.t.Fatalf("gpptest: %s %s returned an empty JSON body with status %d", r.method, r.target, r.Recorder.Code)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(target); err != nil {
		r.t.Fatalf("gpptest: %s %s returned malformed JSON with status %d: %v", r.method, r.target, r.Recorder.Code, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		r.t.Fatalf("gpptest: %s %s returned multiple JSON documents", r.method, r.target)
	}
	return r
}

func Decode[T any](response *ResponseWrapper) T {
	response.t.Helper()
	var target T
	response.DecodeInto(&target)
	return target
}

func (r *ResponseWrapper) Problem() gpp.ProblemDetails {
	r.t.Helper()
	problem, err := ParseProblemDetails(r.Recorder.Body.Bytes())
	if err != nil {
		r.t.Fatalf("gpptest: %s %s returned invalid Problem Details: %v", r.method, r.target, err)
	}
	return problem
}

func ParseProblemDetails(body []byte) (gpp.ProblemDetails, error) {
	var problem gpp.ProblemDetails
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&problem); err != nil {
		return problem, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return problem, errors.New("response contains multiple JSON documents")
	}
	if problem.Type == "" || problem.Title == "" || problem.Status < 400 || problem.Status > 599 || problem.Detail == "" {
		return problem, errors.New("response is not valid Problem Details")
	}
	return problem, nil
}

func (r *ResponseWrapper) AssertProblem(status int, problemType string) *ResponseWrapper {
	r.t.Helper()
	problem := r.Problem()
	if problem.Status != status || r.Recorder.Code != status {
		r.t.Fatalf("gpptest: %s %s expected problem status %d, got response=%d problem=%d", r.method, r.target, status, r.Recorder.Code, problem.Status)
	}
	if problemType != "" && problem.Type != problemType {
		r.t.Fatalf("gpptest: %s %s expected problem type %q, got %q", r.method, r.target, problemType, problem.Type)
	}
	return r
}

func (r *ResponseWrapper) AssertViolation(field, rule string) *ResponseWrapper {
	r.t.Helper()
	problem := r.Problem()
	for _, violation := range problem.Errors {
		if violation.Field == field && violation.Rule == rule {
			return r
		}
	}
	r.t.Fatalf("gpptest: %s %s expected violation field=%q rule=%q; got %#v", r.method, r.target, field, rule, problem.Errors)
	return r
}

func (r *ResponseWrapper) AssertHeader(name, expected string) *ResponseWrapper {
	r.t.Helper()
	if actual := r.Recorder.Header().Get(name); actual != expected {
		r.t.Fatalf("gpptest: %s %s expected header %s=%q, got %q", r.method, r.target, name, expected, actual)
	}
	return r
}

func (r *ResponseWrapper) AssertContentType(expected string) *ResponseWrapper {
	r.t.Helper()
	actual, _, err := mime.ParseMediaType(r.Recorder.Header().Get("Content-Type"))
	if err != nil || actual != expected {
		r.t.Fatalf("gpptest: %s %s expected content type %q, got %q", r.method, r.target, expected, r.Recorder.Header().Get("Content-Type"))
	}
	return r
}

func (r *ResponseWrapper) AssertRequestID() *ResponseWrapper {
	r.t.Helper()
	requestID := r.Recorder.Header().Get("X-Request-ID")
	if requestID == "" {
		r.t.Fatalf("gpptest: %s %s response has no X-Request-ID header", r.method, r.target)
	}
	var problem gpp.ProblemDetails
	if json.Unmarshal(r.Recorder.Body.Bytes(), &problem) == nil && problem.Type != "" && problem.TraceID != requestID {
		r.t.Fatalf("gpptest: %s %s problem trace ID %q does not match response request ID %q", r.method, r.target, problem.TraceID, requestID)
	}
	return r
}

func (r *ResponseWrapper) AssertJSONPath(path string, expected any) *ResponseWrapper {
	r.t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(r.Recorder.Body.Bytes()))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		r.t.Fatalf("gpptest: %s %s returned malformed JSON: %v", r.method, r.target, err)
	}
	actual, ok := jsonPath(document, path)
	if !ok {
		r.t.Fatalf("gpptest: %s %s JSON path %q was not found", r.method, r.target, path)
	}
	if fmtVal(actual) != fmtVal(expected) {
		r.t.Fatalf("gpptest: %s %s expected JSON path %q to equal %v, got %v", r.method, r.target, path, expected, actual)
	}
	return r
}

func jsonPath(value any, path string) (any, bool) {
	current := value
	for _, segment := range strings.Split(path, ".") {
		if index, err := strconv.Atoi(segment); err == nil {
			items, ok := current.([]any)
			if !ok || index < 0 || index >= len(items) {
				return nil, false
			}
			current = items[index]
			continue
		}
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func fmtVal(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
