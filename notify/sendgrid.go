package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultSendGridEndpoint = "https://api.sendgrid.com/v3/mail/send"

// SendGridConfig defines credentials, sender identity, transport, and request bounds.
type SendGridConfig struct {
	APIKey            string
	FromEmail         string
	FromName          string
	Endpoint          string
	HTTPClient        *http.Client
	Timeout           time.Duration
	MaxBodyBytes      int
	AllowInsecureHTTP bool
}

// SendGridProvider sends email through the SendGrid HTTPS API.
type SendGridProvider struct {
	config   SendGridConfig
	endpoint *url.URL
	http     *http.Client
}

// NewSendGridProvider validates configuration without sending a request.
func NewSendGridProvider(config SendGridConfig) (*SendGridProvider, error) {
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, errors.New("notify: SendGrid API key is required")
	}
	if err := validateEmailAddress(config.FromEmail); err != nil {
		return nil, fmt.Errorf("notify: invalid SendGrid sender email: %w", err)
	}
	if strings.ContainsAny(config.FromName, "\r\n") || len(config.FromName) > 256 {
		return nil, errors.New("notify: invalid SendGrid sender name")
	}
	if config.Endpoint == "" {
		config.Endpoint = defaultSendGridEndpoint
	}
	endpoint, err := validateProviderURL(config.Endpoint, config.AllowInsecureHTTP)
	if err != nil {
		return nil, err
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{}
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultNotificationTimeout
	}
	if config.MaxBodyBytes <= 0 {
		config.MaxBodyBytes = defaultMaxEmailBodyBytes
	}
	return &SendGridProvider{config: config, endpoint: endpoint, http: config.HTTPClient}, nil
}

// SendEmail validates and submits one bounded SendGrid request.
func (provider *SendGridProvider) SendEmail(ctx context.Context, message EmailMessage) error {
	if provider == nil || provider.http == nil || provider.endpoint == nil {
		return ErrEmailProviderNotConfigured
	}
	if err := validateEmailMessage(message, provider.config.MaxBodyBytes); err != nil {
		return err
	}
	contentType := "text/plain"
	if message.HTML {
		contentType = "text/html"
	}
	payload := struct {
		Personalizations []struct {
			To []map[string]string `json:"to"`
		} `json:"personalizations"`
		From    map[string]string   `json:"from"`
		Subject string              `json:"subject"`
		Content []map[string]string `json:"content"`
	}{
		Personalizations: []struct {
			To []map[string]string `json:"to"`
		}{{To: []map[string]string{{"email": message.To}}}},
		From:    map[string]string{"email": provider.config.FromEmail, "name": provider.config.FromName},
		Subject: message.Subject,
		Content: []map[string]string{{"type": contentType, "value": message.Body}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("notify: encode SendGrid request: %w", err)
	}
	request, err := http.NewRequest(http.MethodPost, provider.endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("notify: build SendGrid request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+provider.config.APIKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	return executeNotificationRequest(ctx, provider.http, provider.config.Timeout, request)
}
