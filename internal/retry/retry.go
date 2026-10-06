package retry

import (
	"net/http"
	"strings"

	"flexiproxy/internal/config"
)

// RetryPolicy determines whether a request failure is safe to retry on another backend.
type RetryPolicy struct {
	cfg config.RetryConfig
}

// NewRetryPolicy creates a RetryPolicy controller.
func NewRetryPolicy(cfg config.RetryConfig) *RetryPolicy {
	return &RetryPolicy{cfg: cfg}
}

// IsIdempotent checks if the HTTP method is safe/idempotent according to RFC 7231.
func IsIdempotent(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

// ShouldRetry evaluates whether a failed request attempt should trigger failover retry.
func (p *RetryPolicy) ShouldRetry(req *http.Request, attempt int, statusCode int, err error) bool {
	if !p.cfg.Enabled {
		return false
	}

	if attempt >= p.cfg.MaxAttempts {
		return false
	}

	// Non-idempotent requests (e.g. POST) are not blindly retried to avoid duplicate mutation
	if !IsIdempotent(req.Method) {
		return false
	}

	if err != nil {
		return true // Connection reset, timeout, dial failure
	}

	// Check configured error status codes (e.g. 502, 503, 504)
	for _, codeStr := range p.cfg.RetryOn {
		switch codeStr {
		case "502":
			if statusCode == http.StatusBadGateway {
				return true
			}
		case "503":
			if statusCode == http.StatusServiceUnavailable {
				return true
			}
		case "504":
			if statusCode == http.StatusGatewayTimeout {
				return true
			}
		}
	}

	return false
}
