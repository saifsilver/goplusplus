package dbcore

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type driverResult struct {
	lastInsertID int64
}

func (r driverResult) LastInsertId() (int64, error) { return r.lastInsertID, nil }
func (driverResult) RowsAffected() (int64, error)   { return 1, nil }

// Zero-dependency in-memory database driver implementation
type inMemStore struct {
	mu     sync.RWMutex
	tables map[string][]map[string]any
	nextID map[string]int64
}

var globalInMemStore = &inMemStore{
	tables: make(map[string][]map[string]any),
	nextID: make(map[string]int64),
}

type inMemDriver struct{}

func (d *inMemDriver) Open(name string) (driver.Conn, error) {
	return &inMemConn{}, nil
}

type inMemConn struct{}

func (c *inMemConn) Prepare(query string) (driver.Stmt, error) {
	return &inMemStmt{query: query}, nil
}
func (c *inMemConn) Close() error              { return nil }
func (c *inMemConn) Begin() (driver.Tx, error) { return &inMemTx{}, nil }

type inMemTx struct{}

func (tx *inMemTx) Commit() error   { return nil }
func (tx *inMemTx) Rollback() error { return nil }

type inMemStmt struct {
	query string
}

func (s *inMemStmt) Close() error  { return nil }
func (s *inMemStmt) NumInput() int { return -1 }

func (s *inMemStmt) Exec(args []driver.Value) (driver.Result, error) {
	q := strings.TrimSpace(s.query)
	qUpper := strings.ToUpper(q)

	globalInMemStore.mu.Lock()
	defer globalInMemStore.mu.Unlock()

	if strings.HasPrefix(qUpper, "CREATE TABLE") {
		tName := createTableName(q)
		if tName != "" && globalInMemStore.tables[tName] == nil {
			globalInMemStore.tables[tName] = make([]map[string]any, 0)
		}
	} else if strings.HasPrefix(qUpper, "INSERT INTO") {
		tName, columns := insertTarget(q)
		values := args
		if len(values) == 0 {
			values = insertLiteralValues(q)
		}
		record := make(map[string]any, len(columns)+1)
		for index, column := range columns {
			if index < len(values) {
				record[column] = values[index]
			}
		}
		if explicitID, exists := record["id"]; exists {
			if numericID, ok := numericDriverValue(explicitID); ok && int64(numericID) > globalInMemStore.nextID[tName] {
				globalInMemStore.nextID[tName] = int64(numericID)
			}
		} else {
			globalInMemStore.nextID[tName]++
			record["id"] = globalInMemStore.nextID[tName]
		}
		globalInMemStore.tables[tName] = append(globalInMemStore.tables[tName], record)
		return driverResult{lastInsertID: globalInMemStore.nextID[tName]}, nil
	} else if strings.HasPrefix(qUpper, "UPDATE") {
		updateInMemRecord(q, args)
	} else if strings.HasPrefix(qUpper, "DELETE FROM") {
		deleteInMemRecord(q, args)
	}
	return driverResult{lastInsertID: 1}, nil
}

func (s *inMemStmt) Query(args []driver.Value) (driver.Rows, error) {
	q := strings.TrimSpace(s.query)
	qUpper := strings.ToUpper(q)

	globalInMemStore.mu.RLock()
	defer globalInMemStore.mu.RUnlock()

	tableName := selectTableName(q)
	records := filterInMemRecords(globalInMemStore.tables[tableName], q, args)
	if strings.Contains(qUpper, "COUNT(*)") {
		return &inMemRows{
			columns: []string{"count"},
			rows:    [][]driver.Value{{int64(len(records))}},
		}, nil
	}
	columns := selectedColumns(q, records)
	records = paginateInMemRecords(records, q, args)
	rows := make([][]driver.Value, 0, len(records))
	for _, record := range records {
		row := make([]driver.Value, len(columns))
		for index, column := range columns {
			row[index] = record[column]
		}
		rows = append(rows, row)
	}
	return &inMemRows{columns: columns, rows: rows}, nil
}

func createTableName(query string) string {
	fields := strings.Fields(query)
	for index := 2; index < len(fields); index++ {
		word := strings.ToUpper(fields[index])
		if word != "IF" && word != "NOT" && word != "EXISTS" {
			return cleanSQLIdentifier(fields[index])
		}
	}
	return ""
}

func insertTarget(query string) (string, []string) {
	fields := strings.Fields(query)
	if len(fields) < 3 {
		return "", nil
	}
	tableName := cleanSQLIdentifier(fields[2])
	start := strings.Index(query, "(")
	end := strings.Index(query[start+1:], ")")
	if start < 0 || end < 0 {
		return tableName, nil
	}
	return tableName, splitSQLColumns(query[start+1 : start+1+end])
}

func insertLiteralValues(query string) []driver.Value {
	upper := strings.ToUpper(query)
	valuesIndex := strings.Index(upper, " VALUES ")
	if valuesIndex < 0 {
		return nil
	}
	fragment := strings.TrimSpace(query[valuesIndex+8:])
	fragment = strings.TrimSuffix(strings.TrimPrefix(fragment, "("), ");")
	fragment = strings.TrimSuffix(fragment, ")")
	parts := strings.Split(fragment, ",")
	values := make([]driver.Value, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
			values = append(values, strings.ReplaceAll(value[1:len(value)-1], "''", "'"))
			continue
		}
		if integer, err := strconv.ParseInt(value, 10, 64); err == nil {
			values = append(values, integer)
			continue
		}
		if decimal, err := strconv.ParseFloat(value, 64); err == nil {
			values = append(values, decimal)
			continue
		}
		values = append(values, nil)
	}
	return values
}

func updateInMemRecord(query string, args []driver.Value) {
	fields := strings.Fields(query)
	if len(fields) < 2 || len(args) == 0 {
		return
	}
	tableName := cleanSQLIdentifier(fields[1])
	setStart := strings.Index(strings.ToUpper(query), " SET ")
	whereStart := strings.Index(strings.ToUpper(query), " WHERE ")
	if setStart < 0 || whereStart < 0 {
		return
	}
	assignments := strings.Split(query[setStart+5:whereStart], ",")
	matchID := args[len(args)-1]
	for _, record := range globalInMemStore.tables[tableName] {
		if !valuesEqual(record["id"], matchID) {
			continue
		}
		for index, assignment := range assignments {
			column := cleanSQLIdentifier(strings.TrimSpace(strings.SplitN(assignment, "=", 2)[0]))
			if index < len(args)-1 {
				record[column] = args[index]
			}
		}
	}
}

func deleteInMemRecord(query string, args []driver.Value) {
	fields := strings.Fields(query)
	if len(fields) < 3 {
		return
	}
	tableName := cleanSQLIdentifier(fields[2])
	if len(args) == 0 {
		globalInMemStore.tables[tableName] = nil
		return
	}
	kept := globalInMemStore.tables[tableName][:0]
	for _, record := range globalInMemStore.tables[tableName] {
		if !valuesEqual(record["id"], args[0]) {
			kept = append(kept, record)
		}
	}
	globalInMemStore.tables[tableName] = kept
}

func selectTableName(query string) string {
	fields := strings.Fields(query)
	for index := len(fields) - 2; index >= 0; index-- {
		if strings.EqualFold(strings.Trim(fields[index], "("), "FROM") {
			return cleanSQLIdentifier(fields[index+1])
		}
	}
	return ""
}

func selectedColumns(query string, records []map[string]any) []string {
	upper := strings.ToUpper(query)
	from := strings.Index(upper, " FROM ")
	if from < 0 {
		return nil
	}
	selection := strings.TrimSpace(query[len("SELECT "):from])
	if selection != "*" {
		return splitSQLColumns(selection)
	}
	columnSet := make(map[string]struct{})
	for _, record := range records {
		for column := range record {
			columnSet[column] = struct{}{}
		}
	}
	columns := make([]string, 0, len(columnSet))
	for column := range columnSet {
		columns = append(columns, column)
	}
	sort.Strings(columns)
	return columns
}

func splitSQLColumns(columns string) []string {
	parts := strings.Split(columns, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		result = append(result, cleanSQLIdentifier(part))
	}
	return result
}

func cleanSQLIdentifier(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.Trim(value, "`\"();")))
}

func filterInMemRecords(records []map[string]any, query string, args []driver.Value) []map[string]any {
	upper := strings.ToUpper(query)
	whereStart := strings.Index(upper, " WHERE ")
	if whereStart < 0 || len(args) == 0 {
		return append([]map[string]any(nil), records...)
	}
	condition := query[whereStart+7:]
	for _, terminator := range []string{" ORDER BY ", " LIMIT ", " OFFSET ", ") AS "} {
		if index := strings.Index(strings.ToUpper(condition), terminator); index >= 0 {
			condition = condition[:index]
		}
	}
	parts := strings.Fields(condition)
	if len(parts) < 3 {
		return append([]map[string]any(nil), records...)
	}
	column, operator := cleanSQLIdentifier(parts[0]), parts[1]
	result := make([]map[string]any, 0, len(records))
	for _, record := range records {
		if compareInMemValue(record[column], args[0], operator) {
			result = append(result, record)
		}
	}
	return result
}

func compareInMemValue(left, right any, operator string) bool {
	if operator == "=" {
		return valuesEqual(left, right)
	}
	leftNumber, leftOK := numericDriverValue(left)
	rightNumber, rightOK := numericDriverValue(right)
	if !leftOK || !rightOK {
		return false
	}
	switch operator {
	case ">":
		return leftNumber > rightNumber
	case ">=":
		return leftNumber >= rightNumber
	case "<":
		return leftNumber < rightNumber
	case "<=":
		return leftNumber <= rightNumber
	default:
		return false
	}
}

func valuesEqual(left, right any) bool {
	if leftNumber, ok := numericDriverValue(left); ok {
		rightNumber, rightOK := numericDriverValue(right)
		return rightOK && leftNumber == rightNumber
	}
	return fmt.Sprint(left) == fmt.Sprint(right)
}

func numericDriverValue(value any) (float64, bool) {
	switch number := value.(type) {
	case int64:
		return float64(number), true
	case float64:
		return number, true
	case int:
		return float64(number), true
	default:
		return 0, false
	}
}

func paginateInMemRecords(records []map[string]any, query string, args []driver.Value) []map[string]any {
	upper := strings.ToUpper(query)
	limitIndex := strings.LastIndex(upper, " LIMIT $")
	if limitIndex < 0 {
		return records
	}
	limitArg := placeholderArgument(query[limitIndex+7:], args)
	offset := 0
	if offsetIndex := strings.LastIndex(upper, " OFFSET $"); offsetIndex >= 0 {
		offset = placeholderArgument(query[offsetIndex+8:], args)
	}
	if offset >= len(records) {
		return nil
	}
	end := min(offset+limitArg, len(records))
	return records[offset:end]
}

func placeholderArgument(fragment string, args []driver.Value) int {
	fragment = strings.TrimPrefix(strings.TrimSpace(fragment), "$")
	end := 0
	for end < len(fragment) && fragment[end] >= '0' && fragment[end] <= '9' {
		end++
	}
	position, _ := strconv.Atoi(fragment[:end])
	if position < 1 || position > len(args) {
		return 0
	}
	value, _ := numericDriverValue(args[position-1])
	return int(value)
}

type inMemRows struct {
	columns []string
	rows    [][]driver.Value
	pos     int
}

func (r *inMemRows) Columns() []string { return r.columns }
func (r *inMemRows) Close() error      { return nil }
func (r *inMemRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.pos])
	r.pos++
	return nil
}

func init() {
	sql.Register("gpp_inmem", &inMemDriver{})
}
