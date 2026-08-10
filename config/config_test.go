package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type TestAppConfig struct {
	Port      string        `default:"8080"`
	JWTSecret string        `default:"goplusplus-default-jwt-secret-key-2026"`
	DBPath    string        `default:"data/todos.db"`
	Env       string        `default:"development"`
	Timeout   time.Duration `default:"15s"`
	MaxConns  int           `default:"100"`
	Debug     bool          `default:"true"`
	Ratio     float64       `default:"0.75"`
}

type TaggedConfig struct {
	CustomPort string `env:"SERVER_PORT" default:"9090"`
}

func TestMustLoadAndAutoSnakeCase(t *testing.T) {
	// Set environment variables to test overriding defaults
	os.Setenv("PORT", "3000")
	os.Setenv("JWT_SECRET", "custom-secret-123")
	os.Setenv("TIMEOUT", "45s")

	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("TIMEOUT")
	}()

	cfg := MustLoad[TestAppConfig]()

	if cfg.Port != "3000" {
		t.Errorf("expected Port '3000', got '%s'", cfg.Port)
	}
	if cfg.JWTSecret != "custom-secret-123" {
		t.Errorf("expected JWTSecret 'custom-secret-123', got '%s'", cfg.JWTSecret)
	}
	if cfg.DBPath != "data/todos.db" {
		t.Errorf("expected DBPath 'data/todos.db', got '%s'", cfg.DBPath)
	}
	if cfg.Env != "development" {
		t.Errorf("expected Env 'development', got '%s'", cfg.Env)
	}
	if cfg.Timeout != 45*time.Second {
		t.Errorf("expected Timeout 45s, got %v", cfg.Timeout)
	}
	if cfg.MaxConns != 100 {
		t.Errorf("expected MaxConns 100, got %d", cfg.MaxConns)
	}
	if !cfg.Debug {
		t.Errorf("expected Debug true, got false")
	}
	if cfg.Ratio != 0.75 {
		t.Errorf("expected Ratio 0.75, got %f", cfg.Ratio)
	}
}

func TestTaggedConfig(t *testing.T) {
	os.Setenv("SERVER_PORT", "9999")
	defer os.Unsetenv("SERVER_PORT")

	cfg, err := LoadStruct[TaggedConfig]()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CustomPort != "9999" {
		t.Errorf("expected CustomPort '9999', got '%s'", cfg.CustomPort)
	}
}

func TestLoadDotEnvFile(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")
	content := []byte("PORT=5000\nJWT_SECRET=env-file-secret\n")

	if err := os.WriteFile(envFile, content, 0644); err != nil {
		t.Fatalf("failed to write temp .env file: %v", err)
	}

	var cfg TestAppConfig
	if err := Bind(&cfg, envFile); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	if cfg.Port != "5000" {
		t.Errorf("expected Port '5000' from .env, got '%s'", cfg.Port)
	}
	if cfg.JWTSecret != "env-file-secret" {
		t.Errorf("expected JWTSecret 'env-file-secret' from .env, got '%s'", cfg.JWTSecret)
	}
}

func TestHelpers(t *testing.T) {
	os.Setenv("APP_NAME", "go++")
	defer os.Unsetenv("APP_NAME")

	if Get("APP_NAME") != "go++" {
		t.Errorf("Get failed")
	}
	if Env("APP_NAME") != "go++" {
		t.Errorf("Env failed")
	}
	if MaskSecret("secret123456") != "sec...456" {
		t.Errorf("MaskSecret failed")
	}
}
