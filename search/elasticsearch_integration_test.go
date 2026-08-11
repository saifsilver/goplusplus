package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestElasticsearchIndexAndSearchIntegration(t *testing.T) {
	var indexed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "ApiKey test-key" {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/":
			_, _ = writer.Write([]byte(`{"version":{"number":"9.0.0"}}`))
		case request.Method == http.MethodPut && request.URL.Path == "/users/_doc/usr_1":
			var document map[string]any
			if err := json.NewDecoder(request.Body).Decode(&document); err != nil || document["name"] != "Ada" {
				t.Errorf("unexpected index payload: %v, %v", document, err)
			}
			indexed.Store(true)
			writer.WriteHeader(http.StatusCreated)
			_, _ = writer.Write([]byte(`{"result":"created"}`))
		case request.Method == http.MethodPost && request.URL.Path == "/users/_search":
			if !indexed.Load() {
				t.Error("search happened before indexing")
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"hits":{"hits":[{"_id":"usr_1","_score":1.5,"_source":{"name":"Ada"}}]}}`))
		default:
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := NewElasticsearchClient(context.Background(), ESConfig{
		Addresses: []string{server.URL}, APIKey: "test-key", AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatalf("NewElasticsearchClient: %v", err)
	}
	if err := client.IndexDocument(context.Background(), "users", "usr_1", map[string]any{"name": "Ada"}); err != nil {
		t.Fatalf("IndexDocument: %v", err)
	}
	results, err := client.SearchKeyword(context.Background(), "users", "Ada")
	if err != nil {
		t.Fatalf("SearchKeyword: %v", err)
	}
	if len(results) != 1 || results[0]["_id"] != "usr_1" || results[0]["_score"] != 1.5 {
		t.Fatalf("unexpected results: %#v", results)
	}
	document, ok := results[0]["doc"].(map[string]any)
	if !ok || document["name"] != "Ada" {
		t.Fatalf("unexpected document: %#v", results[0]["doc"])
	}
}

func TestElasticsearchValidationAndStatusErrors(t *testing.T) {
	if _, err := NewElasticsearchClient(context.Background(), ESConfig{Addresses: []string{"http://localhost:9200"}}); err == nil {
		t.Fatal("expected insecure HTTP configuration to fail")
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/" {
			_, _ = writer.Write([]byte(`{}`))
			return
		}
		http.Error(writer, "secret internal detail", http.StatusBadRequest)
	}))
	defer server.Close()
	client, err := NewElasticsearchClient(context.Background(), ESConfig{
		Addresses: []string{server.URL}, AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.IndexDocument(context.Background(), "Invalid", "id", map[string]any{}); err == nil {
		t.Fatal("expected invalid index to fail")
	}
	if err := client.IndexDocument(context.Background(), "users", "id", map[string]any{}); err == nil || err.Error() != "search: Elasticsearch returned status 400" {
		t.Fatalf("unexpected status error: %v", err)
	}
}
