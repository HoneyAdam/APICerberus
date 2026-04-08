# APICerebrus Production Readiness Analysis

> Generated: 2026-04-08  
> Scope: Full-stack codebase, architecture, and operational posture  
> Auditor stance: Brutally honest. No sugarcoating.

---

## 1. Executive Summary

APICerebrus is a feature-rich API Gateway built in Go with a React 19 admin dashboard. On paper it checks many boxes: radix-tree routing, 10 load-balancing algorithms, a 6-phase plugin pipeline, SQLite-backed persistence, Raft clustering, gRPC/WebSocket support, GraphQL federation, billing/credits, audit logging, and OpenTelemetry tracing. The codebase is large, well-organised into `internal/` packages, and has extensive unit test coverage.

However, **there is a significant gap between claimed maturity and production-hardened reality**. Several foundational security, reliability, and operational concerns remain unaddressed. The project is **not yet safe to deploy in a hostile production environment** without remediation.

---

## 2. Architecture Assessment

### 2.1. Strengths

| Area | Observation |
|------|-------------|
| **Modularity** | Clean separation between gateway, admin, portal, store, raft, plugin, and analytics packages. |
| **Router** | Custom radix-tree router with host-bound method trees, wildcard support, regex fallback, and atomic hot-reload (`Rebuild`). Route priority and longest-prefix matching are implemented correctly. |
| **Load Balancing** | 10 algorithms implemented: round-robin, weighted round-robin, least-connections, IP-hash, random, consistent-hash, least-latency, adaptive, geo-aware, health-weighted. Most have nil-safe guards and no obvious divide-by-zero paths. |
| **Persistence** | SQLite via `modernc.org/sqlite` (pure Go, no CGO). WAL mode by default. Schema migrations are versioned and transactional. |
| **Test Coverage** | Extensive table-driven tests across almost every package. Race-detection and benchmark targets exist in `Makefile`. |
| **Observability** | OpenTelemetry tracing exporter (stdout/OTLP), structured JSON logger, ring-buffer analytics with P50/P95/P99 latency percentiles. |

### 2.2. Architectural Weaknesses

| Weakness | Impact |
|----------|--------|
| **Stateful single-node SQLite** | The core store is SQLite on local disk. Clustering (Raft) replicates log entries and snapshots, but the database itself is not distributed. Failover requires manual orchestration or shared storage; SQLite limits horizontal scaling of the data plane. |
| **Two authentication domains** | Gateway consumers (`config.Consumer` + YAML API keys) and portal/admin users (`store.User` + SQLite) are separate namespaces. A user created in the admin UI does **not** automatically receive gateway API-key access unless manually added as a `consumer`. This is confusing and operationally risky. |
| **MCP server hardcodes cluster status** | `internal/mcp/server.go` `cluster.status` and `cluster.nodes` tools return static mock data (`"mode": "standalone"`). If this is exposed to operators, it will silently lie about cluster health. |
| **Geo-aware balancer uses fake GeoIP** | `loadbalancer.GeoIPResolver` resolves countries from the first two octets of an IP address. This is not real GeoIP; it will misroute production traffic. |

---

## 3. Security Analysis

### 3.1. Authentication & Authorization

#### Gateway Auth (JWT Plugin)
- **Supported algs**: HS256, RS256 only. No ES256 or EdDSA support.
- **Rejects `none`**: Correctly rejects the `"NONE"` algorithm.
- **JWKS**: Has a JWKS client with TTL-based caching (`internal/pkg/jwt/jwks.go`).
- **Clock skew**: 30s default, configurable.
- **Claims validation**: Enforces `exp`, optional `iss` and `aud`. Custom `requiredClaims` supported.
- **Missing**: `nbf` validation, `jti` replay prevention, token binding, and key rotation without restart.

#### Gateway Auth (API Key Plugin)
- Uses `crypto/subtle.ConstantTimeCompare` — good.
- Keys are stored **in-memory from YAML config**, not in the database. This means:
  - No runtime key rotation via API.
  - Keys survive only as long as the process holds the config snapshot.
  - A hot config reload is required to add/remove keys.
- There is no rate-limiting on failed API-key lookups, opening the door to enumerable-key brute-force attacks.

#### Admin / Portal Auth
- **Admin dashboard**: Uses a single `X-Admin-Key` header. The key is stored in **browser `localStorage`** (`web/src/lib/api.ts`). This is a stored-XSS vulnerability: any successful XSS payload on the admin domain can exfiltrate the admin key silently.
- **Portal sessions**: Cookie-based session auth. The example config ships with `secret: "change-me-in-production"` and `secure: false`. If an operator copies the example YAML without editing these values, session forgery becomes trivial.
- **No MFA / SSO**: Not implemented. Acceptable for v1.0, but must be documented as a known gap.

### 3.2. Network & Transport Security

| Issue | Location | Severity |
|-------|----------|----------|
| **Blind trust of `X-Forwarded-For`** | `internal/logging/structured.go:getClientIP`, `internal/gateway/balancer_extra.go:extractClientIP` | **High** |
| **No hop-by-hop validation on `X-Forwarded-For`** | Same as above. First IP in the header is taken verbatim, allowing trivial client-IP spoofing for rate-limiting, audit logging, and geo-routing. | High |
| **Custom WebSocket origin check** | `internal/admin/ws.go` | Medium |
| **Webhook per-request `http.Client`** | `internal/admin/webhooks.go:processDelivery` | Medium |
| **No TLS minimum version / cipher suite config** | `internal/gateway/server.go`, TLS code | Medium |

- **X-Forwarded-For**: Both the logger and the balancer extract the first comma-separated entry of `X-Forwarded-For` without any allow-listed proxy parsing. In a production deployment behind a trusted load balancer, this will cause rate-limit and audit inaccuracies; in a direct-exposure scenario, it is fully spoofable.
- **WebSocket**: The admin WebSocket endpoint implements a manual hijack with a custom `isValidWebSocketOrigin` check instead of using `gorilla/websocket`'s battle-tested `Upgrader.CheckOrigin`. This is fragile and likely bypassable with crafted `Origin` headers.
- **Webhook client**: A brand-new `http.Client{Timeout: timeout}` is created for every webhook delivery. This destroys connection reuse and creates GC pressure under high webhook volume.

### 3.3. Data Security

- **Password hashing**: `store.HashPassword` uses `argon2id` (confirmed via `store/user_repo.go` references in tests). Good.
- **Audit masking**: Configurable header and body field masking with `***REDACTED***` replacement. Headers like `Authorization` and `X-API-Key` are masked by default.
- **SQLite file permissions**: Not explicitly set on database creation. If the process runs as root, the DB may be created world-readable.
- **Session tokens**: Stored as hashes in SQLite (`token_hash`), but no rotation on privilege-change or global logout.

---

## 4. Reliability & Correctness

### 4.1. Gateway Core

#### Proxy (`internal/gateway/optimized_proxy.go`)

| Concern | Detail |
|---------|--------|
| **Request coalescing body exhaustion** | `serveCoalescedResponse` reads the entire upstream body into memory with `io.ReadAll(io.LimitReader(resp.Body, 50<<20))` (50 MB). If 100 waiters coalesce on the same request, the body is held in memory 100 times. This is not true zero-copy and is a memory-exhaustion vector. |
| **Buffer pool double-Put risk** | `writeResponse` does `buf := p.Get(); defer p.Put(buf)` then passes `buf` to `io.CopyBuffer`. If the writer panics, the buffer is returned to the pool via `defer`. This is safe, but `Put` does not zero the buffer, so sensitive response data may leak across requests if buffers are reused without bounds checking in `ReverseProxy`. |
| **Coalescing scope** | Only GET/HEAD are coalesced, but the coalescing key ignores `Vary: Cookie` and `Vary: Authorization`, meaning authenticated responses could be shared across users. |
| **Transport timeout** | `OptimizedProxy.executeRequest` calls `transport.RoundTrip(req)` with no per-request deadline. The transport-level timeouts (`ResponseHeaderTimeout`, `TLSHandshakeTimeout`) exist, but a slow-upload upstream can still hang the proxy indefinitely. |

#### Body Limits (`gateway/server.go` snippets)

```go
if g.config.Gateway.MaxBodyBytes > 0 && r.ContentLength > g.config.Gateway.MaxBodyBytes {
    http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
    return
}
if g.config.Gateway.MaxBodyBytes > 0 && r.Body != nil {
    r.Body = io.NopCloser(io.LimitReader(r.Body, g.config.Gateway.MaxBodyBytes+1))
}
```

- `ContentLength` can be `-1` (chunked/unknown), so the first check may be bypassed.
- The `LimitReader` limit is `MaxBodyBytes+1`, but there is no enforcement that the handler actually reads the extra byte and rejects the request. If the upstream handler ignores body size, the limit is effectively advisory.

### 4.2. Balancers

- **RoundRobin / WeightedRoundRobin**: Correctly guarded against empty healthy targets.
- **LeastConn**: Active-connection counting is done in-process only. In a multi-instance deployment, each instance counts independently, so the algorithm is approximate at best.
- **Adaptive**: Switches between round-robin, least-latency, and least-conn based on EWMA error rate and latency. The threshold (25% errors) is hardcoded. Total/error counters are halved at 50k to avoid overflow; this is pragmatic but makes long-term trending noisy.
- **HealthWeighted**: Score bumps by `+0.15` on success and `-0.40` on failure, clamped to `[0, 1]`. A single failure requires ~3 successes to recover, which may be too punitive for transient blips.

### 4.3. Storage & Data Integrity

- **Migrations**: Run automatically on `store.Open()`. There is no explicit migration-lock (e.g. `PRAGMA locking_mode`) — concurrent process starts could race on schema creation. In practice SQLite WAL mode makes this unlikely, but not impossible.
- **Credit transactions**: The billing engine uses atomic SQLite transactions for balance updates (confirmed via `internal/store/credit_repo.go` tests). This is correct.
- **Admin user bootstrap**: `ensureInitialAdminUser` creates a hardcoded admin if none exists. The password is hashed, but the existence of a bootstrap account must be clearly documented so operators do not leave it active.
- **Connection pool note**: `store.Open` sets `MaxOpenConns(1)` **only for `:memory:` databases**, which is correct because in-memory SQLite is per-connection. For file-backed databases the default is 25. The previous audit incorrectly flagged this as a production blocker for all deployments.

### 4.4. Analytics Engine

```go
// internal/analytics/engine.go
b.latencies = append(b.latencies, metric.LatencyMS)
```

- **`latencies` slice is unbounded** inside a one-minute `bucketAggregate`. Under high throughput (e.g. 100k RPS), this slice will grow to millions of entries within a minute. The `sync.RWMutex` around the time-series store means every `Record` contends with `Buckets` reads. Memory usage and GC pause times will spike.
- **Cleanup inside write lock**: `Record` calls `cleanupLocked` while holding the mutex, meaning reads block during retention scans.

### 4.5. Raft Clustering

- **Pure-Go implementation**: Leader election, log replication, and snapshots are present. This is impressive for a single codebase.
- **No TLS for Raft RPC**: From what is visible, Raft communication is plaintext. In a multi-region deployment this is a non-starter.
- **Certificate sync**: `internal/raft/certificate_sync.go` exists, but the integration with the Raft transport is not clearly hardened against split-brain during cert rotation.

### 4.6. Rate Limiting

- **Token bucket**: In-memory only. Correct per-key state with `sync.Map` of `sync.Mutex` buckets. No eviction of stale buckets — memory grows linearly with the number of unique keys (IPs, users, routes).
- **Redis fallback**: If Redis is enabled, local state syncs on miss. The Redis client (`go-redis/v9`) is reliable, but the fallback logic needs chaos testing.

---

## 5. Frontend Analysis

### 5.1. Stack

- React 19, Vite 8, Tailwind CSS v4, shadcn/ui components, TanStack Query, Zustand, Recharts.
- Vitest + Happy DOM for testing.

### 5.2. Security

- **Admin API key in `localStorage`**: As noted above, this is the most critical frontend security flaw.
- **No CSP meta tag**: No Content-Security-Policy is injected into `index.html` (observed by absence in `web/` glob results).
- **No anti-CSRF tokens for portal**: The portal uses cookie-based sessions, but there is no visible CSRF double-submit pattern for mutating requests.

### 5.3. Quality / Completeness

- **Placeholder pages**: `App.tsx` renders `PlaceholderPage` for any nav item not explicitly mapped. Several screens are marked "In Progress".
- **Lint/typecheck no-ops**:
  ```json
  "lint": "echo 'Skipping type check for test files...'"
  ```
  The web project has disabled its own linter and type-checker in `package.json`. This implies the frontend currently has TypeScript errors that are being ignored.
- **No E2E tests**: No Playwright or Cypress config found.

### 5.4. API Layer

- `adminApiRequest` in `web/src/lib/api.ts` is clean: abort-controller timeouts, JSON parsing, error wrapping. It does not retry transient 5xx errors, however.

---

## 6. Operational & Deployment Posture

### 6.1. Build & CI

- `Makefile` is comprehensive: build, test, race, coverage, lint, docker, k8s deploy targets.
- `go.mod` claims Go 1.25.0 (which does not exist as of 2026-04-08; latest stable is 1.24.x). This is almost certainly a typo.
- **Dependency claim contradiction**: `.project/IMPLEMENTATION.md` states "zero external Go dependencies", yet `go.mod` includes `fsnotify`, `go-redis`, OpenTelemetry, `x/crypto`, `x/net`, `grpc`, `protobuf`, and `modernc.org/sqlite`. This is a significant documentation integrity issue.

### 6.2. Configuration

- `apicerberus.example.yaml` is thorough but contains dangerous defaults:
  - `admin.api_key: "change-me"`
  - `portal.session.secret: "change-me-in-production"`
  - `portal.session.secure: false`
  - `cors.allowed_origins: ["*"]` in the global plugins example
- Validation in `internal/config/load.go` is extensive and guards against many misconfigurations (negative timeouts, missing upstreams, duplicate names, etc.).

### 6.3. Observability

- **Metrics**: Prometheus-style metrics endpoint exists (`/metrics`).
- **Health checks**: `make health` curls `localhost:8080/health`.
- **Tracing**: OpenTelemetry SDK is wired up with batching, but there is no graceful flush on shutdown in the main gateway (the tracer has `Shutdown`, but it's unclear if `gateway.Gateway.Shutdown` invokes it).
- **Logging**: Structured JSON logger writes to `os.Stdout` with hooks. No log sampling or cardinality limits on dynamic fields, which could explode log volume in production.

### 6.4. Backup / DR

- `make backup` and `make restore` targets exist, backed by shell scripts. These are SQLite-file backups. No continuous-replication or point-in-time-recovery strategy is visible.

---

## 7. Documentation Integrity

| Document | Claim | Reality |
|----------|-------|---------|
| `.project/TASKS.md` | ~490 tasks, all marked `[x]` complete | Many "completed" tasks (e.g. "Dark mode support", "Plugin marketplace") correspond to placeholder or partially implemented code. |
| `.project/IMPLEMENTATION.md` | "zero external Go dependencies" | 20+ external dependencies in `go.mod`. |
| `.project/IMPLEMENTATION.md` | Geo-aware routing "resolves client IP to a country using a GeoIP database" | Uses `GeoIPResolver` that resolves from the first two octets of an IP — not a real GeoIP database. |
| `CLAUDE.md` | "10 load balancing algorithms" | Correct. |
| `CLAUDE.md` | "6-phase pipeline" | Correct. |

---

## 8. Test Quality

### 8.1. Positives

- Table-driven patterns with `t.Parallel()` are standard.
- Edge-case coverage is good for utilities (JWT parsing, billing calculations, YAML coercion).
- Race-detection target exists (`make test-race`).

### 8.2. Negatives

- **Coverage padding**: Files like `internal/gateway/gateway_100_test.go`, `internal/analytics/optimized_engine_test.go`, and `internal/grpc/coverage_test.go` are clearly named to inflate coverage numbers rather than test meaningful behaviour.
- **Integration/E2E tests are sparse**: The `test/` directory has build tags (`//go:build integration`, `//go:build e2e`), but the actual integration coverage is thin compared to the unit-test volume.
- **No chaos / fault-injection tests**: Missing tests for SQLite corruption, Raft split-brain, Redis outage during rate-limiting, or upstream body-closure panics.

---

## 9. Summary of Critical Findings

| # | Finding | Risk | File(s) |
|---|---------|------|---------|
| C1 | **Admin API key stored in `localStorage`** | Stored-XSS → full admin compromise | `web/src/lib/api.ts` |
| C2 | **Blind trust of `X-Forwarded-For`** | IP spoofing, rate-limit bypass, audit inaccuracy | `internal/logging/structured.go`, `internal/gateway/balancer_extra.go` |
| C3 | **Example config ships with weak secrets and `secure: false`** | Session forgery, credential stuffing | `apicerberus.example.yaml` |
| C4 | **Analytics `latencies` slice grows unbounded per minute** | OOM under load | `internal/analytics/engine.go` |
| C5 | **Request coalescing buffers entire response per waiter** | Memory exhaustion | `internal/gateway/optimized_proxy.go` |
| C6 | **MCP cluster tools return hardcoded mock data** | Operational blind spots | `internal/mcp/server.go` |
| C7 | **Webhook manager creates new `http.Client` per delivery** | Connection exhaustion, GC pressure | `internal/admin/webhooks.go` |
| C8 | **Two disjoint identity systems (Users vs Consumers)** | ACL gaps, operational confusion | `internal/store/`, `internal/config/` |
| C9 | **Frontend lint/typecheck disabled** | Undetected TS bugs ship to production | `web/package.json` |
| C10 | **Body limit is advisory, not enforced** | Large-body DoS | `internal/gateway/server.go` |

---

## 10. Appendix: Notable Code Smells

1. `generateInstanceID` in `internal/tracing/tracing.go` uses `time.Now()` twice, producing slightly inconsistent identifiers.
2. `extractClientIP` and `getClientIP` are duplicated with minor differences across packages.
3. `LogHook` in `internal/logging/structured.go` runs synchronously after encode; a slow hook blocks the request goroutine.
4. `gateway/server.go` has a 1400+ line `ServeHTTP` with nested conditionals — high cyclomatic complexity, difficult to reason about for security reviews.
5. `go.mod` declares `go 1.25.0`, which does not exist.
