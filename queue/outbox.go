package queue

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/saifsilver/goplusplus/dbcore"
)

type OutboxDatabase interface {
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	Dialect() string
}

type OutboxPublisher interface {
	Publish(context.Context, string, []byte, []byte, map[string][]byte) error
}

type OutboxConfig struct {
	BatchSize     int
	PollInterval  time.Duration
	LeaseDuration time.Duration
	RetryBackoff  time.Duration
	MaxAttempts   int
}

type OutboxMessage struct {
	ID          string
	Topic       string
	Key         []byte
	Payload     []byte
	Headers     map[string][]byte
	AvailableAt time.Time
}

type Outbox struct {
	database  OutboxDatabase
	publisher OutboxPublisher
	config    OutboxConfig
}

type outboxRecord struct {
	OutboxMessage
	Attempts int
}

func NewOutbox(database OutboxDatabase, publisher OutboxPublisher, config OutboxConfig) (*Outbox, error) {
	if database == nil {
		return nil, errors.New("queue/outbox: database is required")
	}
	if publisher == nil {
		return nil, errors.New("queue/outbox: publisher is required")
	}
	if database.Dialect() != "postgres" && database.Dialect() != "sqlite" {
		return nil, fmt.Errorf("queue/outbox: unsupported database dialect %q", database.Dialect())
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.LeaseDuration <= 0 {
		config.LeaseDuration = 30 * time.Second
	}
	if config.RetryBackoff <= 0 {
		config.RetryBackoff = time.Second
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 10
	}
	return &Outbox{database: database, publisher: publisher, config: config}, nil
}

// OutboxMigrations returns the table and claim-index migrations. startVersion
// lets applications place them safely in their own migration sequence.
func OutboxMigrations(startVersion int, dialect string) ([]dbcore.Migration, error) {
	if startVersion < 1 {
		return nil, errors.New("queue/outbox: migration start version must be positive")
	}
	payloadType := "BYTEA"
	if dialect == "sqlite" {
		payloadType = "BLOB"
	} else if dialect != "postgres" {
		return nil, fmt.Errorf("queue/outbox: unsupported database dialect %q", dialect)
	}
	table := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS gpp_outbox (
	id VARCHAR(64) PRIMARY KEY,
	topic VARCHAR(249) NOT NULL,
	message_key %s,
	payload %s NOT NULL,
	headers TEXT NOT NULL,
	attempts INTEGER NOT NULL DEFAULT 0,
	available_at TIMESTAMP NOT NULL,
	locked_until TIMESTAMP NULL,
	lock_token VARCHAR(64) NULL,
	published_at TIMESTAMP NULL,
	failed_at TIMESTAMP NULL,
	last_error TEXT NULL,
	created_at TIMESTAMP NOT NULL
)`, payloadType, payloadType)
	return []dbcore.Migration{
		{ID: "gpp_outbox_v1", Version: startVersion, Name: "create gpp outbox", SQL: table},
		{
			ID: "gpp_outbox_claim_index_v1", Version: startVersion + 1, Name: "index gpp outbox claims",
			SQL: "CREATE INDEX IF NOT EXISTS gpp_outbox_claim_idx ON gpp_outbox (published_at, failed_at, available_at, locked_until)",
		},
	}, nil
}

// Enqueue inserts an event using the caller's business transaction. Delivery is
// at least once: consumers should deduplicate using the gpp-outbox-id header.
func (outbox *Outbox) Enqueue(ctx context.Context, tx *sql.Tx, message OutboxMessage) (string, error) {
	if tx == nil {
		return "", errors.New("queue/outbox: transaction is required")
	}
	if err := validateKafkaTopic(message.Topic); err != nil {
		return "", err
	}
	if len(message.Payload) > defaultKafkaMaxMessageBytes {
		return "", fmt.Errorf("queue/outbox: payload exceeds %d bytes", defaultKafkaMaxMessageBytes)
	}
	if message.ID == "" {
		id, err := newOutboxID()
		if err != nil {
			return "", err
		}
		message.ID = id
	}
	if len(message.ID) > 64 || strings.ContainsAny(message.ID, "\r\n\t ") {
		return "", errors.New("queue/outbox: invalid message ID")
	}
	headers, err := json.Marshal(message.Headers)
	if err != nil {
		return "", fmt.Errorf("queue/outbox: encode headers: %w", err)
	}
	now := time.Now().UTC()
	if message.AvailableAt.IsZero() {
		message.AvailableAt = now
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO gpp_outbox
		(id, topic, message_key, payload, headers, attempts, available_at, created_at)
		VALUES ($1, $2, $3, $4, $5, 0, $6, $7)`,
		message.ID, message.Topic, message.Key, message.Payload, string(headers), message.AvailableAt.UTC(), now,
	)
	if err != nil {
		return "", fmt.Errorf("queue/outbox: insert message: %w", dbcore.ClassifyError(err))
	}
	return message.ID, nil
}

func (outbox *Outbox) PublishPending(ctx context.Context) (int, error) {
	records, token, err := outbox.claim(ctx)
	if err != nil {
		return 0, err
	}
	var publishErrors []error
	for _, record := range records {
		headers := cloneOutboxHeaders(record.Headers)
		headers["gpp-outbox-id"] = []byte(record.ID)
		err := outbox.publisher.Publish(ctx, record.Topic, record.Key, record.Payload, headers)
		if err == nil {
			err = outbox.markPublished(ctx, record.ID, token)
		} else {
			err = errors.Join(err, outbox.markFailed(ctx, record, token, err))
		}
		if err != nil {
			publishErrors = append(publishErrors, fmt.Errorf("queue/outbox: publish %s: %w", record.ID, err))
		}
	}
	return len(records), errors.Join(publishErrors...)
}

func (outbox *Outbox) Run(ctx context.Context) error {
	ticker := time.NewTicker(outbox.config.PollInterval)
	defer ticker.Stop()
	for {
		if _, err := outbox.PublishPending(ctx); err != nil && ctx.Err() == nil {
			slog.Error("queue/outbox: publish batch failed", slog.Any("error", err))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (outbox *Outbox) claim(ctx context.Context) ([]outboxRecord, string, error) {
	token, err := newOutboxID()
	if err != nil {
		return nil, "", err
	}
	now := time.Now().UTC()
	lockedUntil := now.Add(outbox.config.LeaseDuration)
	tx, err := outbox.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", fmt.Errorf("queue/outbox: begin claim: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	query := sqliteOutboxClaimSQL
	args := []any{lockedUntil, token, now, outbox.config.BatchSize}
	if outbox.database.Dialect() == "postgres" {
		query = postgresOutboxClaimSQL
		args = []any{now, outbox.config.BatchSize, lockedUntil, token}
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("queue/outbox: claim messages: %w", dbcore.ClassifyError(err))
	}
	records, err := scanOutboxRecords(rows)
	if closeErr := rows.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, "", err
	}
	if err := tx.Commit(); err != nil {
		return nil, "", fmt.Errorf("queue/outbox: commit claims: %w", dbcore.ClassifyError(err))
	}
	committed = true
	return records, token, nil
}

func scanOutboxRecords(rows *sql.Rows) ([]outboxRecord, error) {
	var records []outboxRecord
	for rows.Next() {
		var record outboxRecord
		var headers []byte
		if err := rows.Scan(&record.ID, &record.Topic, &record.Key, &record.Payload, &headers, &record.Attempts, &record.AvailableAt); err != nil {
			return nil, fmt.Errorf("queue/outbox: scan claimed message: %w", dbcore.ClassifyError(err))
		}
		if err := json.Unmarshal(headers, &record.Headers); err != nil {
			return nil, fmt.Errorf("queue/outbox: decode headers: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("queue/outbox: iterate claims: %w", dbcore.ClassifyError(err))
	}
	return records, nil
}

func (outbox *Outbox) markPublished(ctx context.Context, id, token string) error {
	return outbox.update(ctx, `UPDATE gpp_outbox SET published_at = $1, locked_until = NULL, lock_token = NULL, last_error = NULL
		WHERE id = $2 AND lock_token = $3`, time.Now().UTC(), id, token)
}

func (outbox *Outbox) markFailed(ctx context.Context, record outboxRecord, token string, publishErr error) error {
	attempts := record.Attempts + 1
	lastError := truncateKafkaError(publishErr)
	if attempts >= outbox.config.MaxAttempts {
		return outbox.update(ctx, `UPDATE gpp_outbox SET attempts = $1, failed_at = $2, locked_until = NULL,
			lock_token = NULL, last_error = $3 WHERE id = $4 AND lock_token = $5`,
			attempts, time.Now().UTC(), lastError, record.ID, token)
	}
	delay := outbox.config.RetryBackoff * time.Duration(1<<min(attempts-1, 8))
	return outbox.update(ctx, `UPDATE gpp_outbox SET attempts = $1, available_at = $2, locked_until = NULL,
		lock_token = NULL, last_error = $3 WHERE id = $4 AND lock_token = $5`,
		attempts, time.Now().UTC().Add(delay), lastError, record.ID, token)
}

func (outbox *Outbox) update(ctx context.Context, statement string, args ...any) error {
	tx, err := outbox.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, statement, args...)
	if err != nil {
		_ = tx.Rollback()
		return dbcore.ClassifyError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		_ = tx.Rollback()
		return errors.New("queue/outbox: claim ownership lost")
	}
	if err := tx.Commit(); err != nil {
		return dbcore.ClassifyError(err)
	}
	return nil
}

func newOutboxID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("queue/outbox: generate ID: %w", err)
	}
	return hex.EncodeToString(random[:]), nil
}

func cloneOutboxHeaders(headers map[string][]byte) map[string][]byte {
	clone := make(map[string][]byte, len(headers)+1)
	for key, value := range headers {
		clone[key] = append([]byte(nil), value...)
	}
	return clone
}

const postgresOutboxClaimSQL = `WITH candidates AS (
	SELECT id FROM gpp_outbox
	WHERE published_at IS NULL AND failed_at IS NULL AND available_at <= $1
		AND (locked_until IS NULL OR locked_until < $1)
	ORDER BY created_at
	LIMIT $2
	FOR UPDATE SKIP LOCKED
)
UPDATE gpp_outbox AS outbox
SET locked_until = $3, lock_token = $4
FROM candidates
WHERE outbox.id = candidates.id
RETURNING outbox.id, outbox.topic, outbox.message_key, outbox.payload, outbox.headers, outbox.attempts, outbox.available_at`

const sqliteOutboxClaimSQL = `UPDATE gpp_outbox
SET locked_until = $1, lock_token = $2
WHERE id IN (
	SELECT id FROM gpp_outbox
	WHERE published_at IS NULL AND failed_at IS NULL AND available_at <= $3
		AND (locked_until IS NULL OR locked_until < $3)
	ORDER BY created_at
	LIMIT $4
)
RETURNING id, topic, message_key, payload, headers, attempts, available_at`
