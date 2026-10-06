package metrics

import (
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"flexiproxy/internal/models"
)

// MetricsRegistry maintains Prometheus counters, gauges, and latency metrics.
type MetricsRegistry struct {
	TotalRequests       uint64
	TotalErrors         uint64
	ActiveRequests      int64
	TotalFailovers      uint64
	TotalRetries        uint64
	RequestDurationBits uint64
}

var globalMetrics MetricsRegistry

func RecordRequestStart() {
	atomic.AddInt64(&globalMetrics.ActiveRequests, 1)
	atomic.AddUint64(&globalMetrics.TotalRequests, 1)
}

func RecordRequestComplete(duration time.Duration, statusCode int) {
	atomic.AddInt64(&globalMetrics.ActiveRequests, -1)
	if statusCode >= 500 {
		atomic.AddUint64(&globalMetrics.TotalErrors, 1)
	}

	// Atomic float addition using CAS loop
	sec := duration.Seconds()
	for {
		oldBits := atomic.LoadUint64(&globalMetrics.RequestDurationBits)
		newVal := math.Float64frombits(oldBits) + sec
		newBits := math.Float64bits(newVal)
		if atomic.CompareAndSwapUint64(&globalMetrics.RequestDurationBits, oldBits, newBits) {
			break
		}
	}
}

func RecordFailover() {
	atomic.AddUint64(&globalMetrics.TotalFailovers, 1)
}

func RecordRetry() {
	atomic.AddUint64(&globalMetrics.TotalRetries, 1)
}

// Handler returns an http.HandlerFunc exporting Prometheus text format metrics.
func Handler(backends []*models.Backend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		var sb strings.Builder

		sb.WriteString("# HELP flexiproxy_requests_total Total number of HTTP requests processed.\n")
		sb.WriteString("# TYPE flexiproxy_requests_total counter\n")
		sb.WriteString(fmt.Sprintf("flexiproxy_requests_total %d\n\n", atomic.LoadUint64(&globalMetrics.TotalRequests)))

		sb.WriteString("# HELP flexiproxy_request_errors_total Total number of 5xx server errors.\n")
		sb.WriteString("# TYPE flexiproxy_request_errors_total counter\n")
		sb.WriteString(fmt.Sprintf("flexiproxy_request_errors_total %d\n\n", atomic.LoadUint64(&globalMetrics.TotalErrors)))

		sb.WriteString("# HELP flexiproxy_active_requests Current active requests in flight.\n")
		sb.WriteString("# TYPE flexiproxy_active_requests gauge\n")
		sb.WriteString(fmt.Sprintf("flexiproxy_active_requests %d\n\n", atomic.LoadInt64(&globalMetrics.ActiveRequests)))

		sb.WriteString("# HELP flexiproxy_failovers_total Total count of failover events executed.\n")
		sb.WriteString("# TYPE flexiproxy_failovers_total counter\n")
		sb.WriteString(fmt.Sprintf("flexiproxy_failovers_total %d\n\n", atomic.LoadUint64(&globalMetrics.TotalFailovers)))

		sb.WriteString("# HELP flexiproxy_retries_total Total count of request retries executed.\n")
		sb.WriteString("# TYPE flexiproxy_retries_total counter\n")
		sb.WriteString(fmt.Sprintf("flexiproxy_retries_total %d\n\n", atomic.LoadUint64(&globalMetrics.TotalRetries)))

		// Per-Backend Metrics
		sb.WriteString("# HELP flexiproxy_backend_health Active health state of backend (1=UP, 0=DOWN).\n")
		sb.WriteString("# TYPE flexiproxy_backend_health gauge\n")
		for _, b := range backends {
			val := 0
			if b.IsHealthy() {
				val = 1
			}
			sb.WriteString(fmt.Sprintf("flexiproxy_backend_health{backend_id=\"%s\",address=\"%s\"} %d\n", b.ID, b.Address, val))
		}
		sb.WriteString("\n")

		sb.WriteString("# HELP flexiproxy_backend_circuit_breaker_state Circuit breaker state (0=CLOSED, 1=OPEN, 2=HALF_OPEN).\n")
		sb.WriteString("# TYPE flexiproxy_backend_circuit_breaker_state gauge\n")
		for _, b := range backends {
			sb.WriteString(fmt.Sprintf("flexiproxy_backend_circuit_breaker_state{backend_id=\"%s\"} %d\n", b.ID, b.CircuitState.Load()))
		}
		sb.WriteString("\n")

		sb.WriteString("# HELP flexiproxy_backend_active_connections Current active connections per backend.\n")
		sb.WriteString("# TYPE flexiproxy_backend_active_connections gauge\n")
		for _, b := range backends {
			sb.WriteString(fmt.Sprintf("flexiproxy_backend_active_connections{backend_id=\"%s\"} %d\n", b.ID, atomic.LoadInt64(&b.ActiveConns)))
		}
		sb.WriteString("\n")

		sb.WriteString("# HELP flexiproxy_backend_latency_seconds Average response latency in seconds per backend.\n")
		sb.WriteString("# TYPE flexiproxy_backend_latency_seconds gauge\n")
		for _, b := range backends {
			sb.WriteString(fmt.Sprintf("flexiproxy_backend_latency_seconds{backend_id=\"%s\"} %.4f\n", b.ID, b.GetLatencyMs()/1000.0))
		}
		sb.WriteString("\n")

		_, _ = w.Write([]byte(sb.String()))
	}
}
