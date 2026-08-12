package dbcore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// Config defines PostgreSQL primary/replica connection and pool parameters.
// RWDSN is required. RODSN is optional; reads use the primary when it is empty.
type Config struct {
	RWDSN                    string
	RODSN                    string
	PgBouncerTransactionMode bool
	MaxOpenConnections       int
	MaxIdleConnections       int
	ConnectionMaxLifetime    time.Duration
	ConnectionMaxIdleTime    time.Duration
	PingTimeout              time.Duration
	SlowQuery                SlowQueryConfig
}

// Client owns PostgreSQL primary/replica pools. For backwards compatibility,
// NewClient also accepts the explicit SQLite test DSNs ":memory:" and "file:".
type Client struct {
	cfg      SlowQueryConfig
	rw       *sql.DB
	ro       *sql.DB
	dialect  string
	close    sync.Once
	closeErr error
}

type queryCacheKey struct{}

// WithCache annotates ctx with a requested query-cache lifetime.
func WithCache(ctx context.Context, ttl time.Duration) context.Context {
	return context.WithValue(ctx, queryCacheKey{}, ttl)
}

// GetCacheTTL returns the query-cache lifetime attached to ctx.
func GetCacheTTL(ctx context.Context) (time.Duration, bool) {
	ttl, ok := ctx.Value(queryCacheKey{}).(time.Duration)
	return ttl, ok
}

// NewClient opens the configured database. PostgreSQL is selected for
// postgres:// and postgresql:// DSNs. Explicit SQLite memory/file DSNs are
// retained for test compatibility; applications should prefer OpenSQLite.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	dsn := strings.TrimSpace(cfg.RWDSN)
	if dsn == ":memory:" || strings.HasPrefix(dsn, "file:") {
		return openCompatibilitySQLite(ctx, cfg)
	}
	return NewPostgresClient(ctx, cfg)
}

// NewPostgresClient opens and verifies real PostgreSQL primary and optional
// replica pools. It never falls back to an in-memory database.
func NewPostgresClient(ctx context.Context, cfg Config) (*Client, error) {
	if err := validatePostgresConfig(cfg); err != nil {
		return nil, err
	}

	rw, err := openPostgresPool(ctx, cfg.RWDSN, cfg)
	if err != nil {
		return nil, fmt.Errorf("dbcore/postgres: open primary: %w", err)
	}

	ro := rw
	if strings.TrimSpace(cfg.RODSN) != "" {
		ro, err = openPostgresPool(ctx, cfg.RODSN, cfg)
		if err != nil {
			_ = rw.Close()
			return nil, fmt.Errorf("dbcore/postgres: open replica: %w", err)
		}
	}

	return &Client{cfg: normalizeSlowQuery(cfg.SlowQuery), rw: rw, ro: ro, dialect: "postgres"}, nil
}

func validatePostgresConfig(cfg Config) error {
	if err := validatePoolConfig(cfg); err != nil {
		return fmt.Errorf("dbcore/postgres: %w", err)
	}
	if err := validatePostgresDSN(cfg.RWDSN, "RWDSN"); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.RODSN) != "" {
		if err := validatePostgresDSN(cfg.RODSN, "RODSN"); err != nil {
			return err
		}
	}
	return nil
}

func validatePostgresDSN(dsn, field string) error {
	parsed, err := url.Parse(strings.TrimSpace(dsn))
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" || strings.TrimPrefix(parsed.Path, "/") == "" {
		return fmt.Errorf("dbcore/postgres: %s must be a valid postgres:// or postgresql:// DSN", field)
	}
	return nil
}

func validatePoolConfig(cfg Config) error {
	if cfg.MaxOpenConnections < 0 || cfg.MaxIdleConnections < 0 {
		return errors.New("connection limits cannot be negative")
	}
	if cfg.MaxOpenConnections > 0 && cfg.MaxIdleConnections > cfg.MaxOpenConnections {
		return errors.New("maximum idle connections cannot exceed maximum open connections")
	}
	if cfg.ConnectionMaxLifetime < 0 || cfg.ConnectionMaxIdleTime < 0 || cfg.PingTimeout < 0 {
		return errors.New("connection durations cannot be negative")
	}
	return nil
}

func openPostgresPool(ctx context.Context, dsn string, cfg Config) (*sql.DB, error) {
	parsed, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("invalid PostgreSQL connection configuration")
	}
	if cfg.PgBouncerTransactionMode {
		parsed.DefaultQueryExecMode = pgx.QueryExecModeExec
	}
	db := stdlib.OpenDB(*parsed)
	configurePool(db, cfg)
	pingCtx := ctx
	cancel := func() {}
	if cfg.PingTimeout > 0 {
		pingCtx, cancel = context.WithTimeout(ctx, cfg.PingTimeout)
	}
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, ClassifyError(err)
	}
	return db, nil
}

func configurePool(db *sql.DB, cfg Config) {
	if cfg.MaxOpenConnections > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConnections)
	}
	if cfg.MaxIdleConnections > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConnections)
	}
	db.SetConnMaxLifetime(cfg.ConnectionMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnectionMaxIdleTime)
}

func normalizeSlowQuery(cfg SlowQueryConfig) SlowQueryConfig {
	if cfg.Threshold <= 0 {
		defaults := DefaultSlowQueryConfig()
		cfg.Threshold = defaults.Threshold
		if cfg.MaxSQLLength == 0 {
			cfg.OmitSQL = defaults.OmitSQL
		}
	}
	return cfg
}

// Dialect returns the configured SQL dialect.
func (c *Client) Dialect() string { return c.dialect }

// DB returns the primary read-write database pool.
func (c *Client) DB() *sql.DB { return c.rw }

// ReadDB returns the read pool, which may be the primary when no replica exists.
func (c *Client) ReadDB() *sql.DB { return c.ro }

// Exec executes a write statement against the primary pool.
func (c *Client) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	start := time.Now()
	res, err := c.rw.ExecContext(ctx, query, args...)
	LogSlowQuery(ctx, c.cfg, "rw", "write", query, time.Since(start), len(args), err)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return res, nil
}

// ExecContext implements context-aware SQL execution against the primary pool.
func (c *Client) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return c.Exec(ctx, query, args...)
}

// ExecIdempotent executes caller-defined idempotent SQL against the primary pool.
func (c *Client) ExecIdempotent(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return c.Exec(ctx, query, args...)
}

// ParallelTask describes one named SQL query for ParallelQuery.
type ParallelTask struct {
	QueryName string
	SQL       string
	Args      []any
}

// ParallelQuery executes independent queries concurrently and returns an observed error.
func (c *Client) ParallelQuery(ctx context.Context, tasks ...ParallelTask) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(tasks))
	for _, task := range tasks {
		wg.Add(1)
		go func(t ParallelTask) {
			defer wg.Done()
			if err := c.Query(WithQueryName(ctx, t.QueryName), t.SQL, nil, t.Args...); err != nil {
				errChan <- err
			}
		}(task)
	}
	wg.Wait()
	close(errChan)
	for err := range errChan {
		if err != nil {
			return err
		}
	}
	return nil
}

// QueryRow executes one read query and passes its row to fn.
func (c *Client) QueryRow(ctx context.Context, query string, fn func(*sql.Row) error, args ...any) error {
	start := time.Now()
	row := c.ro.QueryRowContext(ctx, query, args...)
	var err error
	if fn != nil {
		err = fn(row)
	}
	LogSlowQuery(ctx, c.cfg, "ro", "read_one", query, time.Since(start), len(args), err)
	return err
}

// Query executes a read query, invokes fn, and closes the returned rows.
func (c *Client) Query(ctx context.Context, query string, fn func(*sql.Rows) error, args ...any) error {
	start := time.Now()
	rows, err := c.ro.QueryContext(ctx, query, args...)
	LogSlowQuery(ctx, c.cfg, "ro", "read_many", query, time.Since(start), len(args), err)
	if err != nil {
		return ClassifyError(err)
	}
	defer rows.Close()
	if fn != nil {
		if err := fn(rows); err != nil {
			return err
		}
	}
	return ClassifyError(rows.Err())
}

// QueryContext implements context-aware querying against the read pool.
func (c *Client) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	start := time.Now()
	rows, err := c.ro.QueryContext(ctx, query, args...)
	LogSlowQuery(ctx, c.cfg, "ro", "read_many", query, time.Since(start), len(args), err)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return rows, nil
}

// QueryRowContext returns one row from the read pool.
func (c *Client) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return c.ro.QueryRowContext(ctx, query, args...)
}

// BeginTx begins a transaction on the primary pool.
func (c *Client) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	tx, err := c.rw.BeginTx(ctx, opts)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return tx, nil
}

// InTx commits fn on success and rolls back on failure.
func (c *Client) InTx(ctx context.Context, fn func(*sql.Tx) error) (err error) {
	if fn == nil {
		return errors.New("dbcore: transaction callback is nil")
	}
	start := time.Now()
	tx, err := c.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { LogSlowQuery(ctx, c.cfg, "rw", "tx", "transaction", time.Since(start), 0, err) }()
	if err = fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return ClassifyError(tx.Commit())
}

// PingContext verifies primary database connectivity.
func (c *Client) PingContext(ctx context.Context) error { return ClassifyError(c.rw.PingContext(ctx)) }

// Stats returns redacted primary and replica pool health and utilization.
func (c *Client) Stats() map[string]any {
	rw := c.rw.Stats()
	ro := c.ro.Stats()
	active, idle := rw.InUse, rw.Idle
	if c.ro != c.rw {
		active += ro.InUse
		idle += ro.Idle
	}
	return map[string]any{
		"dialect": c.dialect, "active_connections": active,
		"idle_connections": idle, "primary_healthy": boundedPing(c.rw),
		"replica_healthy": boundedPing(c.ro),
	}
}

func boundedPing(db *sql.DB) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return db.PingContext(ctx) == nil
}

// Close releases owned database pools once.
func (c *Client) Close() error {
	c.close.Do(func() {
		if c.ro != nil && c.ro != c.rw {
			c.closeErr = c.ro.Close()
		}
		if c.rw != nil {
			c.closeErr = errors.Join(c.closeErr, c.rw.Close())
		}
	})
	return c.closeErr
}
