package queue_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/saifsilver/goplusplus/dbcore"
	"github.com/saifsilver/goplusplus/queue"
)

func TestOutboxSharesBusinessTransactionAndPublishes(t *testing.T) {
	database, outbox, publisher := newTestOutbox(t)
	ctx := context.Background()

	rolledBack, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := outbox.Enqueue(ctx, rolledBack, queue.OutboxMessage{
		Topic: "events", Key: []byte("rolled-back"), Payload: []byte("discarded"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := rolledBack.Rollback(); err != nil {
		t.Fatal(err)
	}
	if count, err := outbox.PublishPending(ctx); err != nil || count != 0 {
		t.Fatalf("rolled-back PublishPending = %d, %v", count, err)
	}

	committed, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	id, err := outbox.Enqueue(ctx, committed, queue.OutboxMessage{
		Topic: "events", Key: []byte("order-1"), Payload: []byte(`{"status":"created"}`),
		Headers: map[string][]byte{"content-type": []byte("application/json")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := committed.Commit(); err != nil {
		t.Fatal(err)
	}
	if count, err := outbox.PublishPending(ctx); err != nil || count != 1 {
		t.Fatalf("PublishPending = %d, %v", count, err)
	}
	messages := publisher.messagesSnapshot()
	if len(messages) != 1 || string(messages[0].key) != "order-1" || string(messages[0].headers["gpp-outbox-id"]) != id {
		t.Fatalf("published messages = %#v", messages)
	}
	if count, err := outbox.PublishPending(ctx); err != nil || count != 0 {
		t.Fatalf("already-published message was reclaimed: %d, %v", count, err)
	}
}

func TestOutboxRetriesPublisherFailures(t *testing.T) {
	database, outbox, publisher := newTestOutbox(t)
	ctx := context.Background()
	publisher.failures = 1
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := outbox.Enqueue(ctx, tx, queue.OutboxMessage{Topic: "events", Payload: []byte("retry")}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if count, err := outbox.PublishPending(ctx); err == nil || count != 1 {
		t.Fatalf("first PublishPending = %d, %v", count, err)
	}
	time.Sleep(5 * time.Millisecond)
	if count, err := outbox.PublishPending(ctx); err != nil || count != 1 {
		t.Fatalf("retry PublishPending = %d, %v", count, err)
	}
	if got := len(publisher.messagesSnapshot()); got != 1 {
		t.Fatalf("successful published messages = %d, want 1", got)
	}
}

func newTestOutbox(t *testing.T) (*dbcore.SQLiteClient, *queue.Outbox, *recordingPublisher) {
	t.Helper()
	database, err := dbcore.OpenSQLite(context.Background(), dbcore.SQLiteConfig{InMemory: true, MaxOpenConnections: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	migrations, err := queue.OutboxMigrations(100, database.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	if err := dbcore.AutoMigrate(context.Background(), database, migrations...); err != nil {
		t.Fatal(err)
	}
	publisher := &recordingPublisher{}
	outbox, err := queue.NewOutbox(database, publisher, queue.OutboxConfig{
		BatchSize: 10, LeaseDuration: time.Second, RetryBackoff: time.Millisecond, MaxAttempts: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	return database, outbox, publisher
}

type publishedMessage struct {
	topic   string
	key     []byte
	payload []byte
	headers map[string][]byte
}

type recordingPublisher struct {
	mu       sync.Mutex
	failures int
	messages []publishedMessage
}

func (publisher *recordingPublisher) Publish(
	_ context.Context, topic string, key, payload []byte, headers map[string][]byte,
) error {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	if publisher.failures > 0 {
		publisher.failures--
		return errors.New("broker unavailable")
	}
	publisher.messages = append(publisher.messages, publishedMessage{
		topic: topic, key: append([]byte(nil), key...), payload: append([]byte(nil), payload...), headers: headers,
	})
	return nil
}

func (publisher *recordingPublisher) messagesSnapshot() []publishedMessage {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	return append([]publishedMessage(nil), publisher.messages...)
}
