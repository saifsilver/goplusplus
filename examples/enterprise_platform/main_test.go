package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	gpp "github.com/saifsilver/goplusplus"
)

func TestEnterprisePlatformExample(t *testing.T) {
	app := gpp.New()
	app.GET("/api/v1/ping", func(c *gpp.Context) error {
		return c.String(http.StatusOK, "%s", "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}
