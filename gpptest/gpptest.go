package gpptest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/saifsilver/goplusplus"
)

// ResponseWrapper wraps httptest.ResponseRecorder with ergonomic assertion helpers.
type ResponseWrapper struct {
	t        *testing.T
	Recorder *httptest.ResponseRecorder
}

// Tester provides simple 3-line E2E API integration testing for goplusplus applications.
type Tester struct {
	t   *testing.T
	app *gpp.Engine
}

// New initializes an API tester for a goplusplus application engine.
func New(t *testing.T, app *gpp.Engine) *Tester {
	return &Tester{t: t, app: app}
}

// Request performs a simulated HTTP request against the application engine without opening network sockets.
func (st *Tester) Request(method, target string, body any, headers map[string]string) *ResponseWrapper {
	var bodyReader io.Reader
	if body != nil {
		if b, ok := body.([]byte); ok {
			bodyReader = bytes.NewReader(b)
		} else if s, ok := body.(string); ok {
			bodyReader = strings.NewReader(s)
		} else {
			jsonBytes, _ := json.Marshal(body)
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

	return &ResponseWrapper{t: st.t, Recorder: rec}
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
	if r.Recorder.Code != expected {
		r.t.Fatalf("gpptest: Expected HTTP status %d, got %d. Body: %s", expected, r.Recorder.Code, r.Recorder.Body.String())
	}
	return r
}

// AssertJSON asserts that a JSON field in the response body equals the expected value.
func (r *ResponseWrapper) AssertJSON(key string, expected any) *ResponseWrapper {
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
	if !strings.Contains(r.Recorder.Body.String(), substring) {
		r.t.Fatalf("gpptest: Expected body to contain '%s', got: %s", substring, r.Recorder.Body.String())
	}
	return r
}

func fmtVal(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
