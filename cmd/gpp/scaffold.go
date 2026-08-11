package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	gpp "github.com/saifsilver/goplusplus"
)

var modulePathPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~/-]*$`)

const cliVersion = gpp.Version

type scaffoldOptions struct {
	Directory  string
	ModulePath string
}

type scaffoldFile struct {
	path    string
	content string
	mode    fs.FileMode
}

func parseScaffoldOptions(args []string) (scaffoldOptions, error) {
	if len(args) == 0 {
		return scaffoldOptions{}, errors.New("usage: gpp new <app_name> [--module <module_path>]")
	}

	flags := flag.NewFlagSet("new", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	modulePath := flags.String("module", "", "Go module import path")
	if err := flags.Parse(args[1:]); err != nil {
		return scaffoldOptions{}, fmt.Errorf("parse new command: %w", err)
	}
	if flags.NArg() != 0 {
		return scaffoldOptions{}, errors.New("usage: gpp new <app_name> [--module <module_path>]")
	}

	directory := filepath.Clean(strings.TrimSpace(args[0]))
	if directory == "." || directory == string(filepath.Separator) {
		return scaffoldOptions{}, errors.New("app name must identify a new directory")
	}
	if *modulePath == "" {
		*modulePath = filepath.Base(directory)
	}
	if err := validateModulePath(*modulePath); err != nil {
		return scaffoldOptions{}, err
	}
	return scaffoldOptions{Directory: directory, ModulePath: *modulePath}, nil
}

func validateModulePath(modulePath string) error {
	if strings.TrimSpace(modulePath) != modulePath || !modulePathPattern.MatchString(modulePath) {
		return fmt.Errorf("invalid Go module path %q", modulePath)
	}
	if strings.Contains(modulePath, "//") {
		return fmt.Errorf("invalid Go module path %q", modulePath)
	}
	for _, segment := range strings.Split(modulePath, "/") {
		if segment == "." || segment == ".." || segment == "" {
			return fmt.Errorf("invalid Go module path %q", modulePath)
		}
	}
	return nil
}

func scaffoldApp(options scaffoldOptions) (err error) {
	if _, err := os.Stat(options.Directory); err == nil {
		return fmt.Errorf("target directory %q already exists; refusing to overwrite it", options.Directory)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect target directory: %w", err)
	}

	parent := filepath.Dir(options.Directory)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create target parent: %w", err)
	}
	temporary, err := os.MkdirTemp(parent, ".gpp-scaffold-")
	if err != nil {
		return fmt.Errorf("create temporary scaffold: %w", err)
	}
	defer func() {
		if temporary != "" {
			err = errors.Join(err, os.RemoveAll(temporary))
		}
	}()

	for _, file := range scaffoldFiles(options) {
		path := filepath.Join(temporary, file.path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create directory for %s: %w", file.path, err)
		}
		if err := os.WriteFile(path, []byte(file.content), file.mode); err != nil {
			return fmt.Errorf("write %s: %w", file.path, err)
		}
	}

	if err := os.Rename(temporary, options.Directory); err != nil {
		return fmt.Errorf("publish scaffold: %w", err)
	}
	temporary = ""
	fmt.Printf("✅ Scalable modular-monolith %q created.\n", options.Directory)
	fmt.Printf("👉 Run: cd %s && go mod tidy && make run\n", options.Directory)
	return nil
}

func scaffoldFiles(options scaffoldOptions) []scaffoldFile {
	replacer := strings.NewReplacer(
		"{{MODULE}}", options.ModulePath,
		"{{APP_NAME}}", filepath.Base(options.Directory),
		"{{FRAMEWORK_VERSION}}", gpp.Version,
	)
	file := func(path, content string) scaffoldFile {
		return scaffoldFile{path: path, content: replacer.Replace(content), mode: 0o644}
	}

	return []scaffoldFile{
		file("go.mod", scaffoldGoMod),
		file("cmd/api/main.go", scaffoldMain),
		file("internal/application/application.go", scaffoldApplication),
		file("internal/config/config.go", scaffoldConfig),
		file("internal/config/config_test.go", scaffoldConfigTest),
		file("internal/modules/system/module.go", scaffoldSystemModule),
		file("internal/modules/users/domain.go", scaffoldUsersDomain),
		file("internal/modules/users/repository.go", scaffoldUsersRepository),
		file("internal/modules/users/service.go", scaffoldUsersService),
		file("internal/modules/users/module.go", scaffoldUsersModule),
		file("internal/modules/users/migrations.go", scaffoldMigrations),
		file("internal/modules/users/migrations/0001_users.sql", scaffoldUsersMigration),
		file("internal/application/application_test.go", scaffoldApplicationTest),
		file("data/.gitkeep", ""),
		file(".env.example", scaffoldEnv),
		file(".gitignore", scaffoldGitignore),
		file(".dockerignore", scaffoldDockerignore),
		file("Dockerfile", scaffoldDockerfile),
		file("Makefile", scaffoldMakefile),
		file("README.md", scaffoldReadme),
	}
}

const scaffoldGoMod = `module {{MODULE}}

go 1.26.5

require github.com/saifsilver/goplusplus {{FRAMEWORK_VERSION}}
`

const scaffoldMain = `package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"{{MODULE}}/internal/application"
	"{{MODULE}}/internal/config"
)

func main() {
	if err := run(); err != nil {
		slog.Error("application stopped", "error", err)
		os.Exit(1)
	}
}

func run() (err error) {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	app, err := application.New(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("initialize application: %w", err)
	}
	defer func() { err = errors.Join(err, app.Close()) }()
	return app.Run(cfg.HTTPAddress)
}
`

const scaffoldApplication = `package application

import (
	"context"
	"fmt"
	"time"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/dbcore"
	"github.com/saifsilver/goplusplus/middleware"

	"{{MODULE}}/internal/config"
	"{{MODULE}}/internal/modules/system"
	"{{MODULE}}/internal/modules/users"
)

type Application struct {
	Engine   *gpp.Engine
	database *dbcore.Client
}

func New(ctx context.Context, cfg config.Config) (*Application, error) {
	database, err := dbcore.NewClient(ctx, dbcore.Config{
		RWDSN: cfg.DatabaseURL, MaxOpenConnections: 25, MaxIdleConnections: 10,
		ConnectionMaxLifetime: 30 * time.Minute, ConnectionMaxIdleTime: 5 * time.Minute,
		PingTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := dbcore.AutoMigrate(ctx, database, users.Migrations()...); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	engine := gpp.New()
	cors := middleware.DefaultCORSConfig()
	cors.AllowOrigins = cfg.CORSOrigins
	engine.Use(
		middleware.Observability(), middleware.Logger(), middleware.Recovery(),
		middleware.RequestID(), middleware.Security(), middleware.CORS(cors),
	)
	engine.RegisterModule("/health", system.New(database))

	engine.Group("/api/v1").RegisterModule("/users", users.Build(database))
	engine.GET("/metrics", middleware.Prometheus())
	if cfg.EnableDocs {
		engine.GET("/swagger", engine.AutoSwaggerUI())
	}

	return &Application{Engine: engine, database: database}, nil
}

func (app *Application) Run(address string) error {
	return app.Engine.Listen(address)
}

func (app *Application) Close() error {
	return app.database.Close()
}
`

const scaffoldConfig = `package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	Environment string
	HTTPAddress string
	DatabaseURL string
	CORSOrigins []string
	EnableDocs  bool
}

func Load() (Config, error) {
	environment := valueOrDefault("APP_ENV", "development")
	databaseURL, err := loadDatabaseURL(environment)
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		Environment: environment,
		HTTPAddress: valueOrDefault("HTTP_ADDRESS", ":8080"),
		DatabaseURL: databaseURL,
		CORSOrigins: splitCSV(os.Getenv("CORS_ALLOWED_ORIGINS")),
		EnableDocs:  environment != "production",
	}
	if len(cfg.CORSOrigins) == 0 && cfg.Environment == "development" {
		cfg.CORSOrigins = []string{"http://localhost:3000"}
	}
	return cfg, cfg.Validate()
}

func loadDatabaseURL(environment string) (string, error) {
	if databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL")); databaseURL != "" {
		return databaseURL, nil
	}
	host := strings.TrimSpace(os.Getenv("DB_HOST"))
	if host == "" {
		if environment == "development" {
			return "file:data/{{APP_NAME}}.db", nil
		}
		return "", nil
	}
	name := strings.TrimSpace(os.Getenv("DB_NAME"))
	user := strings.TrimSpace(os.Getenv("DB_USER"))
	if name == "" || user == "" {
		return "", errors.New("DB_NAME and DB_USER are required when DB_HOST is set")
	}
	password, err := loadDatabasePassword()
	if err != nil {
		return "", err
	}
	if password == "" {
		return "", errors.New("DB_PASSWORD or DB_PASSWORD_FILE is required when DB_HOST is set")
	}
	databaseURL := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   net.JoinHostPort(host, valueOrDefault("DB_PORT", "5432")),
		Path:   "/" + name,
	}
	query := databaseURL.Query()
	query.Set("sslmode", valueOrDefault("DB_SSLMODE", "require"))
	databaseURL.RawQuery = query.Encode()
	return databaseURL.String(), nil
}

func loadDatabasePassword() (string, error) {
	password := os.Getenv("DB_PASSWORD")
	passwordFile := strings.TrimSpace(os.Getenv("DB_PASSWORD_FILE"))
	if password != "" && passwordFile != "" {
		return "", errors.New("DB_PASSWORD and DB_PASSWORD_FILE are mutually exclusive")
	}
	if passwordFile == "" {
		return password, nil
	}
	content, err := os.ReadFile(passwordFile)
	if err != nil {
		return "", fmt.Errorf("read DB_PASSWORD_FILE: %w", err)
	}
	return strings.TrimSpace(string(content)), nil
}

func (cfg Config) Validate() error {
	if cfg.Environment != "development" && cfg.Environment != "test" && cfg.Environment != "production" {
		return errors.New("APP_ENV must be development, test, or production")
	}
	if _, _, err := net.SplitHostPort(cfg.HTTPAddress); err != nil {
		return errors.New("HTTP_ADDRESS must be in host:port form")
	}
	if cfg.DatabaseURL == "" {
		return errors.New("DATABASE_URL is required outside development")
	}
	if cfg.Environment == "production" && (cfg.DatabaseURL == ":memory:" || strings.HasPrefix(cfg.DatabaseURL, "file:")) {
		return errors.New("production requires a PostgreSQL DATABASE_URL")
	}
	if cfg.Environment == "production" && contains(cfg.CORSOrigins, "*") {
		return errors.New("production CORS_ALLOWED_ORIGINS cannot contain a wildcard")
	}
	return nil
}

func valueOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func splitCSV(value string) []string {
	var values []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			values = append(values, item)
		}
	}
	return values
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
`

const scaffoldConfigTest = `package config

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestProductionRequiresPostgres(t *testing.T) {
	cfg := Config{Environment: "production", HTTPAddress: ":8080", DatabaseURL: "file:app.db"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected production SQLite configuration to be rejected")
	}
}

func TestProductionRejectsWildcardCORS(t *testing.T) {
	cfg := Config{
		Environment: "production", HTTPAddress: ":8080",
		DatabaseURL: "postgres://user:pass@database/app", CORSOrigins: []string{"*"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected wildcard production CORS to be rejected")
	}
}

func TestLoadBuildsDatabaseURLFromSecretFile(t *testing.T) {
	passwordFile := filepath.Join(t.TempDir(), "database-password")
	if err := os.WriteFile(passwordFile, []byte("secret-value\n"), 0o600); err != nil {
		t.Fatalf("write password file: %v", err)
	}
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DB_HOST", "database.internal")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "app")
	t.Setenv("DB_USER", "app_admin")
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("DB_PASSWORD_FILE", passwordFile)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	parsed, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		t.Fatalf("parse database URL: %v", err)
	}
	password, ok := parsed.User.Password()
	if !ok || password != "secret-value" || parsed.Hostname() != "database.internal" {
		t.Fatalf("unexpected database URL components")
	}
}
`

const scaffoldSystemModule = `package system

import (
	"context"
	"time"

	gpp "github.com/saifsilver/goplusplus"
)

type databasePinger interface {
	PingContext(context.Context) error
}

type Module struct {
	database databasePinger
}

func New(database databasePinger) *Module { return &Module{database: database} }
func (module *Module) Name() string        { return "system" }

func (module *Module) Register(group *gpp.RouterGroup) {
	group.GET("/live", module.live)
	group.GET("/ready", module.ready)
}

func (module *Module) live(c *gpp.Context) error {
	return c.OK(gpp.H{"status": "up"})
}

func (module *Module) ready(c *gpp.Context) error {
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
	defer cancel()
	if err := module.database.PingContext(ctx); err != nil {
		return gpp.NewInternalError("health.database", err, gpp.WithErrorCategory("database"))
	}
	return c.OK(gpp.H{"status": "ready"})
}
`

const scaffoldUsersDomain = `package users

import (
	"errors"
	"time"
)

var (
	ErrNotFound   = errors.New("user not found")
	ErrEmailTaken = errors.New("email already registered")
)

type User struct {
	ID        string    ` + "`json:\"id\"`" + `
	Name      string    ` + "`json:\"name\"`" + `
	Email     string    ` + "`json:\"email\"`" + `
	CreatedAt time.Time ` + "`json:\"created_at\"`" + `
}

type CreateInput struct {
	Name  string
	Email string
}
`

const scaffoldUsersRepository = `package users

import (
	"context"
	"database/sql"
	"errors"

	"github.com/saifsilver/goplusplus/dbcore"
)

type Repository interface {
	Create(context.Context, User) error
	FindByID(context.Context, string) (User, error)
}

type SQLRepository struct {
	database *dbcore.Client
}

func NewSQLRepository(database *dbcore.Client) *SQLRepository {
	return &SQLRepository{database: database}
}

func (repository *SQLRepository) Create(ctx context.Context, user User) error {
	_, err := repository.database.Exec(dbcore.WithQueryName(ctx, "users.create"),
		` + "`INSERT INTO users (id, name, email, created_at) VALUES ($1, $2, $3, $4)`" + `,
		user.ID, user.Name, user.Email, user.CreatedAt,
	)
	if dbcore.IsErrorKind(err, dbcore.ErrorUniqueConstraint) {
		return ErrEmailTaken
	}
	return err
}

func (repository *SQLRepository) FindByID(ctx context.Context, id string) (User, error) {
	var user User
	err := repository.database.QueryRow(dbcore.WithQueryName(ctx, "users.find_by_id"),
		` + "`SELECT id, name, email, created_at FROM users WHERE id = $1`" + `,
		func(row *sql.Row) error {
			return row.Scan(&user.ID, &user.Name, &user.Email, &user.CreatedAt)
		}, id,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return user, err
}
`

const scaffoldUsersService = `package users

import (
	"context"
	"strings"
	"time"

	"github.com/saifsilver/goplusplus/id"
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (service *Service) Create(ctx context.Context, input CreateInput) (User, error) {
	user := User{
		ID: id.NewUUIDv7(), Name: strings.TrimSpace(input.Name),
		Email: strings.ToLower(strings.TrimSpace(input.Email)), CreatedAt: time.Now().UTC(),
	}
	if err := service.repository.Create(ctx, user); err != nil {
		return User{}, err
	}
	return user, nil
}

func (service *Service) FindByID(ctx context.Context, id string) (User, error) {
	return service.repository.FindByID(ctx, id)
}
`

const scaffoldUsersModule = `package users

import (
	"errors"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/dbcore"
)

type Module struct {
	service *Service
}

type createRequest struct {
	Name  string ` + "`json:\"name\" validate:\"required,min=2,max=100\"`" + `
	Email string ` + "`json:\"email\" validate:\"required,email,max=320\"`" + `
}

func Build(database *dbcore.Client) gpp.Module {
	return New(NewService(NewSQLRepository(database)))
}

func New(service *Service) *Module { return &Module{service: service} }
func (module *Module) Name() string { return "users" }

func (module *Module) Register(group *gpp.RouterGroup) {
	group.POST("/", module.create)
	group.GET("/:id", module.findByID)
}

func (module *Module) create(c *gpp.Context) error {
	var request createRequest
	if err := c.BindAndValidate(&request); err != nil {
		return err
	}
	user, err := module.service.Create(c.Request.Context(), CreateInput(request))
	if err != nil {
		return mapError("users.create", err)
	}
	return c.Created(user)
}

func (module *Module) findByID(c *gpp.Context) error {
	user, err := module.service.FindByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		return mapError("users.find_by_id", err)
	}
	return c.OK(user)
}

func mapError(operation string, err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return gpp.ErrNotFound("User not found")
	case errors.Is(err, ErrEmailTaken):
		return gpp.ErrConflict("Email is already registered")
	default:
		return gpp.NewInternalError(operation, err, gpp.WithErrorCategory("database"))
	}
}
`

const scaffoldMigrations = `package users

import (
	_ "embed"

	"github.com/saifsilver/goplusplus/dbcore"
)

//go:embed migrations/0001_users.sql
var createUsers string

func Migrations() []dbcore.Migration {
	return []dbcore.Migration{{ID: "0001_users", Version: 1, Name: "create users", SQL: createUsers}}
}
`

const scaffoldUsersMigration = `CREATE TABLE IF NOT EXISTS users (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	email TEXT NOT NULL UNIQUE,
	created_at TIMESTAMP NOT NULL
);
`

const scaffoldApplicationTest = `package application

import (
	"context"
	"net/http"
	"testing"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/gpptest"

	"{{MODULE}}/internal/config"
)

func TestUserFlow(t *testing.T) {
	app, err := New(context.Background(), config.Config{
		Environment: "test", HTTPAddress: ":0", DatabaseURL: ":memory:",
	})
	if err != nil {
		t.Fatalf("initialize application: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	tester := gpptest.New(t, app.Engine)
	created := tester.POST("/api/v1/users/", gpp.H{
		"name": "Ada Lovelace", "email": "ada@example.com",
	}).AssertStatus(http.StatusCreated)
	var user struct{ ID string ` + "`json:\"id\"`" + ` }
	created.DecodeInto(&user)
	tester.GET("/api/v1/users/" + user.ID).AssertStatus(http.StatusOK).AssertJSON("email", "ada@example.com")
	tester.GET("/health/ready").AssertStatus(http.StatusOK)
}
`

const scaffoldEnv = `APP_ENV=development
HTTP_ADDRESS=:8080
DATABASE_URL=file:data/{{APP_NAME}}.db
CORS_ALLOWED_ORIGINS=http://localhost:3000
`

const scaffoldGitignore = `/bin/
/data/*.db
/data/*.db-shm
/data/*.db-wal
.env
`

const scaffoldDockerignore = `.git
bin
data
.env
`

const scaffoldDockerfile = `FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM alpine:3.23
RUN addgroup -S app && adduser -S -G app app && mkdir -p /app/data && chown -R app:app /app
WORKDIR /app
COPY --from=build --chown=app:app /out/api ./api
USER app
ENV APP_ENV=production HTTP_ADDRESS=:8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://127.0.0.1:8080/health/live || exit 1
ENTRYPOINT ["./api"]
`

const scaffoldMakefile = `.PHONY: build run test verify docker deploy-standard deploy-aws

build:
	go build -trimpath -o bin/{{APP_NAME}} ./cmd/api

run:
	go run ./cmd/api

test:
	go test ./...

verify:
	go vet ./...
	go test -race ./...

docker:
	docker build -t {{APP_NAME}}:latest .

deploy-standard:
	docker compose -f deploy/standard/compose.yaml up -d --build

deploy-aws:
	cd deploy/terraform/aws && ./deploy.sh
`

const scaffoldReadme = `# {{APP_NAME}}

A production-oriented goplusplus modular monolith.

## Run locally

` + "```bash" + `
go mod tidy
make run
` + "```" + `

The development configuration uses SQLite at ` + "`data/{{APP_NAME}}.db`" + `. Production fails closed unless ` + "`DATABASE_URL`" + ` is a PostgreSQL URL.

## Architecture

- ` + "`cmd/api`" + ` is the composition root and process entrypoint.
- ` + "`internal/application`" + ` wires infrastructure to business modules.
- ` + "`internal/modules/<capability>`" + ` owns a capability's domain, use cases, persistence adapter, HTTP transport, and migrations.

Modules communicate through explicit Go interfaces and expose portable ` + "`Build`" + ` and ` + "`Migrations`" + ` functions. Do not let modules query each other's tables directly.

## Extract a microservice

` + "```bash" + `
gpp extract service users --module github.com/acme/users-service
` + "```" + `

HTTP is the default transport. Extraction audits internal dependencies, creates a standalone service under ` + "`services/users`" + `, and preserves the monolith for a controlled data and traffic migration.

## Deployment generators

Run from the application root:

` + "```bash" + `
gpp gen hosting standard
gpp gen terraform aws
` + "```" + `

The standard target is intended for a single Docker-capable VPS. The AWS target provides horizontally scalable ECS/Fargate hosting with managed PostgreSQL.

## Endpoints

- ` + "`POST /api/v1/users/`" + `
- ` + "`GET /api/v1/users/:id`" + `
- ` + "`GET /health/live`" + `
- ` + "`GET /health/ready`" + `
- ` + "`GET /metrics`" + `
- ` + "`GET /swagger`" + ` (disabled in production)
`
