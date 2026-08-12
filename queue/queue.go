package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrQueueNotConfigured indicates that a Queue has no durable publisher.
var ErrQueueNotConfigured = errors.New("queue: no durable queue provider is configured")

// Queue handles async background worker job dispatching.
type Queue struct {
	publisher OutboxPublisher
	topic     string
}

// New initializes a background task worker queue instance.
func New() *Queue {
	return &Queue{}
}

// NewWithPublisher constructs a queue using an explicit durable publisher and topic.
func NewWithPublisher(publisher OutboxPublisher, topic string) (*Queue, error) {
	if publisher == nil {
		return nil, ErrQueueNotConfigured
	}
	if err := validateKafkaTopic(topic); err != nil {
		return nil, err
	}
	return &Queue{publisher: publisher, topic: topic}, nil
}

// Enqueue dispatches a task to the background worker pool.
func (q *Queue) Enqueue(ctx context.Context, taskName string, payload any) error {
	if q == nil || q.publisher == nil {
		return ErrQueueNotConfigured
	}
	taskName = strings.TrimSpace(taskName)
	if taskName == "" || len(taskName) > 128 || strings.ContainsAny(taskName, "\r\n\t") {
		return errors.New("queue: invalid task name")
	}
	id, err := newOutboxID()
	if err != nil {
		return err
	}
	message, err := json.Marshal(struct {
		ID        string    `json:"id"`
		Task      string    `json:"task"`
		Payload   any       `json:"payload"`
		CreatedAt time.Time `json:"created_at"`
	}{ID: id, Task: taskName, Payload: payload, CreatedAt: time.Now().UTC()})
	if err != nil {
		return fmt.Errorf("queue: encode task: %w", err)
	}
	return q.publisher.Publish(ctx, q.topic, []byte(id), message, map[string][]byte{
		"content-type": []byte("application/json"), "gpp-task-name": []byte(taskName),
	})
}
