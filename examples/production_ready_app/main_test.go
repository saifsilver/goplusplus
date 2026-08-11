package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/health"
	"github.com/saifsilver/goplusplus/middleware"
)

func TestProductionReadyApp(t *testing.T) {
	healthChecker := health.NewChecker()
	healthChecker.AddReadinessCheck("db", func(ctx context.Context) error { return nil })

	app := gpp.New()
	app.Use(middleware.Logger(), middleware.Recovery())

	app.GET("/healthz/liveness", healthChecker.Liveness())
	app.GET("/healthz/readiness", healthChecker.Readiness())

	req := httptest.NewRequest(http.MethodGet, "/healthz/liveness", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", w.Code)
	}

	reqPost := httptest.NewRequest(http.MethodPost, "/api/v1/leads", bytes.NewReader([]byte(`{"name":"Alex","email":"alex@example.com"}`)))
	reqPost.Header.Set("Content-Type", "application/json")
	wPost := httptest.NewRecorder()
	app.ServeHTTP(wPost, reqPost)
}
