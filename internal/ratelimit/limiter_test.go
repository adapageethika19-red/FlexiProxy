package ratelimit

import (
	"net/http/httptest"
	"testing"

	"flexiproxy/internal/config"
)

func TestRateLimiter(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled: true,
		RPS:     1,
		Burst:   2,
	}

	limiter := NewLimiter(cfg)

	// Burst allowance = 2
	if !limiter.Allow("192.168.1.1") {
		t.Errorf("expected request 1 to be allowed")
	}
	if !limiter.Allow("192.168.1.1") {
		t.Errorf("expected request 2 to be allowed under burst")
	}

	// 3rd request should exceed burst limit
	if limiter.Allow("192.168.1.1") {
		t.Errorf("expected request 3 to be rejected by rate limiter")
	}

	// Different IP should have independent token bucket
	if !limiter.Allow("192.168.1.2") {
		t.Errorf("expected independent IP 192.168.1.2 to be allowed")
	}
}

func TestExtractIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"

	if ExtractIP(req) != "10.0.0.1" {
		t.Errorf("expected IP 10.0.0.1, got %s", ExtractIP(req))
	}

	req.Header.Set("X-Forwarded-For", "203.0.113.195")
	if ExtractIP(req) != "203.0.113.195" {
		t.Errorf("expected X-Forwarded-For IP 203.0.113.195, got %s", ExtractIP(req))
	}
}
