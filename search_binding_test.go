package gpp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/search"
)

func testSearchResource(t *testing.T) *search.Resource {
	t.Helper()
	schema, err := search.NewSchema(
		search.AttributeDefinition{Key: "title", Type: search.AttributeString, Searchable: true},
		search.AttributeDefinition{Key: "brand", Type: search.AttributeEnum, Filterable: true, Facetable: true, EnumValues: []string{"Nike", "Adidas"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	resource, err := search.NewResource("products", schema, search.NewMemoryBackend())
	if err != nil {
		t.Fatal(err)
	}
	for _, document := range []search.Document{
		{ID: "p1", Attributes: map[string]any{"title": "Running shoe", "brand": "Nike"}},
		{ID: "p2", Attributes: map[string]any{"title": "Trail shoe", "brand": "Adidas"}},
	} {
		if err := resource.Index(context.Background(), document); err != nil {
			t.Fatal(err)
		}
	}
	return resource
}

func TestBindSearchResource(t *testing.T) {
	app := gpp.New()
	resource := testSearchResource(t)
	gpp.BindSearchResource(app.Group("/api"), "/products/search", resource)
	body := bytes.NewBufferString(`{"filters":[{"field":"brand","operator":"eq","value":"Nike"}],"facets":[{"field":"brand"}]}`)
	request := httptest.NewRequest(http.MethodPost, "/api/products/search", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var result search.SearchResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Items[0].ID != "p1" || len(result.Facets["brand"].Buckets) != 2 {
		t.Fatalf("unexpected REST search response: %#v", result)
	}
}

func TestBindSearchGraphQL(t *testing.T) {
	app := gpp.New()
	resource := testSearchResource(t)
	registry, err := search.NewRegistry(resource)
	if err != nil {
		t.Fatal(err)
	}
	if err := gpp.BindSearchGraphQL(app, "productSearch", registry); err != nil {
		t.Fatal(err)
	}
	app.POST("/graphql", app.AutoGraphQLHandler())
	payload := map[string]any{
		"query": `query ProductSearch($resource: String!, $request: SearchRequestInput!) {
			productSearch(resource: $resource, request: $request) {
				total backend items { id attributes } facets { field buckets { value count } }
			}
		}`,
		"variables": map[string]any{
			"resource": "products",
			"request": map[string]any{
				"filters": []any{map[string]any{"field": "brand", "operator": "eq", "value": "Nike"}},
				"facets":  []any{map[string]any{"field": "brand"}},
			},
		},
	}
	body, _ := json.Marshal(payload)
	request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var result struct {
		Data struct {
			ProductSearch struct {
				Total  int `json:"total"`
				Items  []search.Hit
				Facets []search.FacetResult
			} `json:"productSearch"`
		} `json:"data"`
		Errors []any `json:"errors"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected GraphQL errors: %s", response.Body.String())
	}
	if result.Data.ProductSearch.Total != 1 || result.Data.ProductSearch.Items[0].ID != "p1" || len(result.Data.ProductSearch.Facets) != 1 {
		t.Fatalf("unexpected GraphQL response: %s", response.Body.String())
	}
}

func TestAutoGraphQLRejectsAmplifiedQuery(t *testing.T) {
	app := gpp.New()
	app.POST("/graphql", app.AutoGraphQLHandler())
	payload, _ := json.Marshal(map[string]any{"query": "{ " + strings.Repeat("gpp ", 101) + "}"})
	request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected complexity rejection, got %d: %s", response.Code, response.Body.String())
	}
}
