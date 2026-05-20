# Resilient Reverse Proxy

A production-oriented concurrent HTTP reverse proxy and load balancer built entirely from scratch in pure Go.

This project focuses on:

- concurrent request routing
- active + passive health checking
- retry orchestration
- reverse proxy internals
- rate limiting
- observability
- graceful shutdown
- containerization

Built entirely using the Go standard library without third-party dependencies.

---

## Features

### Load Balancing

Supports multiple routing algorithms:

- Round Robin
- Weighted Round Robin

Weighted routing enables heterogeneous backend clusters where stronger nodes receive proportionally more traffic.

---

### Automatic Retry & Failover

Implements lightweight retry orchestration for transient upstream failures.

#### Transport-Level Retries

Automatically retries requests on:

- connection refused
- TCP reset
- EOF
- broken upstream connection
- backend timeout

#### Retryable HTTP Status Retries

Retries selected upstream responses:

- 502 Bad Gateway
- 503 Service Unavailable
- 504 Gateway Timeout

Retries are transparently redirected to alternative healthy backends.

---

### Active + Passive Health Checking

#### Active Health Checks

Background health checker continuously probes:

```text
/health
```

Dead backends are automatically restored when healthy again.

#### Passive Health Checks

Backends are immediately marked unhealthy when:

- transport failures occur
- upstream connections break
- retry attempts fail

This enables fast runtime failure isolation under real traffic.

---

## Per-IP Rate Limiting

Implements concurrent token-bucket rate limiting.

Features:

- per-client-IP buckets
- burst handling
- automatic token refill
- thread-safe bucket management

Protects upstream services from:

- abusive clients
- request floods
- accidental overload

---

## Observability & Stats Endpoint

Structured JSON logging includes:

- request path
- backend selection
- response status
- retry activity
- latency
- rate-limit events

Live runtime statistics available through:

```bash
curl http://localhost:8080/admin/stats
```

Example response:

```json
{
  "algorithm": "weighted-round-robin",
  "total_requests": 1250,
  "retry_count": 18,
  "healthy_backends": 3,
  "unhealthy_backends": 0
}
```

---

# Architecture Flow

```text
                ┌─────────────────────┐
                │       Client        │
                └─────────┬───────────┘
                          │
                          ▼
              ┌─────────────────────┐
              │   Reverse Proxy     │
              │       :8080         │
              └─────────┬───────────┘
                        │
        ┌───────────────┼────────────────┐
        │               │                │
        ▼               ▼                ▼

 ┌────────────┐  ┌────────────┐  ┌────────────┐
 │ Backend 1  │  │ Backend 2  │  │ Backend 3  │
 │   :8081    │  │   :8082    │  │   :8083    │
 └────────────┘  └────────────┘  └────────────┘
```

---

# Core Components

| Component | Responsibility |
|---|---|
| Proxy | Request lifecycle orchestration |
| Balancer | Backend selection |
| Retry Engine | Retry decisions |
| Health Checker | Active backend probing |
| Passive Recovery | Runtime failure isolation |
| Rate Limiter | Per-IP traffic protection |
| Stats Endpoint | Runtime observability |
| Logger | Structured request logging |

---

# Project Structure

```text
.
├── Dockerfile
├── LICENSE
├── README.md
├── build
├── cmd
│   ├── proxy
│   │   └── main.go
│   └── testserver
│       └── main.go
├── config
│   └── config.json
├── go.mod
└── internal
    ├── backend
    │   ├── backend.go
    │   └── pool.go
    ├── balancer
    │   ├── balancer.go
    │   ├── roundrobin.go
    │   └── weightedrr.go
    ├── config
    │   └── config.go
    ├── health
    │   └── checker.go
    ├── logger
    │   └── logger.go
    ├── proxy
    │   ├── proxy.go
    │   └── retry.go
    ├── ratelimit
    │   └── limiter.go
    └── stats
        └── stats.go
```

---

# Quick Start

## 1. Clone Repository

```bash
git clone <your-repository-url>
cd resilient-reverse-proxy
```

---

## 2. Build Binaries

Create build directory:

```bash
mkdir -p build
```

Build reverse proxy:

```bash
go build -o build/proxyServer ./cmd/proxy
```

Build backend test servers:

```bash
go build -o build/testbackends ./cmd/testserver
```

---

## 3. Start Backend Servers

Open terminal 1:

```bash
./build/testbackends
```

This launches:

| Backend | Port |
|---|---|
| Backend 1 | 8081 |
| Backend 2 | 8082 |
| Backend 3 | 8083 |

---

## 4. Start Reverse Proxy

Open terminal 2:

```bash
./build/proxyServer
```

Proxy runs on:

```text
localhost:8080
```

---

# Testing

## Basic Request

```bash
curl http://localhost:8080/
```

---

## Inspect Response Headers

```bash
curl -i http://localhost:8080/
```

Injected headers include:

- X-Backend-Server
- X-Response-Time
- X-Forwarded-For
- X-Forwarded-Proto

---

## Stats Endpoint

```bash
curl http://localhost:8080/admin/stats
```

---

# Failure Simulation

The test backend server supports runtime failure injection.

---

## Simulate Transport Failure

Drops upstream TCP connection mid-request:

```bash
curl "http://localhost:8080/?crash=true"
```

Triggers:

- passive health checks
- retry failover
- backend quarantine

---

## Simulate Retryable HTTP Failure

Returns HTTP 502 from upstream:

```bash
curl "http://localhost:8080/?badgateway=true"
```

Triggers:

- retry orchestration
- alternate backend selection

---

# Load Testing

Example using hey:

```bash
hey -n 10000 -c 200 http://localhost:8080/
```

Tests:

- concurrency safety
- retry handling
- rate limiting
- backend balancing

---

# Docker

## Build Image

```bash
docker build -t resilient-reverse-proxy .
```

---

## Run Container

```bash
docker run --network host resilient-reverse-proxy
```

---

# Configuration

Configuration is loaded from:

```text
config/config.json
```

Example settings:

| Setting | Description |
|---|---|
| algorithm | balancing algorithm |
| backends | upstream backend list |
| retry.max_attempts | retry count |
| health_check.interval | active probe interval |
| rate_limit.requests_per_second | token refill rate |
| rate_limit.burst | bucket burst size |

---

# Technical Highlights

- Go concurrency primitives (`goroutines`, `RWMutex`, `atomic`)
- Reverse proxy internals (`httputil.ReverseProxy`)
- Active + passive recovery patterns
- Retry orchestration
- Token bucket rate limiting
- Graceful shutdown
- Multi-stage Docker builds
- Non-root container execution
- Structured logging
- Zero third-party dependencies

---