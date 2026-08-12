package notify

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
)

const (
	defaultMaxEmailBodyBytes = 1 << 20
	defaultMaxSMSBytes       = 1600
)

var (
	// ErrEmailProviderNotConfigured indicates that email delivery is unavailable.
	ErrEmailProviderNotConfigured = errors.New("notify: email provider is not configured")
	// ErrSMSProviderNotConfigured indicates that SMS delivery is unavailable.
	ErrSMSProviderNotConfigured = errors.New("notify: SMS provider is not configured")
)

// EmailMessage is a bounded email delivery request.
type EmailMessage struct {
	To      string
	Subject string
	Body    string
	HTML    bool
}

// SMSMessage is a bounded SMS delivery request.
type SMSMessage struct {
	ToPhone string
	Text    string
}

// EmailProvider sends validated email messages.
type EmailProvider interface {
	SendEmail(context.Context, EmailMessage) error
}

// SMSProvider sends validated SMS messages.
type SMSProvider interface {
	SendSMS(context.Context, SMSMessage) error
}

// Client routes notifications through explicitly configured providers.
type Client struct {
	email EmailProvider
	sms   SMSProvider
}

// NewClient constructs a notification router with at least one provider.
func NewClient(email EmailProvider, sms SMSProvider) (*Client, error) {
	if email == nil && sms == nil {
		return nil, errors.New("notify: at least one provider is required")
	}
	return &Client{email: email, sms: sms}, nil
}

// SendEmail delegates to the configured email provider.
func (client *Client) SendEmail(ctx context.Context, message EmailMessage) error {
	if client == nil || client.email == nil {
		return ErrEmailProviderNotConfigured
	}
	return client.email.SendEmail(ctx, message)
}

// SendSMS delegates to the configured SMS provider.
func (client *Client) SendSMS(ctx context.Context, message SMSMessage) error {
	if client == nil || client.sms == nil {
		return ErrSMSProviderNotConfigured
	}
	return client.sms.SendSMS(ctx, message)
}

// SendEmail is retained as a fail-closed compatibility function. Construct a
// Client instead so credentials and provider lifecycle remain explicit.
func SendEmail(ctx context.Context, message EmailMessage) error {
	return ErrEmailProviderNotConfigured
}

// SendSMS is retained as a fail-closed compatibility function. Construct a
// Client instead so credentials and provider lifecycle remain explicit.
func SendSMS(ctx context.Context, message SMSMessage) error {
	return ErrSMSProviderNotConfigured
}

func validateEmailMessage(message EmailMessage, maxBodyBytes int) error {
	if err := validateEmailAddress(message.To); err != nil {
		return fmt.Errorf("notify: invalid recipient email: %w", err)
	}
	if strings.TrimSpace(message.Subject) == "" || len(message.Subject) > 998 || strings.ContainsAny(message.Subject, "\r\n") {
		return errors.New("notify: email subject must contain 1 to 998 bytes without newlines")
	}
	if message.Body == "" || len(message.Body) > maxBodyBytes {
		return fmt.Errorf("notify: email body must contain 1 to %d bytes", maxBodyBytes)
	}
	return nil
}

func validateEmailAddress(address string) error {
	parsed, err := mail.ParseAddress(address)
	if err != nil || parsed.Address != address || strings.ContainsAny(address, "\r\n") {
		return errors.New("valid mailbox address required")
	}
	return nil
}
