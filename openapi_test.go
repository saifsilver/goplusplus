package gpp_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gpp "github.com/saifsilver/goplusplus"
)

func TestAutoSwaggerUIAndGraphQL(t *testing.T) {
	app := gpp.New()

	app.GET("/users/:id", func(c *gpp.Context) error {
		return c.JSON(http.StatusOK, gpp.H{"id": c.Param("id")})
	})

	app.GET("/swagger", app.AutoSwaggerUI())
	app.GET("/graphql", app.AutoGraphQLPlayground("/graphql"))
	app.POST("/graphql", app.AutoGraphQLHandler())

	// Test Swagger UI
	reqSwag := httptest.NewRequest(http.MethodGet, "/swagger", nil)
	wSwag := httptest.NewRecorder()
	app.ServeHTTP(wSwag, reqSwag)

	if wSwag.Code != http.StatusOK {
		t.Errorf("expected Swagger UI 200 OK, got %d", wSwag.Code)
	}
	if !strings.Contains(wSwag.Body.String(), "SwaggerUIBundle") {
		t.Errorf("expected Swagger UI body to contain SwaggerUIBundle")
	}

	// Test GraphQL Playground
	reqGQL := httptest.NewRequest(http.MethodGet, "/graphql", nil)
	wGQL := httptest.NewRecorder()
	app.ServeHTTP(wGQL, reqGQL)

	if wGQL.Code != http.StatusOK {
		t.Errorf("expected GraphQL Playground 200 OK, got %d", wGQL.Code)
	}

	// Test Auto GraphQL Handler
	reqGQLPost := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(`{"query":"{ users }"}`))
	reqGQLPost.Header.Set("Content-Type", "application/json")
	wGQLPost := httptest.NewRecorder()
	app.ServeHTTP(wGQLPost, reqGQLPost)

	if wGQLPost.Code != http.StatusOK {
		t.Errorf("expected GraphQL Post 200 OK, got %d", wGQLPost.Code)
	}
}
