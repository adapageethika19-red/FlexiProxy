package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"flexiproxy/internal/config"
	"flexiproxy/internal/models"
)

func TestActiveChecker_HealthTransitions(t *testing.T) {
	isHealthy := true

	// Server that responds 200 OK when isHealthy=true, 500 when false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isHealthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	backend := &models.Backend{ID: "health-test-b1", URL: u}

	cfg := config.HealthCheckConfig{
		Enabled:            true,
		Path:               "/health",
		Interval:           50 * time.Millisecond,
		Timeout:            500 * time.Millisecond,
		HealthyThreshold:   2,
		UnhealthyThreshold: 2,
	}

	checker := NewActiveChecker(cfg, []*models.Backend{backend})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	checker.Start(ctx)
	defer checker.Stop()

	// Initial probe -> healthy
	time.Sleep(120 * time.Millisecond)
	if !backend.IsHealthy() {
		t.Errorf("expected backend to be healthy initially")
	}

	// Trigger failure injection
	isHealthy = false
	time.Sleep(150 * time.Millisecond)

	if backend.IsHealthy() {
		t.Errorf("expected backend to transition to unhealthy after 2 consecutive failures")
	}
}
