package search

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultLimit is the page size used when a request omits a limit.
	DefaultLimit = 20
	// MaxLimit is the largest accepted search page size.
	MaxLimit = 100
	// MaxFilters is the maximum number of filters in one request.
	MaxFilters = 20
	// MaxFacets is the maximum number of facet requests in one search.
	MaxFacets = 10
	// MaxFacetBuckets is the upper bound for buckets returned by one facet.
	MaxFacetBuckets = 100
	// MaxQueryLength is the maximum keyword-query length in bytes.
	MaxQueryLength = 512
	// DefaultFacetLimit is the bucket count used when a facet omits a limit.
	DefaultFacetLimit = 20
)

// AttributeType identifies the scalar representation of a searchable attribute.
type AttributeType string

const (
	// AttributeString represents textual values.
	AttributeString AttributeType = "string"
	// AttributeInteger represents integral numeric values.
	AttributeInteger AttributeType = "integer"
	// AttributeDecimal represents numeric values that may have a fractional part.
	AttributeDecimal AttributeType = "decimal"
	// AttributeBoolean represents boolean values.
	AttributeBoolean AttributeType = "boolean"
	// AttributeDate represents date or time values.
	AttributeDate AttributeType = "date"
	// AttributeEnum represents a value drawn from a declared set.
	AttributeEnum AttributeType = "enum"
)

// Operator identifies a filter comparison operation.
type Operator string

const (
	// OperatorEqual matches equal values.
	OperatorEqual Operator = "eq"
	// OperatorNotEqual matches non-equal values.
	OperatorNotEqual Operator = "neq"
	// OperatorIn matches any value in a supplied set.
	OperatorIn Operator = "in"
	// OperatorNotIn excludes values in a supplied set.
	OperatorNotIn Operator = "not_in"
	// OperatorGreaterThan matches values greater than the operand.
	OperatorGreaterThan Operator = "gt"
	// OperatorGreaterOrEq matches values greater than or equal to the operand.
	OperatorGreaterOrEq Operator = "gte"
	// OperatorLessThan matches values less than the operand.
	OperatorLessThan Operator = "lt"
	// OperatorLessOrEq matches values less than or equal to the operand.
	OperatorLessOrEq Operator = "lte"
	// OperatorBetween matches values within an inclusive range.
	OperatorBetween Operator = "between"
	// OperatorContains performs a case-insensitive substring match.
	OperatorContains Operator = "contains"
	// OperatorExists tests whether the attribute is present.
	OperatorExists Operator = "exists"
)

// SortDirection specifies ascending or descending result order.
type SortDirection string

const (
	// SortAscending orders smaller values first.
	SortAscending SortDirection = "asc"
	// SortDescending orders larger values first.
	SortDescending SortDirection = "desc"
)

// FacetMode controls whether a facet excludes its own filters.
type FacetMode string

const (
	// FacetDisjunctive excludes filters on the faceted field when counting buckets.
	FacetDisjunctive FacetMode = "disjunctive"
	// FacetConjunctive applies all filters when counting buckets.
	FacetConjunctive FacetMode = "conjunctive"
)

// FacetSort selects how facet buckets are ordered.
type FacetSort string

const (
	// FacetSortCount orders buckets by descending document count.
	FacetSortCount FacetSort = "count"
	// FacetSortValue orders buckets by value.
	FacetSortValue FacetSort = "value"
)

// AttributeDefinition declares validation and search capabilities for one attribute.
type AttributeDefinition struct {
	Key        string        `json:"key"`
	Type       AttributeType `json:"type"`
	MultiValue bool          `json:"multi_value,omitempty"`
	Filterable bool          `json:"filterable,omitempty"`
	Facetable  bool          `json:"facetable,omitempty"`
	Searchable bool          `json:"searchable,omitempty"`
	Sortable   bool          `json:"sortable,omitempty"`
	EnumValues []string      `json:"enum_values,omitempty"`
}

// Schema defines the accepted attributes for a search resource.
type Schema struct {
	Attributes   map[string]AttributeDefinition `json:"attributes"`
	AllowUnknown bool                           `json:"allow_unknown,omitempty"`
}

// Filter constrains results by comparing an attribute with a value.
type Filter struct {
	Field     string   `json:"field"`
	Operator  Operator `json:"operator"`
	Value     any      `json:"value,omitempty"`
	mandatory bool
}

// FacetRequest asks the backend to aggregate values for an attribute.
type FacetRequest struct {
	Field          string    `json:"field"`
	Mode           FacetMode `json:"mode,omitempty"`
	Limit          int       `json:"limit,omitempty"`
	Sort           FacetSort `json:"sort,omitempty"`
	IncludeMissing bool      `json:"include_missing,omitempty"`
}

// Sort defines one ordered result field.
type Sort struct {
	Field     string        `json:"field"`
	Direction SortDirection `json:"direction,omitempty"`
}

// SearchRequest contains a validated keyword, filter, facet, sort, and page request.
type SearchRequest struct {
	Query   string         `json:"query,omitempty"`
	Filters []Filter       `json:"filters,omitempty"`
	Facets  []FacetRequest `json:"facets,omitempty"`
	Sort    []Sort         `json:"sort,omitempty"`
	Cursor  string         `json:"cursor,omitempty"`
	Limit   int            `json:"limit,omitempty"`
}

// Document is a resource record stored by a search backend.
type Document struct {
	ID         string         `json:"id"`
	Attributes map[string]any `json:"attributes"`
}

// Hit is a matched search document and its relevance score.
type Hit struct {
	ID         string         `json:"id"`
	Score      float64        `json:"score,omitempty"`
	Attributes map[string]any `json:"attributes"`
}

// FacetBucket contains one aggregated value and document count.
type FacetBucket struct {
	Value any   `json:"value"`
	Count int64 `json:"count"`
}

// FacetResult contains the buckets computed for one attribute.
type FacetResult struct {
	Field   string        `json:"field"`
	Buckets []FacetBucket `json:"buckets"`
}

// SearchResult is the backend-independent response returned for a search request.
type SearchResult struct {
	Items      []Hit                  `json:"items"`
	Total      int64                  `json:"total"`
	Facets     map[string]FacetResult `json:"facets,omitempty"`
	NextCursor string                 `json:"next_cursor,omitempty"`
	Backend    string                 `json:"backend"`
}

var attributeKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,127}$`)

// NewSchema validates definitions and creates a schema keyed by attribute name.
func NewSchema(definitions ...AttributeDefinition) (Schema, error) {
	schema := Schema{Attributes: make(map[string]AttributeDefinition, len(definitions))}
	for _, definition := range definitions {
		if err := validateDefinition(definition); err != nil {
			return Schema{}, err
		}
		if _, exists := schema.Attributes[definition.Key]; exists {
			return Schema{}, fmt.Errorf("search: duplicate attribute %q", definition.Key)
		}
		schema.Attributes[definition.Key] = definition
	}
	return schema, nil
}

func validateDefinition(definition AttributeDefinition) error {
	if !attributeKeyPattern.MatchString(definition.Key) {
		return fmt.Errorf("search: invalid attribute key %q", definition.Key)
	}
	if !validAttributeType(definition.Type) {
		return fmt.Errorf("search: unsupported type %q for attribute %q", definition.Type, definition.Key)
	}
	if definition.Type == AttributeEnum && len(definition.EnumValues) == 0 {
		return fmt.Errorf("search: enum attribute %q requires enum values", definition.Key)
	}
	if definition.Searchable && definition.Type != AttributeString && definition.Type != AttributeEnum {
		return fmt.Errorf("search: attribute %q must be a string or enum to be searchable", definition.Key)
	}
	if definition.MultiValue && definition.Sortable {
		return fmt.Errorf("search: multi-value attribute %q cannot be sortable", definition.Key)
	}
	return nil
}

func validAttributeType(attributeType AttributeType) bool {
	switch attributeType {
	case AttributeString, AttributeInteger, AttributeDecimal, AttributeBoolean, AttributeDate, AttributeEnum:
		return true
	default:
		return false
	}
}

// ValidateDocument verifies an ID and every supplied attribute against the schema.
func (s Schema) ValidateDocument(document Document) error {
	if strings.TrimSpace(document.ID) == "" {
		return errors.New("search: document ID is required")
	}
	for key, value := range document.Attributes {
		definition, exists := s.Attributes[key]
		if !exists {
			if s.AllowUnknown {
				continue
			}
			return fmt.Errorf("search: unknown attribute %q", key)
		}
		if err := validateAttributeValue(definition, value); err != nil {
			return err
		}
	}
	return nil
}

func validateAttributeValue(definition AttributeDefinition, value any) error {
	if value == nil {
		return nil
	}
	if definition.MultiValue {
		values, ok := sliceValues(value)
		if !ok {
			return fmt.Errorf("search: attribute %q must be an array", definition.Key)
		}
		for _, item := range values {
			if err := validateScalar(definition, item); err != nil {
				return err
			}
		}
		return nil
	}
	if _, ok := sliceValues(value); ok {
		return fmt.Errorf("search: attribute %q does not accept an array", definition.Key)
	}
	return validateScalar(definition, value)
}

func validateScalar(definition AttributeDefinition, value any) error {
	valid := false
	switch definition.Type {
	case AttributeString:
		_, valid = value.(string)
	case AttributeInteger:
		valid = isInteger(value)
	case AttributeDecimal:
		_, valid = numberValue(value)
	case AttributeBoolean:
		_, valid = value.(bool)
	case AttributeDate:
		valid = isDate(value)
	case AttributeEnum:
		text, ok := value.(string)
		valid = ok && containsString(definition.EnumValues, text)
	}
	if !valid {
		return fmt.Errorf("search: invalid %s value for attribute %q", definition.Type, definition.Key)
	}
	return nil
}

// NormalizeRequest validates a request and applies documented default limits and modes.
func (s Schema) NormalizeRequest(request SearchRequest) (SearchRequest, error) {
	request.Filters = append([]Filter(nil), request.Filters...)
	request.Facets = append([]FacetRequest(nil), request.Facets...)
	request.Sort = append([]Sort(nil), request.Sort...)
	request.Query = strings.TrimSpace(request.Query)
	if len(request.Query) > MaxQueryLength {
		return SearchRequest{}, fmt.Errorf("search: query exceeds %d characters", MaxQueryLength)
	}
	if request.Limit == 0 {
		request.Limit = DefaultLimit
	}
	if request.Limit < 1 || request.Limit > MaxLimit {
		return SearchRequest{}, fmt.Errorf("search: limit must be between 1 and %d", MaxLimit)
	}
	if len(request.Filters) > MaxFilters || len(request.Facets) > MaxFacets {
		return SearchRequest{}, errors.New("search: request complexity limit exceeded")
	}
	if _, err := decodeCursor(request.Cursor); err != nil {
		return SearchRequest{}, err
	}
	if err := s.validateFilters(request.Filters); err != nil {
		return SearchRequest{}, err
	}
	if err := s.normalizeFacets(request.Facets); err != nil {
		return SearchRequest{}, err
	}
	if err := s.normalizeSort(request.Sort); err != nil {
		return SearchRequest{}, err
	}
	return request, nil
}

func (s Schema) validateFilters(filters []Filter) error {
	for _, filter := range filters {
		definition, exists := s.Attributes[filter.Field]
		if !exists || !definition.Filterable {
			return fmt.Errorf("search: attribute %q is not filterable", filter.Field)
		}
		if err := validateFilter(definition, filter); err != nil {
			return err
		}
	}
	return nil
}

func validateFilter(definition AttributeDefinition, filter Filter) error {
	if !validOperator(filter.Operator) {
		return fmt.Errorf("search: unsupported operator %q", filter.Operator)
	}
	if definition.MultiValue && isRangeOperator(filter.Operator) {
		return fmt.Errorf("search: range operators are not supported for multi-value attribute %q", filter.Field)
	}
	if isRangeOperator(filter.Operator) && (definition.Type == AttributeBoolean || definition.Type == AttributeEnum) {
		return fmt.Errorf("search: operator %q is invalid for attribute %q", filter.Operator, filter.Field)
	}
	if filter.Operator == OperatorContains && definition.Type != AttributeString {
		return fmt.Errorf("search: contains is only valid for string attributes")
	}
	if filter.Operator == OperatorExists {
		if filter.Value != nil {
			if _, ok := filter.Value.(bool); !ok {
				return fmt.Errorf("search: exists value for %q must be boolean", filter.Field)
			}
		}
		return nil
	}
	if filter.Operator == OperatorIn || filter.Operator == OperatorNotIn || filter.Operator == OperatorBetween {
		values, ok := sliceValues(filter.Value)
		if !ok || len(values) == 0 {
			return fmt.Errorf("search: operator %q requires a non-empty array", filter.Operator)
		}
		if filter.Operator == OperatorBetween && len(values) != 2 {
			return errors.New("search: between requires exactly two values")
		}
		for _, value := range values {
			if err := validateScalar(definition, value); err != nil {
				return err
			}
		}
		return nil
	}
	return validateScalar(definition, filter.Value)
}

func validOperator(operator Operator) bool {
	switch operator {
	case OperatorEqual, OperatorNotEqual, OperatorIn, OperatorNotIn, OperatorGreaterThan,
		OperatorGreaterOrEq, OperatorLessThan, OperatorLessOrEq, OperatorBetween,
		OperatorContains, OperatorExists:
		return true
	default:
		return false
	}
}

func isRangeOperator(operator Operator) bool {
	switch operator {
	case OperatorGreaterThan, OperatorGreaterOrEq, OperatorLessThan, OperatorLessOrEq, OperatorBetween:
		return true
	default:
		return false
	}
}

func (s Schema) normalizeFacets(facets []FacetRequest) error {
	seen := make(map[string]struct{}, len(facets))
	for index := range facets {
		facet := &facets[index]
		definition, exists := s.Attributes[facet.Field]
		if !exists || !definition.Facetable {
			return fmt.Errorf("search: attribute %q is not facetable", facet.Field)
		}
		if _, exists := seen[facet.Field]; exists {
			return fmt.Errorf("search: duplicate facet %q", facet.Field)
		}
		seen[facet.Field] = struct{}{}
		if facet.Mode == "" {
			facet.Mode = FacetDisjunctive
		}
		if facet.Mode != FacetDisjunctive && facet.Mode != FacetConjunctive {
			return fmt.Errorf("search: invalid facet mode %q", facet.Mode)
		}
		if facet.Limit == 0 {
			facet.Limit = DefaultFacetLimit
		}
		if facet.Limit < 1 || facet.Limit > MaxFacetBuckets {
			return fmt.Errorf("search: facet limit must be between 1 and %d", MaxFacetBuckets)
		}
		if facet.Sort == "" {
			facet.Sort = FacetSortCount
		}
		if facet.Sort != FacetSortCount && facet.Sort != FacetSortValue {
			return fmt.Errorf("search: invalid facet sort %q", facet.Sort)
		}
	}
	return nil
}

func (s Schema) normalizeSort(sorts []Sort) error {
	for index := range sorts {
		sortField := &sorts[index]
		definition, exists := s.Attributes[sortField.Field]
		if !exists || !definition.Sortable {
			return fmt.Errorf("search: attribute %q is not sortable", sortField.Field)
		}
		if sortField.Direction == "" {
			sortField.Direction = SortAscending
		}
		if sortField.Direction != SortAscending && sortField.Direction != SortDescending {
			return fmt.Errorf("search: invalid sort direction %q", sortField.Direction)
		}
	}
	return nil
}

func sliceValues(value any) ([]any, bool) {
	if value == nil {
		return nil, false
	}
	reflected := reflect.ValueOf(value)
	if reflected.Kind() != reflect.Slice && reflected.Kind() != reflect.Array {
		return nil, false
	}
	values := make([]any, reflected.Len())
	for index := range values {
		values[index] = reflected.Index(index).Interface()
	}
	return values, true
}

func isInteger(value any) bool {
	switch number := value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	case float64:
		return number == float64(int64(number))
	case json.Number:
		_, err := number.Int64()
		return err == nil
	default:
		return false
	}
}

func numberValue(value any) (float64, bool) {
	var result float64
	var valid bool
	switch number := value.(type) {
	case int:
		result, valid = float64(number), true
	case int8:
		result, valid = float64(number), true
	case int16:
		result, valid = float64(number), true
	case int32:
		result, valid = float64(number), true
	case int64:
		result, valid = float64(number), true
	case uint:
		result, valid = float64(number), true
	case uint8:
		result, valid = float64(number), true
	case uint16:
		result, valid = float64(number), true
	case uint32:
		result, valid = float64(number), true
	case uint64:
		result, valid = float64(number), true
	case float32:
		result, valid = float64(number), true
	case float64:
		result, valid = number, true
	case json.Number:
		parsed, err := number.Float64()
		result, valid = parsed, err == nil
	default:
		return 0, false
	}
	return result, valid && !math.IsNaN(result) && !math.IsInf(result, 0)
}

func isDate(value any) bool {
	switch date := value.(type) {
	case time.Time:
		return true
	case string:
		_, err := parseDate(date)
		return err == nil
	default:
		return false
	}
}

func parseDate(value string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	return time.Parse(time.DateOnly, value)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func encodeCursor(offset int) string {
	if offset <= 0 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte("v1:" + strconv.Itoa(offset)))
}

func decodeCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil || !strings.HasPrefix(string(decoded), "v1:") {
		return 0, errors.New("search: invalid cursor")
	}
	offset, err := strconv.Atoi(strings.TrimPrefix(string(decoded), "v1:"))
	if err != nil || offset < 0 {
		return 0, errors.New("search: invalid cursor")
	}
	return offset, nil
}
