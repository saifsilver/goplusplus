package dbcore_test

import (
	"context"
	"testing"
	"time"

	"github.com/saifsilver/goplusplus/dbcore"
)

func TestMaterializePaginationSession(t *testing.T) {
	ctx := context.Background()
	client, err := dbcore.NewClient(ctx, dbcore.Config{RWDSN: ":memory:"})
	if err != nil {
		t.Fatalf("failed creating memory db client: %v", err)
	}
	defer client.Close()
	if _, err := client.Exec(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Exec(ctx, "INSERT INTO users (id, name) VALUES ($1, $2)", 1, "Ada"); err != nil {
		t.Fatal(err)
	}

	session, err := dbcore.MaterializePagination(ctx, client, "SELECT * FROM users", 500*time.Millisecond)
	if err != nil {
		t.Fatalf("MaterializePagination failed: %v", err)
	}

	if session.SessionID == "" {
		t.Error("expected non-empty session ID")
	}
	if session.TotalRows != 1 {
		t.Fatalf("TotalRows = %d, want 1", session.TotalRows)
	}

	rows, _, err := dbcore.PaginateSession(ctx, client, session.SessionID, 1, 10)
	if err != nil {
		t.Fatalf("PaginateSession failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %v, want one materialized row", rows)
	}

	// Wait for expiration and verify table clean up
	time.Sleep(600 * time.Millisecond)

	_, _, err = dbcore.PaginateSession(ctx, client, session.SessionID, 1, 10)
	if err == nil {
		t.Errorf("expected session to be expired after TTL")
	}
}

func TestMaterializePaginationPropagatesQueryFailure(t *testing.T) {
	ctx := context.Background()
	client, err := dbcore.NewClient(ctx, dbcore.Config{RWDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if _, err := dbcore.MaterializePagination(ctx, client, "SELECT * FROM missing_users", time.Minute); err == nil {
		t.Fatal("expected invalid source query to fail")
	}
}

func TestWithCacheContext(t *testing.T) {
	ctx := context.Background()
	cacheCtx := dbcore.WithCache(ctx, 30*time.Second)

	ttl, ok := dbcore.GetCacheTTL(cacheCtx)
	if !ok || ttl != 30*time.Second {
		t.Errorf("GetCacheTTL = %v, want 30s", ttl)
	}
}
