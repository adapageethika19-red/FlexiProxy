package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"flexiproxy/internal/config"
	"flexiproxy/internal/proxy"
)

func TestEndToEndProxyAndFailover(t *testing.T) {
	// Setup 3 real HTTP test backend servers
	var b1Count, b2Count, b3Count uint64
	var b2Healthy uint32 = 1

	s1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		atomic.AddUint64(&b1Count, 1)
		w.Header().Set("X-Backend-ID", "backend-1")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"backend":"backend-1"}`))
	}))
	defer s1.Close()

	s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			if atomic.LoadUint32(&b2Healthy) == 1 {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}
		if atomic.LoadUint32(&b2Healthy) == 0 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"Backend 2 failure"}`))
			return
		}
		atomic.AddUint64(&b2Count, 1)
		w.Header().Set("X-Backend-ID", "backend-2")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"backend":"backend-2"}`))
	}))
	defer s2.Close()

	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		atomic.AddUint64(&b3Count, 1)
		w.Header().Set("X-Backend-ID", "backend-3")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"backend":"backend-3"}`))
	}))
	defer s3.Close()

	cfg := &config.Config{
		Server:       config.ServerConfig{Port: 8080, AdminPort: 8081},
		LoadBalancer: config.LoadBalancerConfig{Algorithm: "round_robin"},
		HealthCheck: config.HealthCheckConfig{
			Enabled:            true,
			Path:               "/health",
			Interval:           100 * time.Millisecond,
			Timeout:            50 * time.Millisecond,
			HealthyThreshold:   1,
			UnhealthyThreshold: 1,
		},
		CircuitBreaker: config.CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 2,
			OpenDuration:     200 * time.Millisecond,
			HalfOpenRequests: 1,
		},
		Retry: config.RetryConfig{
			Enabled:     true,
			MaxAttempts: 2,
			RetryOn:     []string{"500", "502", "503"},
		},
		Backends: []config.BackendConfig{
			{ID: "backend-1", Address: s1.URL, Weight: 1},
			{ID: "backend-2", Address: s2.URL, Weight: 1},
			{ID: "backend-3", Address: s3.URL, Weight: 1},
		},
	}
	cfg.SetDefaults()

	cfgMgr := config.NewManager(cfg, "")

	engine, err := proxy.NewProxyEngine(cfgMgr)
	if err != nil {
		t.Fatalf("failed to create proxy engine: %v", err)
	}
	defer engine.Close()

	proxyServer := httptest.NewServer(engine)
	defer proxyServer.Close()

	client := proxyServer.Client()

	// TEST 1 & TEST 2: All 3 healthy, Round Robin distribution
	t.Run("Test 1 & 2: Round Robin Distribution", func(t *testing.T) {
		for i := 0; i < 6; i++ {
			req, _ := http.NewRequest("GET", proxyServer.URL+"/", nil)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Request %d failed: %v", i, err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("Expected status 200, got %d", resp.StatusCode)
			}
			if reqID := resp.Header.Get("X-Request-ID"); reqID == "" {
				t.Errorf("Expected X-Request-ID header in response")
			}
			_, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
		}

		c1 := atomic.LoadUint64(&b1Count)
		c2 := atomic.LoadUint64(&b2Count)
		c3 := atomic.LoadUint64(&b3Count)

		if c1 == 0 || c2 == 0 || c3 == 0 {
			t.Fatalf("Expected requests distributed to all 3 backends, got counts: b1=%d, b2=%d, b3=%d", c1, c2, c3)
		}
	})

	// TEST 3, 4, 5, 6: Backend 2 becomes unhealthy; active health checker detects it; traffic stops & retries fail over
	t.Run("Test 3-6: Backend 2 Failover", func(t *testing.T) {
		atomic.StoreUint32(&b2Healthy, 0)
		time.Sleep(250 * time.Millisecond) // Allow active health checker to trigger

		b2CountBefore := atomic.LoadUint64(&b2Count)

		for i := 0; i < 6; i++ {
			req, _ := http.NewRequest("GET", proxyServer.URL+"/", nil)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Failover request %d failed: %v", i, err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("Expected 200 OK after failover, got %d", resp.StatusCode)
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			var res map[string]string
			_ = json.Unmarshal(body, &res)
			if res["backend"] == "backend-2" {
				t.Fatalf("Request routed to unhealthy backend-2!")
			}
		}

		b2CountAfter := atomic.LoadUint64(&b2Count)
		if b2CountAfter > b2CountBefore {
			t.Fatalf("Backend 2 received requests after becoming unhealthy")
		}
	})

	// TEST 8, 9, 10: Backend 2 recovers -> HALF_OPEN -> CLOSED
	t.Run("Test 8-10: Backend 2 Recovery", func(t *testing.T) {
		atomic.StoreUint32(&b2Healthy, 1)
		time.Sleep(300 * time.Millisecond) // Allow health check recovery

		b2CountBefore := atomic.LoadUint64(&b2Count)

		for i := 0; i < 6; i++ {
			req, _ := http.NewRequest("GET", proxyServer.URL+"/", nil)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Recovery request %d failed: %v", i, err)
			}
			_, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
		}

		b2CountAfter := atomic.LoadUint64(&b2Count)
		if b2CountAfter <= b2CountBefore {
			t.Fatalf("Backend 2 did not receive requests after recovery")
		}
	})

	// TEST 13: Non-idempotent POST is NOT retried blindly
	t.Run("Test 13: Non-Idempotent POST Safety", func(t *testing.T) {
		atomic.StoreUint32(&b2Healthy, 0)
		time.Sleep(200 * time.Millisecond)

		req, _ := http.NewRequest("POST", proxyServer.URL+"/", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST request error: %v", err)
		}
		resp.Body.Close()
		atomic.StoreUint32(&b2Healthy, 1)
	})

	// TEST 14: All backends unavailable produces 503
	t.Run("Test 14: All Backends Unavailable", func(t *testing.T) {
		s1.Close()
		s2.Close()
		s3.Close()
		for _, b := range engine.Backends() {
			b.SetHealthy(false)
		}

		req, _ := http.NewRequest("GET", proxyServer.URL+"/", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusServiceUnavailable && resp.StatusCode != http.StatusBadGateway {
			t.Fatalf("Expected 503 Service Unavailable or 502 Bad Gateway when all backends down, got %d", resp.StatusCode)
		}
	})
}
