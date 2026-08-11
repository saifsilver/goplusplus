package pubsub_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/saifsilver/goplusplus/pubsub"
)

func TestPubSubBus(t *testing.T) {
	bus := pubsub.New()
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(1)

	var received string
	bus.Subscribe("user_created", func(ctx context.Context, payload any) {
		defer wg.Done()
		if s, ok := payload.(string); ok {
			received = s
		}
	})

	err := bus.Publish(ctx, "user_created", "alex_dev")
	if err != nil {
		t.Fatalf("Publish error: %v", err)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if received != "alex_dev" {
			t.Errorf("expected 'alex_dev', got '%s'", received)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for event handler execution")
	}
}
