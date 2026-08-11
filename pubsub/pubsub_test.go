package pubsub

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestPubSubBus(t *testing.T) {
	bus := New()
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(2)

	var received1, received2 string
	bus.Subscribe("user_created", func(ctx context.Context, payload any) {
		defer wg.Done()
		if s, ok := payload.(string); ok {
			received1 = s
		}
	})
	bus.Subscribe("user_created", func(ctx context.Context, payload any) {
		defer wg.Done()
		if s, ok := payload.(string); ok {
			received2 = s
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
		if received1 != "alex_dev" || received2 != "alex_dev" {
			t.Errorf("expected 'alex_dev', got '%s', '%s'", received1, received2)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for event handler execution")
	}
}
