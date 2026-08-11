package dbcore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Migration struct {
	ID      string
	Version int
	Name    string
	SQL     string
	UpSQL   string
	DownSQL string
}

// MigrationDatabase is the minimal database/sql seam required by migrations.
type MigrationDatabase interface {
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	Dialect() string
}

type preparedMigration struct {
	Migration
	checksum string
}

// AutoMigrate validates and transactionally applies ordered migrations. A
// PostgreSQL advisory transaction lock and SQLite write transaction serialize
// concurrent application starts.
func AutoMigrate(ctx context.Context, database MigrationDatabase, migrations ...Migration) error {
	if database == nil {
		return errors.New("dbcore/migrate: database is nil")
	}
	prepared, err := prepareMigrations(migrations)
	if err != nil {
		return err
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("dbcore/migrate: begin transaction: %w", ClassifyError(err))
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if database.Dialect() == "postgres" {
		if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", int64(0x6770706d696772)); err != nil {
			return fmt.Errorf("dbcore/migrate: acquire migration lock: %w", ClassifyError(err))
		}
	}
	if _, err := tx.ExecContext(ctx, migrationTableSQL); err != nil {
		return fmt.Errorf("dbcore/migrate: initialize history: %w", ClassifyError(err))
	}
	if database.Dialect() == "sqlite" {
		if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS gpp_migration_lock (id INTEGER PRIMARY KEY)`); err != nil {
			return fmt.Errorf("dbcore/migrate: initialize lock: %w", ClassifyError(err))
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO gpp_migration_lock (id) VALUES (1)`); err != nil {
			return fmt.Errorf("dbcore/migrate: initialize lock row: %w", ClassifyError(err))
		}
		if _, err := tx.ExecContext(ctx, `UPDATE gpp_migration_lock SET id = id WHERE id = 1`); err != nil {
			return fmt.Errorf("dbcore/migrate: acquire migration lock: %w", ClassifyError(err))
		}
	}
	applied, err := loadAppliedMigrations(ctx, tx)
	if err != nil {
		return err
	}
	appliedNow := make([]preparedMigration, 0, len(prepared))
	for _, migration := range prepared {
		if checksum, exists := applied[migration.ID]; exists {
			if checksum != migration.checksum {
				return fmt.Errorf("dbcore/migrate: applied migration %q checksum changed", migration.ID)
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
			return fmt.Errorf("dbcore/migrate: apply %q: %w", migration.ID, ClassifyError(err))
		}
		if _, err := tx.ExecContext(ctx, migrationInsertSQL, migration.ID, migration.Version, migration.Name, migration.checksum, time.Now().UTC()); err != nil {
			return fmt.Errorf("dbcore/migrate: record %q: %w", migration.ID, ClassifyError(err))
		}
		appliedNow = append(appliedNow, migration)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("dbcore/migrate: commit: %w", ClassifyError(err))
	}
	committed = true
	for _, migration := range appliedNow {
		slog.Info("dbcore: migration applied", slog.String("id", migration.ID), slog.Int("version", migration.Version))
	}
	return nil
}

const migrationTableSQL = `CREATE TABLE IF NOT EXISTS gpp_migrations (
	id VARCHAR(255) PRIMARY KEY,
	version INTEGER NOT NULL UNIQUE,
	name VARCHAR(255) NOT NULL,
	checksum VARCHAR(64) NOT NULL,
	applied_at TIMESTAMP NOT NULL
)`

const migrationInsertSQL = `INSERT INTO gpp_migrations (id, version, name, checksum, applied_at) VALUES ($1, $2, $3, $4, $5)`

func prepareMigrations(migrations []Migration) ([]preparedMigration, error) {
	prepared := make([]preparedMigration, 0, len(migrations))
	ids := make(map[string]struct{}, len(migrations))
	versions := make(map[int]struct{}, len(migrations))
	for index, migration := range migrations {
		if migration.Version == 0 {
			migration.Version = index + 1
		}
		if migration.Version < 1 {
			return nil, fmt.Errorf("dbcore/migrate: migration %d has invalid version %d", index, migration.Version)
		}
		if strings.TrimSpace(migration.ID) == "" {
			migration.ID = fmt.Sprintf("%04d_%s", migration.Version, sanitizeName(migration.Name))
		}
		migration.ID = strings.TrimSpace(migration.ID)
		if _, exists := ids[migration.ID]; exists {
			return nil, fmt.Errorf("dbcore/migrate: duplicate migration ID %q", migration.ID)
		}
		if _, exists := versions[migration.Version]; exists {
			return nil, fmt.Errorf("dbcore/migrate: duplicate migration version %d", migration.Version)
		}
		ids[migration.ID] = struct{}{}
		versions[migration.Version] = struct{}{}
		sqlText := migration.SQL
		if strings.TrimSpace(sqlText) == "" {
			sqlText = migration.UpSQL
		}
		if strings.TrimSpace(sqlText) == "" {
			return nil, fmt.Errorf("dbcore/migrate: migration %q is empty", migration.ID)
		}
		migration.SQL = sqlText
		digest := sha256.Sum256([]byte(sqlText))
		prepared = append(prepared, preparedMigration{Migration: migration, checksum: hex.EncodeToString(digest[:])})
	}
	sort.Slice(prepared, func(i, j int) bool {
		if prepared[i].Version == prepared[j].Version {
			return prepared[i].ID < prepared[j].ID
		}
		return prepared[i].Version < prepared[j].Version
	})
	return prepared, nil
}

func loadAppliedMigrations(ctx context.Context, tx *sql.Tx) (map[string]string, error) {
	rows, err := tx.QueryContext(ctx, "SELECT id, checksum FROM gpp_migrations")
	if err != nil {
		return nil, fmt.Errorf("dbcore/migrate: read history: %w", ClassifyError(err))
	}
	defer rows.Close()
	applied := make(map[string]string)
	for rows.Next() {
		var id, checksum string
		if err := rows.Scan(&id, &checksum); err != nil {
			return nil, fmt.Errorf("dbcore/migrate: scan history: %w", ClassifyError(err))
		}
		applied[id] = checksum
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dbcore/migrate: iterate history: %w", ClassifyError(err))
	}
	return applied, nil
}

func Migrate(ctx context.Context, database MigrationDatabase, migrations []Migration) error {
	return AutoMigrate(ctx, database, migrations...)
}

func MigrateEmbed(ctx context.Context, database MigrationDatabase, embedFS fs.FS, dir string) error {
	entries, err := fs.ReadDir(embedFS, dir)
	if err != nil {
		return fmt.Errorf("dbcore/migrate: read embedded directory %q: %w", dir, err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".sql") {
			files = append(files, entry.Name())
		}
	}
	if len(files) == 0 {
		return fmt.Errorf("dbcore/migrate: directory %q contains no SQL migrations", dir)
	}
	sort.Strings(files)
	migrations := make([]Migration, 0, len(files))
	for index, filename := range files {
		filePath := path.Join(dir, filename)
		content, err := fs.ReadFile(embedFS, filePath)
		if err != nil {
			return fmt.Errorf("dbcore/migrate: read embedded file %q: %w", filePath, err)
		}
		version, name, err := parseMigrationFilename(filename, index)
		if err != nil {
			return err
		}
		migrations = append(migrations, Migration{ID: filename, Version: version, Name: name, SQL: string(content)})
	}
	return AutoMigrate(ctx, database, migrations...)
}

func parseMigrationFilename(filename string, index int) (int, string, error) {
	name := strings.TrimSuffix(filename, path.Ext(filename))
	version := index + 1
	parts := strings.SplitN(name, "_", 2)
	if len(parts) == 2 {
		parsed, err := strconv.Atoi(parts[0])
		if err != nil || parsed < 1 || strings.TrimSpace(parts[1]) == "" {
			return 0, "", fmt.Errorf("dbcore/migrate: invalid migration filename %q", filename)
		}
		version, name = parsed, parts[1]
	}
	return version, name, nil
}

func sanitizeName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "migration"
	}
	return strings.Join(strings.Fields(value), "_")
}
