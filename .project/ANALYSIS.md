# Project Analysis Report

> Auto-generated comprehensive analysis of APICerebrus
> Generated: 2026-04-14
> Analyzer: Claude Code — Full Codebase Audit

## 1. Executive Summary

APICerebrus is a production-grade API Gateway, Management, and Monetization Platform built in Go with an embedded React admin dashboard and developer portal. It provides HTTP/gRPC/WebSocket reverse proxying with radix-tree routing, a 5-phase plugin pipeline with 25+ built-in plugins, credit-based billing, distributed Raft clustering, GraphQL Federation, and a comprehensive admin REST API with 95+ endpoints. The target audience is platform teams and SaaS providers needing a self-hosted, embeddable API gateway with monetization capabilities.

**Key Metrics:**

| Metric | Value |
|--------|-------|
| Total Project Files | 1,175 |
| Go Source Files (non-test) | 179 |
| Go Test Files | 214 |
| Total Go LOC | 151,171 |
| Non-test Go LOC | 55,491 |
| Test Go LOC | 95,680 |
| Frontend Source Files | ~167 |
| Frontend LOC | ~23,305 |
| External Go Dependencies (direct) | 20 |
| External Frontend Dependencies | 48 |
| Test Coverage | 73.7% |
| Admin API Endpoints | 95+ |
| Portal API Endpoints | 32+ |
| MCP Tools | 43+ |
| Packages with Zero Tests | 2 |

**Overall Health Assessment: 7.5/10**

The codebase demonstrates exceptional breadth and depth for a Go gateway. Architecture is clean, security practices are above average (CWE annotations, SSRF protection, bcrypt, constant-time comparison), and test coverage at 73.7% is solid. The main concerns are: (1) two test suites failing consistently (ratelimit Redis fallback tests, integration TempDir cleanup on Windows), (2) configuration schema inconsistency between app config and K8s/Helm manifests, (3) significant documentation inconsistencies (version numbers, LOC claims, dependency counts), and (4) the frontend has low test coverage relative to its size.

**Top 3 Strengths:**
1. **Comprehensive feature surface** — Full API gateway with 11 load balancers, 25+ plugins, GraphQL Federation, Raft clustering, credit billing, MCP server, and admin CLI — all in a single binary.
2. **Security-conscious implementation** — CWE-annotated code, SSRF protection, constant-time key comparison, bcrypt cost 12, crypto/rand for secrets, comprehensive security headers, #nosec justifications documented.
3. **Well-structured Go code** — Clean package boundaries, proper context propagation, atomic hot-reload with mutex protection, graceful shutdown hooks, and consistent error wrapping.

**Top 3 Concerns:**
1. **Documentation-Reality Gap** — README claims "150,000+ LOC" and "85% coverage" but actual non-test LOC is 55K and coverage is 73.7%. TASKS.md shows 100% completion but CHANGELOG has unreleased fixes. Version numbers are inconsistent across docs.
2. **Failing Tests** — `internal/ratelimit` (6 tests) and `test/integration` (5 tests) fail consistently. The ratelimit failures are logic bugs in factory fallback behavior; the integration failures are Windows TempDir cleanup issues.
3. **K8s/Helm Config Schema Mismatch** — The Kubernetes ConfigMap and Helm chart use `server.address` and `auth.jwt.secret` while the actual application uses `gateway.http_addr` and `admin.token_secret`. Deploying via K8s/Helm would produce invalid configuration.

---

## 2. Architecture Analysis

### 2.1 High-Level Architecture

APICerebrus is a **modular monolith** — a single Go binary with internal packages organized by domain. The binary embeds the React frontend assets via `go:embed` and serves them, requiring no separate frontend deployment.

```
Client Request Flow:
====================

  Client --> Gateway (radix router) --> Plugin Pipeline (5 phases) --> Load Balancer --> Upstream
                  |                              |                            |
                  +- /health, /ready             +- PRE_AUTH: corr-id,       +- Active health
                  +- /metrics                     |  ip-restrict, bot-detect   |  checks
                  +- /admin/api/v1/*              +- AUTH: apikey, jwt,       +- Passive health
                  +- /portal/api/v1/*             |  ip-whitelist              |  checks
                  +- /dashboard/*                 +- PRE_PROXY: rate-limit,   +- Circuit breaker
                                                  |  request-validator, cors
                                                  +- PROXY: circuit-breaker,
                                                  |  retry, timeout, cache
                                                  +- POST_PROXY: response
                                                     transform, compression

Data Layer:
===========
  SQLite (WAL mode) -- Users, API Keys, Sessions, Credits, Audit Logs, Webhooks
  Redis (optional)  -- Distributed rate limiting
  Kafka (optional)  -- Audit log streaming

Clustering (optional):
======================
  Raft Consensus -- Config replication, distributed rate limiting, certificate sync
  mTLS           -- Inter-node encryption with auto-generated CA
```

**Concurrency Model:**
- Standard `net/http` server goroutine-per-request model
- `sync.RWMutex` for hot-reload of gateway state (router, pools, config)
- `sync.Map` for rate limiter sharding, connection pools
- `atomic` operations for metrics counters, balancer state
- `sync.Pool` for HTTP client reuse, buffer pooling
- Goroutine lifecycle: `context.WithCancel` for audit drain, health checkers, analytics engine
- Graceful shutdown via `shutdown.Manager` with LIFO hook execution

### 2.2 Package Structure Assessment

| Package | Responsibility | LOC (non-test) | Cohesion | Assessment |
|---------|---------------|----------------|----------|------------|
| `cmd/apicerberus` | Entry point | ~20 | High | Clean delegation to `cli.Run` |
| `internal/config` | Config types, loading, env overrides, watch | ~1,500 | High | Comprehensive validation, env override via reflection |
| `internal/gateway` | HTTP server, router, proxy, balancer, health | ~7,000 | Medium-High | Largest package; could split proxy/router/balancer |
| `internal/store` | SQLite repositories, migrations | ~4,000 | High | Consistent repo pattern, 8 tables |
| `internal/plugin` | 25+ plugin implementations, pipeline, registry | ~6,000 | High | Excellent phase-based pipeline architecture |
| `internal/admin` | REST API server, WebSocket, OIDC, RBAC | ~7,000 | Medium | Very large; graphql.go (860 LOC) and server.go (640 LOC) are oversized |
| `internal/billing` | Credit engine | ~300 | High | Clean, focused |
| `internal/ratelimit` | 4 algorithms + Redis distributed | ~1,200 | High | Good factory pattern |
| `internal/loadbalancer` | Subnet resolver, adaptive LB | ~500 | High | Focused utility package |
| `internal/raft` | Raft consensus, TLS, cert sync | ~3,500 | High | Complex but well-structured |
| `internal/federation` | GraphQL Federation composer/planner/executor | ~2,100 | High | Clean separation of concerns |
| `internal/graphql` | Parser, analyzer, APQ, proxy, subscriptions | ~2,200 | High | Comprehensive GraphQL support |
| `internal/grpc` | gRPC proxy, transcoding, health, streaming | ~1,500 | High | Full gRPC stack |
| `internal/audit` | Logger, capture, masking, retention, Kafka | ~1,300 | High | Async buffering with batch flush |
| `internal/analytics` | Ring buffer engine, alerts, webhook templates | ~1,500 | Medium | webhook_templates.go (718 LOC) is oversized |
| `internal/mcp` | MCP server, tools, resources | ~1,000 | High | 43+ tools for AI integration |
| `internal/portal` | User portal API | ~1,000 | High | Clean session-based auth |
| `internal/cli` | CLI commands, admin client | ~2,500 | Medium | cmd_user.go (744 LOC) is oversized |
| `internal/certmanager` | ACME/Let's Encrypt | ~400 | High | Focused ACME implementation |
| `internal/tracing` | OpenTelemetry setup | ~200 | High | Minimal, focused |
| `internal/metrics` | Prometheus metrics | ~600 | Medium | Single large file |
| `internal/migrations` | Migration runner | ~100 | High | Simple version tracker |
| `internal/shutdown` | Graceful shutdown manager | ~80 | High | LIFO hook execution |
| `internal/logging` | Structured logging, rotation | ~300 | High | Clean logging abstraction |
| `internal/pkg/*` | JWT, JSON, YAML, UUID, netutil, coerce, template | ~1,200 | High | Well-isolated utilities |

**Circular Dependency Risk:** Low. All packages depend on `internal/store` and `internal/config`, but no circular dependencies observed.

**Oversized Files (candidates for refactoring):**
- `internal/gateway/server.go` (1,213 LOC) — handles too many concerns
- `internal/admin/graphql.go` (860 LOC) — GraphQL admin API in one file
- `internal/admin/admin_users.go` (866 LOC) — user management CRUD
- `internal/gateway/balancer_extra.go` (842 LOC) — multiple balancer implementations
- `internal/plugin/registry.go` (817 LOC) — registry + route pipeline builder
- `internal/federation/executor.go` (792 LOC) — complex execution logic

### 2.3 Dependency Analysis

#### Go Dependencies (direct, from go.mod)

| Dependency | Version | Purpose | Maintenance | Could Use Stdlib? |
|------------|---------|---------|-------------|-------------------|
| `modernc.org/sqlite` | v1.48.0 | Pure-Go SQLite driver | Active | No — core storage |
| `google.golang.org/grpc` | v1.80.0 | gRPC framework | Active | No |
| `google.golang.org/protobuf` | v1.36.11 | Protocol buffers | Active | No |
| `go.opentelemetry.io/otel/*` | v1.43.0 | Distributed tracing | Active | No |
| `golang.org/x/crypto` | v0.49.0 | bcrypt, argon2 | Active | No — stdlib lacks bcrypt |
| `golang.org/x/net` | v0.52.0 | HTTP/2, context | Active | No — needed for h2 |
| `golang.org/x/oauth2` | v0.36.0 | OAuth2 client | Active | No |
| `golang.org/x/text` | v0.35.0 | Text processing | Active | No |
| `github.com/redis/go-redis/v9` | v9.7.3 | Redis client | Active | No |
| `github.com/alicebob/miniredis/v2` | v2.37.0 | Redis mock for tests | Active | No |
| `github.com/graphql-go/graphql` | v0.8.1 | GraphQL execution | Active | No |
| `github.com/tetratelabs/wazero` | v1.11.0 | WASM runtime | Active | No |
| `github.com/coder/websocket` | v1.8.14 | WebSocket (conforming) | Active | No |
| `github.com/coreos/go-oidc/v3` | v3.18.0 | OIDC client | Active | No |
| `github.com/golang-jwt/jwt/v5` | v5.3.1 | JWT parsing/validation | Active | No |
| `gopkg.in/yaml.v3` | v3.0.1 | YAML parsing | Active | No |

**Assessment:** All dependencies are actively maintained, well-known libraries. No deprecated or abandoned packages. The "zero dependencies" claim in README/BRANDING is misleading — there are 16 direct dependencies, but this is lean for the scope.

#### Frontend Dependencies (from web/package.json)

**Production (30 packages):** All at latest versions. React 19.2, React Router 7.13, TanStack Query 5.95, Zustand 5.0, Radix UI 1.4, Recharts 3.8, CodeMirror 6, Tailwind 4.2, Vite 8.0, TypeScript 5.9.

**Assessment:** No deprecated packages. `manualChunks` properly splits heavy deps (recharts, codemirror, react-flow) into separate bundles.

### 2.4 API & Interface Design

#### HTTP Endpoint Inventory

**Admin API (95+ endpoints on port 9876):**

| Category | Count | Methods |
|----------|-------|---------|
| Auth (token + OIDC SSO) | 8 | POST, GET |
| System (status, info, config) | 5 | GET, POST |
| Services CRUD | 5 | GET, POST, PUT, DELETE |
| Routes CRUD | 5 | GET, POST, PUT, DELETE |
| Upstreams CRUD + targets | 7 | GET, POST, PUT, DELETE |
| Users CRUD + status/role | 10 | GET, POST, PUT, DELETE |
| User API Keys | 3 | GET, POST, DELETE |
| User Permissions | 5 | GET, POST, PUT, DELETE |
| User IP Whitelist | 3 | GET, POST, DELETE |
| Credits | 8 | GET, POST |
| Audit Logs | 6 | GET, DELETE |
| Analytics | 8 | GET |
| Alerts | 4 | GET, POST, PUT, DELETE |
| Billing Config | 4 | GET, PUT |
| Subgraphs (Federation) | 5 | GET, POST, DELETE |
| WebSocket | 1 | GET (upgrade) |
| Webhooks | 4+ | GET, POST, PUT, DELETE |
| Bulk Operations | 2+ | POST |
| Advanced Analytics | 4+ | GET |
| GraphQL Admin | 1 | POST |
| pprof Debug | 5 | GET |
| Dashboard UI | 1 | GET (static) |
| Branding | 2 | GET |

**Portal API (32 endpoints on port 9877):** Auth (4), API Keys (4), APIs (2), Playground (4), Usage (4), Logs (3), Credits (4), Security (4), Settings (3).

**API Consistency Assessment:**
- **Naming:** Consistent REST patterns — plural nouns, `{id}` for resources
- **Response format:** JSON with consistent envelope
- **Error handling:** `PluginError` type with code/message/status; standard HTTP codes
- **Authentication:** Dual system — admin uses Bearer token; portal uses session cookies + CSRF
- **Rate limiting:** Per-route and per-user; not on admin endpoints by default

---

## 3. Code Quality Assessment

### 3.1 Go Code Quality

**Code Style:** Consistent. All files pass `go fmt`. Naming follows Go conventions.

**Error Handling:**
- Consistent `fmt.Errorf("context: %w", err)` wrapping
- `PluginError` type for pipeline errors with HTTP status codes
- Centralized `writeJSON`/`writeError` in admin handlers
- **Concern:** Fire-and-forget goroutines in `api_key_repo.go:UpdateLastUsed` log errors but don't propagate

**Context Usage:**
- All store methods accept `context.Context`
- Gateway creates per-request context with timeout
- **Concern:** `billing.Engine.Deduct()` uses `context.Background()` instead of request context

**Logging:** Structured JSON via `internal/logging`. Proper level usage (debug/info/warn/error).

**Configuration:** YAML + env vars + SIGHUP hot reload. Comprehensive validation with accumulated errors.

**Magic Numbers:** Security limits are hardcoded but reasonable (8KB path max, 256 segments, 1KB regex max). Connection pool settings (100 max idle, 90s timeout) should be configurable.

**TODOs:** Only 1 in non-test source: `internal/plugin/request_transform.go`.

### 3.2 Frontend Code Quality

**React Patterns:** 100% functional components + hooks. TanStack Query for server state. Zustand for client state. React.lazy + Suspense for code splitting.

**TypeScript Quality:** `strict: true` with additional flags. Zero `any` types. 45+ explicit type definitions.

**Component Structure:** Feature-based organization. Consistent shadcn/ui pattern (33 primitives).

**CSS:** Tailwind v4 with `@theme inline` directive. CSS custom properties for theming.

**Accessibility:** Semantic HTML, `aria-label` on icon buttons, touch targets enforced. **Concerns:** Missing `aria-sort` on DataTable, DiffViewer lacks ARIA.

**Frontend Test Coverage:** 11 test files for ~90 source files (~12%). Limited but infrastructure (MSW, Testing Library) is solid.

### 3.3 Concurrency & Safety

- `sync.RWMutex` for hot-reload — correct read/write separation
- `sync.Map` for rate limiter sharding — appropriate for read-heavy access
- `sync.Pool` for HTTP transport reuse — correct
- `atomic` operations for metrics — correct
- **Medium risk:** `api_key_repo.go:UpdateLastUsed` fire-and-forget goroutine without lifecycle management
- **Medium risk:** `denyPrivateUpstreams` package-level var — not goroutine-safe for concurrent init

### 3.4 Security Assessment

**Input Validation:** Body size limits, path length limits, regex length limits (CWE-1333), null byte rejection (CWE-20), JSON Schema validation plugin.

**SQL Injection:** All queries use parameterized placeholders. No string concatenation in SQL.

**XSS Protection:** Security headers (CSP, X-Frame-Options, X-Content-Type-Options), React JSX auto-escaping, `html.EscapeString` in error pages.

**Secrets Management:** Config uses `${ENV_VAR}` pattern. Initial password files gitignored. API keys SHA-256 hashed. bcrypt cost 12 for passwords. JWT secret validated for min 32 chars.

**TLS:** TLS 1.0/1.1 rejected. Safe cipher suites. ACME auto-provisioning.

**93 gosec suppressions** documented in `SECURITY-JUSTIFICATIONS.md` with justified reasons.

---

## 4. Testing Assessment

### 4.1 Test Coverage

**Overall Coverage: 73.7%**

**Test Results (latest run):**
- 32/34 packages PASS
- 2 packages FAIL:
  - `internal/ratelimit` — 6 tests fail: factory returns wrong type when Redis unavailable (logic bug)
  - `test/integration` — 5 tests fail: Windows TempDir cleanup (SQLite file handle not released)

**Packages with ZERO Tests:** `internal/migrations`, `internal/pkg/coerce`

### 4.2 Test Types Present

- **Unit tests:** 214 files across all packages
- **Integration tests:** `test/integration/` — auth flow, request lifecycle, plugin chain, hot reload, Kafka
- **E2E tests:** `test/e2e_*_test.go`
- **Benchmark tests:** `test/benchmark/`
- **Fuzz tests:** 4 files (router, JSON, JWT, YAML)
- **Load tests:** `test/loadtest/` — 500+ concurrent request validation

**CI Pipeline:** 12-job GitHub Actions with 70% coverage threshold enforcement.

---

## 5. Specification vs Implementation Gap Analysis

### 5.1 Feature Completion Matrix

| Planned Feature | Status | Notes |
|----------------|--------|-------|
| HTTP/1.1 + HTTP/2 reverse proxy | COMPLETE | Full proxy with coalescing |
| TLS + ACME | COMPLETE | Let's Encrypt auto-provisioning |
| WebSocket proxying | COMPLETE | Full-duplex tunneling |
| gRPC proxy + transcoding | COMPLETE | Native + Web + transcoding |
| GraphQL proxy + APQ | COMPLETE | Parser, analyzer, subscriptions |
| GraphQL Federation | COMPLETE | Apollo-compatible |
| Radix tree router | COMPLETE | O(k) with regex support |
| Plugin pipeline (5 phases) | COMPLETE | 25+ plugins |
| API Key auth | COMPLETE | Header/query/cookie |
| JWT auth (RS256/HS256/ES256/JWKS) | COMPLETE | Full JWT stack |
| 4 rate limit algorithms | COMPLETE | + Redis distributed |
| 11 load balancers | COMPLETE | SubnetAware added beyond spec's 10 |
| Health checking | COMPLETE | Active + passive |
| Raft clustering | COMPLETE | With mTLS |
| Credit billing | COMPLETE | Atomic SQLite transactions |
| Audit logging | COMPLETE | Async, PII masking, Kafka |
| Analytics engine | COMPLETE | Ring buffer, time-series |
| Admin REST API | COMPLETE | 95+ endpoints |
| User portal API | COMPLETE | 32 endpoints |
| Web dashboard | COMPLETE | React 19 + Tailwind v4 |
| CLI commands | COMPLETE | 40+ commands |
| MCP server | COMPLETE | 43+ tools |
| OIDC SSO | COMPLETE | Login/callback/logout/status |
| RBAC | COMPLETE | Role-based access |
| WASM plugins | PARTIAL | Implementation exists, minimal tests |
| Kafka audit streaming | PARTIAL | Writer exists, tested minimally |
| Plugin marketplace | PARTIAL | Implementation exists |
| Brotli compression | MISSING | Not implemented, Go stdlib lacks Brotli |

### 5.2 Architectural Deviations

1. **YAML parser:** IMPLEMENTATION.md describes custom parser. **Actual:** Uses `gopkg.in/yaml.v3`. Improvement.
2. **SQLite:** IMPLEMENTATION describes both CGO and pure-Go. **Actual:** Pure Go via `modernc.org/sqlite` with `CGO_ENABLED=0`.
3. **Password hashing:** TASKS/IMPLEMENTATION say SHA-256+salt. **Actual:** bcrypt cost 12. Significant security improvement.
4. **Load balancer count:** Spec says 10. **Actual:** 11 — SubnetAware added.

### 5.3 Task Completion Assessment

TASKS.md claims 490 tasks at 100%. Realistic estimate: **90-95%**. Unreleased fixes in CHANGELOG, failing tests, and undocumented features (WASM, Kafka, Marketplace) indicate the project is not fully complete.

### 5.4 Scope Creep Detection

| Unplanned Feature | Assessment |
|-------------------|------------|
| SubnetAware LB | Valuable replacement for "geo_aware" |
| JTI replay protection | Valuable security hardening |
| GraphQL guard | Valuable depth/complexity limiting |
| Request coalescing | Valuable performance optimization |
| Plugin marketplace | Questionable — adds complexity |
| Bulk operations | Valuable operational convenience |
| Advanced analytics (forecast) | Valuable beyond spec |

### 5.5 Missing Critical Components

1. **Brotli compression** — Low priority, gzip sufficient
2. **K8s/Helm config schema alignment** — **High priority**, will break K8s deployments
3. **Frontend error boundaries** — Medium priority
4. **OpenAPI spec sync** — Medium priority

---

## 6. Performance & Scalability

### 6.1 Performance Patterns

**Hot Paths:** `Gateway.ServeHTTP()` → radix tree O(k) → plugin pipeline → `OptimizedProxy.Forward()` → `Balancer.Next()` → upstream

**Potential Bottlenecks:**
- SQLite WAL write serialization under heavy audit + credit load
- Rate limiter `sync.Map` per-key mutex under extreme cardinality
- Admin WebSocket hub broadcasts to all connections without topic filtering
- Audit `LIKE` queries on text columns on large tables

**Memory:** Buffer pools, ring buffers (fixed size), rate limiter maps grow unbounded (no TTL cleanup for stale keys)

### 6.2 Scalability Assessment

- **Horizontal:** Stateless serving with Raft config replication; Redis for distributed rate limiting
- **Billing:** Limited — SQLite single-writer requires Raft leader for credit operations
- **Sessions:** Server-side SQLite — no sticky sessions but requires leader DB access

---

## 7. Developer Experience

**Onboarding:** `go build` works with zero config. Docker compose straightforward. Example config comprehensive.

**Documentation:** README is good but has inflated metrics. CLAUDE.md is excellent and accurate. SPECIFICATION is extremely detailed (2,848 lines).

**Build/Deploy:** Makefile with 30+ targets. Multi-stage Docker. Cross-compilation for 5 platforms. 12-job CI pipeline.

---

## 8. Technical Debt Inventory

### Critical (blocks production)

1. **Ratelimit factory fallback bug** — `internal/ratelimit/redis.go` — 6 test failures. Fix: 2h.
2. **K8s/Helm config schema mismatch** — deployment manifests vs app config. Fix: 4-8h.
3. **Integration test cleanup on Windows** — SQLite handle leak. Fix: 2-4h.

### Important (before v1.0)

4. **Documentation metric inflation** — README inaccurate. Fix: 1h.
5. **Fire-and-forget goroutine** — `api_key_repo.go:UpdateLastUsed`. Fix: 2h.
6. **Frontend test coverage** — 12% vs ~90 files. Fix: 40-60h.
7. **Dockerfile HEALTHCHECK syntax** — exec form with shell operator. Fix: 15min.
8. **Admin port exposed in prod compose** — port 9876 `mode: host`. Fix: 15min.
9. **Helm secret template non-idempotent** — regenerates on upgrade. Fix: 1h.
10. **Duplicate Makefile targets** — Fix: 15min.
11. **GoReleaser not integrated with CI** — maintained but unused. Fix: 2-4h.

### Minor

12. Missing frontend error boundaries. 1-2h.
13. `use-cluster.ts` DRY violation. 1h.
14. `BrandingProvider.tsx` raw fetch. 30min.
15. Version number inconsistency across docs. 30min.
16. BRANDING.md font references outdated. 15min.
17. Binary artifacts in working directory. Cleanup.
18. Missing `aria-sort` on DataTable. 1h.
19. K8s Secret placeholder values. 30min.
20. Monitoring alert duplication across 3 files. 2-3h.

---

## 9. Metrics Summary Table

| Metric | Value |
|--------|-------|
| Total Project Files | 1,175 |
| Total Go Files | 393 |
| Go Source Files (non-test) | 179 |
| Go Test Files | 214 |
| Total Go LOC | 151,171 |
| Non-test Go LOC | 55,491 |
| Test Go LOC | 95,680 |
| Frontend Source Files | ~167 |
| Frontend LOC | ~23,305 |
| Frontend Test Files | 11 |
| Test Coverage | 73.7% |
| Packages with Zero Tests | 2 |
| Failing Test Packages | 2 |
| External Go Dependencies | 20 |
| External Frontend Dependencies | 48 |
| Admin API Endpoints | 95+ |
| Portal API Endpoints | 32+ |
| MCP Tools | 43+ |
| Built-in Plugins | 25+ |
| Load Balancing Algorithms | 11 |
| Open TODOs (non-test) | 1 |
| #nosec Suppressions | 93 |
| Spec Feature Completion | ~95% |
| Overall Health Score | 7.5/10 |
