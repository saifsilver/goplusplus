package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/middleware"
)

func TestRequestIDAndObservability(t *testing.T) {
	app := gpp.New()
	app.Use(
		middleware.RequestID(),
		middleware.Observability(),
		middleware.Logger(),
	)

	app.GET("/metrics", middleware.Prometheus())

	app.GET("/ping", func(c *gpp.Context) error {
		if c.RequestID() == "" {
			t.Errorf("expected RequestID to be populated")
		}
		return c.String(http.StatusOK, "%s", "pong")
	})

	reqPing := httptest.NewRequest(http.MethodGet, "/ping", nil)
	wPing := httptest.NewRecorder()
	app.ServeHTTP(wPing, reqPing)

	if wPing.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", wPing.Code)
	}
	if wPing.Header().Get("X-Request-ID") == "" {
		t.Errorf("expected X-Request-ID header to be populated")
	}

	reqMetrics := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	wMetrics := httptest.NewRecorder()
	app.ServeHTTP(wMetrics, reqMetrics)

	if wMetrics.Code != http.StatusOK {
		t.Errorf("expected /metrics status 200 OK, got %d", wMetrics.Code)
	}
}

func TestIdempotencyAndSingleflight(t *testing.T) {
	app := gpp.New()
	app.Use(
		middleware.Idempotency(),
		middleware.Singleflight(),
	)

	counter := 0
	app.POST("/charge", func(c *gpp.Context) error {
		counter++
		return c.JSON(http.StatusOK, gpp.H{"charge_count": counter})
	})

	// Request 1 with Idempotency-Key
	req1 := httptest.NewRequest(http.MethodPost, "/charge", nil)
	req1.Header.Set("Idempotency-Key", "key_abc123")
	w1 := httptest.NewRecorder()
	app.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", w1.Code)
	}

	// Request 2 with SAME Idempotency-Key (should return cached response, counter stays 1)
	req2 := httptest.NewRequest(http.MethodPost, "/charge", nil)
	req2.Header.Set("Idempotency-Key", "key_abc123")
	w2 := httptest.NewRecorder()
	app.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("expected 200 OK for cached idempotency request, got %d", w2.Code)
	}
	if counter != 1 {
		t.Errorf("expected counter to be 1 due to idempotency, got %d", counter)
	}
}

func TestSwaggerAndGraphQLMiddleware(t *testing.T) {
	app := gpp.New()

	app.GET("/swagger", middleware.SwaggerUI(`{"openapi":"3.0.0"}`))
	app.GET("/graphql", middleware.GraphQLPlayground("/graphql"))
	app.POST("/graphql", middleware.GraphQLHandler(func(query string, variables map[string]any) (any, error) {
		return map[string]string{"hero": "Superman"}, nil
	}))

	// Swagger UI test
	reqSwag := httptest.NewRequest(http.MethodGet, "/swagger", nil)
	wSwag := httptest.NewRecorder()
	app.ServeHTTP(wSwag, reqSwag)
	if wSwag.Code != http.StatusOK {
		t.Errorf("expected Swagger UI 200, got %d", wSwag.Code)
	}

	// GraphQL Playground test
	reqPlay := httptest.NewRequest(http.MethodGet, "/graphql", nil)
	wPlay := httptest.NewRecorder()
	app.ServeHTTP(wPlay, reqPlay)
	if wPlay.Code != http.StatusOK {
		t.Errorf("expected GraphQL Playground 200, got %d", wPlay.Code)
	}

	// GraphQL Handler test
	reqGQL := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(`{"query":"{ hero }"}`))
	reqGQL.Header.Set("Content-Type", "application/json")
	wGQL := httptest.NewRecorder()
	app.ServeHTTP(wGQL, reqGQL)
	if wGQL.Code != http.StatusOK {
		t.Errorf("expected GraphQL Handler 200, got %d", wGQL.Code)
	}
}

func TestGRPCMultiplexMiddleware(t *testing.T) {
	app := gpp.New()
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("grpc_ok"))
	})
	app.Use(middleware.GRPCMultiplex(dummyHandler))

	app.GET("/api", func(c *gpp.Context) error {
		return c.String(http.StatusOK, "%s", "http_ok")
	})

	// Standard HTTP request
	req1 := httptest.NewRequest(http.MethodGet, "/api", nil)
	w1 := httptest.NewRecorder()
	app.ServeHTTP(w1, req1)
	if w1.Body.String() != "http_ok" {
		t.Errorf("expected http_ok, got %s", w1.Body.String())
	}

	// Simulated gRPC request over HTTP/2
	req2 := httptest.NewRequest(http.MethodPost, "/api", nil)
	req2.ProtoMajor = 2
	req2.Header.Set("Content-Type", "application/grpc")
	w2 := httptest.NewRecorder()
	app.ServeHTTP(w2, req2)
	if w2.Body.String() != "grpc_ok" {
		t.Errorf("expected grpc_ok, got %s", w2.Body.String())
	}
}
