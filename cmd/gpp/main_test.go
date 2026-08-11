package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCLIScaffoldApp(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gpp_test_app_*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	appName := filepath.Join(tempDir, "sampleapp")
	scaffoldApp(appName)

	expectedFiles := []string{
		filepath.Join(appName, "main.go"),
		filepath.Join(appName, "config", "config.go"),
		filepath.Join(appName, "handlers", "health.go"),
		filepath.Join(appName, "middleware", "custom.go"),
		filepath.Join(appName, "migrations", "0001_init.sql"),
		filepath.Join(appName, "modules", "user", "module.go"),
		filepath.Join(appName, "Makefile"),
		filepath.Join(appName, "Dockerfile"),
	}

	for _, file := range expectedFiles {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			t.Errorf("expected generated file '%s' to exist", file)
		}
	}
}

func TestCLIGenerators(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gpp_test_gen_*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origWd, _ := os.Getwd()
	_ = os.Chdir(tempDir)
	defer func() { _ = os.Chdir(origWd) }()

	generateModule("order")
	if _, err := os.Stat(filepath.Join("modules", "order", "module.go")); os.IsNotExist(err) {
		t.Errorf("generateModule failed to create modules/order/module.go")
	}

	generateMiddleware("auth_header")
	if _, err := os.Stat(filepath.Join("middleware", "auth_header.go")); os.IsNotExist(err) {
		t.Errorf("generateMiddleware failed to create middleware/auth_header.go")
	}

	generateMigration("create_orders")
	entries, _ := os.ReadDir("migrations")
	if len(entries) == 0 {
		t.Errorf("generateMigration failed to create SQL file in migrations/")
	}

	generateHandler("user_profile")
	if _, err := os.Stat(filepath.Join("handlers", "user_profile.go")); os.IsNotExist(err) {
		t.Errorf("generateHandler failed to create handlers/user_profile.go")
	}
}
