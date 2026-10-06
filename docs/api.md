# FlexiProxy Admin REST API Documentation

The Admin API runs on `:8081` by default and exposes operational status, real-time metrics, configuration, and event streams.

## Endpoints

### 1. GET `/api/v1/status`
Returns general system health, operational status, uptime, active goroutines, memory usage, and backend counts.

**Example Request:**
```bash
curl http://localhost:8081/api/v1/status
```

**Example Response:**
```json
{
  "active_goroutines": 7,
  "backends_healthy": 3,
  "backends_total": 3,
  "load_balancer_algorithm": "round_robin",
  "memory_alloc_mb": 0.467,
  "status": "OPERATIONAL",
  "system": "FlexiProxy",
  "uptime_seconds": 42,
  "version": "0.1.0"
}
```

### 2. GET `/api/v1/backends`
Lists all configured upstream backend targets, their active health, latency, total requests, and circuit state.

**Example Request:**
```bash
curl http://localhost:8081/api/v1/backends
```

### 3. GET `/metrics` or `/api/v1/metrics`
Exposes Prometheus-compatible metrics text representation.

**Example Request:**
```bash
curl http://localhost:8081/metrics
```

### 4. GET `/api/v1/circuit-breakers`
Returns map of backend IDs to current circuit state (`CLOSED`, `OPEN`, `HALF_OPEN`).

### 5. GET `/api/v1/events` (or `/api/v1/ws`)
Real-time Server-Sent Events (SSE) stream broadcasting backend updates to connected dashboard clients.
