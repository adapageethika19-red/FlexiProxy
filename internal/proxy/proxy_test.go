package proxy

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"flexiproxy/internal/config"
)

func TestProxyEngine_Forwarding(t *testing.T) {
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Response-Header", "ProxyTestValue")
		w.Header().Set("Content-Type", "application/json")

		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"method":"` + r.Method + `","path":"` + r.URL.Path + `","body":"` + string(body) + `"}`))
	}))
	defer backendServer.Close()

	u, _ := url.Parse(backendServer.URL)

	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8080, ReadTimeout: 5 * time.Second},
		LoadBalancer: config.LoadBalancerConfig{
			Algorithm: config.AlgorithmRoundRobin,
		},
		Backends: []config.BackendConfig{
			{ID: "test-b1", Address: backendServer.URL, Weight: 1, URL: u},
		},
	}
	cfg.SetDefaults()

	cfgMgr := config.NewManager(cfg, "")
	engine, err := NewProxyEngine(cfgMgr)
	if err != nil {
		t.Fatalf("failed to create proxy engine: %v", err)
	}
	defer engine.Close()

	// 1. Test GET forwarding
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test?foo=bar", nil)
	rec := httptest.NewRecorder()

	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if rec.Header().Get("X-Custom-Response-Header") != "ProxyTestValue" {
		t.Errorf("expected header X-Custom-Response-Header to be preserved, got %s", rec.Header().Get("X-Custom-Response-Header"))
	}

	// 2. Test POST with body forwarding
	postBody := []byte(`{"hello":"world"}`)
	reqPost := httptest.NewRequest(http.MethodPost, "/submit", bytes.NewReader(postBody))
	recPost := httptest.NewRecorder()

	engine.ServeHTTP(recPost, reqPost)

	if recPost.Code != http.StatusOK {
		t.Errorf("expected POST status 200, got %d", recPost.Code)
	}
	if !bytes.Contains(recPost.Body.Bytes(), postBody) {
		t.Errorf("expected response body to echo request body, got %s", recPost.Body.String())
	}
}

func TestProxyEngine_UpstreamUnavailable(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8080, ReadTimeout: 1 * time.Second},
		LoadBalancer: config.LoadBalancerConfig{
			Algorithm: config.AlgorithmRoundRobin,
		},
		Backends: []config.BackendConfig{
			{ID: "bad-backend", Address: "http://127.0.0.1:59999", Weight: 1},
		},
	}
	cfg.SetDefaults()

	cfgMgr := config.NewManager(cfg, "")
	engine, err := NewProxyEngine(cfgMgr)
	if err != nil {
		t.Fatalf("failed to create proxy engine: %v", err)
	}
	defer engine.Close()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected 502 Bad Gateway for unreachable backend, got %d", rec.Code)
	}
}
