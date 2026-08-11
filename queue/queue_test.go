package queue

import (
	"context"
	"testing"
)

func TestQueueEnqueue(t *testing.T) {
	q := New()
	ctx := context.Background()

	err := q.Enqueue(ctx, "send_welcome_email", map[string]any{"user_id": 42})
	if err != nil {
		t.Errorf("Enqueue returned error: %v", err)
	}
}
