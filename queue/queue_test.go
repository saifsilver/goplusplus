package queue

import (
	"context"
	"errors"
	"testing"
)

func TestQueueEnqueue(t *testing.T) {
	q := New()
	ctx := context.Background()

	err := q.Enqueue(ctx, "send_welcome_email", map[string]any{"user_id": 42})
	if !errors.Is(err, ErrQueueNotConfigured) {
		t.Errorf("Enqueue error = %v", err)
	}
}

func TestQueueWithPublisherEnqueuesTask(t *testing.T) {
	publisher := &taskPublisher{}
	q, err := NewWithPublisher(publisher, "tasks")
	if err != nil {
		t.Fatal(err)
	}
	if err := q.Enqueue(context.Background(), "send_welcome_email", map[string]any{"user_id": 42}); err != nil {
		t.Fatal(err)
	}
	if publisher.topic != "tasks" || string(publisher.headers["gpp-task-name"]) != "send_welcome_email" {
		t.Fatalf("publisher = %#v", publisher)
	}
}

type taskPublisher struct {
	topic   string
	headers map[string][]byte
}

func (publisher *taskPublisher) Publish(
	_ context.Context, topic string, _, _ []byte, headers map[string][]byte,
) error {
	publisher.topic = topic
	publisher.headers = headers
	return nil
}
