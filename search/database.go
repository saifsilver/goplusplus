package search

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type SQLDatabase interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

type DatabaseConfig struct {
	Table string
}

type DatabaseBackend struct {
	db    SQLDatabase
	table string
}

type SQLQuery struct {
	Statement string
	Args      []any
}

type DatabaseQueryPlan struct {
	Count  SQLQuery
	Hits   SQLQuery
	Facets map[string]SQLQuery
}

var sqlIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,47}$`)

func NewDatabaseBackend(db SQLDatabase, config DatabaseConfig) (*DatabaseBackend, error) {
	if db == nil {
		return nil, fmt.Errorf("search: database is required")
	}
	if config.Table == "" {
		config.Table = "gpp_search_documents"
	}
	if !sqlIdentifierPattern.MatchString(config.Table) {
		return nil, fmt.Errorf("search: invalid database table %q", config.Table)
	}
	return &DatabaseBackend{db: db, table: config.Table}, nil
}

func (b *DatabaseBackend) Name() string {
	return "database"
}

func (b *DatabaseBackend) Setup(ctx context.Context) error {
	statements := []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			resource TEXT NOT NULL,
			id TEXT NOT NULL,
			document JSONB NOT NULL,
			search_text TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (resource, id)
		)`, b.table),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s_document_gin ON %s USING GIN (document)", b.table, b.table),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s_search_gin ON %s USING GIN (to_tsvector('simple', search_text))", b.table, b.table),
	}
	for _, statement := range statements {
		if _, err := b.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("search: initialize database backend: %w", err)
		}
	}
	return nil
}

func (b *DatabaseBackend) Index(ctx context.Context, resource string, schema Schema, document Document) error {
	payload, err := json.Marshal(document.Attributes)
	if err != nil {
		return fmt.Errorf("search: encode document %q: %w", document.ID, err)
	}
	statement := fmt.Sprintf(`INSERT INTO %s (resource, id, document, search_text, updated_at)
		VALUES ($1, $2, $3::jsonb, $4, CURRENT_TIMESTAMP)
		ON CONFLICT (resource, id) DO UPDATE SET
			document = EXCLUDED.document,
			search_text = EXCLUDED.search_text,
			updated_at = CURRENT_TIMESTAMP`, b.table)
	_, err = b.db.ExecContext(ctx, statement, resource, document.ID, string(payload), searchableText(schema, document))
	if err != nil {
		return fmt.Errorf("search: index document %q: %w", document.ID, err)
	}
	return nil
}

func (b *DatabaseBackend) Delete(ctx context.Context, resource, documentID string) error {
	statement := fmt.Sprintf("DELETE FROM %s WHERE resource = $1 AND id = $2", b.table)
	if _, err := b.db.ExecContext(ctx, statement, resource, documentID); err != nil {
		return fmt.Errorf("search: delete document %q: %w", documentID, err)
	}
	return nil
}

func searchableText(schema Schema, document Document) string {
	var values []string
	for key, definition := range schema.Attributes {
		if !definition.Searchable {
			continue
		}
		for _, value := range attributeValues(document.Attributes[key]) {
			values = append(values, fmt.Sprint(value))
		}
	}
	return strings.Join(values, " ")
}

func (b *DatabaseBackend) Search(ctx context.Context, resource string, schema Schema, request SearchRequest) (SearchResult, error) {
	normalized, err := schema.NormalizeRequest(request)
	if err != nil {
		return SearchResult{}, err
	}
	request = normalized
	plan, err := b.Compile(resource, schema, request)
	if err != nil {
		return SearchResult{}, err
	}
	total, err := b.queryCount(ctx, plan.Count)
	if err != nil {
		return SearchResult{}, err
	}
	items, err := b.queryHits(ctx, plan.Hits)
	if err != nil {
		return SearchResult{}, err
	}
	facets, err := b.queryFacets(ctx, schema, request, plan.Facets)
	if err != nil {
		return SearchResult{}, err
	}
	offset, _ := decodeCursor(request.Cursor)
	nextCursor := ""
	if int64(offset+len(items)) < total {
		nextCursor = encodeCursor(offset + len(items))
	}
	return SearchResult{Items: items, Total: total, Facets: facets, NextCursor: nextCursor, Backend: b.Name()}, nil
}

func (b *DatabaseBackend) Compile(resource string, schema Schema, request SearchRequest) (DatabaseQueryPlan, error) {
	normalized, err := schema.NormalizeRequest(request)
	if err != nil {
		return DatabaseQueryPlan{}, err
	}
	count := b.compileCount(resource, schema, normalized)
	hits, err := b.compileHits(resource, schema, normalized)
	if err != nil {
		return DatabaseQueryPlan{}, err
	}
	facets := make(map[string]SQLQuery, len(normalized.Facets))
	for _, facet := range normalized.Facets {
		facets[facet.Field] = b.compileFacet(resource, schema, normalized, facet)
	}
	return DatabaseQueryPlan{Count: count, Hits: hits, Facets: facets}, nil
}

func (b *DatabaseBackend) compileCount(resource string, schema Schema, request SearchRequest) SQLQuery {
	builder := newSQLBuilder(resource)
	where := builder.where(schema, request.Query, request.Filters)
	return SQLQuery{Statement: fmt.Sprintf("SELECT COUNT(*) FROM %s d WHERE %s", b.table, where), Args: builder.args}
}

func (b *DatabaseBackend) compileHits(resource string, schema Schema, request SearchRequest) (SQLQuery, error) {
	builder := newSQLBuilder(resource)
	where := builder.where(schema, request.Query, request.Filters)
	scoreExpression := "0::double precision"
	if request.Query != "" {
		query := builder.bind(request.Query)
		scoreExpression = fmt.Sprintf("ts_rank_cd(to_tsvector('simple', d.search_text), websearch_to_tsquery('simple', %s))", query)
	}
	orderBy := builder.orderBy(schema, request, scoreExpression)
	offset, err := decodeCursor(request.Cursor)
	if err != nil {
		return SQLQuery{}, err
	}
	limitPlaceholder := builder.bind(request.Limit)
	offsetPlaceholder := builder.bind(offset)
	statement := fmt.Sprintf(
		"SELECT d.id, d.document, %s FROM %s d WHERE %s ORDER BY %s LIMIT %s OFFSET %s",
		scoreExpression, b.table, where, orderBy, limitPlaceholder, offsetPlaceholder,
	)
	return SQLQuery{Statement: statement, Args: builder.args}, nil
}

func (b *DatabaseBackend) compileFacet(resource string, schema Schema, request SearchRequest, facet FacetRequest) SQLQuery {
	filters := request.Filters
	if facet.Mode == FacetDisjunctive {
		filters = filtersWithoutField(filters, facet.Field)
	}
	builder := newSQLBuilder(resource)
	where := builder.where(schema, request.Query, filters)
	keyPlaceholder := builder.bind(facet.Field)
	limitPlaceholder := builder.bind(facet.Limit)
	missingCondition := "facet.value IS NOT NULL"
	if facet.IncludeMissing {
		missingCondition = "TRUE"
	}
	orderBy := "COUNT(DISTINCT d.id) DESC, facet.value ASC"
	if facet.Sort == FacetSortValue {
		orderBy = "facet.value ASC"
	}
	statement := fmt.Sprintf(`SELECT facet.value, COUNT(DISTINCT d.id)
		FROM %s d
		LEFT JOIN LATERAL jsonb_array_elements_text(
			CASE
				WHEN NOT (d.document ? %s) OR d.document -> %s = 'null'::jsonb THEN '[]'::jsonb
				WHEN jsonb_typeof(d.document -> %s) = 'array' THEN d.document -> %s
				ELSE jsonb_build_array(d.document -> %s)
			END
		) facet(value) ON TRUE
		WHERE %s AND %s
		GROUP BY facet.value
		ORDER BY %s
		LIMIT %s`, b.table, keyPlaceholder, keyPlaceholder, keyPlaceholder, keyPlaceholder, keyPlaceholder,
		where, missingCondition, orderBy, limitPlaceholder)
	return SQLQuery{Statement: statement, Args: builder.args}
}

type sqlBuilder struct {
	args []any
}

func newSQLBuilder(resource string) *sqlBuilder {
	return &sqlBuilder{args: []any{resource}}
}

func (b *sqlBuilder) bind(value any) string {
	b.args = append(b.args, value)
	return "$" + strconv.Itoa(len(b.args))
}

func (b *sqlBuilder) where(schema Schema, query string, filters []Filter) string {
	conditions := []string{"d.resource = $1"}
	if query != "" {
		placeholder := b.bind(query)
		conditions = append(conditions, fmt.Sprintf("to_tsvector('simple', d.search_text) @@ websearch_to_tsquery('simple', %s)", placeholder))
	}
	for _, filter := range filters {
		conditions = append(conditions, b.filter(schema.Attributes[filter.Field], filter))
	}
	return strings.Join(conditions, " AND ")
}

func (b *sqlBuilder) filter(definition AttributeDefinition, filter Filter) string {
	switch filter.Operator {
	case OperatorExists:
		key := b.bind(filter.Field)
		expected, ok := filter.Value.(bool)
		if !ok {
			expected = true
		}
		if expected {
			return fmt.Sprintf("d.document ? %s", key)
		}
		return fmt.Sprintf("NOT (d.document ? %s)", key)
	case OperatorEqual, OperatorNotEqual:
		condition := b.equality(definition, filter.Field, filter.Value)
		if filter.Operator == OperatorNotEqual {
			return "NOT (" + condition + ")"
		}
		return condition
	case OperatorIn, OperatorNotIn:
		values, _ := sliceValues(filter.Value)
		conditions := make([]string, 0, len(values))
		for _, value := range values {
			conditions = append(conditions, b.equality(definition, filter.Field, value))
		}
		condition := "(" + strings.Join(conditions, " OR ") + ")"
		if filter.Operator == OperatorNotIn {
			return "NOT " + condition
		}
		return condition
	case OperatorContains:
		key := b.bind(filter.Field)
		value := b.bind(fmt.Sprint(filter.Value))
		return fmt.Sprintf("LOWER(d.document ->> %s) LIKE '%%' || LOWER(%s) || '%%'", key, value)
	default:
		key := b.bind(filter.Field)
		return b.rangeFilter(definition, key, filter)
	}
}

func (b *sqlBuilder) equality(definition AttributeDefinition, field string, value any) string {
	containedValue := value
	if definition.MultiValue {
		containedValue = []any{value}
	}
	payload, _ := json.Marshal(map[string]any{field: containedValue})
	return fmt.Sprintf("d.document @> %s::jsonb", b.bind(string(payload)))
}

func (b *sqlBuilder) rangeFilter(definition AttributeDefinition, key string, filter Filter) string {
	expression := fmt.Sprintf("d.document ->> %s", key)
	if definition.Type == AttributeInteger || definition.Type == AttributeDecimal {
		expression = fmt.Sprintf("CASE WHEN jsonb_typeof(d.document -> %s) = 'number' THEN (d.document ->> %s)::numeric END", key, key)
	}
	if definition.Type == AttributeDate {
		expression = fmt.Sprintf("(d.document ->> %s)::timestamptz", key)
	}
	if filter.Operator == OperatorBetween {
		bounds, _ := sliceValues(filter.Value)
		return fmt.Sprintf("%s BETWEEN %s AND %s", expression, b.bind(sqlScalar(bounds[0])), b.bind(sqlScalar(bounds[1])))
	}
	operators := map[Operator]string{
		OperatorGreaterThan: ">", OperatorGreaterOrEq: ">=", OperatorLessThan: "<", OperatorLessOrEq: "<=",
	}
	return fmt.Sprintf("%s %s %s", expression, operators[filter.Operator], b.bind(sqlScalar(filter.Value)))
}

func sqlScalar(value any) any {
	if date, ok := value.(interface{ Format(string) string }); ok {
		return date.Format("2006-01-02T15:04:05Z07:00")
	}
	return value
}

func (b *sqlBuilder) orderBy(schema Schema, request SearchRequest, scoreExpression string) string {
	if len(request.Sort) == 0 {
		if request.Query == "" {
			return "d.id ASC"
		}
		return scoreExpression + " DESC, d.id ASC"
	}
	clauses := make([]string, 0, len(request.Sort)+1)
	for _, sortField := range request.Sort {
		key := b.bind(sortField.Field)
		expression := fmt.Sprintf("d.document ->> %s", key)
		definition := schema.Attributes[sortField.Field]
		if definition.Type == AttributeInteger || definition.Type == AttributeDecimal {
			expression = fmt.Sprintf("CASE WHEN jsonb_typeof(d.document -> %s) = 'number' THEN (d.document ->> %s)::numeric END", key, key)
		}
		if definition.Type == AttributeDate {
			expression = fmt.Sprintf("(d.document ->> %s)::timestamptz", key)
		}
		clauses = append(clauses, fmt.Sprintf("%s %s NULLS LAST", expression, strings.ToUpper(string(sortField.Direction))))
	}
	return strings.Join(append(clauses, "d.id ASC"), ", ")
}

func (b *DatabaseBackend) queryCount(ctx context.Context, query SQLQuery) (int64, error) {
	rows, err := b.db.QueryContext(ctx, query.Statement, query.Args...)
	if err != nil {
		return 0, fmt.Errorf("search: count query: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, rows.Err()
	}
	var total int64
	if err := rows.Scan(&total); err != nil {
		return 0, fmt.Errorf("search: scan count: %w", err)
	}
	return total, rows.Err()
}

func (b *DatabaseBackend) queryHits(ctx context.Context, query SQLQuery) ([]Hit, error) {
	rows, err := b.db.QueryContext(ctx, query.Statement, query.Args...)
	if err != nil {
		return nil, fmt.Errorf("search: hits query: %w", err)
	}
	defer rows.Close()
	items := make([]Hit, 0)
	for rows.Next() {
		var id string
		var payload []byte
		var score float64
		if err := rows.Scan(&id, &payload, &score); err != nil {
			return nil, fmt.Errorf("search: scan hit: %w", err)
		}
		attributes := make(map[string]any)
		if err := json.Unmarshal(payload, &attributes); err != nil {
			return nil, fmt.Errorf("search: decode hit %q: %w", id, err)
		}
		items = append(items, Hit{ID: id, Score: score, Attributes: attributes})
	}
	return items, rows.Err()
}

func (b *DatabaseBackend) queryFacets(ctx context.Context, schema Schema, request SearchRequest, queries map[string]SQLQuery) (map[string]FacetResult, error) {
	if len(queries) == 0 {
		return nil, nil
	}
	results := make(map[string]FacetResult, len(queries))
	for _, facet := range request.Facets {
		result, err := b.queryFacet(ctx, schema.Attributes[facet.Field], facet, queries[facet.Field])
		if err != nil {
			return nil, err
		}
		results[facet.Field] = result
	}
	return results, nil
}

func (b *DatabaseBackend) queryFacet(ctx context.Context, definition AttributeDefinition, request FacetRequest, query SQLQuery) (FacetResult, error) {
	rows, err := b.db.QueryContext(ctx, query.Statement, query.Args...)
	if err != nil {
		return FacetResult{}, fmt.Errorf("search: facet %q query: %w", request.Field, err)
	}
	defer rows.Close()
	buckets := make([]FacetBucket, 0)
	for rows.Next() {
		var raw sql.NullString
		var count int64
		if err := rows.Scan(&raw, &count); err != nil {
			return FacetResult{}, fmt.Errorf("search: scan facet %q: %w", request.Field, err)
		}
		var value any
		if raw.Valid {
			value = parseFacetValue(definition.Type, raw.String)
		}
		buckets = append(buckets, FacetBucket{Value: value, Count: count})
	}
	return FacetResult{Field: request.Field, Buckets: buckets}, rows.Err()
}

func parseFacetValue(attributeType AttributeType, value string) any {
	switch attributeType {
	case AttributeInteger:
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			return parsed
		}
	case AttributeDecimal:
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed
		}
	case AttributeBoolean:
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return value
}
