package notify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const defaultNotificationTimeout = 10 * time.Second

func validateProviderURL(rawURL string, allowInsecureHTTP bool) (*url.URL, error) {
	endpoint, err := url.Parse(rawURL)
	if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, errors.New("notify: invalid provider endpoint")
	}
	if endpoint.Scheme != "https" && !(allowInsecureHTTP && endpoint.Scheme == "http") {
		return nil, errors.New("notify: provider endpoint must use HTTPS")
	}
	return endpoint, nil
}

func executeNotificationRequest(
	ctx context.Context, client *http.Client, timeout time.Duration, request *http.Request,
) error {
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	response, err := client.Do(request.WithContext(requestCtx))
	if err != nil {
		return fmt.Errorf("notify: provider request: %w", err)
	}
	_, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	closeErr := response.Body.Close()
	if readErr != nil {
		return fmt.Errorf("notify: read provider response: %w", readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("notify: close provider response: %w", closeErr)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("notify: provider returned status %d", response.StatusCode)
	}
	return nil
}
