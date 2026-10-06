# FlexiProxy Configuration Guide

FlexiProxy uses YAML for centralized runtime configuration.

## Sample Configuration (`configs/flexiproxy.yaml`)

```yaml
server:
  port: 8080
  admin_port: 8081
  read_timeout: 10s
  write_timeout: 10s
  idle_timeout: 120s

load_balancer:
  algorithm: round_robin # Options: round_robin, weighted_round_robin, least_connections, adaptive
  adaptive_weights:
    latency: 0.35
    active_connections: 0.25
    cpu: 0.15
    memory: 0.10
    error_rate: 0.15

health_check:
  enabled: true
  path: /health
  interval: 2s
  timeout: 1s
  healthy_threshold: 2
  unhealthy_threshold: 3

circuit_breaker:
  enabled: true
  failure_threshold: 5
  open_duration: 10s
  half_open_requests: 3

retry:
  enabled: true
  max_attempts: 2
  retry_on:
    - connection_error
    - timeout
    - 502
    - 503
    - 504

backends:
  - id: backend-1
    address: http://localhost:8001
    weight: 1
  - id: backend-2
    address: http://localhost:8002
    weight: 1
  - id: backend-3
    address: http://localhost:8003
    weight: 1
```

## Parameter Reference

| Section | Key | Type | Description |
|---|---|---|---|
| `server` | `port` | int | Port for client HTTP reverse proxy traffic |
| `server` | `admin_port` | int | Port for Admin API, Prometheus `/metrics`, and Dashboard |
| `load_balancer` | `algorithm` | string | `round_robin`, `weighted_round_robin`, `least_connections`, `adaptive` |
| `health_check` | `interval` | duration | Interval between active health check probes |
| `circuit_breaker`| `failure_threshold` | int | Consecutive failures before opening circuit |
| `retry` | `max_attempts` | int | Max retry attempts for idempotent requests |
