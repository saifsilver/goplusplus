package pubsub

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const defaultRabbitMQMaxPayload = 1 << 20

var ErrRabbitMQNotConfigured = errors.New("pubsub: RabbitMQ provider is not configured")

type RabbitMQConfig struct {
	URL               string
	Exchange          string
	ExchangeType      string
	TLS               *tls.Config
	ConnectionTimeout time.Duration
	ConfirmTimeout    time.Duration
	Heartbeat         time.Duration
	MaxPayloadBytes   int
	Prefetch          int
	QuorumQueues      bool
}

type RabbitMQBus struct {
	config         RabbitMQConfig
	connection     *amqp.Connection
	publishChannel *amqp.Channel
	publishReturns <-chan amqp.Return
	publishMu      sync.Mutex
	closeOnce      sync.Once
	closeErr       error
}

func NewRabbitMQBus(ctx context.Context, config RabbitMQConfig) (*RabbitMQBus, error) {
	config, err := normalizeRabbitMQConfig(config)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: config.ConnectionTimeout}
	connection, err := amqp.DialConfig(config.URL, amqp.Config{
		Heartbeat: config.Heartbeat, TLSClientConfig: cloneRabbitTLS(config.TLS),
		Dial: func(network, address string) (net.Conn, error) { return dialer.DialContext(ctx, network, address) },
	})
	if err != nil {
		return nil, fmt.Errorf("pubsub: connect RabbitMQ: %w", err)
	}
	channel, err := connection.Channel()
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("pubsub: open RabbitMQ publish channel: %w", err)
	}
	if err := declareRabbitExchange(channel, config); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return nil, err
	}
	if err := channel.Confirm(false); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return nil, fmt.Errorf("pubsub: enable RabbitMQ publisher confirms: %w", err)
	}
	returns := channel.NotifyReturn(make(chan amqp.Return, 1))
	return &RabbitMQBus{
		config: config, connection: connection, publishChannel: channel, publishReturns: returns,
	}, nil
}

func (bus *RabbitMQBus) Publish(
	ctx context.Context, exchange, routingKey string, payload []byte,
) error {
	if bus == nil || bus.connection == nil || bus.publishChannel == nil {
		return ErrRabbitMQNotConfigured
	}
	if exchange == "" {
		exchange = bus.config.Exchange
	}
	if exchange != bus.config.Exchange {
		return errors.New("pubsub: RabbitMQ publish exchange does not match configured exchange")
	}
	if err := validateRabbitName(routingKey, "routing key"); err != nil {
		return err
	}
	if len(payload) > bus.config.MaxPayloadBytes {
		return fmt.Errorf("pubsub: RabbitMQ payload exceeds %d bytes", bus.config.MaxPayloadBytes)
	}
	messageID, err := newRabbitMessageID()
	if err != nil {
		return err
	}
	bus.publishMu.Lock()
	defer bus.publishMu.Unlock()
	confirmation, err := bus.publishChannel.PublishWithDeferredConfirmWithContext(
		ctx, exchange, routingKey, true, false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent, ContentType: "application/octet-stream", Body: payload,
			MessageId: messageID, Timestamp: time.Now().UTC(), AppId: "goplusplus",
		},
	)
	if err != nil {
		return fmt.Errorf("pubsub: publish RabbitMQ message: %w", err)
	}
	confirmCtx, cancel := context.WithTimeout(ctx, bus.config.ConfirmTimeout)
	defer cancel()
	acknowledged, err := confirmation.WaitContext(confirmCtx)
	if err != nil {
		return fmt.Errorf("pubsub: wait for RabbitMQ publisher confirm: %w", err)
	}
	if !acknowledged {
		return errors.New("pubsub: RabbitMQ negatively acknowledged message")
	}
	select {
	case returned := <-bus.publishReturns:
		return fmt.Errorf("pubsub: RabbitMQ message was unroutable: %s", returned.ReplyText)
	default:
		return nil
	}
}

// Subscribe declares a durable queue and blocks until ctx cancellation or an
// AMQP/handler error. Failed handler messages are negatively acknowledged and requeued.
func (bus *RabbitMQBus) Subscribe(
	ctx context.Context,
	queueName, bindingKey string,
	handler func(context.Context, []byte) error,
) error {
	if bus == nil || bus.connection == nil {
		return ErrRabbitMQNotConfigured
	}
	if handler == nil {
		return errors.New("pubsub: RabbitMQ subscriber handler is required")
	}
	if err := validateRabbitName(queueName, "queue"); err != nil {
		return err
	}
	if err := validateRabbitName(bindingKey, "binding key"); err != nil {
		return err
	}
	channel, err := bus.connection.Channel()
	if err != nil {
		return fmt.Errorf("pubsub: open RabbitMQ consumer channel: %w", err)
	}
	defer channel.Close()
	if err := declareRabbitExchange(channel, bus.config); err != nil {
		return err
	}
	queueArguments := amqp.Table(nil)
	if bus.config.QuorumQueues {
		queueArguments = amqp.Table{amqp.QueueTypeArg: amqp.QueueTypeQuorum}
	}
	if _, err := channel.QueueDeclare(queueName, true, false, false, false, queueArguments); err != nil {
		return fmt.Errorf("pubsub: declare RabbitMQ queue: %w", err)
	}
	if err := channel.QueueBind(queueName, bindingKey, bus.config.Exchange, false, nil); err != nil {
		return fmt.Errorf("pubsub: bind RabbitMQ queue: %w", err)
	}
	if err := channel.Qos(bus.config.Prefetch, 0, false); err != nil {
		return fmt.Errorf("pubsub: configure RabbitMQ prefetch: %w", err)
	}
	deliveries, err := channel.ConsumeWithContext(ctx, queueName, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("pubsub: consume RabbitMQ queue: %w", err)
	}
	for delivery := range deliveries {
		if len(delivery.Body) > bus.config.MaxPayloadBytes {
			if err := delivery.Nack(false, false); err != nil {
				return fmt.Errorf("pubsub: reject oversized RabbitMQ message: %w", err)
			}
			continue
		}
		if err := handler(ctx, append([]byte(nil), delivery.Body...)); err != nil {
			if nackErr := delivery.Nack(false, true); nackErr != nil {
				return errors.Join(err, fmt.Errorf("pubsub: nack RabbitMQ message: %w", nackErr))
			}
			return fmt.Errorf("pubsub: RabbitMQ subscriber handler: %w", err)
		}
		if err := delivery.Ack(false); err != nil {
			return fmt.Errorf("pubsub: ack RabbitMQ message: %w", err)
		}
	}
	if ctx.Err() != nil {
		return nil
	}
	return errors.New("pubsub: RabbitMQ delivery channel closed")
}

func (bus *RabbitMQBus) Close() error {
	if bus == nil {
		return nil
	}
	bus.closeOnce.Do(func() {
		bus.publishMu.Lock()
		defer bus.publishMu.Unlock()
		if bus.publishChannel != nil {
			bus.closeErr = errors.Join(bus.closeErr, bus.publishChannel.Close())
		}
		if bus.connection != nil {
			bus.closeErr = errors.Join(bus.closeErr, bus.connection.Close())
		}
	})
	return bus.closeErr
}

func normalizeRabbitMQConfig(config RabbitMQConfig) (RabbitMQConfig, error) {
	parsed, err := url.Parse(config.URL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "amqp" && parsed.Scheme != "amqps") {
		return RabbitMQConfig{}, errors.New("pubsub: valid amqp:// or amqps:// RabbitMQ URL is required")
	}
	if config.TLS != nil && parsed.Scheme != "amqps" {
		return RabbitMQConfig{}, errors.New("pubsub: RabbitMQ TLS configuration requires amqps://")
	}
	if err := validateRabbitName(config.Exchange, "exchange"); err != nil {
		return RabbitMQConfig{}, err
	}
	if config.ExchangeType == "" {
		config.ExchangeType = "topic"
	}
	switch config.ExchangeType {
	case "direct", "fanout", "topic", "headers":
	default:
		return RabbitMQConfig{}, errors.New("pubsub: invalid RabbitMQ exchange type")
	}
	if config.ConnectionTimeout <= 0 {
		config.ConnectionTimeout = 10 * time.Second
	}
	if config.ConfirmTimeout <= 0 {
		config.ConfirmTimeout = 10 * time.Second
	}
	if config.Heartbeat <= 0 {
		config.Heartbeat = 10 * time.Second
	}
	if config.MaxPayloadBytes <= 0 {
		config.MaxPayloadBytes = defaultRabbitMQMaxPayload
	}
	if config.Prefetch <= 0 {
		config.Prefetch = 10
	}
	return config, nil
}

func declareRabbitExchange(channel *amqp.Channel, config RabbitMQConfig) error {
	if err := channel.ExchangeDeclare(config.Exchange, config.ExchangeType, true, false, false, false, nil); err != nil {
		return fmt.Errorf("pubsub: declare RabbitMQ exchange: %w", err)
	}
	return nil
}

func validateRabbitName(value, field string) error {
	if value == "" || len(value) > 255 || strings.ContainsAny(value, "\r\n\t ") {
		return fmt.Errorf("pubsub: invalid RabbitMQ %s", field)
	}
	return nil
}

func cloneRabbitTLS(config *tls.Config) *tls.Config {
	if config == nil {
		return nil
	}
	clone := config.Clone()
	if clone.MinVersion == 0 {
		clone.MinVersion = tls.VersionTLS12
	}
	return clone
}

func newRabbitMessageID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("pubsub: generate RabbitMQ message ID: %w", err)
	}
	return hex.EncodeToString(random[:]), nil
}
