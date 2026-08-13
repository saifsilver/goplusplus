package gpp

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/saifsilver/goplusplus/dbcore"
)

// AppConfig defines options for high-level zero-boilerplate app initialization.
type AppConfig struct {
	Env         string
	Port        int
	DBDriver    string
	DBPath      string
	RWDSN       string
	RODSN       string
	AuthSecret  string
	AutoMigrate bool
}

// NewApp creates a zero-boilerplate application engine pre-configured with
// logging, recovery, security headers, CORS, and auto-DB connection.
func NewApp(configs ...AppConfig) *Engine {
	cfg := AppConfig{}
	if len(configs) > 0 {
		cfg = configs[0]
	}

	if cfg.Env == "" {
		cfg.Env = os.Getenv("APP_ENV")
		if cfg.Env == "" {
			cfg.Env = "development"
		}
	}
	if cfg.Port == 0 {
		if envPort := os.Getenv("PORT"); envPort != "" {
			if p, err := strconv.Atoi(envPort); err == nil {
				cfg.Port = p
			}
		}
		if cfg.Port == 0 {
			cfg.Port = 8080
		}
	}

	engine := New()
	engine.Use(
		defaultLoggerMiddleware(),
		defaultRecoveryMiddleware(),
		defaultSecurityMiddleware(),
		defaultCORSMiddleware(),
	)

	// Auto-configure database connection
	rwDSN := cfg.RWDSN
	if rwDSN == "" {
		rwDSN = os.Getenv("DATABASE_URL")
	}
	dbPath := cfg.DBPath
	if dbPath == "" {
		dbPath = os.Getenv("DB_PATH")
	}

	if rwDSN != "" || dbPath != "" {
		client, err := initAutoDBClient(cfg.DBDriver, dbPath, rwDSN)
		if err != nil {
			slog.Warn("gpp: Auto database initialization failed", slog.String("error", err.Error()))
		} else {
			engine.dbClient = client
		}
	} else if _, err := os.Stat("app.db"); err == nil || cfg.AutoMigrate {
		client, err := initAutoDBClient("sqlite", "app.db", "")
		if err == nil {
			engine.dbClient = client
		}
	}

	return engine
}

// SetDBClient attaches an explicit database client to the app engine.
func (engine *Engine) SetDBClient(client *dbcore.Client) {
	engine.dbClient = client
}

// DBClient returns the initialized dbcore.Client associated with the engine.
func (engine *Engine) DBClient() *dbcore.Client {
	return engine.dbClient
}

// SetAuthManager attaches an authentication token manager or provider to the app engine.
func (engine *Engine) SetAuthManager(mgr any) {
	engine.authManager = mgr
}

// AuthManager returns the registered authentication manager handle.
func (engine *Engine) AuthManager() any {
	return engine.authManager
}

// DB returns the primary *sql.DB database connection handle.
func (engine *Engine) DB() *sql.DB {
	if engine.dbClient == nil {
		return nil
	}
	return engine.dbClient.DB()
}

// RegisterModel registers entity models and automatically creates their database tables.
func (engine *Engine) RegisterModel(ctx context.Context, models ...any) error {
	if engine.dbClient == nil {
		return fmt.Errorf("gpp: cannot register models without an active database connection")
	}

	for _, model := range models {
		if err := autoMigrateModelAny(ctx, engine.dbClient, model); err != nil {
			return fmt.Errorf("gpp: failed to auto-migrate model %T: %w", model, err)
		}
	}
	return nil
}

func initAutoDBClient(driver, path, dsn string) (*dbcore.Client, error) {
	ctx := context.Background()
	if dsn == "" {
		if path == "" {
			path = "app.db"
		}
		if path == ":memory:" {
			dsn = ":memory:"
		} else if strings.HasPrefix(path, "file:") {
			dsn = path
		} else {
			dsn = "file:" + path
		}
	}
	return dbcore.NewClient(ctx, dbcore.Config{RWDSN: dsn})
}

func autoMigrateModelAny(ctx context.Context, client *dbcore.Client, model any) error {
	switch m := model.(type) {
	case interface{ AutoMigrate(context.Context) error }:
		return m.AutoMigrate(ctx)
	default:
		return dbcore.AutoMigrateModel(ctx, client, model)
	}
}

func defaultSecurityMiddleware() HandlerFunc {
	return func(c *Context) error {
		c.SetHeader("X-Content-Type-Options", "nosniff")
		c.SetHeader("X-Frame-Options", "DENY")
		c.SetHeader("X-XSS-Protection", "1; mode=block")
		return c.Next()
	}
}

func defaultCORSMiddleware() HandlerFunc {
	return func(c *Context) error {
		c.SetHeader("Access-Control-Allow-Origin", "*")
		c.SetHeader("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		c.SetHeader("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, X-Requested-With")
		if c.Request.Method == "OPTIONS" {
			return c.NoContent()
		}
		return c.Next()
	}
}

func defaultRecoveryMiddleware() HandlerFunc {
	return func(c *Context) error {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("gpp: Panic recovered in handler", slog.Any("error", r))
				_ = c.InternalError("Internal server error")
			}
		}()
		return c.Next()
	}
}

func defaultLoggerMiddleware() HandlerFunc {
	return func(c *Context) error {
		start := time.Now()
		err := c.Next()
		slog.Info("HTTP Request",
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Duration("latency", time.Since(start)),
		)
		return err
	}
}
