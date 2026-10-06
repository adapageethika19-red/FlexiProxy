package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"flexiproxy/internal/api"
	"flexiproxy/internal/config"
	"flexiproxy/internal/proxy"
)

func TestRealWorldProductionFeatures(t *testing.T) {
	b1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend-Server", "b1")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"backend":"b1"}`))
	}))
	defer b1.Close()

	b2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend-Server", "b2")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"backend":"b2"}`))
	}))
	defer b2.Close()

	u1, _ := url.Parse(b1.URL)
	u2, _ := url.Parse(b2.URL)

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:        8080,
			AdminPort:   8081,
			AdminAPIKey: "admin-secret-token",
			MaxBodySize: 5 * 1024 * 1024,
			RateLimit: config.RateLimitConfig{
				Enabled: true,
				RPS:     100,
				Burst:   2,
			},
		},
		LoadBalancer: config.LoadBalancerConfig{Algorithm: config.AlgorithmRoundRobin},
		Backends: []config.BackendConfig{
			{ID: "b1", Address: b1.URL, Weight: 1, URL: u1},
			{ID: "b2", Address: b2.URL, Weight: 1, URL: u2},
		},
	}
	cfg.SetDefaults()

	tmpDir := t.TempDir()
	configPath := fmt.Sprintf("%s/flexiproxy.yaml", tmpDir)
	_ = os.WriteFile(configPath, []byte(fmt.Sprintf(`
server:
  port: 8080
  admin_port: 8081
  admin_api_key: admin-secret-token
load_balancer:
  algorithm: round_robin
backends:
  - id: b1
    address: %s
  - id: b2
    address: %s
`, b1.URL, b2.URL)), 0644)

	cfgMgr := config.NewManager(cfg, configPath)

	proxyEngine, err := proxy.NewProxyEngine(cfgMgr)
	if err != nil {
		t.Fatalf("failed to create proxy engine: %v", err)
	}
	defer proxyEngine.Close()

	adminServer := api.NewAdminServer(cfgMgr, proxyEngine.Backends())

	// ----------------------------------------------------
	// TEST 1: Request ID Injection & Header Security
	// ----------------------------------------------------
	t.Run("Request ID and Security Headers", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()
		proxyEngine.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", rec.Code)
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("expected security header X-Content-Type-Options nosniff")
		}
	})

	// ----------------------------------------------------
	// TEST 2: Token Bucket Rate Limiting (Burst = 2)
	// ----------------------------------------------------
	t.Run("Rate Limiting Throttling", func(t *testing.T) {
		clientIP := "203.0.113.50:12345"

		req1 := httptest.NewRequest("GET", "/api", nil)
		req1.RemoteAddr = clientIP
		rec1 := httptest.NewRecorder()
		proxyEngine.ServeHTTP(rec1, req1)
		if rec1.Code != http.StatusOK {
			t.Errorf("expected attempt 1 to be 200 OK")
		}

		req2 := httptest.NewRequest("GET", "/api", nil)
		req2.RemoteAddr = clientIP
		rec2 := httptest.NewRecorder()
		proxyEngine.ServeHTTP(rec2, req2)
		if rec2.Code != http.StatusOK {
			t.Errorf("expected attempt 2 to be 200 OK")
		}

		// 3rd attempt from same IP should be 429 Too Many Requests
		req3 := httptest.NewRequest("GET", "/api", nil)
		req3.RemoteAddr = clientIP
		rec3 := httptest.NewRecorder()
		proxyEngine.ServeHTTP(rec3, req3)
		if rec3.Code != http.StatusTooManyRequests {
			t.Errorf("expected attempt 3 to return 429 Too Many Requests, got %d", rec3.Code)
		}
	})

	// ----------------------------------------------------
	// TEST 3: Admin API Key Authentication
	// ----------------------------------------------------
	t.Run("Admin API Key Protection", func(t *testing.T) {
		reqUnauth := httptest.NewRequest("POST", "/api/v1/reload", nil)
		recUnauth := httptest.NewRecorder()
		adminServer.ServeHTTP(recUnauth, reqUnauth)

		if recUnauth.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized for unauthenticated admin request, got %d", recUnauth.Code)
		}

		reqAuth := httptest.NewRequest("POST", "/api/v1/reload", nil)
		reqAuth.Header.Set("X-API-Key", "admin-secret-token")
		recAuth := httptest.NewRecorder()
		adminServer.ServeHTTP(recAuth, reqAuth)

		if recAuth.Code != http.StatusOK {
			t.Errorf("expected 200 OK for authenticated admin reload, got %d", recAuth.Code)
		}
	})

	// ----------------------------------------------------
	// TEST 4: Atomic Hot Configuration Reload
	// ----------------------------------------------------
	t.Run("Hot Configuration Reload", func(t *testing.T) {
		newConfigYAML := fmt.Sprintf(`
server:
  port: 9090
  admin_port: 9091
load_balancer:
  algorithm: round_robin
backends:
  - id: b1
    address: %s
    weight: 1
`, b1.URL)

		reqReload := httptest.NewRequest("POST", "/api/v1/reload", bytes.NewBufferString(newConfigYAML))
		reqReload.Header.Set("X-API-Key", "admin-secret-token")
		recReload := httptest.NewRecorder()
		adminServer.ServeHTTP(recReload, reqReload)

		if recReload.Code != http.StatusOK {
			body, _ := io.ReadAll(recReload.Body)
			t.Fatalf("expected hot reload success 200 OK, got %d body: %s", recReload.Code, string(body))
		}

		active := cfgMgr.Get()
		if active.Server.Port != 9090 {
			t.Errorf("expected active config server port to be 9090 after hot reload, got %d", active.Server.Port)
		}

		reqStatus := httptest.NewRequest("GET", "/api/v1/status", nil)
		recStatus := httptest.NewRecorder()
		adminServer.ServeHTTP(recStatus, reqStatus)

		var statusMap map[string]interface{}
		_ = json.Unmarshal(recStatus.Body.Bytes(), &statusMap)
		if statusMap["status"] != "OPERATIONAL" {
			t.Errorf("expected operational system status after hot reload")
		}
	})
}
