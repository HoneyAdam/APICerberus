# Verified Security Findings

**Date:** 2026-04-14 (Rescan)
**Method:** Manual code review across all internal/ and web/src/ packages
**Previous:** 62 findings (2026-04-09) -- all resolved; 50 findings (2026-04-13) -- 42 fixed, 8 acknowledged
**This Scan:** Full rescan confirming all prior fixes + 1 new finding

---

## New Finding (2026-04-14)

| ID | Severity | Finding | Confidence | File | CWE | Status |
|----|----------|---------|------------|------|-----|--------|
| N1 | CRITICAL | Hardcoded production secrets in git-tracked `apicerberus.yaml` | HIGH | `apicerberus.yaml:41-42,52` | CWE-798 | PENDING |

---

## Confirmed Fixed (from 2026-04-13 Audit)

### Critical (6/6 Fixed)

| ID | Finding | File | Verified |
|----|---------|------|----------|
| C1 | SSTI via `printf` in webhook template engine | `internal/analytics/webhook_templates.go` | YES |
| C2 | Horizontal privilege escalation -- password reset | `internal/admin/admin_users.go` | YES |
| C3 | Non-atomic credit operations | `internal/admin/admin_billing.go`, `internal/billing/engine.go` | YES |
| C4 | URL param auth state injection | `web/src/App.tsx` | YES |
| C5 | Plaintext initial admin password to file | `internal/store/user_repo.go` | YES |
| C6 | Private key PEM in Raft FSM snapshots | `internal/raft/fsm.go` | YES |

### High (15/20 Fixed, 5 Acknowledged)

| ID | Finding | File | Status |
|----|---------|------|--------|
| H1 | Slice offset underflow in Raft | `internal/raft/node.go` | FIXED |
| H2 | OIDC provider race condition | `internal/admin/oidc.go` | FIXED |
| H3 | Session cookie Secure=false | `internal/admin/oidc.go` | FIXED |
| H4 | Test key bypasses credits | `internal/billing/engine.go` | FIXED |
| H5 | Database errors leaked | `internal/admin/admin_billing.go` | FIXED |
| H6 | Raft RPC token in cleartext | `internal/raft/transport.go` | FIXED |
| H7 | Response body leak | `internal/raft/cluster.go` | FIXED |
| H8 | RBAC bypass for static key | `internal/admin/rbac.go` | FIXED |
| H9 | Privilege escalation -- role | `internal/admin/admin_users.go` | FIXED |
| H10 | Mass assignment | `internal/admin/admin_users.go` | FIXED |
| H11 | OIDC rate limiting missing | `internal/admin/oidc.go` | FIXED |
| H12 | Session in sessionStorage | `web/src/lib/api.ts` | FIXED |
| H13 | CSRF token in sessionStorage | `web/src/lib/portal-api.ts` | FIXED |
| H14 | WebSocket origin + transport | `web/src/lib/ws.ts` | ACKNOWLEDGED |
| H15 | Realtime store caching | `web/src/stores/realtime.ts` | ACKNOWLEDGED |
| H16 | Playground API key in state | `web/src/components/portal/playground/types.ts` | ACKNOWLEDGED |
| H17 | API endpoints in query cache | `web/src/hooks/query-keys.ts` | ACKNOWLEDGED |
| H18 | recharts CVE-2024-21539 | `web/package.json` | ACKNOWLEDGED |
| H19 | MCP SSE unauthenticated | `internal/mcp/server.go` | FIXED |
| H20 | WASI not gated by config | `internal/plugin/wasm.go` | FIXED |

### Medium (15/16 Fixed, 1 Acknowledged)

| ID | Finding | File | Status |
|----|---------|------|--------|
| M1 | JWT lacks aud/iss/jti | `internal/admin/token.go` | FIXED |
| M2 | No iat validation | `internal/admin/token.go` | FIXED |
| M3 | Webhook SSRF | `internal/admin/webhooks.go` | FIXED |
| M4 | Upstream private IPs | `internal/gateway/proxy.go` | FIXED |
| M5 | Open redirect | `internal/admin/oidc.go` | FIXED |
| M6 | bcrypt cost | `internal/store/user_repo.go` | FIXED |
| M7 | math/rand jitter | `internal/raft/node.go` | FIXED |
| M8 | OIDC error in redirect | `internal/admin/oidc.go` | FIXED |
| M9 | TOCTOU rate limiter | `internal/ratelimit/fixed_window.go` | FIXED |
| M10 | IP whitelist validation | `internal/admin/admin_users.go` | FIXED |
| M11 | Unauthenticated /info | `internal/admin/server.go` | FIXED |
| M12 | Credit amount overflow | `internal/admin/admin_billing.go` | FIXED |
| M13 | adminApiRequest auth | `web/src/lib/api.ts` | ACKNOWLEDGED |
| M14 | Admin key min length | `web/src/pages/admin/Login.tsx` | FIXED |
| M15 | Portal rate limit unbounded | `internal/portal/server.go` | FIXED |

### Low (6/8 Fixed, 2 Acknowledged)

| ID | Finding | File | Status |
|----|---------|------|--------|
| L1 | Webhook test SSRF | `internal/admin/webhooks.go` | FIXED |
| L2 | JSON encoding errors | `internal/raft/transport.go` | FIXED |
| L3 | Log injection | `internal/audit/logger.go` | FIXED |
| L4 | iat/nbf validation | `internal/admin/token.go` | FIXED |
| L5 | Hardcoded API paths | `web/src/lib/constants.ts` | ACKNOWLEDGED |
| L6 | localStorage for theme | `web/src/stores/theme.ts` | ACKNOWLEDGED |
| L7 | constantTimeEqual | `internal/admin/oidc.go` | FIXED |
| L8 | Client-side rate limit | `web/src/pages/portal/Login.tsx` | ACKNOWLEDGED |

---

## Ruled Out (False Positives)

| Category | Result | Notes |
|----------|--------|-------|
| SQL Injection | **None found** | All store repos use parameterized `?` placeholders; `normalizeUserSortBy` whitelists columns |
| Command Injection | **None found** | No `os/exec` usage anywhere in codebase |
| LDAP Injection | **N/A** | No LDAP integration |
| XXE | **N/A** | No XML parsing |
| Deserialization | **None critical** | JSON unmarshaling into typed structs |
| `math/rand` for secrets | **Acceptable** | Only for analytics sampling and jitter (annotated, using math/rand/v2) |

---

## Summary

| Severity | Total | Fixed | Acknowledged | New/Pending |
|----------|-------|-------|--------------|-------------|
| Critical | 7 | 6 | 0 | 1 (N1) |
| High | 20 | 15 | 5 | 0 |
| Medium | 16 | 15 | 1 | 0 |
| Low | 8 | 6 | 2 | 0 |
| **Total** | **51** | **42** | **8** | **1** |
