package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	gpp "github.com/saifsilver/goplusplus"
)

func TestReactEmbeddedExample(t *testing.T) {
	app := gpp.New()
	app.GET("/", func(c *gpp.Context) error {
		return c.String(http.StatusOK, "%s", "react_app")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}
