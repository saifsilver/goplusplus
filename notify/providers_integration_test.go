package notify_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/saifsilver/goplusplus/notify"
)

var testAccountSID = "AC" + strings.Repeat("0", 32)

func TestSendGridAndTwilioProvidersIntegration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v3/mail/send":
			if request.Header.Get("Authorization") != "Bearer sendgrid-key" {
				t.Errorf("SendGrid authorization = %q", request.Header.Get("Authorization"))
			}
			var payload struct {
				Subject string `json:"subject"`
				Content []struct {
					Type  string `json:"type"`
					Value string `json:"value"`
				} `json:"content"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Errorf("decode SendGrid payload: %v", err)
			}
			if payload.Subject != "Welcome" || len(payload.Content) != 1 || payload.Content[0].Type != "text/html" {
				t.Errorf("unexpected SendGrid payload: %#v", payload)
			}
			writer.WriteHeader(http.StatusAccepted)
		case "/2010-04-01/Accounts/" + testAccountSID + "/Messages.json":
			username, password, ok := request.BasicAuth()
			if !ok || username != testAccountSID || password != "twilio-token" {
				t.Error("invalid Twilio basic authentication")
			}
			if err := request.ParseForm(); err != nil {
				t.Errorf("parse Twilio form: %v", err)
			}
			if request.Form.Get("To") != "+15551234567" || request.Form.Get("From") != "+15557654321" || request.Form.Get("Body") != "OTP 123456" {
				t.Errorf("unexpected Twilio form: %#v", request.Form)
			}
			writer.WriteHeader(http.StatusCreated)
		default:
			http.Error(writer, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	email, err := notify.NewSendGridProvider(notify.SendGridConfig{
		APIKey: "sendgrid-key", FromEmail: "sender@example.com", FromName: "Example",
		Endpoint: server.URL + "/v3/mail/send", AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	sms, err := notify.NewTwilioProvider(notify.TwilioConfig{
		AccountSID: testAccountSID, AuthToken: "twilio-token", FromPhone: "+15557654321",
		Endpoint: server.URL, AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := notify.NewClient(email, sms)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SendEmail(context.Background(), notify.EmailMessage{
		To: "user@example.com", Subject: "Welcome", Body: "<b>Hello</b>", HTML: true,
	}); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if err := client.SendSMS(context.Background(), notify.SMSMessage{
		ToPhone: "+15551234567", Text: "OTP 123456",
	}); err != nil {
		t.Fatalf("SendSMS: %v", err)
	}
}

func TestNotificationProviderValidationAndRedactedErrors(t *testing.T) {
	if _, err := notify.NewSendGridProvider(notify.SendGridConfig{}); err == nil {
		t.Fatal("expected empty SendGrid config to fail")
	}
	if _, err := notify.NewTwilioProvider(notify.TwilioConfig{}); err == nil {
		t.Fatal("expected empty Twilio config to fail")
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "credential-sensitive provider detail", http.StatusUnauthorized)
	}))
	defer server.Close()
	email, err := notify.NewSendGridProvider(notify.SendGridConfig{
		APIKey: "key", FromEmail: "sender@example.com",
		Endpoint: server.URL, AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = email.SendEmail(context.Background(), notify.EmailMessage{
		To: "user@example.com", Subject: "subject", Body: "body",
	})
	if err == nil || err.Error() != "notify: provider returned status 401" {
		t.Fatalf("unexpected provider error: %v", err)
	}
}
