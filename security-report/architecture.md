# APICerebrus Architecture Map — Security Audit Reference

## Tech Stack

| Component | Version | Notes |
|-----------|---------|-------|
| Go | 1.26.2 | Backend |
| React | 19.2.4 | Dashboard |
| Vite | 8.0.1 | Build tool |
| Node.js | unpinned | Frontend build |
| SQLite | modernc.org/sqlite v1.48.0 | Default database |
| PostgreSQL | jackc/pgx v5.9.1 | Optional HA/cluster |
| Redis | go-redis/v9 v9.7.3 | Optional distributed rate limiting |
| JWT | golang-jwt/jwt/v5 v5.3.1 | Admin authentication |
| gRPC | google.golang.org/grpc v1.80.0 | API proxying |
| WASM | tetratelabs/wazero v1.11.0 | Plugin sandbox |
| WebSocket | coder/websocket v1.8.14 | Admin real-time |
| OIDC | coreos/go-oidc/v3 v3.18.0 | SSO integration |

## Service Ports

| Port | Service | Trust Level | Protocol |
|------|---------|-------------|----------|
| 8080 | Gateway HTTP | UNTRUSTED | net/http |
| 8443 | Gateway HTTPS | UNTRUSTED | net/http + TLS |
| 50051 | Gateway gRPC | UNTRUSTED | h2c |
| 9876 | Admin API | TRUSTED | net/http |
| 9877 | Portal API | TRUSTED | net/http |
| 12000 | Raft RPC | CLUSTER | net/http (mTLS optional) |
| 9090 | Prometheus | MONITORING | net/http |

## Core Modules

```
internal/
├── admin/        # Admin REST API (:9876), OIDC IdP, webhooks, GraphQL, WebSocket hub
├── gateway/      # HTTP proxy engine (:8080/:8443), routing, load balancing
├── portal/       # User portal (:9877), session auth
├── store/        # SQLite/PostgreSQL persistence (users, api_keys, sessions, audit)
├── raft/        # Distributed consensus, cluster communication
├── plugin/      # Plugin registry, WASM runtime, auth plugins (API key, JWT, OIDC)
├── ratelimit/   # Token bucket, sliding window, leaky bucket; Redis-backed
├── billing/     # Credit-based billing engine
├── audit/       # Audit logging, Kafka export, retention
├── graphql/     # GraphQL query parsing, APQ
├── federation/  # GraphQL Federation, subgraph composition, query planning
├── mcp/         # Model Context Protocol server (stdio + SSE)
├── certmanager/ # ACME/Let's Encrypt certificate management
├── tracing/     # OpenTelemetry tracer initialization
├── config/      # YAML config loading, env var overrides, validation
├── cli/         # CLI commands (user, credit, analytics, audit, config)
├── analytics/   # Analytics engine, alerting
├── loadbalancer/# Load balancing algorithms
├── logging/     # Structured logging with rotation
└── migrations/  # Database schema migrations
```

## Authentication Mechanisms

### Admin API (:9876)
1. **Static API Key** — `X-Admin-Key` header, validated via `subtle.ConstantTimeCompare`
2. **JWT Bearer Token** — issued by `/admin/api/v1/auth/token`, HS256, 15-min TTL default
3. **IP Allow-list** — checked before auth via CIDR matching
4. **Rate Limiting** — 5 failures per 15-min window per IP, 30-min block
5. **OIDC SSO** — optional OIDC provider for SSO login

### Portal API (:9877)
1. **Session Cookie** — `apicerberus_session`, HttpOnly, Secure, SameSite=Lax
2. **CSRF Double-Submit** — `csrf_token` cookie + `X-CSRF-Token` header
3. **Rate-Limited Login** — 5 failures per 15-min window per IP

### Gateway (API Consumers)
1. **API Key** — `X-API-Key`, `Authorization`, query param, or cookie
2. **JWT Validation** — `auth-jwt` plugin validates HS256/RS256/ES256 at gateway level

## Security Boundaries

```
[Internet]
    │
    ▼
Gateway :8080 / :8443 (UNTRUSTED)
    │
    ├─► Plugin Pipeline (PRE_AUTH → AUTH → PRE_PROXY → PROXY → POST_PROXY)
    ├─► Radix Tree Router
    ├─► Load Balancer (11 algorithms)
    └─► Audit Logger (async ring buffer)
            │
            ├─► SQLite WAL
            ├─► Kafka (optional, full bodies)
            └─► Retention scheduler

[Admin Network]
    │
    ▼
Admin API :9876 (TRUSTED)
    │
    ├─► Static API Key validation
    ├─► JWT Bearer validation
    ├─► IP Allow-list check
    ├─► RBAC enforcement
    └─► OIDC Provider (unauthenticated endpoints)

[User Network]
    │
    ▼
Portal API :9877 (TRUSTED)
    │
    ├─► Session cookie validation
    ├─► CSRF double-submit
    └─► Rate limiting

[Cluster Network]
    │
    ▼
Raft :12000 (CLUSTER)
    │
    ├─► Optional mTLS (RSA 4096-bit CA)
    ├─► RPC secret (shared, in config)
    └─► BoltDB log storage
```

## Key Security Controls

| Control | Location |
|---------|----------|
| SQL injection prevention | All store/*.go use parameterized queries |
| JWT algorithm enforcement | admin/token.go:97 rejects non-HS256 |
| RSA key size minimum | pkg/jwt/rs256.go:64 enforces 2048-bit |
| bcrypt password hashing | store/user_repo.go:499 cost 12 |
| Constant-time comparison | All secret comparisons use crypto/subtle |
| TLS minimum version | gateway/tls.go:102-105 TLS 1.2 floor |
| Session token generation | crypto/rand 32-byte, SHA-256 hashed |
| Secure cookies | HttpOnly + Secure + SameSite |
| WASM sandboxing | plugin/wasm.go: WASI gated by AllowFilesystem |
| SSRF protection | gateway/proxy.go validateUpstreamHost() |
| Webhook signing | admin/webhooks.go HMAC-SHA256 |
| Audit PII masking | audit/masker.go default mask list |
| GraphQL depth/complexity | plugin/graphql_guard.go depth 15, complexity 1000 |
| IP allow-list | admin/token.go:144-147 CIDR matching |
| Rate limiting | ratelimit/*.go token bucket, sliding window, leaky bucket |
| CORS validation | plugin/cors.go allowlist with wildcard rejection |
| OIDC state/nonce | crypto/rand generation, constant-time comparison |

## Configuration Files

| File | Purpose | Security-Relevant |
|------|---------|-----------------|
| `apicerberus.example.yaml` | Full annotated example | OTLP Bearer token placeholder |
| `deployments/docker/.env.example` | Docker env vars | Default Grafana password |
| `deployments/monitoring/.env.example` | Monitoring credentials | SMTP, Slack, PagerDuty credentials |
| `deployments/kubernetes/base/secret.yaml` | K8s secrets | Hardcoded placeholder secrets |
| `deployments/kubernetes/base/configmap.yaml` | K8s config | Empty api_key, token_secret |
| `.github/workflows/ci.yml` | CI/CD pipeline | Secrets in helm args, KUBE_CONFIG handling |

## Positive Security Controls (Verified)

- JWT "none" algorithm rejected — `internal/plugin/auth_jwt.go`
- Parameterized SQL throughout — all `internal/store/` files
- TLS 1.2+ minimum enforced — `internal/gateway/tls.go`
- RSA 2048-bit minimum key size — `internal/pkg/jwt/rs256.go`
- HMAC-SHA256 32-byte minimum secret — `internal/pkg/jwt/hs256.go`
- bcrypt cost 12 for passwords — `internal/store/user_repo.go:499`
- Constant-time admin key comparison — `crypto/subtle.ConstantTimeCompare`
- Secure/HttpOnly/SameSite cookies — admin token + portal session
- RBAC with role-based permissions — `internal/admin/rbac.go`
- Rate limiting on auth endpoints — admin API + portal login
- WASM sandboxing — `internal/plugin/wasm.go`
- Plugin signature verification — `internal/plugin/marketplace.go`
- CSRF double-submit on portal — `internal/portal/server.go`
- Security headers on all responses — X-Content-Type-Options, X-Frame-Options, CSP, Permissions-Policy
- Audit log PII masking — `internal/audit/masker.go`
- Webhook HMAC-SHA256 signing — `internal/admin/webhooks.go`
- API keys hashed with SHA-256 — only prefix stored in DB
- Session tokens via `crypto/rand` — 32-byte entropy
- Trusted proxy anti-spoofing — `internal/pkg/netutil/clientip.go`

*Generated: 2026-04-15*
