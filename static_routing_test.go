package gpp_test

import (
	"embed"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gpp "github.com/saifsilver/goplusplus"
)

//go:embed testdata/static/*
var staticTestFS embed.FS

func TestRootStaticEmbedNeverShadowsExplicitRoutes(t *testing.T) {
	tests := []struct {
		name        string
		staticFirst bool
	}{
		{name: "static registered first", staticFirst: true},
		{name: "routes registered first", staticFirst: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := gpp.New()
			registerStatic := func() { app.StaticEmbed("/", staticTestFS, "testdata/static") }
			registerRoutes := func() {
				app.POST("/api/auth/login", func(c *gpp.Context) error { return c.String(200, "login-route") })
				app.GET("/healthz/readiness", func(c *gpp.Context) error { return c.String(200, "ready-route") })
			}
			if test.staticFirst {
				registerStatic()
				registerRoutes()
			} else {
				registerRoutes()
				registerStatic()
			}

			assertHTTPBody(t, app, http.MethodPost, "/api/auth/login", 200, "login-route")
			assertHTTPBody(t, app, http.MethodGet, "/healthz/readiness", 200, "ready-route")
			assertHTTPBody(t, app, http.MethodGet, "/client/route", 200, "spa-shell")
		})
	}
}

func TestStaticEmbedStripsNonRootMountAndDoesNotFallback(t *testing.T) {
	app := gpp.New()
	app.StaticEmbed("/assets", staticTestFS, "testdata/static")

	request := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "secure-static") {
		t.Fatalf("asset serving failed: %d %s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, "javascript") {
		t.Fatalf("expected JavaScript content type, got %q", contentType)
	}
	assertHTTPBody(t, app, http.MethodGet, "/assets/missing.js", 404, "Static resource not found")
	assertHTTPBody(t, app, http.MethodGet, "/assets/../index.html", 404, "Resource Not Found")
}

func assertHTTPBody(t *testing.T, app *gpp.Engine, method, path string, status int, contains string) {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != status || !strings.Contains(response.Body.String(), contains) {
		t.Fatalf("%s %s: expected %d containing %q, got %d %q", method, path, status, contains, response.Code, response.Body.String())
	}
}
