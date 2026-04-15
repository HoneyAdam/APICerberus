# Verified Findings — APICerebrus Security Audit 2026-04-15

## Summary

| Severity | Count |
|----------|-------|
| CRITICAL | 5 |
| HIGH | 14 |
| MEDIUM | 23 |
| LOW | 11 |
| **Total** | **53** |

---

## CRITICAL

### C-001: OIDC refresh_token Grant Accepts Arbitrary Value
- **File:** `internal/admin/oidc_provider.go:393-400`
- **CWE:** CWE-288
- **CVSS:** 9.1
- **Status:** VERIFIED — `generateAccessToken` called with hardcoded `"user@example.com"` subject and no refresh token validation
- **Fix:** Store issued refresh tokens (hashed) in database. Validate provided token against stored, non-revoked, non-expired tokens.

### C-002: Exposed Docker Socket in Promtail
- **File:** `deployments/monitoring/docker-compose.yml:89-90`
- **CWE:** CWE-276
- **CVSS:** 9.1
- **Status:** VERIFIED — `/var/run:/var/run:ro` mount confirmed
- **Fix:** Remove socket mount. Use Docker logging driver or sidecar pattern instead.

### C-003: Hardcoded Placeholder Secrets in K8s
- **File:** `deployments/kubernetes/base/secret.yaml:16-17`
- **CWE:** CWE-259
- **CVSS:** 9.1
- **Status:** VERIFIED — `jwt-secret: "CHANGE_ME_IN_PRODUCTION"` confirmed
- **Fix:** Remove stringData entirely. Use `kubectl create secret` or external secrets manager. Add to `.gitignore`.

### C-004: OIDC Auth Codes Not Persisted
- **File:** `internal/admin/oidc_provider.go:299-307`
- **CWE:** CWE-613
- **CVSS:** 8.9
- **Status:** VERIFIED — `authCodes` is in-memory `map[string]*authCodeEntry`
- **Fix:** Persist to database with status tracking (pending/used/revoked). Implement revocation endpoint.

### C-005: Template Injection in Webhook Templates
- **File:** `internal/analytics/webhook_templates.go:348-388, 553-558`
- **CWE:** CWE-94 / CWE-1336
- **CVSS:** 8.6
- **Status:** VERIFIED — `template.New().Funcs().Parse(tpl.Body)` with user-controlled body and header values
- **Fix:** Validate template bodies before saving. Reject `{{` `}}` except in documented safe patterns. Consider allowlist-based field access.

---

## HIGH

| ID | Finding | CWE | CVSS | File | Fix Required |
|----|---------|-----|------|------|--------------|
| H-001 | SSRF: Hostnames bypass upstream validation | CWE-918 | 8.6 | `proxy.go:330-335`, `optimized_proxy.go:465` | Resolve hostname inline, validate IP |
| H-002 | SSRF: Federation executor no re-validation | CWE-918 | 8.6 | `federation/executor.go:341` | Re-validate URL before each execution |
| H-003 | CORS wildcard origin misconfiguration | CWE-942 | 8.1 | `cors.go:28-44`, `RouteBuilder.tsx:51` | Reject wildcard at config validation |
| H-004 | JWT algorithm confusion HS256/RS256 | CWE-346 | 7.5 | `auth_jwt.go:182-212` | Algorithm allowlist per key type |
| H-005 | Admin password printed to stderr | CWE-532 | 7.5 | `user_repo.go:531-541` | Remove all plaintext password output |
| H-006 | Default Grafana password | CWE-259 | 7.5 | `.env.example:55-56` | Empty default, fail if not set |
| H-007 | OTLP Bearer token placeholder | CWE-532 | 7.1 | `apicerberus.example.yaml:100-101` | Remove placeholder |
| H-008 | Kafka TLS InsecureSkipVerify configurable | CWE-295 | 7.1 | `kafka.go:363-368` | Make skip verify an error |
| H-009 | WebSocket subdomain bypass | CWE-346 | 7.1 | `ws.go:296-309` | Require dot in prefix |
| H-010 | Admin port via K8s ingress | CWE-306 | 8.1 | `ingress.yaml:40-49` | Remove admin from ingress |
| H-011 | DB password in Docker env | CWE-312 | 7.1 | `docker-compose.swarm.yml:84` | Use Swarm secrets |
| H-012 | Postgres default credential | CWE-259 | 8.6 | `swarm-raft.yml:104-105` | Use `:${VAR:?error}` syntax |
| H-013 | CI/CD no production approval | CWE-284 | 8.1 | `ci.yml:513-517` | GitHub Environments with reviewers |
| H-014 | Prometheus scrapes admin metrics | CWE-306 | 7.5 | `servicemonitor.yaml:15-16` | Dedicated internal metrics port |

---

## MEDIUM

| ID | Finding | CWE | CVSS | File |
|----|---------|-----|------|------|
| M-001 | JWT `iat` not validated | CWE-345 | 6.5 | `auth_jwt.go:292-389` |
| M-002 | JTI replay cache silent when nil | CWE-345 | 6.5 | `auth_jwt.go:259-290` |
| M-003 | X-Real-IP not validated | CWE-346 | 6.8 | `clientip.go:126-130` |
| M-004 | `/health`, `/metrics` bypass rate limiting | CWE-307 | 5.3 | `server.go:979-1039` |
| M-005 | Token bucket race condition | CWE-307 | 6.8 | `token_bucket.go:58-68` |
| M-006 | Leaky bucket allows capacity+1 | CWE-799 | 5.3 | `leaky_bucket.go:56` |
| M-007 | Admin token endpoint no rate limit | CWE-307 | 6.5 | `server.go:136` |
| M-008 | OIDC token endpoint no rate limit | CWE-307 | 6.5 | `oidc_provider.go:318-421` |
| M-009 | OIDC introspect no sig verification | CWE-345 | 4.3 | `oidc_provider.go:612-654` |
| M-010 | OIDC introspect no `aud` validation | CWE-345 | 5.3 | `oidc_provider.go:639-650` |
| M-011 | Redis silent fallback to local | CWE-703 | 5.3 | `redis.go:199-204` |
| M-012 | GraphQL batch no max size | CWE-799 | 5.3 | `server.go:1115-1178` |
| M-013 | RBAC default-allow for unmapped | CWE-285 | 4.3 | `rbac.go:194-199` |
| M-014 | Mass assignment admin update | CWE-915 | 6.5 | `admin_routes.go:72-118` |
| M-015 | Consumer API key no entropy check | CWE-307 | 5.3 | `config/load.go:540-548` |
| M-016 | WASM memory bounds not validated | CWE-787 | 6.5 | `wasm.go:351-403` |
| M-017 | WASM EnvVars host leak risk | CWE-200 | 5.3 | `wasm.go:35` |
| M-018 | GraphQL parser no depth limit | CWE-400 | 5.3 | `graphql/parser.go:334` |
| M-019 | JSON no nesting depth limit | CWE-770 | 4.8 | `graphql/request.go:99-127` |
| M-020 | Portal CSRF JSON bypass | CWE-352 | 4.8 | `portal/server.go:660-684` |
| M-021 | Admin API no CSRF token | CWE-352 | 5.3 | `web/src/lib/api.ts:83` |
| M-022 | Auth state in sessionStorage | CWE-312 | 6.1 | `web/src/lib/api.ts:29,40` |
| M-023 | WebSocket no origin validation | CWE-346 | 5.3 | `web/src/lib/ws.ts:85` |

---

## LOW

| ID | Finding | CWE | CVSS | File |
|----|---------|-----|------|------|
| L-001 | FTS5 sanitization incomplete | CWE-89 | 3.1 | `audit_search.go:222` |
| L-002 | Billing amounts in audit trail | CWE-532 | 3.1 | `billing/engine.go:167-176` |
| L-003 | Kafka may export masked body data | CWE-209 | 3.1 | `audit/kafka.go:49-56` |
| L-004 | API key linear scan fallback | CWE-799 | 2.1 | `auth_apikey.go:181-206` |
| L-005 | HS256 32-byte minimum may be weak | CWE-326 | 3.7 | `hs256.go:5` |
| L-006 | SameSite=Lax on admin cookie | CWE-1275 | 4.3 | `admin/token.go:293-301` |
| L-007 | Raw random session token | CWE-340 | 3.1 | `session_repo.go:44-50` |
| L-008 | OIDC hardcoded placeholder subject | CWE-IMP | 3.7 | `oidc_provider.go:283-284` |
| L-009 | UpdateLastUsed swallows errors | CWE-391 | 2.1 | `api_key_repo.go:269-288` |
| L-010 | Test mode key no entropy check | CWE-327 | 5.3 | `billing/engine.go:199-201` |
| L-011 | Prometheus admin API enabled | CWE-306 | 7.5 | `docker-compose.monitoring.yml:32-33` |

---

## Ruled Out (False Positives)

| Category | Result | Evidence |
|----------|--------|----------|
| SQL Injection | None found | All store repos use parameterized `?` placeholders; `normalizeUserSortBy` whitelists columns |
| Command Injection | None found | No `os/exec` usage in codebase |
| LDAP Injection | N/A | No LDAP integration |
| XXE | N/A | No XML parsing |
| Deserialization | None critical | JSON unmarshaling into typed structs |
| Path Traversal (dashboard) | Not exploitable | embed.FS read-only sandbox, path.Clean normalization |
| Open Redirect (redirect plugin) | Not exploitable | TargetURL from admin config only |
| WASM Path Traversal | Protected | safeResolvePath with `..` check |

*Generated: 2026-04-15*
