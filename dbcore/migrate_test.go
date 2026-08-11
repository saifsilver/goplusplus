package dbcore_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/saifsilver/goplusplus/dbcore"
)

func TestAutoMigrateMemory(t *testing.T) {
	ctx := context.Background()
	client, err := dbcore.NewClient(ctx, dbcore.Config{
		RWDSN: ":memory:",
	})
	if err != nil {
		t.Fatalf("failed creating memory db client: %v", err)
	}

	migrations := []dbcore.Migration{
		{
			Version: 1,
			Name:    "create_users",
			SQL:     `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);`,
		},
		{
			Version: 2,
			Name:    "create_orders",
			SQL:     `CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, amount REAL);`,
		},
	}

	if err := dbcore.AutoMigrate(ctx, client, migrations...); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	// Running AutoMigrate again should be idempotent and skip executed migrations
	if err := dbcore.AutoMigrate(ctx, client, migrations...); err != nil {
		t.Fatalf("Idempotent AutoMigrate failed: %v", err)
	}
}

func TestMigrateEmbed(t *testing.T) {
	ctx := context.Background()
	client, err := dbcore.NewClient(ctx, dbcore.Config{
		RWDSN: ":memory:",
	})
	if err != nil {
		t.Fatalf("failed creating memory db client: %v", err)
	}

	mockFS := fstest.MapFS{
		"migrations/0001_init.sql": &fstest.MapFile{
			Data: []byte(`CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT);`),
		},
		"migrations/0002_add_price.sql": &fstest.MapFile{
			Data: []byte(`ALTER TABLE items ADD COLUMN price REAL;`),
		},
	}

	if err := dbcore.MigrateEmbed(ctx, client, mockFS, "migrations"); err != nil {
		t.Fatalf("MigrateEmbed failed: %v", err)
	}
}
