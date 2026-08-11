package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	gpp "github.com/saifsilver/goplusplus"
)

func TestHyperScaleAppExample(t *testing.T) {
	app := gpp.New()
	app.GET("/scale", func(c *gpp.Context) error {
		return c.JSON(http.StatusOK, gpp.H{"scale": "hyper"})
	})

	req := httptest.NewRequest(http.MethodGet, "/scale", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}
