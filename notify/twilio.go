package notify

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const defaultTwilioEndpoint = "https://api.twilio.com"

var (
	twilioAccountPattern = regexp.MustCompile(`^AC[0-9a-fA-F]{32}$`)
	twilioServicePattern = regexp.MustCompile(`^MG[0-9a-fA-F]{32}$`)
	e164Pattern          = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)
)

type TwilioConfig struct {
	AccountSID          string
	AuthToken           string
	FromPhone           string
	MessagingServiceSID string
	Endpoint            string
	HTTPClient          *http.Client
	Timeout             time.Duration
	MaxTextBytes        int
	AllowInsecureHTTP   bool
}

type TwilioProvider struct {
	config   TwilioConfig
	endpoint *url.URL
	http     *http.Client
}

func NewTwilioProvider(config TwilioConfig) (*TwilioProvider, error) {
	if !twilioAccountPattern.MatchString(config.AccountSID) {
		return nil, errors.New("notify: valid Twilio account SID is required")
	}
	if strings.TrimSpace(config.AuthToken) == "" {
		return nil, errors.New("notify: Twilio auth token is required")
	}
	usesPhone := config.FromPhone != ""
	usesService := config.MessagingServiceSID != ""
	if usesPhone == usesService {
		return nil, errors.New("notify: configure exactly one Twilio sender phone or messaging service SID")
	}
	if usesPhone && !e164Pattern.MatchString(config.FromPhone) {
		return nil, errors.New("notify: Twilio sender phone must use E.164 format")
	}
	if usesService && !twilioServicePattern.MatchString(config.MessagingServiceSID) {
		return nil, errors.New("notify: invalid Twilio messaging service SID")
	}
	if config.Endpoint == "" {
		config.Endpoint = defaultTwilioEndpoint
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
	if config.MaxTextBytes <= 0 {
		config.MaxTextBytes = defaultMaxSMSBytes
	}
	return &TwilioProvider{config: config, endpoint: endpoint, http: config.HTTPClient}, nil
}

func (provider *TwilioProvider) SendSMS(ctx context.Context, message SMSMessage) error {
	if provider == nil || provider.http == nil || provider.endpoint == nil {
		return ErrSMSProviderNotConfigured
	}
	if !e164Pattern.MatchString(message.ToPhone) {
		return errors.New("notify: SMS recipient phone must use E.164 format")
	}
	if message.Text == "" || len(message.Text) > provider.config.MaxTextBytes {
		return fmt.Errorf("notify: SMS text must contain 1 to %d bytes", provider.config.MaxTextBytes)
	}
	form := url.Values{"To": {message.ToPhone}, "Body": {message.Text}}
	if provider.config.FromPhone != "" {
		form.Set("From", provider.config.FromPhone)
	} else {
		form.Set("MessagingServiceSid", provider.config.MessagingServiceSID)
	}
	endpoint := *provider.endpoint
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/") + "/2010-04-01/Accounts/" + provider.config.AccountSID + "/Messages.json"
	request, err := http.NewRequest(http.MethodPost, endpoint.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("notify: build Twilio request: %w", err)
	}
	request.SetBasicAuth(provider.config.AccountSID, provider.config.AuthToken)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	return executeNotificationRequest(ctx, provider.http, provider.config.Timeout, request)
}
