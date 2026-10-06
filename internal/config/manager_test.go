package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigManager_HotReload(t *testing.T) {
	yaml1 := `
server:
  port: 8080
backends:
  - id: b1
    address: http://localhost:8001
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "reload_test.yaml")
	if err := os.WriteFile(configPath, []byte(yaml1), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg1, err := LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("failed to load initial config: %v", err)
	}

	mgr := NewManager(cfg1, configPath)
	if mgr.Get().Server.Port != 8080 {
		t.Errorf("expected initial port 8080, got %d", mgr.Get().Server.Port)
	}

	// Update file with new config
	yaml2 := `
server:
  port: 9090
backends:
  - id: b1
    address: http://localhost:8001
  - id: b2
    address: http://localhost:8002
`
	if err := os.WriteFile(configPath, []byte(yaml2), 0644); err != nil {
		t.Fatalf("failed to overwrite config: %v", err)
	}

	reloadedCfg, err := mgr.ReloadFromFile(configPath)
	if err != nil {
		t.Fatalf("hot reload failed: %v", err)
	}

	if reloadedCfg.Server.Port != 9090 {
		t.Errorf("expected reloaded port 9090, got %d", reloadedCfg.Server.Port)
	}
	if len(mgr.Get().Backends) != 2 {
		t.Errorf("expected 2 backends after reload, got %d", len(mgr.Get().Backends))
	}

	// Test invalid config payload during reload (should fail and keep existing active config)
	invalidYaml := `server: port: -10`
	_, err = mgr.ReloadFromBytes([]byte(invalidYaml))
	if err == nil {
		t.Errorf("expected error when reloading invalid YAML bytes")
	}
	// Active config must still be valid
	if mgr.Get().Server.Port != 9090 {
		t.Errorf("expected active config to remain 9090 after failed reload, got %d", mgr.Get().Server.Port)
	}
}
