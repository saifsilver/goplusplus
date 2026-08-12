package dbcore

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	modernsqlite "modernc.org/sqlite"
)

// ErrorKind is a stable category for database failures.
type ErrorKind string

const (
	// ErrorUniqueConstraint indicates a uniqueness violation.
	ErrorUniqueConstraint ErrorKind = "unique_constraint"
	// ErrorForeignKeyConstraint indicates a foreign-key violation.
	ErrorForeignKeyConstraint ErrorKind = "foreign_key_constraint"
	// ErrorNotNullConstraint indicates a required-column violation.
	ErrorNotNullConstraint ErrorKind = "not_null_constraint"
	// ErrorCheckConstraint indicates a check-constraint violation.
	ErrorCheckConstraint ErrorKind = "check_constraint"
	// ErrorBusy indicates database lock or busy contention.
	ErrorBusy ErrorKind = "busy_or_locked"
	// ErrorCanceled indicates cancellation or deadline expiry.
	ErrorCanceled ErrorKind = "query_canceled"
	// ErrorUnknown indicates an unclassified database failure.
	ErrorUnknown ErrorKind = "unknown"
)

// DatabaseError provides a stable category while retaining the driver error.
type DatabaseError struct {
	Kind  ErrorKind
	cause error
}

// Error returns the stable database error message.
func (e *DatabaseError) Error() string { return "database operation failed: " + string(e.Kind) }

// Unwrap returns the driver or standard-library cause.
func (e *DatabaseError) Unwrap() error { return e.cause }

// IsErrorKind reports whether err contains a DatabaseError with kind.
func IsErrorKind(err error, kind ErrorKind) bool {
	var databaseErr *DatabaseError
	return errors.As(err, &databaseErr) && databaseErr.Kind == kind
}

// ClassifyError maps SQLite and PostgreSQL failures without exposing raw driver
// text. Nil and sql.ErrNoRows are returned unchanged.
func ClassifyError(err error) error {
	if err == nil || errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var existing *DatabaseError
	if errors.As(err, &existing) {
		return err
	}
	kind := ErrorUnknown
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		kind = ErrorCanceled
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			kind = ErrorUniqueConstraint
		case "23503":
			kind = ErrorForeignKeyConstraint
		case "23502":
			kind = ErrorNotNullConstraint
		case "23514":
			kind = ErrorCheckConstraint
		case "55P03", "40001", "40P01":
			kind = ErrorBusy
		case "57014":
			kind = ErrorCanceled
		}
	}
	var sqliteErr *modernsqlite.Error
	if errors.As(err, &sqliteErr) {
		switch sqliteErr.Code() {
		case 2067, 1555:
			kind = ErrorUniqueConstraint
		case 787:
			kind = ErrorForeignKeyConstraint
		case 1299:
			kind = ErrorNotNullConstraint
		case 275:
			kind = ErrorCheckConstraint
		case 5, 6, 261, 262, 517:
			kind = ErrorBusy
		case 9:
			kind = ErrorCanceled
		}
	}
	return &DatabaseError{Kind: kind, cause: err}
}
