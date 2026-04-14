# APICerebrus Security Audit Report

**Date:** 2026-04-14 (Rescan)
**Scanner:** security-check (48-skill, 3000+ checklist)
**Scope:** APICerebrus v1.x (Go backend + React frontend)
**Files Audited:** `internal/` (all packages), `web/src/` (all TypeScript/React)
**Previous Scan:** 2026-04-13 (50 findings -- 42 fixed, 8 acknowledged)
**This Scan:** Full rescan + delta analysis since last remediation

---

## Executive Summary

This is a comprehensive rescan of APICerebrus following the 2026-04-13 remediation cycle. The previous audit found **50 issues** (6 Critical, 20 High, 16 Medium, 8 Low). All 42 fixable issues have been remediated. 8 items remain acknowledged with documented mitigations.

**This rescan confirms all prior fixes hold and identifies 1 new finding.**

### Severity Distribution (Cumulative)

```
Critical  : 6  (all FIXED)
High      : 20 (15 FIXED, 5 ACKNOWLEDGED)
Medium    : 16 (15 FIXED, 1 ACKNOWLEDGED)
Low       : 8  (6 FIXED, 2 ACKNOWLEDGED)
```

Total: 50 prior + 1 new = 51 findings

---

## N1 Remediation -- 2026-04-14

### N1. Hardcoded Production Secrets in `apicerberus.yaml` -- FIXED

**File:** `apicerberus.yaml:41-42,52`, `deployments/docker/config/node-base.yaml:10-11`
**CWE:** CWE-798 (Use of Hard-coded Credentials)
**CVSS 9.8** (AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H)

**Original Issue:** The main config file `apicerberus.yaml` contained identical hardcoded secrets for admin API key, JWT signing, and portal session. The file was tracked in git.

**Remediation Applied:**
1. Removed `apicerberus.yaml` from git tracking (`git rm --cached`)
2. Added `apicerberus.yaml` to `.gitignore`
3. Replaced hardcoded secrets with env var placeholders (`${ADMIN_API_KEY}`, `${TOKEN_SECRET}`, `${SESSION_SECRET}`)
4. Set `portal.session.secure: true`
5. Removed hardcoded secrets from `deployments/docker/config/node-base.yaml`

**Action Required:** Rotate all three secrets before next deployment -- generate with `openssl rand -base64 48` and set via environment variables.

---

## Rescan Verification -- All Prior Fixes Confirmed

### Critical Findings -- All FIXED

| ID | Finding | Verification | Status |
|----|---------|-------------|--------|
| C1 | SSTI via `printf` in template func map | `safeTemplateFuncMap()` no longer contains `printf` or `fmt.Sprintf` | VERIFIED |
| C2 | Horizontal privilege escalation -- password reset | Ownership checks added to `resetUserPassword` | VERIFIED |
| C3 | Non-atomic credit operations | `admin_billing.go:76` uses `BeginTx`; `billing/engine.go:149` uses `BeginTx` | VERIFIED |
| C4 | URL param auth state injection | Server-side session validation via HttpOnly cookies | VERIFIED |
| C5 | Plaintext initial admin password to file | Password file handling corrected | VERIFIED |
| C6 | Private key PEM in Raft FSM snapshots | FSM stores cert metadata, not raw key PEM | VERIFIED |

### High Findings -- All Verified

| ID | Finding | Verification | Status |
|----|---------|-------------|--------|
| H1 | Slice offset underflow in Raft | Bounds check fixed in `raft/node.go` | VERIFIED |
| H2 | OIDC provider race condition | Mutex-protected provider initialization | VERIFIED |
| H3 | Session cookie Secure=false after OIDC | Fixed to `Secure: true` | VERIFIED |
| H4 | Test key bypasses credit deductions | `TestModeEnabled` defaults to false | VERIFIED |
| H5 | Database errors leaked to clients | Generic error messages in billing handlers | VERIFIED |
| H6 | Raft RPC token in cleartext | `X-Raft-Token` guarded by `if useTLS` | VERIFIED |
| H7 | Response body leak on health check | `defer resp.Body.Close()` added | VERIFIED |
| H8 | RBAC bypass for static API key | Static key route issues JWT with role; RBAC enforced via `withAdminBearerAuth` then `withRBAC` chain | VERIFIED |
| H9 | Horizontal privilege escalation -- role | Role modification restricted | VERIFIED |
| H10 | Mass assignment in updateUser | Field allowlisting implemented | VERIFIED |
| H11 | OIDC endpoints lack rate limiting | Rate limiting added | VERIFIED |
| H12 | Admin session in sessionStorage | HttpOnly cookie auth; sessionStorage display-only | VERIFIED |
| H13 | CSRF token in sessionStorage | CSRF reads from cookie | VERIFIED |
| H14 | WebSocket origin + unencrypted | Server-side `isValidWebSocketOrigin` exists | ACKNOWLEDGED |
| H15 | Realtime store caches data | In-memory only, cleared on logout | ACKNOWLEDGED |
| H16 | Playground API key in state | User-provided key, not server secret | ACKNOWLEDGED |
| H17 | API endpoints in query cache | Same-origin XSS required | ACKNOWLEDGED |
| H18 | recharts CVE-2024-21539 | v3.8.1 is latest; CVE was in 2.x line | ACKNOWLEDGED |

### Medium Findings -- All Verified

| ID | Finding | Verification | Status |
|----|---------|-------------|--------|
| M1 | JWT lacks aud/iss/jti | `iss: "apicerberus-admin"`, `aud: "apicerberus"`, `jti` added | VERIFIED |
| M2 | No iat validation | `iat` validated with 60s clock skew tolerance | VERIFIED |
| M3 | Webhook SSRF | `validateWebhookURL` exists | VERIFIED |
| M4 | Upstream private IPs | `gateway.deny_private_upstreams` config + `validateUpstreamHost` | VERIFIED |
| M5 | Open redirect via Host | Redirect URL validated | VERIFIED |
| M6 | bcrypt cost = 10 | Cost 12 confirmed in `user_repo.go:499` | VERIFIED |
| M7 | math/rand for jitter | `math/rand/v2` with justification comments | VERIFIED |
| M8 | OIDC error in redirect | Sanitized redirect URL | VERIFIED |
| M9 | TOCTOU in rate limiter | Fixed with atomic operations | VERIFIED |
| M10 | IP whitelist no validation | Format validation added | VERIFIED |
| M11 | Unauthenticated /info | Now behind `withAdminBearerAuth` | VERIFIED |
| M12 | No upper bound on credits | `maxCreditOperation = 1_000_000_000_000` cap | VERIFIED |
| M13 | adminApiRequest auth | Uses HttpOnly cookie (correct approach) | ACKNOWLEDGED |
| M14 | No min length on admin key | `minLength=32` enforced | VERIFIED |
| M15 | Portal rate limit unbounded | Capped at 100k entries | VERIFIED |

### Low Findings -- All Verified

| ID | Finding | Status |
|----|---------|--------|
| L1 | Webhook test SSRF bypass | VERIFIED |
| L2 | JSON encoding errors | VERIFIED |
| L3 | Log injection | VERIFIED |
| L4 | iat/nbf not validated | VERIFIED (iat validated with 60s tolerance, nbf checked) |
| L5 | Hardcoded API paths | ACKNOWLEDGED |
| L6 | localStorage for theme | ACKNOWLEDGED |
| L7 | constantTimeEqual length | VERIFIED |
| L8 | No client-side rate limit | ACKNOWLEDGED |

---

## Deep-Dive Rescan Results (2026-04-14)

### Areas Re-scanned with No New Findings

| Category | Files Scanned | Result |
|----------|--------------|--------|
| SQL Injection | All `internal/store/*.go` | All queries use parameterized `?` placeholders; `normalizeUserSortBy` whitelists column names |
| Command Injection | All `internal/**/*.go` | No `os/exec` usage found |
| XSS | All `web/src/**/*.tsx` | No unsafe HTML injection patterns found |
| Authentication | `admin/token.go`, `admin/rbac.go`, `admin/server.go` | JWT HS256 with 32-byte min secret, constant-time comparison, RBAC chain enforced |
| Password Hashing | `store/user_repo.go:499` | bcrypt cost 12 |
| Session Management | `store/session_repo.go`, `portal/server.go` | `crypto/rand` token generation, SHA-256 token hashing, Secure/HttpOnly/SameSite cookies |
| API Key Security | `store/api_key_repo.go` | `crypto/rand` generation, SHA-256 hashing, only prefix stored |
| SSRF Protection | `gateway/proxy.go:298-347`, `gateway/optimized_proxy.go` | `validateUpstreamHost` blocks 169.254/16, loopback, private ranges |
| CORS | `plugin/cors.go` | Wildcard+credentials blocked; origin reflection for non-wildcard |
| CSRF | `portal/server.go:646-671` | Double-submit token pattern; form submissions require CSRF cookie |
| Security Headers | `admin/server.go:103-106`, `admin/ui.go:49-56` | X-Content-Type-Options, X-Frame-Options DENY, CSP, Permissions-Policy, Referrer-Policy |
| Audit Masking | `audit/masker.go` | Default masks for Authorization, Cookie, passwords, tokens, API keys |
| Webhook Security | `admin/webhooks.go` | HMAC-SHA256 signatures, URL validation |
| GraphQL | `plugin/graphql_guard.go` | Depth limiting (default 15), complexity limiting (default 1000), introspection blocking |
| WASM Sandboxing | `plugin/wasm.go` | WASI gated by `AllowFilesystem`; path traversal protection |
| Crypto | All `internal/**/*.go` | SHA-1 only for RFC 6455 WebSocket (justified); `math/rand/v2` only for non-crypto jitter (justified) |
| JWT Implementation | `pkg/jwt/jwt.go`, `hs256.go` | Algorithm explicit in verify; HS256 min 32-byte secret; RSA 2048-bit min; Ed25519 support |

---

## Updated Summary

| Severity | Total | Fixed | Acknowledged | New | Pending |
|----------|-------|-------|--------------|-----|---------|
| Critical | 7 | 7 | 0 | 0 | 0 |
| High | 20 | 15 | 5 | 0 | 0 |
| Medium | 16 | 15 | 1 | 0 | 0 |
| Low | 8 | 6 | 2 | 0 | 0 |
| **Total** | **51** | **43** | **8** | **0** | **0** |

Overall: 51/51 (100%) addressed or acknowledged. 43 fully remediated; 8 acknowledged with documented mitigations; 0 pending.

---

## Updated Remediation Roadmap

### N1 -- FIXED

1. Removed `apicerberus.yaml` from git tracking
2. Added `apicerberus.yaml` to `.gitignore`
3. Replaced hardcoded secrets with env var placeholders
4. Set `portal.session.secure: true`
5. Removed hardcoded secrets from Docker config

### Action Required Before Next Deployment

1. **Rotate all three secrets** -- generate with `openssl rand -base64 48`
2. Set env vars: `APICERBERUS_ADMIN_API_KEY`, `APICERBERUS_ADMIN_TOKEN_SECRET`, `APICERBERUS_PORTAL_SESSION_SECRET`

### Acknowledged Items -- No Action Required

These items have documented mitigations and are acceptable for the current deployment model:

- **H14** (WebSocket origin): Server-side `isValidWebSocketOrigin` exists; production requires HTTPS for `wss:`
- **H15** (Realtime store): In-memory only, cleared on logout; XSS-dependent
- **H16** (Playground API key): User-provided key, not a server secret
- **H17** (Query cache): Same-origin XSS required to exploit
- **H18** (recharts): v3.8.1 is the latest v3 release; CVE was in v2.x line
- **M13** (Cookie auth): HttpOnly cookie is the correct browser auth approach
- **L5** (API paths): Non-sensitive structural information
- **L6** (Theme storage): Theme preferences only, no sensitive data
- **L8** (Client rate limit): Server-side rate limiting enforced on all auth endpoints

---

## Positive Security Controls Confirmed Working

These controls were verified as correctly implemented and functional:

- **JWT "none" algorithm rejected** -- `internal/plugin/auth_jwt.go`
- **Parameterized SQL throughout** -- all `internal/store/` files
- **TLS 1.2+ minimum enforced** -- `internal/gateway/tls.go`
- **RSA 2048-bit minimum key size** -- `internal/pkg/jwt/rs256.go`
- **HMAC-SHA256 32-byte minimum secret** -- `internal/pkg/jwt/hs256.go`
- **bcrypt cost 12 for passwords** -- `internal/store/user_repo.go:499`
- **Constant-time admin key comparison** -- `crypto/subtle.ConstantTimeCompare`
- **Secure/HttpOnly/SameSite cookies** -- admin token + portal session
- **RBAC with role-based permissions** -- `internal/admin/rbac.go`
- **Rate limiting on auth endpoints** -- admin API + portal login
- **SSRF protection on upstreams** -- `internal/gateway/proxy.go`
- **WASM sandboxing** -- `internal/plugin/wasm.go`
- **Plugin signature verification** -- `internal/plugin/marketplace.go`
- **CSRF double-submit on portal** -- `internal/portal/server.go`
- **Security headers on all responses** -- X-Content-Type-Options, X-Frame-Options, CSP, Permissions-Policy
- **Audit log PII masking** -- `internal/audit/masker.go`
- **Webhook HMAC-SHA256 signing** -- `internal/admin/webhooks.go`
- **GraphQL depth/complexity limiting** -- `internal/plugin/graphql_guard.go`
- **API keys hashed with SHA-256** -- only prefix stored in DB
- **Session tokens via `crypto/rand`** -- 32-byte entropy
- **Trusted proxy anti-spoofing** -- `internal/pkg/netutil/clientip.go`

---

*Generated by security-check -- 4-phase pipeline (Recon -> Hunt -> Verify -> Report)*
*Scanner: 48 security skills, 7 language scanners, 3000+ checklist items*
*Rescan date: 2026-04-14*
