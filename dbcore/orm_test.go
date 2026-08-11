package dbcore

import (
	"context"
	"reflect"
	"testing"
	"time"
)

type Product struct {
	ID        int64     `db:"id,primary_key"`
	Name      string    `db:"name"`
	Price     float64   `db:"price"`
	CreatedAt time.Time `db:"created_at,auto_create"`
}

type implicitColumnNames struct {
	ID         int64
	APIKey     string
	HTTPServer string
	CreatedAt  time.Time
}

func TestGetStructMetaUsesSnakeCaseForImplicitColumns(t *testing.T) {
	meta := getStructMeta(reflect.TypeOf(implicitColumnNames{}))
	want := []string{"id", "api_key", "http_server", "created_at"}
	if !reflect.DeepEqual(meta.columns, want) {
		t.Fatalf("expected columns %v, got %v", want, meta.columns)
	}
}

func TestORMAndTypedQueriesSuite(t *testing.T) {
	ctx := context.Background()

	client, err := NewClient(ctx, Config{RWDSN: ":memory:"})
	if err != nil {
		t.Fatalf("failed creating memory db client: %v", err)
	}
	defer client.Close()

	orm := NewORM[Product](client, "products")

	// 1. AutoMigrate
	if err := orm.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	// 2. Save (Insert)
	p1 := &Product{Name: "Laptop", Price: 1299.99}
	if err := orm.Save(ctx, p1); err != nil {
		t.Fatalf("Save insert failed: %v", err)
	}

	p2 := &Product{Name: "Keyboard", Price: 99.50}
	if err := orm.Save(ctx, p2); err != nil {
		t.Fatalf("Save insert p2 failed: %v", err)
	}

	// 3. FindByID
	found, err := orm.FindByID(ctx, p1.ID)
	if err != nil || found == nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found.Name != "Laptop" {
		t.Errorf("expected Name = 'Laptop', got '%s'", found.Name)
	}

	// 4. Find with Where and OrderBy
	items, err := orm.Where("name", "Keyboard").OrderBy("id DESC").Find(ctx)
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}
	if len(items) == 0 {
		t.Errorf("expected at least 1 item matching Keyboard")
	}

	// 5. Paginate
	pItems, total, err := orm.Paginate(ctx, 1, 10)
	if err != nil || total == 0 {
		t.Fatalf("Paginate failed: %v, total=%d", err, total)
	}
	if len(pItems) == 0 {
		t.Errorf("expected paginated items")
	}

	// 6. Save (Update)
	p1.Price = 1199.99
	if err := orm.Save(ctx, p1); err != nil {
		t.Fatalf("Save update failed: %v", err)
	}

	// 7. Typed Raw Queries
	rawItems, err := QueryTyped[Product](ctx, client, "SELECT * FROM products WHERE price > $1", 50.0)
	if err != nil {
		t.Fatalf("QueryTyped failed: %v", err)
	}
	if len(rawItems) == 0 {
		t.Errorf("expected rawItems matching price > 50.0")
	}

	rawSingle, err := QueryRowTyped[Product](ctx, client, "SELECT * FROM products WHERE name = $1 LIMIT 1", "Keyboard")
	if err != nil || rawSingle == nil {
		t.Fatalf("QueryRowTyped failed: %v", err)
	}

	rawPaginated, count, err := QueryPaginated[Product](ctx, client, "SELECT * FROM products", 1, 5)
	if err != nil || count == 0 || len(rawPaginated) == 0 {
		t.Fatalf("QueryPaginated failed: %v", err)
	}

	// 8. Delete
	if err := orm.Delete(ctx, p1); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}
