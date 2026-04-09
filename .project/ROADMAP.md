# APICerebrus Production Readiness Roadmap

> Generated: 2026-04-08  
> Priority: P0 = Blocker, P1 = High, P2 = Medium, P3 = Low  
> Milestones: Harden → Stabilise → Scale → Polish

---

## Milestone 1: Security Hardening (P0)

**Goal**: Remove the most dangerous production blockers.

### 1.1 Fix Admin Key Storage (P0) ✅ DONE
- **Status**: Admin login now uses native HTML form POST (`<form action="/admin/login" method="POST">`).
  The key goes directly from browser to server without entering JavaScript memory.
  Server validates against static API key and sets an HttpOnly, SameSite=Strict session cookie.
  Legacy `exchangeAdminKeyForToken()` retained for programmatic/SSO flows only.
- **Files**: `web/src/pages/admin/Login.tsx`, `internal/admin/token.go`, `internal/admin/server.go`, `web/src/lib/api.ts`

### 1.2 Harden Example Configuration (P0) ✅ DONE
- **Status**: `apicerberus.example.yaml` uses empty strings for all secrets.
  Config validation in `validate()` enforces:
  - `admin.api_key` required, rejects placeholder values (change/secret/password/123)
  - `admin.token_secret` min 32 chars
  - `portal.session.secret` min 32 chars, rejects placeholder values
  - `portal.session.secure` must be true when HTTPS is configured
  - `cors.allowed_origins` uses `[]` not `["*"]`
- **Files**: `apicerberus.example.yaml`, `internal/config/load.go`

### 1.3 Validate X-Forwarded-For (P0) ✅ DONE
- **Status**: Already implemented. `ExtractClientIP` (`netutil/clientip.go`):
  - **Secure by default**: When `gateway.trusted_proxies` is empty, `X-Forwarded-For` and `X-Real-IP` are ignored — `RemoteAddr` is used
  - **Right-to-left parsing**: Walks XFF from right to left, skipping trusted proxies, returning the rightmost untrusted IP
  - **CIDR support**: Trusted proxies support individual IPs and CIDR ranges
  - **Anti-spoofing**: Only trusts forwarding headers if the immediate connection IP is a trusted proxy
- **Files**: `internal/pkg/netutil/clientip.go`, `internal/config/load.go` (TrustedProxies config), `internal/cli/run.go` (SetTrustedProxies on start)

### 1.4 WebSocket Origin Security (P1) ✅ DONE
- **Status**: Strengthened `isValidWebSocketOrigin` — removed Referer fallback, enforced http/https schemes, proper port matching, host boundary checking for wildcards, 25 unit tests added.
- **Files**: `internal/admin/ws.go`, `internal/admin/ws_origin_test.go`

### 1.5 Add TLS Minimum Version / Cipher Suite Config (P1) ✅ DONE
- **Status**: Already implemented. `TLSConfig` struct has `MinVersion` and `CipherSuites` fields.
  `TLSManager.TLSConfig()` parses them and builds `*tls.Config` with proper defaults (TLS 1.2 min, modern ciphers).
- **Files**: `internal/gateway/tls.go` (`parseTLSMinVersion`, `parseTLSCipherSuites`, `TLSConfig()` method)

---

## Milestone 2: Reliability & Resource Safety (P0–P1)

### 2.1 Enforce Request Body Limits (P0) ✅ DONE
- **Status**: Already implemented with hard limits, not advisory.
  - **Incoming requests**: Content-Length checked first (fast path → 413). Chunked bodies buffered with `io.LimitReader(limit+1)`, rejected if over.
  - **Coalesced responses**: Content-Length pre-check, bounded `io.ReadAll(io.LimitReader(maxBody+1))`, `CompleteTooLarge` for over-limit.
  - **Non-coalesced responses**: Streamed with bounded `io.CopyBuffer` — no memory accumulation.
  - **Audit capture**: Response body capture bounded by `maxBodyBytes` with truncation.
- **Files**: `internal/gateway/server.go:209-231`, `internal/gateway/optimized_proxy.go:366-391`, `internal/audit/capture.go:63-75`

### 2.2 Cap Analytics Latency Buffer (P0) ✅ DONE
- **Status**: Reservoir sampling (Vitter's Algorithm R) with `maxLatencySamples = 10_000` per bucket is already implemented in `engine.go` lines 288-295.
- **Verification**: `defaultBucketRetention = 24h` caps total buckets. Worst case: 1,440 × 10,000 × 8 bytes ≈ 115 MB.
- **Files**: `internal/analytics/engine.go`

### 2.3 Fix Request Coalescing Memory Risk (P1) ✅ DONE
- **Status**: Already bounded. `CoalescingMaxBodyBytes` (default 1MB) caps buffered responses.
  Responses over the limit trigger `CompleteTooLarge`, causing waiters to retry independently.
  Content-Length pre-check avoids buffering entirely for known-large responses.
- **Files**: `internal/gateway/optimized_proxy.go`

### 2.4 Reuse Webhook HTTP Client (P1) ✅ DONE
- **Status**: WebhookManager already used a shared client. Added tuned `http.Transport` with connection pooling (MaxIdleConns=100, HTTP/2, 90s idle timeout).
- **Bonus**: Fixed `HTTPClientPool.GetStats()` returning zeroed values instead of actual stats.
- **Files**: `internal/admin/webhooks.go`, `internal/gateway/connection_pool.go`

### 2.5 Add Per-Request Upstream Timeout (P1) ✅ DONE
- **Status**: Added `UpstreamTimeout` to `RequestContext`. Wired from `service.ReadTimeout` (default 30s) into both `proxy.Forward` and `proxy.Do`.
- **Files**: `internal/gateway/proxy.go`, `internal/gateway/server.go`

### 2.6 Raft Transport Encryption (P1) ✅ DONE
- **Status**: mTLS implemented with auto CA generation, node cert signing, and TLS certificate manager.
- **Files**: `internal/raft/tls.go`

### 2.7 Async Log Hook (P2) ✅ DONE
- **Status**: `AsyncLogHook` wraps synchronous `LogHook` with buffered channel + background goroutine.
  Entries are drained asynchronously; buffer-full drops silently to avoid blocking the caller.
  `Close()` drains remaining entries with non-blocking channel read and 5s timeout.
- **Files**: `internal/logging/structured.go`, `internal/logging/structured_test.go`

---

## Milestone 3: Identity & Auth Unification (P1)

### 3.1 Unify Users and Consumers (P1) ✅ DONE
- **Task**: Bridge the gap between `store.User` (portal/admin) and `config.Consumer` (gateway).
- **Status**: Fixed gateway-level auth wiring to use SQLite-backed API key lookup instead of only
  YAML-defined consumers. Previously `newAuthAPIKey` received `nil` as the lookup function in both
  `New()` and `Reload()`, so store-based keys only worked in route-level plugin pipelines.
  Now both paths query SQLite first, falling back to YAML consumers for backwards compatibility.
- **`userToConsumer()` enhancements**:
  - Populates `Consumer.RateLimit` from `user.RateLimits` JSON map (`requests_per_second`, `burst`)
  - Extracts `ACLGroups` from `user.Metadata["acl_groups"]` with type-safe conversion
  - Carries `credit_balance` into metadata for billing-aware plugins
- **Data flow**: `AuthAPIKey.Authenticate` → `APIKeyLookup` (SQLite) → `ResolveUserByRawKey` →
  JOIN `api_keys` to `users` → `userToConsumer` → `*config.Consumer`
- **Files**: `internal/gateway/server.go` (`New`, `Reload`, `userToConsumer`)

### 3.2 Add Rate-Limiting to Failed Auth (P2) ✅ DONE
- **Status**: Implemented `AuthBackoff` — per-IP exponential backoff (100ms → 30s max) for invalid API key attempts.
  Integrated into `AuthAPIKey` via options. Only triggers on `invalid_api_key` errors.
- **Files**: `internal/plugin/auth_backoff.go`, `internal/plugin/auth_apikey.go`, `internal/gateway/server.go`

### 3.3 JWT Enhancements (P2) ✅ DONE
- **Task**: Add `nbf` validation, `jti` tracking (optional Redis-backed replay cache), and ES256/EdDSA support.
- **Status**:
  - **`nbf` validation**: Validates Not Before claim with configurable clock skew tolerance.
    Rejects tokens used before their `nbf` time (`invalid_jwt_claims` error).
  - **`jti` replay cache**: Optional in-memory replay cache (`JTIReplayCache`) that tracks
    JWT IDs with per-entry TTLs (based on token expiry). Enabled via `enable_jti_replay: true`
    in plugin config. Automatic cleanup of expired entries every 5 minutes.
  - **ES256**: ECDSA P-256 signature verification via `internal/pkg/jwt/es256.go`.
    Supports both direct `ECDSAPublicKey` config and JWKS key resolution (EC keys with P-256 curve).
  - **EdDSA**: Ed25519 signature verification via `internal/pkg/jwt/es256.go`.
    Requires `EdDSAPublicKey` to be configured (no JWKS support yet).
  - **JWKS extended**: `JWKSClient` now parses both RSA and EC keys from JWKS documents.
    `GetECDSAKey()` resolves P-256 EC keys by `kid`.
- **Files**: `internal/plugin/auth_jwt.go`, `internal/plugin/jti_replay.go`,
  `internal/plugin/registry.go`, `internal/pkg/jwt/es256.go`, `internal/pkg/jwt/jwks.go`,
  `internal/pkg/jwt/rs256.go`

---

## Milestone 4: Operational Excellence (P1–P2)

### 4.1 Fix MCP Cluster Mock Data (P1) ✅ DONE
- **Status**: Already implemented. `cluster.status` and `cluster.nodes` query the actual Raft node state
  (`GetState()`, `GetTerm()`, `GetLeaderID()`, `CommitIndex`, `LastApplied`, `Peers`).
- **Files**: `internal/mcp/server.go`, `internal/raft/node.go`

### 4.2 Real GeoIP or Rename Feature (P2) ✅ DONE
- **Status**: Renamed to `subnet_aware`. `geo_aware` kept as deprecated alias for backward compatibility.
- **Files**: `internal/loadbalancer/geo.go`, `internal/gateway/balancer.go`, `internal/gateway/balancer_extra.go`

### 4.3 Graceful Shutdown Hooks (P2) ✅ DONE
- **Status**: `Gateway.Shutdown` now waits for the audit goroutine to finish draining
  and flushing its buffer (`auditDone` channel). Tracer flush was already wired
  (`tracer.Shutdown(ctx)`). Analytics is in-memory with synchronous writes — no
  flush needed (data is lost on process exit regardless).
- **Files**: `internal/gateway/server.go`, `internal/analytics/engine.go`

### 4.6 Fix Reload Panic (P1) ✅ DONE
- **Status**: `Gateway.Reload` was panicking with `close of closed channel` when
  restarting the audit goroutine. The fix cancels the old audit context, releases
  the mutex, waits for the old goroutine to signal completion via `auditDone`
  channel (10s timeout), then re-acquires the lock and creates the replacement.
- **Files**: `internal/gateway/server.go` (Reload function)

### 4.4 SQLite Backup with Locking (P2) ✅ DONE
- **Status**: Script already uses `sqlite3 ".backup"` (SQLite backup API).
  Added `.timeout 5000` for BUSY handling, `VACUUM INTO` fallback,
  and `PRAGMA integrity_check` verification after backup creation.
- **Files**: `scripts/backup.sh`

### 4.5 Add Frontend CSP + CSRF (P2) ✅ DONE
- **Status**: CSP header already set via `<meta http-equiv>` in `web/index.html`.
  Added server-side `Content-Security-Policy` headers to both admin (`ui.go`) and
  portal (`ui.go`) HTML responses with strict policies (no `unsafe-eval`, no
  `object-src`, `form-action 'self'`, `frame-ancestors 'none'`).
  CSRF double-submit cookie already implemented in portal (`withCSRF` middleware).
- **Files**: `web/index.html`, `internal/admin/ui.go`, `internal/portal/ui.go`

---

## Milestone 5: Frontend Quality (P2–P3)

### 5.1 Re-enable TypeScript Checks (P2) ✅ DONE
- **Status**: `tsc --noEmit` passes cleanly. `lint` and `typecheck` scripts both run real TypeScript checks.

### 5.2 Complete Placeholder Pages (P3) ✅ DONE
- **Status**: All admin pages are fully implemented. System Logs has live WebSocket
  tail streaming, filtering (level/source/search), JSON export, auto-scroll, and
  metadata display. The `PlaceholderPage` in `App.tsx` serves as a fallback for
  any future nav items that don't have dedicated routes yet.
- **Files**: `web/src/pages/admin/SystemLogs.tsx`, `web/src/components/logs/LogTail.tsx`

### 5.3 Add E2E Smoke Tests (P3) ✅ DONE
- **Status**: 15 Go E2E test files (~4,355 lines) covering:
  - Gateway routing, auth, billing, rate limiting, proxy forwarding
  - CLI commands (user, credit, audit, analytics, gateway entities)
  - MCP server (stdio transport)
  - Full request lifecycle with plugin pipeline
  - Benchmarks and performance tests
- **Files**: `test/e2e_v*_test.go`, `test/e2e_v010_cli_smoke_test.go`, `test/e2e_v010_mcp_stdio_test.go`, `test/e2e_v003_bench_test.go`

---

## Milestone 6: Documentation Cleanup (P2)

### 6.1 Correct Dependency Claims (P2) ✅ DONE
- **Status**: Updated `IMPLEMENTATION.md` with accurate external dependency table (9 direct deps documented).

### 6.2 Task List Integrity (P2) ✅ DONE
- **Status**: `.project/TASKS.md` has 0 unchecked items. All completed features are properly tracked.

### 6.3 Fix Go Version (P2) ✅ DONE
- **Status**: Verified. `go 1.25.0` is valid for Go 1.26.x installations.

---

## Suggested Execution Order

| Week | Focus |
|------|-------|
| 1 | P0 security (admin key, secrets, XFF), P0 body limit, analytics buffer cap |
| 2 | P0–P1 reliability (webhook client, proxy timeout, coalescing memory) |
| 3 | P1 auth unification (gateway keys in DB), P1 MCP cluster fix |
| 4 | P2 frontend CSP/CSRF, TS fixes, operational hooks, docs cleanup |
| 5+ | Raft mTLS, E2E tests, GeoIP decision, long-tail polish |

---

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| XSS steals admin key | Medium | Critical | ~~Move key to HttpOnly cookie (Week 1)~~ ✅ **Done** |
| OOM from analytics | High | High | ~~Cap latency samples (Week 1)~~ ✅ **Done** |
| Rate-limit bypass via XFF | High | Medium | ~~Trusted proxy parsing (Week 1)~~ ✅ **Done** |
| Coalescing memory spike | Medium | High | ~~Disable or bound coalescing (Week 2)~~ ✅ **Done** |
| Request body over-limit | Medium | High | ~~Hard limit enforcement~~ ✅ **Done** |
| Raft plaintext traffic | Low | High | ~~mTLS on Raft RPC (Week 2–3)~~ ✅ **Done** |
| Placeholder pages disappoint users | High | Low | Hide or implement before GA (Week 4–5) — only System Logs remains |
