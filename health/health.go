package health

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	gpp "github.com/saifsilver/goplusplus"
)

type CheckFunc func(context.Context) error

type ContextPinger interface {
	PingContext(context.Context) error
}

type CheckerOption func(*Checker)

func WithTimeout(timeout time.Duration) CheckerOption {
	return func(checker *Checker) {
		if timeout > 0 {
			checker.timeout = timeout
		}
	}
}

type Checker struct {
	mu              sync.RWMutex
	readinessChecks map[string]CheckFunc
	timeout         time.Duration
}

func NewChecker(options ...CheckerOption) *Checker {
	checker := &Checker{readinessChecks: make(map[string]CheckFunc), timeout: 2 * time.Second}
	for _, option := range options {
		if option != nil {
			option(checker)
		}
	}
	return checker
}

// AddReadinessCheck preserves the historical replace-on-duplicate behavior.
// New applications can use RegisterReadinessCheck to reject duplicates.
func (checker *Checker) AddReadinessCheck(name string, check CheckFunc) {
	checker.mu.Lock()
	checker.readinessChecks[name] = check
	checker.mu.Unlock()
}

func (checker *Checker) RegisterReadinessCheck(name string, check CheckFunc) error {
	name = strings.TrimSpace(name)
	if name == "" || check == nil {
		return errors.New("health: check name and function are required")
	}
	checker.mu.Lock()
	defer checker.mu.Unlock()
	if _, exists := checker.readinessChecks[name]; exists {
		return fmt.Errorf("health: readiness check %q already exists", name)
	}
	checker.readinessChecks[name] = check
	return nil
}

func SQLReadiness(pinger ContextPinger) CheckFunc {
	return func(ctx context.Context) error {
		if pinger == nil {
			return errors.New("SQL pinger is nil")
		}
		return pinger.PingContext(ctx)
	}
}

func (checker *Checker) Liveness() gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		return c.JSON(http.StatusOK, gpp.H{"status": "UP", "checker": "liveness"})
	}
}

type namedCheck struct {
	name string
	fn   CheckFunc
}

type checkResult struct {
	name string
	err  error
}

func (checker *Checker) snapshot() []namedCheck {
	checker.mu.RLock()
	checks := make([]namedCheck, 0, len(checker.readinessChecks))
	for name, check := range checker.readinessChecks {
		checks = append(checks, namedCheck{name: name, fn: check})
	}
	checker.mu.RUnlock()
	slices.SortFunc(checks, func(a, b namedCheck) int { return cmp.Compare(a.name, b.name) })
	return checks
}

func (checker *Checker) Readiness() gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		checks := checker.snapshot()
		results := make(chan checkResult, len(checks))
		for _, check := range checks {
			go runCheck(c.Request.Context(), checker.timeout, check, results)
		}
		details := make(map[string]string, len(checks))
		allHealthy := true
		for range checks {
			result := <-results
			if result.err != nil {
				allHealthy = false
				details[result.name] = "DOWN"
				slog.Error("health: readiness dependency failed", slog.String("dependency", result.name),
					slog.String("request_id", c.RequestID()), slog.Any("error", result.err))
			} else {
				details[result.name] = "UP"
			}
		}
		status, state := http.StatusOK, "UP"
		if !allHealthy {
			status, state = http.StatusServiceUnavailable, "DOWN"
		}
		return c.JSON(status, gpp.H{"status": state, "checks": details, "checker": "readiness"})
	}
}

func runCheck(parent context.Context, timeout time.Duration, check namedCheck, results chan<- checkResult) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	result := checkResult{name: check.name}
	done := make(chan error, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				done <- errors.New("dependency check panicked")
			}
		}()
		if check.fn == nil {
			done <- errors.New("dependency check is nil")
			return
		}
		done <- check.fn(ctx)
	}()
	select {
	case result.err = <-done:
	case <-ctx.Done():
		result.err = ctx.Err()
	}
	results <- result
}
