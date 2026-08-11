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
	MaxUint   uint64        `default:"500"`
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
	if cfg.MaxUint != 500 {
		t.Errorf("expected MaxUint 500, got %d", cfg.MaxUint)
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
	content := []byte("PORT=5000\nJWT_SECRET=env-file-secret\n# Comment line\n\nINVALID_LINE\n")

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

func TestHelpersAndGetters(t *testing.T) {
	os.Setenv("APP_NAME", "go++")
	os.Setenv("APP_PORT", "8080")
	os.Setenv("APP_DEBUG", "true")
	os.Setenv("APP_TIMEOUT", "10s")
	defer func() {
		os.Unsetenv("APP_NAME")
		os.Unsetenv("APP_PORT")
		os.Unsetenv("APP_DEBUG")
		os.Unsetenv("APP_TIMEOUT")
	}()

	if Get("APP_NAME") != "go++" {
		t.Errorf("Get failed")
	}
	if Env("APP_NAME") != "go++" {
		t.Errorf("Env failed")
	}
	if GetInt("APP_PORT", 3000) != 8080 {
		t.Errorf("GetInt failed")
	}
	if GetInt("NON_EXISTENT", 9000) != 9000 {
		t.Errorf("GetInt default failed")
	}
	if !GetBool("APP_DEBUG", false) {
		t.Errorf("GetBool failed")
	}
	if GetBool("NON_EXISTENT", true) != true {
		t.Errorf("GetBool default failed")
	}
	if GetDuration("APP_TIMEOUT", time.Minute) != 10*time.Second {
		t.Errorf("GetDuration failed")
	}
	if GetDuration("NON_EXISTENT", 5*time.Second) != 5*time.Second {
		t.Errorf("GetDuration default failed")
	}

	if MaskSecret("secret123456") != "sec...456" {
		t.Errorf("MaskSecret long failed")
	}
	if MaskSecret("short") != "******" {
		t.Errorf("MaskSecret short failed")
	}

	if err := Unmarshal("not_a_pointer_to_struct"); err == nil {
		t.Errorf("expected error for non-pointer struct target")
	}
}

type InlineTagConfig struct {
	Host     string `config:"APP_HOST,default=localhost"`
	ApiKey   string `env:"API_KEY,required"`
	Required string `required:"true"`
}

func TestAdvancedTagOptions(t *testing.T) {
	// Missing required fields should error
	var cfg InlineTagConfig
	if err := Unmarshal(&cfg); err == nil {
		t.Errorf("expected error for missing required fields, got nil")
	}

	os.Setenv("API_KEY", "secret-key")
	os.Setenv("REQUIRED", "value")
	defer func() {
		os.Unsetenv("API_KEY")
		os.Unsetenv("REQUIRED")
	}()

	var cfg2 InlineTagConfig
	if err := Unmarshal(&cfg2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg2.Host != "localhost" {
		t.Errorf("expected Host 'localhost', got '%s'", cfg2.Host)
	}
	if cfg2.ApiKey != "secret-key" {
		t.Errorf("expected ApiKey 'secret-key', got '%s'", cfg2.ApiKey)
	}
	if cfg2.Required != "value" {
		t.Errorf("expected Required 'value', got '%s'", cfg2.Required)
	}
}
