package gpp_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gpp "github.com/saifsilver/goplusplus"
)

func TestSecureJSONBindingFailures(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	tests := []struct {
		name        string
		body        string
		contentType string
		configure   func(*gpp.Engine)
		status      int
		problemType string
	}{
		{name: "empty", body: "", contentType: "application/json", status: 400, problemType: "empty-body"},
		{name: "malformed", body: `{`, contentType: "application/json", status: 400, problemType: "malformed-json"},
		{name: "wrong type", body: `{"name":42}`, contentType: "application/json", status: 400, problemType: "type-mismatch"},
		{name: "unknown field", body: `{"name":"ok","admin":true}`, contentType: "application/json", status: 400, problemType: "unknown-field"},
		{name: "multiple documents", body: `{"name":"one"} {"name":"two"}`, contentType: "application/json", status: 400, problemType: "trailing-data"},
		{name: "trailing data", body: `{"name":"one"} trailing`, contentType: "application/json", status: 400, problemType: "trailing-data"},
		{name: "wrong content type", body: `{"name":"ok"}`, contentType: "text/plain", status: 415, problemType: "content-type"},
		{
			name: "oversized", body: `{"name":"a body that is too large"}`, contentType: "application/json", status: 413,
			problemType: "body-too-large", configure: func(app *gpp.Engine) { app.JSONBinding.MaxBodyBytes = 16 },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := gpp.New()
			if test.configure != nil {
				test.configure(app)
			}
			app.POST("/bind", func(c *gpp.Context) error {
				var request payload
				if err := c.BindJSON(&request); err != nil {
					return err
				}
				return c.NoContent()
			})

			request := httptest.NewRequest(http.MethodPost, "/bind", strings.NewReader(test.body))
			request.Header.Set("Content-Type", test.contentType)
			response := httptest.NewRecorder()
			app.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("expected status %d, got %d: %s", test.status, response.Code, response.Body.String())
			}
			var problem gpp.ProblemDetails
			if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
				t.Fatalf("decode problem: %v", err)
			}
			if !strings.HasSuffix(problem.Type, "/"+test.problemType) || (test.body != "" && strings.Contains(problem.Detail, test.body)) {
				t.Fatalf("unexpected sanitized problem: %+v", problem)
			}
		})
	}
}

func TestJSONBindingCompatibilityOptions(t *testing.T) {
	app := gpp.New()
	app.JSONBinding.AllowUnknownFields = true
	app.JSONBinding.AllowNonJSONContentType = true
	app.POST("/bind", func(c *gpp.Context) error {
		var request struct {
			Name string `json:"name"`
		}
		if err := c.BindJSON(&request); err != nil {
			return err
		}
		return c.String(http.StatusOK, "%s", request.Name)
	})

	request := httptest.NewRequest(http.MethodPost, "/bind", strings.NewReader(`{"name":"ok","extra":true}`))
	request.Header.Set("Content-Type", "text/plain")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != 200 || response.Body.String() != "ok" {
		t.Fatalf("compatibility binding failed: %d %s", response.Code, response.Body.String())
	}
}

func TestDefaultErrorsAndRecoveryDoNotLeakInternals(t *testing.T) {
	app := gpp.New()
	app.GET("/error", func(*gpp.Context) error {
		return errors.New("database password=/private/secret.db")
	})

	request := httptest.NewRequest(http.MethodGet, "/error", nil)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != 500 || bytes.Contains(response.Body.Bytes(), []byte("password")) || bytes.Contains(response.Body.Bytes(), []byte("/private")) {
		t.Fatalf("internal details leaked: %s", response.Body.String())
	}
}
