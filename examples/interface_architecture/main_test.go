package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	gpp "github.com/saifsilver/goplusplus"
)

func TestInterfaceArchitectureExample(t *testing.T) {
	app := gpp.New()
	app.GET("/arch", func(c *gpp.Context) error {
		return c.String(http.StatusOK, "%s", "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/arch", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}
