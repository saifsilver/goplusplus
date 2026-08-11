package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var migrationVersionPattern = regexp.MustCompile(`Version:\s*([0-9]+)`)

func generateModule(moduleName string) error {
	packageName := strings.ToLower(moduleName)
	target := filepath.Join("internal", "modules", packageName)
	migrationVersion, err := nextMigrationVersion(filepath.Join("internal", "modules"))
	if err != nil {
		return err
	}
	replacer := strings.NewReplacer(
		"{{PACKAGE}}", packageName,
		"{{TABLE}}", "module_"+packageName+"_records",
		"{{MIGRATION_ID}}", time.Now().UTC().Format("20060102150405")+"_"+packageName,
		"{{MIGRATION_VERSION}}", strconv.Itoa(migrationVersion),
	)
	file := func(path, content string) scaffoldFile {
		return scaffoldFile{path: path, content: replacer.Replace(content), mode: 0o644}
	}
	return publishGeneratedTree(target, "Portable domain module", []scaffoldFile{
		file("domain.go", generatedModuleDomain),
		file("repository.go", generatedModuleRepository),
		file("service.go", generatedModuleService),
		file("module.go", generatedModuleHTTP),
		file("migrations.go", generatedModuleMigrations),
		file("migrations/0001_create_records.sql", generatedModuleMigrationSQL),
		file("module_test.go", generatedModuleTest),
	})
}

func nextMigrationVersion(root string) (int, error) {
	maximum := 0
	err := filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(filePath) != ".go" {
			return nil
		}
		content, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		for _, match := range migrationVersionPattern.FindAllSubmatch(content, -1) {
			version, err := strconv.Atoi(string(match[1]))
			if err != nil {
				return err
			}
			if version > maximum {
				maximum = version
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	return maximum + 1, nil
}

const generatedModuleDomain = `package {{PACKAGE}}

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("{{PACKAGE}} record not found")

type Record struct {
	ID        string    ` + "`json:\"id\"`" + `
	CreatedAt time.Time ` + "`json:\"created_at\"`" + `
}
`

const generatedModuleRepository = `package {{PACKAGE}}

import (
	"context"
	"database/sql"
	"errors"

	"github.com/saifsilver/goplusplus/dbcore"
)

type Repository interface {
	Create(context.Context, Record) error
	FindByID(context.Context, string) (Record, error)
}

type SQLRepository struct{ database *dbcore.Client }

func NewSQLRepository(database *dbcore.Client) *SQLRepository {
	return &SQLRepository{database: database}
}

func (repository *SQLRepository) Create(ctx context.Context, record Record) error {
	_, err := repository.database.Exec(dbcore.WithQueryName(ctx, "{{PACKAGE}}.create"),
		` + "`INSERT INTO {{TABLE}} (id, created_at) VALUES ($1, $2)`" + `,
		record.ID, record.CreatedAt,
	)
	return err
}

func (repository *SQLRepository) FindByID(ctx context.Context, id string) (Record, error) {
	var record Record
	err := repository.database.QueryRow(dbcore.WithQueryName(ctx, "{{PACKAGE}}.find_by_id"),
		` + "`SELECT id, created_at FROM {{TABLE}} WHERE id = $1`" + `,
		func(row *sql.Row) error { return row.Scan(&record.ID, &record.CreatedAt) }, id,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	return record, err
}
`

const generatedModuleService = `package {{PACKAGE}}

import (
	"context"
	"time"

	"github.com/saifsilver/goplusplus/id"
)

type Service struct{ repository Repository }

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (service *Service) Create(ctx context.Context) (Record, error) {
	record := Record{ID: id.NewUUIDv7(), CreatedAt: time.Now().UTC()}
	if err := service.repository.Create(ctx, record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (service *Service) FindByID(ctx context.Context, id string) (Record, error) {
	return service.repository.FindByID(ctx, id)
}
`

const generatedModuleHTTP = `package {{PACKAGE}}

import (
	"errors"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/dbcore"
)

type Module struct{ service *Service }

func Build(database *dbcore.Client) gpp.Module {
	return New(NewService(NewSQLRepository(database)))
}

func New(service *Service) *Module { return &Module{service: service} }
func (module *Module) Name() string { return "{{PACKAGE}}" }

func (module *Module) Register(group *gpp.RouterGroup) {
	group.POST("/", module.create)
	group.GET("/:id", module.findByID)
}

func (module *Module) create(c *gpp.Context) error {
	record, err := module.service.Create(c.Request.Context())
	if err != nil {
		return gpp.NewInternalError("{{PACKAGE}}.create", err, gpp.WithErrorCategory("database"))
	}
	return c.Created(record)
}

func (module *Module) findByID(c *gpp.Context) error {
	record, err := module.service.FindByID(c.Request.Context(), c.Param("id"))
	if errors.Is(err, ErrNotFound) {
		return gpp.ErrNotFound("{{PACKAGE}} record not found")
	}
	if err != nil {
		return gpp.NewInternalError("{{PACKAGE}}.find_by_id", err, gpp.WithErrorCategory("database"))
	}
	return c.OK(record)
}
`

const generatedModuleMigrations = `package {{PACKAGE}}

import (
	_ "embed"

	"github.com/saifsilver/goplusplus/dbcore"
)

//go:embed migrations/0001_create_records.sql
var initialMigration string

func Migrations() []dbcore.Migration {
	return []dbcore.Migration{{
		ID: "{{MIGRATION_ID}}", Version: {{MIGRATION_VERSION}},
		Name: "create {{PACKAGE}} records", SQL: initialMigration,
	}}
}
`

const generatedModuleMigrationSQL = `CREATE TABLE IF NOT EXISTS {{TABLE}} (
	id TEXT PRIMARY KEY,
	created_at TIMESTAMP NOT NULL
);
`

const generatedModuleTest = `package {{PACKAGE}}

import (
	"context"
	"net/http"
	"testing"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/dbcore"
	"github.com/saifsilver/goplusplus/gpptest"
)

func TestModuleFlow(t *testing.T) {
	database, err := dbcore.NewClient(context.Background(), dbcore.Config{RWDSN: ":memory:"})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := dbcore.AutoMigrate(context.Background(), database, Migrations()...); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	app := gpp.New()
	app.RegisterModule("/{{PACKAGE}}", Build(database))
	tester := gpptest.New(t, app)
	created := tester.POST("/{{PACKAGE}}/", gpp.H{}).AssertStatus(http.StatusCreated)
	var record Record
	created.DecodeInto(&record)
	tester.GET("/{{PACKAGE}}/" + record.ID).AssertStatus(http.StatusOK)
}
`
