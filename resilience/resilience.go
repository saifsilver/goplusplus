package resilience

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/saifsilver/goplusplus"
)

// CircuitState identifies the current circuit-breaker state.
type CircuitState int32

const (
	// StateClosed permits calls and records failures.
	StateClosed CircuitState = iota
	// StateHalfOpen permits a trial call after the reset timeout.
	StateHalfOpen
	// StateOpen rejects calls until the reset timeout elapses.
	StateOpen
)

// CircuitBreaker Config holds state thresholds.
type CircuitBreakerConfig struct {
	FailureThreshold int
	ResetTimeout     time.Duration
}

// CircuitBreaker traps cascading system failures when downstream APIs degrade.
type CircuitBreaker struct {
	mu           sync.RWMutex
	state        CircuitState
	failures     int
	cfg          CircuitBreakerConfig
	lastStateChg time.Time
}

// NewCircuitBreaker creates a circuit breaker instance.
func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.ResetTimeout <= 0 {
		cfg.ResetTimeout = 10 * time.Second
	}
	return &CircuitBreaker{
		state: StateClosed,
		cfg:   cfg,
	}
}

// Execute executes a function protected by the circuit breaker.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.mu.Lock()
	if cb.state == StateOpen {
		if time.Since(cb.lastStateChg) > cb.cfg.ResetTimeout {
			cb.state = StateHalfOpen
			cb.lastStateChg = time.Now()
		} else {
			cb.mu.Unlock()
			return errors.New("circuit breaker is OPEN; downstream requests rejected to prevent system collapse")
		}
	}
	cb.mu.Unlock()

	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failures++
		if cb.failures >= cb.cfg.FailureThreshold {
			cb.state = StateOpen
			cb.lastStateChg = time.Now()
		}
		return err
	}

	if cb.state == StateHalfOpen {
		cb.state = StateClosed
		cb.failures = 0
		cb.lastStateChg = time.Now()
	}
	return nil
}

// AdaptiveLimiter dynamically limits concurrency based on Little's Law latency drift.
type AdaptiveLimiter struct {
	activeRequests int64
	maxLimit       int64
}

// NewAdaptiveLimiter creates a new adaptive concurrency limiter instance.
func NewAdaptiveLimiter(maxLimit int64) *AdaptiveLimiter {
	if maxLimit <= 0 {
		maxLimit = 1000
	}
	return &AdaptiveLimiter{maxLimit: maxLimit}
}

// Middleware returns adaptive concurrency limiting middleware.
func (al *AdaptiveLimiter) Middleware() gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		current := atomic.AddInt64(&al.activeRequests, 1)
		defer atomic.AddInt64(&al.activeRequests, -1)

		if current > al.maxLimit {
			return gpp.ErrInternal(fmt.Sprintf("Adaptive Concurrency Limit Exceeded (Active: %d > Max: %d)", current, al.maxLimit))
		}
		return c.Next()
	}
}
