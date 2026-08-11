package seed

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/saifsilver/goplusplus/dbcore"
)

// Plan defines a declarative database seeding task.
type Plan struct {
	Table         string
	Count         int
	Factory       func(f *Faker) map[string]any
	TruncateFirst bool
}

// Run executes batch seeding plans against a dbcore.Client instance.
func Run(ctx context.Context, client *dbcore.Client, plans ...Plan) error {
	faker := NewFaker()

	for _, plan := range plans {
		if plan.Count <= 0 {
			plan.Count = 10
		}

		if plan.TruncateFirst {
			truncateSQL := fmt.Sprintf("DELETE FROM %s", plan.Table)
			if _, err := client.Exec(ctx, truncateSQL); err != nil {
				slog.Warn("dbcore/seed: Failed to truncate table", slog.String("table", plan.Table), slog.String("error", err.Error()))
			}
		}

		for i := 0; i < plan.Count; i++ {
			record := plan.Factory(faker)
			if len(record) == 0 {
				continue
			}

			columns := make([]string, 0, len(record))
			placeholders := make([]string, 0, len(record))
			args := make([]any, 0, len(record))

			idx := 1
			for col, val := range record {
				columns = append(columns, col)
				placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
				args = append(args, val)
				idx++
			}

			insertSQL := fmt.Sprintf(
				"INSERT INTO %s (%s) VALUES (%s)",
				plan.Table,
				strings.Join(columns, ", "),
				strings.Join(placeholders, ", "),
			)

			if _, err := client.Exec(ctx, insertSQL, args...); err != nil {
				return fmt.Errorf("dbcore/seed: Error inserting row %d into table '%s': %w", i+1, plan.Table, err)
			}
		}
		slog.Info("dbcore/seed: Successfully seeded table", slog.String("table", plan.Table), slog.Int("count", plan.Count))
	}
	return nil
}
