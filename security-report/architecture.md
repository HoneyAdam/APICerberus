# Architecture Map -- APICerebrus

**Generated:** 2026-04-14

## System Overview

APICerebrus is a production-ready API Gateway built in Go with a React-based admin dashboard. It provides routing, authentication, rate limiting, billing/credits, audit logging, GraphQL Federation, and Raft-based clustering.

## Services & Ports

| Service | Port | Protocol | Auth |
|---------|------|----------|------|
| Gateway HTTP | 8080 | HTTP/1.1, WebSocket | Plugin pipeline (configurable) |
| Gateway HTTPS | 8443 | HTTP/2, TLS | Plugin pipeline (configurable) |
| Admin API | 9876 | HTTP/JSON | Bearer JWT (HttpOnly cookie) or static X-Admin-Key |
| User Portal | 9877 | HTTP/HTML + REST | Session cookie (HttpOnly, SameSite) |
| gRPC | 50051 | HTTP/2 + Protobuf | gRPC metadata auth |
| Raft Consensus | 12000 | HTTP/RPC | Shared token (TLS-guarded) |
| MCP | stdio/SSE | JSON-RPC | X-Admin-Key |

## Tech Stack

### Backend (Go 1.26)
- **Language:** Go 1.26.2
- **Database:** SQLite via `modernc.org/sqlite` (pure Go, no CGO), WAL mode
- **JWT:** `github.com/golang-jwt/jwt/v5` (audited library) + custom wrapper
- **Raft:** Custom implementation with mTLS support
- **WebSocket:** `github.com/coder/websocket` v1.8.14
- **GraphQL:** `github.com/graphql-go/graphql` v0.8.1
- **WASM:** `github.com/tetratelabs/wazero` v1.11.0
- **Redis:** `github.com/redis/go-redis/v9` v9.7.3
- **gRPC:** `google.golang.org/grpc` v1.80.0
- **OIDC:** `github.com/coreos/go-oidc/v3` v3.18.0
- **YAML:** `gopkg.in/yaml.v3` + custom wrapper

### Frontend (React 19)
- **Framework:** React 19.2 + TypeScript 5.9 + Vite 8
- **UI:** shadcn/ui + Radix UI + Tailwind v4
- **State:** Zustand + Redux Toolkit (realtime store)
- **Data fetching:** TanStack React Query v5
- **Charts:** Recharts v3.8.1
- **Testing:** Vitest + Playwright + Testing Library

## Core Modules (`internal/`)

| Module | Purpose | Key Files |
|--------|---------|-----------|
| `gateway/` | HTTP/gRPC/WebSocket servers, radix tree router, proxy engine, 11 LB algorithms | `router.go`, `optimized_proxy.go`, `server.go`, `health.go` |
| `plugin/` | 5-phase pipeline (PRE_AUTH to POST_PROXY), 20+ plugins | `pipeline.go`, `auth_apikey.go`, `auth_jwt.go`, `cors.go` |
| `admin/` | REST API for management, webhook delivery, JWT token management, RBAC | `server.go`, `token.go`, `rbac.go`, `webhooks.go` |
| `portal/` | User-facing web portal with playground | `server.go`, `handlers_playground_usage.go` |
| `store/` | SQLite repositories (WAL mode): users, API keys, sessions, audit logs | `user_repo.go`, `api_key_repo.go`, `session_repo.go` |
| `raft/` | Custom Raft consensus, FSM, transport, mTLS cert manager | `node.go`, `fsm.go`, `transport.go` |
| `federation/` | GraphQL Federation (schema composition, query planning, executor) | `composer.go`, `planner.go`, `executor.go` |
| `analytics/` | Metrics with ring buffers, time-series aggregation, webhook templates | `engine.go`, `webhook_templates.go` |
| `audit/` | Async request/response logging with field masking, Kafka export | `logger.go`, `masker.go`, `retention.go` |
| `ratelimit/` | Token bucket, fixed/sliding window, leaky bucket; Redis-backed | `token_bucket.go`, `sliding_window.go` |
| `billing/` | Credit system with atomic SQLite transactions | `engine.go` |
| `mcp/` | Model Context Protocol server (stdio + SSE transports) | `server.go`, `call_tool.go` |
| `config/` | Configuration loading, env overrides, hot reload | `load.go`, `types.go`, `env.go` |
| `pkg/jwt/` | JWT wrapper over golang-jwt/jwt/v5 | `jwt.go`, `hs256.go`, `rs256.go`, `es256.go`, `jwks.go` |
| `pkg/netutil/` | Client IP extraction with trusted proxy support | `clientip.go` |

## Security Controls

### Implemented and Verified
- Parameterized SQL queries (zero string concatenation)
- `crypto/rand` for all secret generation (sessions, API keys, CSRF tokens)
- bcrypt cost 12 for passwords
- SHA-256 hashed API keys in database
- `crypto/subtle.ConstantTimeCompare` for auth comparisons
- Trusted proxy: forwarding headers ignored by default
- Security headers: X-Frame-Options DENY, X-Content-Type-Options, CSP, Permissions-Policy, Referrer-Policy
- RBAC with 4 roles (admin, manager, user, viewer) and 21 granular permissions
- SSRF protection on upstream hosts (`validateUpstreamHost`)
- GraphQL depth (15) and complexity (1000) limits
- WASM sandboxing with path traversal protection
- Plugin signature verification for marketplace
- CSRF double-submit on portal API
- Audit log PII masking with configurable fields
- Webhook HMAC-SHA256 signing
- JWT: 32-byte minimum HS256 secret, RSA 2048-bit minimum, algorithm explicit in verify

### Known Acknowledged Weaknesses
- WebSocket `wss:` requires HTTPS deployment (server-side origin validation exists)
- Recharts v3.8.1 (CVE was in 2.x line, not applicable)
- Client-side state caching (requires same-origin XSS to exploit)
