package notify_test

import (
	"context"
	"errors"
	"testing"

	"github.com/saifsilver/goplusplus/notify"
)

func TestNotifySendEmailAndSMS(t *testing.T) {
	ctx := context.Background()

	errEmail := notify.SendEmail(ctx, notify.EmailMessage{
		To:      "user@example.com",
		Subject: "Welcome!",
		Body:    "Hello world",
		HTML:    true,
	})
	if !errors.Is(errEmail, notify.ErrEmailProviderNotConfigured) {
		t.Errorf("SendEmail error = %v", errEmail)
	}

	errSMS := notify.SendSMS(ctx, notify.SMSMessage{
		ToPhone: "+15551234567",
		Text:    "Your OTP code is 123456",
	})
	if !errors.Is(errSMS, notify.ErrSMSProviderNotConfigured) {
		t.Errorf("SendSMS error = %v", errSMS)
	}
}
