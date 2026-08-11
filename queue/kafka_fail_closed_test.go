package queue

import (
	"context"
	"errors"
	"testing"
)

func TestUnconfiguredKafkaFailsClosed(t *testing.T) {
	worker := NewKafkaWorker(nil, "")
	if err := worker.PublishMessage(context.Background(), "key", nil); !errors.Is(err, ErrKafkaNotConfigured) {
		t.Fatalf("PublishMessage error = %v", err)
	}
}
