package balancer

import (
	"fmt"
	"math"
	"net/http"
	"sync"
	"sync/atomic"

	"flexiproxy/internal/config"
	"flexiproxy/internal/models"
)

// LoadBalancer defines the interface for selecting an upstream backend server.
type LoadBalancer interface {
	SelectBackend(backends []*models.Backend, req *http.Request) *models.Backend
	Algorithm() string
}

// RoundRobinBalancer implements simple round-robin selection among healthy backends.
type RoundRobinBalancer struct {
	counter uint64
}

func NewRoundRobinBalancer() *RoundRobinBalancer {
	return &RoundRobinBalancer{}
}

func (b *RoundRobinBalancer) Algorithm() string {
	return config.AlgorithmRoundRobin
}

func (b *RoundRobinBalancer) SelectBackend(backends []*models.Backend, req *http.Request) *models.Backend {
	healthyBackends := filterHealthy(backends)
	if len(healthyBackends) == 0 {
		return nil
	}
	idx := atomic.AddUint64(&b.counter, 1) - 1
	return healthyBackends[idx%uint64(len(healthyBackends))]
}

// WeightedRoundRobinBalancer implements smooth weighted round-robin distribution.
type WeightedRoundRobinBalancer struct {
	mu             sync.Mutex
	currentWeights map[string]int
}

func NewWeightedRoundRobinBalancer() *WeightedRoundRobinBalancer {
	return &WeightedRoundRobinBalancer{
		currentWeights: make(map[string]int),
	}
}

func (b *WeightedRoundRobinBalancer) Algorithm() string {
	return config.AlgorithmWeightedRoundRobin
}

func (b *WeightedRoundRobinBalancer) SelectBackend(backends []*models.Backend, req *http.Request) *models.Backend {
	healthyBackends := filterHealthy(backends)
	if len(healthyBackends) == 0 {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	totalWeight := 0
	var best *models.Backend
	bestWeight := math.MinInt32

	for _, backend := range healthyBackends {
		w := backend.Weight
		if w <= 0 {
			w = 1
		}
		totalWeight += w

		b.currentWeights[backend.ID] += w

		if b.currentWeights[backend.ID] > bestWeight {
			bestWeight = b.currentWeights[backend.ID]
			best = backend
		}
	}

	if best != nil {
		b.currentWeights[best.ID] -= totalWeight
	}

	return best
}

// LeastConnectionsBalancer selects the healthy backend with the minimum active HTTP connections.
type LeastConnectionsBalancer struct{}

func NewLeastConnectionsBalancer() *LeastConnectionsBalancer {
	return &LeastConnectionsBalancer{}
}

func (b *LeastConnectionsBalancer) Algorithm() string {
	return config.AlgorithmLeastConnections
}

func (b *LeastConnectionsBalancer) SelectBackend(backends []*models.Backend, req *http.Request) *models.Backend {
	healthyBackends := filterHealthy(backends)
	if len(healthyBackends) == 0 {
		return nil
	}

	var best *models.Backend
	minConns := int64(math.MaxInt64)

	for _, backend := range healthyBackends {
		conns := atomic.LoadInt64(&backend.ActiveConns)
		if conns < minConns {
			minConns = conns
			best = backend
		}
	}

	return best
}

// AdaptiveBalancer routes requests using a dynamic heuristic scoring model.
// Score = w_latency * latency_ms + w_conns * active_conns + w_cpu * cpu + w_mem * mem + w_err * err_rate
// Lower score = better target server candidate.
type AdaptiveBalancer struct {
	weights config.AdaptiveWeightsConfig
}

func NewAdaptiveBalancer(weights config.AdaptiveWeightsConfig) *AdaptiveBalancer {
	return &AdaptiveBalancer{weights: weights}
}

func (b *AdaptiveBalancer) Algorithm() string {
	return config.AlgorithmAdaptive
}

func (b *AdaptiveBalancer) SelectBackend(backends []*models.Backend, req *http.Request) *models.Backend {
	healthyBackends := filterHealthy(backends)
	if len(healthyBackends) == 0 {
		return nil
	}

	var best *models.Backend
	minScore := math.MaxFloat64

	for _, backend := range healthyBackends {
		latencyMs := backend.GetLatencyMs()
		activeConns := float64(atomic.LoadInt64(&backend.ActiveConns))
		cpuPct := backend.CPUUsage
		memMB := backend.MemUsageMB

		totalReqs := atomic.LoadUint64(&backend.TotalReqs)
		failedReqs := atomic.LoadUint64(&backend.FailedReqs)
		var errRate float64
		if totalReqs > 0 {
			errRate = float64(failedReqs) / float64(totalReqs) * 100.0
		}

		score := (b.weights.Latency * latencyMs) +
			(b.weights.ActiveConnections * activeConns) +
			(b.weights.CPU * cpuPct) +
			(b.weights.Memory * memMB) +
			(b.weights.ErrorRate * errRate)

		if score < minScore {
			minScore = score
			best = backend
		}
	}

	return best
}

// NewBalancer creates a LoadBalancer instance corresponding to the configured algorithm.
func NewBalancer(cfg *config.LoadBalancerConfig) (LoadBalancer, error) {
	switch cfg.Algorithm {
	case config.AlgorithmRoundRobin:
		return NewRoundRobinBalancer(), nil
	case config.AlgorithmWeightedRoundRobin:
		return NewWeightedRoundRobinBalancer(), nil
	case config.AlgorithmLeastConnections:
		return NewLeastConnectionsBalancer(), nil
	case config.AlgorithmAdaptive:
		return NewAdaptiveBalancer(cfg.AdaptiveWeights), nil
	default:
		return nil, fmt.Errorf("unsupported load balancer algorithm: %s", cfg.Algorithm)
	}
}

// Helper: filters out unhealthy backends or backends with open circuit breakers.
func filterHealthy(backends []*models.Backend) []*models.Backend {
	healthy := make([]*models.Backend, 0, len(backends))
	for _, b := range backends {
		if b.IsHealthy() && b.CircuitState.Load() != 1 { // 1 = OPEN circuit breaker state
			healthy = append(healthy, b)
		}
	}
	return healthy
}
