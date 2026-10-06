package models

import (
	"net/http/httputil"
	"net/url"
	"sync/atomic"
	"time"
)

// Backend represents a target upstream HTTP server managed by FlexiProxy.
type Backend struct {
	ID      string   `json:"id"`
	URL     *url.URL `json:"url"`
	Weight  int      `json:"weight"`
	Address string   `json:"address"`

	// Dynamic status flags
	Healthy      atomic.Bool  `json:"healthy"`
	CircuitState atomic.Int32 `json:"circuit_state"` // 0=CLOSED, 1=OPEN, 2=HALF_OPEN

	// Concurrency & Traffic Metrics
	ActiveConns int64   `json:"active_conns"`
	TotalReqs   uint64  `json:"total_reqs"`
	FailedReqs  uint64  `json:"failed_reqs"`
	SuccessReqs uint64  `json:"success_reqs"`
	LatencyNano int64   `json:"latency_nano"` // Moving average response latency in ns
	CPUUsage    float64 `json:"cpu_usage"`    // Recorded CPU usage percentage
	MemUsageMB  float64 `json:"mem_usage_mb"` // Recorded Memory usage in MB

	// Pre-configured reverse proxy instance for high-throughput zero-allocation forwarding
	ReverseProxy *httputil.ReverseProxy `json:"-"`
}

// GetLatencyMs returns moving average latency in milliseconds.
func (b *Backend) GetLatencyMs() float64 {
	nano := atomic.LoadInt64(&b.LatencyNano)
	return float64(nano) / float64(time.Millisecond)
}

// IsHealthy returns current active health state.
func (b *Backend) IsHealthy() bool {
	return b.Healthy.Load()
}

// SetHealthy updates active health state.
func (b *Backend) SetHealthy(healthy bool) {
	b.Healthy.Store(healthy)
}
