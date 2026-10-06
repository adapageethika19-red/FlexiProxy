package ratelimit

import (
	"net"
	"net/http"
	"sync"
	"time"

	"flexiproxy/internal/config"
)

type clientVisitor struct {
	tokens shadowTokenBucket
	last   time.Time
}

type shadowTokenBucket struct {
	rate       float64
	capacity   float64
	tokens     float64
	lastUpdate time.Time
}

func (b *shadowTokenBucket) allow() bool {
	now := time.Now()
	elapsed := now.Sub(b.lastUpdate).Seconds()
	b.lastUpdate = now

	b.tokens += elapsed * b.rate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}
	return false
}

// Limiter manages per-IP token bucket rate limiting.
type Limiter struct {
	mu       sync.Mutex
	cfg      config.RateLimitConfig
	visitors map[string]*clientVisitor
}

// NewLimiter creates a Rate Limiter instance.
func NewLimiter(cfg config.RateLimitConfig) *Limiter {
	l := &Limiter{
		cfg:      cfg,
		visitors: make(map[string]*clientVisitor),
	}

	// Evict stale visitors every minute
	go func() {
		for {
			time.Sleep(1 * time.Minute)
			l.mu.Lock()
			for ip, v := range l.visitors {
				if time.Since(v.last) > 3*time.Minute {
					delete(l.visitors, ip)
				}
			}
			l.mu.Unlock()
		}
	}()

	return l
}

// Allow checks if the client IP is within rate limit thresholds.
func (l *Limiter) Allow(ip string) bool {
	if !l.cfg.Enabled {
		return true
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	v, exists := l.visitors[ip]
	if !exists {
		v = &clientVisitor{
			tokens: shadowTokenBucket{
				rate:       l.cfg.RPS,
				capacity:   float64(l.cfg.Burst),
				tokens:     float64(l.cfg.Burst),
				lastUpdate: time.Now(),
			},
			last: time.Now(),
		}
		l.visitors[ip] = v
	}

	v.last = time.Now()
	return v.tokens.allow()
}

// ExtractIP returns the remote IP address from the request.
func ExtractIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := net.ParseIP(fwd)
		if parts != nil {
			return parts.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
