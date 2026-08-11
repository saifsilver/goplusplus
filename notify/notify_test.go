package notify_test

import (
	"context"
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
	if errEmail != nil {
		t.Errorf("SendEmail returned error: %v", errEmail)
	}

	errSMS := notify.SendSMS(ctx, notify.SMSMessage{
		ToPhone: "+15551234567",
		Text:    "Your OTP code is 123456",
	})
	if errSMS != nil {
		t.Errorf("SendSMS returned error: %v", errSMS)
	}
}
