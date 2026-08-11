package pubsub

import (
	"context"
	"crypto/tls"
	"testing"
)

func TestRabbitMQConfigurationValidation(t *testing.T) {
	tests := []RabbitMQConfig{
		{},
		{URL: "http://localhost", Exchange: "events"},
		{URL: "amqp://localhost", Exchange: ""},
		{URL: "amqp://localhost", Exchange: "bad exchange"},
		{URL: "amqp://localhost", Exchange: "events", TLS: &tls.Config{}},
		{URL: "amqp://localhost", Exchange: "events", ExchangeType: "unknown"},
	}
	for _, config := range tests {
		if _, err := NewRabbitMQBus(context.Background(), config); err == nil {
			t.Fatalf("expected configuration to fail: %#v", config)
		}
	}
}

func TestRabbitMQTLSMinimum(t *testing.T) {
	config := cloneRabbitTLS(&tls.Config{})
	if config.MinVersion != tls.VersionTLS12 {
		t.Fatalf("minimum TLS version = %d", config.MinVersion)
	}
}
