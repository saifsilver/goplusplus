package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoaderPrecedenceNormalizationValidationAndIsolation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("PORT=7000\nENV= DEVELOPMENT \nTIMEOUT=3s\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loader, err := NewLoader(LoaderOptions{DotEnvPath: path, Environment: map[string]string{"PORT": "8000"}})
	if err != nil {
		t.Fatal(err)
	}
	type appConfig struct {
		Port    int           `env:"PORT,required"`
		Env     string        `env:"ENV" default:"development"`
		Timeout time.Duration `env:"TIMEOUT" default:"1s"`
	}
	var cfg appConfig
	err = loader.Load(&cfg, func() error { cfg.Env = strings.ToLower(strings.TrimSpace(cfg.Env)); return nil }, func() error {
		if err := Port("Port", cfg.Port); err != nil {
			return err
		}
		return OneOf("Env", cfg.Env, "development", "production")
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 8000 || cfg.Env != "development" || cfg.Timeout != 3*time.Second {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if os.Getenv("PORT") == "8000" {
		t.Fatal("isolated loader mutated process environment")
	}
}

func TestLoaderRejectsMalformedAndRedactsValues(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("NOT VALID secret-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewLoader(LoaderOptions{DotEnvPath: path, Environment: map[string]string{}})
	if err == nil || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("unsafe malformed file error: %v", err)
	}
	loader, err := NewLoader(LoaderOptions{Environment: map[string]string{"SECRET": "very-secret-value"}})
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Secret int `env:"SECRET"`
	}
	err = loader.Load(&cfg, nil, nil)
	if err == nil || strings.Contains(err.Error(), "very-secret-value") {
		t.Fatalf("unsafe conversion error: %v", err)
	}
}
