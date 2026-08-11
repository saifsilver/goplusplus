package main

const extractedApplication = `package application

import (
	"context"
	"fmt"
	"time"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/dbcore"
	"github.com/saifsilver/goplusplus/middleware"

	"{{MODULE}}/internal/config"
	"{{MODULE}}/internal/modules/system"
	capability "{{MODULE}}/internal/modules/{{CAPABILITY}}"
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
	if err := dbcore.AutoMigrate(ctx, database, capability.Migrations()...); err != nil {
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
	engine.RegisterModule("{{ROUTE}}", capability.Build(database))
	engine.GET("/metrics", middleware.Prometheus())
	if cfg.EnableDocs {
		engine.GET("/swagger", engine.AutoSwaggerUI())
	}
	return &Application{Engine: engine, database: database}, nil
}

func (app *Application) Run(address string) error { return app.Engine.Listen(address) }
func (app *Application) Close() error             { return app.database.Close() }
`

const extractedApplicationTest = `package application

import (
	"context"
	"net/http"
	"testing"

	"github.com/saifsilver/goplusplus/gpptest"

	"{{MODULE}}/internal/config"
)

func TestExtractedServiceStarts(t *testing.T) {
	app, err := New(context.Background(), config.Config{
		Environment: "test", HTTPAddress: ":0", DatabaseURL: ":memory:",
	})
	if err != nil {
		t.Fatalf("initialize application: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	tester := gpptest.New(t, app.Engine)
	tester.GET("/health/live").AssertStatus(http.StatusOK)
	tester.GET("/health/ready").AssertStatus(http.StatusOK)
}
`

const extractedMakefile = `.PHONY: build run test verify docker deploy-standard deploy-aws

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

const extractedReadme = `# {{CAPABILITY}} service

Standalone HTTP service extracted from the ` + "`{{CAPABILITY}}`" + ` module.

` + "```bash" + `
go mod tidy
make test
make run
` + "```" + `

The module is mounted at ` + "`{{ROUTE}}`" + `. It owns its business logic, persistence adapter, and migrations. Configuration fails closed in production unless PostgreSQL is configured.

## Hosting

` + "```bash" + `
gpp gen hosting standard
# or
gpp gen terraform aws
` + "```" + `

Read ` + "`MIGRATION.md`" + ` before routing production traffic. The extraction command creates a safe copy; it does not delete the monolith module or move production data automatically.
`

const extractedMigrationGuide = `# Extraction migration checklist

The service code is extracted, but production ownership must be transferred deliberately.

1. Make this service the only writer for the capability's tables.
2. Provision its database and apply the copied module migrations.
3. Backfill data with a restartable, checksummed process.
4. Replace monolith callers with an explicit HTTP client or event adapter.
5. Run contract, authorization, timeout, retry, and idempotency tests.
6. Shadow traffic or compare reads before changing the source of truth.
7. Cut traffic over gradually and retain a tested rollback path.
8. Remove the monolith implementation only after the rollback window closes.

Do not let both deployments write the same records without an explicit consistency strategy.
`
