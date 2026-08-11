package search

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

func productSchema(t *testing.T) Schema {
	t.Helper()
	schema, err := NewSchema(
		AttributeDefinition{Key: "title", Type: AttributeString, Searchable: true},
		AttributeDefinition{Key: "brand", Type: AttributeEnum, Filterable: true, Facetable: true, Sortable: true, EnumValues: []string{"Nike", "Adidas"}},
		AttributeDefinition{Key: "color", Type: AttributeString, Filterable: true, Facetable: true},
		AttributeDefinition{Key: "sizes", Type: AttributeInteger, MultiValue: true, Filterable: true, Facetable: true},
		AttributeDefinition{Key: "price", Type: AttributeDecimal, Filterable: true, Sortable: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func indexedProductResource(t *testing.T) *Resource {
	t.Helper()
	resource, err := NewResource("products", productSchema(t), NewMemoryBackend())
	if err != nil {
		t.Fatal(err)
	}
	documents := []Document{
		{ID: "p1", Attributes: map[string]any{"title": "Road running shoe", "brand": "Nike", "color": "red", "sizes": []int{8, 9}, "price": 100.0}},
		{ID: "p2", Attributes: map[string]any{"title": "Daily running shoe", "brand": "Adidas", "color": "blue", "sizes": []int{9, 10}, "price": 80.0}},
		{ID: "p3", Attributes: map[string]any{"title": "Trail running shoe", "brand": "Nike", "color": "blue", "sizes": []int{10}, "price": 130.0}},
	}
	for _, document := range documents {
		if err := resource.Index(context.Background(), document); err != nil {
			t.Fatal(err)
		}
	}
	return resource
}

func TestSchemaRejectsInvalidDynamicAttributes(t *testing.T) {
	schema := productSchema(t)
	err := schema.ValidateDocument(Document{ID: "p1", Attributes: map[string]any{"price": "cheap"}})
	if err == nil || !strings.Contains(err.Error(), "invalid decimal") {
		t.Fatalf("expected decimal validation error, got %v", err)
	}
	_, err = schema.NormalizeRequest(SearchRequest{Filters: []Filter{{Field: "title", Operator: OperatorEqual, Value: "shoe"}}})
	if err == nil || !strings.Contains(err.Error(), "not filterable") {
		t.Fatalf("expected field allowlist error, got %v", err)
	}
}

func TestMemoryBackendFiltersAndDisjunctiveFacets(t *testing.T) {
	resource := indexedProductResource(t)
	result, err := resource.Search(context.Background(), SearchRequest{
		Query: "running",
		Filters: []Filter{
			{Field: "brand", Operator: OperatorEqual, Value: "Nike"},
			{Field: "sizes", Operator: OperatorEqual, Value: 9},
		},
		Facets: []FacetRequest{{Field: "brand"}, {Field: "color"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Items) != 1 || result.Items[0].ID != "p1" {
		t.Fatalf("unexpected filtered result: %#v", result)
	}
	brandBuckets := result.Facets["brand"].Buckets
	if len(brandBuckets) != 2 || brandBuckets[0].Value != "Adidas" || brandBuckets[0].Count != 1 || brandBuckets[1].Value != "Nike" || brandBuckets[1].Count != 1 {
		t.Fatalf("unexpected disjunctive brand buckets: %#v", brandBuckets)
	}
	colorBuckets := result.Facets["color"].Buckets
	if len(colorBuckets) != 1 || colorBuckets[0].Value != "red" || colorBuckets[0].Count != 1 {
		t.Fatalf("unexpected color buckets: %#v", colorBuckets)
	}
}

func TestMemoryBackendSortAndCursor(t *testing.T) {
	resource := indexedProductResource(t)
	first, err := resource.Search(context.Background(), SearchRequest{
		Sort: []Sort{{Field: "price", Direction: SortAscending}}, Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.Items[0].ID != "p2" || first.NextCursor == "" {
		t.Fatalf("unexpected first page: %#v", first)
	}
	second, err := resource.Search(context.Background(), SearchRequest{
		Sort: []Sort{{Field: "price", Direction: SortAscending}}, Limit: 2, Cursor: first.NextCursor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].ID != "p3" || second.NextCursor != "" {
		t.Fatalf("unexpected second page: %#v", second)
	}
}

func TestResourceScopeAppliesToHitsAndFacets(t *testing.T) {
	schema := productSchema(t)
	backend := NewMemoryBackend()
	resource, err := NewResource("products", schema, backend, WithScope(func(context.Context) ([]Filter, error) {
		return []Filter{{Field: "brand", Operator: OperatorEqual, Value: "Nike"}}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, document := range []Document{
		{ID: "p1", Attributes: map[string]any{"title": "Shoe", "brand": "Nike", "color": "red", "price": 10.0}},
		{ID: "p2", Attributes: map[string]any{"title": "Shoe", "brand": "Adidas", "color": "blue", "price": 10.0}},
	} {
		if err := resource.Index(context.Background(), document); err != nil {
			t.Fatal(err)
		}
	}
	result, err := resource.Search(context.Background(), SearchRequest{Facets: []FacetRequest{{Field: "color"}, {Field: "brand"}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Items[0].ID != "p1" || len(result.Facets["color"].Buckets) != 1 {
		t.Fatalf("scope did not constrain hits and facets: %#v", result)
	}
	if buckets := result.Facets["brand"].Buckets; len(buckets) != 1 || buckets[0].Value != "Nike" {
		t.Fatalf("disjunctive facet removed mandatory scope: %#v", buckets)
	}
}

type inertDatabase struct{}

func (inertDatabase) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, nil
}

func (inertDatabase) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, nil
}

func TestDatabaseCompilerParameterizesDynamicValues(t *testing.T) {
	backend, err := NewDatabaseBackend(inertDatabase{}, DatabaseConfig{})
	if err != nil {
		t.Fatal(err)
	}
	injection := "' OR TRUE --"
	plan, err := backend.Compile("products", productSchema(t), SearchRequest{
		Filters: []Filter{{Field: "color", Operator: OperatorEqual, Value: injection}},
		Sort:    []Sort{{Field: "price", Direction: SortDescending}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plan.Hits.Statement, injection) {
		t.Fatalf("filter value was interpolated into SQL: %s", plan.Hits.Statement)
	}
	if !strings.Contains(plan.Hits.Statement, "d.document @>") || !strings.Contains(plan.Hits.Statement, "NULLS LAST") {
		t.Fatalf("unexpected compiled SQL: %s", plan.Hits.Statement)
	}
	if _, err := NewDatabaseBackend(inertDatabase{}, DatabaseConfig{Table: "documents; DROP TABLE users"}); err == nil {
		t.Fatal("expected unsafe table name to be rejected")
	}
}
