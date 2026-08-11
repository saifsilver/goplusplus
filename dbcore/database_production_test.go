package dbcore_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/saifsilver/goplusplus/dbcore"
)

func TestSQLiteExecutesSQLAndEnforcesConstraints(t *testing.T) {
	t.Parallel()
	database, err := dbcore.OpenSQLite(context.Background(), dbcore.SQLiteConfig{InMemory: true, MaxOpenConnections: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	statements := []string{
		`CREATE TABLE parents (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE children (id INTEGER PRIMARY KEY, parent_id INTEGER NOT NULL REFERENCES parents(id), code TEXT NOT NULL UNIQUE, score INTEGER CHECK(score > 0))`,
	}
	for _, statement := range statements {
		if _, err := database.ExecContext(context.Background(), statement); err != nil {
			t.Fatal(err)
		}
	}

	_, err = database.ExecContext(context.Background(), `INSERT INTO children (id, parent_id, code, score) VALUES (1, 99, 'a', 1)`)
	if !dbcore.IsErrorKind(err, dbcore.ErrorForeignKeyConstraint) {
		t.Fatalf("expected foreign-key classification, got %v", err)
	}
	if _, err := database.ExecContext(context.Background(), `INSERT INTO parents (id) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(context.Background(), `INSERT INTO children (id, parent_id, code, score) VALUES (1, 1, 'a', 1)`); err != nil {
		t.Fatal(err)
	}
	_, err = database.ExecContext(context.Background(), `INSERT INTO children (id, parent_id, code, score) VALUES (2, 1, 'a', 1)`)
	if !dbcore.IsErrorKind(err, dbcore.ErrorUniqueConstraint) {
		t.Fatalf("expected unique classification, got %v", err)
	}
	_, err = database.ExecContext(context.Background(), `INSERT INTO children (id, parent_id, code, score) VALUES (3, 1, NULL, 1)`)
	if !dbcore.IsErrorKind(err, dbcore.ErrorNotNullConstraint) {
		t.Fatalf("expected not-null classification, got %v", err)
	}
	_, err = database.ExecContext(context.Background(), `INSERT INTO children (id, parent_id, code, score) VALUES (4, 1, 'd', 0)`)
	if !dbcore.IsErrorKind(err, dbcore.ErrorCheckConstraint) {
		t.Fatalf("expected check classification, got %v", err)
	}
}

func TestSQLitePragmasApplyAcrossPoolConnections(t *testing.T) {
	t.Parallel()
	database, err := dbcore.OpenSQLite(context.Background(), dbcore.SQLiteConfig{InMemory: true, MaxOpenConnections: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	var wg sync.WaitGroup
	errCh := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var enabled int
			if err := database.QueryRowContext(context.Background(), `PRAGMA foreign_keys`).Scan(&enabled); err != nil {
				errCh <- err
				return
			}
			if enabled != 1 {
				errCh <- errors.New("foreign_keys pragma is disabled")
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

func TestSQLiteCreatesSecureDirectory(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "database")
	database, err := dbcore.OpenSQLite(context.Background(), dbcore.SQLiteConfig{Path: filepath.Join(dir, "app.db")})
	if err != nil {
		t.Fatal(err)
	}
	_ = database.Close()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o750 {
		t.Fatalf("directory mode = %o, want 750", got)
	}
}

func TestMigrationsDetectDriftAndRollback(t *testing.T) {
	t.Parallel()
	database, err := dbcore.OpenSQLite(context.Background(), dbcore.SQLiteConfig{InMemory: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()
	base := dbcore.Migration{ID: "create_items", Version: 1, Name: "create items", SQL: `CREATE TABLE items (id INTEGER PRIMARY KEY)`}
	if err := dbcore.AutoMigrate(ctx, database, base); err != nil {
		t.Fatal(err)
	}
	changed := base
	changed.SQL = `CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)`
	if err := dbcore.AutoMigrate(ctx, database, changed); err == nil {
		t.Fatal("expected checksum drift failure")
	}

	failing := []dbcore.Migration{
		{ID: "create_temp", Version: 2, SQL: `CREATE TABLE temp_items (id INTEGER)`},
		{ID: "fail", Version: 3, SQL: `THIS IS NOT SQL`},
	}
	if err := dbcore.AutoMigrate(ctx, database, failing...); err == nil {
		t.Fatal("expected migration failure")
	}
	var count int
	err = database.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='temp_items'`).Scan(&count)
	if err != nil || count != 0 {
		t.Fatalf("failed transaction left schema behind: count=%d err=%v", count, err)
	}
}

func TestMigrationValidation(t *testing.T) {
	t.Parallel()
	database, err := dbcore.OpenSQLite(context.Background(), dbcore.SQLiteConfig{InMemory: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	tests := [][]dbcore.Migration{
		{{ID: "same", Version: 1, SQL: "SELECT 1"}, {ID: "same", Version: 2, SQL: "SELECT 1"}},
		{{ID: "a", Version: 1, SQL: "SELECT 1"}, {ID: "b", Version: 1, SQL: "SELECT 1"}},
		{{ID: "empty", Version: 1}},
	}
	for _, migrations := range tests {
		if err := dbcore.AutoMigrate(context.Background(), database, migrations...); err == nil {
			t.Fatalf("expected validation error for %#v", migrations)
		}
	}
}

func TestPostgresConfigAndErrorClassification(t *testing.T) {
	t.Parallel()
	invalid := []dbcore.Config{
		{},
		{RWDSN: ":memory:", MaxOpenConnections: -1},
		{RWDSN: "mysql://localhost/app"},
		{RWDSN: "postgres://localhost"},
		{RWDSN: "postgres://localhost/app", MaxOpenConnections: 1, MaxIdleConnections: 2},
	}
	for _, cfg := range invalid {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		_, err := dbcore.NewPostgresClient(ctx, cfg)
		cancel()
		if err == nil {
			t.Fatalf("expected config error for %#v", cfg)
		}
	}

	cause := &pgconn.PgError{Code: "23505", Message: "secret table detail"}
	err := dbcore.ClassifyError(cause)
	if !dbcore.IsErrorKind(err, dbcore.ErrorUniqueConstraint) || !errors.Is(err, cause) {
		t.Fatalf("classification failed: %v", err)
	}
	if got := err.Error(); got == cause.Error() {
		t.Fatal("classified error leaked the driver message")
	}

	canceled := dbcore.ClassifyError(context.Canceled)
	if !dbcore.IsErrorKind(canceled, dbcore.ErrorCanceled) || !errors.Is(canceled, context.Canceled) {
		t.Fatalf("cancellation classification failed: %v", canceled)
	}
	if got := dbcore.ClassifyError(sql.ErrNoRows); !errors.Is(got, sql.ErrNoRows) {
		t.Fatalf("sql.ErrNoRows changed: %v", got)
	}
}
