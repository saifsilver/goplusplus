package seed_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/saifsilver/goplusplus/dbcore"
	"github.com/saifsilver/goplusplus/dbcore/seed"
)

func TestFakerGenerators(t *testing.T) {
	f := seed.NewFaker()

	if f.Name() == "" {
		t.Error("expected non-empty name")
	}
	if f.Email() == "" {
		t.Error("expected non-empty email")
	}
	if f.UUID() == "" {
		t.Error("expected non-empty UUID")
	}
	if f.Phone() == "" {
		t.Error("expected non-empty phone")
	}
	if f.Select("A", "B") == "" {
		t.Error("expected non-empty selection")
	}
}

func TestSeederRun(t *testing.T) {
	ctx := context.Background()
	client, err := dbcore.NewClient(ctx, dbcore.Config{RWDSN: ":memory:"})
	if err != nil {
		t.Fatalf("failed creating memory db client: %v", err)
	}

	_ = client.Exec(ctx, `CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, email TEXT);`)

	err = seed.Run(ctx, client, seed.Plan{
		Table: "users",
		Count: 5,
		Factory: func(f *seed.Faker) map[string]any {
			return map[string]any{
				"name":  f.Name(),
				"email": f.Email(),
			}
		},
	})

	if err != nil {
		t.Fatalf("seed.Run failed: %v", err)
	}

	var count int
	_ = client.Query(ctx, "SELECT COUNT(*) FROM users", func(rows *sql.Rows) error {
		if rows.Next() {
			return rows.Scan(&count)
		}
		return nil
	})

	if count != 5 {
		t.Fatalf("expected 5 seeded rows, got %d", count)
	}
}
