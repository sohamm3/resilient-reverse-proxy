# Resilient Reverse Proxy

A production-oriented concurrent HTTP reverse proxy and load balancer implemented in pure Go using low-level concurrency, networking, and standard library primitives with zero third-party dependencies.

The project demonstrates low-level systems and backend engineering concepts, including:

- Concurrent request routing
- Reverse proxy internals
- Active + passive health checking
- Retry and failover coordination
- Thread-safe shared state management
- Per-IP token bucket rate limiting
- Structured observability
- Graceful shutdown
- Containerized deployment workflows

The proxy leverages Go concurrency primitives such as:

- `goroutines`
- `sync.RWMutex`
- `sync/atomic`
- `context` cancellation
- channel-based coordination patterns

to implement thread-safe backend management, runtime failure isolation, concurrent health monitoring, and coordinated retry under load.

---

## Features

### Load Balancing

Supports multiple routing algorithms, selected via `config/config.json`:

- Round Robin
- Weighted Round Robin

Weighted routing enables heterogeneous backend clusters where stronger nodes receive proportionally more traffic. If a backend becomes unhealthy, traffic is redistributed across currently healthy backends automatically.

---

### Automatic Retry & Failover

Retries are coordinated independently for each request. On failure, the proxy picks a different healthy backend that hasn't already been tried in that request cycle.

#### Transport-Level Retries

Automatically retries requests on:

- Connection refused (`ECONNREFUSED`)
- TCP reset (`ECONNRESET`)
- EOF / broken upstream connection
- Any `net.Error` (includes backend timeout)

When a transport error occurs, the backend is immediately **marked unhealthy** (passive health check).

#### HTTP Status Retries

Retries on these upstream response codes:

- `502 Bad Gateway`
- `503 Service Unavailable`
- `504 Gateway Timeout`

Retries are transparently redirected to a different healthy backend. Clients are transparently retried against alternate healthy backends whenever possible.

---

### Failure Detection Strategy

The proxy combines both active and passive health checking. The active and passive checks serve different failure-detection roles and work together to improve recovery responsiveness.

#### Active Health Checks

A background health checker goroutine probes every backend (including currently dead ones) on a configurable interval using:

```text
GET <backend-url>/health
```

Only an HTTP `200` response marks a backend healthy. Because dead backends are also probed, this is what enables **automatic recovery** - a backend that was quarantined by a passive check will come back online once it passes an active check.

#### Passive Health Checks

Backends are immediately marked unhealthy during live traffic when:

- A transport failure occurs mid-request
- A retry attempt exhausts all available healthy backends

This provides fast failure isolation without waiting for the next active check cycle

---

### Per-IP Rate Limiting

Implements concurrent token-bucket rate limiting.

Features:

- Per-client-IP buckets
- Burst handling
- Automatic token refill
- Thread-safe bucket management

Protects upstream services from:

- Abusive clients
- Request floods
- Accidental overload

How it works:

- Each IP starts with a full bucket (`burst` tokens)
- Tokens refill at `requests_per_second` per second
- Each request consumes one token
- When the bucket is empty, the request is rejected with `429 Too Many Requests`
- Stale buckets are periodically cleaned up to prevent unbounded memory growth

Rate limiting can be disabled entirely by setting `rate_limit.enabled: false` in config.

---

### Observability & Stats Endpoint

#### Structured Request Logging

Each proxied request emits a log entry. With `logging.format: "json"` (default):

```json
{
  "time": "2025-01-15T10:30:00Z",
  "client": "127.0.0.1",
  "method": "GET",
  "path": "/api/data",
  "backend": "http://localhost:8081",
  "status": 200,
  "duration_ms": 12,
  "algorithm": "weighted-round-robin"
}
```

Set `logging.format` to any other value (e.g. `"text"`) for plain-text output.

The `rate_limited` field appears only when a request is rejected by the rate limiter.

#### Stats Endpoint

Live runtime statistics:

```bash
curl http://localhost:8080/admin/stats
```

Full example response shape:

```json
{
  "algorithm": "weighted-round-robin",
  "total_requests": 125,
  "retry_count": 18,
  "rate_limited_requests": 3,
  "backend_stats": [
    { "url": "http://localhost:8081", "alive": true,  "weight": 5 },
    { "url": "http://localhost:8082", "alive": true,  "weight": 3 },
    { "url": "http://localhost:8083", "alive": false, "weight": 2 }
  ],
  "healthy_backends": 2,
  "unhealthy_backends": 1,
  "uptime": "4m32s",
  "timestamp": "2025-01-15T10:30:00Z"
}
```

---

### Graceful Shutdown

Send `Ctrl+C` (or `SIGTERM`) to the proxy. It will:

The proxy supports graceful termination using `SIGINT` / `SIGTERM`.

1. Stop accepting new incoming connections
2. Wait up to **15 seconds** for active in-flight requests to complete
3. Stop the health checker goroutine via `context` cancellation
4. Exit cleanly

```text
[shutdown] received signal: interrupt - initiating graceful shutdown
[health] checker stopped
[shutdown] proxy has been shut down gracefully
```

---

## Architecture Flow

```text
                                     ┌─────────────────────┐
                                     │       Client        │
                                     └──────────┬──────────┘
                                                │
                                                ▼
                           ┌──────────────────────────────────┐
                           │     Reverse Proxy (:8080)        │
                           │----------------------------------│
                           │ • Rate Limiting                  │
                           │ • Retry Orchestration            │
                           │ • Header Rewriting               │
                           │ • Load Balancing                 │
                           │ • Passive Health Checks          │
                           │ • Structured Logging             │
                           │ • Graceful Shutdown              │
                           └────────────────┬─────────────────┘
                                            │
                                            ▼
                              ┌────────────────────────┐
                              │      Backend Pool      │
                              │------------------------│
                              │ Active Health Checker  │
                              │ probes all backends    │
                              │ including dead nodes   │
                              └──────────┬─────────────┘
                                         │
                ┌────────────────────────┼────────────────────────┐
                ▼                        ▼                        ▼
        ┌────────────────┐      ┌────────────────┐      ┌────────────────┐
        │   Backend 1    │      │   Backend 2    │      │   Backend 3    │
        │   localhost    │      │   localhost    │      │   localhost    │
        │     :8081      │      │     :8082      │      │     :8083      │
        │   Weight: 5    │      │   Weight: 3    │      │   Weight: 2    │
        └────────────────┘      └────────────────┘      └────────────────┘
```

---

## Core Components

| Component | Responsibility |
|---|---|
| Proxy | Request lifecycle management |
| Balancer | Backend selection |
| Retry Engine | Classifies transport errors and HTTP status codes |
| Health Checker | Active backend probing |
| Passive Recovery | Immediate quarantine on transport failure |
| Rate Limiter | Per-IP traffic protection |
| Stats Endpoint | Runtime observability |
| Logger | Structured per-request logging |

---

## Project Structure

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

## Configuration

Configuration is loaded from `config/config.json` at startup.

### Config Field Reference

| Field | Type | Default | Description |
|---|---|---|---|
| `server.port` | int | `8080` | Port the proxy listens on |
| `server.read_timeout` | duration | `"10s"` | HTTP read timeout |
| `server.write_timeout` | duration | `"30s"` | HTTP write timeout |
| `algorithm` | string | `"round-robin"` | Load balancing algorithm |
| `backends[].url` | string | - | Upstream backend address |
| `backends[].weight` | int | `1` | Backend traffic weight |
| `health_check.enabled` | bool | `true` | Enable/disable active health checks |
| `health_check.interval` | duration | `"10s"` | Time between active probe cycles |
| `health_check.timeout` | duration | `"3s"` | Per-probe HTTP timeout |
| `health_check.path` | string | `"/health"` | Endpoint path probed on each backend |
| `rate_limit.enabled` | bool | `true` | Enable/disable per-IP rate limiting |
| `rate_limit.requests_per_second` | float | `5` | Token refill rate per IP |
| `rate_limit.burst` | int | `10` | Bucket burst size (initial token count) |
| `logging.format` | string | `"json"` | Log format |
| `logging.level` | string | `"info"` | Log Verbosity |
| `retry.enabled` | bool | `true` | Enable/disable retry & failover handling |
| `retry.max_attempts` | int | `3` | Maximum retry attempts |

Duration values use Go duration syntax: `"10s"`, `"500ms"`, `"1m30s"`.

---

## Prerequisites

- **Go 1.26+** (see `go.mod`)
- Linux/macOS recommended
- Docker (optional)

Verify Go version:

```bash
go version
```

---

## Quick Start

### 1. Clone Repository

```bash
git clone https://github.com/sohamm3/resilient-reverse-proxy.git
cd resilient-reverse-proxy
```

---

### 2. Build Binaries

Create the build directory:

```bash
mkdir -p build
```

Build the reverse proxy:

```bash
go build -o build/proxyServer ./cmd/proxy
```

Build the test backend servers:

```bash
go build -o build/testServer ./cmd/testserver
```

---

### 3. Start Backend Servers

**Option A - Start all three backends (recommended for first run):**

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

**Option B - Start specific backends only:**

```bash
# Single backend
./build/testbackends 8081
 
# Two specific backends
./build/testbackends 8081 8082
```

---

### 4. Start Reverse Proxy

Open terminal 2:

```bash
./build/proxyServer
```

The proxy reads `config/config.json`, runs an initial health check on all backends, then starts listening:

```text
localhost:8080
```

---

## Request Flow & Forwarded Headers

The proxy rewrites outbound upstream requests and injects forwarding metadata for observability and upstream awareness.

### Inspect Response Headers

```bash
curl -i http://localhost:8080/
```

### Headers Injected Into Upstream Requests (sent to backends)

These headers are forwarded internally to backend servers.

| Header              | Value                                         |
| ------------------- | --------------------------------------------- |
| `X-Forwarded-For`   | Original client IP address chain              |
| `X-Forwarded-Host`  | Original `Host` header from client request    |
| `X-Forwarded-Proto` | Original request protocol (`http` or `https`) |

### Headers Injected Into Downstream Responses (sent back to client)

These headers are returned back to the client.

| Header             | Value                                                     |
| ------------------ | ----------------------------------------------------------|
| `X-Backend-Server` | URL of the backend selected for handling the request      |
| `X-Response-Time`  | Total proxy round-trip latency in milliseconds            |
|                    |                                                           |

---

## Testing

### Normal Request

```bash
curl http://localhost:8080/
```

---

### Observe Weighted Round Robin

```bash
for i in {1..10}; do
  curl -s http://localhost:8080/
done
```

#### Expected Behavior

- Backend 1 appears more frequently
- Backend 2 appears moderately
- Backend 3 appears less frequently

Traffic distribution follows configured backend weights.

---

### Runtime Stats

```bash
curl http://localhost:8080/admin/stats
```

---

### Verify Backend Recovery

#### Step 1 - Start Only One Backend

```bash
./build/testbackends 8081
```

#### Step 2 - Start Reverse Proxy

```bash
./build/proxyServer
```

#### Step 3 - Observe Runtime Stats

```bash
curl http://localhost:8080/admin/stats
```

Expected:

- Backend `8081` appears healthy
- Backends `8082` and `8083` appear unhealthy

#### Step 4 - Start Remaining Backends

```bash
./build/testbackends 8082 8083
```

The active health checker continuously probes unhealthy nodes and automatically restores recovered backends back into rotation.

---

### Rate Limiting Test

Generate concurrent requests:

```bash
for i in {1..50}; do
  curl -s http://localhost:8080/ &
done

wait
```

#### Expected Behavior

- Some requests succeed normally
- Some requests return:

```text
429 Too Many Requests
```

This exercises:

- Concurrency safety (goroutines, atomics, RWMutex)
- Retry handling under partial failure
- Rate limiting (the default config allows 5 req/s burst 10 - tune `rate_limit` in config for high-throughput tests)
- Weighted backend distribution

Check the stats endpoint after the test to see live counters:

```bash
curl http://localhost:8080/admin/stats
```

---

## Failure Simulation

The test backend server has built-in failure injection via query parameters.

### Simulate Transport Failure

Abruptly closes the TCP connection mid-request using HTTP connection hijacking:

```bash
curl "http://localhost:8080/?crash=true"
```

Triggers:

- Passive health check => the backend is immediately marked unhealthy
- Retry failover => the request is retried on a different backend
- Backend quarantine => excluded from routing until active health checks restore it

### Simulate Retryable HTTP Failure

Returns `HTTP 502` from the upstream backend:

```bash
curl "http://localhost:8080/?badgateway=true"
```

Triggers:

- HTTP status retry classification
- Retry on an alternate healthy backend

---

## Load Testing & Failure Validation

The proxy was stress-tested under concurrent workloads using [`hey`](https://github.com/rakyll/hey) with structured JSON logging, active/passive health checks, retry orchestration, and simulated upstream latency enabled.

### Throughput Benchmark

```bash
hey -n 100000 -c 500 http://localhost:8080/
```

| Metric | Value |
|---|---|
| Total Requests | 100,000 |
| Concurrent Clients | 500 |
| Throughput | ~10k req/sec |
| Success Rate | 100% |
| P95 Latency | ~94ms |

Validated concurrent request routing, backend selection, observability overhead, and runtime stability under sustained load.

---

### Retry & Failover Validation

```bash
hey -n 50000 -c 200 "http://localhost:8080/?badgateway=true"
```

Simulated upstream `502 Bad Gateway` failures under concurrent load.

The proxy automatically:
- retried requests on alternate healthy backends
- prevented retry storms using per-request backend exclusion
- maintained successful responses during failure injection

---

### Transport Failure Injection

```bash
hey -n 50000 -c 200 "http://localhost:8080/?crash=true"
```

Validated passive health isolation, transport failure detection, and retry coordination during abrupt upstream TCP connection drops.

---

## Docker

### Build Image

```bash
docker build -t resilient-reverse-proxy .
```

### Run Container

```bash
docker run --network host resilient-reverse-proxy
```

### Current Limitation

The Docker image contains only the **proxy binary**. Backend servers must run separately and be reachable from the container

> `--network host` gives the container direct access to the host's network interfaces, allowing the proxy to reach backends running on `localhost:8081–8083`. This flag is **Linux-only**; on macOS and Windows, replace `localhost` in `config/config.json` with `host.docker.internal` and update the `COPY config` step accordingly.

Future improvements may include:

- docker-compose orchestration
- containerized backend clusters
- Kubernetes deployment manifests
- service discovery integration

---

# Technical Highlights

- Go concurrency primitives (`goroutines`, `sync.RWMutex`, `sync/atomic`)
- Reverse proxy internals (`httputil.ReverseProxy`)
- Active + passive health recovery with automatic backend restoration
- Coordinated retry and failover handling
- Token-bucket rate limiting with double-checked locking for bucket creation
- Context-based graceful shutdown propagated to all background goroutines
- Multi-stage Docker builds producing a minimal, non-root runtime image
- Non-root container execution
- Structured request logging with a thread-safe bounded in-memory request history for recent-request inspection and stats access.
- Zero third-party dependencies

---
