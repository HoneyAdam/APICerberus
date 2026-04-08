# API Cerberus — API Gateway & Management Platform

## SPECIFICATION.md

> **API Cerberus** — Yunan mitolojisinde yeraltı dünyasının üç başlı kapı bekçisi.
> Üç baş = HTTP/HTTPS + gRPC + GraphQL. Hiçbir istek kontrolsüz geçemez.

---

## 1. Project Overview

**API Cerberus** is a full-stack API Gateway, API Management, and API Monetization Platform written in pure Go with minimal, curated external dependencies. It provides L7 protocol proxying (HTTP/HTTPS, gRPC, GraphQL federation), multi-tenant user/client management, credit-based billing, per-endpoint access control, full request/response audit logging, rate limiting, load balancing, request/response transformation, real-time analytics, and a plugin/middleware pipeline — all in a single self-contained binary.

**Two interfaces**: Admin Panel (full platform control) and User Portal (self-service API management).

### Core Philosophy

- **Minimal, Curated Dependencies**: Core logic relies on the Go standard library; a small set of vetted third-party libraries is used for SQLite (`modernc.org/sqlite`), Redis (`go-redis`), OpenTelemetry, and gRPC.
- **Single Binary**: One executable contains the gateway, admin API, user portal, embedded web dashboard, MCP server, and Raft clustering.
- **Configuration-Driven**: In-memory runtime state with YAML/JSON file-based configuration. Embedded SQLite for user/credit/log data.
- **Commercially Viable**: Built-in credit system, usage billing, and access control — sell API access out of the box.
- **Production-Grade**: Built for real-world API management at scale with enterprise features.
- **Kong Killer**: Feature parity with Kong/Tyk/KrakenD but without the dependency hell (Kong has 200+ dependencies).

### Architecture Summary

```
                    ┌──────────────────────────────────────────────────────┐
                    │                API CERBERUS PLATFORM                  │
                    │                                                      │
  API Clients ────▶ │  ┌──────────┐  ┌──────────┐  ┌───────────┐          │ ────▶ Upstream APIs
  (HTTP/gRPC/GQL)   │  │ Listener │─▶│ Pipeline │─▶│  Router   │          │
                    │  │ (TLS)    │  │(Plugins) │  │(Balancer) │          │
                    │  └──────────┘  └──────────┘  └───────────┘          │
                    │        │             │                               │
                    │        ▼             ▼                               │
                    │  ┌──────────┐  ┌──────────┐  ┌───────────┐          │
                    │  │  Auth &  │  │  Credit  │  │  Audit    │          │
                    │  │  ACL     │  │  Engine  │  │  Logger   │          │
                    │  └──────────┘  └──────────┘  └───────────┘          │
                    │                                                      │
                    │  ┌──────────────────────┐  ┌───────────────────────┐ │
                    │  │    Admin Panel       │  │    User Portal        │ │
                    │  │  (Platform Control)  │  │  (Self-Service)       │ │
                    │  └──────────────────────┘  └───────────────────────┘ │
                    │                                                      │
                    │  ┌──────────┐  ┌──────────┐  ┌───────────┐          │
                    │  │Analytics │  │   Raft   │  │    MCP    │          │
                    │  │ Engine   │  │ Cluster  │  │  Server   │          │
                    │  └──────────┘  └──────────┘  └───────────┘          │
                    │                                                      │
                    │  ┌──────────────────────────────────────────────┐    │
                    │  │  Embedded SQLite (users, credits, audit log) │    │
                    │  └──────────────────────────────────────────────┘    │
                    └──────────────────────────────────────────────────────┘
```

---

## 2. Protocol Support (Three Heads of API Cerberus)

### 2.1 Head 1: HTTP/HTTPS Gateway

- Full HTTP/1.1 and HTTP/2 reverse proxy
- TLS termination with automatic certificate management (ACME/Let's Encrypt)
- SNI-based virtual hosting
- WebSocket proxying (upgrade handling)
- HTTP/2 to upstream support (h2c and h2)
- Keep-alive connection pooling to upstreams
- Request buffering and streaming modes
- Custom error pages per route/service

### 2.2 Head 2: gRPC Gateway

- Native gRPC proxying (HTTP/2 based)
- gRPC-Web support (browser clients)
- gRPC health checking for upstreams
- gRPC metadata manipulation (headers)
- Unary, server-streaming, client-streaming, bidirectional streaming
- gRPC reflection proxy (pass-through)
- gRPC ↔ JSON transcoding (REST-to-gRPC bridge)
- Protocol detection (auto-detect gRPC vs HTTP)

### 2.3 Head 3: GraphQL Federation

- GraphQL query proxying to upstream GraphQL services
- Schema federation (compose multiple GraphQL services)
- Query depth limiting (prevent abuse)
- Query complexity analysis and cost limiting
- Introspection control (enable/disable per environment)
- Field-level authorization
- Automatic persisted queries (APQ) support
- Query batching support
- Subscription proxying (WebSocket-based)

---

## 3. Core Gateway Features

### 3.1 Routing Engine

```yaml
# Route matching hierarchy (highest to lowest priority):
# 1. Exact path match:     /api/v1/users
# 2. Prefix match:         /api/v1/*
# 3. Regex match:          /api/v[0-9]+/users/[0-9]+
# 4. Host-based routing:   api.example.com
# 5. Header-based routing: X-Version: v2
# 6. Method-based routing: GET /users vs POST /users
# 7. Query param routing:  /search?type=advanced
```

- **Services**: Logical grouping of upstream targets (e.g., "user-service")
- **Routes**: Map incoming requests to services based on host, path, headers, methods
- **Upstreams**: Backend server pools with health checking and load balancing
- **Consumers**: API clients identified by credentials (API key, JWT)

### 3.2 Plugin/Middleware Pipeline

API Cerberus uses a **middleware chain** architecture. Every request passes through an ordered pipeline of plugins:

```
Request ──▶ [Auth] ──▶ [RateLimit] ──▶ [Transform] ──▶ [Log] ──▶ Upstream
                                                                      │
Response ◀── [Transform] ◀── [Analytics] ◀── [Cors] ◀──────────────────┘
```

#### Plugin Execution Order (configurable):
1. **Pre-auth plugins**: IP whitelist/blacklist, CORS preflight
2. **Authentication**: API Key validation, JWT verification
3. **Post-auth plugins**: Rate limiting (consumer-aware), ACL
4. **Request transformation**: Header injection, body rewrite, URL rewrite
5. **Proxy**: Forward to upstream
6. **Response transformation**: Header manipulation, body rewrite
7. **Post-response plugins**: Analytics recording, logging

#### Built-in Plugins:

| Plugin | Phase | Description |
|--------|-------|-------------|
| `auth-apikey` | auth | API Key authentication (header, query, cookie) |
| `auth-jwt` | auth | JWT validation (RS256, HS256) |
| `rate-limit` | pre-proxy | Multi-algorithm rate limiting |
| `acl` | pre-proxy | Access Control Lists (consumer groups) |
| `cors` | pre-auth | Cross-Origin Resource Sharing |
| `ip-restrict` | pre-auth | IP whitelist/blacklist |
| `request-transform` | pre-proxy | Modify request headers, body, query, path |
| `response-transform` | post-proxy | Modify response headers, body, status |
| `request-size-limit` | pre-proxy | Limit request body size |
| `bot-detect` | pre-auth | User-agent based bot detection |
| `request-validator` | pre-proxy | JSON Schema validation for request body |
| `url-rewrite` | pre-proxy | Path rewriting with regex support |
| `redirect` | pre-proxy | HTTP redirect rules |
| `retry` | proxy | Automatic retry with backoff |
| `circuit-breaker` | proxy | Circuit breaker pattern for upstreams |
| `timeout` | proxy | Per-route timeout configuration |
| `cache` | post-proxy | Response caching (in-memory) |
| `compression` | post-proxy | gzip/brotli response compression |
| `analytics` | post-proxy | Request/response metrics collection |
| `logger` | post-proxy | Structured logging (JSON) |
| `correlation-id` | pre-auth | Request tracing with correlation IDs |
| `grpc-transcode` | pre-proxy | REST ↔ gRPC transcoding |
| `graphql-guard` | pre-proxy | GraphQL depth/complexity limiting |

#### Custom Plugin Interface:

```go
// Plugin interface — all built-in and custom plugins implement this
type Plugin interface {
    // Name returns the plugin identifier
    Name() string
    
    // Priority returns execution order (lower = earlier)
    Priority() int
    
    // Phase returns when this plugin executes
    Phase() PluginPhase // PreAuth, Auth, PreProxy, Proxy, PostProxy
    
    // Execute runs the plugin logic
    Execute(ctx *RequestContext) error
    
    // Configure applies plugin-specific config
    Configure(config map[string]any) error
}

// RequestContext carries all request/response state through the pipeline
type RequestContext struct {
    Request        *ProxyRequest
    Response       *ProxyResponse
    Route          *Route
    Service        *Service
    Consumer       *Consumer
    Upstream       *Upstream
    Metadata       map[string]any
    StartTime      time.Time
    CorrelationID  string
}
```

### 3.3 Authentication

#### API Key Authentication
```yaml
# API keys can be sent via:
# - Header: X-API-Key, Authorization: Bearer <key>, or custom header
# - Query parameter: ?apikey=xxx
# - Cookie: apikey=xxx

consumers:
  - name: "mobile-app"
    api_keys:
      - key: "ck_live_abc123..."
        created_at: "2025-01-01T00:00:00Z"
        expires_at: "2026-01-01T00:00:00Z"
    rate_limit:
      requests_per_second: 100
      burst: 150
    acl_groups: ["mobile-tier"]
```

#### JWT Authentication
```yaml
jwt:
  # Validation modes
  algorithms: ["RS256", "HS256"]
  
  # Key sources
  secret: "your-hmac-secret"           # For HS256
  jwks_url: "https://auth.example.com/.well-known/jwks.json"  # For RS256
  
  # Claim mapping
  consumer_claim: "sub"                 # Map JWT claim to consumer
  claims_to_headers:
    sub: "X-Consumer-ID"
    email: "X-Consumer-Email"
    roles: "X-Consumer-Roles"
  
  # Validation rules
  required_claims: ["sub", "exp"]
  audience: "api.example.com"
  issuer: "auth.example.com"
  clock_skew: "30s"
```

### 3.4 Rate Limiting

Four algorithms, all configurable per-route, per-consumer, or globally:

#### Token Bucket (burst-friendly)
```yaml
rate_limit:
  algorithm: "token_bucket"
  capacity: 100        # Max tokens (burst size)
  refill_rate: 10      # Tokens per second
  scope: "consumer"    # global | consumer | ip | route | consumer+route
```

#### Sliding Window (precise)
```yaml
rate_limit:
  algorithm: "sliding_window"
  requests: 1000
  window: "1m"         # 1 minute sliding window
  precision: "1s"      # Sub-window precision
  scope: "consumer"
```

#### Fixed Window (simple)
```yaml
rate_limit:
  algorithm: "fixed_window"
  requests: 10000
  window: "1h"         # Reset every hour
  scope: "global"
```

#### Leaky Bucket (smooth)
```yaml
rate_limit:
  algorithm: "leaky_bucket"
  capacity: 50         # Queue size
  leak_rate: 10        # Requests drained per second
  scope: "ip"
```

**Rate Limit Response Headers:**
```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 87
X-RateLimit-Reset: 1706889600
Retry-After: 30        # Only on 429 responses
```

### 3.5 Load Balancing

Ten algorithms for upstream traffic distribution:

| Algorithm | Description | Use Case |
|-----------|-------------|----------|
| **Round Robin** | Sequential rotation | General purpose |
| **Weighted Round Robin** | Weighted sequential | Heterogeneous upstreams |
| **Least Connections** | Fewest active connections | Long-lived connections |
| **IP Hash** | Client IP affinity | Session persistence |
| **Random** | Random selection | Simple distribution |
| **Consistent Hash** | Ring-based hashing | Cache-friendly routing |
| **Least Latency** | Lowest response time | Latency-sensitive APIs |
| **Adaptive** | Dynamic algo switching | Auto-optimization |
| **Geo-aware** | Nearest geographic server | Global deployments |
| **Health-weighted** | Health score based | Reliability priority |

#### Upstream Health Checking:
```yaml
upstreams:
  - name: "user-service"
    targets:
      - address: "10.0.1.1:8080"
        weight: 100
      - address: "10.0.1.2:8080"
        weight: 100
      - address: "10.0.1.3:8080"
        weight: 50     # Lower capacity server
    
    algorithm: "least_latency"
    
    health_check:
      active:
        path: "/health"
        interval: "10s"
        timeout: "3s"
        healthy_threshold: 3
        unhealthy_threshold: 2
        expected_status: [200]
      passive:
        enabled: true
        error_threshold: 5
        error_window: "30s"
        success_threshold: 3
    
    circuit_breaker:
      enabled: true
      error_threshold: 50       # percent
      volume_threshold: 20      # min requests before tripping
      sleep_window: "30s"
      half_open_requests: 3
```

### 3.6 Request/Response Transformation

```yaml
routes:
  - path: "/api/v2/users"
    service: "user-service"
    plugins:
      request_transform:
        # Header manipulation
        add_headers:
          X-Gateway: "apicerberus"
          X-Request-Start: "$timestamp_ms"
        remove_headers:
          - "X-Internal-Debug"
        rename_headers:
          Authorization: "X-Upstream-Auth"
        
        # Path rewriting
        path:
          regex: "/api/v2/(.*)"
          replacement: "/internal/v1/$1"
        
        # Query parameter manipulation
        add_query:
          source: "gateway"
        remove_query:
          - "debug"
        
        # Body transformation (JSON)
        body:
          add:
            gateway_timestamp: "$timestamp_iso"
          remove:
            - "internal_field"
          rename:
            userName: "username"
          template: |
            {
              "wrapped": true,
              "data": $body,
              "meta": { "version": "v2" }
            }
      
      response_transform:
        add_headers:
          X-Gateway-Latency: "$upstream_latency_ms"
        remove_headers:
          - "X-Powered-By"
          - "Server"
        body:
          remove:
            - "internal_id"
            - "debug_info"
          template: |
            {
              "success": true,
              "result": $body,
              "timestamp": "$timestamp_iso"
            }
```

**Template Variables:**
- `$body` — Original request/response body
- `$timestamp_ms` — Unix timestamp in milliseconds
- `$timestamp_iso` — ISO 8601 timestamp
- `$upstream_latency_ms` — Upstream response time
- `$consumer_id` — Authenticated consumer ID
- `$route_name` — Matched route name
- `$request_id` — Correlation ID
- `$remote_addr` — Client IP address
- `$header.X-Custom` — Request header value

---

## 4. Analytics & Monitoring

### 4.1 Metrics Collection

API Cerberus collects real-time analytics with in-memory ring buffers:

```
Metrics collected per request:
├── Timestamp
├── Route / Service / Consumer
├── HTTP method & status code
├── Request size (bytes)
├── Response size (bytes)
├── Upstream latency (ms)
├── Gateway latency (ms) — total processing time
├── Client IP & User-Agent
├── Rate limit status (allowed/throttled)
├── Cache status (hit/miss/bypass)
├── Error details (if any)
└── Plugin execution times
```

### 4.2 Analytics API

```
GET  /admin/api/v1/analytics/overview        # Dashboard summary
GET  /admin/api/v1/analytics/requests        # Request log (paginated)
GET  /admin/api/v1/analytics/timeseries      # Time-series metrics
GET  /admin/api/v1/analytics/top-routes      # Most requested routes
GET  /admin/api/v1/analytics/top-consumers   # Most active consumers
GET  /admin/api/v1/analytics/errors          # Error breakdown
GET  /admin/api/v1/analytics/latency         # Latency percentiles (p50, p95, p99)
GET  /admin/api/v1/analytics/throughput      # Requests per second
GET  /admin/api/v1/analytics/status-codes    # Status code distribution
GET  /admin/api/v1/analytics/geo             # Geographic distribution
```

### 4.3 Real-time Dashboard Metrics

```
┌──────────────────────────────────────────────────────┐
│  API CERBERUS DASHBOARD                              │
├──────────────────────────────────────────────────────┤
│                                                      │
│  Total Requests    Active Users         Error Rate   │
│  ████ 1.2M/day     ████ 342            ████ 0.03%   │
│                                                      │
│  Credits Consumed  Active Connections   Avg Latency  │
│  ████ 2.4M today   ████ 3,421          ████ 12ms    │
│                                                      │
│  ┌────────────────────────────────────────────────┐  │
│  │ Requests/sec (real-time graph)                 │  │
│  │ ▁▂▃▅▆▇█▇▆▅▃▂▁▂▃▅▆▇█▇▆▅▃▂▁                   │  │
│  └────────────────────────────────────────────────┘  │
│                                                      │
│  Top Routes          Status Codes      Rate Limited  │
│  /api/users  45%     2xx: 96.2%       Allowed: 99.1% │
│  /api/orders 30%     4xx: 3.5%        Throttled: 0.9%│
│  /api/search 15%     5xx: 0.3%                       │
│                                                      │
│  Top Consumers       Credit Usage      Blocked Reqs  │
│  mobile-app   45%    AI endpoints 60%  No credits: 12│
│  web-client   30%    Search APIs  25%  IP blocked: 3 │
│  partner-api  15%    User APIs    15%  No perm: 1    │
│                                                      │
│  ┌─────────────────────────────────────────────────┐ │
│  │ Recent Requests (live tail)                     │ │
│  │ 12:01:23 GET /api/users 200 12ms user:app1 -1cr │ │
│  │ 12:01:23 POST /api/ai   201 45ms user:web -50cr │ │
│  │ 12:01:24 GET /api/search 429 1ms RATE_LIMITED   │ │
│  │ 12:01:24 POST /api/gen  402 0ms NO_CREDITS      │ │
│  └─────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────┘
```

### 4.4 Alerting (Built-in)

```yaml
alerts:
  - name: "high-error-rate"
    condition: "error_rate > 5%"
    window: "5m"
    action: "webhook"
    webhook_url: "https://hooks.slack.com/..."
    cooldown: "15m"
  
  - name: "high-latency"
    condition: "p99_latency > 500ms"
    window: "5m"
    action: "log"    # log | webhook
  
  - name: "upstream-down"
    condition: "upstream_health < 50%"
    action: "webhook"
    webhook_url: "https://hooks.slack.com/..."
```

---

## 5. Raft Clustering & High Availability

### 5.1 Cluster Architecture

```
                    ┌──────────────────┐
                    │   API Cerberus Node  │
                    │    (Leader)      │
                    │  Raft Port: 7946 │
                    └────────┬─────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
    ┌─────────▼──────┐ ┌────▼───────────┐ ┌▼────────────────┐
    │ API Cerberus Node  │ │ API Cerberus Node  │ │ API Cerberus Node   │
    │  (Follower)    │ │  (Follower)    │ │  (Follower)     │
    │ Raft: 7946     │ │ Raft: 7946     │ │ Raft: 7946      │
    └────────────────┘ └────────────────┘ └─────────────────┘
```

### 5.2 Replicated State

Raft consensus replicates:
- **Configuration**: Routes, services, upstreams, consumers, plugins
- **Rate limit counters**: Distributed rate limiting across cluster
- **Health check results**: Shared upstream health state
- **Analytics aggregation**: Cluster-wide metrics

### 5.3 Cluster Configuration

```yaml
cluster:
  enabled: true
  node_id: "node-1"
  bind_addr: "0.0.0.0:7946"
  advertise_addr: "10.0.1.1:7946"
  peers:
    - "10.0.1.2:7946"
    - "10.0.1.3:7946"
  
  raft:
    election_timeout: "1s"
    heartbeat_interval: "200ms"
    snapshot_interval: "5m"
    snapshot_threshold: 1000
    max_append_entries: 64
    trailing_logs: 10000
```

---

## 6. Configuration System

### 6.1 Configuration File (apicerberus.yaml)

```yaml
# ═══════════════════════════════════════════════════════════
# API CERBERUS — API Gateway Configuration
# ═══════════════════════════════════════════════════════════

gateway:
  # Listen addresses
  http_addr: ":8080"
  https_addr: ":8443"
  grpc_addr: ":9090"
  
  # TLS
  tls:
    auto: true                          # ACME/Let's Encrypt
    acme_email: "admin@example.com"
    acme_dir: "/var/apicerberus/certs"
    # Or manual certs:
    # cert_file: "/path/to/cert.pem"
    # key_file: "/path/to/key.pem"
  
  # Performance
  read_timeout: "30s"
  write_timeout: "30s"
  idle_timeout: "120s"
  max_header_bytes: 1048576             # 1MB
  max_body_bytes: 10485760              # 10MB

admin:
  addr: ":9876"
  # Admin API authentication
  api_key: "admin-secret-key"
  # Web UI
  ui_enabled: true
  ui_path: "/dashboard"

# ── Services ──────────────────────────────────────────────
services:
  - name: "user-service"
    protocol: "http"                    # http | grpc | graphql
    upstream: "user-upstream"
    path_prefix: "/internal/api"
    retries: 3
    connect_timeout: "5s"
    read_timeout: "30s"
    write_timeout: "30s"
  
  - name: "payment-service"
    protocol: "grpc"
    upstream: "payment-upstream"
    retries: 2
    connect_timeout: "3s"
  
  - name: "graph-service"
    protocol: "graphql"
    upstream: "graph-upstream"
    graphql:
      max_depth: 10
      max_complexity: 1000
      introspection: false

# ── Routes ────────────────────────────────────────────────
routes:
  - name: "users-api"
    service: "user-service"
    hosts: ["api.example.com"]
    paths: ["/api/v1/users", "/api/v1/users/*"]
    methods: ["GET", "POST", "PUT", "DELETE"]
    strip_path: true
    preserve_host: false
    plugins:
      - name: "auth-apikey"
        config:
          key_names: ["X-API-Key", "apikey"]
      - name: "rate-limit"
        config:
          algorithm: "token_bucket"
          capacity: 100
          refill_rate: 10
          scope: "consumer"
      - name: "request-transform"
        config:
          add_headers:
            X-Gateway: "apicerberus"
  
  - name: "public-health"
    service: "user-service"
    paths: ["/health"]
    methods: ["GET"]
    # No auth — public endpoint

# ── Upstreams ─────────────────────────────────────────────
upstreams:
  - name: "user-upstream"
    algorithm: "least_latency"
    targets:
      - address: "10.0.1.1:8080"
        weight: 100
      - address: "10.0.1.2:8080"
        weight: 100
    health_check:
      active:
        path: "/health"
        interval: "10s"
        timeout: "3s"
        healthy_threshold: 3
        unhealthy_threshold: 2
      passive:
        enabled: true
        error_threshold: 5
        error_window: "30s"

# ── Consumers ─────────────────────────────────────────────
consumers:
  - name: "mobile-app"
    api_keys:
      - key: "ck_live_mobile_abc123"
    jwt_credentials:
      - algorithm: "RS256"
        issuer: "auth.example.com"
        secret_or_jwks: "https://auth.example.com/.well-known/jwks.json"
    rate_limit:
      requests_per_second: 100
      burst: 150
    acl_groups: ["mobile-tier", "read-only"]
    metadata:
      app_name: "Mobile App v2"
      contact: "mobile-team@example.com"

# ── Global Plugins ────────────────────────────────────────
global_plugins:
  - name: "cors"
    config:
      origins: ["*"]
      methods: ["GET", "POST", "PUT", "DELETE", "OPTIONS"]
      headers: ["Content-Type", "Authorization", "X-API-Key"]
      max_age: 3600
  
  - name: "correlation-id"
    config:
      header: "X-Request-ID"
      generator: "uuid"
  
  - name: "logger"
    config:
      format: "json"
      output: "stdout"
      level: "info"
      include_body: false
  
  - name: "analytics"
    config:
      enabled: true
      buffer_size: 10000
      flush_interval: "5s"
      retention: "7d"

# ── Logging ───────────────────────────────────────────────
logging:
  level: "info"                         # debug | info | warn | error
  format: "json"                        # json | text
  output: "stdout"                      # stdout | stderr | file
  file:
    path: "/var/log/apicerberus/gateway.log"
    max_size: "100MB"
    max_backups: 5
    compress: true

# ── Cluster ───────────────────────────────────────────────
cluster:
  enabled: false
  node_id: "node-1"
  bind_addr: "0.0.0.0:7946"
  peers: []

# ── Data Store (Users, Credits, Audit) ────────────────────
store:
  type: "sqlite"
  path: "/var/apicerberus/data.db"
  wal_mode: true
  busy_timeout: "5s"
  max_open_conns: 25

# ── User Portal ───────────────────────────────────────────
portal:
  enabled: true
  addr: ":9877"
  path_prefix: "/portal"
  session:
    secret: "change-me-in-production"
    max_age: "24h"
    secure: true
  registration:
    enabled: false
    require_approval: true
    default_credits: 100
    default_rate_limit_rps: 10
  password:
    min_length: 8
    require_uppercase: true
    require_number: true

# ── Billing / Credits ────────────────────────────────────
billing:
  enabled: true
  default_cost: 1
  route_costs:
    "ai-generate": 50
    "search-api": 5
  method_multipliers:
    GET: 1.0
    POST: 2.0
  zero_balance_action: "reject"
  low_balance_threshold: 100
  self_purchase:
    enabled: false

# ── Audit Logging ────────────────────────────────────────
audit_log:
  enabled: true
  store_request_body: true
  store_response_body: true
  max_request_body_size: 10240
  max_response_body_size: 10240
  mask_headers: ["Authorization", "X-API-Key", "Cookie"]
  mask_body_fields: ["password", "credit_card", "ssn"]
  retention:
    default: "30d"
    cleanup_interval: "1h"
  archive:
    enabled: false
    format: "jsonl"
    path: "/var/apicerberus/archive/"
    compress: true
```

### 6.2 Environment Variable Override

All config values can be overridden via environment variables:

```bash
APICERBERUS_GATEWAY_HTTP_ADDR=":80"
APICERBERUS_GATEWAY_HTTPS_ADDR=":443"
APICERBERUS_ADMIN_ADDR=":9876"
APICERBERUS_ADMIN_API_KEY="my-secret"
APICERBERUS_CLUSTER_ENABLED="true"
APICERBERUS_CLUSTER_NODE_ID="node-1"
APICERBERUS_LOGGING_LEVEL="debug"
```

### 6.3 Hot Reload

- Configuration changes via Admin API are applied immediately (in-memory)
- File-based config supports `SIGHUP` for hot reload
- Raft cluster propagates config changes to all nodes

---

## 7. Admin REST API

### 7.1 API Endpoints

```
# ── Services ──────────────────────────────────
GET    /admin/api/v1/services
POST   /admin/api/v1/services
GET    /admin/api/v1/services/:id
PUT    /admin/api/v1/services/:id
DELETE /admin/api/v1/services/:id
GET    /admin/api/v1/services/:id/routes
GET    /admin/api/v1/services/:id/plugins

# ── Routes ────────────────────────────────────
GET    /admin/api/v1/routes
POST   /admin/api/v1/routes
GET    /admin/api/v1/routes/:id
PUT    /admin/api/v1/routes/:id
DELETE /admin/api/v1/routes/:id
GET    /admin/api/v1/routes/:id/plugins

# ── Upstreams ─────────────────────────────────
GET    /admin/api/v1/upstreams
POST   /admin/api/v1/upstreams
GET    /admin/api/v1/upstreams/:id
PUT    /admin/api/v1/upstreams/:id
DELETE /admin/api/v1/upstreams/:id
GET    /admin/api/v1/upstreams/:id/targets
POST   /admin/api/v1/upstreams/:id/targets
DELETE /admin/api/v1/upstreams/:id/targets/:target_id
GET    /admin/api/v1/upstreams/:id/health

# ── Consumers ─────────────────────────────────
GET    /admin/api/v1/consumers
POST   /admin/api/v1/consumers
GET    /admin/api/v1/consumers/:id
PUT    /admin/api/v1/consumers/:id
DELETE /admin/api/v1/consumers/:id
GET    /admin/api/v1/consumers/:id/api-keys
POST   /admin/api/v1/consumers/:id/api-keys
DELETE /admin/api/v1/consumers/:id/api-keys/:key_id
GET    /admin/api/v1/consumers/:id/jwt
POST   /admin/api/v1/consumers/:id/jwt
DELETE /admin/api/v1/consumers/:id/jwt/:jwt_id

# ── Plugins ───────────────────────────────────
GET    /admin/api/v1/plugins
POST   /admin/api/v1/plugins
GET    /admin/api/v1/plugins/:id
PUT    /admin/api/v1/plugins/:id
DELETE /admin/api/v1/plugins/:id
GET    /admin/api/v1/plugins/available     # List all available plugin types

# ── Analytics ─────────────────────────────────
GET    /admin/api/v1/analytics/overview
GET    /admin/api/v1/analytics/requests
GET    /admin/api/v1/analytics/timeseries
GET    /admin/api/v1/analytics/top-routes
GET    /admin/api/v1/analytics/top-consumers
GET    /admin/api/v1/analytics/errors
GET    /admin/api/v1/analytics/latency
GET    /admin/api/v1/analytics/throughput
GET    /admin/api/v1/analytics/status-codes

# ── Cluster ───────────────────────────────────
GET    /admin/api/v1/cluster/status
GET    /admin/api/v1/cluster/nodes
POST   /admin/api/v1/cluster/nodes         # Join node
DELETE /admin/api/v1/cluster/nodes/:id      # Remove node
POST   /admin/api/v1/cluster/snapshot       # Force snapshot

# ── System ────────────────────────────────────
GET    /admin/api/v1/status                 # Gateway health
GET    /admin/api/v1/info                   # Version, uptime, config
POST   /admin/api/v1/config/reload          # Hot reload config file
GET    /admin/api/v1/config/export          # Export current config
POST   /admin/api/v1/config/import          # Import config
```

---

## 8. MCP Server Integration

API Cerberus exposes an MCP (Model Context Protocol) server for AI-native gateway management:

### 8.1 MCP Tools

```
# ── Gateway Management ────────────────────────
apicerberus_list_services          # List all services
apicerberus_create_service         # Create a new service
apicerberus_update_service         # Update service config
apicerberus_delete_service         # Delete a service
apicerberus_list_routes            # List all routes
apicerberus_create_route           # Create a new route
apicerberus_update_route           # Update route config
apicerberus_delete_route           # Delete a route

# ── Upstream Management ───────────────────────
apicerberus_list_upstreams         # List all upstreams
apicerberus_create_upstream        # Create upstream pool
apicerberus_add_target             # Add target to upstream
apicerberus_remove_target          # Remove target from upstream
apicerberus_upstream_health        # Check upstream health status

# ── Consumer Management ──────────────────────
apicerberus_list_consumers         # List all consumers
apicerberus_create_consumer        # Create consumer
apicerberus_create_api_key         # Generate API key for consumer
apicerberus_revoke_api_key         # Revoke an API key

# ── Plugin Management ─────────────────────────
apicerberus_list_plugins           # List active plugins
apicerberus_enable_plugin          # Enable plugin on route/service
apicerberus_disable_plugin         # Disable plugin
apicerberus_configure_plugin       # Update plugin config

# ── Analytics ─────────────────────────────────
apicerberus_analytics_overview     # Dashboard overview
apicerberus_analytics_requests     # Recent request log
apicerberus_analytics_errors       # Error analysis
apicerberus_analytics_latency      # Latency percentiles
apicerberus_analytics_top_routes   # Top routes by traffic

# ── Cluster ───────────────────────────────────
apicerberus_cluster_status         # Cluster health
apicerberus_cluster_nodes          # List cluster nodes

# ── User Management ──────────────────────────
apicerberus_list_users             # List all users
apicerberus_create_user            # Create new user
apicerberus_update_user            # Update user settings
apicerberus_suspend_user           # Suspend user
apicerberus_activate_user          # Activate user
apicerberus_user_apikeys           # List user's API keys
apicerberus_create_user_apikey     # Create API key for user
apicerberus_revoke_user_apikey     # Revoke user's API key
apicerberus_user_permissions       # List user's permissions
apicerberus_grant_permission       # Grant endpoint access
apicerberus_revoke_permission      # Revoke endpoint access

# ── Credit Management ────────────────────────
apicerberus_credit_overview        # Platform credit overview
apicerberus_user_credit_balance    # User's credit balance
apicerberus_topup_credits          # Add credits to user
apicerberus_deduct_credits         # Deduct credits from user
apicerberus_credit_transactions    # Credit transaction history
apicerberus_billing_config         # View/update billing config

# ── Audit Logs ────────────────────────────────
apicerberus_search_audit_logs      # Search audit logs with filters
apicerberus_audit_log_detail       # Get full request/response detail
apicerberus_user_audit_logs        # User's request log
apicerberus_audit_stats            # Audit log statistics
apicerberus_audit_cleanup          # Trigger log cleanup

# ── System ────────────────────────────────────
apicerberus_status                 # Gateway status
apicerberus_config_export          # Export configuration
apicerberus_config_import          # Import configuration
apicerberus_reload                 # Hot reload config
```

### 8.2 MCP Resources

```
apicerberus://services             # All services config
apicerberus://routes               # All routes config
apicerberus://upstreams            # All upstreams with health
apicerberus://consumers            # All consumers
apicerberus://plugins              # All active plugins
apicerberus://users                # All users
apicerberus://users/{id}           # User detail with permissions & credits
apicerberus://credits/overview     # Credit platform overview
apicerberus://audit-logs           # Recent audit logs
apicerberus://billing/config       # Billing configuration
apicerberus://analytics/overview   # Current analytics
apicerberus://cluster/status       # Cluster state
apicerberus://config               # Full configuration
```

---

## 9. Web Dashboard (Embedded UI)

### 9.1 Technology Stack

```
┌─────────────────────────────────────────────────────────────┐
│                    FRONTEND STACK                            │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  Framework        React 19 + TypeScript 5.x                 │
│  Build            Vite 6.x (production build → embed.FS)    │
│  Styling          Tailwind CSS v4.1                          │
│  Component Lib    shadcn/ui (latest, Radix UI primitives)   │
│  Icons            Lucide React                               │
│  Topology/Flow    React Flow (cluster map, pipeline visual) │
│  Charts           Recharts 2.x (analytics, time-series)     │
│  Code Editor      CodeMirror 6 (YAML config, JSON body)     │
│  Routing          React Router v7 (SPA, admin + portal)     │
│  State            Zustand (lightweight global state)         │
│  Data Fetching    TanStack Query v5 (caching, polling)      │
│  Forms            React Hook Form + Zod validation           │
│  Tables           TanStack Table v8 (sorting, filtering)    │
│  Notifications    Sonner (toast notifications)               │
│  Date/Time        date-fns (lightweight date formatting)     │
│  Font             Geist Sans + Geist Mono                    │
│  Embedding        Go embed.FS (single binary)                │
│                                                             │
│  Design Tokens:                                              │
│  ├── Dark/Light theme (system pref + manual toggle)          │
│  ├── CSS variables via shadcn/ui theming                     │
│  ├── Fully responsive (mobile, tablet, desktop)              │
│  ├── Accessible (WCAG 2.1 AA compliant)                     │
│  └── RTL-ready layout                                        │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

#### Design System (shadcn/ui based)

```
Color Palette (CSS Variables):
├── --primary:        Deep Purple (#6B21A8)
├── --primary-hover:  (#7C3AED)
├── --destructive:    Crimson (#DC2626)
├── --success:        Emerald (#059669)
├── --warning:        Amber (#D97706)
├── --background:     White / Slate-950 (dark)
├── --card:           White / Slate-900 (dark)
├── --muted:          Slate-100 / Slate-800 (dark)
├── --border:         Slate-200 / Slate-700 (dark)
├── --ring:           Purple-400
└── --sidebar:        Slate-50 / Slate-900 (dark)

Typography:
├── Headings:  Geist Sans (semibold/bold)
├── Body:      Geist Sans (regular/medium)
├── Code/Mono: Geist Mono (API keys, JSON, YAML, logs)
└── Sizes:     Tailwind default scale (text-xs → text-4xl)

Breakpoints (Tailwind defaults):
├── sm:  640px   (mobile landscape)
├── md:  768px   (tablet)
├── lg:  1024px  (desktop)
├── xl:  1280px  (wide desktop)
└── 2xl: 1536px  (ultra-wide)
```

#### shadcn/ui Components Used

```
Layout & Navigation:
├── Sidebar (collapsible, with icons + labels)
├── Breadcrumb
├── Tabs
├── Navigation Menu
├── Command (⌘K command palette)
├── Sheet (mobile navigation)
└── Separator

Data Display:
├── Table (with TanStack Table integration)
├── Card
├── Badge
├── Avatar
├── Skeleton (loading states)
├── Progress
├── Chart (Recharts wrapper)
└── Tooltip

Forms & Inputs:
├── Input
├── Textarea
├── Select
├── Checkbox
├── Switch
├── Slider
├── Radio Group
├── Calendar / Date Picker
├── Combobox (searchable select)
└── Form (React Hook Form integration)

Feedback & Overlay:
├── Alert / Alert Dialog
├── Dialog (modal)
├── Drawer
├── Dropdown Menu
├── Context Menu
├── Popover
├── Sonner (toast)
└── Hover Card

Special:
├── Accordion (plugin config sections)
├── Collapsible (log detail expand)
├── Scroll Area (log viewer, JSON bodies)
├── Resizable (playground split panes)
└── Toggle Group (view mode switches)
```

#### React Flow Usage

React Flow is used for interactive topology and visual pipeline views:

```
React Flow Views:
│
├── Cluster Topology (/dashboard/cluster)
│   ├── Node types: Leader (purple), Follower (slate), Unhealthy (red)
│   ├── Edges: Raft connections with heartbeat animation
│   ├── Node data: hostname, status, uptime, last heartbeat
│   ├── Interactive: click node → detail panel
│   └── Auto-layout: dagre/elk layout algorithm
│
├── Request Pipeline (/dashboard/routes/:id)
│   ├── Node types: Plugin nodes (auth, rate-limit, transform, etc.)
│   ├── Edges: Request flow direction with animated dots
│   ├── Visual: drag-to-reorder plugin execution sequence
│   ├── Node data: plugin config summary, execution time
│   └── Interactive: click plugin → config editor dialog
│
├── Upstream Health Map (/dashboard/upstreams/:id)
│   ├── Node types: Gateway (center), Upstream targets (ring)
│   ├── Edges: Traffic flow with thickness = request volume
│   ├── Node data: health status, latency, active connections
│   ├── Color coding: green (healthy), yellow (degraded), red (down)
│   └── Interactive: click target → health history chart
│
├── Service Dependency Graph (/dashboard/services)
│   ├── Node types: Services, Routes, Upstreams
│   ├── Edges: Route-to-service-to-upstream mapping
│   ├── Visual: full platform topology overview
│   └── Interactive: click any node → navigate to detail
│
└── GraphQL Federation Schema (/dashboard/services/:id for graphql)
    ├── Node types: Federated subgraphs
    ├── Edges: Schema relationships / shared types
    └── Node data: subgraph name, endpoint, types contributed
```

### 9.2 Dashboard Pages

The platform has two separate interfaces sharing the same React app:

```
# ═══ ADMIN PANEL ═══════════════════════════════════════════
/dashboard
├── /                          # Platform overview — users, credits, traffic, health
├── /services                  # Service management (CRUD)
├── /services/:id              # Service detail + routes
├── /routes                    # Route management (CRUD)
├── /routes/:id                # Route detail + plugin config
├── /upstreams                 # Upstream pools + health status
├── /upstreams/:id             # Upstream detail + targets
├── /consumers                 # Consumer management
├── /consumers/:id             # Consumer detail + credentials
├── /plugins                   # Global plugin management
├── /users                     # User management (CRUD, suspend, activate)
├── /users/:id                 # User detail (keys, permissions, credits, logs)
├── /credits                   # Credit overview, pricing config, bulk topup
├── /audit-logs                # Global audit log viewer (all users)
├── /analytics                 # Traffic analytics
│   ├── /analytics/requests    # Request log (searchable, filterable)
│   ├── /analytics/latency     # Latency analysis (percentiles, heatmap)
│   ├── /analytics/errors      # Error breakdown
│   └── /analytics/traffic     # Traffic patterns (time-series)
├── /cluster                   # Cluster status + node management
├── /config                    # Configuration editor (YAML)
└── /settings                  # Platform settings (portal, billing, retention)

# ═══ USER PORTAL ═══════════════════════════════════════════
/portal
├── /login                     # User login
├── /                          # Usage overview — balance, recent activity, stats
├── /api-keys                  # API key management (generate, rename, revoke)
├── /apis                      # Available APIs (permitted endpoints + docs)
├── /playground                # API tester (send requests, view responses)
├── /usage                     # Usage analytics (requests, credits, errors)
├── /logs                      # My request log (search, filter, export)
├── /logs/:id                  # Request detail (full req/res)
├── /credits                   # Credit balance, transactions, purchase
├── /security                  # IP whitelist, API key rotation, activity log
└── /settings                  # Profile, password, notifications
```

### 9.3 UI Components (Implementation Detail)

#### Admin Panel Components

- **Platform Overview Dashboard**
  - KPI cards (Card + Badge): total requests, active users, credits consumed, error rate
  - Real-time traffic chart (Recharts AreaChart with WebSocket data feed)
  - Top routes/consumers tables (TanStack Table with sorting/filtering)
  - Live request tail (ScrollArea + auto-scroll with Sonner notifications for errors)

- **Service/Route/Upstream/Consumer CRUD**
  - List views: TanStack Table with pagination, search (Input + Command), bulk actions
  - Create/Edit: Dialog or Sheet with React Hook Form + Zod schema validation
  - Delete confirmation: AlertDialog with destructive action
  - Inline status badges: Badge variants (default, success, destructive, warning)

- **User Manager**
  - User table with status filter (Tabs: All / Active / Suspended / Pending)
  - User detail: Tabs (Profile | API Keys | Permissions | Credits | Logs)
  - Permission matrix: Table with Checkbox per route × method combinations
  - Credit assignment: Input with quick presets (Dropdown Menu) + Dialog confirmation
  - IP whitelist: Editable list with Input + validation (CIDR format)

- **Credit Dashboard**
  - Balance overview cards with trend indicators (Card + Progress)
  - Credit consumption chart (Recharts BarChart, stacked by route)
  - Transaction table with type badges (topup=green, consume=slate, refund=amber)
  - Pricing editor: Form with per-route cost inputs + method multiplier sliders

- **Audit Log Viewer**
  - Request log table: TanStack Table with virtual scrolling for performance
  - Advanced filter panel: Collapsible with Combobox (user, route), DatePicker (range), Select (status, method)
  - Request detail: Sheet (slide-out) with Tabs (Request | Response | Timing | Credits)
  - JSON body viewer: CodeMirror 6 (read-only, syntax highlighted, collapsible)
  - Export: Dropdown Menu (CSV / JSON / JSONL) with date range Dialog
  - Real-time mode: Toggle Switch for live tail with auto-scroll

- **API Playground** (also in Portal)
  - Method selector: Select (GET, POST, PUT, DELETE, PATCH)
  - URL input: Input with route auto-complete (Command/Combobox)
  - Request builder: Tabs (Headers | Params | Body | Auth)
  - Headers: dynamic key-value pairs with add/remove (Input rows)
  - Body editor: CodeMirror 6 (JSON/XML with syntax highlighting)
  - Response viewer: Resizable split pane (request left, response right)
  - Response: Tabs (Body | Headers | Timing) with status Badge
  - Credit cost preview: Badge showing cost before send
  - Template manager: Dialog with save/load/delete

- **Plugin Configurator**
  - Plugin registry: Grid of Card components with plugin icon (Lucide), name, description
  - Config editor: dynamic Form generated from plugin JSON schema
  - Execution order: drag-and-drop sortable list (or React Flow pipeline view)
  - Per-route vs global: Toggle + scope selector

- **Analytics Charts**
  - Time-series: Recharts LineChart with zoom/pan, multiple series
  - Status code distribution: Recharts PieChart with interactive legend
  - Latency heatmap: custom Recharts ScatterChart (time × latency × density)
  - Error breakdown: Recharts BarChart grouped by error type/route
  - All charts: responsive, dark/light aware, configurable time window (Select)

- **Cluster Topology** (React Flow)
  - Interactive node graph (see React Flow Usage above)
  - Side panel: click node → detail Card with metrics
  - Real-time: WebSocket updates for node status changes
  - Controls: zoom, fit-to-screen, fullscreen toggle

- **Config Editor**
  - CodeMirror 6 with YAML syntax highlighting
  - Schema validation with inline error markers
  - Diff view: side-by-side comparison (current vs proposed)
  - Apply: AlertDialog confirmation + hot reload trigger
  - History: recent config changes with rollback option

#### User Portal Components

Portal reuses many admin components but with user-scoped data:

- **Portal Dashboard**: KPI cards (my balance, my requests today, my error rate) + mini usage chart
- **API Key Manager**: Table with copy-to-clipboard, rename (inline edit), revoke (AlertDialog), generate (Dialog with key display once)
- **Available APIs**: Card grid with route name, description, methods (Badge), credit cost, rate limit
- **Usage Analytics**: same Recharts components but filtered to current user
- **Request Logs**: same Audit Log Viewer but user-scoped (no user filter)
- **Credit Balance**: Card with large balance display + transaction table + forecast chart
- **Security Settings**: IP whitelist editor + activity log table (logins, key events)
- **Profile Settings**: Form (React Hook Form) with save + password change Dialog

#### Shared UI Patterns

```
Loading States:    Skeleton components matching content layout
Empty States:      Illustration + description + action Button
Error States:      Alert (destructive) with retry Button
Confirmation:      AlertDialog for destructive/irreversible actions
Success Feedback:  Sonner toast (bottom-right, auto-dismiss)
Navigation:        Sidebar (desktop) + Sheet (mobile) + Breadcrumb
Search:            Command palette (⌘K) for global search
Theme Toggle:      Sun/Moon icon Button in header (Lucide icons)
Responsive:        sidebar collapses at lg:, Sheet replaces at md:
Tables:            column visibility toggle, export button, per-page select
```

---

## 10. CLI Interface

```bash
# ── Server Commands ───────────────────────────
apicerberus start                              # Start gateway
apicerberus start --config /path/to/config.yaml
apicerberus start --http-addr :80 --https-addr :443
apicerberus stop                               # Graceful shutdown
apicerberus reload                             # Hot reload config
apicerberus version                            # Print version info

# ── Service Management ────────────────────────
apicerberus service list
apicerberus service add --name user-svc --url http://10.0.1.1:8080
apicerberus service get user-svc
apicerberus service update user-svc --retries 5
apicerberus service delete user-svc

# ── Route Management ──────────────────────────
apicerberus route list
apicerberus route add --name users-api --service user-svc --path /api/users
apicerberus route get users-api
apicerberus route delete users-api

# ── Upstream Management ───────────────────────
apicerberus upstream list
apicerberus upstream add --name user-pool --algorithm least_latency
apicerberus upstream target add user-pool --address 10.0.1.1:8080 --weight 100
apicerberus upstream health user-pool

# ── Consumer Management ───────────────────────
apicerberus consumer list
apicerberus consumer add --name mobile-app
apicerberus consumer apikey create mobile-app
apicerberus consumer apikey revoke mobile-app ck_live_abc123

# ── Plugin Management ─────────────────────────
apicerberus plugin list
apicerberus plugin available
apicerberus plugin enable rate-limit --route users-api --config '{"algorithm":"token_bucket"}'

# ── User Management ──────────────────────────
apicerberus user list
apicerberus user list --status active --sort created_at
apicerberus user create --email dev@example.com --name "Dev Team" --credits 5000
apicerberus user get <user-id>
apicerberus user update <user-id> --rate-limit-rps 100
apicerberus user suspend <user-id>
apicerberus user activate <user-id>
apicerberus user delete <user-id>

# ── User API Keys ─────────────────────────────
apicerberus user apikey list <user-id>
apicerberus user apikey create <user-id> --name "Production"
apicerberus user apikey revoke <user-id> <key-id>

# ── User Permissions ─────────────────────────
apicerberus user permission list <user-id>
apicerberus user permission grant <user-id> --route users-api --methods GET,POST --rps 50
apicerberus user permission revoke <user-id> <permission-id>

# ── User IP Whitelist ─────────────────────────
apicerberus user ip list <user-id>
apicerberus user ip add <user-id> --ip 192.168.1.0/24
apicerberus user ip remove <user-id> --ip 192.168.1.0/24

# ── Credit Management ────────────────────────
apicerberus credit overview                    # Platform credit summary
apicerberus credit balance <user-id>           # User's balance
apicerberus credit topup <user-id> --amount 10000 --reason "Monthly allocation"
apicerberus credit deduct <user-id> --amount 500 --reason "Refund adjustment"
apicerberus credit transactions <user-id> --from 2025-01-01 --to 2025-01-31
apicerberus credit bulk-topup --file users-credits.csv

# ── Audit Logs ────────────────────────────────
apicerberus audit search --user <user-id> --route users-api --status 4xx --last 24h
apicerberus audit tail --follow                # Real-time log tail
apicerberus audit detail <log-id>              # Full request/response
apicerberus audit export --user <user-id> --format jsonl --output logs.jsonl
apicerberus audit stats --last 7d              # Audit log statistics
apicerberus audit cleanup --older-than 30d     # Manual cleanup
apicerberus audit retention                    # View retention config

# ── Analytics ─────────────────────────────────
apicerberus analytics overview
apicerberus analytics requests --tail 100
apicerberus analytics latency --percentile p99 --window 1h

# ── Cluster ───────────────────────────────────
apicerberus cluster status
apicerberus cluster join --peer 10.0.1.2:7946
apicerberus cluster leave

# ── Config ────────────────────────────────────
apicerberus config export > config.yaml
apicerberus config import < config.yaml
apicerberus config validate config.yaml
apicerberus config diff old.yaml new.yaml

# ── MCP Server ────────────────────────────────
apicerberus mcp start                          # Start MCP server (stdio)
apicerberus mcp start --transport sse --port 3000
```

---

## 11. Project Structure

```
apicerberus/
├── cmd/
│   └── apicerberus/
│       └── main.go                    # Entry point
│
├── internal/
│   ├── config/
│   │   ├── config.go                  # Configuration types & loading
│   │   ├── parser.go                  # YAML/JSON parser (custom, no deps)
│   │   ├── validator.go               # Config validation
│   │   ├── env.go                     # Environment variable override
│   │   └── watcher.go                 # File watcher for hot reload
│   │
│   ├── gateway/
│   │   ├── gateway.go                 # Main gateway server
│   │   ├── listener.go                # HTTP/HTTPS/gRPC listeners
│   │   ├── router.go                  # Request routing engine
│   │   ├── proxy.go                   # Reverse proxy core
│   │   ├── websocket.go               # WebSocket proxy
│   │   ├── grpc.go                    # gRPC proxy
│   │   ├── graphql.go                 # GraphQL proxy & federation
│   │   └── tls.go                     # TLS management & ACME
│   │
│   ├── pipeline/
│   │   ├── pipeline.go                # Plugin execution pipeline
│   │   ├── context.go                 # Request context
│   │   └── registry.go                # Plugin registry
│   │
│   ├── plugin/
│   │   ├── plugin.go                  # Plugin interface
│   │   ├── auth_apikey.go             # API Key authentication
│   │   ├── auth_jwt.go                # JWT authentication
│   │   ├── rate_limit.go              # Rate limiting
│   │   ├── rate_limit_token_bucket.go
│   │   ├── rate_limit_sliding_window.go
│   │   ├── rate_limit_fixed_window.go
│   │   ├── rate_limit_leaky_bucket.go
│   │   ├── acl.go                     # Access Control Lists
│   │   ├── cors.go                    # CORS handling
│   │   ├── ip_restrict.go             # IP whitelist/blacklist
│   │   ├── request_transform.go       # Request transformation
│   │   ├── response_transform.go      # Response transformation
│   │   ├── request_size_limit.go      # Body size limiting
│   │   ├── request_validator.go       # JSON Schema validation
│   │   ├── url_rewrite.go             # URL rewriting
│   │   ├── redirect.go               # HTTP redirects
│   │   ├── retry.go                   # Retry logic
│   │   ├── circuit_breaker.go         # Circuit breaker
│   │   ├── timeout.go                 # Timeout control
│   │   ├── cache.go                   # Response caching
│   │   ├── compression.go            # gzip/brotli compression
│   │   ├── analytics.go              # Analytics collector
│   │   ├── logger.go                  # Structured logging
│   │   ├── correlation_id.go          # Request correlation
│   │   ├── bot_detect.go             # Bot detection
│   │   ├── grpc_transcode.go         # gRPC transcoding
│   │   └── graphql_guard.go          # GraphQL protection
│   │
│   ├── balancer/
│   │   ├── balancer.go                # Balancer interface
│   │   ├── round_robin.go
│   │   ├── weighted_round_robin.go
│   │   ├── least_conn.go
│   │   ├── ip_hash.go
│   │   ├── random.go
│   │   ├── consistent_hash.go
│   │   ├── least_latency.go
│   │   ├── adaptive.go
│   │   ├── geo_aware.go
│   │   └── health_weighted.go
│   │
│   ├── health/
│   │   ├── checker.go                 # Health check engine
│   │   ├── active.go                  # Active health checks
│   │   └── passive.go                 # Passive health checks
│   │
│   ├── analytics/
│   │   ├── engine.go                  # Analytics engine
│   │   ├── ringbuffer.go             # Ring buffer for metrics
│   │   ├── timeseries.go             # Time-series aggregation
│   │   ├── alerting.go               # Alert rule engine
│   │   └── export.go                 # Metrics export (Prometheus format)
│   │
│   ├── cluster/
│   │   ├── raft.go                    # Raft consensus implementation
│   │   ├── fsm.go                     # Finite state machine
│   │   ├── snapshot.go                # Snapshot management
│   │   ├── transport.go               # Raft network transport
│   │   └── discovery.go               # Peer discovery
│   │
│   ├── admin/
│   │   ├── server.go                  # Admin API server
│   │   ├── handlers_services.go       # Service CRUD handlers
│   │   ├── handlers_routes.go         # Route CRUD handlers
│   │   ├── handlers_upstreams.go      # Upstream CRUD handlers
│   │   ├── handlers_consumers.go      # Consumer CRUD handlers
│   │   ├── handlers_plugins.go        # Plugin CRUD handlers
│   │   ├── handlers_users.go          # User management handlers
│   │   ├── handlers_credits.go        # Credit management handlers
│   │   ├── handlers_permissions.go    # Permission management handlers
│   │   ├── handlers_audit.go          # Audit log handlers
│   │   ├── handlers_analytics.go      # Analytics API handlers
│   │   ├── handlers_cluster.go        # Cluster API handlers
│   │   ├── handlers_system.go         # System API handlers
│   │   ├── middleware.go              # Admin auth middleware
│   │   └── websocket.go              # WebSocket for live updates
│   │
│   ├── portal/
│   │   ├── server.go                  # User portal API server
│   │   ├── handlers_auth.go           # Login/logout/session
│   │   ├── handlers_apikeys.go        # API key management
│   │   ├── handlers_apis.go           # Available APIs listing
│   │   ├── handlers_playground.go     # API playground
│   │   ├── handlers_usage.go          # Usage analytics
│   │   ├── handlers_logs.go           # Request log viewer
│   │   ├── handlers_credits.go        # Credit balance & purchase
│   │   ├── handlers_security.go       # IP whitelist, activity log
│   │   ├── handlers_settings.go       # Profile & preferences
│   │   └── middleware.go              # User session middleware
│   │
│   ├── user/
│   │   ├── user.go                    # User entity & repository
│   │   ├── apikey.go                  # API key entity & generation
│   │   ├── permission.go             # Endpoint permission entity
│   │   ├── session.go                 # Session management
│   │   └── password.go               # Password hashing (bcrypt)
│   │
│   ├── billing/
│   │   ├── credit.go                  # Credit engine
│   │   ├── transaction.go            # Credit transaction log
│   │   ├── pricing.go                # Per-route pricing config
│   │   ├── purchase.go               # Self-purchase webhook handler
│   │   └── forecast.go               # Usage forecasting
│   │
│   ├── audit/
│   │   ├── logger.go                  # Audit log writer
│   │   ├── entry.go                   # Audit log entry types
│   │   ├── search.go                  # Log search & filtering
│   │   ├── retention.go              # Retention & cleanup scheduler
│   │   ├── archive.go                # Log archival (JSONL export)
│   │   └── export.go                 # CSV/JSON export
│   │
│   ├── store/
│   │   ├── sqlite.go                  # Embedded SQLite driver (pure Go)
│   │   ├── migrations.go             # Schema migrations
│   │   ├── user_repo.go              # User repository
│   │   ├── apikey_repo.go            # API key repository
│   │   ├── permission_repo.go        # Permission repository
│   │   ├── credit_repo.go            # Credit transaction repository
│   │   └── audit_repo.go             # Audit log repository
│   │
│   ├── mcp/
│   │   ├── server.go                  # MCP server
│   │   ├── tools.go                   # MCP tool definitions
│   │   ├── resources.go               # MCP resource definitions
│   │   └── handlers.go               # MCP tool handlers
│   │
│   ├── cli/
│   │   ├── cli.go                     # CLI framework
│   │   ├── cmd_start.go               # Start command
│   │   ├── cmd_service.go             # Service commands
│   │   ├── cmd_route.go               # Route commands
│   │   ├── cmd_upstream.go            # Upstream commands
│   │   ├── cmd_consumer.go            # Consumer commands
│   │   ├── cmd_plugin.go              # Plugin commands
│   │   ├── cmd_user.go                # User management commands
│   │   ├── cmd_credit.go              # Credit management commands
│   │   ├── cmd_audit.go               # Audit log commands
│   │   ├── cmd_analytics.go           # Analytics commands
│   │   ├── cmd_cluster.go             # Cluster commands
│   │   ├── cmd_config.go              # Config commands
│   │   └── cmd_mcp.go                 # MCP commands
│   │
│   └── pkg/
│       ├── json/
│       │   ├── encoder.go             # Custom JSON encoder
│       │   └── decoder.go             # Custom JSON decoder
│       ├── yaml/
│       │   ├── parser.go              # Custom YAML parser
│       │   └── emitter.go             # Custom YAML emitter
│       ├── jwt/
│       │   ├── jwt.go                 # JWT parser & validator
│       │   ├── rs256.go               # RSA-SHA256
│       │   └── hs256.go               # HMAC-SHA256
│       ├── uuid/
│       │   └── uuid.go                # UUID v4 generator
│       ├── pool/
│       │   ├── buffer.go              # Buffer pool
│       │   └── conn.go                # Connection pool
│       ├── crypto/
│       │   ├── hash.go                # Hashing utilities
│       │   └── random.go              # Secure random
│       └── template/
│           └── template.go            # Body template engine
│
├── web/                               # React 19 + Tailwind v4.1 + shadcn/ui
│   ├── src/
│   │   ├── App.tsx                    # Root app with React Router v7
│   │   ├── main.tsx                   # Entry point
│   │   ├── globals.css                # Tailwind v4.1 + shadcn/ui CSS variables
│   │   ├── lib/
│   │   │   ├── utils.ts              # cn() helper, formatters
│   │   │   ├── api.ts                # API client (fetch wrapper, TanStack Query)
│   │   │   ├── ws.ts                 # WebSocket client for real-time data
│   │   │   └── constants.ts          # Routes, colors, config
│   │   ├── stores/
│   │   │   ├── auth.ts               # Zustand: auth state (admin/user session)
│   │   │   ├── theme.ts              # Zustand: dark/light theme
│   │   │   └── realtime.ts           # Zustand: WebSocket live data
│   │   ├── hooks/
│   │   │   ├── use-services.ts       # TanStack Query: services CRUD
│   │   │   ├── use-routes.ts         # TanStack Query: routes CRUD
│   │   │   ├── use-upstreams.ts      # TanStack Query: upstreams
│   │   │   ├── use-users.ts          # TanStack Query: user management
│   │   │   ├── use-credits.ts        # TanStack Query: credit operations
│   │   │   ├── use-audit-logs.ts     # TanStack Query: audit logs
│   │   │   ├── use-analytics.ts      # TanStack Query: analytics data
│   │   │   ├── use-cluster.ts        # TanStack Query: cluster status
│   │   │   ├── use-playground.ts     # Playground request state
│   │   │   ├── use-realtime.ts       # WebSocket subscription hook
│   │   │   └── use-media-query.ts    # Responsive breakpoint hook
│   │   ├── components/
│   │   │   ├── ui/                   # shadcn/ui components (auto-generated)
│   │   │   │   ├── button.tsx
│   │   │   │   ├── card.tsx
│   │   │   │   ├── dialog.tsx
│   │   │   │   ├── table.tsx
│   │   │   │   ├── input.tsx
│   │   │   │   ├── select.tsx
│   │   │   │   ├── badge.tsx
│   │   │   │   ├── tabs.tsx
│   │   │   │   ├── sidebar.tsx
│   │   │   │   ├── command.tsx
│   │   │   │   ├── sheet.tsx
│   │   │   │   ├── alert-dialog.tsx
│   │   │   │   ├── dropdown-menu.tsx
│   │   │   │   ├── scroll-area.tsx
│   │   │   │   ├── skeleton.tsx
│   │   │   │   ├── sonner.tsx
│   │   │   │   ├── resizable.tsx
│   │   │   │   └── ... (all shadcn/ui components)
│   │   │   ├── layout/
│   │   │   │   ├── AdminLayout.tsx    # Admin sidebar + header + breadcrumb
│   │   │   │   ├── PortalLayout.tsx   # Portal sidebar + header
│   │   │   │   ├── AppSidebar.tsx     # Collapsible sidebar (Lucide icons)
│   │   │   │   ├── Header.tsx         # Top bar (search, theme toggle, user menu)
│   │   │   │   └── ThemeProvider.tsx   # Dark/light theme provider
│   │   │   ├── charts/
│   │   │   │   ├── AreaChart.tsx       # Recharts: real-time traffic
│   │   │   │   ├── BarChart.tsx        # Recharts: credit usage, errors
│   │   │   │   ├── LineChart.tsx       # Recharts: latency trends
│   │   │   │   ├── PieChart.tsx        # Recharts: status code distribution
│   │   │   │   ├── HeatmapChart.tsx    # Recharts: latency heatmap
│   │   │   │   └── KPICard.tsx         # Metric card with trend indicator
│   │   │   ├── flow/                   # React Flow components
│   │   │   │   ├── ClusterTopology.tsx # Raft cluster node graph
│   │   │   │   ├── PipelineView.tsx    # Plugin execution pipeline
│   │   │   │   ├── UpstreamMap.tsx     # Upstream health topology
│   │   │   │   ├── ServiceGraph.tsx    # Service dependency graph
│   │   │   │   ├── nodes/             # Custom React Flow node types
│   │   │   │   │   ├── GatewayNode.tsx
│   │   │   │   │   ├── ServiceNode.tsx
│   │   │   │   │   ├── UpstreamNode.tsx
│   │   │   │   │   ├── PluginNode.tsx
│   │   │   │   │   ├── ClusterNode.tsx
│   │   │   │   │   └── SubgraphNode.tsx
│   │   │   │   └── edges/             # Custom React Flow edge types
│   │   │   │       ├── TrafficEdge.tsx  # Animated traffic flow
│   │   │   │       └── RaftEdge.tsx     # Heartbeat connection
│   │   │   ├── playground/
│   │   │   │   ├── PlaygroundView.tsx  # Full playground layout
│   │   │   │   ├── RequestBuilder.tsx  # Method + URL + headers + body
│   │   │   │   ├── ResponseViewer.tsx  # Status + headers + body + timing
│   │   │   │   ├── HeaderEditor.tsx    # Dynamic key-value pair editor
│   │   │   │   ├── BodyEditor.tsx      # CodeMirror 6 JSON editor
│   │   │   │   └── TemplateManager.tsx # Save/load request templates
│   │   │   ├── editors/
│   │   │   │   ├── YAMLEditor.tsx      # CodeMirror 6 YAML editor
│   │   │   │   ├── JSONViewer.tsx      # CodeMirror 6 read-only JSON
│   │   │   │   └── DiffViewer.tsx      # Side-by-side config diff
│   │   │   ├── tables/
│   │   │   │   ├── DataTable.tsx       # TanStack Table wrapper
│   │   │   │   ├── DataTablePagination.tsx
│   │   │   │   ├── DataTableToolbar.tsx # Search + filter + column toggle
│   │   │   │   └── DataTableExport.tsx  # CSV/JSON export button
│   │   │   └── shared/
│   │   │       ├── EmptyState.tsx      # Illustration + CTA
│   │   │       ├── LoadingState.tsx    # Skeleton grid
│   │   │       ├── ErrorState.tsx      # Alert + retry
│   │   │       ├── ConfirmDialog.tsx   # Reusable destructive confirm
│   │   │       ├── CopyButton.tsx      # Click-to-copy (API keys, etc.)
│   │   │       ├── StatusBadge.tsx     # Color-coded status indicator
│   │   │       ├── TimeAgo.tsx         # Relative time display
│   │   │       └── CreditBadge.tsx     # Credit cost/balance display
│   │   ├── pages/
│   │   │   ├── admin/                 # Admin panel pages
│   │   │   │   ├── Dashboard.tsx
│   │   │   │   ├── Services.tsx
│   │   │   │   ├── ServiceDetail.tsx
│   │   │   │   ├── Routes.tsx
│   │   │   │   ├── RouteDetail.tsx
│   │   │   │   ├── Upstreams.tsx
│   │   │   │   ├── UpstreamDetail.tsx
│   │   │   │   ├── Consumers.tsx
│   │   │   │   ├── Plugins.tsx
│   │   │   │   ├── Users.tsx
│   │   │   │   ├── UserDetail.tsx
│   │   │   │   ├── Credits.tsx
│   │   │   │   ├── AuditLogs.tsx
│   │   │   │   ├── AuditLogDetail.tsx
│   │   │   │   ├── Analytics.tsx
│   │   │   │   ├── Cluster.tsx
│   │   │   │   ├── Config.tsx
│   │   │   │   └── Settings.tsx
│   │   │   ├── portal/               # User portal pages
│   │   │   │   ├── Dashboard.tsx
│   │   │   │   ├── APIKeys.tsx
│   │   │   │   ├── APIs.tsx
│   │   │   │   ├── Playground.tsx
│   │   │   │   ├── Usage.tsx
│   │   │   │   ├── Logs.tsx
│   │   │   │   ├── LogDetail.tsx
│   │   │   │   ├── Credits.tsx
│   │   │   │   ├── Security.tsx
│   │   │   │   ├── Settings.tsx
│   │   │   │   └── Login.tsx
│   │   │   └── shared/
│   │   │       └── NotFound.tsx
│   │   └── schemas/                   # Zod validation schemas
│   │       ├── service.ts
│   │       ├── route.ts
│   │       ├── upstream.ts
│   │       ├── consumer.ts
│   │       ├── user.ts
│   │       ├── credit.ts
│   │       ├── permission.ts
│   │       └── plugin.ts
│   ├── index.html
│   ├── vite.config.ts
│   ├── tailwind.config.ts             # Tailwind v4.1 config
│   ├── components.json                # shadcn/ui config
│   ├── tsconfig.json
│   └── package.json                   # React 19, shadcn/ui, Lucide, React Flow, Recharts, etc.
│
│   # ── Frontend Dependencies (package.json) ──────────
│   # react: ^19.0           react-dom: ^19.0
│   # react-router: ^7.0     @tanstack/react-query: ^5.0
│   # @tanstack/react-table: ^8.0
│   # zustand: ^5.0          react-hook-form: ^7.0
│   # zod: ^3.0              @hookform/resolvers: ^3.0
│   # tailwindcss: ^4.1      @tailwindcss/vite: ^4.1
│   # lucide-react: latest   recharts: ^2.0
│   # @xyflow/react: latest  (React Flow)
│   # @codemirror/lang-json   @codemirror/lang-yaml
│   # codemirror: ^6.0       @codemirror/theme-one-dark
│   # sonner: latest         date-fns: ^4.0
│   # class-variance-authority: latest
│   # clsx: latest           tailwind-merge: latest
│   # @radix-ui/*: latest    (via shadcn/ui)
│   # vite: ^6.0             typescript: ^5.7
│
├── embed.go                           # Go embed for web assets
├── go.mod                             # Zero dependencies
├── Makefile
├── Dockerfile
├── apicerberus.example.yaml
├── SPECIFICATION.md
├── IMPLEMENTATION.md
├── TASKS.md
├── BRANDING.md
├── README.md
└── LICENSE                            # MIT
```

---

## 12. Performance Targets

| Metric | Target |
|--------|--------|
| Throughput | 50,000+ requests/sec (single node) |
| Latency overhead | < 1ms (gateway processing, excluding upstream) |
| Memory | < 100MB base (without analytics data) |
| Startup time | < 500ms |
| Config reload | < 50ms (hot reload) |
| Binary size | < 30MB (with embedded UI) |
| Cluster sync | < 100ms (config propagation) |
| Health check | < 10ms per upstream target |
| Max connections | 100,000+ concurrent |
| Max routes | 10,000+ |

---

## 13. Security

- Admin API authentication (API key + optional JWT)
- TLS everywhere (auto-ACME or manual certs)
- Request size limits (configurable per route)
- IP whitelist/blacklist
- Rate limiting (DDoS protection)
- Bot detection (User-Agent filtering)
- CORS enforcement
- No eval, no dynamic code execution
- Constant-time comparison for API key validation
- JWT signature verification (RS256/HS256)
- Secure random for key/ID generation
- Audit log for admin actions

---

## 14. Deployment

### Docker

```dockerfile
FROM scratch
COPY apicerberus /apicerberus
COPY apicerberus.yaml /etc/apicerberus/apicerberus.yaml
EXPOSE 8080 8443 9090 9876 7946
ENTRYPOINT ["/apicerberus", "start"]
```

### Docker Compose (3-node cluster)

```yaml
services:
  apicerberus-1:
    image: apicerberus:latest
    environment:
      APICERBERUS_CLUSTER_ENABLED: "true"
      APICERBERUS_CLUSTER_NODE_ID: "node-1"
      APICERBERUS_CLUSTER_PEERS: "apicerberus-2:7946,apicerberus-3:7946"
    ports:
      - "8080:8080"
      - "9876:9876"
  
  apicerberus-2:
    image: apicerberus:latest
    environment:
      APICERBERUS_CLUSTER_ENABLED: "true"
      APICERBERUS_CLUSTER_NODE_ID: "node-2"
      APICERBERUS_CLUSTER_PEERS: "apicerberus-1:7946,apicerberus-3:7946"
  
  apicerberus-3:
    image: apicerberus:latest
    environment:
      APICERBERUS_CLUSTER_ENABLED: "true"
      APICERBERUS_CLUSTER_NODE_ID: "node-3"
      APICERBERUS_CLUSTER_PEERS: "apicerberus-1:7946,apicerberus-2:7946"
```

---

## 15. Comparison with Alternatives

| Feature | API Cerberus | Kong | Tyk | KrakenD | Traefik |
|---------|----------|------|-----|---------|---------|
| Language | Go | Lua/Go | Go | Go | Go |
| Dependencies | 0 | 200+ | 50+ | 20+ | 30+ |
| Database required | Embedded SQLite | PostgreSQL | Redis | No | No |
| Single binary | ✅ | ❌ | ❌ | ✅ | ✅ |
| Built-in Admin UI | ✅ | ❌ (Enterprise) | ✅ | ❌ | ✅ |
| User Self-Service Portal | ✅ | ❌ | ❌ (Enterprise) | ❌ | ❌ |
| Credit/Billing System | ✅ | ❌ | ❌ | ❌ | ❌ |
| API Playground | ✅ | ❌ | ❌ | ❌ | ❌ |
| Per-user Endpoint ACL | ✅ | Plugin | ✅ | ❌ | ❌ |
| Audit Log (req+res) | ✅ | Plugin | ✅ | ❌ | ❌ |
| gRPC proxy | ✅ | ✅ | ✅ | ✅ | ✅ |
| GraphQL federation | ✅ | Plugin | Plugin | ✅ | ❌ |
| Clustering | Raft | PostgreSQL | Redis | ❌ | ❌ |
| Plugin system | ✅ | Lua scripts | Go/Python | ❌ | Middleware |
| MCP server | ✅ | ❌ | ❌ | ❌ | ❌ |
| Rate limiting algos | 4 | 2 | 2 | 1 | 1 |
| LB algorithms | 10 | 4 | 3 | 2 | 4 |
| Config hot reload | ✅ | ✅ | ✅ | ❌ | ✅ |
| License | MIT | Apache-2.0 | MPL-2.0 | Apache-2.0 | MIT |
| Price | Free | $$$$ | $$$ | Free | Free |

---

## 16. Multi-Tenant Users & Clients

### 16.1 Data Model

API Cerberus uses embedded SQLite for persistent user/client/credit/log data (gateway config remains YAML/JSON in-memory):

```
┌─────────────────────────────────────────────────────────────────┐
│                        DATA MODEL                               │
│                                                                 │
│  ┌──────────┐    ┌──────────────┐    ┌───────────────────┐      │
│  │  Admin   │    │    User      │    │   API Key         │      │
│  │  (owner) │    │  (client)    │───▶│  (credentials)    │      │
│  └──────────┘    └──────┬───────┘    └───────────────────┘      │
│                         │                                       │
│              ┌──────────┼──────────┐                            │
│              ▼          ▼          ▼                            │
│  ┌───────────────┐ ┌────────┐ ┌─────────────┐                  │
│  │  Permissions  │ │Credits │ │  IP White   │                  │
│  │ (per-endpoint)│ │(balance│ │   list      │                  │
│  └───────────────┘ │ & txns)│ └─────────────┘                  │
│                     └────────┘                                  │
│              ┌──────────────────────┐                           │
│              │    Audit Log         │                           │
│              │ (req/res per user)   │                           │
│              └──────────────────────┘                           │
└─────────────────────────────────────────────────────────────────┘
```

### 16.2 User Entity

```go
type User struct {
    ID            string    `json:"id"`             // UUID
    Email         string    `json:"email"`          // Unique, login identifier
    Name          string    `json:"name"`           // Display name
    Company       string    `json:"company"`        // Optional company name
    PasswordHash  string    `json:"-"`              // bcrypt hash (never exposed)
    Role          UserRole  `json:"role"`           // admin | user
    Status        string    `json:"status"`         // active | suspended | pending
    
    // Credit balance
    CreditBalance int64     `json:"credit_balance"` // Current available credits
    
    // Rate limiting (global for this user)
    RateLimitRPS  int       `json:"rate_limit_rps"` // Requests per second (0 = unlimited)
    RateLimitRPM  int       `json:"rate_limit_rpm"` // Requests per minute
    RateLimitRPD  int       `json:"rate_limit_rpd"` // Requests per day
    
    // IP restrictions
    IPWhitelist   []string  `json:"ip_whitelist"`   // Empty = allow all
    
    // Metadata
    Metadata      map[string]string `json:"metadata"`
    CreatedAt     time.Time `json:"created_at"`
    UpdatedAt     time.Time `json:"updated_at"`
    LastLoginAt   time.Time `json:"last_login_at"`
    LastActiveAt  time.Time `json:"last_active_at"`
}

type UserRole string
const (
    RoleAdmin UserRole = "admin"
    RoleUser  UserRole = "user"
)
```

### 16.3 API Keys (per user)

```go
type APIKey struct {
    ID          string    `json:"id"`           // UUID
    UserID      string    `json:"user_id"`      // Owner
    Key         string    `json:"key"`          // ck_live_xxxx (shown once on creation)
    KeyHash     string    `json:"-"`            // SHA-256 hash (stored)
    KeyPrefix   string    `json:"key_prefix"`   // First 8 chars (for display: ck_live_ab...)
    Name        string    `json:"name"`         // User-given label ("Production", "Staging")
    Status      string    `json:"status"`       // active | revoked | expired
    ExpiresAt   *time.Time `json:"expires_at"`  // Optional expiration
    LastUsedAt  *time.Time `json:"last_used_at"`
    LastUsedIP  string    `json:"last_used_ip"`
    CreatedAt   time.Time `json:"created_at"`
}

// Key format: ck_live_<32 random chars>  (production)
//             ck_test_<32 random chars>  (test mode — no credits consumed)
```

### 16.4 Admin vs User Roles

| Capability | Admin | User |
|------------|-------|------|
| Create/manage users | ✅ | ❌ |
| Assign credits | ✅ | ❌ |
| Set rate limits per user | ✅ | ❌ |
| Set endpoint permissions per user | ✅ | ❌ |
| Configure services/routes/upstreams | ✅ | ❌ |
| Manage plugins | ✅ | ❌ |
| View all users' logs | ✅ | ❌ |
| Set credit pricing | ✅ | ❌ |
| View platform analytics | ✅ | ❌ |
| Manage cluster | ✅ | ❌ |
| Generate own API keys | ✅ | ✅ |
| Rotate/revoke own keys | ✅ | ✅ |
| Set own IP whitelist | ✅ | ✅ |
| View own usage/logs | ✅ | ✅ |
| View own credit balance | ✅ | ✅ |
| Test APIs from portal | ✅ | ✅ |
| View own analytics | ✅ | ✅ |
| Purchase credits (if enabled) | ✅ | ✅ |

---

## 17. Credit System & Billing

### 17.1 Credit Model

Credits are the unit of API consumption. Every API call costs a configurable number of credits.

```go
type CreditConfig struct {
    // Global defaults
    DefaultCostPerRequest  int64  `json:"default_cost_per_request"`  // Default: 1
    
    // Per-route overrides
    RouteCosts map[string]int64  `json:"route_costs"`
    // e.g., "users-api": 1, "ai-generate": 50, "search": 5
    
    // Method multipliers (optional)
    MethodMultipliers map[string]float64 `json:"method_multipliers"`
    // e.g., "GET": 1.0, "POST": 2.0, "DELETE": 1.5
    
    // Response-based pricing (optional)
    ResponseSizePricing bool   `json:"response_size_pricing"` // Charge based on response size
    BytesPerCredit      int64  `json:"bytes_per_credit"`      // e.g., 1 credit per 10KB
    
    // Test mode
    TestModeEnabled     bool   `json:"test_mode_enabled"`     // ck_test_ keys don't consume credits
}
```

### 17.2 Credit Transactions

```go
type CreditTransaction struct {
    ID            string    `json:"id"`
    UserID        string    `json:"user_id"`
    Type          string    `json:"type"`          // topup | consume | refund | admin_adjust | purchase
    Amount        int64     `json:"amount"`        // Positive for topup, negative for consume
    BalanceBefore int64     `json:"balance_before"`
    BalanceAfter  int64     `json:"balance_after"`
    Description   string    `json:"description"`   // "API call: GET /api/users", "Admin topup", "Purchase: 10000 credits"
    RequestID     string    `json:"request_id"`    // Linked audit log entry (for consume type)
    RouteID       string    `json:"route_id"`      // Which route consumed credits
    CreatedAt     time.Time `json:"created_at"`
}
```

### 17.3 Credit Management (Admin)

```yaml
# Admin can configure credit allocation and pricing:

billing:
  enabled: true
  
  # Credit costs per route
  default_cost: 1
  route_costs:
    "users-api": 1
    "ai-generate": 50
    "search-api": 5
    "analytics-export": 100
  
  # Method multipliers
  method_multipliers:
    GET: 1.0
    POST: 2.0
    PUT: 1.5
    DELETE: 1.0
  
  # Low balance warning
  low_balance_threshold: 100
  low_balance_webhook: "https://hooks.slack.com/..."
  
  # Auto-suspend when credits run out
  zero_balance_action: "reject"   # reject | allow_with_flag | suspend_user
  zero_balance_response:
    status: 402
    body: '{"error":"insufficient_credits","message":"Your credit balance is exhausted. Please top up."}'
  
  # Self-purchase (optional — users can buy credits)
  self_purchase:
    enabled: false
    # When enabled, integrates with external payment webhook
    webhook_url: "https://your-billing-system.com/verify"
    packages:
      - name: "Starter"
        credits: 1000
        price_usd: 9.99
      - name: "Pro"
        credits: 10000
        price_usd: 79.99
      - name: "Enterprise"
        credits: 100000
        price_usd: 499.99
```

### 17.4 Credit Flow

```
Request arrives ──▶ Auth (identify user) ──▶ Credit Check
                                                  │
                                    ┌─────────────┼─────────────┐
                                    ▼             ▼             ▼
                              Has Credits?   Test Key?    Zero Action?
                                    │             │             │
                                   YES           YES        reject/allow
                                    │             │             │
                                    ▼             ▼             ▼
                              Deduct Credit   Skip Deduct   402 / Flag
                                    │             │
                                    ▼             ▼
                              Forward to Upstream ──▶ Response ──▶ Log Transaction
```

---

## 18. Per-Endpoint Access Control & Permissions

### 18.1 Permission Model

Each user has explicit permissions defining which endpoints they can access:

```go
type EndpointPermission struct {
    ID          string   `json:"id"`
    UserID      string   `json:"user_id"`
    RouteID     string   `json:"route_id"`       // Which route/endpoint
    Methods     []string `json:"methods"`         // ["GET", "POST"] or ["*"] for all
    Allowed     bool     `json:"allowed"`         // true = whitelist, false = blacklist
    
    // Per-endpoint rate limits (override user-level)
    RateLimitRPS  *int   `json:"rate_limit_rps"`  // nil = use user default
    RateLimitRPM  *int   `json:"rate_limit_rpm"`
    RateLimitRPD  *int   `json:"rate_limit_rpd"`
    
    // Per-endpoint credit cost override
    CreditCost    *int64 `json:"credit_cost"`     // nil = use route default
    
    // Time-based access
    ValidFrom     *time.Time `json:"valid_from"`  // nil = always
    ValidUntil    *time.Time `json:"valid_until"` // nil = never expires
    
    // Day/hour restrictions
    AllowedDays   []string `json:"allowed_days"`  // ["mon","tue",...] empty = all
    AllowedHours  string   `json:"allowed_hours"` // "09:00-17:00" or "" = all
}
```

### 18.2 Permission Strategy

```yaml
# Two modes:
access_control:
  # Mode 1: Whitelist (default) — users can ONLY access explicitly granted endpoints
  mode: "whitelist"
  
  # Mode 2: Blacklist — users can access ALL endpoints EXCEPT explicitly denied ones
  # mode: "blacklist"

# Admin assigns permissions per user:
user_permissions:
  - user: "mobile-app-user"
    permissions:
      - route: "users-api"
        methods: ["GET"]
        rate_limit_rps: 50
      - route: "search-api"
        methods: ["GET", "POST"]
        rate_limit_rps: 20
        credit_cost: 10
      - route: "ai-generate"
        methods: ["POST"]
        rate_limit_rps: 5
        credit_cost: 100
        allowed_hours: "09:00-18:00"
```

### 18.3 Permission Resolution Order

```
1. Is user active? ──▶ No ──▶ 403 Forbidden
2. Is endpoint in user's permissions? ──▶ No ──▶ 403 (whitelist) / Allow (blacklist)
3. Is HTTP method allowed? ──▶ No ──▶ 405 Method Not Allowed
4. Is within time/day restriction? ──▶ No ──▶ 403 (with reason)
5. Is IP in user's whitelist? ──▶ No ──▶ 403 (IP not allowed)
6. Rate limit check (endpoint-level > user-level > global) ──▶ Exceeded ──▶ 429
7. Credit check ──▶ Insufficient ──▶ 402 Payment Required
8. ✅ Forward request to upstream
```

---

## 19. Request/Response Audit Logging

### 19.1 Audit Log Entry

Every API request is logged with full request/response detail:

```go
type AuditLogEntry struct {
    ID              string    `json:"id"`              // UUID
    RequestID       string    `json:"request_id"`      // Correlation ID
    UserID          string    `json:"user_id"`         // Who made the request
    APIKeyID        string    `json:"api_key_id"`      // Which key was used
    APIKeyPrefix    string    `json:"api_key_prefix"`  // "ck_live_ab..." (safe display)
    
    // Request details
    Method          string    `json:"method"`
    Path            string    `json:"path"`
    Host            string    `json:"host"`
    QueryParams     string    `json:"query_params"`    // URL query string
    RequestHeaders  map[string]string `json:"request_headers"`
    RequestBody     string    `json:"request_body"`    // Stored if enabled (truncated to max size)
    RequestSize     int64     `json:"request_size"`    // Bytes
    ClientIP        string    `json:"client_ip"`
    UserAgent       string    `json:"user_agent"`
    
    // Routing
    RouteID         string    `json:"route_id"`
    RouteName       string    `json:"route_name"`
    ServiceID       string    `json:"service_id"`
    ServiceName     string    `json:"service_name"`
    UpstreamTarget  string    `json:"upstream_target"` // Which backend served this
    
    // Response details
    StatusCode      int       `json:"status_code"`
    ResponseHeaders map[string]string `json:"response_headers"`
    ResponseBody    string    `json:"response_body"`   // Stored if enabled (truncated)
    ResponseSize    int64     `json:"response_size"`   // Bytes
    
    // Timing
    GatewayLatency  int64     `json:"gateway_latency_ms"`  // Total gateway processing
    UpstreamLatency int64     `json:"upstream_latency_ms"` // Upstream response time
    
    // Credits
    CreditsConsumed int64     `json:"credits_consumed"`
    CreditBalance   int64     `json:"credit_balance_after"`
    
    // Status
    Blocked         bool      `json:"blocked"`         // Was request blocked?
    BlockReason     string    `json:"block_reason"`    // "rate_limited", "no_credits", "no_permission", "ip_blocked"
    
    // Metadata
    Protocol        string    `json:"protocol"`        // http | grpc | graphql
    TLSVersion      string    `json:"tls_version"`
    Timestamp       time.Time `json:"timestamp"`
}
```

### 19.2 Audit Log Configuration

```yaml
audit_log:
  enabled: true
  
  # What to store
  store_request_headers: true
  store_request_body: true
  store_response_headers: true
  store_response_body: true
  
  # Size limits
  max_request_body_size: 10240      # 10KB — truncate larger bodies
  max_response_body_size: 10240     # 10KB
  
  # Header filtering (sensitive headers to mask)
  mask_headers:
    - "Authorization"
    - "X-API-Key"
    - "Cookie"
    - "Set-Cookie"
  mask_replacement: "***REDACTED***"
  
  # Body filtering
  mask_body_fields:                  # JSON field paths to mask
    - "password"
    - "credit_card"
    - "ssn"
    - "token"
  
  # Retention & cleanup
  retention:
    default: "30d"                   # Default retention period
    per_route:                       # Override per route
      "ai-generate": "90d"
      "health-check": "1d"
    
    # Cleanup schedule
    cleanup_interval: "1h"           # Run cleanup every hour
    cleanup_batch_size: 10000        # Delete in batches
  
  # Storage
  storage:
    type: "sqlite"                   # Embedded SQLite
    path: "/var/apicerberus/audit.db"
    max_size: "10GB"                 # Max database size
    wal_mode: true                   # WAL mode for better concurrent writes
    
    # Archival (optional — export old logs before deletion)
    archive:
      enabled: false
      format: "jsonl"               # JSON Lines format
      path: "/var/apicerberus/archive/"
      compress: true                # gzip compression
```

### 19.3 Log Search & Filtering

```
# Audit log API supports rich filtering:
GET /admin/api/v1/audit-logs?
    user_id=xxx&
    api_key_prefix=ck_live_ab&
    route=users-api&
    method=POST&
    status_code=200&
    status_min=400&status_max=499&    # 4xx errors
    client_ip=192.168.1.1&
    blocked=true&
    block_reason=rate_limited&
    from=2025-01-01T00:00:00Z&
    to=2025-01-31T23:59:59Z&
    min_latency=100&                  # Slow requests (ms)
    search=keyword&                    # Full-text search in path/body
    sort=timestamp&
    order=desc&
    page=1&
    per_page=50
```

---

## 20. User Self-Service Portal

### 20.1 Portal Overview

Users get their own dashboard at `/portal` — separate from the admin panel:

```
/portal
├── /                              # Overview — usage summary, credit balance, recent activity
├── /api-keys                      # API Key management
│   ├── Generate new key
│   ├── Rename / label keys
│   ├── Revoke keys
│   └── View key usage stats
├── /apis                          # Available APIs (endpoints user has access to)
│   ├── API documentation (auto-generated from routes)
│   ├── Endpoint list with credit costs
│   └── Method & permission details
├── /playground                    # API testing / playground
│   ├── Select endpoint
│   ├── Set headers, body, params
│   ├── Send request
│   ├── View response (formatted)
│   ├── View request cost (credits)
│   └── Save request templates
├── /usage                         # Usage analytics
│   ├── Request count (time-series)
│   ├── Credit consumption graph
│   ├── Top endpoints by usage
│   ├── Error rate breakdown
│   ├── Latency statistics
│   └── Rate limit hit frequency
├── /logs                          # Request log viewer
│   ├── Full request/response detail
│   ├── Search & filter
│   ├── Export (CSV / JSON)
│   └── Real-time tail mode
├── /credits                       # Credit management
│   ├── Current balance
│   ├── Transaction history
│   ├── Usage forecast
│   └── Purchase credits (if enabled)
├── /security                      # Security settings
│   ├── IP whitelist management
│   ├── API key rotation
│   └── Activity log (logins, key events)
└── /settings                      # Account settings
    ├── Profile (name, email, company)
    ├── Change password
    └── Notification preferences
```

### 20.2 API Playground

The built-in API playground lets users test endpoints directly from the portal:

```
┌──────────────────────────────────────────────────────────┐
│  API PLAYGROUND                                          │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  Endpoint: [GET ▼] [/api/v1/users________________]      │
│                                                          │
│  ┌─── Headers ──────────────────────────────────────┐    │
│  │ X-API-Key: [ck_live_ab...xxxxx] (auto-filled)    │    │
│  │ Content-Type: [application/json]                  │    │
│  │ [+ Add Header]                                    │    │
│  └──────────────────────────────────────────────────┘    │
│                                                          │
│  ┌─── Query Params ─────────────────────────────────┐    │
│  │ page: [1]    per_page: [20]                       │    │
│  │ [+ Add Param]                                     │    │
│  └──────────────────────────────────────────────────┘    │
│                                                          │
│  ┌─── Request Body (JSON) ──────────────────────────┐    │
│  │ {                                                 │    │
│  │   "name": "John Doe",                            │    │
│  │   "email": "john@example.com"                     │    │
│  │ }                                                 │    │
│  └──────────────────────────────────────────────────┘    │
│                                                          │
│  Cost: 1 credit  │  Balance: 9,847 credits               │
│                                                          │
│  [▶ Send Request]  [💾 Save Template]                    │
│                                                          │
│  ┌─── Response ─────────────────────────────────────┐    │
│  │ Status: 200 OK  │  Time: 45ms  │  Size: 1.2KB    │    │
│  │                                                   │    │
│  │ {                                                 │    │
│  │   "users": [                                      │    │
│  │     { "id": 1, "name": "John Doe" },              │    │
│  │     { "id": 2, "name": "Jane Smith" }             │    │
│  │   ],                                              │    │
│  │   "total": 42,                                    │    │
│  │   "page": 1                                       │    │
│  │ }                                                 │    │
│  └──────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────┘
```

### 20.3 User Authentication

```yaml
# User portal authentication
portal:
  enabled: true
  addr: ":9877"                    # Separate port from admin (or same with path prefix)
  path_prefix: "/portal"
  
  # Session management
  session:
    secret: "session-secret-key"
    max_age: "24h"
    cookie_name: "apicerberus_session"
    secure: true                   # HTTPS only
    same_site: "strict"
  
  # Registration
  registration:
    enabled: false                 # Admin creates users by default
    # If enabled:
    require_approval: true         # Admin must approve new registrations
    default_credits: 100           # Credits given on registration
    default_rate_limit_rps: 10
  
  # Password policy
  password:
    min_length: 8
    require_uppercase: true
    require_number: true
    require_special: false
```

---

## 21. Admin Panel (Enhanced)

### 21.1 Admin Panel Pages

The admin panel at `/dashboard` is enhanced with user/credit/audit management:

```
/dashboard
├── /                              # Platform overview
│   ├── Total users, active users
│   ├── Total credits distributed/consumed
│   ├── Revenue (if self-purchase enabled)
│   ├── Request volume, error rate
│   └── Real-time traffic graph
│
├── /users                         # User management
│   ├── User list (search, filter, sort)
│   ├── Create user (email, name, role, initial credits)
│   ├── Edit user (rate limits, status, metadata)
│   ├── Suspend / activate user
│   ├── View user's API keys
│   ├── View user's permissions
│   ├── View user's credit history
│   └── View user's request logs
│
├── /users/:id                     # User detail
│   ├── Profile & settings
│   ├── Credit balance & transactions
│   ├── API keys (create, revoke)
│   ├── Endpoint permissions (add, edit, remove)
│   ├── Rate limit configuration
│   ├── IP whitelist
│   ├── Usage analytics (per-user)
│   └── Audit log (per-user)
│
├── /credits                       # Credit management
│   ├── Platform credit overview
│   ├── Bulk credit assignment
│   ├── Credit pricing configuration
│   ├── Per-route cost configuration
│   ├── Transaction history (all users)
│   └── Revenue reports
│
├── /audit-logs                    # Audit log viewer
│   ├── Global request log (all users)
│   ├── Advanced search & filter
│   ├── Request/response detail view
│   ├── Export (CSV, JSON, JSONL)
│   ├── Retention settings
│   └── Archive management
│
├── /services                      # (existing) Service management
├── /routes                        # (existing) Route management
├── /upstreams                     # (existing) Upstream management
├── /plugins                       # (existing) Plugin management
├── /analytics                     # (existing) Traffic analytics
├── /cluster                       # (existing) Cluster management
├── /config                        # (existing) Configuration
└── /settings                      # Platform settings
    ├── Admin accounts
    ├── Portal configuration
    ├── Billing settings
    ├── Retention policies
    └── Webhook configuration
```

---

## 22. Enhanced Admin REST API (New Endpoints)

### 22.1 User Management Endpoints

```
# ── Users ─────────────────────────────────────
GET    /admin/api/v1/users                      # List users (paginated, searchable)
POST   /admin/api/v1/users                      # Create user
GET    /admin/api/v1/users/:id                  # Get user detail
PUT    /admin/api/v1/users/:id                  # Update user
DELETE /admin/api/v1/users/:id                  # Delete user (soft delete)
POST   /admin/api/v1/users/:id/suspend          # Suspend user
POST   /admin/api/v1/users/:id/activate         # Activate user
POST   /admin/api/v1/users/:id/reset-password   # Admin reset password

# ── User API Keys ─────────────────────────────
GET    /admin/api/v1/users/:id/api-keys         # List user's API keys
POST   /admin/api/v1/users/:id/api-keys         # Create API key for user
DELETE /admin/api/v1/users/:id/api-keys/:key_id # Revoke API key

# ── User Permissions ──────────────────────────
GET    /admin/api/v1/users/:id/permissions      # List user's endpoint permissions
POST   /admin/api/v1/users/:id/permissions      # Grant permission
PUT    /admin/api/v1/users/:id/permissions/:pid # Update permission
DELETE /admin/api/v1/users/:id/permissions/:pid # Revoke permission
POST   /admin/api/v1/users/:id/permissions/bulk # Bulk assign permissions

# ── User IP Whitelist ─────────────────────────
GET    /admin/api/v1/users/:id/ip-whitelist     # List whitelisted IPs
POST   /admin/api/v1/users/:id/ip-whitelist     # Add IP to whitelist
DELETE /admin/api/v1/users/:id/ip-whitelist/:ip # Remove IP from whitelist
```

### 22.2 Credit Management Endpoints

```
# ── Credits ───────────────────────────────────
GET    /admin/api/v1/credits/overview           # Platform credit overview
POST   /admin/api/v1/users/:id/credits/topup    # Add credits to user
POST   /admin/api/v1/users/:id/credits/deduct   # Deduct credits from user
GET    /admin/api/v1/users/:id/credits/balance   # Get user credit balance
GET    /admin/api/v1/users/:id/credits/transactions # Credit transaction history
POST   /admin/api/v1/credits/bulk-topup         # Bulk credit assignment

# ── Credit Configuration ─────────────────────
GET    /admin/api/v1/billing/config             # Get billing configuration
PUT    /admin/api/v1/billing/config             # Update billing config
GET    /admin/api/v1/billing/route-costs        # Get per-route costs
PUT    /admin/api/v1/billing/route-costs        # Update per-route costs
GET    /admin/api/v1/billing/revenue            # Revenue report
```

### 22.3 Audit Log Endpoints

```
# ── Audit Logs ────────────────────────────────
GET    /admin/api/v1/audit-logs                 # Search/filter audit logs
GET    /admin/api/v1/audit-logs/:id             # Get single log entry detail
GET    /admin/api/v1/audit-logs/export          # Export logs (CSV/JSON/JSONL)
GET    /admin/api/v1/audit-logs/stats           # Audit log statistics
DELETE /admin/api/v1/audit-logs/cleanup         # Trigger manual cleanup
GET    /admin/api/v1/audit-logs/retention       # Get retention config
PUT    /admin/api/v1/audit-logs/retention       # Update retention config

# ── Per-user audit logs ───────────────────────
GET    /admin/api/v1/users/:id/audit-logs       # User's request log
GET    /admin/api/v1/users/:id/audit-logs/export # Export user's logs
GET    /admin/api/v1/users/:id/audit-logs/stats  # User's log statistics
```

### 22.4 User Portal API (authenticated as user)

```
# ── Portal Auth ───────────────────────────────
POST   /portal/api/v1/auth/login                # Login (email + password)
POST   /portal/api/v1/auth/logout               # Logout
GET    /portal/api/v1/auth/me                   # Current user info
PUT    /portal/api/v1/auth/password             # Change password

# ── Portal API Keys ───────────────────────────
GET    /portal/api/v1/api-keys                  # List my API keys
POST   /portal/api/v1/api-keys                  # Generate new API key
PUT    /portal/api/v1/api-keys/:id              # Rename key
DELETE /portal/api/v1/api-keys/:id              # Revoke key

# ── Portal APIs ───────────────────────────────
GET    /portal/api/v1/apis                      # List available APIs (my permissions)
GET    /portal/api/v1/apis/:route_id            # API detail (docs, cost, limits)

# ── Portal Playground ─────────────────────────
POST   /portal/api/v1/playground/send           # Send test request
GET    /portal/api/v1/playground/templates       # Saved request templates
POST   /portal/api/v1/playground/templates       # Save request template
DELETE /portal/api/v1/playground/templates/:id   # Delete template

# ── Portal Usage ──────────────────────────────
GET    /portal/api/v1/usage/overview            # Usage summary
GET    /portal/api/v1/usage/timeseries          # Usage time-series
GET    /portal/api/v1/usage/top-endpoints       # Top endpoints by my usage
GET    /portal/api/v1/usage/errors              # My error breakdown

# ── Portal Logs ───────────────────────────────
GET    /portal/api/v1/logs                      # My request logs (paginated, filterable)
GET    /portal/api/v1/logs/:id                  # Request detail (full req/res)
GET    /portal/api/v1/logs/export               # Export my logs

# ── Portal Credits ────────────────────────────
GET    /portal/api/v1/credits/balance           # My credit balance
GET    /portal/api/v1/credits/transactions      # My credit history
GET    /portal/api/v1/credits/forecast          # Usage forecast
POST   /portal/api/v1/credits/purchase          # Purchase credits (if enabled)

# ── Portal Security ──────────────────────────
GET    /portal/api/v1/security/ip-whitelist     # My IP whitelist
POST   /portal/api/v1/security/ip-whitelist     # Add IP
DELETE /portal/api/v1/security/ip-whitelist/:ip # Remove IP
GET    /portal/api/v1/security/activity         # My activity log (logins, key events)

# ── Portal Settings ──────────────────────────
GET    /portal/api/v1/settings/profile          # My profile
PUT    /portal/api/v1/settings/profile          # Update profile
PUT    /portal/api/v1/settings/notifications    # Notification preferences
```

---

## 23. Version Roadmap

### v0.0.1 — Core Gateway
- HTTP/HTTPS reverse proxy with routing engine
- Basic service/route/upstream configuration (YAML)
- Round Robin + Weighted Round Robin load balancing
- Active health checking
- Admin REST API (services, routes, upstreams CRUD)
- CLI (start, stop, version, config validate)
- Structured logging (JSON)
- Hot reload (SIGHUP)

### v0.0.2 — Authentication & Rate Limiting
- API Key authentication (header, query, cookie)
- JWT validation (RS256, HS256)
- Token Bucket + Fixed Window rate limiting
- Consumer entity (API key ↔ consumer mapping)
- IP whitelist/blacklist plugin
- CORS plugin

### v0.0.3 — Full Load Balancing & Resilience
- All 10 LB algorithms (Least Conn, IP Hash, Random, Consistent Hash, Least Latency, Adaptive, Geo-aware, Health-weighted)
- Passive health checking
- Circuit breaker
- Retry with backoff
- Timeout control per route
- Sliding Window + Leaky Bucket rate limiting

### v0.0.4 — Transformation & Plugins
- Request/Response transformation (headers, body, path, query)
- URL rewriting (regex)
- Body template engine ($body, $timestamp, etc.)
- Plugin pipeline architecture (ordered middleware chain)
- Request size limit, request validator (JSON Schema)
- Compression (gzip/brotli)
- Correlation ID plugin
- Bot detection plugin

### v0.0.5 — Multi-Tenant Users & Credits
- Embedded SQLite for persistent data
- User management (admin + user roles)
- API key generation per user (ck_live_ / ck_test_)
- Credit system (balance, topup, consume, transactions)
- Per-route credit cost configuration
- Per-endpoint access control & permissions
- User IP whitelist
- Admin API: user/credit/permission endpoints

### v0.0.6 — Audit Logging & Analytics
- Request/response audit logging (full req/res capture)
- Sensitive field masking (headers, body fields)
- Log retention & cleanup scheduler
- Log archival (JSONL export)
- Analytics engine with ring buffer
- Time-series aggregation (requests, latency, errors)
- Analytics API endpoints

### v0.0.7 — Web Dashboard (Admin Panel)
- Embedded React 19 + Tailwind v4.1 + shadcn/ui dashboard
- Admin Panel: services, routes, upstreams, consumers CRUD
- User management UI (create, edit, suspend, permissions)
- Credit dashboard (balance, transactions, pricing editor)
- Audit log viewer (search, filter, detail, export)
- Analytics charts (Recharts: traffic, latency, errors, status codes)
- Config editor (CodeMirror 6 YAML)
- Dark/light theme, responsive, ⌘K command palette

### v0.0.8 — User Portal & Playground
- User Portal: login, dashboard, API key management
- Available APIs listing (permitted endpoints + docs)
- API Playground (send requests, view responses, credit cost preview)
- Usage analytics (per-user requests, credits, errors)
- Request log viewer (user-scoped)
- Credit balance & transaction history
- Security settings (IP whitelist, activity log)
- Profile & password management

### v0.0.9 — Topology & Flow Visualization
- React Flow: Cluster topology (placeholder for clustering)
- React Flow: Plugin pipeline visual (drag-to-reorder)
- React Flow: Upstream health map
- React Flow: Service dependency graph
- WebSocket real-time data feed (live metrics, log tail)
- Alert rules engine (high error rate, latency, upstream down)

### v0.1.0 — MCP Server & CLI Completion
- MCP server (stdio + SSE transport)
- All MCP tools (gateway, user, credit, audit, cluster management)
- MCP resources (services, routes, users, credits, config)
- Full CLI coverage (user, credit, audit, analytics commands)
- TLS termination with ACME/Let's Encrypt
- Config export/import/diff

### v0.2.0 — gRPC Support
- gRPC proxying (unary, streaming)
- gRPC-Web support
- gRPC health checking for upstreams
- gRPC metadata manipulation
- gRPC ↔ JSON transcoding (REST-to-gRPC bridge)
- Protocol auto-detection (gRPC vs HTTP)

### v0.3.0 — GraphQL Support
- GraphQL query proxying
- Query depth limiting
- Query complexity analysis & cost limiting
- Introspection control
- Field-level authorization
- Automatic persisted queries (APQ)
- Subscription proxying (WebSocket)
- React Flow: GraphQL federation schema view

### v0.4.0 — GraphQL Federation
- Schema federation (compose multiple GraphQL services)
- Query batching
- Federated subgraph management

### v0.5.0 — Raft Clustering & HA
- Raft consensus implementation (pure Go)
- Config replication across cluster nodes
- Distributed rate limiting (cluster-wide counters)
- Distributed credit balance (cluster-wide)
- Health check result sharing
- Cluster-wide analytics aggregation
- Audit log replication
- React Flow: live cluster topology with Raft state

### v0.6.0 — Advanced Features
- Response caching (in-memory, cache-control aware)
- Geo-aware load balancing
- Adaptive load balancing (dynamic algo switching)
- Prometheus metrics export (/metrics endpoint)
- OpenTelemetry tracing integration
- Webhook notifications (low balance, user events, alerts)

### v0.7.0 — Monetization & Enterprise
- Self-purchase credit packages (webhook-based payment verification)
- Usage-based billing reports & revenue dashboard
- Multi-workspace / organization support
- RBAC (role-based access control) beyond admin/user
- SSO / OAuth2 login for portal
- White-label portal (custom branding per deployment)

### v1.0.0 — Production Release
- Full test coverage
- Performance benchmarks verified (50K+ req/sec)
- Security audit
- Complete documentation (docs site)
- Docker images (multi-arch)
- Helm chart for Kubernetes
- Migration guides from Kong/Tyk/KrakenD

---

## 24. GitHub & Branding

- **Repository**: `github.com/APICerberus/APICerberus`
- **Organization**: `APICerberus`
- **Website**: `apicerberus.com`
- **License**: MIT
- **Tagline**: "Three-headed guardian for your APIs — Zero dependencies, single binary, full control."
- **Logo concept**: Three-headed dog silhouette formed from network/data flow lines, modern geometric style
- **Color palette**: Deep purple (#6B21A8) primary, crimson (#DC2626) accent, dark (#0F172A) background
