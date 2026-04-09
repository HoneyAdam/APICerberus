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

### 1.2 Harden Example Configuration (P0)
- **Task**: Remove all placeholder secrets from `apicerberus.example.yaml`.
- **Changes**:
  - `admin.api_key`: leave empty and fail validation if unset.
  - `portal.session.secret`: enforce minimum entropy (e.g. 32 bytes base64) in `config/validate.go`.
  - `portal.session.secure`: default to `true`; reject `false` when `addr` uses HTTPS.
  - Remove `cors: allowed_origins: ["*"]` from the example.
- **Files**: `apicerberus.example.yaml`, `internal/config/load.go`

### 1.3 Validate X-Forwarded-For (P0)
- **Task**: Do not trust the first `X-Forwarded-For` entry blindly.
- **Approach**:
  - Add `gateway.trusted_proxies` / `gateway.trusted_cidrs` config list.
  - Parse `X-Forwarded-For` from right-to-left, stopping at the first untrusted IP.
  - Fallback to `X-Real-Ip` only if the source is a trusted proxy.
- **Files**: `internal/logging/structured.go`, `internal/gateway/balancer_extra.go`, `internal/gateway/server.go`, `internal/config/`

### 1.4 WebSocket Origin Security (P1) ✅ DONE
- **Status**: Strengthened `isValidWebSocketOrigin` — removed Referer fallback, enforced http/https schemes, proper port matching, host boundary checking for wildcards, 25 unit tests added.
- **Files**: `internal/admin/ws.go`, `internal/admin/ws_origin_test.go`

### 1.5 Add TLS Minimum Version / Cipher Suite Config (P1) ✅ DONE
- **Status**: Already implemented. `TLSConfig` struct has `MinVersion` and `CipherSuites` fields.
  `TLSManager.TLSConfig()` parses them and builds `*tls.Config` with proper defaults (TLS 1.2 min, modern ciphers).
- **Files**: `internal/gateway/tls.go` (`parseTLSMinVersion`, `parseTLSCipherSuites`, `TLSConfig()` method)

---

## Milestone 2: Reliability & Resource Safety (P0–P1)

### 2.1 Enforce Request Body Limits (P0)
- **Task**: Make `MaxBodyBytes` a hard limit, not an advisory `io.LimitReader`.
- **Approach**:
  - Read up to `MaxBodyBytes+1` into a buffer.
  - If the `+1` byte is read, return `413 Payload Too Large` immediately.
- **Files**: `internal/gateway/server.go`

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

---

## Milestone 3: Identity & Auth Unification (P1)

### 3.1 Unify Users and Consumers (P1)
- **Task**: Bridge the gap between `store.User` (portal/admin) and `config.Consumer` (gateway).
- **Approach**:
  - Store API keys in the `api_keys` table with a `scope` column (`portal` | `gateway`).
  - Allow gateway auth to query SQLite for keys, falling back to in-memory config for backwards compatibility.
  - Deprecate YAML-only consumer keys over two minor releases.
- **Files**: `internal/plugin/auth_apikey.go`, `internal/store/api_key_repo.go`, `internal/config/`

### 3.2 Add Rate-Limiting to Failed Auth (P2) ✅ DONE
- **Status**: Implemented `AuthBackoff` — per-IP exponential backoff (100ms → 30s max) for invalid API key attempts.
  Integrated into `AuthAPIKey` via options. Only triggers on `invalid_api_key` errors.
- **Files**: `internal/plugin/auth_backoff.go`, `internal/plugin/auth_apikey.go`, `internal/gateway/server.go`

### 3.3 JWT Enhancements (P2)
- **Task**: Add `nbf` validation, `jti` tracking (optional Redis-backed replay cache), and ES256/EdDSA support.
- **Files**: `internal/plugin/auth_jwt.go`, `internal/pkg/jwt/`

---

## Milestone 4: Operational Excellence (P1–P2)

### 4.1 Fix MCP Cluster Mock Data (P1) ✅ DONE
- **Status**: Already implemented. `cluster.status` and `cluster.nodes` query the actual Raft node state
  (`GetState()`, `GetTerm()`, `GetLeaderID()`, `CommitIndex`, `LastApplied`, `Peers`).
- **Files**: `internal/mcp/server.go`, `internal/raft/node.go`

### 4.2 Real GeoIP or Rename Feature (P2) ✅ DONE
- **Status**: Renamed to `subnet_aware`. `geo_aware` kept as deprecated alias for backward compatibility.
- **Files**: `internal/loadbalancer/geo.go`, `internal/gateway/balancer.go`, `internal/gateway/balancer_extra.go`

### 4.3 Graceful Shutdown Hooks (P2)
- **Task**: Ensure `gateway.Gateway.Shutdown` calls:
  - tracer `Shutdown` flush,
  - audit log buffer flush,
  - analytics snapshot persistence (if desired).
- **Files**: `internal/gateway/server.go`, `internal/tracing/tracing.go`, `internal/audit/`

### 4.4 SQLite Backup with Locking (P2)
- **Task**: Use SQLite `backup` API or `VACUUM INTO` in the backup script, taking a `BUSY` lock to guarantee consistency.
- **Files**: `scripts/backup.sh`

### 4.5 Add Frontend CSP + CSRF (P2)
- **Task**:
  - Inject a strict `Content-Security-Policy` in the Vite build (`index.html`).
  - Add CSRF double-submit cookie for portal mutations.
- **Files**: `web/index.html`, `web/src/lib/api.ts`, `internal/portal/server.go`

---

## Milestone 5: Frontend Quality (P2–P3)

### 5.1 Re-enable TypeScript Checks (P2)
- **Task**: Fix all TypeScript errors and restore real `lint` / `typecheck` scripts.
- **Files**: `web/package.json`, all `.ts`/`.tsx` files with errors.

### 5.2 Complete Placeholder Pages (P3)
- **Task**: Implement or hide unfinished admin screens.
- **Files**: `web/src/App.tsx`, relevant page components.

### 5.3 Add E2E Smoke Tests (P3)
- **Task**: Add 3–5 Playwright tests covering admin login, service creation, route proxy, and portal login.
- **Files**: `web/e2e/` (new)

---

## Milestone 6: Documentation Cleanup (P2)

### 6.1 Correct Dependency Claims (P2)
- **Task**: Update `.project/IMPLEMENTATION.md` to accurately describe external dependencies.
- **Files**: `.project/IMPLEMENTATION.md`

### 6.2 Task List Integrity (P2)
- **Task**: Audit `.project/TASKS.md` and uncheck items that are placeholder-level or incomplete.
- **Files**: `.project/TASKS.md`

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
| XSS steals admin key | Medium | Critical | Move key to HttpOnly cookie (Week 1) |
| OOM from analytics | High | High | Cap latency samples (Week 1) |
| Rate-limit bypass via XFF | High | Medium | Trusted proxy parsing (Week 1) |
| Coalescing memory spike | Medium | High | Disable or bound coalescing (Week 2) |
| Raft plaintext traffic | Low | High | mTLS on Raft RPC (Week 2–3) |
| Placeholder pages disappoint users | High | Low | Hide or implement before GA (Week 4–5) |
