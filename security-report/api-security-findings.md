# API Security Findings — APICerebrus

**Scan Date:** 2026-04-18
**Scanner:** Claude Code Security Scan
**Scope:** Admin API, Gateway Proxy, GraphQL/Federation, MCP Server

---

## Executive Summary

APICerebrus implements defense-in-depth across its API surface. The most recent security audit (2026-04-16) verified remediation of 26 high-confidence findings. This report independently confirms the current state of security controls in the four focus areas and identifies any remaining gaps.

**Overall Assessment:** The codebase demonstrates strong security practices. Multiple layers of authentication (static key, Bearer JWT, CSRF tokens), RBAC, rate limiting, and query complexity controls are present. Residual findings are primarily around operation-level consistency and documentation.

---

## 1. Admin API (`internal/admin/`)

### 1.1 Authentication — STRONG

**Findings:**

| ID | Severity | Title | Status |
|----|----------|-------|--------|
| SEC-ADM-001 | Info | Static API key auth with constant-time comparison | ✅ Implemented |
| SEC-ADM-002 | Info | JWT Bearer tokens with key version rotation | ✅ Implemented |
| SEC-ADM-003 | Info | Rate limiting on failed auth attempts | ✅ Implemented |
| SEC-ADM-004 | Info | IP allow-list before auth check | ✅ Implemented |
| SEC-ADM-005 | Info | CSRF token validation (double-submit cookie) | ✅ Implemented |

**Details:**

- `token.go:247`: Static key compared with `subtle.ConstantTimeCompare` — timing-attack safe.
- `token.go:113-129`: JWT verification includes `key_version` check; key rotation invalidates all existing sessions.
- `server.go:68-83`: In-memory rate limiting with cleanup goroutine — `maxCreditOpsPerMinute = 30`.
- `token.go:172-175`: IP allow-list evaluated before authentication.
- `token.go:199-210`: CSRF validation for state-changing requests; skips login endpoints to avoid chicken-and-egg.

**Residual Risk:** Low. Auth layer is comprehensive.

### 1.2 Authorization (RBAC) — STRONG

**Findings:**

| ID | Severity | Title | Status |
|----|----------|-------|--------|
| SEC-ADM-006 | Info | Role-based permission mapping | ✅ Implemented |
| SEC-ADM-007 | Info | Default-deny for unmapped endpoints | ✅ Implemented |
| SEC-ADM-008 | Info | Path normalization for ID-based routes | ✅ Implemented |

**Details:**

- `rbac.go:59-85`: `RolePermissions` map defines 4 roles with granular permissions (21 permission constants).
- `rbac.go:306-312`: Unmapped endpoints return `403 permission_denied` — default deny.
- `rbac.go:226-240`: Path segments matching ID patterns (UUIDs, `wh_*`, `srv_*`, etc.) normalized to `{id}` for permission lookup.

**Residual Risk:** Low. RBAC is well-structured.

### 1.3 Rate Limiting — STRONG

**Findings:**

| ID | Severity | Title | Status |
|----|----------|-------|--------|
| SEC-ADM-009 | Info | Auth attempt rate limiting (per IP) | ✅ Implemented |
| SEC-ADM-010 | Info | Credit operation rate limiting | ✅ Implemented |

**Details:**

- `server.go:68-83`: `adminAuthAttempts` map tracks failed attempts per IP; blocked after threshold.
- `server.go:75-82`: `creditRateLimitEntry` tracks credit operations per key; 30 ops/minute limit.

### 1.4 Input Validation — STRONG

**Findings:**

| ID | Severity | Title | Status |
|----|----------|-------|--------|
| SEC-ADM-011 | Info | JSON payload size limits (1<<20 = 1MB) | ✅ Implemented |
| SEC-ADM-012 | Info | Service/Route/Upstream validation | ✅ Implemented |
| SEC-ADM-013 | Info | Config import sensitive-field stripping | ✅ Implemented |

**Details:**

- `admin_routes.go:21`: `jsonutil.ReadJSON(r, &in, 1<<20)` — 1MB limit.
- `server.go:442`: Config import uses `io.LimitReader(r.Body, 2<<20)` — 2MB limit.
- `server.go:453-458`: `stripSensitiveFields` blocks injection of credentials via config import.
- `server.go:470-488`: Temp file created with `0600` permissions; immediately unlinked after load.

### 1.5 Security Headers — STRONG

**Findings:**

| ID | Severity | Title | Status |
|----|----------|-------|--------|
| SEC-ADM-014 | Info | X-Content-Type-Options: nosniff | ✅ Implemented |
| SEC-ADM-015 | Info | X-Frame-Options: DENY | ✅ Implemented |
| SEC-ADM-016 | Info | Referrer-Policy: strict-origin-when-cross-origin | ✅ Implemented |

**Details:**

- `server.go:128-131`: `ServeHTTP` sets security headers on every response.

### 1.6 Open Finding — Admin GraphQL Handler Introspection Check Ordering

**Finding ID:** SEC-ADM-017
**Severity:** Low (Informational)
**Title:** Admin GraphQL introspection check occurs after parse

**Description:**

In `graphql.go:39-82`, the introspection check:

```go
// F-012: Block introspection queries when disabled (default).
h.server.mu.RLock()
introspectionEnabled := h.server.cfg.Admin.GraphQLIntrospection
h.server.mu.RUnlock()
if !introspectionEnabled && isIntrospectionQuery(req.Query) {
    // Returns error
    return
}
```

Occurs **after** `jsonutil.ReadJSON` parses the request body. The query string is parsed but not executed — no data leakage occurs. However, the check could be performed on the raw query string before JSON parsing to avoid any parsing-related side effects.

**Impact:** Theoretical. No actual vulnerability since query is not executed before the check.

**Remediation:** Move the introspection check to occur on the raw `req.Query` string before `jsonutil.ReadJSON` is called. The current implementation is acceptable for production use.

---

## 2. Gateway Proxy (`internal/gateway/`)

### 2.1 Request Validation — STRONG

**Findings:**

| ID | Severity | Title | Status |
|----|----------|-------|--------|
| SEC-GW-001 | Info | MaxBodyBytes enforcement with Content-Length fast path | ✅ Implemented |
| SEC-GW-002 | Info | SSRF protection via host validation | ✅ Implemented |
| SEC-GW-003 | Info | Trusted proxy + client IP extraction | ✅ Implemented |
| SEC-GW-004 | Info | Security headers on all responses | ✅ Implemented |

**Details:**

- `server.go:215-233`: Enforces `MaxBodyBytes` via Content-Length check (no buffering) and `io.LimitReader` for chunked bodies.
- `optimized_proxy.go:465-468`: `validateUpstreamHost(base.Host)` called before proxy — blocks private/loopback/link-local IPs.
- `server.go:235-236`: `addSecurityHeaders(w, g.config.Gateway.HTTPSAddr != "")` called for every request.
- `netutil/clientip.go`: "Secure by default" — when `trusted_proxies` is empty, `X-Forwarded-For` is ignored.

### 2.2 Health Endpoints — DOCUMENTED

**Finding ID:** SEC-GW-005
**Severity:** Info (Documented, not a vulnerability)
**Title:** /health and /ready bypass plugin pipeline

**Description:**

As noted in `server.go:978-981`:
```go
// M-004 NOTE: These endpoints bypass the plugin pipeline and cannot be rate-limited
// by the standard rate limiting plugins. They also skip authentication.
// Network-level protection (firewall, load balancer rate limiting) should be used
// in front of APICerebrus to protect these endpoints from DoS attacks.
```

The `/ready` endpoint also discloses internal state (DB connectivity, health checker status) only to IPs in `AllowedHealthIPs`.

**Impact:** Acceptable. Health endpoints are for internal load balancer probes; network-level protection is the correct approach.

### 2.3 Request Coalescing — SECURE

**Finding ID:** SEC-GW-006
**Severity:** Info
**Title:** Coalescing key includes all identity headers

**Description:**

`optimized_proxy.go:568-574`:
```go
var coalesceIdentityHeaders = []string{
    "Authorization",
    "Proxy-Authorization",
    "X-API-Key",
    "X-Admin-Key",
    "Cookie",
}
```

`SEC-PROXY-006` was a prior finding where only `Authorization` and `X-API-Key` were used, allowing cross-user response injection. This is now fixed — every identity-bearing header partitions coalescing keys.

**Status:** ✅ Fixed

### 2.4 Federation Batch Endpoint — SECURE

**Finding ID:** SEC-GW-007
**Severity:** Info
**Title:** Federation batch requires API key auth when consumers configured

**Description:**

`server.go:1140-1161`:
```go
// SEC-GQL-001: enforce API-key authentication when the gateway has any
// consumer configured. The batch endpoint is dispatched before the route
// pipeline runs, so without this guard an unauthenticated caller can
// amplify one HTTP request into up to maxBatchSize federated plans.
```

**Status:** ✅ Fixed

---

## 3. GraphQL & Federation (`internal/graphql/`, `internal/federation/`)

### 3.1 Query Complexity and Depth — STRONG

**Findings:**

| ID | Severity | Title | Status |
|----|----------|-------|--------|
| SEC-GQL-001 | Info | Query depth limits (default 15) | ✅ Implemented |
| SEC-GQL-002 | Info | Query complexity limits (default 1000) | ✅ Implemented |
| SEC-GQL-003 | Info | Field cost configuration | ✅ Implemented |

**Details:**

- `analyzer.go:37-42`: Defaults: `maxDepth = 15`, `maxComplexity = 1000`.
- `analyzer.go:136-144`: Field cost multiplied by argument count for complexity calculation.
- `analyzer.go:186-207`: `ValidateDepth` and `ValidateComplexity` methods available.

### 3.2 Introspection Control — STRONG

**Findings:**

| ID | Severity | Title | Status |
|----|----------|-------|--------|
| SEC-GQL-004 | Info | Introspection configurable (default disabled) | ✅ Implemented |
| SEC-GQL-005 | Info | Admin GraphQL blocks introspection when disabled | ✅ Implemented |
| SEC-GQL-006 | Info | Federation executor SSRF re-validation | ✅ Implemented |

**Details:**

- `config/types.go:193`: `GraphQLIntrospection bool` field.
- `graphql.go:59-71`: Blocks queries containing `__schema` or `__type` when disabled.
- `proxy.go:111-127`: `IntrospectionChecker` available for gateway-level introspection control.
- `federation/executor.go:440-444`: URL re-validation before every subgraph request (SEC-GQL-005).
- `federation/executor.go:691-696`: URL re-validation before batch execution.
- `federation/executor.go:787-793`: URL re-validation before WebSocket subscription dial.

### 3.3 Federation Authorization — STRONG

**Finding ID:** SEC-GQL-007
**Severity:** Info
**Title:** `@authorized` directive enforcement in executor

**Description:**

`federation/executor.go:405-415`:
```go
// SEC-GQL-006: enforce @authorized BEFORE issuing the subgraph request,
// so that a denied role doesn't cause the subgraph to leak the protected
// field via its own data path.
if checker := authCheckerFromContext(ctx); checker != nil {
    if err := enforceFieldAuth(step, checker); err != nil {
        return nil, err
    }
}
```

`WithAuthChecker(ctx, checker)` must be called by the caller to attach the checker to the context.

**Status:** ✅ Implemented — callers must wire the auth checker.

### 3.4 Open Finding — Federation Field Auth One-Level Scope

**Finding ID:** SEC-GQL-008
**Severity:** Low (Informational)
**Title:** `enforceFieldAuth` walks only one level of nesting

**Description:**

`executor.go:329-357`:
```go
// Scope note: this walks one level of nesting (the step's direct selection
// set on ResultType). It does NOT recurse into nested types — doing so
// needs full supergraph type info, which the Executor doesn't hold.
```

Deep nested `@authorized` fields (e.g., `User.address.street` where `address` is a nested type) are not covered by the one-level walk. The executor lacks full supergraph type info for recursive enforcement.

**Impact:** Low — typical `@authorized` usage is on direct fields of the step's return type.

**Remediation:** For Wave 3, add full supergraph type info to enable recursive auth enforcement.

---

## 4. MCP Server (`internal/mcp/`)

### 4.1 Authentication — STRONG

**Findings:**

| ID | Severity | Title | Status |
|----|----------|-------|--------|
| SEC-MCP-001 | Info | SSE transport requires X-Admin-Key | ✅ Implemented |
| SEC-MCP-002 | Info | Stdio transport is inherently local | ✅ Documented |
| SEC-MCP-003 | Info | Admin token exchange for in-process calls | ✅ Implemented |

**Details:**

- `server.go:254-260`: `POST /mcp` checks `X-Admin-Key` via `checkAdminKey()`.
- `server.go:271-280`: `GET /sse` also checks `X-Admin-Key` (SEC-GQL-011 fix).
- `server.go:255-256`: Comment explains stdio is local-only; SSE is network-accessible.
- `server.go:329-370`: `ensureAdminToken` exchanges admin key for session cookie for in-process admin API calls.

### 4.2 Tool Access Control — STRONG

**Findings:**

| ID | Severity | Title | Status |
|----|----------|-------|--------|
| SEC-MCP-004 | Info | Tools call through admin API (auth enforced there) | ✅ Implemented |
| SEC-MCP-005 | Info | ID path escaping in tool handlers | ✅ Implemented |
| SEC-MCP-006 | Info | Config import path argument removed | ✅ Implemented (SEC-GQL-010) |

**Details:**

- `call_tool.go`: All tool calls route through `callAdmin()` which uses Bearer token auth (from `ensureAdminToken`).
- `call_tool.go:23`: IDs escaped with `url.PathEscape(id)` — prevents path traversal.
- `config_import.go:26-29`: `path` argument explicitly rejected; only `yaml` or `config` accepted. Prevents arbitrary file read (SEC-GQL-010).

### 4.3 Security Observations — MCP

| ID | Severity | Title | Status |
|----|----------|-------|--------|
| SEC-MCP-007 | Info | Tool definitions do not expose permission requirements | Informational |
| SEC-MCP-008 | Info | No per-tool rate limiting | Informational |

**Details:**

- `tools_definitions.go:40-90`: Tool names and descriptions are defined, but the RBAC permission requirements are not declared in the tool metadata. The MCP server relies on the admin API to enforce permissions.
- No per-tool rate limiting exists in the MCP layer. However, rate limiting is enforced at the admin API level via `withAdminBearerAuth`.

---

## Summary of Findings

### Fixed Findings (from prior audit)

| Finding | Area | Status |
|---------|------|--------|
| Introspection enabled in production | GraphQL | ✅ Fixed |
| Executor SSRF (no URL re-validation) | Federation | ✅ Fixed |
| Request coalescing identity leak | Gateway | ✅ Fixed |
| Federation batch unauthenticated amplification | Gateway | ✅ Fixed |
| MCP SSE endpoint unauthenticated | MCP | ✅ Fixed |
| Config import arbitrary file read (path arg) | MCP | ✅ Fixed |
| CSRF token missing on admin API | Admin | ✅ Fixed |
| Admin key version not enforced | Admin | ✅ Fixed |

### New Findings (this scan)

| Finding | Severity | Area | Remediation |
|---------|----------|------|-------------|
| SEC-ADM-017: GraphQL introspection check ordering | Low | Admin | Move check to raw query string before parse |
| SEC-GQL-008: Federation field auth one-level scope | Low | Federation | Add full supergraph type info for recursion |

### Risk Assessment

**Overall Risk: LOW**

APICerebrus implements defense-in-depth across all four focus areas:
- **Admin API**: Multi-layer auth (static key, JWT, CSRF), RBAC with default-deny, rate limiting, input validation
- **Gateway**: SSRF protection, max body enforcement, trusted proxy logic, security headers, safe coalescing
- **GraphQL/Federation**: Query depth/complexity limits, configurable introspection, executor SSRF re-validation, `@authorized` enforcement
- **MCP**: Auth on all network endpoints, path escaping, removal of dangerous `path` argument

The two low-severity findings are architectural observations rather than exploitable vulnerabilities.

---

## Recommendations

1. **SEC-ADM-017 (Low):** Consider moving the introspection check to the raw query string before JSON parsing. Current implementation is acceptable.

2. **SEC-GQL-008 (Low):** For federation Wave 3, consider adding full supergraph type info to enable recursive `@authorized` enforcement.

3. **Documentation:** The `M-004` note in `server.go:978-981` correctly documents that health endpoints bypass the plugin pipeline. Ensure operators use network-level protection (firewall/LB rate limiting) for these endpoints.

4. **Monitoring:** Continue monitoring the security-report/ directory for any new findings from ongoing security work.

---

## Security Report Location

This report is written to: `security-report/api-security-findings.md`

Related reports in `security-report/`:
- `verified-findings.md` — Prior audit findings with fix verification
- `findings-auth.md` — Authentication and authorization findings
- `findings-injection.md` — Injection-related findings
- `sc-federation-mcp-results.md` — Security scan results for Federation/MCP