# FlexiProxy Architecture Overview

FlexiProxy is a high-throughput, event-driven HTTP reverse proxy and load balancer built in Go. It provides real-time traffic management, health monitoring, circuit breaking, passive observability, and adaptive resource-aware routing.

## Architecture Diagram

```mermaid
flowchart TD
    Client["Client / External Traffic"] -->|HTTP Request| Proxy["FlexiProxy Core Listener (:8080)"]
    
    subgraph FlexiProxy ["FlexiProxy Engine"]
        Proxy --> ReqID["Request ID Middleware"]
        ReqID --> LB["Load Balancer Engine"]
        LB --> CB["Circuit Breaker Evaluation"]
        CB --> Transport["Optimized Transport Pool"]
        
        subgraph Subsystems ["Background Subsystems"]
            ActiveHealthCheck["Active Health Checker"]
            PassiveMonitor["Passive Traffic Observer"]
            MetricsCollector["Prometheus Metrics Collector"]
            WSNotifier["WebSocket Event Stream"]
            AdminAPI["Admin REST API (:8081)"]
        end

        Transport --> RetryLoop["Retry & Failover Controller"]
    end

    RetryLoop -->|HTTP Proxy Pass| B1["Backend 1 (:8001)"]
    RetryLoop -->|HTTP Proxy Pass| B2["Backend 2 (:8002)"]
    RetryLoop -->|HTTP Proxy Pass| B3["Backend 3 (:8003)"]

    ActiveHealthCheck -->|Poll /health| B1
    ActiveHealthCheck -->|Poll /health| B2
    ActiveHealthCheck -->|Poll /health| B3

    PassiveMonitor -.->|Track 5xx/Errors| CB
    ActiveHealthCheck -.->|Update State| LB
```

## Core Components

1. **HTTP Proxy Core**: Uses Go's `net/http/httputil.ReverseProxy` with customized transport connection pooling, context cancellation propagation, header injection (`X-Request-ID`), and streaming response preservation.
2. **Load Balancers**: Extensible `LoadBalancer` interface supporting:
   - **Round Robin**: Sequential distribution across healthy backends.
   - **Weighted Round Robin**: Proportional traffic distribution based on assigned backend weights.
   - **Least Connections**: Prefers backends with the lowest active request concurrency.
   - **Adaptive Resource-Aware**: Configurable heuristic scoring based on measured latency, active connections, CPU/Memory metrics, and recent error rates.
3. **Active Health Checker**: Periodically sends probe requests to `/health` endpoints and transitions backend health status based on configurable thresholds.
4. **Passive Health Monitor**: Monitors live user traffic status codes, transport errors, and latency to detect degraded backends between active checks.
5. **Circuit Breakers**: Concurrency-safe state machines (`CLOSED`, `OPEN`, `HALF_OPEN`) preventing cascading backend overload.
6. **Retry & Failover Unit**: Safe retry execution (idempotency checks for non-mutating methods or configured failure codes) with failover to alternate healthy backends.
7. **Observability Suite**: Prometheus `/metrics` endpoint, structured JSON logging (`slog`), and real-time WebSocket broadcast channel powering the admin dashboard.
