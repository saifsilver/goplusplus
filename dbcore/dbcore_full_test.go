package dbcore_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/saifsilver/goplusplus/dbcore"
)

type UserItem struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func TestDBClientFullSuite(t *testing.T) {
	ctx := context.Background()

	client, err := dbcore.NewClient(ctx, dbcore.Config{RWDSN: ":memory:"})
	if err != nil {
		t.Fatalf("failed creating dbcore client: %v", err)
	}
	defer client.Close()

	// 1. Stats
	stats := client.Stats()
	if stats["primary_healthy"] != true {
		t.Errorf("expected primary_healthy true")
	}

	// 2. Exec & ExecIdempotent
	_, err = client.Exec(ctx, "CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT);")
	if err != nil {
		t.Fatalf("Exec CREATE TABLE failed: %v", err)
	}

	_, err = client.ExecIdempotent(ctx, "INSERT INTO users (id, name) VALUES (1, 'Alex');")
	if err != nil {
		t.Fatalf("ExecIdempotent INSERT failed: %v", err)
	}

	// 3. QueryRow & Query
	var name string
	err = client.QueryRow(ctx, "SELECT name FROM users WHERE id = $1", func(row *sql.Row) error {
		return row.Scan(&name)
	}, 1)
	if err != nil {
		t.Fatalf("QueryRow failed: %v", err)
	}

	err = client.Query(ctx, "SELECT COUNT(*) FROM users", func(rows *sql.Rows) error {
		return nil
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	// 4. InTx
	err = client.InTx(ctx, func(tx *sql.Tx) error {
		return nil
	})
	if err != nil {
		t.Fatalf("InTx failed: %v", err)
	}

	// 5. ParallelQuery
	err = client.ParallelQuery(ctx, dbcore.ParallelTask{
		QueryName: "user.count",
		SQL:       "SELECT COUNT(*) FROM users",
	})
	if err != nil {
		t.Fatalf("ParallelQuery failed: %v", err)
	}

	// 6. WithCache
	cachedCtx := dbcore.WithCache(ctx, 30*time.Second)
	ttl, ok := dbcore.GetCacheTTL(cachedCtx)
	if !ok || ttl != 30*time.Second {
		t.Errorf("GetCacheTTL failed")
	}

	// 7. SQLiteClient
	sqClient, err := dbcore.NewSQLiteClient(t.TempDir() + "/test.db")
	if err != nil || sqClient == nil {
		t.Fatalf("NewSQLiteClient failed: %v", err)
	}
	_ = sqClient.Exec(ctx, "CREATE TABLE test (id INT);")

	// 8. Repository[UserItem]
	repo := dbcore.NewRepository[UserItem](client, "users")
	if repo.TableName() != "users" {
		t.Errorf("expected TableName 'users'")
	}
	_ = repo.Create(ctx, &UserItem{ID: 2, Name: "Bob"})
	_ = repo.Update(ctx, 2, &UserItem{ID: 2, Name: "Bobby"})
	_ = repo.Delete(ctx, 2)
}
