package dbcore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// EphemeralSession holds metadata for a materialized pagination temporary table.
type EphemeralSession struct {
	SessionID string    `json:"session_id"`
	TableName string    `json:"table_name"`
	TotalRows int       `json:"total_rows"`
	ExpiresAt time.Time `json:"expires_at"`
}

type EphemeralManager struct {
	mu       sync.Mutex
	sessions map[string]*EphemeralSession
}

var globalEphemeral = &EphemeralManager{
	sessions: make(map[string]*EphemeralSession),
}

// MaterializePagination executes a query, creates a temporary session table gpp_tmp_page_<session_id>, populates it, and returns an EphemeralSession.
func MaterializePagination(ctx context.Context, client *Client, query string, ttl time.Duration, args ...any) (*EphemeralSession, error) {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}

	sessionID := generateSessionID()
	tableName := fmt.Sprintf("gpp_tmp_page_%s", sessionID)

	createTableSQL := fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		row_num INTEGER PRIMARY KEY,
		data_json TEXT NOT NULL
	);`, tableName)

	if _, err := client.Exec(ctx, createTableSQL); err != nil {
		return nil, fmt.Errorf("dbcore/ephemeral: failed to create temporary result table: %w", err)
	}

	totalCount := 0
	if query != "" {
		err := client.Query(ctx, query, func(rows *sql.Rows) error {
			if rows == nil {
				return nil
			}
			cols, err := rows.Columns()
			if err != nil {
				return err
			}
			for rows.Next() {
				totalCount++
				columnPointers := make([]any, len(cols))
				columnValues := make([]any, len(cols))
				for i := range cols {
					columnPointers[i] = &columnValues[i]
				}
				if err := rows.Scan(columnPointers...); err != nil {
					continue
				}
				rowMap := make(map[string]any)
				for i, colName := range cols {
					val := columnValues[i]
					if b, ok := val.([]byte); ok {
						rowMap[colName] = string(b)
					} else {
						rowMap[colName] = val
					}
				}
				jsonBytes, _ := json.Marshal(rowMap)
				insertSQL := fmt.Sprintf("INSERT INTO %s (row_num, data_json) VALUES ($1, $2)", tableName)
				_, _ = client.Exec(ctx, insertSQL, totalCount, string(jsonBytes))
			}
			return rows.Err()
		}, args...)
		if err != nil {
			slog.Warn("dbcore/ephemeral: Query row population warning", slog.String("error", err.Error()))
		}
	}

	session := &EphemeralSession{
		SessionID: sessionID,
		TableName: tableName,
		TotalRows: totalCount,
		ExpiresAt: time.Now().Add(ttl),
	}

	globalEphemeral.mu.Lock()
	globalEphemeral.sessions[sessionID] = session
	globalEphemeral.mu.Unlock()

	// Schedule automatic table cleanup on expiration
	go func(tName, sID string, delay time.Duration) {
		time.Sleep(delay)
		cleanCtx := context.Background()
		dropSQL := fmt.Sprintf("DROP TABLE IF EXISTS %s", tName)
		_, _ = client.Exec(cleanCtx, dropSQL)

		globalEphemeral.mu.Lock()
		delete(globalEphemeral.sessions, sID)
		globalEphemeral.mu.Unlock()
		slog.Info("dbcore/ephemeral: Auto-cleaned temporary materialized pagination table", slog.String("session_id", sID))
	}(tableName, sessionID, ttl)

	return session, nil
}

// MaterializeHistoryPagination creates an immutable snapshot table tailored for historical data tables (audit logs, historical sales, past reports).
func MaterializeHistoryPagination(ctx context.Context, client *Client, historyQuery string, ttl time.Duration, args ...any) (*EphemeralSession, error) {
	if ttl <= 0 {
		ttl = 30 * time.Minute // Longer default TTL snapshot for historical data
	}
	slog.Info("dbcore/ephemeral: Materializing historical query snapshot table", slog.Duration("ttl", ttl))
	return MaterializePagination(ctx, client, historyQuery, ttl, args...)
}

// PaginateSession queries an ephemeral session table with O(1) indexed page lookup.
func PaginateSession(ctx context.Context, client *Client, sessionID string, page, limit int) ([]string, int, error) {
	globalEphemeral.mu.Lock()
	session, exists := globalEphemeral.sessions[sessionID]
	globalEphemeral.mu.Unlock()

	if !exists || time.Now().After(session.ExpiresAt) {
		return nil, 0, fmt.Errorf("dbcore/ephemeral: session '%s' expired or not found", sessionID)
	}

	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}

	startRow := (page-1)*limit + 1
	endRow := page * limit

	query := fmt.Sprintf("SELECT data_json FROM %s WHERE row_num BETWEEN $1 AND $2 ORDER BY row_num ASC", session.TableName)

	var rowsJSON []string
	err := client.Query(ctx, query, func(rows *sql.Rows) error {
		if rows == nil {
			return nil
		}
		for rows.Next() {
			var jsonStr string
			if err := rows.Scan(&jsonStr); err == nil {
				rowsJSON = append(rowsJSON, jsonStr)
			}
		}
		return rows.Err()
	}, startRow, endRow)

	if err != nil {
		return nil, 0, err
	}

	return rowsJSON, session.TotalRows, nil
}

// PaginateSessionTyped queries an ephemeral session table and unmarshals row JSON into typed structs []T.
func PaginateSessionTyped[T any](ctx context.Context, client *Client, sessionID string, page, limit int) ([]T, int, error) {
	jsonRows, total, err := PaginateSession(ctx, client, sessionID, page, limit)
	if err != nil {
		return nil, 0, err
	}

	var results []T
	for _, jsonStr := range jsonRows {
		var item T
		if err := json.Unmarshal([]byte(jsonStr), &item); err == nil {
			results = append(results, item)
		}
	}
	return results, total, nil
}

func generateSessionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
