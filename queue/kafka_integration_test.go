package queue_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/saifsilver/goplusplus/queue"
	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/twmb/franz-go/pkg/kgo"
)

func TestKafkaProducerConsumerIntegration(t *testing.T) {
	cluster := kfake.MustCluster(kfake.SeedTopics(1, "events", "events.dlq"))
	t.Cleanup(cluster.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	producer, err := queue.NewKafkaProducer(ctx, queue.KafkaProducerConfig{
		KafkaConfig: queue.KafkaConfig{Brokers: cluster.ListenAddrs(), ClientID: "producer-test"},
		Topic:       "events",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = producer.Close(context.Background()) })
	consumer, err := queue.NewKafkaConsumer(ctx, queue.KafkaConsumerConfig{
		KafkaConfig:       queue.KafkaConfig{Brokers: cluster.ListenAddrs(), ClientID: "consumer-test"},
		Topics:            []string{"events"},
		GroupID:           "integration-group",
		DLQTopic:          "events.dlq",
		MaxPollRecords:    10,
		RetryBackoff:      time.Millisecond,
		MaxHandlerRetries: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	runCtx, stop := context.WithCancel(ctx)
	received := make(chan queue.KafkaMessage, 1)
	runErr := make(chan error, 1)
	go func() {
		runErr <- consumer.Run(runCtx, func(_ context.Context, message queue.KafkaMessage) error {
			received <- message
			return nil
		})
	}()
	if err := producer.PublishMessage(ctx, "order-1", []byte(`{"status":"created"}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-received:
		if string(message.Key) != "order-1" || string(message.Value) != `{"status":"created"}` {
			t.Fatalf("message = %#v", message)
		}
	case <-ctx.Done():
		t.Fatal("Kafka message was not consumed")
	}
	time.Sleep(20 * time.Millisecond)
	stop()
	if err := <-runErr; err != nil {
		t.Fatal(err)
	}
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer closeCancel()
	if err := consumer.Close(closeCtx); err != nil {
		t.Fatal(err)
	}
}

func TestKafkaConsumerRetriesThenPublishesDLQ(t *testing.T) {
	cluster := kfake.MustCluster(kfake.SeedTopics(1, "commands", "commands.dlq"))
	t.Cleanup(cluster.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	producer, err := queue.NewKafkaProducer(ctx, queue.KafkaProducerConfig{
		KafkaConfig: queue.KafkaConfig{Brokers: cluster.ListenAddrs()}, Topic: "commands",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = producer.Close(context.Background()) })
	consumer, err := queue.NewKafkaConsumer(ctx, queue.KafkaConsumerConfig{
		KafkaConfig: queue.KafkaConfig{Brokers: cluster.ListenAddrs()}, Topics: []string{"commands"},
		GroupID: "dlq-group", DLQTopic: "commands.dlq", MaxHandlerRetries: 2, RetryBackoff: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	runCtx, stop := context.WithCancel(ctx)
	runErr := make(chan error, 1)
	var attempts atomic.Int32
	go func() {
		runErr <- consumer.Run(runCtx, func(context.Context, queue.KafkaMessage) error {
			attempts.Add(1)
			return errors.New("invalid command")
		})
	}()
	if err := producer.PublishMessage(ctx, "command-1", []byte("poison")); err != nil {
		t.Fatal(err)
	}
	dlqRecord := consumeKafkaRecord(t, ctx, cluster.ListenAddrs(), "commands.dlq")
	if string(dlqRecord.Value) != "poison" || kafkaHeader(dlqRecord, "gpp-handler-attempts") != "3" {
		t.Fatalf("DLQ record = %#v", dlqRecord)
	}
	if got := attempts.Load(); got != 3 {
		t.Fatalf("handler attempts = %d, want 3", got)
	}
	time.Sleep(20 * time.Millisecond)
	stop()
	if err := <-runErr; err != nil {
		t.Fatal(err)
	}
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer closeCancel()
	if err := consumer.Close(closeCtx); err != nil {
		t.Fatal(err)
	}
}

func TestKafkaConfigurationFailsClosed(t *testing.T) {
	ctx := context.Background()
	if _, err := queue.NewKafkaProducer(ctx, queue.KafkaProducerConfig{}); err == nil {
		t.Fatal("empty Kafka configuration was accepted")
	}
	if _, err := queue.NewKafkaProducer(ctx, queue.KafkaProducerConfig{
		KafkaConfig: queue.KafkaConfig{
			Brokers: []string{"broker:9092"}, SASL: queue.KafkaSASLConfig{Mechanism: "plain", Username: "user", Password: "secret"},
		},
		Topic: "events",
	}); err == nil {
		t.Fatal("plaintext SASL configuration was accepted")
	}
}

func consumeKafkaRecord(t *testing.T, ctx context.Context, brokers []string, topic string) *kgo.Record {
	t.Helper()
	client, err := kgo.NewClient(kgo.SeedBrokers(brokers...), kgo.ConsumeTopics(topic), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	for {
		fetches := client.PollRecords(ctx, 1)
		if errs := fetches.Errors(); len(errs) > 0 {
			t.Fatal(errs[0].Err)
		}
		if records := fetches.Records(); len(records) > 0 {
			return records[0]
		}
	}
}

func kafkaHeader(record *kgo.Record, key string) string {
	for _, header := range record.Headers {
		if header.Key == key {
			return string(header.Value)
		}
	}
	return ""
}
