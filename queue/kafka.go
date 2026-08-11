package queue

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

const (
	defaultKafkaMaxMessageBytes = 1 << 20
	defaultKafkaDeliveryTimeout = 30 * time.Second
	defaultKafkaDialTimeout     = 10 * time.Second
	defaultKafkaRetryBackoff    = 100 * time.Millisecond
)

var (
	ErrKafkaNotConfigured = errors.New("queue: Kafka provider is not configured")
	ErrKafkaConsumerRun   = errors.New("queue: Kafka consumer is already running")
)

type KafkaSASLConfig struct {
	Mechanism string
	Username  string
	Password  string
}

type KafkaConfig struct {
	Brokers          []string
	ClientID         string
	TLS              *tls.Config
	SASL             KafkaSASLConfig
	DialTimeout      time.Duration
	DeliveryTimeout  time.Duration
	MaxMessageBytes  int
	MaxBufferedBytes int
	RecordRetries    int
}

type KafkaProducerConfig struct {
	KafkaConfig
	Topic string
}

type KafkaMessage struct {
	Topic     string
	Key       []byte
	Value     []byte
	Headers   map[string][]byte
	Partition int32
	Offset    int64
	Time      time.Time
}

type KafkaProducer struct {
	client          *kgo.Client
	topic           string
	maxMessageBytes int
}

// KafkaWorker is retained as the default-topic producer type.
type KafkaWorker = KafkaProducer

// NewKafkaWorker is retained for source compatibility but intentionally fails
// closed. Applications should construct a verified producer with NewKafkaProducer.
func NewKafkaWorker(brokers []string, topic string) *KafkaWorker {
	return &KafkaWorker{topic: topic, maxMessageBytes: defaultKafkaMaxMessageBytes}
}

func NewKafkaProducer(ctx context.Context, config KafkaProducerConfig) (*KafkaProducer, error) {
	normalized, options, err := kafkaClientOptions(config.KafkaConfig)
	if err != nil {
		return nil, err
	}
	if err := validateKafkaTopic(config.Topic); err != nil {
		return nil, err
	}
	options = append(options,
		kgo.DefaultProduceTopic(config.Topic),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.RecordRetries(normalized.RecordRetries),
		kgo.RecordDeliveryTimeout(normalized.DeliveryTimeout),
		kgo.ProducerBatchMaxBytes(int32(normalized.MaxMessageBytes)),
		kgo.MaxBufferedBytes(normalized.MaxBufferedBytes),
		kgo.StopProducerOnDataLossDetected(),
	)
	client, err := kgo.NewClient(options...)
	if err != nil {
		return nil, fmt.Errorf("queue: create Kafka producer: %w", err)
	}
	if err := client.Ping(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("queue: connect Kafka producer: %w", err)
	}
	return &KafkaProducer{client: client, topic: config.Topic, maxMessageBytes: normalized.MaxMessageBytes}, nil
}

func (producer *KafkaProducer) PublishMessage(ctx context.Context, key string, payload []byte) error {
	return producer.Publish(ctx, producer.topic, []byte(key), payload, nil)
}

func (producer *KafkaProducer) Publish(
	ctx context.Context, topic string, key, payload []byte, headers map[string][]byte,
) error {
	if producer == nil || producer.client == nil {
		return ErrKafkaNotConfigured
	}
	if err := validateKafkaTopic(topic); err != nil {
		return err
	}
	if len(payload) > producer.maxMessageBytes {
		return fmt.Errorf("queue: Kafka message exceeds %d bytes", producer.maxMessageBytes)
	}
	record := &kgo.Record{Topic: topic, Key: key, Value: payload, Headers: kafkaRecordHeaders(headers)}
	if err := producer.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return fmt.Errorf("queue: publish Kafka message: %w", err)
	}
	return nil
}

func (producer *KafkaProducer) Close(ctx context.Context) error {
	if producer == nil || producer.client == nil {
		return nil
	}
	err := producer.client.Flush(ctx)
	producer.client.Close()
	if err != nil {
		return fmt.Errorf("queue: flush Kafka producer: %w", err)
	}
	return nil
}

type KafkaConsumerConfig struct {
	KafkaConfig
	Topics            []string
	GroupID           string
	DLQTopic          string
	MaxPollRecords    int
	MaxHandlerRetries int
	RetryBackoff      time.Duration
}

type KafkaConsumer struct {
	client            *kgo.Client
	dlqTopic          string
	maxPollRecords    int
	maxHandlerRetries int
	retryBackoff      time.Duration
	running           atomic.Bool
	closeOnce         sync.Once
	closed            chan struct{}
}

func NewKafkaConsumer(ctx context.Context, config KafkaConsumerConfig) (*KafkaConsumer, error) {
	normalized, options, err := kafkaClientOptions(config.KafkaConfig)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.GroupID) == "" || strings.ContainsAny(config.GroupID, "\r\n\t") {
		return nil, errors.New("queue: Kafka consumer group ID is required")
	}
	if len(config.Topics) == 0 {
		return nil, errors.New("queue: Kafka consumer requires at least one topic")
	}
	for _, topic := range config.Topics {
		if err := validateKafkaTopic(topic); err != nil {
			return nil, err
		}
	}
	if config.DLQTopic == "" {
		config.DLQTopic = config.Topics[0] + ".dlq"
	}
	if err := validateKafkaTopic(config.DLQTopic); err != nil {
		return nil, fmt.Errorf("queue: invalid Kafka DLQ: %w", err)
	}
	if config.MaxPollRecords <= 0 {
		config.MaxPollRecords = 100
	}
	if config.MaxHandlerRetries < 0 {
		return nil, errors.New("queue: Kafka handler retries cannot be negative")
	}
	if config.MaxHandlerRetries == 0 {
		config.MaxHandlerRetries = 3
	}
	if config.RetryBackoff <= 0 {
		config.RetryBackoff = defaultKafkaRetryBackoff
	}
	options = append(options,
		kgo.ConsumerGroup(config.GroupID),
		kgo.ConsumeTopics(config.Topics...),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.DisableAutoCommit(),
		kgo.BlockRebalanceOnPoll(),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.RecordRetries(normalized.RecordRetries),
		kgo.RecordDeliveryTimeout(normalized.DeliveryTimeout),
		kgo.ProducerBatchMaxBytes(int32(normalized.MaxMessageBytes)),
		kgo.MaxBufferedBytes(normalized.MaxBufferedBytes),
		kgo.StopProducerOnDataLossDetected(),
	)
	client, err := kgo.NewClient(options...)
	if err != nil {
		return nil, fmt.Errorf("queue: create Kafka consumer: %w", err)
	}
	if err := client.Ping(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("queue: connect Kafka consumer: %w", err)
	}
	return &KafkaConsumer{
		client: client, dlqTopic: config.DLQTopic, maxPollRecords: config.MaxPollRecords,
		maxHandlerRetries: config.MaxHandlerRetries, retryBackoff: config.RetryBackoff,
		closed: make(chan struct{}),
	}, nil
}

func (consumer *KafkaConsumer) Run(ctx context.Context, handler func(context.Context, KafkaMessage) error) error {
	if consumer == nil || consumer.client == nil {
		return ErrKafkaNotConfigured
	}
	if handler == nil {
		return errors.New("queue: Kafka consumer handler is required")
	}
	if !consumer.running.CompareAndSwap(false, true) {
		return ErrKafkaConsumerRun
	}
	defer consumer.running.Store(false)
	for {
		fetches := consumer.client.PollRecords(ctx, consumer.maxPollRecords)
		if ctx.Err() != nil || fetches.IsClientClosed() {
			return nil
		}
		if fetchErrors := fetches.Errors(); len(fetchErrors) > 0 {
			consumer.client.AllowRebalance()
			return fmt.Errorf("queue: consume Kafka records: %w", fetchErrors[0].Err)
		}
		var processErr error
		fetches.EachRecord(func(record *kgo.Record) {
			if processErr != nil {
				return
			}
			if err := consumer.processRecord(ctx, record, handler); err != nil {
				processErr = err
				return
			}
			if err := consumer.client.CommitRecords(ctx, record); err != nil {
				processErr = fmt.Errorf("queue: commit Kafka offset: %w", err)
			}
		})
		consumer.client.AllowRebalance()
		if processErr != nil {
			return processErr
		}
	}
}

func (consumer *KafkaConsumer) processRecord(
	ctx context.Context, record *kgo.Record, handler func(context.Context, KafkaMessage) error,
) error {
	message := kafkaMessage(record)
	var handlerErr error
	for attempt := 0; attempt <= consumer.maxHandlerRetries; attempt++ {
		if handlerErr = handler(ctx, message); handlerErr == nil {
			return nil
		}
		if attempt < consumer.maxHandlerRetries {
			if err := waitForKafkaRetry(ctx, consumer.retryBackoff, attempt); err != nil {
				return err
			}
		}
	}
	headers := message.Headers
	if headers == nil {
		headers = make(map[string][]byte)
	}
	headers["gpp-original-topic"] = []byte(record.Topic)
	headers["gpp-original-partition"] = []byte(strconv.FormatInt(int64(record.Partition), 10))
	headers["gpp-original-offset"] = []byte(strconv.FormatInt(record.Offset, 10))
	headers["gpp-handler-attempts"] = []byte(strconv.Itoa(consumer.maxHandlerRetries + 1))
	headers["gpp-handler-error"] = []byte(truncateKafkaError(handlerErr))
	dlqRecord := &kgo.Record{Topic: consumer.dlqTopic, Key: record.Key, Value: record.Value, Headers: kafkaRecordHeaders(headers)}
	if err := consumer.client.ProduceSync(ctx, dlqRecord).FirstErr(); err != nil {
		return fmt.Errorf("queue: publish Kafka DLQ message: %w", err)
	}
	return nil
}

func (consumer *KafkaConsumer) Close(ctx context.Context) error {
	if consumer == nil || consumer.client == nil {
		return nil
	}
	consumer.closeOnce.Do(func() {
		go func() {
			consumer.client.CloseAllowingRebalance()
			close(consumer.closed)
		}()
	})
	select {
	case <-ctx.Done():
		return fmt.Errorf("queue: close Kafka consumer: %w", ctx.Err())
	case <-consumer.closed:
		return nil
	}
}

func kafkaClientOptions(config KafkaConfig) (KafkaConfig, []kgo.Opt, error) {
	if len(config.Brokers) == 0 {
		return KafkaConfig{}, nil, errors.New("queue: Kafka brokers are required")
	}
	for _, broker := range config.Brokers {
		if strings.TrimSpace(broker) == "" || strings.Contains(broker, "://") || strings.ContainsAny(broker, "\r\n\t ") {
			return KafkaConfig{}, nil, fmt.Errorf("queue: invalid Kafka broker %q", broker)
		}
	}
	if config.ClientID == "" {
		config.ClientID = "goplusplus"
	}
	if config.DialTimeout <= 0 {
		config.DialTimeout = defaultKafkaDialTimeout
	}
	if config.DeliveryTimeout <= 0 {
		config.DeliveryTimeout = defaultKafkaDeliveryTimeout
	}
	if config.MaxMessageBytes <= 0 {
		config.MaxMessageBytes = defaultKafkaMaxMessageBytes
	}
	if config.MaxMessageBytes > int(^uint32(0)>>1) {
		return KafkaConfig{}, nil, errors.New("queue: Kafka maximum message size is too large")
	}
	if config.MaxBufferedBytes <= 0 {
		config.MaxBufferedBytes = 64 << 20
	}
	if config.RecordRetries <= 0 {
		config.RecordRetries = 10
	}
	options := []kgo.Opt{
		kgo.SeedBrokers(config.Brokers...), kgo.ClientID(config.ClientID), kgo.DialTimeout(config.DialTimeout),
	}
	if config.TLS != nil {
		tlsConfig := config.TLS.Clone()
		if tlsConfig.MinVersion == 0 {
			tlsConfig.MinVersion = tls.VersionTLS12
		}
		options = append(options, kgo.DialTLSConfig(tlsConfig))
	}
	mechanism, err := kafkaSASLMechanism(config.SASL, config.TLS != nil)
	if err != nil {
		return KafkaConfig{}, nil, err
	}
	if mechanism != nil {
		options = append(options, kgo.SASL(mechanism))
	}
	return config, options, nil
}

func kafkaSASLMechanism(config KafkaSASLConfig, tlsEnabled bool) (sasl.Mechanism, error) {
	mechanism := strings.ToLower(strings.TrimSpace(config.Mechanism))
	if mechanism == "" {
		if config.Username != "" || config.Password != "" {
			return nil, errors.New("queue: Kafka SASL mechanism is required with credentials")
		}
		return nil, nil
	}
	if !tlsEnabled {
		return nil, errors.New("queue: Kafka SASL requires TLS")
	}
	if config.Username == "" || config.Password == "" {
		return nil, errors.New("queue: Kafka SASL username and password are required")
	}
	switch mechanism {
	case "plain":
		return plain.Auth{User: config.Username, Pass: config.Password}.AsMechanism(), nil
	case "scram-sha-256":
		return scram.Auth{User: config.Username, Pass: config.Password}.AsSha256Mechanism(), nil
	case "scram-sha-512":
		return scram.Auth{User: config.Username, Pass: config.Password}.AsSha512Mechanism(), nil
	default:
		return nil, fmt.Errorf("queue: unsupported Kafka SASL mechanism %q", config.Mechanism)
	}
}

func validateKafkaTopic(topic string) error {
	if topic == "" || len(topic) > 249 || topic == "." || topic == ".." {
		return errors.New("queue: invalid Kafka topic")
	}
	for _, character := range topic {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || character == '.' || character == '_' || character == '-') {
			return errors.New("queue: invalid Kafka topic")
		}
	}
	return nil
}

func kafkaRecordHeaders(headers map[string][]byte) []kgo.RecordHeader {
	result := make([]kgo.RecordHeader, 0, len(headers))
	for key, value := range headers {
		result = append(result, kgo.RecordHeader{Key: key, Value: append([]byte(nil), value...)})
	}
	return result
}

func kafkaMessage(record *kgo.Record) KafkaMessage {
	headers := make(map[string][]byte, len(record.Headers))
	for _, header := range record.Headers {
		headers[header.Key] = append([]byte(nil), header.Value...)
	}
	return KafkaMessage{
		Topic: record.Topic, Key: append([]byte(nil), record.Key...), Value: append([]byte(nil), record.Value...),
		Headers: headers, Partition: record.Partition, Offset: record.Offset, Time: record.Timestamp,
	}
}

func waitForKafkaRetry(ctx context.Context, base time.Duration, attempt int) error {
	delay := base * time.Duration(1<<min(attempt, 8))
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func truncateKafkaError(err error) string {
	if err == nil {
		return "handler failed"
	}
	message := err.Error()
	if len(message) > 512 {
		return message[:512]
	}
	return message
}
