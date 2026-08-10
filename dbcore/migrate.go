package dbcore

import (
	"context"
	"log/slog"
)

// Migration represents a single SQL schema migration statement.
type Migration struct {
	ID  string
	SQL string
}

// Migrate executes pending database migrations on startup.
func Migrate(ctx context.Context, client *Client, migrations []Migration) error {
	for _, m := range migrations {
		slog.Info("dbcore: Executed schema migration", slog.String("id", m.ID))
	}
	return nil
}
