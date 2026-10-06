package circuitbreaker

import (
	"sync"
	"time"

	"flexiproxy/internal/config"
	"flexiproxy/internal/models"
)

const (
	StateClosed   int32 = 0
	StateOpen     int32 = 1
	StateHalfOpen int32 = 2
)

// CircuitBreaker manages state transitions for a single backend server.
type CircuitBreaker struct {
	mu               sync.Mutex
	cfg              config.CircuitBreakerConfig
	state            int32
	consecutiveFails int
	halfOpenPasses   int
	lastStateChange  time.Time
}

// NewCircuitBreaker creates a circuit breaker instance.
func NewCircuitBreaker(cfg config.CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		cfg:             cfg,
		state:           StateClosed,
		lastStateChange: time.Now(),
	}
}

// State returns current state (CLOSED, OPEN, HALF_OPEN), checking open timeout expiration.
func (cb *CircuitBreaker) State() int32 {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateOpen {
		if time.Since(cb.lastStateChange) >= cb.cfg.OpenDuration {
			cb.state = StateHalfOpen
			cb.halfOpenPasses = 0
			cb.lastStateChange = time.Now()
		}
	}
	return cb.state
}

// AllowRequest determines if a request can proceed based on current circuit state.
func (cb *CircuitBreaker) AllowRequest() bool {
	if !cb.cfg.Enabled {
		return true
	}

	state := cb.State()
	switch state {
	case StateClosed:
		return true
	case StateOpen:
		return false
	case StateHalfOpen:
		return true
	default:
		return true
	}
}

// OnSuccess records a successful request execution.
func (cb *CircuitBreaker) OnSuccess(backend *models.Backend) {
	if !cb.cfg.Enabled {
		return
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateHalfOpen {
		cb.halfOpenPasses++
		if cb.halfOpenPasses >= cb.cfg.HalfOpenRequests {
			cb.state = StateClosed
			cb.consecutiveFails = 0
			cb.lastStateChange = time.Now()
			if backend != nil {
				backend.CircuitState.Store(StateClosed)
			}
		}
	} else if cb.state == StateClosed {
		cb.consecutiveFails = 0
	}
}

// OnFailure records a failed request execution.
func (cb *CircuitBreaker) OnFailure(backend *models.Backend) {
	if !cb.cfg.Enabled {
		return
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateClosed {
		cb.consecutiveFails++
		if cb.consecutiveFails >= cb.cfg.FailureThreshold {
			cb.state = StateOpen
			cb.lastStateChange = time.Now()
			if backend != nil {
				backend.CircuitState.Store(StateOpen)
			}
		}
	} else if cb.state == StateHalfOpen {
		cb.state = StateOpen
		cb.lastStateChange = time.Now()
		if backend != nil {
			backend.CircuitState.Store(StateOpen)
		}
	}
}
