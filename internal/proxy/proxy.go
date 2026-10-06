package proxy

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync/atomic"
	"time"

	"flexiproxy/internal/balancer"
	"flexiproxy/internal/circuitbreaker"
	"flexiproxy/internal/config"
	"flexiproxy/internal/health"
	"flexiproxy/internal/logging"
	"flexiproxy/internal/metrics"
	"flexiproxy/internal/models"
	"flexiproxy/internal/ratelimit"
	"flexiproxy/internal/retry"
)

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

// ProxyEngine manages HTTP request forwarding, rate limiting, load balancing,
// health monitoring, circuit breaking, and retries.
type ProxyEngine struct {
	configManager   *config.Manager
	backends        []*models.Backend
	balancer        balancer.LoadBalancer
	activeChecker   *health.ActiveChecker
	passiveMonitor  *health.PassiveMonitor
	retryPolicy     *retry.RetryPolicy
	limiter         *ratelimit.Limiter
	circuitBreakers map[string]*circuitbreaker.CircuitBreaker
	transport       *http.Transport
}

// NewProxyEngine initializes reusable HTTP transports, rate limiters,
// health checkers, circuit breakers, and load balancer strategies.
func NewProxyEngine(cfgMgr *config.Manager) (*ProxyEngine, error) {
	cfg := cfgMgr.Get()

	lb, err := balancer.NewBalancer(&cfg.LoadBalancer)
	if err != nil {
		return nil, fmt.Errorf("failed to create load balancer: %w", err)
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          500,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: cfg.Server.ReadTimeout,
	}

	backends := make([]*models.Backend, len(cfg.Backends))
	cbs := make(map[string]*circuitbreaker.CircuitBreaker)

	for i, bCfg := range cfg.Backends {
		u, err := url.Parse(bCfg.Address)
		if err != nil {
			return nil, fmt.Errorf("invalid backend address %s: %w", bCfg.Address, err)
		}

		backend := &models.Backend{
			ID:      bCfg.ID,
			URL:     u,
			Address: bCfg.Address,
			Weight:  bCfg.Weight,
		}
		backend.SetHealthy(true)

		cb := circuitbreaker.NewCircuitBreaker(cfg.CircuitBreaker)
		cbs[bCfg.ID] = cb

		rp := &httputil.ReverseProxy{
			Transport:     transport,
			FlushInterval: -1,

			Director: func(req *http.Request) {
				req.URL.Scheme = u.Scheme
				req.URL.Host = u.Host
				req.Host = u.Host

				if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
					if prior := req.Header.Get("X-Forwarded-For"); prior != "" {
						req.Header.Set("X-Forwarded-For", prior+", "+clientIP)
					} else {
						req.Header.Set("X-Forwarded-For", clientIP)
					}
				}

				if req.Header.Get("X-Forwarded-Proto") == "" {
					if req.TLS != nil {
						req.Header.Set("X-Forwarded-Proto", "https")
					} else {
						req.Header.Set("X-Forwarded-Proto", "http")
					}
				}
			},

			ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
				if req.Context().Err() == context.Canceled {
					logging.LogJSON(logging.LogEvent{
						Level:  "warn",
						Event:  "client_cancelled",
						Reason: err.Error(),
						Status: 499,
					})
					w.WriteHeader(499)
					return
				}

				logging.LogJSON(logging.LogEvent{
					Level:  "error",
					Event:  "upstream_error",
					Reason: err.Error(),
					Status: http.StatusBadGateway,
				})

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(
					`{"error":"Bad Gateway","message":"Upstream server connection failed"}`,
				))
			},
		}

		backend.ReverseProxy = rp
		backends[i] = backend
	}

	activeChecker := health.NewActiveChecker(cfg.HealthCheck, backends)
	passiveMonitor := health.NewPassiveMonitor()
	retryPolicy := retry.NewRetryPolicy(cfg.Retry)
	limiter := ratelimit.NewLimiter(cfg.Server.RateLimit)

	engine := &ProxyEngine{
		configManager:   cfgMgr,
		backends:        backends,
		balancer:        lb,
		activeChecker:   activeChecker,
		passiveMonitor:  passiveMonitor,
		retryPolicy:     retryPolicy,
		limiter:         limiter,
		circuitBreakers: cbs,
		transport:       transport,
	}

	activeChecker.Start(context.Background())

	return engine, nil
}

// Backends returns the list of configured backends.
func (p *ProxyEngine) Backends() []*models.Backend {
	return p.backends
}

// ServeHTTP implements http.Handler for the primary proxy port with
// rate limiting, security headers, and failover retry.
func (p *ProxyEngine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	metrics.RecordRequestStart()

	// 1. Ensure Request ID
	reqID := logging.EnsureRequestID(r)

	// IMPORTANT:
	// Return the request ID to the client as a response header.
	// This allows dashboards, browser extensions, and clients
	// to trace individual requests through FlexiProxy.
	w.Header().Set("X-Request-ID", reqID)

	// 2. Set Security Headers
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-XSS-Protection", "1; mode=block")

	// 3. Rate Limiting Check
	clientIP := ratelimit.ExtractIP(r)

	if !p.limiter.Allow(clientIP) {
		metrics.RecordRequestComplete(0, http.StatusTooManyRequests)

		logging.LogJSON(logging.LogEvent{
			Level:     "warn",
			Event:     "rate_limit_exceeded",
			RequestID: reqID,
			Status:    http.StatusTooManyRequests,
			Message:   fmt.Sprintf("Client IP %s exceeded rate limits", clientIP),
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)

		_, _ = w.Write([]byte(
			`{"error":"Too Many Requests","message":"Rate limit threshold exceeded"}`,
		))
		return
	}

	cfg := p.configManager.Get()

	// 4. Max Body Size Limiter
	if cfg.Server.MaxBodySize > 0 && r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, cfg.Server.MaxBodySize)
	}

	attempts := 0
	maxAttempts := cfg.Retry.MaxAttempts

	if !cfg.Retry.Enabled || !retry.IsIdempotent(r.Method) {
		maxAttempts = 1
	}

	var lastStatus int

	for attempts < maxAttempts {
		attempts++

		if attempts > 1 {
			metrics.RecordRetry()
		}

		backend := p.balancer.SelectBackend(p.backends, r)

		if backend == nil {
			metrics.RecordRequestComplete(
				0,
				http.StatusServiceUnavailable,
			)

			logging.LogJSON(logging.LogEvent{
				Level:     "error",
				Event:     "no_backend_available",
				RequestID: reqID,
				Method:    r.Method,
				Path:      r.URL.Path,
				Status:    http.StatusServiceUnavailable,
				Message:   "No healthy backends available for routing",
			})

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)

			_, _ = w.Write([]byte(
				`{"error":"Service Unavailable","message":"No healthy backends available"}`,
			))
			return
		}

		cb := p.circuitBreakers[backend.ID]

		if cb != nil && !cb.AllowRequest() {
			log.Printf(
				"[CircuitBreaker] Skipping backend %s (Circuit OPEN)",
				backend.ID,
			)
			continue
		}

		atomic.AddInt64(&backend.ActiveConns, 1)
		atomic.AddUint64(&backend.TotalReqs, 1)

		startTime := time.Now()

		recorder := &statusRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		backend.ReverseProxy.ServeHTTP(recorder, r)

		duration := time.Since(startTime)

		atomic.StoreInt64(
			&backend.LatencyNano,
			duration.Nanoseconds(),
		)

		atomic.AddInt64(
			&backend.ActiveConns,
			-1,
		)

		lastStatus = recorder.statusCode

		p.passiveMonitor.ObserveRequest(
			backend,
			lastStatus,
			duration,
			nil,
		)

		if lastStatus >= 500 {
			if cb != nil {
				cb.OnFailure(backend)
			}

			if p.retryPolicy.ShouldRetry(
				r,
				attempts,
				lastStatus,
				nil,
			) {
				metrics.RecordFailover()

				logging.LogJSON(logging.LogEvent{
					Level:     "warn",
					Event:     "failover",
					RequestID: reqID,
					From:      backend.ID,
					Status:    lastStatus,
					Message: fmt.Sprintf(
						"Attempt %d failed on backend %s with status %d. Retrying failover...",
						attempts,
						backend.ID,
						lastStatus,
					),
				})

				continue
			}
		} else {
			if cb != nil {
				cb.OnSuccess(backend)
			}

			metrics.RecordRequestComplete(
				duration,
				lastStatus,
			)

			logging.LogJSON(logging.LogEvent{
				Level:     "info",
				Event:     "request_complete",
				RequestID: reqID,
				Method:    r.Method,
				Path:      r.URL.Path,
				Backend:   backend.ID,
				Status:    lastStatus,
				LatencyMS: duration.Milliseconds(),
			})

			return
		}
	}

	metrics.RecordRequestComplete(
		0,
		http.StatusBadGateway,
	)

	logging.LogJSON(logging.LogEvent{
		Level:     "error",
		Event:     "all_retries_failed",
		RequestID: reqID,
		Method:    r.Method,
		Path:      r.URL.Path,
		Status:    http.StatusBadGateway,
		Message:   "All retry attempts exhausted",
	})
}

// Close releases idle connection pools and stops health checkers.
func (p *ProxyEngine) Close() {
	if p.activeChecker != nil {
		p.activeChecker.Stop()
	}

	if p.transport != nil {
		p.transport.CloseIdleConnections()
	}
}
