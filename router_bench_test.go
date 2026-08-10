package gpp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func BenchmarkRouterStatic(b *testing.B) {
	app := New()
	app.GET("/api/v1/users", func(c *Context) error {
		return c.String(http.StatusOK, "OK")
	})

	req := httptest.NewRequest("GET", "/api/v1/users", nil)
	rec := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		app.ServeHTTP(rec, req)
	}
}

func BenchmarkRouterParam(b *testing.B) {
	app := New()
	app.GET("/api/v1/users/:id", func(c *Context) error {
		_ = c.Param("id")
		return c.String(http.StatusOK, "OK")
	})

	req := httptest.NewRequest("GET", "/api/v1/users/10091", nil)
	rec := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		app.ServeHTTP(rec, req)
	}
}

func BenchmarkRouterParallel(b *testing.B) {
	app := New()
	app.GET("/api/v1/orders/:id", func(c *Context) error {
		return c.JSON(http.StatusOK, H{"id": c.Param("id")})
	})

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		req := httptest.NewRequest("GET", "/api/v1/orders/991", nil)
		rec := httptest.NewRecorder()
		for pb.Next() {
			app.ServeHTTP(rec, req)
		}
	})
}

func BenchmarkZeroAllocEndpoint(b *testing.B) {
	app := New()
	payload := []byte(`{"status":"ok"}`)
	app.GET("/api/v1/bench/:id", func(c *Context) error {
		return c.Data(http.StatusOK, "application/json", payload)
	})

	req := httptest.NewRequest("GET", "/api/v1/bench/100", nil)
	rec := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		app.ServeHTTP(rec, req)
	}
}
