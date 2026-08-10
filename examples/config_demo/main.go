package main

import (
	"fmt"
	"net/http"
	"time"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/config"
	"github.com/saifsilver/goplusplus/middleware"
)

type AppConfig struct {
	Port     string        `env:"PORT" default:":8080"`
	DBURL    string        `env:"DATABASE_URL" default:"postgres://localhost:5432/app"`
	Secret   string        `env:"JWT_SECRET" default:"super_secret_jwt_key_991823"`
	Timeout  time.Duration `env:"TIMEOUT" default:"30s"`
	Debug    bool          `env:"DEBUG" default:"true"`
}

func main() {
	// 1. One-line configuration loading (.env + env vars + struct defaults)
	cfg := config.MustLoad[AppConfig]()

	// 3. Initialize goplusplus App Engine
	app := gpp.New()

	app.Use(
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Security(),
	)

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

	fmt.Printf("🚀 Server running on %s (Secret Masked: %s)\n", cfg.Port, config.MaskSecret(cfg.Secret))

	if err := app.Listen(cfg.Port); err != nil {
		panic(err)
	}
}
