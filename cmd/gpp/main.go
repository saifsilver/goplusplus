package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var generatorNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

func main() {
	if err := runCLI(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}
}

func runCLI(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	command := args[0]
	switch command {
	case "new":
		options, err := parseScaffoldOptions(args[1:])
		if err != nil {
			return err
		}
		return scaffoldApp(options)

	case "gen":
		if len(args) < 3 {
			return fmt.Errorf("usage: gpp gen <module|middleware|migration|handler|terraform|hosting> <name>")
		}
		targetType := strings.ToLower(args[1])
		targetName := args[2]
		if !generatorNamePattern.MatchString(targetName) {
			return fmt.Errorf("generator name %q must be a Go identifier", targetName)
		}

		switch targetType {
		case "module":
			return generateModule(targetName)
		case "middleware":
			return generateMiddleware(targetName)
		case "migration":
			return generateMigration(targetName)
		case "handler":
			return generateHandler(targetName)
		case "terraform":
			return generateTerraform(targetName)
		case "hosting":
			return generateHosting(targetName)
		default:
			return fmt.Errorf("unknown generator target %q; valid targets: module, middleware, migration, handler, terraform, hosting", targetType)
		}

	case "extract":
		options, err := parseExtractOptions(args[1:])
		if err != nil {
			return err
		}
		return extractService(options)

	case "version":
		fmt.Printf("goplusplus (gpp) CLI Tool %s\n", cliVersion)

	default:
		printUsage()
	}
	return nil
}

func printUsage() {
	fmt.Printf("🚀 goplusplus (gpp) CLI Tool %s\n", cliVersion)
	fmt.Println("Usage:")
	fmt.Println("  gpp new <app_name> [--module <path>] - Scaffold a scalable modular-monolith application")
	fmt.Println("  gpp gen module <name>         - Generate a new domain module in internal/modules/<name>")
	fmt.Println("  gpp gen middleware <name>     - Generate custom middleware in middleware/<name>.go")
	fmt.Println("  gpp gen migration <name>      - Generate timestamped SQL migration in migrations/<timestamp>_<name>.sql")
	fmt.Println("  gpp gen handler <name>        - Generate HTTP Handler function in handlers/<name>.go")
	fmt.Println("  gpp gen terraform aws         - Generate scalable AWS ECS/RDS Terraform")
	fmt.Println("  gpp gen hosting standard      - Generate Docker Compose VPS hosting")
	fmt.Println("  gpp extract service <module> --module <path> [--output <dir>] - Extract a module as an HTTP microservice")
	fmt.Println("  gpp version                   - Display framework version")
}

func generateMiddleware(name string) error {
	dir := "middleware"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create middleware directory: %w", err)
	}
	fileName := strings.ToLower(name) + ".go"
	filePath := filepath.Join(dir, fileName)

	funcName := capitalize(name)
	content := fmt.Sprintf(`package middleware

import (
	gpp "github.com/saifsilver/goplusplus"
)

// %s returns custom HTTP middleware.
func %s() gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		// Middleware pre-handler execution logic
		c.SetHeader("X-Custom-%s", "enabled")

		if err := c.Next(); err != nil {
			return err
		}

		// Middleware post-handler execution logic
		return nil
	}
}
`, funcName, funcName, funcName)

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write middleware file: %w", err)
	}
	fmt.Printf("✅ Custom middleware generated in %s!\n", filePath)
	return nil
}

func generateMigration(name string) error {
	dir := "migrations"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create migration directory: %w", err)
	}
	timestamp := time.Now().Format("20060102150405")
	fileName := fmt.Sprintf("%s_%s.sql", timestamp, strings.ToLower(name))
	filePath := filepath.Join(dir, fileName)

	content := fmt.Sprintf(`-- UP Migration: %s
CREATE TABLE IF NOT EXISTS %s (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- DOWN Migration
-- DROP TABLE IF EXISTS %s;
`, name, strings.ToLower(name), strings.ToLower(name))

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write migration file: %w", err)
	}
	fmt.Printf("✅ Migration file generated in %s!\n", filePath)
	return nil
}

func generateHandler(name string) error {
	dir := "handlers"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create handler directory: %w", err)
	}
	fileName := strings.ToLower(name) + ".go"
	filePath := filepath.Join(dir, fileName)

	funcName := capitalize(name) + "Handler"
	content := fmt.Sprintf(`package handlers

import (
	"net/http"

	gpp "github.com/saifsilver/goplusplus"
)

// %s handles requests for %s.
func %s(c *gpp.Context) error {
	id := c.Param("id")
	userID := c.GetInt64("user_id")

	return c.JSON(http.StatusOK, gpp.H{
		"id":      id,
		"user_id": userID,
		"handler": "%s",
	})
}
`, funcName, name, funcName, funcName)

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write handler file: %w", err)
	}
	fmt.Printf("✅ Handler generated in %s!\n", filePath)
	return nil
}

func capitalize(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
