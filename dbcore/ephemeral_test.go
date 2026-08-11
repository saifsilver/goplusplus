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

	session, err := dbcore.MaterializePagination(ctx, client, "SELECT * FROM users", 500*time.Millisecond)
	if err != nil {
		t.Fatalf("MaterializePagination failed: %v", err)
	}

	if session.SessionID == "" {
		t.Error("expected non-empty session ID")
	}

	rows, _, err := dbcore.PaginateSession(ctx, client, session.SessionID, 1, 10)
	if err != nil {
		t.Fatalf("PaginateSession failed: %v", err)
	}
	_ = rows

	// Wait for expiration and verify table clean up
	time.Sleep(600 * time.Millisecond)

	_, _, err = dbcore.PaginateSession(ctx, client, session.SessionID, 1, 10)
	if err == nil {
		t.Errorf("expected session to be expired after TTL")
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
