package config

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// Manager handles thread-safe atomic configuration access and hot reloads without downtime.
type Manager struct {
	mu           sync.Mutex
	activeConfig atomic.Pointer[Config]
	configPath   string
}

// NewManager initializes the config manager with an initial validated configuration.
func NewManager(initialConfig *Config, path string) *Manager {
	m := &Manager{
		configPath: path,
	}
	m.activeConfig.Store(initialConfig)
	return m
}

// Get returns a pointer to the currently active, immutable Config object.
func (m *Manager) Get() *Config {
	return m.activeConfig.Load()
}

// ReloadFromFile reads, validates, and atomically replaces active configuration from disk.
// If the new file contains invalid YAML or fails validation, the current active configuration is preserved.
func (m *Manager) ReloadFromFile(path string) (*Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if path == "" {
		path = m.configPath
	}

	newCfg, err := LoadFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("hot reload failed validation: %w", err)
	}

	m.activeConfig.Store(newCfg)
	m.configPath = path
	return newCfg, nil
}

// ReloadFromBytes parses and atomically updates configuration from raw YAML bytes.
func (m *Manager) ReloadFromBytes(data []byte) (*Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	newCfg, err := ParseBytes(data)
	if err != nil {
		return nil, fmt.Errorf("hot reload bytes failed validation: %w", err)
	}

	m.activeConfig.Store(newCfg)
	return newCfg, nil
}
