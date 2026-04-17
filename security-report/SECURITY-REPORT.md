# APICerebrus Security Report

**Date:** 2026-04-18 (updated)
**Project:** APICerebrus API Gateway
**Scope:** Full codebase (Go backend + React frontend + Infrastructure)
**Phase:** Hunt + Verify complete. 4-phase pipeline run (2026-04-18).
**Analysis:** 4 parallel vulnerability scanning agents (Injection, Auth, Secrets, Server-Side) + manual verification.

## Executive Summary

APICerebrus demonstrates a **strong security posture** overall. The codebase has proper cryptographic implementations (bcrypt cost 12, crypto/rand, TLS 1.2+ enforcement, constant-time comparisons, HS256 minimum 32-byte secret). Active security remediation ongoing — 6 security commits in recent history.

**Critical Vulnerabilities: 0**
**High Vulnerabilities: 1** (was 7 — 4 fixed 2026-04-18, 2 infrastructure hardened)
**Medium Vulnerabilities: 11** (was 13 — REDIR-001, REDIR-002, GQL-001, GQL-002 fixed)
**Low/Info Findings: 10** (was 12)

**Overall Risk Level: MEDIUM**

---

## Critical — 0

None.

---

## High — 1 (was 7)

| ID | Category | CWE | Title | Location | Status |
|----|----------|-----|-------|----------|--------|
| H-003 | Business Logic | CWE-362 | TOCTOU race condition in credit PreCheck vs Deduct | internal/billing/engine.go:92-192 | Open |

---

## Medium — 13 (was 16)

| ID | Category | CWE | Title | Location | Status |
|----|----------|-----|-------|----------|--------|
| GQL-001 | GraphQL | CWE-943 | ~~Batch query string interpolation without escaping~~ | internal/federation/executor.go:675-677 | **FIXED** — `escapeGraphQLString()` added |
| GQL-002 | GraphQL | CWE-943 | ~~Field argument interpolation uses `%v` instead of JSON encoding~~ | internal/federation/planner.go:222 | **FIXED** — JSON encoding for field args |
| REDIR-001 | SSRF/Open Redirect | CWE-601 | ~~Redirect plugin accepts arbitrary `TargetURL` (no scheme validation)~~ | internal/plugin/redirect.go:61 | **FIXED** — scheme allow-list in `isValidRedirectTarget()` |
| REDIR-002 | Open Redirect | CWE-601 | ~~OIDC logout `post_logout_redirect_uri` reflected to IdP~~ | internal/admin/oidc.go:406-410 | **FIXED** — hard-coded to `/dashboard?logout=1` |
| OIDC-001 | Auth | CWE-306 | OIDC authorize endpoint uses hardcoded `"user@example.com"` placeholder | internal/admin/oidc_provider.go:292-294 | OPEN |
| OIDC-002 | Auth | CWE-287 | OIDC provider lacks PKCE support (RFC 7636) for public clients | internal/admin/oidc_provider.go:247-326 | OPEN |
| S-001 | Crypto | CWE-328 | Raft TLS hardcoded serial numbers (`big.NewInt(1)`, `big.NewInt(2)`) | internal/raft/tls.go:40,80 | OPEN |
| S-002 | Crypto | CWE-295 | Unnecessary `"localhost"` in node certificate DNSNames | internal/raft/tls.go:89 | OPEN |
| M-014 | Frontend | CWE-352 | ~~Admin API missing CSRF on state-changing requests~~ | internal/admin/token.go | **FIXED 2026-04-18** |
| H-001 | Auth | CWE-287 | ~~Admin key rotation does not revoke existing sessions~~ | internal/admin/token.go:311-373 | **FIXED 2026-04-18** |
| H-NEW-1 | OIDC | CWE-284 | ~~OIDC introspection exposes claims for expired tokens~~ | internal/admin/oidc_provider.go:757-764 | **FIXED 2026-04-18** |
| CRIT-1 | Auth | CWE-345 | ~~OIDC UserInfo token signature not verified~~ | internal/admin/oidc_provider.go:591-596 | **FIXED 2026-04-18** |
| H-005 | Data | CWE-311 | SQLite database not encrypted at rest | internal/store/store.go | Open (won't fix — operator responsibility) |

---

## Positive Security Findings

| Category | Finding |
|----------|---------|
| Password Hashing | bcrypt cost 12 |
| Admin JWT Secret | Minimum 32 characters enforced |
| crypto/rand | All random generation uses crypto/rand.Reader correctly |
| Constant-Time Compare | Admin key uses subtle.ConstantTimeCompare() |
| TLS Enforcement | TLS 1.0/1.1 rejected, TLS 1.3 required in K8s configs |
| Raft mTLS | TLS 1.3 minimum, client certs required |
| HttpOnly Cookies | Admin cookies set HttpOnly, Secure, SameSite=StrictMode |
| SQL Injection | All queries use parameterized placeholders |
| NoSQL Injection | Redis Lua scripts use KEYS/ARGV safely |
| XSS | No dangerouslySetInnerHTML, no innerHTML assignments |
| WASM Panic Recovery | SEC-WASM-003: defer recover() implemented |
| WASM Phase Validation | SEC-WASM-001/002: PhaseAuth and PhasePostProxy forbidden |
| Non-root Containers | All Docker images run as non-root |

---

## Dependency Audit

| Category | Status |
|----------|--------|
| Direct Dependencies | 23 |
| Indirect Dependencies | 27 |
| Known CVEs | 0 unpatched |
| License Compliance | CLEAN |
| Unofficial Modules | NONE |

---

## Remediated Since Last Audit

| ID | Description | Commit |
|----|-------------|--------|
| WASM-003 | Panic recovery in WASM Execute/Run/AfterProxy | 8787ce2 |
| GQL-011 | X-Admin-Key required on GET /sse | b9f221a |
| GQL-010 | Drop path arg from system.config.import | c9add9d |
| GQL-007 | Origin allow-list for subscription WS+SSE | 96d32aa |
| GQL-006 | @authorized enforced at execution time | 1ea67fa |

---

## Remediation Roadmap

### Immediate (High)
1. H-001: Implement JWT token revocation on admin key rotation
2. H-002: Add field allowlisting to config import
3. H-003: Use SELECT FOR UPDATE for atomic billing
4. H-004: Reject test_mode_enabled in production
5. H-005: Document SQLite access controls

### Short-term (Medium)
6. M-001: Add admin key minimum length validation
7. M-002: Implement JWT blacklisting on logout
8. M-003: gRPC-Web — configurable allowed origins
9. M-005: Fix sliding window race condition
10. M-007: Add rate limiting to credit endpoints
11. M-009: Reject unresolved hostnames
12. M-010: Run security scans on forked PRs
13. M-013: Set allowed_health_ips default to localhost

---
Report generated: 2026-04-18 (updated)
**Previous report:** `security-report/verified-findings.md`
