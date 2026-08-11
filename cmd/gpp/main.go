package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	command := os.Args[1]
	switch command {
	case "new":
		if len(os.Args) < 3 {
			fmt.Println("❌ Error: Please provide an app name. Usage: gpp new <app_name>")
			return
		}
		scaffoldApp(os.Args[2])

	case "gen":
		if len(os.Args) < 4 {
			fmt.Println("❌ Error: Usage: gpp gen <module|middleware|migration|handler> <name>")
			return
		}
		targetType := strings.ToLower(os.Args[2])
		targetName := os.Args[3]

		switch targetType {
		case "module":
			generateModule(targetName)
		case "middleware":
			generateMiddleware(targetName)
		case "migration":
			generateMigration(targetName)
		case "handler":
			generateHandler(targetName)
		default:
			fmt.Printf("❌ Unknown generator target '%s'. Valid targets: module, middleware, migration, handler\n", targetType)
		}

	case "version":
		fmt.Println("goplusplus (gpp) CLI Tool v1.6.0")

	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println("🚀 goplusplus (gpp) CLI Tool v1.6.0")
	fmt.Println("Usage:")
	fmt.Println("  gpp new <app_name>            - Scaffold a full production-ready goplusplus application skeleton")
	fmt.Println("  gpp gen module <name>         - Generate a new domain module in modules/<name>")
	fmt.Println("  gpp gen middleware <name>     - Generate custom middleware in middleware/<name>.go")
	fmt.Println("  gpp gen migration <name>      - Generate timestamped SQL migration in migrations/<timestamp>_<name>.sql")
	fmt.Println("  gpp gen handler <name>        - Generate HTTP Handler function in handlers/<name>.go")
	fmt.Println("  gpp version                   - Display framework version")
}

func scaffoldApp(appName string) {
	fmt.Printf("🚀 Scaffolding full production-ready goplusplus app '%s'...\n", appName)

	dirs := []string{
		appName,
		filepath.Join(appName, "config"),
		filepath.Join(appName, "handlers"),
		filepath.Join(appName, "middleware"),
		filepath.Join(appName, "migrations"),
		filepath.Join(appName, "modules", "user"),
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			fmt.Printf("❌ Error creating directory %s: %v\n", d, err)
			return
		}
	}

	// 1. main.go
	mainContent := fmt.Sprintf(`package main

import (
	"context"
	"fmt"
	"net/http"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/dbcore"
	"github.com/saifsilver/goplusplus/middleware"
	"%s/config"
	"%s/handlers"
	"%s/modules/user"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()

	db, _ := dbcore.NewClient(ctx, dbcore.Config{
		RWDSN: cfg.DatabaseURL,
	})

	app := gpp.New()

	// 1-Liner Binary CLI Flag Handler (./myapp migrate, ./myapp seed)
	if app.HandleCLI(gpp.CLIOptions{Client: db}) {
		return
	}

	// Global Security & Observability Middleware
	app.Use(
		middleware.Observability(),
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Security(),
		middleware.CORS(),
		middleware.RequestID(),
	)

	// API Groups
	v1 := app.Group("/api/v1")
	v1.GET("/health", handlers.HealthHandler)

	// Register Domain Modules
	app.RegisterModule("/api/v1/users", &user.Module{})

	// Interactive OpenAPI & GraphQL Endpoints
	app.GET("/swagger", app.AutoSwaggerUI())
	app.GET("/graphql", app.AutoGraphQLPlayground("/graphql"))

	fmt.Printf("🚀 Server running on http://localhost:%%s\n", cfg.Port)
	if err := app.Listen(":" + cfg.Port); err != nil {
		panic(err)
	}
}
`, appName, appName, appName)

	_ = os.WriteFile(filepath.Join(appName, "main.go"), []byte(mainContent), 0644)

	// 2. config/config.go
	configContent := `package config

import "os"

type Config struct {
	Port        string
	DatabaseURL string
	Environment string
}

func Load() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = ":memory:"
	}
	return Config{
		Port:        port,
		DatabaseURL: dbURL,
		Environment: "development",
	}
}
`
	_ = os.WriteFile(filepath.Join(appName, "config", "config.go"), []byte(configContent), 0644)

	// 3. handlers/health.go
	healthContent := `package handlers

import (
	"net/http"

	gpp "github.com/saifsilver/goplusplus"
)

func HealthHandler(c *gpp.Context) error {
	return c.JSON(http.StatusOK, gpp.H{
		"status":     "UP",
		"request_id": c.RequestID(),
	})
}
`
	_ = os.WriteFile(filepath.Join(appName, "handlers", "health.go"), []byte(healthContent), 0644)

	// 4. middleware/custom.go
	middlewareContent := `package middleware

import (
	gpp "github.com/saifsilver/goplusplus"
)

func CustomHeader() gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		c.SetHeader("X-Framework", "goplusplus")
		return c.Next()
	}
}
`
	_ = os.WriteFile(filepath.Join(appName, "middleware", "custom.go"), []byte(middlewareContent), 0644)

	// 5. migrations/0001_init.sql
	migrationContent := `-- UP Migration: Create users table
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	email TEXT UNIQUE NOT NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- DOWN Migration
-- DROP TABLE IF EXISTS users;
`
	_ = os.WriteFile(filepath.Join(appName, "migrations", "0001_init.sql"), []byte(migrationContent), 0644)

	// 6. modules/user/module.go
	userModuleContent := `package user

import (
	"net/http"

	gpp "github.com/saifsilver/goplusplus"
)

type Module struct{}

func (m *Module) Name() string { return "UserModule" }

func (m *Module) Register(group *gpp.RouterGroup) {
	group.GET("/profile/:id", m.getProfile)
}

func (m *Module) getProfile(c *gpp.Context) error {
	id := c.Param("id")
	return c.JSON(http.StatusOK, gpp.H{
		"id":   id,
		"name": "Alex Dev",
		"role": "admin",
	})
}
`
	_ = os.WriteFile(filepath.Join(appName, "modules", "user", "module.go"), []byte(userModuleContent), 0644)

	// 7. Makefile
	makefileContent := fmt.Sprintf(`.PHONY: build run migrate seed docker

build:
	go build -o bin/%s main.go

run:
	go run main.go

migrate:
	go run main.go migrate

seed:
	go run main.go seed

docker:
	docker build -t %s:latest .
`, appName, appName)
	_ = os.WriteFile(filepath.Join(appName, "Makefile"), []byte(makefileContent), 0644)

	// 8. Dockerfile
	dockerfileContent := fmt.Sprintf(`FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server main.go

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/server .
EXPOSE 8080
CMD ["./server"]
`)
	_ = os.WriteFile(filepath.Join(appName, "Dockerfile"), []byte(dockerfileContent), 0644)

	fmt.Printf("✅ Application skeleton '%s' created successfully!\n", appName)
	fmt.Printf("👉 Run 'cd %s && go run main.go' to launch server!\n", appName)
}

func generateModule(moduleName string) {
	pkgName := strings.ToLower(moduleName)
	dir := filepath.Join("modules", pkgName)
	fmt.Printf("🧱 Generating domain module '%s' in %s/...\n", moduleName, dir)
	_ = os.MkdirAll(dir, 0755)

	titleName := capitalize(moduleName)
	content := fmt.Sprintf(`package %s

import (
	"net/http"

	gpp "github.com/saifsilver/goplusplus"
)

type Module struct{}

func (m *Module) Name() string { return "%sModule" }

func (m *Module) Register(group *gpp.RouterGroup) {
	group.GET("/:id", m.get%s)
	group.POST("/", m.create%s)
}

func (m *Module) get%s(c *gpp.Context) error {
	id := c.Param("id")
	return c.JSON(http.StatusOK, gpp.H{
		"id":     id,
		"module": "%s",
		"status": "active",
	})
}

func (m *Module) create%s(c *gpp.Context) error {
	return c.JSON(http.StatusCreated, gpp.H{
		"message": "%s created successfully",
	})
}
`, pkgName, titleName, titleName, titleName, titleName, titleName, titleName, titleName)

	_ = os.WriteFile(filepath.Join(dir, "module.go"), []byte(content), 0644)
	fmt.Printf("✅ Domain module '%s' generated in %s/module.go!\n", titleName, dir)
}

func generateMiddleware(name string) {
	dir := "middleware"
	_ = os.MkdirAll(dir, 0755)
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

	_ = os.WriteFile(filePath, []byte(content), 0644)
	fmt.Printf("✅ Custom middleware generated in %s!\n", filePath)
}

func generateMigration(name string) {
	dir := "migrations"
	_ = os.MkdirAll(dir, 0755)
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

	_ = os.WriteFile(filePath, []byte(content), 0644)
	fmt.Printf("✅ Migration file generated in %s!\n", filePath)
}

func generateHandler(name string) {
	dir := "handlers"
	_ = os.MkdirAll(dir, 0755)
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

	_ = os.WriteFile(filePath, []byte(content), 0644)
	fmt.Printf("✅ Handler generated in %s!\n", filePath)
}

func capitalize(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
