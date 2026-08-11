package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
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
		if isTerminal(os.Stdin) {
			return runInteractiveCLI(os.Stdin, os.Stdout)
		}
		printUsage()
		return nil
	}

	command := args[0]
	switch command {
	case "interactive", "-i", "--interactive":
		return runInteractiveCLI(os.Stdin, os.Stdout)

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

func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func runInteractiveCLI(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	fmt.Fprintf(w, "\n🚀 Welcome to goplusplus (gpp) Interactive CLI %s\n\n", cliVersion)
	fmt.Fprintln(w, "Select an action:")
	fmt.Fprintln(w, "  1) 🚀 Scaffold new project (gpp new)")
	fmt.Fprintln(w, "  2) 📦 Generate domain module (gpp gen module)")
	fmt.Fprintln(w, "  3) 🛡️ Generate custom middleware (gpp gen middleware)")
	fmt.Fprintln(w, "  4) 🗄️ Generate SQL migration (gpp gen migration)")
	fmt.Fprintln(w, "  5) ⚡ Generate HTTP handler (gpp gen handler)")
	fmt.Fprintln(w, "  6) ☁️ Generate AWS ECS/RDS Terraform (gpp gen terraform aws)")
	fmt.Fprintln(w, "  7) 🐳 Generate Docker Compose VPS hosting (gpp gen hosting standard)")
	fmt.Fprintln(w, "  8) 🧩 Extract module as microservice (gpp extract service)")
	fmt.Fprintln(w, "  9) ℹ️  Display version")
	fmt.Fprintln(w, " 10) ❌ Exit")
	fmt.Fprintln(w)

	choice, err := promptInput(scanner, w, "Enter choice [1-10]", "1")
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	switch choice {
	case "1":
		appName, err := promptInput(scanner, w, "App name / directory (e.g. myapp)", "")
		if err != nil || appName == "" {
			return errors.New("app name is required")
		}
		modPath, err := promptInput(scanner, w, "Go module import path", appName)
		if err != nil {
			return err
		}
		options, err := parseScaffoldOptions([]string{appName, "--module", modPath})
		if err != nil {
			return err
		}
		return scaffoldApp(options)

	case "2":
		modName, err := promptInput(scanner, w, "Module name (e.g. orders)", "")
		if err != nil || modName == "" {
			return errors.New("module name is required")
		}
		if !generatorNamePattern.MatchString(modName) {
			return fmt.Errorf("generator name %q must be a Go identifier", modName)
		}
		return generateModule(modName)

	case "3":
		mwName, err := promptInput(scanner, w, "Middleware name (e.g. auth)", "")
		if err != nil || mwName == "" {
			return errors.New("middleware name is required")
		}
		if !generatorNamePattern.MatchString(mwName) {
			return fmt.Errorf("generator name %q must be a Go identifier", mwName)
		}
		return generateMiddleware(mwName)

	case "4":
		migName, err := promptInput(scanner, w, "Migration name (e.g. create_users_table)", "")
		if err != nil || migName == "" {
			return errors.New("migration name is required")
		}
		if !generatorNamePattern.MatchString(migName) {
			return fmt.Errorf("generator name %q must be a Go identifier", migName)
		}
		return generateMigration(migName)

	case "5":
		hName, err := promptInput(scanner, w, "Handler name (e.g. get_user)", "")
		if err != nil || hName == "" {
			return errors.New("handler name is required")
		}
		if !generatorNamePattern.MatchString(hName) {
			return fmt.Errorf("generator name %q must be a Go identifier", hName)
		}
		return generateHandler(hName)

	case "6":
		return generateTerraform("aws")

	case "7":
		return generateHosting("standard")

	case "8":
		capability, err := promptInput(scanner, w, "Module to extract (e.g. users)", "")
		if err != nil || capability == "" {
			return errors.New("module name is required")
		}
		modPath, err := promptInput(scanner, w, "Microservice Go module path (e.g. example.com/acme/users-service)", "")
		if err != nil || modPath == "" {
			return errors.New("microservice Go module path is required")
		}
		outDir, err := promptInput(scanner, w, "Output directory", filepath.Join("services", capability))
		if err != nil {
			return err
		}
		routePath, err := promptInput(scanner, w, "HTTP route prefix", "/api/v1/"+capability)
		if err != nil {
			return err
		}
		options, err := parseExtractOptions([]string{
			"service", capability,
			"--module", modPath,
			"--output", outDir,
			"--route", routePath,
		})
		if err != nil {
			return err
		}
		return extractService(options)

	case "9":
		fmt.Fprintf(w, "goplusplus (gpp) CLI Tool %s\n", cliVersion)
		return nil

	case "10":
		fmt.Fprintln(w, "Goodbye! 👋")
		return nil

	default:
		return fmt.Errorf("invalid choice %q", choice)
	}
}

func promptInput(scanner *bufio.Scanner, writer io.Writer, promptText, defaultValue string) (string, error) {
	if defaultValue != "" {
		fmt.Fprintf(writer, "%s [%s]: ", promptText, defaultValue)
	} else {
		fmt.Fprintf(writer, "%s: ", promptText)
	}
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", io.EOF
	}
	val := strings.TrimSpace(scanner.Text())
	if val == "" {
		val = defaultValue
	}
	return val, nil
}

func printUsage() {
	fmt.Printf("🚀 goplusplus (gpp) CLI Tool %s\n", cliVersion)
	fmt.Println("Usage:")
	fmt.Println("  gpp                           - Launch interactive wizard (when run in TTY terminal)")
	fmt.Println("  gpp -i, --interactive         - Force interactive wizard mode")
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
