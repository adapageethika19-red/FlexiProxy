package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadFromFile_Valid(t *testing.T) {
	yamlContent := `
server:
  port: 9090
  admin_port: 9091
  read_timeout: 5s
  write_timeout: 5s
  idle_timeout: 60s

load_balancer:
  algorithm: round_robin

health_check:
  enabled: true
  path: /healthz
  interval: 1s
  timeout: 500ms
  healthy_threshold: 2
  unhealthy_threshold: 2

circuit_breaker:
  enabled: true
  failure_threshold: 3
  open_duration: 5s
  half_open_requests: 2

retry:
  enabled: true
  max_attempts: 3
  retry_on:
    - 502
    - 503

backends:
  - id: b1
    address: http://127.0.0.1:8001
    weight: 2
  - id: b2
    address: http://127.0.0.1:8002
    weight: 1
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "flexiproxy_test.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config file: %v", err)
	}

	cfg, err := LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("expected valid config load, got err: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Server.ReadTimeout != 5*time.Second {
		t.Errorf("expected read_timeout 5s, got %v", cfg.Server.ReadTimeout)
	}
	if len(cfg.Backends) != 2 {
		t.Errorf("expected 2 backends, got %d", len(cfg.Backends))
	}
	if cfg.Backends[0].URL == nil || cfg.Backends[0].URL.Host != "127.0.0.1:8001" {
		t.Errorf("expected backend URL parsed host 127.0.0.1:8001, got %v", cfg.Backends[0].URL)
	}
}

func TestLoadFromFile_Defaults(t *testing.T) {
	yamlContent := `
backends:
  - id: b1
    address: http://localhost:8001
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "minimal.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config file: %v", err)
	}

	cfg, err := LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("expected valid config load with defaults, got err: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.LoadBalancer.Algorithm != AlgorithmRoundRobin {
		t.Errorf("expected default algorithm round_robin, got %s", cfg.LoadBalancer.Algorithm)
	}
	if cfg.HealthCheck.Path != "/health" {
		t.Errorf("expected default health check path /health, got %s", cfg.HealthCheck.Path)
	}
	if cfg.Backends[0].Weight != 1 {
		t.Errorf("expected default backend weight 1, got %d", cfg.Backends[0].Weight)
	}
}

func TestConfig_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		yamlStr string
	}{
		{
			name: "Invalid Port",
			yamlStr: `
server:
  port: 70000
backends:
  - id: b1
    address: http://localhost:8001
`,
		},
		{
			name: "Duplicate Admin Port",
			yamlStr: `
server:
  port: 8080
  admin_port: 8080
backends:
  - id: b1
    address: http://localhost:8001
`,
		},
		{
			name: "Invalid Algorithm",
			yamlStr: `
load_balancer:
  algorithm: magic_random
backends:
  - id: b1
    address: http://localhost:8001
`,
		},
		{
			name: "Empty Backends",
			yamlStr: `
backends: []
`,
		},
		{
			name: "Duplicate Backend IDs",
			yamlStr: `
backends:
  - id: b1
    address: http://localhost:8001
  - id: b1
    address: http://localhost:8002
`,
		},
		{
			name: "Invalid Backend Scheme",
			yamlStr: `
backends:
  - id: b1
    address: ftp://localhost:8001
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "invalid.yaml")
			_ = os.WriteFile(configPath, []byte(tt.yamlStr), 0644)

			_, err := LoadFromFile(configPath)
			if err == nil {
				t.Errorf("expected error for case %q, but got nil", tt.name)
			}
		})
	}
}
