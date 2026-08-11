package gpp_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saifsilver/goplusplus"
)

func TestRouterStaticRoute(t *testing.T) {
	app := gpp.New()
	visited := false

	app.GET("/hello", func(c *gpp.Context) error {
		visited = true
		return c.String(http.StatusOK, "Hello World")
	})

	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if !visited {
		t.Fatal("expected route handler to be invoked")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "Hello World" {
		t.Fatalf("expected body 'Hello World', got '%s'", w.Body.String())
	}
}

func TestRouterParamRoute(t *testing.T) {
	app := gpp.New()
	var userID string

	app.GET("/users/:id", func(c *gpp.Context) error {
		userID = c.Param("id")
		return c.JSON(http.StatusOK, gpp.H{"id": userID})
	})

	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if userID != "42" {
		t.Fatalf("expected param id to be '42', got '%s'", userID)
	}
}

func TestRouterGroups(t *testing.T) {
	app := gpp.New()
	api := app.Group("/api/v1")

	var hit bool
	api.GET("/status", func(c *gpp.Context) error {
		hit = true
		return c.JSON(http.StatusOK, gpp.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if !hit {
		t.Fatal("expected group route to be hit")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func BenchmarkStaticRouteMatching(b *testing.B) {
	app := gpp.New()
	app.GET("/v1/users/profile/settings", func(c *gpp.Context) error {
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/users/profile/settings", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		app.ServeHTTP(w, req)
	}
}

func BenchmarkParamRouteMatching(b *testing.B) {
	app := gpp.New()
	app.GET("/v1/users/:id/posts/:post_id", func(c *gpp.Context) error {
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/users/123/posts/456", nil)
	w := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		app.ServeHTTP(w, req)
	}
}
