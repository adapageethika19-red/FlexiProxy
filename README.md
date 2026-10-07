# FlexiProxy

**FlexiProxy: A High-Throughput Event-Driven Reverse Proxy with Dynamic Load Balancing, Health Monitoring, Failover, Circuit Breaking, Observability, and Resource-Aware Routing.**

![Go Version](https://img.shields.io/badge/Go-1.22-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)
![Deployment](https://img.shields.io/badge/Deployment-Render-purple)
![Status](https://img.shields.io/badge/Status-Live-success)

---

## 🚀 Live Deployment

FlexiProxy is deployed as a publicly accessible cloud service.

### 🌐 Live Proxy

**https://flexiproxy-1j4q.onrender.com**

### 📊 Live Monitoring Dashboard

**https://flexiproxy-1j4q.onrender.com/dashboard/**

The live dashboard provides real-time visibility into:

- Proxy status
- Backend server health
- Request distribution
- Response latency
- Active connections
- CPU and memory usage
- Circuit breaker state
- Load-balancing activity
- Backend availability
- Automatic failover

> **No local installation is required to access the live deployment.**
> The proxy and dashboard can be opened from any laptop or browser with an internet connection.

---

## 📖 Overview

FlexiProxy is an open-source HTTP reverse proxy and load balancer built in Go.

It sits between client applications and backend services and intelligently manages incoming HTTP traffic.

Instead of allowing clients to communicate directly with a single backend server, requests first pass through FlexiProxy. The proxy then selects a healthy backend using the configured load-balancing strategy.

FlexiProxy continuously monitors backend health and can automatically reroute traffic when a backend becomes unavailable.

### Main Responsibilities

1. Receive incoming client requests.
2. Select an appropriate backend server.
3. Forward the request to the selected backend.
4. Monitor backend health and performance.
5. Detect backend failures.
6. Prevent traffic from being sent to unhealthy servers.
7. Retry failed requests when appropriate.
8. Automatically reroute traffic to healthy backends.
9. Expose real-time monitoring and metrics.

---

## 🏗️ System Architecture

### Production Architecture

```mermaid
flowchart TD
    Client["Client / Browser"] --> Proxy["FlexiProxy Cloud"]

    subgraph FlexiProxy["FlexiProxy Runtime"]
        Proxy --> RequestID["Request ID Middleware"]
        RequestID --> LB["Load Balancer"]
        LB --> CB["Circuit Breaker"]
        CB --> Health["Health Evaluation"]
        Health --> Transport["HTTP Transport Pool"]
        Transport --> Retry["Retry & Failover"]
    end

    Retry --> B1["Backend 1"]
    Retry --> B2["Backend 2"]
    Retry --> B3["Backend 3"]

    Monitor["Monitoring Dashboard"] --> Admin["Admin API"]
    Admin --> FlexiProxy

    Health --> B1
    Health --> B2
    Health --> B3
