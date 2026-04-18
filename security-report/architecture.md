# APICerebrus Security Architecture Report

**Phase 1: Recon - Architecture Map**
**Date:** 2026-04-18 (updated)
**Project:** APICerebrus - Production API Gateway
**Classification:** INTERNAL

---

## 1. Tech Stack

### 1.1 Backend (Go 1.26.2)

| Library | Version | Purpose | Risk Profile |
|---------|---------|---------|--------------|
| `modernc.org/sqlite` | v1.48.0 | SQLite database (pure Go, no CGO) | Low - WAL mode |
| `github.com/redis/go-redis/v9` | v9.8.0 | Distributed rate limiting | Low - CVE-2025-49150 fixed |
| `google.golang.org/grpc` | v1.80.0 | gRPC server, HTTP transcoding | Low |
| `google.golang.org/protobuf` | v1.36.11 | Protocol buffers | Low |
| `github.com/golang-jwt/jwt/v5` | v5.3.1 | JWT parsing/validation | Medium - crypto dependency |
| `github.com/tetratelabs/wazero` | v1.11.0 | WASM runtime (sandboxed) | Medium - sandbox escape risk |
| `go.opentelemetry.io/otel/*` | v1.43.0 | Distributed tracing | Low |
| `golang.org/x/crypto` | v0.49.0 | Cryptographic operations | Low |
| `golang.org/x/oauth2` | v0.36.0 | OAuth2/OIDC integration | Medium |
| `github.com/coreos/go-oidc/v3` | v3.18.0 | OIDC provider | Medium |
| `gopkg.in/yaml.v3` | v3.0.1 | Config parsing | Low |
| `github.com/coder/websocket` | v1.8.14 | WebSocket support | Low |
| `github.com/andybalholm/brotli` | v1.2.1 | Brotli compression | Low |

### 1.2 Frontend (React 19.2.4)

| Component | Version |
|-----------|---------|
| React | 19.2.4 |
| TypeScript | 5.9.3 |
| Vite | 8.0.1 |
| Tailwind CSS | 4.2.2 |
| Zustand | 5.0.12 |
| TanStack Query | 5.95.2 |
| React Router | 7.13.2 |
| Radix UI | 1.4.3 |

### 1.3 Infrastructure

- **Database:** SQLite (WAL mode) / PostgreSQL (future)
- **Cache/RateLimit:** Redis 9.x (CVE-2025-49150 fixed in v9.8.0)
- **Message Queue:** Kafka (optional, audit export)
- **Tracing:** OpenTelemetry (Jaeger, Zipkin, OTLP, stdout)
- **Certificates:** ACME/Let's Encrypt, mTLS for Raft

---

## 2. Entry Points

### 2.1 Main Entry Point

```
cmd/apicerberus/main.go
    └── cli.Run(os.Args[1:])
            ├── start         → runStart()
            ├── stop          → runStop()
            ├── config        → runConfig()
            ├── mcp           → runMCP()
            ├── user          → runUser()
            ├── credit        → runCredit()
            ├── audit          → runAudit()
            ├── analytics      → runAnalytics()
            ├── service        → runService()
            ├── route          → runRoute()
            ├── upstream       → runUpstream()
            └── db             → runDB()
```

### 2.2 Network Entry Points

| Port | Service | Protocol | Auth Required | Purpose |
|------|---------|----------|---------------|---------|
| 8080 | Gateway HTTP | HTTP/1.1, HTTP/2 | Per-route (API key, JWT) | Proxy traffic |
| 8443 | Gateway HTTPS | TLS | Per-route | Secure proxy |
| 9876 | Admin API | REST, WebSocket | X-Admin-Key header / Bearer JWT | Management |
| 9877 | User Portal | HTTP | Session-based | User-facing |
| 50051 | gRPC | HTTP/2 | Per-method | gRPC services |
| 12000 | Raft | Custom RPC | mTLS (optional) | Clustering |
| stdio | MCP Server | JSON-RPC | None (local) | CLI tools |

---

## 3. Trust Boundaries

### 3.1 Component Trust Map

```
HIGH-TRUST ZONE (Internal)
├── Gateway Process ──────────────┐
│   - Radix Router (O(k) path match)
│   - Plugin Pipeline
│   - Load Balancer
│   - Proxy Engine
├── Admin API Process ────────────┤
│   - REST API Server
│   - OIDC Provider
│   - Webhook Manager
│   - GraphQL Federation
├── MCP Server ───────────────────┤
│   - JSON-RPC over stdio/SSE
│   - 25+ management tools
├── Store Layer ──────────────────┤
│   - SQLite WAL
│   - Repositories (users, api_keys, sessions, audit_logs)
├── Billing Engine ───────────────┤
├── Audit Logger ────────────────┤
└── Raft Cluster (optional) ─────┘

LOW-TRUST ZONE (External)
├── Client Requests ──────────────┐
│   - Untrusted input
│   - X-Forwarded-For (if trusted_proxies set)
├── Upstream Servers ─────────────┤
│   - Backend APIs proxied to
├── Redis Server ─────────────────┤
│   - Distributed rate limiting
├── Kafka (optional) ─────────────┤
│   - Audit log export
└── ACME/LE Servers ──────────────┘
```

### 3.2 Trust Boundaries Summary

| Boundary | Trust Level | Protection |
|----------|-------------|------------|
| Admin API → Store | VERY HIGH | X-Admin-Key, Bearer JWT, CSRF, RBAC |
| Gateway → Upstream | MEDIUM | Per-route auth, plugin pipeline |
| WASM → Host | LOW | wazero sandbox (128MB memory, no FS) |
| Raft → Raft | LOW-MEDIUM | mTLS optional, RPC secret |
| Client IP Extraction | SECURE DEFAULT | XFF ignored when trusted_proxies=[] |

---

## 4. Authentication Mechanisms

### 4.1 API Key Authentication (Gateway)

**Location:** `internal/plugin/auth_apikey.go`

```
Consumer API Keys:
├── Prefix: ck_live_* (production) or ck_test_* (test, bypasses credits)
├── Storage: SHA256 hash in api_keys table (raw key never stored)
├── Lookup: Hash bucket O(1) with linear scan fallback
├── Extraction: Header (X-API-Key), Query (apikey), Cookie
├── Validation: subtle.ConstantTimeCompare()
├── Expiry: Optional RFC3339 timestamp
└── Backoff: Per-IP auth failure rate limiting
```

### 4.2 Admin Authentication

**Location:** `internal/admin/token.go`

```
Admin Auth Methods:
├── Static Key: X-Admin-Key header (minimum 32 chars)
│   └── Constant-time comparison
├── Bearer Token: JWT (HS256)
│   ├── HttpOnly cookie or Authorization header
│   ├── 15-minute TTL (configurable)
│   ├── keyVersion embedded for rotation invalidation
│   ├── jti (unique token ID) for revocation
│   └── CSRF double-submit protection (M-014)
└── Session Cookie: apicerberus_admin_session
    ├── HttpOnly: true
    ├── Secure: true
    └── SameSite: StrictMode
```

### 4.3 OIDC Authentication

**Location:** `internal/admin/oidc.go`

```
OIDC Provider (Authorization Server):
├── /.well-known/openid-configuration
├── /oidc/jwks
├── /oidc/authorize
├── /oidc/token
├── /oidc/userinfo
├── /oidc/revoke
└── /oidc/introspect

OIDC Client (External IdP):
├── PKCE S256 support
├── State parameter CSRF protection
└── JWT validation with JWKS
```

### 4.4 Portal Session Authentication

**Location:** `internal/portal/server.go`

```
Portal Sessions:
├── Secret: configured in portal.session.secret
├── Cookie: Name + Secure + HttpOnly
└── TTL: portal.session.max_age
```

---

## 5. Data Flows

### 5.1 Gateway Request Flow

```
Client Request
     │
     ▼
┌─────────────────────────────┐
│ Security Headers Check      │ → X-Content-Type-Options, X-Frame-Options
│ MaxBodyBytes Check         │
└─────────────────────────────┘
     │
     ▼
┌─────────────────────────────┐
│ Health Endpoints (bypass)   │ → /health, /ready, /metrics
└─────────────────────────────┘
     │
     ▼
┌─────────────────────────────┐
│ Radix Tree Router           │ → O(k) path matching
│ Route Match (method-based)  │   Method-specific trees
└─────────────────────────────┘
     │
     ▼
┌─────────────────────────────┐
│ Plugin Pipeline (5 phases)  │
│ ─────────────────────────── │
│ PRE_AUTH (correlation_id,   │
│   ip_restrict, bot_detect)  │
│ AUTH (auth_apikey, auth_jwt,│
│   endpoint_permission)      │
│ PRE_PROXY (rate_limit, cors,│
│   request_validator, etc.)  │
│ PROXY (circuit_breaker,     │
│   retry, timeout, WASM)     │
│ POST_PROXY (response_trans, │
│   compression, WASM)        │
└─────────────────────────────┘
     │
     ├─ Auth Failed ──► 401/403 Error
     │
     ▼
┌─────────────────────────────┐
│ Billing Pre-Check            │ → ck_test_* bypasses
│ Credits deducted atomically  │   ck_live_* enforced
└─────────────────────────────┘
     │
     ├─ Insufficient ──► 402 Payment Required
     │
     ▼
┌─────────────────────────────┐
│ Upstream Selection           │ → 11 algorithms
│ (health-weighted)          │
└─────────────────────────────┘
     │
     ▼
┌─────────────────────────────┐
│ Proxy Engine                │ → Connection pooling
│ (optimized_proxy.go)        │   Retry, circuit breaker
└─────────────────────────────┘
     │
     ▼
┌─────────────────────────────┐
│ Response Capture            │ → Audit logging
│ + Analytics Record          │   Field masking
└─────────────────────────────┘
     │
     ▼
Client Response
```

### 5.2 Admin API Flow

```
External Client
     │
     ▼
┌─────────────────────────────┐
│ X-Admin-Key Header OR        │
│ Bearer Token / Cookie       │
└─────────────────────────────┘
     │
     ▼
┌─────────────────────────────┐
│ IP Allow-list Check         │ → cfg.Admin.AllowedIPs
│ (before auth)               │
└─────────────────────────────┘
     │
     ▼
┌─────────────────────────────┐
│ Rate Limit Check            │ → Per-IP failed auth tracking
│ (auth backoff)             │   Exponential backoff
└─────────────────────────────┘
     │
     ▼
┌─────────────────────────────┐
│ JWT Validation              │
│ - HS256 algorithm check     │
│ - Signature verification    │
│ - keyVersion check (M-001) │
│ - Expiry/iat/nbf validation│
└─────────────────────────────┘
     │
     ▼
┌─────────────────────────────┐
│ CSRF Double-Submit (M-014) │
│ - Cookie vs X-CSRF-Token    │
│ - Skip for GET/OPTIONS      │
│ - Skip for login endpoints  │
└─────────────────────────────┘
     │
     ▼
┌─────────────────────────────┐
│ RBAC Middleware             │ → Role extraction from JWT
│ (withRBAC)                 │   Future expansion
└─────────────────────────────┘
     │
     ▼
┌─────────────────────────────┐
│ REST Handlers               │ → CRUD routes, users, credits
│ GraphQL Federation          │   Subgraph management
│ Webhook Management          │
└─────────────────────────────┘
     │
     ▼
┌─────────────────────────────┐
│ Store Layer (SQLite WAL)    │
│ Audit Logging               │
└─────────────────────────────┘
```

### 5.3 Credit/Billing Flow

```
1. PRE-PROXY CREDIT CHECK
┌────────────┐     ┌────────────┐     ┌────────────┐
│ Get Consumer │ ─► │ Load User  │ ─► │ Get Credit │
│ from Auth    │   │ from DB    │   │ Balance    │
└────────────┘     └────────────┘     └────────────┘
                                           │
                    ┌──────────────────────┘
                    ▼
┌────────────┐     ┌────────────┐     ┌────────────┐
│ Check Test │ ─► │ Calc Route │ ─► │ Sufficient? │
│ Key Flag   │   │ Cost       │   │ Balance >=  │
│ ck_test_*  │   │            │   │ Cost?       │
└────────────┘     └────────────┘     └────────────┘
                     │                      │
     ┌───────────────┴──────────────┐       │
     │ No                            │ Yes   │
     ▼                               ▼       ▼
┌─────────────────┐        ┌──────────────────┐
│ 402 Payment      │        │ Continue to Proxy│
│ Required        │        │ Deducted atomically│
└─────────────────┘        └──────────────────┘

2. ATOMIC TRANSACTION
BEGIN TRANSACTION;
  INSERT INTO credit_transactions (...) VALUES (...);
  UPDATE users SET credit_balance = credit_balance - cost
  WHERE id = ? AND credit_balance >= cost;
COMMIT;
```

### 5.4 Audit Log Flow

```
Request Complete
      │
      ▼
┌─────────────────┐
│ Response Capture │ → Status, headers, body (optional)
└────────┬────────┘
         │
         ▼
┌─────────────────┐    ┌─────────────────┐
│ Field Masking   │ ─► │ MaskHeaders()   │ ← Authorization, X-API-Key
│                 │    │ MaskBody()      │ ← password, token fields
└────────┬────────┘    └─────────────────┘
         │
         ▼
┌─────────────────┐    ┌─────────────────┐
│ Non-blocking    │ ─► │ entries channel │ ← 10k buffer
│ Queue           │    │ (buffered)      │
└────────┬────────┘    └─────────────────┘
         │
         │    ┌────────────────────────────────────┐
         │    │ Background Goroutine (batch flush) │
         │    │ Every 1s OR 100 entries:           │
         │    │ ┌──────────────┐ ┌──────────────┐ │
         └────►│ BatchInsert  │ │ KafkaWriter  │ │
               │ (SQLite WAL) │ │ (async)      │ │
               └──────────────┘ └──────────────┘ │
               │ Retry: SQLITE_BUSY → backoff     │
               │ Drop: buffer full → l.dropped    │
               └────────────────────────────────────┘
         │
         ▼
┌─────────────────┐    ┌─────────────────┐
│ Retention       │ ─► │ Cleanup Every    │
│ Scheduler       │    │ 1h: DELETE old   │
└─────────────────┘    └─────────────────┘
```

---

## 6. Sensitive Assets

### 6.1 Secrets Inventory

| Secret | Location | Protection |
|--------|----------|------------|
| Admin API Key | `admin.api_key` config | Minimum 32 chars, constant-time compare |
| JWT Token Secret | `admin.token_secret` config | Minimum 32 chars, HS256 |
| Portal Session Secret | `portal.session.secret` config | Required |
| Raft RPC Secret | `cluster.rpc_secret` config | Inter-node auth |
| Redis Password | `redis.password` config | Optional |
| Kafka SASL Password | `kafka.sasl.password` config | Optional |
| OIDC Client Secret | `admin.oidc.client_secret` config | bcrypt hashed |
| ACME Account Email | `gateway.tls.acme_email` config | Not sensitive |

### 6.2 Protected Data

| Data | Storage | Protection |
|------|---------|------------|
| User Passwords | `users.password_hash` | bcrypt cost 12 |
| API Key Hashes | `api_keys.key_hash` | SHA256 (raw never stored) |
| Credit Balances | `users.credit_balance` | Atomic SQLite transactions |
| Audit Logs | `audit_logs` | Field masking, retention policies |
| Admin Sessions | `sessions` table | Token hash, expiry |

### 6.3 Security-Critical Files

| File | Purpose | Risk |
|------|---------|------|
| `cmd/apicerberus/main.go` | Entry point | Low |
| `internal/cli/run.go` | Command dispatcher | Medium - starts all servers |
| `internal/admin/token.go` | JWT/Bearer auth | HIGH - session management |
| `internal/admin/server.go` | Admin REST API | HIGH - management interface |
| `internal/plugin/auth_apikey.go` | API key auth | HIGH - consumer identity |
| `internal/plugin/wasm.go` | WASM sandbox | MEDIUM - sandbox escape |
| `internal/raft/tls.go` | Raft mTLS | MEDIUM - cert management |
| `internal/config/load.go` | Config parsing | MEDIUM - secret validation |
| `internal/pkg/netutil/clientip.go` | IP extraction | MEDIUM - spoofing prevention |
| `internal/billing/billing.go` | Credit operations | HIGH - financial |
| `internal/audit/logger.go` | Audit logging | MEDIUM - tamper evidence |

### 6.4 Database Schema (Sensitive Tables)

```sql
-- Users (password_hash is bcrypt)
users: id, email, name, password_hash, role, created_at, updated_at

-- API Keys (key_hash is SHA256 of raw key)
api_keys: id, user_id, name, key_hash, key_prefix, expires_at, created_at

-- Sessions (token_hash is SHA256 of JWT)
sessions: id, user_id, token_hash, expires_at, created_at

-- Credit Transactions (immutable ledger)
credit_transactions: id, user_id, amount, balance_after, reason, created_at

-- Audit Logs (field masking applied)
audit_logs: id, request_id, timestamp, user_id, method, path, status, duration_ms, masked_headers, masked_body
```

---

## 7. Security Controls Summary

### 7.1 Implemented Controls

| Control | Location | Maturity |
|---------|----------|----------|
| API Key hashing (SHA256) | `internal/plugin/auth_apikey.go` | High |
| Constant-time key comparison | `internal/plugin/auth_apikey.go:186` | High |
| Auth backoff (DoS protection) | `internal/plugin/auth_backoff.go` | High |
| WASM sandbox (wazero) | `internal/plugin/wasm.go` | Medium |
| XFF right-to-left parsing | `internal/pkg/netutil/clientip.go` | High |
| X-Real-IP validation (M-003) | `internal/pkg/netutil/clientip.go:162` | High |
| Credit atomic transactions | `internal/billing/billing.go` | High |
| SQL parameterization | `internal/store/*.go` | High |
| Admin key placeholder check | `internal/config/load.go` | Medium |
| Kafka TLS skip_verify check | `internal/config/load.go` | High |
| Batch size limits (M-012) | `internal/gateway/server.go` | Medium |
| CSRF double-submit (M-014) | `internal/admin/token.go` | High |
| Admin key rotation (M-001) | `internal/admin/token.go` | High |
| JWT keyVersion invalidation | `internal/admin/token.go` | High |
| bcrypt cost 12 | `internal/store/user_repo.go` | High |
| TLS 1.2+ minimum | `internal/config/load.go` | High |
| Raft mTLS with TLS 1.3 | `internal/raft/tls.go` | High |

### 7.2 Recent Security Fixes (2026-04-18)

| Commit | Fix |
|--------|-----|
| ae438d8 | go-redis/v9 upgrade to v9.8.0 (CVE-2025-49150) |
| d394dcf | Raft TLS: crypto/rand serial numbers, remove localhost SAN |
| 50e870d | GraphQL: escapeGraphQLString(), JSON encoding args |
| 50e870d | Open redirect: isValidRedirectTarget() scheme allow-list |
| ed2522a | OIDC: real auth via admin JWT session cookie |
| ed2522a | OIDC: PKCE S256 support |
| dd68aea | Admin API: CSRF double-submit protection (M-014) |
| c42e82b | OIDC userinfo signature verification |
| c42e82b | Admin key rotation invalidates sessions (keyVersion) |

---

## Appendix: File Reference Map

### Core Entry Points
- `cmd/apicerberus/main.go` - Application entrypoint
- `internal/cli/run.go` - Command dispatcher, starts gateway/admin/portal/raft

### Gateway
- `internal/gateway/server.go` - Main HTTP server, routing
- `internal/gateway/router.go` - Radix tree router
- `internal/gateway/optimized_proxy.go` - Proxy engine
- `internal/gateway/balancer.go` - Load balancing algorithms
- `internal/gateway/health.go` - Health checking

### Admin API
- `internal/admin/server.go` - REST API server
- `internal/admin/token.go` - JWT/Bearer auth, CSRF
- `internal/admin/rbac.go` - RBAC middleware
- `internal/admin/webhooks.go` - Webhook delivery

### Plugin System
- `internal/plugin/types.go` - Plugin interface, phases
- `internal/plugin/pipeline.go` - Pipeline execution
- `internal/plugin/registry.go` - Plugin registry
- `internal/plugin/auth_apikey.go` - API key auth
- `internal/plugin/auth_backoff.go` - Auth rate limiting
- `internal/plugin/wasm.go` - WASM sandbox

### Store Layer
- `internal/store/store.go` - Store initialization
- `internal/store/user_repo.go` - User repository
- `internal/store/api_key_repo.go` - API key repository
- `internal/store/credit_repo.go` - Credit transactions

### Billing
- `internal/billing/billing.go` - Billing engine

### Audit
- `internal/audit/logger.go` - Audit logger
- `internal/audit/masker.go` - PII masking

### Federation
- `internal/federation/` - GraphQL Federation

### Raft
- `internal/raft/node.go` - Raft node implementation
- `internal/raft/tls.go` - mTLS certificate management
- `internal/raft/transport.go` - RPC transport

### MCP
- `internal/mcp/server.go` - MCP server

### Config
- `internal/config/load.go` - Config loading/validation

### Networking
- `internal/pkg/netutil/clientip.go` - Client IP extraction

---

*Report generated: 2026-04-18*
*Reconnaissance completed with rtk commands and direct file analysis*
