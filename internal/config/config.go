package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Algorithm names supported by FlexiProxy.
const (
	AlgorithmRoundRobin         = "round_robin"
	AlgorithmWeightedRoundRobin = "weighted_round_robin"
	AlgorithmLeastConnections   = "least_connections"
	AlgorithmAdaptive           = "adaptive"
)

// RateLimitConfig defines client request throttling parameters.
type RateLimitConfig struct {
	Enabled bool    `yaml:"enabled"`
	RPS     float64 `yaml:"rps"`
	Burst   int     `yaml:"burst"`
}

// TLSConfig holds HTTPS listening configuration options.
type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// ServerConfig holds HTTP server listening and timeout options.
type ServerConfig struct {
	Port         int             `yaml:"port"`
	AdminPort    int             `yaml:"admin_port"`
	AdminAPIKey  string          `yaml:"admin_api_key"`
	ReadTimeout  time.Duration   `yaml:"read_timeout"`
	WriteTimeout time.Duration   `yaml:"write_timeout"`
	IdleTimeout  time.Duration   `yaml:"idle_timeout"`
	MaxBodySize  int64           `yaml:"max_body_size"` // Max body size in bytes (e.g. 10MB)
	TLS          TLSConfig       `yaml:"tls"`
	RateLimit    RateLimitConfig `yaml:"rate_limit"`
}

// AdaptiveWeightsConfig holds scoring weights for adaptive load balancing.
type AdaptiveWeightsConfig struct {
	Latency           float64 `yaml:"latency"`
	ActiveConnections float64 `yaml:"active_connections"`
	CPU               float64 `yaml:"cpu"`
	Memory            float64 `yaml:"memory"`
	ErrorRate         float64 `yaml:"error_rate"`
}

// LoadBalancerConfig configures load balancing strategy.
type LoadBalancerConfig struct {
	Algorithm       string                `yaml:"algorithm"`
	AdaptiveWeights AdaptiveWeightsConfig `yaml:"adaptive_weights"`
}

// HealthCheckConfig defines parameters for active backend probing.
type HealthCheckConfig struct {
	Enabled            bool          `yaml:"enabled"`
	Path               string        `yaml:"path"`
	Interval           time.Duration `yaml:"interval"`
	Timeout            time.Duration `yaml:"timeout"`
	HealthyThreshold   int           `yaml:"healthy_threshold"`
	UnhealthyThreshold int           `yaml:"unhealthy_threshold"`
}

// CircuitBreakerConfig defines threshold rules for individual backend circuit breakers.
type CircuitBreakerConfig struct {
	Enabled          bool          `yaml:"enabled"`
	FailureThreshold int           `yaml:"failure_threshold"`
	OpenDuration     time.Duration `yaml:"open_duration"`
	HalfOpenRequests int           `yaml:"half_open_requests"`
}

// RetryConfig specifies rules for retrying failed HTTP requests.
type RetryConfig struct {
	Enabled     bool     `yaml:"enabled"`
	MaxAttempts int      `yaml:"max_attempts"`
	RetryOn     []string `yaml:"retry_on"`
}

// BackendConfig represents a single downstream target server.
type BackendConfig struct {
	ID      string   `yaml:"id"`
	Address string   `yaml:"address"`
	Weight  int      `yaml:"weight"`
	URL     *url.URL `yaml:"-"` // Parsed URL populated upon validation
}

// Config represents the complete FlexiProxy configuration schema.
type Config struct {
	Server         ServerConfig         `yaml:"server"`
	LoadBalancer   LoadBalancerConfig   `yaml:"load_balancer"`
	HealthCheck    HealthCheckConfig    `yaml:"health_check"`
	CircuitBreaker CircuitBreakerConfig `yaml:"circuit_breaker"`
	Retry          RetryConfig          `yaml:"retry"`
	Backends       []BackendConfig      `yaml:"backends"`
}

// SetDefaults assigns sensible default values for unassigned configuration fields.
func (c *Config) SetDefaults() {
	if c.Server.Port <= 0 {
		c.Server.Port = 8080
	}
	if c.Server.AdminPort <= 0 {
		c.Server.AdminPort = 8081
	}
	if c.Server.ReadTimeout <= 0 {
		c.Server.ReadTimeout = 10 * time.Second
	}
	if c.Server.WriteTimeout <= 0 {
		c.Server.WriteTimeout = 10 * time.Second
	}
	if c.Server.IdleTimeout <= 0 {
		c.Server.IdleTimeout = 120 * time.Second
	}
	if c.Server.MaxBodySize <= 0 {
		c.Server.MaxBodySize = 10 * 1024 * 1024 // 10 MB Default
	}
	if c.Server.RateLimit.RPS <= 0 {
		c.Server.RateLimit.RPS = 1000
	}
	if c.Server.RateLimit.Burst <= 0 {
		c.Server.RateLimit.Burst = 2000
	}

	if c.LoadBalancer.Algorithm == "" {
		c.LoadBalancer.Algorithm = AlgorithmRoundRobin
	}
	if c.LoadBalancer.Algorithm == AlgorithmAdaptive {
		w := &c.LoadBalancer.AdaptiveWeights
		if w.Latency == 0 && w.ActiveConnections == 0 && w.CPU == 0 && w.Memory == 0 && w.ErrorRate == 0 {
			w.Latency = 0.35
			w.ActiveConnections = 0.25
			w.CPU = 0.15
			w.Memory = 0.10
			w.ErrorRate = 0.15
		}
	}

	if c.HealthCheck.Path == "" {
		c.HealthCheck.Path = "/health"
	}
	if c.HealthCheck.Interval <= 0 {
		c.HealthCheck.Interval = 2 * time.Second
	}
	if c.HealthCheck.Timeout <= 0 {
		c.HealthCheck.Timeout = 1 * time.Second
	}
	if c.HealthCheck.HealthyThreshold <= 0 {
		c.HealthCheck.HealthyThreshold = 2
	}
	if c.HealthCheck.UnhealthyThreshold <= 0 {
		c.HealthCheck.UnhealthyThreshold = 3
	}

	if c.CircuitBreaker.FailureThreshold <= 0 {
		c.CircuitBreaker.FailureThreshold = 5
	}
	if c.CircuitBreaker.OpenDuration <= 0 {
		c.CircuitBreaker.OpenDuration = 10 * time.Second
	}
	if c.CircuitBreaker.HalfOpenRequests <= 0 {
		c.CircuitBreaker.HalfOpenRequests = 3
	}

	if c.Retry.MaxAttempts <= 0 {
		c.Retry.MaxAttempts = 2
	}
	if len(c.Retry.RetryOn) == 0 {
		c.Retry.RetryOn = []string{"connection_error", "timeout", "502", "503", "504"}
	}

	for i := range c.Backends {
		if c.Backends[i].Weight <= 0 {
			c.Backends[i].Weight = 1
		}
	}
}

// Validate verifies the syntactic and semantic correctness of the configuration.
func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d (must be between 1 and 65535)", c.Server.Port)
	}
	if c.Server.AdminPort <= 0 || c.Server.AdminPort > 65535 {
		return fmt.Errorf("invalid server admin_port: %d (must be between 1 and 65535)", c.Server.AdminPort)
	}
	if c.Server.Port == c.Server.AdminPort {
		return fmt.Errorf("server port (%d) and admin_port (%d) cannot be identical", c.Server.Port, c.Server.AdminPort)
	}

	if c.Server.TLS.Enabled {
		if c.Server.TLS.CertFile == "" || c.Server.TLS.KeyFile == "" {
			return errors.New("server.tls.cert_file and server.tls.key_file must be specified when TLS is enabled")
		}
	}

	alg := strings.ToLower(c.LoadBalancer.Algorithm)
	switch alg {
	case AlgorithmRoundRobin, AlgorithmWeightedRoundRobin, AlgorithmLeastConnections, AlgorithmAdaptive:
		c.LoadBalancer.Algorithm = alg
	default:
		return fmt.Errorf("unsupported load_balancer algorithm: %q (supported: round_robin, weighted_round_robin, least_connections, adaptive)", c.LoadBalancer.Algorithm)
	}

	if len(c.Backends) == 0 {
		return errors.New("at least one backend must be defined in backends configuration")
	}

	seenIDs := make(map[string]bool)
	for i := range c.Backends {
		b := &c.Backends[i]
		if strings.TrimSpace(b.ID) == "" {
			return fmt.Errorf("backend at index %d has empty ID", i)
		}
		if seenIDs[b.ID] {
			return fmt.Errorf("duplicate backend ID detected: %q", b.ID)
		}
		seenIDs[b.ID] = true

		if strings.TrimSpace(b.Address) == "" {
			return fmt.Errorf("backend %q has empty address", b.ID)
		}

		u, err := url.Parse(b.Address)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("backend %q address %q is not a valid absolute URL (e.g. http://localhost:8001)", b.ID, b.Address)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("backend %q scheme %q is unsupported; must be http or https", b.ID, u.Scheme)
		}
		b.URL = u

		if b.Weight <= 0 {
			return fmt.Errorf("backend %q has invalid weight %d (must be > 0)", b.ID, b.Weight)
		}
	}

	if c.HealthCheck.Enabled {
		if !strings.HasPrefix(c.HealthCheck.Path, "/") {
			return fmt.Errorf("health_check path %q must start with '/'", c.HealthCheck.Path)
		}
		if c.HealthCheck.Interval <= 0 {
			return errors.New("health_check interval must be greater than 0")
		}
		if c.HealthCheck.Timeout <= 0 {
			return errors.New("health_check timeout must be greater than 0")
		}
		if c.HealthCheck.HealthyThreshold <= 0 {
			return errors.New("health_check healthy_threshold must be greater than 0")
		}
		if c.HealthCheck.UnhealthyThreshold <= 0 {
			return errors.New("health_check unhealthy_threshold must be greater than 0")
		}
	}

	if c.Retry.Enabled {
		if c.Retry.MaxAttempts <= 0 {
			return errors.New("retry max_attempts must be greater than 0 when retries are enabled")
		}
	}

	return nil
}

// ParseBytes parses, populates defaults, and validates configuration from raw bytes.
func ParseBytes(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML configuration: %w", err)
	}
	cfg.SetDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	return &cfg, nil
}

// LoadFromFile reads, parses, defaults, and validates configuration from a YAML file.
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}
	return ParseBytes(data)
}
