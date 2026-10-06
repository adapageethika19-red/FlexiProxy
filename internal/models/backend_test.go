package models

import (
	"net/url"
	"testing"
	"time"
)

func TestBackendModel(t *testing.T) {
	u, _ := url.Parse("http://localhost:8001")

	b := &Backend{
		ID:      "backend-test",
		Address: "http://localhost:8001",
		URL:     u,
		Weight:  2,
	}

	if b.IsHealthy() {
		t.Fatalf("New uninitialized backend state expected false (unhealthy/zero), got true")
	}

	b.SetHealthy(true)

	if !b.IsHealthy() {
		t.Fatalf("Expected backend to be healthy after SetHealthy(true)")
	}

	b.SetHealthy(false)

	if b.IsHealthy() {
		t.Fatalf("Expected backend to be unhealthy after SetHealthy(false)")
	}

	b.LatencyNano = (25 * time.Millisecond).Nanoseconds()

	if b.GetLatencyMs() != 25.0 {
		t.Fatalf("Expected 25.0 ms latency, got %f", b.GetLatencyMs())
	}
}