package metrics

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"flexiproxy/internal/models"
)

func TestMetricsRecordAndHandler(t *testing.T) {
	RecordRequestStart()
	RecordRequestComplete(50*time.Millisecond, 200)

	RecordRequestStart()
	RecordRequestComplete(100*time.Millisecond, 502)

	RecordFailover()
	RecordRetry()

	u, _ := url.Parse("http://localhost:8001")
	backend := &models.Backend{
		ID:      "backend-1",
		Address: "http://localhost:8001",
		URL:     u,
	}
	backend.SetHealthy(true)

	handler := Handler([]*models.Backend{backend})
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from /metrics, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	expectedMetrics := []string{
		"flexiproxy_requests_total",
		"flexiproxy_request_errors_total",
		"flexiproxy_active_requests",
		"flexiproxy_failovers_total",
		"flexiproxy_retries_total",
		"flexiproxy_backend_health",
		"backend-1",
	}

	for _, m := range expectedMetrics {
		if !strings.Contains(body, m) {
			t.Errorf("Expected metrics response to contain %q, body:\n%s", m, body)
		}
	}
}
