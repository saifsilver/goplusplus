package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

const (
	defaultESRequestLimit  = 5 << 20
	defaultESResponseLimit = 10 << 20
	defaultESQueryLimit    = 4 << 10
)

var ErrElasticsearchNotConfigured = errors.New("search: Elasticsearch provider is not configured")

// ESConfig holds Elasticsearch cluster configuration. HTTPS is required unless
// AllowInsecureHTTP is explicitly enabled for a trusted development network.
type ESConfig struct {
	Addresses         []string
	Username          string
	Password          string
	APIKey            string
	HTTPClient        *http.Client
	AllowInsecureHTTP bool
	RequestTimeout    time.Duration
	MaxRequestBytes   int64
	MaxResponseBytes  int64
	MaxQueryBytes     int
	RetryAttempts     int
}

// ESClient manages Elasticsearch indexing and full-text keyword queries.
type ESClient struct {
	config    ESConfig
	addresses []*url.URL
	http      *http.Client
	next      atomic.Uint64
}

// NewElasticsearchClient validates the configuration and verifies cluster
// connectivity. It never returns a client that will silently accept operations.
func NewElasticsearchClient(ctx context.Context, config ESConfig) (*ESClient, error) {
	config, addresses, err := normalizeESConfig(config)
	if err != nil {
		return nil, err
	}
	client := &ESClient{config: config, addresses: addresses, http: config.HTTPClient}
	if _, err := client.do(ctx, http.MethodGet, "/", nil); err != nil {
		return nil, fmt.Errorf("search: verify Elasticsearch connection: %w", err)
	}
	return client, nil
}

// IndexDocument indexes a document using an idempotent PUT request.
func (client *ESClient) IndexDocument(ctx context.Context, indexName, docID string, payload any) error {
	if !client.configured() {
		return ErrElasticsearchNotConfigured
	}
	if err := validateESIndex(indexName); err != nil {
		return err
	}
	if err := validateESDocumentID(docID); err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("search: encode Elasticsearch document: %w", err)
	}
	if int64(len(body)) > client.config.MaxRequestBytes {
		return fmt.Errorf("search: Elasticsearch document exceeds %d bytes", client.config.MaxRequestBytes)
	}
	path := "/" + url.PathEscape(indexName) + "/_doc/" + url.PathEscape(docID)
	_, err = client.do(ctx, http.MethodPut, path, body)
	return err
}

// SearchKeyword performs a bounded simple-query-string search. Results retain
// Elasticsearch metadata while exposing the document under the stable "doc" key.
func (client *ESClient) SearchKeyword(
	ctx context.Context, indexName, query string,
) ([]map[string]any, error) {
	if !client.configured() {
		return nil, ErrElasticsearchNotConfigured
	}
	if err := validateESIndex(indexName); err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if query == "" || len(query) > client.config.MaxQueryBytes {
		return nil, fmt.Errorf("search: Elasticsearch query must contain 1 to %d bytes", client.config.MaxQueryBytes)
	}
	body, err := json.Marshal(map[string]any{
		"query": map[string]any{"simple_query_string": map[string]any{"query": query}},
	})
	if err != nil {
		return nil, fmt.Errorf("search: encode Elasticsearch query: %w", err)
	}
	responseBody, err := client.do(ctx, http.MethodPost, "/"+url.PathEscape(indexName)+"/_search", body)
	if err != nil {
		return nil, err
	}
	var response struct {
		Hits struct {
			Hits []struct {
				ID     string         `json:"_id"`
				Score  *float64       `json:"_score"`
				Source map[string]any `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, fmt.Errorf("search: decode Elasticsearch response: %w", err)
	}
	results := make([]map[string]any, 0, len(response.Hits.Hits))
	for _, hit := range response.Hits.Hits {
		result := map[string]any{"_id": hit.ID, "doc": hit.Source}
		if hit.Score != nil {
			result["_score"] = *hit.Score
		}
		results = append(results, result)
	}
	return results, nil
}

func (client *ESClient) configured() bool {
	return client != nil && client.http != nil && len(client.addresses) > 0
}

func (client *ESClient) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	requestCtx, cancel := context.WithTimeout(ctx, client.config.RequestTimeout)
	defer cancel()
	start := int(client.next.Add(1)-1) % len(client.addresses)
	var lastErr error
	for attempt := 0; attempt < client.config.RetryAttempts; attempt++ {
		address := client.addresses[(start+attempt)%len(client.addresses)]
		requestURL := *address
		requestURL.Path = strings.TrimSuffix(address.Path, "/") + path
		request, err := http.NewRequestWithContext(requestCtx, method, requestURL.String(), bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("search: build Elasticsearch request: %w", err)
		}
		request.Header.Set("Accept", "application/json")
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		client.authorize(request)
		response, err := client.http.Do(request)
		if err != nil {
			lastErr = fmt.Errorf("search: Elasticsearch request: %w", err)
			if requestCtx.Err() != nil {
				break
			}
			continue
		}
		responseBody, readErr := readBoundedESBody(response.Body, client.config.MaxResponseBytes)
		closeErr := response.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, fmt.Errorf("search: close Elasticsearch response: %w", closeErr)
		}
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return responseBody, nil
		}
		lastErr = fmt.Errorf("search: Elasticsearch returned status %d", response.StatusCode)
		if !retryableESStatus(response.StatusCode) {
			return nil, lastErr
		}
	}
	return nil, lastErr
}

func (client *ESClient) authorize(request *http.Request) {
	if client.config.APIKey != "" {
		request.Header.Set("Authorization", "ApiKey "+client.config.APIKey)
		return
	}
	if client.config.Username != "" {
		request.SetBasicAuth(client.config.Username, client.config.Password)
	}
}

func normalizeESConfig(config ESConfig) (ESConfig, []*url.URL, error) {
	if len(config.Addresses) == 0 {
		return ESConfig{}, nil, ErrElasticsearchNotConfigured
	}
	if config.APIKey != "" && (config.Username != "" || config.Password != "") {
		return ESConfig{}, nil, errors.New("search: configure either Elasticsearch API key or basic authentication")
	}
	if (config.Username == "") != (config.Password == "") {
		return ESConfig{}, nil, errors.New("search: Elasticsearch username and password must be configured together")
	}
	addresses := make([]*url.URL, 0, len(config.Addresses))
	for _, rawAddress := range config.Addresses {
		address, err := url.Parse(rawAddress)
		if err != nil || address.Host == "" || address.User != nil || address.RawQuery != "" || address.Fragment != "" {
			return ESConfig{}, nil, errors.New("search: invalid Elasticsearch address")
		}
		if address.Scheme != "https" && !(config.AllowInsecureHTTP && address.Scheme == "http") {
			return ESConfig{}, nil, errors.New("search: Elasticsearch address must use HTTPS")
		}
		addresses = append(addresses, address)
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{}
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = 10 * time.Second
	}
	if config.MaxRequestBytes <= 0 {
		config.MaxRequestBytes = defaultESRequestLimit
	}
	if config.MaxResponseBytes <= 0 {
		config.MaxResponseBytes = defaultESResponseLimit
	}
	if config.MaxQueryBytes <= 0 {
		config.MaxQueryBytes = defaultESQueryLimit
	}
	if config.RetryAttempts <= 0 {
		config.RetryAttempts = len(addresses) + 1
	}
	return config, addresses, nil
}

func validateESIndex(index string) error {
	if index == "" || len(index) > 255 || index != strings.ToLower(index) || index == "." || index == ".." {
		return errors.New("search: invalid Elasticsearch index name")
	}
	if strings.ContainsAny(index, `\\/*?"<>| ,#:`) || strings.ContainsAny(index, "\r\n\t") || strings.Contains("_-+", index[:1]) {
		return errors.New("search: invalid Elasticsearch index name")
	}
	return nil
}

func validateESDocumentID(id string) error {
	if id == "" || len(id) > 512 || strings.ContainsAny(id, "/\\\r\n\x00") {
		return errors.New("search: invalid Elasticsearch document ID")
	}
	return nil
}

func readBoundedESBody(body io.Reader, limit int64) ([]byte, error) {
	content, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("search: read Elasticsearch response: %w", err)
	}
	if int64(len(content)) > limit {
		return nil, fmt.Errorf("search: Elasticsearch response exceeds %d bytes", limit)
	}
	return content, nil
}

func retryableESStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}
