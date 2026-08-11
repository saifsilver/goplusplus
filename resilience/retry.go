package resilience

import (
	"context"
	"crypto/rand"
	"math"
	"math/big"
	"time"
)

type RetryConfig struct {
	MaxAttempts   int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	BackoffFactor float64
	Retryable     func(err error) bool
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      3 * time.Second,
		BackoffFactor: 2.0,
	}
}

// Retry executes an operation function with exponential backoff and jitter on failure.
func Retry(ctx context.Context, cfg RetryConfig, fn func(ctx context.Context) error) error {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.InitialDelay <= 0 {
		cfg.InitialDelay = 100 * time.Millisecond
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = 3 * time.Second
	}
	if cfg.BackoffFactor <= 0 {
		cfg.BackoffFactor = 2.0
	}

	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := fn(ctx)
		if err == nil {
			return nil
		}

		lastErr = err
		if cfg.Retryable != nil && !cfg.Retryable(err) {
			return err
		}

		if attempt == cfg.MaxAttempts {
			break
		}

		jitter := addJitter(delay)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(jitter):
		}

		delay = time.Duration(float64(delay) * cfg.BackoffFactor)
		if delay > cfg.MaxDelay {
			delay = cfg.MaxDelay
		}
	}

	return lastErr
}

func addJitter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	nBig, err := rand.Int(rand.Reader, big.NewInt(int64(d/2)))
	if err != nil {
		return d
	}
	jitter := time.Duration(nBig.Int64())
	if math.Mod(float64(jitter), 2) == 0 {
		return d + jitter
	}
	return d - jitter
}
