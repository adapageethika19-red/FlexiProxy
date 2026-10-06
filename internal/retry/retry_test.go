package retry

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"flexiproxy/internal/config"
)

func TestRetryPolicy_IdempotencyAndFailover(t *testing.T) {
	cfg := config.RetryConfig{
		Enabled:     true,
		MaxAttempts: 2,
		RetryOn:     []string{"connection_error", "timeout", "502", "503", "504"},
	}

	policy := NewRetryPolicy(cfg)

	// 1. GET (Idempotent) + 502 -> Should Retry
	reqGet := httptest.NewRequest(http.MethodGet, "/test", nil)
	if !policy.ShouldRetry(reqGet, 1, 502, nil) {
		t.Errorf("expected GET with 502 to be retried on attempt 1")
	}

	// 2. GET attempt 2 (MaxAttempts reached) -> Should NOT Retry
	if policy.ShouldRetry(reqGet, 2, 502, nil) {
		t.Errorf("expected GET to stop retrying when attempt >= MaxAttempts")
	}

	// 3. POST (Non-Idempotent) + 502 -> Should NOT Retry
	reqPost := httptest.NewRequest(http.MethodPost, "/submit", nil)
	if policy.ShouldRetry(reqPost, 1, 502, nil) {
		t.Errorf("expected POST request to NOT be retried due to idempotency protection")
	}

	// 4. GET + Connection Error -> Should Retry
	if !policy.ShouldRetry(reqGet, 1, 0, errors.New("dial tcp: connection refused")) {
		t.Errorf("expected GET with transport error to be retried")
	}
}
