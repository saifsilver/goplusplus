package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/config"
	"github.com/saifsilver/goplusplus/middleware"
)

func TestConfigDemo(t *testing.T) {
	cfg := config.MustLoad[AppConfig]()
	app := gpp.New()
	app.Use(middleware.Logger(), middleware.Recovery(), middleware.Security())

	app.GET("/api/v1/config", func(c *gpp.Context) error {
		return c.JSON(http.StatusOK, gpp.H{
			"port":          cfg.Port,
			"debug":         cfg.Debug,
			"masked_secret": config.MaskSecret(cfg.Secret),
			"masked_db_url": config.MaskSecret(cfg.DBURL),
			"port_get":      config.GetString("PORT", ":8080"),
			"conns_get":     config.GetInt("MAX_CONNS", 100),
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", w.Code)
	}
}
