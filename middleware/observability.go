package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/saifsilver/goplusplus"
)

type metricsStore struct {
	mu             sync.RWMutex
	totalRequests  uint64
	activeRequests int64
	statusCounts   map[int]uint64
	latencies      []float64
}

var globalMetrics = &metricsStore{
	statusCounts: make(map[int]uint64),
}

// Observability returns middleware that records Prometheus request counters, active connection gauges, and latency metrics.
func Observability() gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		start := time.Now()
		atomic.AddInt64(&globalMetrics.activeRequests, 1)
		atomic.AddUint64(&globalMetrics.totalRequests, 1)

		defer func() {
			atomic.AddInt64(&globalMetrics.activeRequests, -1)
			duration := time.Since(start).Seconds()

			globalMetrics.mu.Lock()
			globalMetrics.latencies = append(globalMetrics.latencies, duration)
			if len(globalMetrics.latencies) > 10000 {
				globalMetrics.latencies = globalMetrics.latencies[5000:]
			}
			globalMetrics.mu.Unlock()
		}()

		err := c.Next()

		status := http.StatusOK
		if err != nil {
			status = http.StatusInternalServerError
		}

		globalMetrics.mu.Lock()
		globalMetrics.statusCounts[status]++
		globalMetrics.mu.Unlock()

		return err
	}
}

// Prometheus returns a HandlerFunc that serves Prometheus metrics output on /metrics.
func Prometheus() gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		globalMetrics.mu.RLock()
		total := atomic.LoadUint64(&globalMetrics.totalRequests)
		active := atomic.LoadInt64(&globalMetrics.activeRequests)

		var statusLines []string
		for code, count := range globalMetrics.statusCounts {
			statusLines = append(statusLines, fmt.Sprintf("http_requests_total{status=\"%d\"} %d", code, count))
		}
		globalMetrics.mu.RUnlock()

		output := fmt.Sprintf(`# HELP http_requests_total Total number of HTTP requests processed.
# TYPE http_requests_total counter
http_requests_total %d
%s

# HELP http_active_requests Currently active HTTP requests.
# TYPE http_active_requests gauge
http_active_requests %d
`, total, strings.Join(statusLines, "\n"), active)

		return c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(output))
	}
}
