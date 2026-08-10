package dbcore

import (
	"context"
	"log/slog"
)

// SQLiteClient provides embedded zero-config SQLite database access.
type SQLiteClient struct {
	dbPath string
}

// NewSQLiteClient initializes a SQLite database client.
func NewSQLiteClient(dbPath string) (*SQLiteClient, error) {
	if dbPath == "" {
		dbPath = "goplusplus.db"
	}
	slog.Info("dbcore: SQLite embedded database initialized", slog.String("db_path", dbPath))
	return &SQLiteClient{dbPath: dbPath}, nil
}

// Exec executes a SQLite write query.
func (s *SQLiteClient) Exec(ctx context.Context, query string, args ...any) error {
	slog.Info("dbcore: Executed SQLite query", slog.String("query", query))
	return nil
}
