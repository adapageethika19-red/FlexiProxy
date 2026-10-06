package circuitbreaker

import (
	"testing"
	"time"

	"flexiproxy/internal/config"
	"flexiproxy/internal/models"
)

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	cfg := config.CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 2,
		OpenDuration:     50 * time.Millisecond,
		HalfOpenRequests: 2,
	}

	cb := NewCircuitBreaker(cfg)
	backend := &models.Backend{ID: "test-cb-backend"}

	// Initial state: CLOSED
	if cb.State() != StateClosed {
		t.Errorf("expected initial state CLOSED (0), got %d", cb.State())
	}

	// First failure -> still CLOSED
	cb.OnFailure(backend)
	if cb.State() != StateClosed {
		t.Errorf("expected state CLOSED after 1 failure, got %d", cb.State())
	}

	// Second failure -> OPEN
	cb.OnFailure(backend)
	if cb.State() != StateOpen {
		t.Errorf("expected state OPEN (1) after 2 failures, got %d", cb.State())
	}
	if backend.CircuitState.Load() != StateOpen {
		t.Errorf("expected backend CircuitState OPEN, got %d", backend.CircuitState.Load())
	}

	// Should disallow requests when OPEN
	if cb.AllowRequest() {
		t.Errorf("expected AllowRequest to return false when OPEN")
	}

	// Wait for OpenDuration to expire -> transition to HALF_OPEN
	time.Sleep(60 * time.Millisecond)

	if cb.State() != StateHalfOpen {
		t.Errorf("expected state HALF_OPEN (2) after timeout, got %d", cb.State())
	}

	// Half Open recovery: 2 consecutive successes -> CLOSED
	cb.OnSuccess(backend)
	if cb.State() != StateHalfOpen {
		t.Errorf("expected state HALF_OPEN after 1 success, got %d", cb.State())
	}

	cb.OnSuccess(backend)
	if cb.State() != StateClosed {
		t.Errorf("expected state CLOSED (0) after 2 successes in HALF_OPEN, got %d", cb.State())
	}
}
