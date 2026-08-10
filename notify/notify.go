package notify

import (
	"context"
	"log/slog"
)

// EmailMessage represents an email notification payload.
type EmailMessage struct {
	To      string
	Subject string
	Body    string
	HTML    bool
}

// SMSMessage represents an SMS text notification payload.
type SMSMessage struct {
	ToPhone string
	Text    string
}

// SendEmail dispatches an email notification.
func SendEmail(ctx context.Context, msg EmailMessage) error {
	slog.Info("notify: Dispatched Email Notification",
		slog.String("to", msg.To),
		slog.String("subject", msg.Subject),
	)
	return nil
}

// SendSMS dispatches an SMS text message notification.
func SendSMS(ctx context.Context, msg SMSMessage) error {
	slog.Info("notify: Dispatched SMS Notification",
		slog.String("phone", msg.ToPhone),
		slog.String("text", msg.Text),
	)
	return nil
}
