package pubsub_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/saifsilver/goplusplus/pubsub"
)

func TestRedisBusPublishSubscribeIntegration(t *testing.T) {
	server := miniredis.RunT(t)
	bus, err := pubsub.NewRedisBus(context.Background(), pubsub.RedisConfig{
		URL: "redis://" + server.Addr(), ChannelPrefix: "test:",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bus.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	received := make(chan any, 1)
	subscriptionDone := make(chan error, 1)
	go func() {
		subscriptionDone <- bus.Subscribe(ctx, "events", func(message any) { received <- message })
	}()
	for server.PubSubNumSub("test:events")["test:events"] == 0 {
		select {
		case <-ctx.Done():
			t.Fatal("Redis subscription was not established")
		case <-time.After(time.Millisecond):
		}
	}
	if err := bus.Publish(ctx, "events", map[string]any{"id": "event-1"}); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-received:
		object, ok := message.(map[string]any)
		if !ok || object["id"] != "event-1" {
			t.Fatalf("message = %#v", message)
		}
	case <-ctx.Done():
		t.Fatal("Redis message was not received")
	}
	cancel()
	if err := <-subscriptionDone; err != nil {
		t.Fatal(err)
	}
}

func TestRedisBusRequiresConfiguration(t *testing.T) {
	if _, err := pubsub.NewRedisBus(context.Background(), pubsub.RedisConfig{}); err == nil {
		t.Fatal("empty Redis pub-sub configuration was accepted")
	}
}
