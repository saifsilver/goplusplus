package dbcore

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"time"
)

type queryNameKey struct{}

// WithQueryName attaches a readable application query name to the context for dashboards and slow query logs.
func WithQueryName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, queryNameKey{}, name)
}

// GetQueryName extracts the query name attached to the context, or returns "unnamed".
func GetQueryName(ctx context.Context) string {
	if name, ok := ctx.Value(queryNameKey{}).(string); ok && name != "" {
		return name
	}
	return "unnamed"
}

// SlowQueryConfig configures slow query thresholds, logging, and rules advisor settings.
type SlowQueryConfig struct {
	Threshold    time.Duration
	MaxSQLLength int
	OmitSQL      bool
}

// DefaultSlowQueryConfig returns standard slow query logging parameters.
func DefaultSlowQueryConfig() SlowQueryConfig {
	return SlowQueryConfig{
		Threshold:    250 * time.Millisecond,
		MaxSQLLength: 1500,
		OmitSQL:      false,
	}
}

// SlowQueryEvent represents a structured log event generated when a query exceeds the configured duration threshold.
type SlowQueryEvent struct {
	Role        string   `json:"role"`
	Operation   string   `json:"operation"`
	QueryName   string   `json:"query_name"`
	DurationMS  int64    `json:"duration_ms"`
	ThresholdMS int64    `json:"threshold_ms"`
	Fingerprint string   `json:"fingerprint"`
	ArgsCount   int      `json:"args_count"`
	SQL         string   `json:"sql,omitempty"`
	Suggestions []string `json:"suggestions"`
	Error       string   `json:"error,omitempty"`
}

var (
	literalRegex  = regexp.MustCompile(`(?i)'[^']*'|\b\d+\b`)
	paramNumRegex = regexp.MustCompile(`\$\d+`)
)

// GenerateFingerprint sanitizes SQL by replacing literal values and parameters with placeholder tokens.
func GenerateFingerprint(sql string) string {
	cleaned := strings.Join(strings.Fields(sql), " ")
	cleaned = paramNumRegex.ReplaceAllString(cleaned, "$?")
	cleaned = literalRegex.ReplaceAllString(cleaned, "?")
	return strings.ToLower(cleaned)
}

// AnalyzeQuery inspects SQL text and produces actionable performance suggestions.
func AnalyzeQuery(sql string) []string {
	var suggestions []string
	upperSQL := strings.ToUpper(sql)

	if strings.Contains(upperSQL, "SELECT *") {
		suggestions = append(suggestions, "Avoid 'SELECT *'; fetch only explicitly required columns to reduce I/O and memory overhead.")
	}
	if strings.Contains(upperSQL, "SELECT ") && !strings.Contains(upperSQL, "WHERE") && !strings.Contains(upperSQL, "LIMIT") {
		suggestions = append(suggestions, "Query lacks WHERE and LIMIT clauses; consider adding a selective filter or row limit.")
	}
	if strings.Contains(upperSQL, "ORDER BY") && !strings.Contains(upperSQL, "LIMIT") {
		suggestions = append(suggestions, "ORDER BY without LIMIT may cause expensive full table sorts; add a LIMIT or ensure an index covers the sort.")
	}
	if strings.Contains(upperSQL, "OFFSET") {
		suggestions = append(suggestions, "OFFSET pagination degrades performance on large datasets; consider keyset (cursor-based) pagination.")
	}
	if strings.Contains(upperSQL, "LIKE '%") || strings.Contains(upperSQL, "ILIKE '%") {
		suggestions = append(suggestions, "Leading wildcard LIKE '%...' cannot use B-tree indexes; consider trigram GIN indexes or full-text search (tsvector).")
	}
	if strings.Contains(upperSQL, "LOWER(") || strings.Contains(upperSQL, "UPPER(") || strings.Contains(upperSQL, "DATE(") {
		suggestions = append(suggestions, "Function wrapped around column in WHERE clause prevents standard index usage; use an expression index or normalized column.")
	}
	if strings.Contains(upperSQL, "COUNT(*)") {
		suggestions = append(suggestions, "Large COUNT(*) triggers full table scans; consider cached counter tables or pg_class estimation.")
	}

	if len(suggestions) == 0 {
		suggestions = append(suggestions, "Run EXPLAIN (ANALYZE, BUFFERS) in staging to inspect query execution plan and index coverage.")
	}

	return suggestions
}

// LogSlowQuery logs a slow query event to slog without logging bind arguments (PII protection).
func LogSlowQuery(ctx context.Context, cfg SlowQueryConfig, role, operation, sql string, duration time.Duration, argsCount int, err error) {
	if cfg.Threshold <= 0 {
		cfg.Threshold = 250 * time.Millisecond
	}

	if duration < cfg.Threshold {
		return
	}

	queryName := GetQueryName(ctx)
	fingerprint := GenerateFingerprint(sql)
	suggestions := AnalyzeQuery(sql)

	event := SlowQueryEvent{
		Role:        role,
		Operation:   operation,
		QueryName:   queryName,
		DurationMS:  duration.Milliseconds(),
		ThresholdMS: cfg.Threshold.Milliseconds(),
		Fingerprint: fingerprint,
		ArgsCount:   argsCount,
		Suggestions: suggestions,
	}

	if !cfg.OmitSQL {
		sqlText := sql
		if cfg.MaxSQLLength > 0 && len(sqlText) > cfg.MaxSQLLength {
			sqlText = sqlText[:cfg.MaxSQLLength] + "... [TRUNCATED]"
		}
		event.SQL = sqlText
	}

	if err != nil {
		event.Error = err.Error()
	}

	logAttrs := []any{
		slog.String("role", event.Role),
		slog.String("operation", event.Operation),
		slog.String("query_name", event.QueryName),
		slog.Int64("duration_ms", event.DurationMS),
		slog.Int64("threshold_ms", event.ThresholdMS),
		slog.Int("args_count", event.ArgsCount),
		slog.String("fingerprint", event.Fingerprint),
		slog.Any("suggestions", event.Suggestions),
	}

	if event.SQL != "" {
		logAttrs = append(logAttrs, slog.String("sql", event.SQL))
	}
	if event.Error != "" {
		logAttrs = append(logAttrs, slog.String("error", event.Error))
	}

	slog.Warn("dbcore: Slow Database Query Detected", logAttrs...)
}
