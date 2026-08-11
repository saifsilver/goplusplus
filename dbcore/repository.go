package dbcore

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
)

// Repository provides type-safe generic CRUD operations for database tables.
type Repository[T any] struct {
	client    *Client
	tableName string
}

// NewRepository creates a new generic Repository instance.
func NewRepository[T any](client *Client, tableName string) *Repository[T] {
	return &Repository[T]{
		client:    client,
		tableName: tableName,
	}
}

// FindByID retrieves a record by primary key ID.
func (r *Repository[T]) FindByID(ctx context.Context, id any) (*T, error) {
	query := fmt.Sprintf("SELECT * FROM %s WHERE id = $1 LIMIT 1", r.tableName)
	var entity T
	found := false

	err := r.client.Query(ctx, query, func(rows *sql.Rows) error {
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

// FindAll retrieves records matching a custom WHERE condition.
func (r *Repository[T]) FindAll(ctx context.Context, whereClause string, args ...any) ([]T, error) {
	query := fmt.Sprintf("SELECT * FROM %s", r.tableName)
	if strings.TrimSpace(whereClause) != "" {
		query += " WHERE " + whereClause
	}

	var results []T
	err := r.client.Query(ctx, query, func(rows *sql.Rows) error {
		for rows.Next() {
			var entity T
			if err := scanStruct(rows, &entity); err != nil {
				return err
			}
			results = append(results, entity)
		}
		return rows.Err()
	}, args...)

	if err != nil {
		return nil, err
	}
	return results, nil
}

// Create inserts a new record entity into the table.
func (r *Repository[T]) Create(ctx context.Context, entity *T) error {
	cols, args := extractStructFields(entity, true)
	if len(cols) == 0 {
		return fmt.Errorf("dbcore: no exported db fields found on struct %T", entity)
	}

	placeholders := make([]string, len(cols))
	for i := range cols {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		r.tableName,
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)

	return r.client.Exec(ctx, query, args...)
}

// Update updates an existing record matching ID.
func (r *Repository[T]) Update(ctx context.Context, id any, entity *T) error {
	cols, args := extractStructFields(entity, true)
	if len(cols) == 0 {
		return fmt.Errorf("dbcore: no fields to update for struct %T", entity)
	}

	setClauses := make([]string, len(cols))
	for i, col := range cols {
		setClauses[i] = fmt.Sprintf("%s = $%d", col, i+1)
	}

	args = append(args, id)
	query := fmt.Sprintf(
		"UPDATE %s SET %s WHERE id = $%d",
		r.tableName,
		strings.Join(setClauses, ", "),
		len(args),
	)

	return r.client.Exec(ctx, query, args...)
}

// Delete removes a record by primary key ID.
func (r *Repository[T]) Delete(ctx context.Context, id any) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1", r.tableName)
	return r.client.Exec(ctx, query, id)
}

// Paginate retrieves a paginated window of records along with total count.
func (r *Repository[T]) Paginate(ctx context.Context, page, limit int) ([]T, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s", r.tableName)
	err := r.client.Query(ctx, countQuery, func(rows *sql.Rows) error {
		if rows.Next() {
			return rows.Scan(&total)
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	dataQuery := fmt.Sprintf("SELECT * FROM %s LIMIT $1 OFFSET $2", r.tableName)
	var items []T
	err = r.client.Query(ctx, dataQuery, func(rows *sql.Rows) error {
		for rows.Next() {
			var entity T
			if err := scanStruct(rows, &entity); err != nil {
				return err
			}
			items = append(items, entity)
		}
		return rows.Err()
	}, limit, offset)

	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// CursorResult represents cursor-based pagination results for O(1) high-performance query execution.
type CursorResult[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor"`
	HasMore    bool   `json:"has_more"`
	Limit      int    `json:"limit"`
}

// PaginateCursor performs high-efficiency O(1) cursor pagination (e.g. WHERE id > cursor ORDER BY id ASC LIMIT limit + 1).
func (r *Repository[T]) PaginateCursor(ctx context.Context, cursorColumn string, cursorVal any, limit int) (*CursorResult[T], error) {
	if cursorColumn == "" {
		cursorColumn = "id"
	}
	if limit < 1 {
		limit = 20
	}
	fetchLimit := limit + 1

	var query string
	var args []any

	if cursorVal != nil && fmt.Sprintf("%v", cursorVal) != "" && fmt.Sprintf("%v", cursorVal) != "0" {
		query = fmt.Sprintf("SELECT * FROM %s WHERE %s > $1 ORDER BY %s ASC LIMIT $2", r.tableName, cursorColumn, cursorColumn)
		args = append(args, cursorVal, fetchLimit)
	} else {
		query = fmt.Sprintf("SELECT * FROM %s ORDER BY %s ASC LIMIT $1", r.tableName, cursorColumn)
		args = append(args, fetchLimit)
	}

	var items []T
	err := r.client.Query(ctx, query, func(rows *sql.Rows) error {
		for rows.Next() {
			var entity T
			if err := scanStruct(rows, &entity); err != nil {
				return err
			}
			items = append(items, entity)
		}
		return rows.Err()
	}, args...)

	if err != nil {
		return nil, err
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	var nextCursor string
	if len(items) > 0 {
		lastItem := items[len(items)-1]
		val := extractFieldValue(lastItem, cursorColumn)
		if val != nil {
			nextCursor = fmt.Sprintf("%v", val)
		}
	}

	return &CursorResult[T]{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Limit:      limit,
	}, nil
}

func extractFieldValue(item any, fieldName string) any {
	val := reflect.ValueOf(item)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil
	}
	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		dbTag := field.Tag.Get("db")
		if strings.EqualFold(dbTag, fieldName) || strings.EqualFold(field.Name, fieldName) {
			return val.Field(i).Interface()
		}
	}
	return nil
}

func scanStruct(rows *sql.Rows, dest any) error {
	cols, err := rows.Columns()
	if err != nil {
		return err
	}

	val := reflect.ValueOf(dest)
	if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("dbcore: scanStruct target must be a pointer to a struct")
	}

	structElem := val.Elem()
	structType := structElem.Type()

	fieldMap := make(map[string]reflect.Value)
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		dbTag := field.Tag.Get("db")
		if dbTag == "-" {
			continue
		}
		if dbTag == "" {
			dbTag = strings.ToLower(field.Name)
		}
		fieldMap[dbTag] = structElem.Field(i)
	}

	scanTargets := make([]any, len(cols))
	for i, col := range cols {
		if fieldVal, ok := fieldMap[strings.ToLower(col)]; ok && fieldVal.CanSet() {
			scanTargets[i] = fieldVal.Addr().Interface()
		} else {
			var dummy any
			scanTargets[i] = &dummy
		}
	}

	return rows.Scan(scanTargets...)
}

func extractStructFields(target any, skipID bool) ([]string, []any) {
	val := reflect.ValueOf(target)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil, nil
	}

	typ := val.Type()
	var cols []string
	var args []any

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		dbTag := field.Tag.Get("db")
		if dbTag == "-" {
			continue
		}
		colName := dbTag
		if colName == "" {
			colName = strings.ToLower(field.Name)
		}
		if skipID && strings.ToLower(colName) == "id" {
			continue
		}

		cols = append(cols, colName)
		args = append(args, val.Field(i).Interface())
	}

	return cols, args
}
