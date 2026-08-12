package dbcore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	defaultEphemeralTTL     = 10 * time.Minute
	maxEphemeralSessions    = 10_000
	maxEphemeralRows        = 100_000
	maxEphemeralBytes       = 64 << 20
	ephemeralCleanupTimeout = 5 * time.Second
)

// EphemeralSession holds metadata for a materialized pagination temporary table.
type EphemeralSession struct {
	SessionID string    `json:"session_id"`
	TableName string    `json:"table_name"`
	TotalRows int       `json:"total_rows"`
	ExpiresAt time.Time `json:"expires_at"`
}

// EphemeralManager tracks bounded process-local materialization sessions.
type EphemeralManager struct {
	mu       sync.Mutex
	sessions map[string]*EphemeralSession
}

// ErrEphemeralCapacity indicates that the process-local session bound was reached.
var ErrEphemeralCapacity = errors.New("dbcore/ephemeral: session capacity reached")

// ErrEphemeralResultTooLarge indicates that a materialization exceeded its memory safety bound.
var ErrEphemeralResultTooLarge = errors.New("dbcore/ephemeral: materialized result is too large")

var globalEphemeral = &EphemeralManager{
	sessions: make(map[string]*EphemeralSession),
}

// MaterializePagination executes a query, creates a temporary session table gpp_tmp_page_<session_id>, populates it, and returns an EphemeralSession.
func MaterializePagination(ctx context.Context, client *Client, query string, ttl time.Duration, args ...any) (*EphemeralSession, error) {
	if client == nil {
		return nil, errors.New("dbcore/ephemeral: database client is required")
	}
	if ttl <= 0 {
		ttl = defaultEphemeralTTL
	}
	if !globalEphemeral.hasCapacity() {
		return nil, ErrEphemeralCapacity
	}

	sessionID, err := generateSessionID()
	if err != nil {
		return nil, err
	}
	tableName := fmt.Sprintf("gpp_tmp_page_%s", sessionID)

	createTableSQL := fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		row_num INTEGER PRIMARY KEY,
		data_json TEXT NOT NULL
	);`, tableName)

	if _, err := client.Exec(ctx, createTableSQL); err != nil {
		return nil, fmt.Errorf("dbcore/ephemeral: failed to create temporary result table: %w", err)
	}
	cleanupOnFailure := true
	defer func() {
		if cleanupOnFailure {
			cleanupEphemeralTable(client, tableName, "")
		}
	}()

	materializedRows := make([]string, 0)
	materializedBytes := 0
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
				columnPointers := make([]any, len(cols))
				columnValues := make([]any, len(cols))
				for i := range cols {
					columnPointers[i] = &columnValues[i]
				}
				if err := rows.Scan(columnPointers...); err != nil {
					return fmt.Errorf("scan materialized row: %w", err)
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
				jsonBytes, err := json.Marshal(rowMap)
				if err != nil {
					return fmt.Errorf("encode materialized row: %w", err)
				}
				if len(materializedRows) >= maxEphemeralRows || materializedBytes+len(jsonBytes) > maxEphemeralBytes {
					return fmt.Errorf("%w: limit is %d rows or %d bytes", ErrEphemeralResultTooLarge, maxEphemeralRows, maxEphemeralBytes)
				}
				materializedRows = append(materializedRows, string(jsonBytes))
				materializedBytes += len(jsonBytes)
			}
			return rows.Err()
		}, args...)
		if err != nil {
			return nil, fmt.Errorf("dbcore/ephemeral: populate result table: %w", err)
		}
	}
	if len(materializedRows) > 0 {
		insertSQL := fmt.Sprintf("INSERT INTO %s (row_num, data_json) VALUES ($1, $2)", tableName)
		if err := client.InTx(ctx, func(tx *sql.Tx) error {
			for index, encodedRow := range materializedRows {
				if _, err := tx.ExecContext(ctx, insertSQL, index+1, encodedRow); err != nil {
					return fmt.Errorf("store materialized row %d: %w", index+1, err)
				}
			}
			return nil
		}); err != nil {
			return nil, fmt.Errorf("dbcore/ephemeral: persist result table: %w", err)
		}
	}
	totalCount := len(materializedRows)

	session := &EphemeralSession{
		SessionID: sessionID,
		TableName: tableName,
		TotalRows: totalCount,
		ExpiresAt: time.Now().Add(ttl),
	}

	if !globalEphemeral.add(session) {
		return nil, ErrEphemeralCapacity
	}
	cleanupOnFailure = false

	// Schedule automatic table cleanup on expiration
	go func(tName, sID string, delay time.Duration) {
		time.Sleep(delay)
		cleanupEphemeralTable(client, tName, sID)
	}(tableName, sessionID, ttl)

	return session, nil
}

func (manager *EphemeralManager) hasCapacity() bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return len(manager.sessions) < maxEphemeralSessions
}

func (manager *EphemeralManager) add(session *EphemeralSession) bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if len(manager.sessions) >= maxEphemeralSessions {
		return false
	}
	manager.sessions[session.SessionID] = session
	return true
}

func cleanupEphemeralTable(client *Client, tableName, sessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), ephemeralCleanupTimeout)
	defer cancel()
	_, dropErr := client.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName))
	if dropErr != nil {
		slog.Error("dbcore/ephemeral: failed to drop temporary table", slog.String("session_id", sessionID), slog.Any("error", dropErr))
	}
	if sessionID != "" {
		globalEphemeral.mu.Lock()
		delete(globalEphemeral.sessions, sessionID)
		globalEphemeral.mu.Unlock()
		if dropErr == nil {
			slog.Info("dbcore/ephemeral: auto-cleaned temporary materialized pagination table", slog.String("session_id", sessionID))
		}
	}
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
			if err := rows.Scan(&jsonStr); err != nil {
				return fmt.Errorf("dbcore/ephemeral: scan paginated row: %w", err)
			}
			rowsJSON = append(rowsJSON, jsonStr)
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
		if err := json.Unmarshal([]byte(jsonStr), &item); err != nil {
			return nil, 0, fmt.Errorf("dbcore/ephemeral: decode paginated row: %w", err)
		}
		results = append(results, item)
	}
	return results, total, nil
}

func generateSessionID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("dbcore/ephemeral: generate session ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}
