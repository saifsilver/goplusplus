package id

import (
	"strings"
	"sync"
	"testing"
)

func TestNewULID(t *testing.T) {
	id1 := NewULID()
	id2 := NewULID()

	if len(id1) != 26 {
		t.Errorf("ULID length expected 26, got %d: %s", len(id1), id1)
	}
	if id1 == id2 {
		t.Errorf("expected unique ULIDs, got duplicates: %s", id1)
	}
	// Monotonic ordering: id2 should be >= id1
	if id2 < id1 {
		t.Errorf("ULID not monotonically sorted: %s < %s", id2, id1)
	}
}

func TestNewULIDConcurrency(t *testing.T) {
	count := 5000
	ids := make([]string, count)
	var wg sync.WaitGroup
	wg.Add(count)

	for i := 0; i < count; i++ {
		go func(idx int) {
			defer wg.Done()
			ids[idx] = NewULID()
		}(i)
	}
	wg.Wait()

	seen := make(map[string]bool, count)
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("duplicate ULID detected in concurrent generation: %s", id)
		}
		seen[id] = true
	}
}

func TestNewSnowflake(t *testing.T) {
	id1 := NewSnowflake()
	id2 := NewSnowflake()

	if id1 == 0 || id2 == 0 {
		t.Fatalf("snowflake returned zero: %d, %d", id1, id2)
	}
	if id1 == id2 {
		t.Errorf("expected unique snowflake IDs, got duplicates: %d", id1)
	}
	if id1 > id2 {
		t.Errorf("snowflake not monotonically increasing: %d > %d", id1, id2)
	}
}

func TestNewSnowflakeNode(t *testing.T) {
	node, err := NewSnowflakeNode(0)
	if err != nil || node == nil {
		t.Fatalf("NewSnowflakeNode(0) failed: %v", err)
	}

	id1 := node.NextID()
	id2 := node.NextID()
	if id1 == id2 {
		t.Errorf("snowflake node duplicates: %d", id1)
	}

	_, err = NewSnowflakeNode(9999)
	if err == nil {
		t.Errorf("expected error for nodeID > 1023")
	}

	_, err = NewSnowflakeNode(-1)
	if err == nil {
		t.Errorf("expected error for negative nodeID")
	}
}

func TestNewSnowflakeConcurrency(t *testing.T) {
	node, _ := NewSnowflakeNode(1)
	count := 5000
	ids := make([]int64, count)
	var wg sync.WaitGroup
	wg.Add(count)

	for i := 0; i < count; i++ {
		go func(idx int) {
			defer wg.Done()
			ids[idx] = node.NextID()
		}(i)
	}
	wg.Wait()

	seen := make(map[int64]bool, count)
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("duplicate snowflake ID in concurrent generation: %d", id)
		}
		seen[id] = true
	}
}

func TestNewPrefixed(t *testing.T) {
	userID := NewPrefixed("usr")
	todoID := NewPrefixed("todo")
	orderID := NewPrefixed("ord")

	if !strings.HasPrefix(userID, "usr_") {
		t.Errorf("expected 'usr_' prefix, got %s", userID)
	}
	if !strings.HasPrefix(todoID, "todo_") {
		t.Errorf("expected 'todo_' prefix, got %s", todoID)
	}
	if !strings.HasPrefix(orderID, "ord_") {
		t.Errorf("expected 'ord_' prefix, got %s", orderID)
	}

	if len(userID) != 30 { // "usr_" (4) + ULID (26)
		t.Errorf("prefixed ID length mismatch: got %d for %s", len(userID), userID)
	}
}

func TestNewUUID(t *testing.T) {
	id := NewUUID()
	if len(id) != 36 {
		t.Errorf("UUID length expected 36, got %d: %s", len(id), id)
	}
	// Verify version 4 nibble
	if id[14] != '4' {
		t.Errorf("UUID v4 version nibble mismatch: got %c in %s", id[14], id)
	}
	// Verify uniqueness
	id2 := NewUUID()
	if id == id2 {
		t.Errorf("expected unique UUIDs, got duplicates: %s", id)
	}
}

func TestNewUUIDv7(t *testing.T) {
	id := NewUUIDv7()
	if len(id) != 36 {
		t.Errorf("UUIDv7 length expected 36, got %d: %s", len(id), id)
	}
	// Verify version 7 nibble
	if id[14] != '7' {
		t.Errorf("UUID v7 version nibble mismatch: got %c in %s", id[14], id)
	}
	// Verify k-sortable ordering
	id2 := NewUUIDv7()
	if id2 < id {
		t.Errorf("UUIDv7 not k-sortable: %s < %s", id2, id)
	}
}

func TestGenerateAutoID(t *testing.T) {
	// ULID
	ulid := GenerateAutoID("ulid")
	if s, ok := ulid.(string); !ok || len(s) != 26 {
		t.Errorf("GenerateAutoID(ulid) failed: %v", ulid)
	}

	// Snowflake
	sf := GenerateAutoID("snowflake")
	if n, ok := sf.(int64); !ok || n == 0 {
		t.Errorf("GenerateAutoID(snowflake) failed: %v", sf)
	}

	// UUID
	uuid := GenerateAutoID("uuid")
	if s, ok := uuid.(string); !ok || len(s) != 36 {
		t.Errorf("GenerateAutoID(uuid) failed: %v", uuid)
	}

	// UUIDv7
	uuidv7 := GenerateAutoID("uuidv7")
	if s, ok := uuidv7.(string); !ok || len(s) != 36 {
		t.Errorf("GenerateAutoID(uuidv7) failed: %v", uuidv7)
	}

	// Prefixed
	prefixed := GenerateAutoID("prefix:usr")
	if s, ok := prefixed.(string); !ok || !strings.HasPrefix(s, "usr_") {
		t.Errorf("GenerateAutoID(prefix:usr) failed: %v", prefixed)
	}

	// Unknown defaults to ULID
	unknown := GenerateAutoID("unknown_strategy")
	if s, ok := unknown.(string); !ok || len(s) != 26 {
		t.Errorf("GenerateAutoID(unknown) failed: %v", unknown)
	}
}

func BenchmarkNewULID(b *testing.B) {
	for b.Loop() {
		NewULID()
	}
}

func BenchmarkNewSnowflake(b *testing.B) {
	node, _ := NewSnowflakeNode(1)
	for b.Loop() {
		node.NextID()
	}
}

func BenchmarkNewUUID(b *testing.B) {
	for b.Loop() {
		NewUUID()
	}
}

func BenchmarkNewUUIDv7(b *testing.B) {
	for b.Loop() {
		NewUUIDv7()
	}
}
