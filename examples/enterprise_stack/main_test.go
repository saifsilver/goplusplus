package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	gpp "github.com/saifsilver/goplusplus"
)

func TestEnterpriseStackExample(t *testing.T) {
	app := gpp.New()
	app.GET("/status", func(c *gpp.Context) error {
		return c.JSON(http.StatusOK, gpp.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}
