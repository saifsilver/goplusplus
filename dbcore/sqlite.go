package dbcore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	defaultSQLitePath        = "goplusplus.db"
	defaultSQLiteDirMode     = os.FileMode(0o750)
	defaultSQLiteBusyTimeout = 5 * time.Second
)

// SQLiteConfig is validated before the database is opened. Path and DSN are
// mutually exclusive. InMemory must be explicitly selected for a memory DB.
type SQLiteConfig struct {
	Path                  string
	DSN                   string
	DirectoryMode         os.FileMode
	BusyTimeout           time.Duration
	ForeignKeys           *bool
	MaxOpenConnections    int
	MaxIdleConnections    int
	ConnectionMaxLifetime time.Duration
	ConnectionMaxIdleTime time.Duration
	ReadOnly              bool
	InMemory              bool
	PingTimeout           time.Duration
	SlowQuery             SlowQueryConfig
}

// SQLiteClient owns a real database/sql SQLite pool.
type SQLiteClient struct {
	db   *sql.DB
	cfg  SQLiteConfig
	slow SlowQueryConfig
}

// NewSQLiteClient preserves the historical constructor with deterministic
// defaults. New code should use OpenSQLite for explicit configuration.
func NewSQLiteClient(dbPath string) (*SQLiteClient, error) {
	if strings.TrimSpace(dbPath) == "" {
		dbPath = defaultSQLitePath
	}
	return OpenSQLite(context.Background(), SQLiteConfig{Path: dbPath})
}

// OpenSQLite validates, opens, configures, and pings a pure-Go SQLite database.
func OpenSQLite(ctx context.Context, cfg SQLiteConfig) (*SQLiteClient, error) {
	normalized, dsn, err := normalizeSQLiteConfig(cfg)
	if err != nil {
		return nil, err
	}
	if !normalized.InMemory && normalized.DSN == "" && !normalized.ReadOnly {
		dir := filepath.Dir(normalized.Path)
		if err := os.MkdirAll(dir, normalized.DirectoryMode); err != nil {
			return nil, fmt.Errorf("dbcore/sqlite: create database directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("dbcore/sqlite: open database: %w", err)
	}
	poolCfg := Config{
		MaxOpenConnections:    normalized.MaxOpenConnections,
		MaxIdleConnections:    normalized.MaxIdleConnections,
		ConnectionMaxLifetime: normalized.ConnectionMaxLifetime,
		ConnectionMaxIdleTime: normalized.ConnectionMaxIdleTime,
	}
	configurePool(db, poolCfg)

	pingCtx := ctx
	cancel := func() {}
	if normalized.PingTimeout > 0 {
		pingCtx, cancel = context.WithTimeout(ctx, normalized.PingTimeout)
	}
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("dbcore/sqlite: ping database: %w", ClassifyError(err))
	}
	return &SQLiteClient{db: db, cfg: normalized, slow: normalizeSlowQuery(normalized.SlowQuery)}, nil
}

func normalizeSQLiteConfig(cfg SQLiteConfig) (SQLiteConfig, string, error) {
	if strings.TrimSpace(cfg.Path) != "" && strings.TrimSpace(cfg.DSN) != "" {
		return cfg, "", errors.New("dbcore/sqlite: Path and DSN are mutually exclusive")
	}
	if cfg.InMemory && (strings.TrimSpace(cfg.Path) != "" || strings.TrimSpace(cfg.DSN) != "") {
		return cfg, "", errors.New("dbcore/sqlite: InMemory cannot be combined with Path or DSN")
	}
	if cfg.ReadOnly && cfg.InMemory {
		return cfg, "", errors.New("dbcore/sqlite: read-only in-memory databases are unsupported")
	}
	if cfg.DirectoryMode == 0 {
		cfg.DirectoryMode = defaultSQLiteDirMode
	}
	if cfg.DirectoryMode.Perm()&0o022 != 0 {
		return cfg, "", errors.New("dbcore/sqlite: DirectoryMode must not be group- or world-writable")
	}
	if cfg.BusyTimeout == 0 {
		cfg.BusyTimeout = defaultSQLiteBusyTimeout
	}
	if cfg.BusyTimeout < 0 || cfg.BusyTimeout > 5*time.Minute {
		return cfg, "", errors.New("dbcore/sqlite: BusyTimeout must be between zero and five minutes")
	}
	if cfg.MaxOpenConnections < 0 || cfg.MaxIdleConnections < 0 || cfg.ConnectionMaxLifetime < 0 || cfg.ConnectionMaxIdleTime < 0 || cfg.PingTimeout < 0 {
		return cfg, "", errors.New("dbcore/sqlite: pool values cannot be negative")
	}
	if cfg.MaxOpenConnections > 0 && cfg.MaxIdleConnections > cfg.MaxOpenConnections {
		return cfg, "", errors.New("dbcore/sqlite: maximum idle connections cannot exceed maximum open connections")
	}
	if cfg.ForeignKeys == nil {
		enabled := true
		cfg.ForeignKeys = &enabled
	}
	if !cfg.InMemory && strings.TrimSpace(cfg.Path) == "" && strings.TrimSpace(cfg.DSN) == "" {
		return cfg, "", errors.New("dbcore/sqlite: Path, DSN, or InMemory is required")
	}

	dsn := strings.TrimSpace(cfg.DSN)
	if cfg.InMemory {
		name, err := randomSQLiteName()
		if err != nil {
			return cfg, "", fmt.Errorf("dbcore/sqlite: create memory database name: %w", err)
		}
		dsn = "file:" + name + "?mode=memory&cache=shared"
		if cfg.MaxOpenConnections == 0 {
			cfg.MaxOpenConnections = 1
		}
		if cfg.MaxIdleConnections == 0 {
			cfg.MaxIdleConnections = 1
		}
	} else if dsn == "" {
		cfg.Path = filepath.Clean(cfg.Path)
		dsn = "file:" + cfg.Path
		if cfg.ReadOnly {
			dsn += "?mode=ro"
		}
	}
	if cfg.ReadOnly && !cfg.InMemory {
		var err error
		dsn, err = sqliteDSNWithMode(dsn, "ro")
		if err != nil {
			return cfg, "", err
		}
	}
	dsn, err := sqliteDSNWithPragmas(dsn, *cfg.ForeignKeys, cfg.BusyTimeout)
	if err != nil {
		return cfg, "", err
	}
	return cfg, dsn, nil
}

func sqliteDSNWithMode(dsn, mode string) (string, error) {
	parts := strings.SplitN(dsn, "?", 2)
	values := make(url.Values)
	if len(parts) == 2 {
		parsed, err := url.ParseQuery(parts[1])
		if err != nil {
			return "", errors.New("dbcore/sqlite: malformed DSN query")
		}
		values = parsed
	}
	values.Set("mode", mode)
	return parts[0] + "?" + values.Encode(), nil
}

func randomSQLiteName() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "gpp-memory-" + hex.EncodeToString(buf), nil
}

func sqliteDSNWithPragmas(dsn string, foreignKeys bool, busy time.Duration) (string, error) {
	if !strings.HasPrefix(dsn, "file:") {
		return "", errors.New("dbcore/sqlite: DSN must use the file: URI form")
	}
	parts := strings.SplitN(dsn, "?", 2)
	values := make(url.Values)
	if len(parts) == 2 {
		parsed, err := url.ParseQuery(parts[1])
		if err != nil {
			return "", errors.New("dbcore/sqlite: malformed DSN query")
		}
		values = parsed
	}
	foreignKeyValue := "0"
	if foreignKeys {
		foreignKeyValue = "1"
	}
	values.Add("_pragma", "foreign_keys("+foreignKeyValue+")")
	values.Add("_pragma", "busy_timeout("+strconv.FormatInt(busy.Milliseconds(), 10)+")")
	return parts[0] + "?" + values.Encode(), nil
}

func openCompatibilitySQLite(ctx context.Context, cfg Config) (*Client, error) {
	sqliteCfg := SQLiteConfig{
		MaxOpenConnections: cfg.MaxOpenConnections, MaxIdleConnections: cfg.MaxIdleConnections,
		ConnectionMaxLifetime: cfg.ConnectionMaxLifetime, ConnectionMaxIdleTime: cfg.ConnectionMaxIdleTime,
		PingTimeout: cfg.PingTimeout, SlowQuery: cfg.SlowQuery,
	}
	if strings.TrimSpace(cfg.RWDSN) == ":memory:" {
		sqliteCfg.InMemory = true
	} else {
		sqliteCfg.DSN = cfg.RWDSN
	}
	s, err := OpenSQLite(ctx, sqliteCfg)
	if err != nil {
		return nil, err
	}
	return &Client{cfg: s.slow, rw: s.db, ro: s.db, dialect: "sqlite"}, nil
}

// Dialect returns sqlite.
func (s *SQLiteClient) Dialect() string { return "sqlite" }

// DB returns the underlying SQLite database pool.
func (s *SQLiteClient) DB() *sql.DB { return s.db }

// PingContext verifies SQLite connectivity.
func (s *SQLiteClient) PingContext(ctx context.Context) error {
	return ClassifyError(s.db.PingContext(ctx))
}

// Close releases the SQLite database pool.
func (s *SQLiteClient) Close() error { return s.db.Close() }

// Stats returns SQLite pool statistics.
func (s *SQLiteClient) Stats() sql.DBStats { return s.db.Stats() }

// Exec preserves the legacy error-only signature while executing real SQL.
func (s *SQLiteClient) Exec(ctx context.Context, query string, args ...any) error {
	_, err := s.ExecContext(ctx, query, args...)
	return err
}

// ExecContext executes a write statement and classifies failures.
func (s *SQLiteClient) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	start := time.Now()
	result, err := s.db.ExecContext(ctx, query, args...)
	LogSlowQuery(ctx, s.slow, "rw", "write", query, time.Since(start), len(args), err)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return result, nil
}

// QueryContext executes a query and classifies startup failures.
func (s *SQLiteClient) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return rows, nil
}

// QueryRowContext returns one SQLite query row.
func (s *SQLiteClient) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, query, args...)
}

// BeginTx begins a SQLite transaction.
func (s *SQLiteClient) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	tx, err := s.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return tx, nil
}

// InTx commits fn on success and rolls back on failure.
func (s *SQLiteClient) InTx(ctx context.Context, fn func(*sql.Tx) error) error {
	if fn == nil {
		return errors.New("dbcore/sqlite: transaction callback is nil")
	}
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return ClassifyError(tx.Commit())
}
