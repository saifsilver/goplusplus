package search

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type MemoryBackend struct {
	mu        sync.RWMutex
	documents map[string]map[string]Document
}

func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{documents: make(map[string]map[string]Document)}
}

func (b *MemoryBackend) Name() string {
	return "memory"
}

func (b *MemoryBackend) Index(_ context.Context, resource string, _ Schema, document Document) error {
	cloned, err := cloneDocument(document)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.documents[resource] == nil {
		b.documents[resource] = make(map[string]Document)
	}
	b.documents[resource][document.ID] = cloned
	return nil
}

func (b *MemoryBackend) Delete(_ context.Context, resource, documentID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.documents[resource], documentID)
	return nil
}

func (b *MemoryBackend) Search(ctx context.Context, resource string, schema Schema, request SearchRequest) (SearchResult, error) {
	documents, err := b.snapshot(resource)
	if err != nil {
		return SearchResult{}, err
	}
	matched, err := filterDocuments(ctx, documents, schema, request.Query, request.Filters)
	if err != nil {
		return SearchResult{}, err
	}
	sortDocuments(matched, schema, request)
	items, nextCursor, err := pageDocuments(matched, request)
	if err != nil {
		return SearchResult{}, err
	}
	facets, err := buildMemoryFacets(ctx, documents, schema, request)
	if err != nil {
		return SearchResult{}, err
	}
	return SearchResult{
		Items: items, Total: int64(len(matched)), Facets: facets,
		NextCursor: nextCursor, Backend: b.Name(),
	}, nil
}

func (b *MemoryBackend) snapshot(resource string) ([]Document, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	documents := make([]Document, 0, len(b.documents[resource]))
	for _, document := range b.documents[resource] {
		cloned, err := cloneDocument(document)
		if err != nil {
			return nil, err
		}
		documents = append(documents, cloned)
	}
	return documents, nil
}

type scoredDocument struct {
	document Document
	score    float64
}

func filterDocuments(ctx context.Context, documents []Document, schema Schema, query string, filters []Filter) ([]scoredDocument, error) {
	matched := make([]scoredDocument, 0, len(documents))
	for _, document := range documents {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		score, queryMatch := matchQuery(document, schema, query)
		if queryMatch && matchFilters(document, schema, filters) {
			matched = append(matched, scoredDocument{document: document, score: score})
		}
	}
	return matched, nil
}

func matchQuery(document Document, schema Schema, query string) (float64, bool) {
	if query == "" {
		return 0, true
	}
	needle := strings.ToLower(query)
	var score float64
	for key, definition := range schema.Attributes {
		if !definition.Searchable {
			continue
		}
		for _, value := range attributeValues(document.Attributes[key]) {
			if strings.Contains(strings.ToLower(fmt.Sprint(value)), needle) {
				score++
			}
		}
	}
	return score, score > 0
}

func matchFilters(document Document, schema Schema, filters []Filter) bool {
	for _, filter := range filters {
		definition := schema.Attributes[filter.Field]
		value, exists := document.Attributes[filter.Field]
		if !matchFilter(value, exists, definition, filter) {
			return false
		}
	}
	return true
}

func matchFilter(value any, exists bool, definition AttributeDefinition, filter Filter) bool {
	if filter.Operator == OperatorExists {
		expected, ok := filter.Value.(bool)
		if !ok {
			expected = true
		}
		return exists == expected
	}
	if !exists {
		return filter.Operator == OperatorNotEqual || filter.Operator == OperatorNotIn
	}
	values := attributeValues(value)
	switch filter.Operator {
	case OperatorEqual:
		return anyValueMatches(values, func(item any) bool { return equalValue(definition, item, filter.Value) })
	case OperatorNotEqual:
		return !anyValueMatches(values, func(item any) bool { return equalValue(definition, item, filter.Value) })
	case OperatorIn, OperatorNotIn:
		candidates, _ := sliceValues(filter.Value)
		matched := anyValueMatches(values, func(item any) bool {
			return anyValueMatches(candidates, func(candidate any) bool { return equalValue(definition, item, candidate) })
		})
		return matched == (filter.Operator == OperatorIn)
	case OperatorContains:
		return anyValueMatches(values, func(item any) bool {
			return strings.Contains(strings.ToLower(fmt.Sprint(item)), strings.ToLower(fmt.Sprint(filter.Value)))
		})
	case OperatorBetween:
		bounds, _ := sliceValues(filter.Value)
		return compareValue(definition, value, bounds[0]) >= 0 && compareValue(definition, value, bounds[1]) <= 0
	default:
		return matchRange(value, definition, filter)
	}
}

func matchRange(value any, definition AttributeDefinition, filter Filter) bool {
	comparison := compareValue(definition, value, filter.Value)
	switch filter.Operator {
	case OperatorGreaterThan:
		return comparison > 0
	case OperatorGreaterOrEq:
		return comparison >= 0
	case OperatorLessThan:
		return comparison < 0
	case OperatorLessOrEq:
		return comparison <= 0
	default:
		return false
	}
}

func attributeValues(value any) []any {
	if values, ok := sliceValues(value); ok {
		return values
	}
	if value == nil {
		return nil
	}
	return []any{value}
}

func anyValueMatches(values []any, predicate func(any) bool) bool {
	for _, value := range values {
		if predicate(value) {
			return true
		}
	}
	return false
}

func equalValue(definition AttributeDefinition, left, right any) bool {
	return compareValue(definition, left, right) == 0
}

func compareValue(definition AttributeDefinition, left, right any) int {
	switch definition.Type {
	case AttributeInteger, AttributeDecimal:
		leftNumber, _ := numberValue(left)
		rightNumber, _ := numberValue(right)
		return compareOrdered(leftNumber, rightNumber)
	case AttributeBoolean:
		return compareOrdered(fmt.Sprint(left), fmt.Sprint(right))
	case AttributeDate:
		leftDate := dateValue(left)
		rightDate := dateValue(right)
		return compareOrdered(leftDate.UnixNano(), rightDate.UnixNano())
	default:
		return compareOrdered(fmt.Sprint(left), fmt.Sprint(right))
	}
}

func compareOrdered[T ~float64 | ~int64 | ~string](left, right T) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func dateValue(value any) time.Time {
	if date, ok := value.(time.Time); ok {
		return date
	}
	date, _ := parseDate(fmt.Sprint(value))
	return date
}

func sortDocuments(documents []scoredDocument, schema Schema, request SearchRequest) {
	sort.SliceStable(documents, func(left, right int) bool {
		leftDoc, rightDoc := documents[left], documents[right]
		for _, sortField := range request.Sort {
			leftValue, leftExists := leftDoc.document.Attributes[sortField.Field]
			rightValue, rightExists := rightDoc.document.Attributes[sortField.Field]
			leftExists = leftExists && leftValue != nil
			rightExists = rightExists && rightValue != nil
			if leftExists != rightExists {
				return leftExists
			}
			comparison := compareValue(schema.Attributes[sortField.Field], leftValue, rightValue)
			if comparison != 0 {
				return (comparison < 0) == (sortField.Direction == SortAscending)
			}
		}
		if len(request.Sort) == 0 && leftDoc.score != rightDoc.score {
			return leftDoc.score > rightDoc.score
		}
		return leftDoc.document.ID < rightDoc.document.ID
	})
}

func pageDocuments(documents []scoredDocument, request SearchRequest) ([]Hit, string, error) {
	offset, err := decodeCursor(request.Cursor)
	if err != nil {
		return nil, "", err
	}
	if offset >= len(documents) {
		return []Hit{}, "", nil
	}
	end := min(offset+request.Limit, len(documents))
	items := make([]Hit, 0, end-offset)
	for _, item := range documents[offset:end] {
		items = append(items, Hit{ID: item.document.ID, Score: item.score, Attributes: item.document.Attributes})
	}
	nextCursor := ""
	if end < len(documents) {
		nextCursor = encodeCursor(end)
	}
	return items, nextCursor, nil
}

func buildMemoryFacets(ctx context.Context, documents []Document, schema Schema, request SearchRequest) (map[string]FacetResult, error) {
	if len(request.Facets) == 0 {
		return nil, nil
	}
	results := make(map[string]FacetResult, len(request.Facets))
	for _, facet := range request.Facets {
		filters := request.Filters
		if facet.Mode == FacetDisjunctive {
			filters = filtersWithoutField(filters, facet.Field)
		}
		matched, err := filterDocuments(ctx, documents, schema, request.Query, filters)
		if err != nil {
			return nil, err
		}
		results[facet.Field] = aggregateFacet(matched, schema.Attributes[facet.Field], facet)
	}
	return results, nil
}

func filtersWithoutField(filters []Filter, field string) []Filter {
	result := make([]Filter, 0, len(filters))
	for _, filter := range filters {
		if filter.Field != field || filter.mandatory {
			result = append(result, filter)
		}
	}
	return result
}

func aggregateFacet(documents []scoredDocument, definition AttributeDefinition, request FacetRequest) FacetResult {
	counts := make(map[string]FacetBucket)
	for _, item := range documents {
		values := uniqueAttributeValues(item.document.Attributes[request.Field], definition)
		if len(values) == 0 && request.IncludeMissing {
			values = []any{nil}
		}
		for _, value := range values {
			key := fmt.Sprintf("%T:%v", value, value)
			bucket := counts[key]
			bucket.Value = value
			bucket.Count++
			counts[key] = bucket
		}
	}
	buckets := make([]FacetBucket, 0, len(counts))
	for _, bucket := range counts {
		buckets = append(buckets, bucket)
	}
	sortFacetBuckets(buckets, definition, request.Sort)
	if len(buckets) > request.Limit {
		buckets = buckets[:request.Limit]
	}
	return FacetResult{Field: request.Field, Buckets: buckets}
}

func uniqueAttributeValues(value any, definition AttributeDefinition) []any {
	unique := make(map[string]any)
	for _, item := range attributeValues(value) {
		key := fmt.Sprintf("%T:%v", item, item)
		unique[key] = item
	}
	values := make([]any, 0, len(unique))
	for _, item := range unique {
		values = append(values, item)
	}
	sort.Slice(values, func(left, right int) bool {
		return compareValue(definition, values[left], values[right]) < 0
	})
	return values
}

func sortFacetBuckets(buckets []FacetBucket, definition AttributeDefinition, facetSort FacetSort) {
	sort.Slice(buckets, func(left, right int) bool {
		if facetSort == FacetSortCount && buckets[left].Count != buckets[right].Count {
			return buckets[left].Count > buckets[right].Count
		}
		return compareValue(definition, buckets[left].Value, buckets[right].Value) < 0
	})
}

func cloneDocument(document Document) (Document, error) {
	encoded, err := json.Marshal(document.Attributes)
	if err != nil {
		return Document{}, fmt.Errorf("search: encode document %q: %w", document.ID, err)
	}
	attributes := make(map[string]any)
	if err := json.Unmarshal(encoded, &attributes); err != nil {
		return Document{}, fmt.Errorf("search: decode document %q: %w", document.ID, err)
	}
	return Document{ID: document.ID, Attributes: attributes}, nil
}
