# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## ⚠️ MANDATORY LOAD

**Before any work in this project, read and obey `AGENT_DIRECTIVES.md` in the project root.**

All rules in that file are hard overrides. They govern:
- Pre-work protocol (dead code cleanup, phased execution)
- Code quality (senior dev override, forced verification, type safety)
- Context management (sub-agent swarming, decay awareness, read budget)
- Edit safety (re-read before/after edit, grep-based rename, import hygiene)
- Commit discipline (atomic commits, no broken commits)
- Communication (state plan, report honestly, no hallucinated APIs)

**Violation of any rule is a blocking issue.**

---

## Project Overview

APICerebrus is a production-ready API Gateway built in Go with a React-based admin dashboard. It provides routing, authentication, rate limiting, billing/credits, audit logging, and GraphQL Federation with Raft-based clustering.

## Build & Development Commands

| Task | Command |
|------|---------|
| Full build (includes web) | `make build` |
| Go only (skip web build) | `go build -o bin/apicerberus ./cmd/apicerberus` |
| Run all tests | `make test` or `go test ./...` |
| Run single test | `go test -run TestName ./path/to/package` |
| Race detection | `make test-race` |
| Coverage report | `make coverage` → opens `coverage/coverage.html` |
| Integration tests | `make integration` (requires `-tags=integration`) |
| E2E tests | `make e2e` (requires `-tags=e2e`) |
| Benchmarks | `make benchmark` |
| Lint | `make lint` (runs `go vet` + `golangci-lint` if available) |
| Format | `make fmt` or `go fmt ./...` |
| Security scan | `make security` |
| CI pipeline | `make ci` (fmt + lint + test-race + security + coverage) |

**Web Dashboard** (React + Vite + Tailwind v4 + shadcn/ui):
- Location: `web/`
- Dev server: `cd web && npm run dev`
- Build: `cd web && npm ci && npm run build`
- The Makefile `build` target embeds web assets automatically

## Architecture

### Core Modules (`internal/`)

| Module | Purpose |
|--------|---------|
| `gateway/` | HTTP/gRPC/WebSocket servers, radix tree router, proxy engine, 10 load balancing algorithms |
| `plugin/` | 5-phase pipeline (PRE_AUTH→AUTH→PRE_PROXY→PROXY→POST_PROXY), 20+ plugins |
| `ratelimit/` | Token bucket, fixed/sliding window, leaky bucket; Redis-backed for distributed |
| `billing/` | Credit system with atomic SQLite transactions |
| `store/` | SQLite repositories (WAL mode): users, API keys, sessions, audit logs |
| `admin/` | REST API for management (default port 9876) |
| `portal/` | User-facing web portal (default port 9877) |
| `mcp/` | Model Context Protocol server (stdio + SSE transports) |
| `raft/` | Distributed consensus for clustering |
| `federation/` | GraphQL Federation (Apollo-compatible schema composition, query planning) |
| `graphql/` | GraphQL query parsing, execution, subscriptions |
| `grpc/` | gRPC server, HTTP transcoding, gRPC-Web |
| `analytics/` | Metrics collection with ring buffers and time-series aggregation |
| `audit/` | Request/response logging with field masking, GZIP compression |
| `cli/` | 40+ CLI commands for administration |

### Plugin Architecture

The plugin system uses a **5-phase pipeline** defined in `internal/plugin/types.go`:

```
PRE_AUTH → AUTH → PRE_PROXY → PROXY → POST_PROXY
```

Each plugin implements the `Plugin` interface and registers for specific phases:
- **PRE_AUTH**: Correlation ID, IP restrictions, bot detection
- **AUTH**: API key, JWT, user IP whitelist
- **PRE_PROXY**: Rate limiting, request validation, transforms, CORS
- **PROXY**: Circuit breaker, retry, timeout, caching
- **POST_PROXY**: Response transforms, compression

Plugins are chained in `internal/plugin/pipeline.go` and executed sequentially per phase.

### Request Flow

```
Client → Gateway (Radix Router) → Plugin Pipeline → Load Balancer → Upstream
              ↓                         ↓                    ↓
        Admin API                   Audit Logging      Health Checks
                                  Analytics
```

### Key Files

- `cmd/apicerberus/main.go` - Application entrypoint
- `internal/config/load.go` - Configuration parsing and validation
- `internal/config/dynamic_reload.go` - Hot config reload (SIGHUP)
- `internal/gateway/router.go` - Radix tree router with O(k) path matching
- `internal/gateway/optimized_proxy.go` - HTTP proxy with connection pooling
- `internal/store/*.go` - Repository pattern for SQLite entities
- `apicerberus.example.yaml` - Comprehensive configuration example
- `embed.go` - Embeds web dashboard into Go binary

### Router Implementation

The radix tree router (`internal/gateway/router.go`) supports:
- **Static paths**: `/api/v1/users`
- **Wildcard parameters**: `/api/v1/users/*id` → `Params(req)["id"]`
- **Host matching**: Routes can be scoped to specific hosts
- **Method-based trees**: Separate radix trees per HTTP method for O(k) lookup

Path parameters are extracted during routing and stored in request context. Retrieve with `gateway.Params(req)`.

### Load Balancing Algorithms

Defined in `internal/loadbalancer/`:
1. **Round Robin** - Sequential distribution
2. **Weighted Round Robin** - Proportional by weight
3. **Least Connections** - Fewest active connections
4. **IP Hash** - Consistent hashing on client IP
5. **Consistent Hash** - Hash ring for cache affinity
6. **Random** - Uniform random selection
7. **Least Latency** - Lowest observed response time
8. **Adaptive** - Dynamic weight adjustment based on health
9. **Health Weighted** - Weights adjusted by health score
10. **Weighted Least Connections** - Combined weight and connection count
11. **GeoAware (Subnet-based)** - Routes by IP subnet, NOT real GeoIP. See note below.

**⚠️ GeoAware Balancer Note:**
The "GeoAware" algorithm in `internal/loadbalancer/geo.go` does NOT use real GeoIP data.
It only groups IPs by their first two octets (subnet) for basic regional routing.
For true geographic routing, integrate MaxMind GeoIP2 or similar database.

## Critical Implementation Details

### SQLite (pure Go)
- Uses `modernc.org/sqlite` (no CGO required)
- WAL mode enabled for better concurrency
- **Never delete** `-wal` or `-shm` files while running

### Store Layer

SQLite repositories in `internal/store/` follow a consistent pattern:
- `New(db *sql.DB) *Store` constructor
- Context-aware methods: `Get(ctx, id)`, `Create(ctx, entity)`, `Update(ctx, entity)`, `Delete(ctx, id)`
- Transaction support: `WithTx(tx *sql.Tx) *Store`
- **Always uses WAL mode** - never delete `-wal` or `-shm` files

Key repositories: `user`, `apikey`, `session`, `auditlog`, `route_cost`

### Web Dashboard Architecture

**Location**: `web/` - React + TypeScript + Vite + Tailwind v4 + shadcn/ui

**Key patterns**:
- API client in `web/src/lib/api.ts` - centralizes all Admin API calls
- WebSocket connection in `web/src/lib/ws.ts` - real-time updates via `/admin/api/v1/ws`
- React Query hooks in `web/src/hooks/` - data fetching with caching
- Components use shadcn/ui with Tailwind v4 utility classes

**Development**:
```bash
cd web && npm run dev      # Dev server on port 5173
cd web && npm run build    # Production build → dist/
```

The Go binary embeds `web/dist/` via `embed.go` and serves at `/dashboard` (configurable).

### API Key Conventions
- Live keys: `ck_live_*` prefix
- Test keys: `ck_test_*` prefix (bypass credit checks)

### Rate Limiting
- Local: In-memory implementations (token bucket, sliding window, etc.)
- Distributed: Redis-backed with fallback to local on connection failure
- Configured per-route or per-user in `ratelimit/` package

### GraphQL Federation

Apollo-compatible federation in `internal/federation/`:
- **Schema Composition** (`composer.go`): Merges subgraph schemas using `@key`, `@external`, `@requires`, `@provides` directives
- **Query Planning** (`planner.go`): Analyzes GraphQL queries and generates execution plans across subgraphs
- **Subgraph Management** (`subgraph.go`): Registers/unregisters subgraphs with health checking
- **Execution** (`executor.go`): Parallel query execution with result merging

Subgraphs are defined in config and composed at runtime. Queries requiring multiple subgraphs use query planning to minimize requests.

### Raft Clustering

Distributed consensus in `internal/raft/`:
- **Node** (`node.go`): Single Raft node implementation using hashicorp/raft
- **FSM** (`fsm.go`): Finite state machine for applying cluster configuration changes
- **Transport** (`transport.go`): HTTP-based RPC between cluster nodes with optional mTLS
- **TLS** (`tls.go`): Certificate manager for mTLS with automatic CA/node cert generation
- **Storage** (`storage.go`): BoltDB-backed log store and stable store
- **Multi-region** (`multiregion.go`): Geographic distribution support with region-aware routing

Cluster operations:
- Join: `POST /admin/api/v1/cluster/join` with node address
- Leave: `POST /admin/api/v1/cluster/leave/{node_id}`
- Status: `GET /admin/api/v1/cluster/status`

**Raft mTLS Encryption**:
Inter-node communication can be encrypted with mutual TLS:
- **Auto-generate**: Leader generates CA, shares via Raft log, followers auto-enroll
- **Manual**: Provide CA cert + node certs via config paths
- **TLSCertificateManager** (`tls.go`): Handles CA generation, node cert signing, PEM import/export

Configuration in `apicerberus.example.yaml`:
```yaml
cluster:
  mtls:
    enabled: true
    auto_generate: true      # Auto-generate CA and certs
    auto_cert_dir: "./certs" # Where to store generated certs
    # Or manual mode:
    # ca_cert_path: "/path/to/ca.crt"
    # node_cert_path: "/path/to/node.crt"
    # node_key_path: "/path/to/node.key"
```

### MCP Server

Model Context Protocol implementation in `internal/mcp/`:
- **Transports**: stdio (for CLI tools) and SSE (for web clients)
- **Tools**: 25+ built-in tools for gateway inspection and management
- **Resources**: Exposes configuration, metrics, and audit logs as resources
- **Integration**: Uses in-process admin API for real-time data

Start MCP server:
```bash
apicerberus mcp start --transport stdio
apicerberus mcp start --transport sse --port 3000
```

### Audit Logging

Request/response logging in `internal/audit/`:
- **Async buffering** (`logger.go`): Ring buffer with batch flush to SQLite
- **Field masking** (`masker.go`): Automatic PII redaction (passwords, tokens, auth headers)
- **Retention** (`retention.go`): Automated cleanup with configurable retention per route
- **Kafka export** (`kafka.go`): Optional streaming to Kafka for SIEM integration
- **GZIP compression**: Automatic compression of archived logs

Configuration in `apicerberus.example.yaml`:
- `audit.enabled`: Master toggle
- `audit.retention_days`: Default retention period
- `audit.mask_headers`: Headers to redact
- `audit.mask_body_fields`: JSON fields to redact

### Analytics & Metrics

Real-time metrics in `internal/analytics/`:
- **Ring buffers**: Lock-free circular buffers for high-throughput collection
- **Time-series aggregation**: Per-minute rollup of latency, throughput, errors
- **Top-K tracking**: Top routes and consumers by request volume
- **Prometheus**: Metrics exposed at `/metrics` endpoint

Key metric types:
- Request latency (p50, p95, p99)
- Throughput (requests/minute)
- Error rates by status code
- Active connections

### Health Checks

Upstream health monitoring in `internal/gateway/health.go`:
- **Active checks**: Periodic HTTP/TCP probes to upstream targets
- **Passive checks**: Marks unhealthy on connection failures
- **Circuit breaker**: Automatic recovery with exponential backoff
- **Health scores**: Used by adaptive load balancing algorithms

### Default Ports
| Service | Port |
|---------|------|
| Gateway HTTP | 8080 |
| Admin API | 9876 |
| User Portal | 9877 |
| gRPC | 50051 |
| Raft | 12000 |

### CLI Reference

Key commands in `internal/cli/`:

```bash
# Gateway
apicerberus start --config apicerberus.yaml
apicerberus stop
apicerberus config validate apicerberus.yaml

# Users
apicerberus user create --email "user@example.com" --name "John Doe"
apicerberus user apikey create --user <id> --name "Production Key" --mode live
apicerberus user credit topup --user <id> --amount 1000 --reason "Bonus"

# Gateway entities
apicerberus service add --name "api" --upstream "upstream1"
apicerberus route add --name "api-route" --service "api" --paths "/api/*"
apicerberus upstream add --name "upstream1" --algorithm round_robin
apicerberus upstream target add --upstream "upstream1" --address "localhost:3000"

# Audit & Analytics
apicerberus audit search --user <id> --since 2024-01-01
apicerberus audit tail --follow
apicerberus analytics overview

# Clustering
apicerberus cluster status
apicerberus cluster join --address "192.168.1.10:12000"

# MCP
apicerberus mcp start --transport stdio
```

### Testing

- Unit tests: Standard `*_test.go` files alongside source
- Integration tests: `test/*_test.go` with `//go:build integration` tag
- E2E tests: `test/e2e_*_test.go` with `//go:build e2e` tag
- All tests use table-driven patterns with parallel subtests

**Test patterns:**
```go
func TestFeature(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {"case1", "input1", "output1"},
        {"case2", "input2", "output2"},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            result := Feature(tt.input)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

**Test database:** Tests use `:memory:` SQLite or temporary files cleaned up after each test.

### WebSocket Support

Bidirectional WebSocket proxying in `internal/gateway/`:
- Full-duplex tunneling between client and upstream
- Connection upgrade detection (`Connection: Upgrade`, `Upgrade: websocket`)
- Configurable origin allow-list for security
- Message framing and ping/pong handling
- Integrated with plugin pipeline (auth applies before upgrade)

**Dashboard WebSocket** (`internal/admin/ws.go`):
- Real-time updates via `/admin/api/v1/ws`
- JSON message protocol for events (gateway stats, config changes)
- Automatic reconnection with exponential backoff
- Client-side in `web/src/lib/ws.ts`

### Admin API Authentication

All Admin API endpoints require `X-Admin-Key` header:
```bash
curl -H "X-Admin-Key: your-secret-key" http://localhost:9876/admin/api/v1/status
```

The admin key is configured in `admin.api_key` and must be:
- Minimum 32 characters
- Cryptographically random (use `openssl rand -base64 32`)
- **Never commit to version control**

Some endpoints (like config reload) may require additional verification.

### Hot Reload

Configuration hot reload via SIGHUP:
```bash
kill -HUP <pid>
# or
apicerberus config reload --config apicerberus.yaml
```

**Reloadable** (no restart needed):
- Routes, services, upstreams
- Rate limits, billing settings
- Plugins and their configuration
- Audit log settings

**Requires restart**:
- Port bindings (gateway, admin, portal)
- TLS certificates
- Database path
- Raft cluster configuration

### gRPC Support

Full gRPC stack in `internal/grpc/`:
- **Native gRPC server**: HTTP/2 with protobuf message handling
- **gRPC-Web**: Browser-compatible gRPC with base64 encoding
- **HTTP transcoding**: REST JSON → gRPC conversion
- **Reflection**: Server reflection for grpcurl/debugging

Configuration:
```yaml
grpc:
  enabled: true
  addr: ":50051"
  enable_web: true
  enable_transcoding: true
```

### Webhook System

Event webhooks in `internal/admin/webhook.go`:
- **Event types**: `request.completed`, `user.created`, `credit.low`, `alert.triggered`
- **Delivery**: Async with retry (exponential backoff)
- **Signing**: HMAC-SHA256 signature in `X-Webhook-Signature` header
- **Circuit breaker**: Automatic disable on repeated failures

Webhook configuration in admin API:
```bash
POST /admin/api/v1/webhooks
{
  "url": "https://example.com/webhook",
  "events": ["request.completed", "user.created"],
  "secret": "webhook-signing-secret"
}
```

### Security Best Practices

1. **Secrets management**: Use environment variables for all secrets (`APICERBERUS_ADMIN_API_KEY`, `APICERBERUS_TOKEN_SECRET`)
2. **HTTPS in production**: Enable TLS with ACME/Let's Encrypt auto-provisioning
3. **Rate limiting**: Always configure per-route and per-user rate limits
4. **IP restrictions**: Use `user_ip_whitelist` plugin for admin access
5. **Audit logging**: Enable with appropriate retention for compliance
6. **Test keys**: Use `ck_test_*` keys for development, `ck_live_*` for production
7. **CORS**: Configure strict origin allow-list, avoid `*` in production

### Performance Tuning

Key settings for high throughput:
```yaml
gateway:
  read_timeout: "30s"
  write_timeout: "30s"
  max_body_bytes: 10485760  # 10MB

store:
  busy_timeout: "5s"
  journal_mode: "WAL"

audit:
  buffer_size: 10000  # Increase for high throughput
  batch_size: 100
  flush_interval: "1s"
```

Connection pooling in `internal/gateway/optimized_proxy.go`:
- HTTP keep-alive with configurable idle timeout
- Max idle connections per host: 100
- Connection reuse reduces latency significantly

### Run a specific test
```bash
go test -run TestCreditDeduction ./internal/billing/...
```

### Run tests with verbose output
```bash
go test -v ./internal/gateway/...
```

### Build and run locally
```bash
make build
./bin/apicerberus start --config apicerberus.yaml
```

### Check test coverage for a package
```bash
go test -coverprofile=coverage.out ./internal/billing
go tool cover -func=coverage.out
```
