package health

import (
	"context"
	"net/http"
	"sync"

	"github.com/saifsilver/goplusplus"
)

// CheckFunc defines a health check function signature (e.g. database, redis ping).
type CheckFunc func(ctx context.Context) error

// Checker manages Kubernetes liveness and readiness health probes.
type Checker struct {
	mu            sync.RWMutex
	readinessChecks map[string]CheckFunc
}

// NewChecker initializes a new health checker instance.
func NewChecker() *Checker {
	return &Checker{
		readinessChecks: make(map[string]CheckFunc),
	}
}

// AddReadinessCheck registers a health check dependency (e.g., "database", "cache").
func (hc *Checker) AddReadinessCheck(name string, check CheckFunc) {
	hc.mu.Lock()
	hc.readinessChecks[name] = check
	hc.mu.Unlock()
}

// Liveness returns a HandlerFunc for Kubernetes liveness probe (/healthz/liveness).
func (hc *Checker) Liveness() gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		return c.JSON(http.StatusOK, gpp.H{
			"status":  "UP",
			"checker": "liveness",
		})
	}
}

// Readiness returns a HandlerFunc for Kubernetes readiness probe (/healthz/readiness).
func (hc *Checker) Readiness() gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		hc.mu.RLock()
		defer hc.mu.RUnlock()

		ctx := c.Request.Context()
		details := make(map[string]string)
		allHealthy := true

		for name, check := range hc.readinessChecks {
			if err := check(ctx); err != nil {
				details[name] = "DOWN: " + err.Error()
				allHealthy = false
			} else {
				details[name] = "UP"
			}
		}

		status := http.StatusOK
		statusStr := "UP"
		if !allHealthy {
			status = http.StatusServiceUnavailable
			statusStr = "DOWN"
		}

		return c.JSON(status, gpp.H{
			"status":  statusStr,
			"checks":  details,
			"checker": "readiness",
		})
	}
}
