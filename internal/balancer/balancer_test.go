package balancer

import (
	"net/http/httptest"
	"testing"
	"time"

	"flexiproxy/internal/config"
	"flexiproxy/internal/models"
)

func createTestBackends() []*models.Backend {
	b1 := &models.Backend{ID: "b1", Weight: 3}
	b1.SetHealthy(true)

	b2 := &models.Backend{ID: "b2", Weight: 1}
	b2.SetHealthy(true)

	b3 := &models.Backend{ID: "b3", Weight: 1}
	b3.SetHealthy(false) // Unhealthy backend

	return []*models.Backend{b1, b2, b3}
}

func TestRoundRobinBalancer(t *testing.T) {
	backends := createTestBackends()
	balancer := NewRoundRobinBalancer()
	req := httptest.NewRequest("GET", "/", nil)

	selected1 := balancer.SelectBackend(backends, req)
	selected2 := balancer.SelectBackend(backends, req)
	selected3 := balancer.SelectBackend(backends, req)

	if selected1.ID != "b1" || selected2.ID != "b2" || selected3.ID != "b1" {
		t.Errorf("unexpected round robin sequence: %s, %s, %s", selected1.ID, selected2.ID, selected3.ID)
	}
}

func TestWeightedRoundRobinBalancer(t *testing.T) {
	backends := createTestBackends() // b1 weight=3, b2 weight=1
	balancer := NewWeightedRoundRobinBalancer()
	req := httptest.NewRequest("GET", "/", nil)

	counts := make(map[string]int)
	for i := 0; i < 4; i++ {
		selected := balancer.SelectBackend(backends, req)
		counts[selected.ID]++
	}

	if counts["b1"] != 3 || counts["b2"] != 1 {
		t.Errorf("expected weighted distribution 3:1 (b1:3, b2:1), got %+v", counts)
	}
}

func TestLeastConnectionsBalancer(t *testing.T) {
	b1 := &models.Backend{ID: "b1", ActiveConns: 5}
	b1.SetHealthy(true)

	b2 := &models.Backend{ID: "b2", ActiveConns: 1}
	b2.SetHealthy(true)

	backends := []*models.Backend{b1, b2}
	balancer := NewLeastConnectionsBalancer()
	req := httptest.NewRequest("GET", "/", nil)

	selected := balancer.SelectBackend(backends, req)
	if selected.ID != "b2" {
		t.Errorf("expected backend with least connections (b2), got %s", selected.ID)
	}
}

func TestAdaptiveBalancer(t *testing.T) {
	b1 := &models.Backend{ID: "b1", ActiveConns: 10, LatencyNano: int64(100 * time.Millisecond)}
	b1.SetHealthy(true)

	b2 := &models.Backend{ID: "b2", ActiveConns: 1, LatencyNano: int64(5 * time.Millisecond)}
	b2.SetHealthy(true)

	backends := []*models.Backend{b1, b2}

	weights := config.AdaptiveWeightsConfig{
		Latency:           0.5,
		ActiveConnections: 0.5,
	}

	balancer := NewAdaptiveBalancer(weights)
	req := httptest.NewRequest("GET", "/", nil)

	selected := balancer.SelectBackend(backends, req)
	if selected.ID != "b2" {
		t.Errorf("expected backend with lower latency and fewer connections (b2), got %s", selected.ID)
	}
}

func TestFilterHealthy_CircuitBreakerOpen(t *testing.T) {
	b1 := &models.Backend{ID: "b1"}
	b1.SetHealthy(true)
	b1.CircuitState.Store(1) // OPEN circuit breaker state

	b2 := &models.Backend{ID: "b2"}
	b2.SetHealthy(true)
	b2.CircuitState.Store(0) // CLOSED state

	backends := []*models.Backend{b1, b2}
	balancer := NewRoundRobinBalancer()
	req := httptest.NewRequest("GET", "/", nil)

	selected := balancer.SelectBackend(backends, req)
	if selected.ID != "b2" {
		t.Errorf("expected open circuit backend b1 to be filtered out, got %s", selected.ID)
	}
}
