package api

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"flexiproxy/internal/config"
	"flexiproxy/internal/models"
)

func TestAdminServerStatus(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:      8080,
			AdminPort: 8081,
		},
		LoadBalancer: config.LoadBalancerConfig{
			Algorithm: "round_robin",
		},
	}

	cfgMgr := config.NewManager(cfg, "")

	b1 := &models.Backend{
		ID:      "backend-1",
		Address: "http://localhost:8001",
		Healthy: atomic.Bool{},
	}

	b1.Healthy.Store(true)

	admin := NewAdminServer(
		cfgMgr,
		[]*models.Backend{b1},
	)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/status",
		nil,
	)

	rec := httptest.NewRecorder()

	admin.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusOK,
			rec.Code,
		)
	}
}

func TestAdminServerBackends(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:      8080,
			AdminPort: 8081,
		},
		LoadBalancer: config.LoadBalancerConfig{
			Algorithm: "round_robin",
		},
	}

	cfgMgr := config.NewManager(cfg, "")

	admin := NewAdminServer(
		cfgMgr,
		[]*models.Backend{},
	)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/backends",
		nil,
	)

	rec := httptest.NewRecorder()

	admin.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusOK,
			rec.Code,
		)
	}
}
