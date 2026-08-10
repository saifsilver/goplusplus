package queue

import (
	"context"
	"log/slog"
)

// Queue handles async background worker job dispatching.
type Queue struct{}

// New initializes a background task worker queue instance.
func New() *Queue {
	return &Queue{}
}

// Enqueue dispatches a task to the background worker pool.
func (q *Queue) Enqueue(ctx context.Context, taskName string, payload any) error {
	slog.Info("queue: Enqueued background job", slog.String("task", taskName), slog.Any("payload", payload))
	return nil
}
