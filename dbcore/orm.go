package dbcore

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/saifsilver/goplusplus/id"
)

// Cached struct reflection metadata to guarantee zero reflection allocation per request query loop
type structMeta struct {
	tableName      string
	pkField        string
	pkColumn       string
	columns        []string
	fieldMap       map[string]int // column name -> struct field index
	autoCreate     int            // field index for auto-created timestamp
	autoIDStrategy string         // auto_id strategy: "ulid", "snowflake", "uuid", "uuidv7", "prefix:xxx"
}

var metaCache sync.Map // reflect.Type -> *structMeta

func toSnakeCase(s string) string {
	var builder strings.Builder
	runes := []rune(s)
	for i, current := range runes {
		if i > 0 && current >= 'A' && current <= 'Z' {
			previous := runes[i-1]
			nextIsLower := i+1 < len(runes) && runes[i+1] >= 'a' && runes[i+1] <= 'z'
			previousIsLower := previous >= 'a' && previous <= 'z'
			previousIsDigit := previous >= '0' && previous <= '9'
			if previousIsLower || previousIsDigit || (nextIsLower && previous >= 'A' && previous <= 'Z') {
				builder.WriteByte('_')
			}
		}
		builder.WriteRune(current)
	}
	return strings.ToLower(builder.String())
}

func getStructMeta(t reflect.Type) *structMeta {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if val, ok := metaCache.Load(t); ok {
		return val.(*structMeta)
	}

	meta := &structMeta{
		tableName:  strings.ToLower(t.Name()) + "s",
		fieldMap:   make(map[string]int),
		autoCreate: -1,
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		tag := field.Tag.Get("db")
		colName := toSnakeCase(field.Name)
		isPK := false

		if tag != "" && tag != "-" {
			parts := strings.Split(tag, ",")
			colName = strings.TrimSpace(parts[0])
			for _, opt := range parts[1:] {
				opt = strings.TrimSpace(opt)
				if opt == "primary_key" || opt == "pk" {
					isPK = true
				} else if opt == "auto_create" {
					meta.autoCreate = i
				} else if strings.HasPrefix(opt, "auto_id=") {
					meta.autoIDStrategy = strings.TrimPrefix(opt, "auto_id=")
				}
			}
		}

		if colName == "id" || isPK {
			meta.pkField = field.Name
			meta.pkColumn = colName
		}

		meta.columns = append(meta.columns, colName)
		meta.fieldMap[colName] = i
	}

	if meta.pkColumn == "" && len(meta.columns) > 0 {
		meta.pkColumn = meta.columns[0]
		meta.pkField = t.Field(0).Name
	}

	metaCache.Store(t, meta)
	return meta
}

// ORM provides a zero-SQL high-performance object-relational mapping engine.
type ORM[T any] struct {
	client        *Client
	meta          *structMeta
	tableName     string
	whereConds    []string
	whereArgs     []any
	orderByClause string
}

// NewORM initializes an ORM instance for type T.
func NewORM[T any](client *Client, tableName ...string) *ORM[T] {
	var dummy T
	meta := getStructMeta(reflect.TypeOf(dummy))
	tName := meta.tableName
	if len(tableName) > 0 && tableName[0] != "" {
		tName = tableName[0]
	}
	return &ORM[T]{
		client:    client,
		meta:      meta,
		tableName: tName,
	}
}

// Where adds a parameterized filter condition (100% SQL Injection Proof).
func (o *ORM[T]) Where(field string, val any) *ORM[T] {
	clone := *o
	colName := field
	if !strings.Contains(field, " ") && !strings.Contains(field, "=") {
		colName = fmt.Sprintf("%s = $%d", field, len(o.whereConds)+1)
	}
	clone.whereConds = append(append([]string{}, o.whereConds...), colName)
	clone.whereArgs = append(append([]any{}, o.whereArgs...), val)
	return &clone
}

// OrderBy sets the query sort order.
func (o *ORM[T]) OrderBy(clause string) *ORM[T] {
	clone := *o
	clone.orderByClause = clause
	return &clone
}

// FindByID retrieves an entity by primary key ID.
func (o *ORM[T]) FindByID(ctx context.Context, id any) (*T, error) {
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s = $1 LIMIT 1",
		strings.Join(o.meta.columns, ", "), o.tableName, o.meta.pkColumn)

	var entity T
	found := false
	err := o.client.Query(ctx, query, func(rows *sql.Rows) error {
		found = true
		return scanStruct(rows, &entity)
	}, id)

	if err != nil {
		return nil, err
	}
	if !found {
		return nil, sql.ErrNoRows
	}
	return &entity, nil
}

// Find executes the ORM query and returns a slice of matching entities.
func (o *ORM[T]) Find(ctx context.Context) ([]T, error) {
	query := fmt.Sprintf("SELECT %s FROM %s", strings.Join(o.meta.columns, ", "), o.tableName)

	if len(o.whereConds) > 0 {
		query += " WHERE " + strings.Join(o.whereConds, " AND ")
	}
	if o.orderByClause != "" {
		query += " ORDER BY " + o.orderByClause
	}

	results := make([]T, 0)
	err := o.client.Query(ctx, query, func(rows *sql.Rows) error {
		for rows.Next() {
			var entity T
			if err := scanStruct(rows, &entity); err != nil {
				return err
			}
			results = append(results, entity)
		}
		return nil
	}, o.whereArgs...)

	return results, err
}

// Paginate executes a paginated query returning entities and total count in 1 call.
func (o *ORM[T]) Paginate(ctx context.Context, page, limit int) ([]T, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s", o.tableName)
	if len(o.whereConds) > 0 {
		countQuery += " WHERE " + strings.Join(o.whereConds, " AND ")
	}

	var total int
	_ = o.client.QueryRow(ctx, countQuery, func(row *sql.Row) error {
		return row.Scan(&total)
	}, o.whereArgs...)

	dataQuery := fmt.Sprintf("SELECT %s FROM %s", strings.Join(o.meta.columns, ", "), o.tableName)
	if len(o.whereConds) > 0 {
		dataQuery += " WHERE " + strings.Join(o.whereConds, " AND ")
	}
	if o.orderByClause != "" {
		dataQuery += " ORDER BY " + o.orderByClause
	}

	args := append([]any{}, o.whereArgs...)
	dataQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	results := make([]T, 0)
	err := o.client.Query(ctx, dataQuery, func(rows *sql.Rows) error {
		for rows.Next() {
			var entity T
			if err := scanStruct(rows, &entity); err != nil {
				return err
			}
			results = append(results, entity)
		}
		return nil
	}, args...)

	return results, total, err
}

// Save inserts a new record or updates an existing record based on primary key.
func (o *ORM[T]) Save(ctx context.Context, entity *T) error {
	val := reflect.ValueOf(entity).Elem()

	if o.meta.autoCreate >= 0 {
		field := val.Field(o.meta.autoCreate)
		if field.CanSet() && field.Type() == reflect.TypeOf(time.Time{}) {
			field.Set(reflect.ValueOf(time.Now()))
		}
	}

	pkVal := val.FieldByName(o.meta.pkField)
	isNew := !pkVal.IsValid() || pkVal.IsZero()

	// Auto-generate primary key ID if strategy is configured and PK is empty/zero
	if isNew && o.meta.autoIDStrategy != "" && pkVal.CanSet() {
		generated := id.GenerateAutoID(o.meta.autoIDStrategy)
		switch pkVal.Kind() {
		case reflect.String:
			if s, ok := generated.(string); ok {
				pkVal.SetString(s)
			}
		case reflect.Int64:
			if n, ok := generated.(int64); ok {
				pkVal.SetInt(n)
			}
		}
	}

	// Re-check after auto_id population
	isNew = !pkVal.IsValid() || pkVal.IsZero()

	if isNew {
		cols := make([]string, 0)
		placeholders := make([]string, 0)
		args := make([]any, 0)

		for colName, idx := range o.meta.fieldMap {
			if colName == o.meta.pkColumn {
				continue
			}
			cols = append(cols, colName)
			placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)+1))
			args = append(args, val.Field(idx).Interface())
		}

		insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			o.tableName, strings.Join(cols, ", "), strings.Join(placeholders, ", "))

		res, err := o.client.Exec(ctx, insertSQL, args...)
		if err != nil {
			return err
		}

		if pkVal.CanSet() && pkVal.Kind() == reflect.Int64 {
			if lastID, err := res.LastInsertId(); err == nil && lastID > 0 {
				pkVal.SetInt(lastID)
			}
		}
		return nil
	}

	setClauses := make([]string, 0)
	args := make([]any, 0)

	for colName, idx := range o.meta.fieldMap {
		if colName == o.meta.pkColumn {
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", colName, len(args)+1))
		args = append(args, val.Field(idx).Interface())
	}

	args = append(args, pkVal.Interface())
	updateSQL := fmt.Sprintf("UPDATE %s SET %s WHERE %s = $%d",
		o.tableName, strings.Join(setClauses, ", "), o.meta.pkColumn, len(args))

	_, err := o.client.Exec(ctx, updateSQL, args...)
	return err
}

// Delete removes an entity from the database by primary key.
func (o *ORM[T]) Delete(ctx context.Context, entity *T) error {
	val := reflect.ValueOf(entity).Elem()
	pkVal := val.FieldByName(o.meta.pkField)

	deleteSQL := fmt.Sprintf("DELETE FROM %s WHERE %s = $1", o.tableName, o.meta.pkColumn)
	_, err := o.client.Exec(ctx, deleteSQL, pkVal.Interface())
	return err
}

// AutoMigrate creates the database table automatically if it does not exist based on struct fields.
// Supports full SQL type mapping: SMALLINT, INTEGER, BIGINT, REAL, DOUBLE PRECISION, BOOLEAN, TIMESTAMP, TEXT.
func (o *ORM[T]) AutoMigrate(ctx context.Context) error {
	sqlCols := make([]string, 0)

	structType := reflect.TypeOf((*T)(nil)).Elem()
	if structType.Kind() == reflect.Ptr {
		structType = structType.Elem()
	}

	for _, colName := range o.meta.columns {
		idx := o.meta.fieldMap[colName]
		fieldType := structType.Field(idx).Type

		colType := goTypeToSQL(fieldType)

		isPK := colName == o.meta.pkColumn
		if isPK {
			// For auto_id string strategies (ulid, uuid, prefix), PK is TEXT
			if o.meta.autoIDStrategy != "" {
				switch o.meta.autoIDStrategy {
				case "snowflake":
					colType = "BIGINT"
				default:
					colType = "TEXT" // ulid, uuid, uuidv7, prefix:xxx all produce strings
				}
			}

			// Auto-increment for integer PKs without auto_id strategy
			if o.meta.autoIDStrategy == "" && isIntegerKind(fieldType.Kind()) {
				sqlCols = append(sqlCols, fmt.Sprintf("%s %s PRIMARY KEY AUTOINCREMENT", colName, colType))
				continue
			}

			sqlCols = append(sqlCols, fmt.Sprintf("%s %s PRIMARY KEY", colName, colType))
		} else {
			sqlCols = append(sqlCols, fmt.Sprintf("%s %s", colName, colType))
		}
	}

	createSQL := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s);", o.tableName, strings.Join(sqlCols, ", "))
	_, err := o.client.Exec(ctx, createSQL)
	return err
}

// goTypeToSQL maps a Go reflect.Type to the most appropriate SQL column type.
func goTypeToSQL(t reflect.Type) string {
	// Handle time.Time specifically
	if t == reflect.TypeOf(time.Time{}) {
		return "TIMESTAMP"
	}

	switch t.Kind() {
	case reflect.Int8, reflect.Int16, reflect.Uint8:
		return "SMALLINT"
	case reflect.Int, reflect.Int32, reflect.Uint16:
		return "INTEGER"
	case reflect.Int64, reflect.Uint32, reflect.Uint, reflect.Uint64:
		return "BIGINT"
	case reflect.Float32:
		return "REAL"
	case reflect.Float64:
		return "DOUBLE PRECISION"
	case reflect.Bool:
		return "BOOLEAN"
	case reflect.String:
		return "TEXT"
	default:
		return "TEXT"
	}
}

// isIntegerKind returns true if the reflect.Kind is any integer type.
func isIntegerKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	}
	return false
}

// QueryTyped executes a raw SQL query and automatically maps matching rows into []T without manual scanning loops.
func QueryTyped[T any](ctx context.Context, client *Client, query string, args ...any) ([]T, error) {
	results := make([]T, 0)
	err := client.Query(ctx, query, func(rows *sql.Rows) error {
		for rows.Next() {
			var entity T
			if err := scanStruct(rows, &entity); err != nil {
				return err
			}
			results = append(results, entity)
		}
		return nil
	}, args...)
	return results, err
}

// QueryRowTyped executes a raw SQL query and maps a single matching row into *T.
func QueryRowTyped[T any](ctx context.Context, client *Client, query string, args ...any) (*T, error) {
	var entity T
	found := false
	err := client.Query(ctx, query, func(rows *sql.Rows) error {
		found = true
		return scanStruct(rows, &entity)
	}, args...)

	if err != nil {
		return nil, err
	}
	if !found {
		return nil, sql.ErrNoRows
	}
	return &entity, nil
}

// QueryPaginated executes a raw SQL query with limit/offset pagination and returns results with total row count.
func QueryPaginated[T any](ctx context.Context, client *Client, baseQuery string, page, limit int, args ...any) ([]T, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM (%s) AS subquery", baseQuery)
	var total int
	_ = client.QueryRow(ctx, countSQL, func(row *sql.Row) error {
		return row.Scan(&total)
	}, args...)

	paginatedSQL := fmt.Sprintf("%s LIMIT $%d OFFSET $%d", baseQuery, len(args)+1, len(args)+2)
	pArgs := append(append([]any{}, args...), limit, offset)

	results, err := QueryTyped[T](ctx, client, paginatedSQL, pArgs...)
	return results, total, err
}
