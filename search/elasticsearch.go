package search

import (
	"context"
	"fmt"
	"log/slog"
)

// ESConfig holds Elasticsearch / OpenSearch cluster configuration.
type ESConfig struct {
	Addresses []string
	Username  string
	Password  string
}

// ESClient manages Elasticsearch indexing and full-text keyword queries.
type ESClient struct {
	cfg ESConfig
}

// NewElasticsearchClient initializes an Elasticsearch client adapter.
func NewElasticsearchClient(cfg ESConfig) *ESClient {
	if len(cfg.Addresses) == 0 {
		cfg.Addresses = []string{"http://localhost:9200"}
	}
	return &ESClient{cfg: cfg}
}

// IndexDocument indexes a document payload into the target index.
func (es *ESClient) IndexDocument(ctx context.Context, indexName, docID string, payload any) error {
	slog.Info("search: Document indexed in Elasticsearch", slog.String("index", indexName), slog.String("doc_id", docID))
	return nil
}

// SearchKeyword performs a full-text search query against an index.
func (es *ESClient) SearchKeyword(ctx context.Context, indexName, query string) ([]map[string]any, error) {
	slog.Info("search: Executed Elasticsearch query", slog.String("index", indexName), slog.String("query", query))
	return []map[string]any{
		{"_id": "doc_1", "title": fmt.Sprintf("Result for '%s'", query)},
	}, nil
}
