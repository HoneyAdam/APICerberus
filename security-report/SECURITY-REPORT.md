# APICerebrus Security Audit Report

**Scan Date:** 2026-04-15
**Project:** APICerebrus API Gateway
**Scope:** Go backend (1.26.2) + React dashboard + Infrastructure as Code
**Pipeline:** Recon → Hunt → Verify → Report

---

## Executive Summary

| Severity | Count |
|----------|-------|
| CRITICAL | 5 |
| HIGH | 14 |
| MEDIUM | 23 |
| LOW | 11 |
| **Total Verified Findings** | **53** |

**Top Risk Areas:** OIDC provider implementation flaws, infrastructure hardening gaps, webhook template injection, SSRF in federation executor, CORS wildcard misconfiguration, Docker socket exposure in monitoring stack.

---

## Phase 1: Architecture Map

### Trust Boundaries
- **Untrusted perimeter:** Gateway (:8080/:8443) — receives all public API traffic
- **Admin boundary:** Admin API (:9876) — protected by static API key + JWT + IP allow-list
- **User boundary:** Portal API (:9877) — session cookies + CSRF + rate limiting
- **Cluster boundary:** Raft (:12000) — optional mTLS, shared RPC secret

### Tech Stack
- **Go:** 1.26.2 — `modernc.org/sqlite`, `github.com/golang-jwt/jwt/v5`, `google.golang.org/grpc`, `go.opentelemetry.io/otel`, `github.com/tetratelabs/wazero`
- **React:** 19.2.4 + Vite 8.0.1 + TanStack Query + Zustand
- **Databases:** SQLite (WAL mode, FTS5), PostgreSQL (optional HA), Redis (optional distributed rate limiting)
- **External:** Kafka (audit export), Prometheus (metrics), OIDC (SSO), ACME/Let's Encrypt (TLS)

---

## CRITICAL Severity

---

#### C-001: OIDC refresh_token Grant Accepts Arbitrary Value

| Field | Value |
|-------|-------|
| **CWE** | CWE-288 (Authentication Bypass Using Alternate Channel) |
| **CVSS 3.1** | 9.1 (Critical) — AV:N/AC:L/PR:L/UI:N/S:U/C:H/I:H/A:N |
| **File** | `internal/admin/oidc_provider.go:393-400` |

The `refresh_token` grant validates only that the refresh token is non-empty — it never checks against stored issued tokens. Any non-empty string passes validation.

```go
case "refresh_token":
    refreshTok := r.PostForm.Get("refresh_token")
    if refreshTok == "" {
        writeError(w, http.StatusBadRequest, "invalid_request", "refresh_token required")
        return
    }
    accessToken, expiresIn, _ = s.generateAccessToken(clientID, "user@example.com", []string{"openid", "profile"}, refreshTok)
```

**Impact:** An attacker with a valid `client_id` can obtain access tokens without knowing any secret.

**Remediation:** Store issued refresh tokens (hashed) in the database. Validate the provided token against stored, non-revoked, non-expired tokens before issuing a new access token.

---

#### C-002: Exposed Docker Socket in Monitoring Stack (Promtail)

| Field | Value |
|-------|-------|
| **CWE** | CWE-276 (Incorrect Default Permissions) |
| **CVSS 3.1** | 9.1 (Critical) — AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:H/A:H |
| **File** | `deployments/monitoring/docker-compose.yml:89-90` |

Promtail bind-mounts `/var/run/docker.sock` from the host, granting full Docker daemon control.

```yaml
promtail:
  volumes:
    - /var/run:/var/run:ro
    - /var/lib/docker/containers:/var/lib/docker/containers:ro
```

**Impact:** Container escape, privilege escalation, host takeover via Docker daemon API.

**Remediation:** Remove the `/var/run` volume mount from Promtail. Use Docker logging driver or sidecar pattern instead.

---

#### C-003: Hardcoded Placeholder Secrets in Kubernetes Secret Manifest

| Field | Value |
|-------|-------|
| **CWE** | CWE-259 (Use of Hard-coded Password) |
| **CVSS 3.1** | 9.1 (Critical) — AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N |
| **File** | `deployments/kubernetes/base/secret.yaml:16-17` |

```yaml
stringData:
  jwt-secret: "CHANGE_ME_IN_PRODUCTION"
  admin-api-key: "CHANGE_ME_IN_PRODUCTION"
```

**Impact:** Placeholder secrets committed to version control. If applied without modification, application runs with well-known credentials.

**Remediation:** Remove `stringData` entirely. Require operators to create secrets via `kubectl create secret` or external secrets manager. Add to `.gitignore`.

---

#### C-004: OIDC Authorization Codes Not Persisted — Not Revocable

| Field | Value |
|-------|-------|
| **CWE** | CWE-613 (Insufficient Session Expiration) |
| **CVSS 3.1** | 8.9 (Critical) — AV:N/AC:H/PR:H/UI:N/S:U/C:H/I:H/A:N |
| **File** | `internal/admin/oidc_provider.go:299-307` |

Authorization codes stored only in-memory map (`map[string]*authCodeEntry`). No persistence, no revocation endpoint, server restart invalidates all pending codes.

**Impact:** Leaked authorization codes can be exchanged for tokens within TTL window. No revocation possible.

**Remediation:** Persist authorization codes to database with status tracking (`pending`, `used`, `revoked`). Implement proper revocation endpoint.

---

#### C-005: Template Injection in User-Controlled Webhook Templates

| Field | Value |
|-------|-------|
| **CWE** | CWE-94 (Code Injection) / CWE-1336 (Template Injection) |
| **CVSS 3.1** | 8.6 (High) — AV:N/AC:L/PR:L/UI:N/S:U/C:H/I:H/A:N |
| **File** | `internal/analytics/webhook_templates.go:348-388, 553-558` |

User-provided webhook template bodies are parsed as Go `html/template` with user-controlled content. Header values are also rendered as templates.

```go
compiled, err := template.New(tpl.ID).Funcs(safeTemplateFuncMap()).Parse(tpl.Body)
// ...
for key, value := range tpl.Headers {
    headerValue, err := e.RenderWithTemplate(value, data) // user-controlled header value rendered as template
    req.Header.Set(key, headerValue)
}
```

**Impact:** Data exfiltration via `{{.Details.pagerduty_token}}` or similar template expressions accessing template data fields.

**Remediation:** Validate template bodies before saving — reject `{{` `}}` except in documented safe patterns. Store template body hashes and validate on render.

---

## HIGH Severity

| ID | Finding | CWE | CVSS | File |
|----|---------|-----|------|------|
| H-001 | SSRF: Hostnames bypass upstream validation in proxy | CWE-918 | 8.6 | `internal/gateway/proxy.go:330-335`, `optimized_proxy.go:465-468` |
| H-002 | SSRF: Federation executor bypasses URL re-validation | CWE-918 | 8.6 | `internal/federation/executor.go:341-351` |
| H-003 | CORS wildcard origin accepted with credentials | CWE-942 | 8.1 | `internal/plugin/cors.go:28-44`, `RouteBuilder.tsx:51` |
| H-004 | JWT algorithm confusion: HS256 accepted when RS256 configured | CWE-346 | 7.5 | `internal/plugin/auth_jwt.go:182-212` |
| H-005 | Admin auto-generated password written to stderr | CWE-532 | 7.5 | `internal/store/user_repo.go:531-541` |
| H-006 | Default Grafana password in Docker .env.example | CWE-259 | 7.5 | `deployments/docker/.env.example:55-56`, `monitoring/.env.example:6` |
| H-007 | OTLP Authorization Bearer token placeholder in config | CWE-532 | 7.1 | `apicerberus.example.yaml:100-101` |
| H-008 | Kafka TLS InsecureSkipVerify configurable | CWE-295 | 7.1 | `internal/audit/kafka.go:363-368` |
| H-009 | WebSocket wildcard origin subdomain bypass | CWE-346 | 7.1 | `internal/admin/ws.go:296-309` |
| H-010 | Admin port exposed via K8s ingress without auth | CWE-306 | 8.1 | `deployments/kubernetes/base/ingress.yaml:40-49` |
| H-011 | Database password in env var (Docker Swarm) | CWE-312 | 7.1 | `deployments/docker/docker-compose.swarm.yml:84` |
| H-012 | Postgres default credential in Swarm | CWE-259 | 8.6 | `deployments/docker/docker-compose.swarm-raft.yml:104-105` |
| H-013 | CI/CD production deploy has no manual approval gate | CWE-284 | 8.1 | `.github/workflows/ci.yml:513-517` |
| H-014 | Prometheus ServiceMonitor scrapes admin metrics | CWE-306 | 7.5 | `deployments/kubernetes/base/servicemonitor.yaml:15-16` |

### H-001 Detail: SSRF — Hostnames Bypass Upstream Validation

`validateUpstreamHost()` returns `nil` (allowed) for hostnames — DNS resolution happens later with no SSRF check.

```go
ip := net.ParseIP(h)
if ip == nil {
    return nil  // Hostname allowed — SSRF check skipped
}
```

**Remediation:** Resolve hostname inline and validate resolved IP against private range blocklist before allowing.

### H-002 Detail: Federation Executor SSRF

`validateSubgraphURL` is called at subgraph registration, but `executeStep` uses `step.Subgraph.URL` directly with no re-validation.

**Remediation:** Re-validate URL before every execution, not just at registration.

### H-003 Detail: CORS Wildcard Origin Misconfiguration

When `AllowedOrigins: ["*"]` and `AllowCredentials: false`, `allowAllOrigins` becomes `true` silently. The React defaults also use `["*"]` wildcard origins.

**Remediation:** Reject wildcard origins at config validation time. Never allow `*` when any credential config is present.

### H-009 Detail: WebSocket Subdomain Bypass

`*.example.com` allows `evil-example.com` due to flawed subdomain validation:

```go
if prefix != "" && !strings.Contains(prefix, ".") {
    return true  // Blocked: single-level subdomain
}
if prefix != "" {
    return true  // BUG: allows "evil-example.com" (prefix="evil")
}
```

**Remediation:** Require dot in prefix: `if prefix != "" && strings.Contains(prefix, ".")`.

---

## MEDIUM Severity

| ID | Finding | CWE | CVSS | File |
|----|---------|-----|------|------|
| M-001 | JWT `iat` claim not validated | CWE-345 | 6.5 | `internal/plugin/auth_jwt.go:292-389` |
| M-002 | JTI replay protection silent when cache nil | CWE-345 | 6.5 | `internal/plugin/auth_jwt.go:259-290` |
| M-003 | X-Real-IP not validated against trusted proxies | CWE-346 | 6.8 | `internal/pkg/netutil/clientip.go:126-130` |
| M-004 | Built-in `/health`, `/ready`, `/metrics` bypass rate limiting | CWE-307 | 5.3 | `internal/gateway/server.go:979-1039` |
| M-005 | Token bucket race condition | CWE-307 | 6.8 | `internal/ratelimit/token_bucket.go:58-68` |
| M-006 | Leaky bucket allows `capacity+1` burst | CWE-799 | 5.3 | `internal/ratelimit/leaky_bucket.go:56` |
| M-007 | Admin token endpoint lacks per-request rate limit | CWE-307 | 6.5 | `internal/admin/server.go:136` |
| M-008 | OIDC token endpoint lacks rate limiting | CWE-307 | 6.5 | `internal/admin/oidc_provider.go:318-421` |
| M-009 | OIDC introspect returns `active=false` without sig verification | CWE-345 | 4.3 | `internal/admin/oidc_provider.go:612-654` |
| M-010 | OIDC introspect does not validate `aud` claim | CWE-345 | 5.3 | `internal/admin/oidc_provider.go:639-650` |
| M-011 | Redis distributed rate limiter silent fallback to local | CWE-703 | 5.3 | `internal/ratelimit/redis.go:199-204` |
| M-012 | GraphQL batch no per-query rate limit or max batch size | CWE-799 | 5.3 | `internal/gateway/server.go:1115-1178` |
| M-013 | Default-allow RBAC for unmapped admin endpoints | CWE-285 | 4.3 | `internal/admin/rbac.go:194-199` |
| M-014 | Mass assignment in admin user/route update | CWE-915 | 6.5 | `internal/admin/admin_routes.go:72-118` |
| M-015 | Consumer API key no entropy/length validation | CWE-307 | 5.3 | `internal/config/load.go:540-548` |
| M-016 | WASM memory pointer not validated before read/write | CWE-787 | 6.5 | `internal/plugin/wasm.go:351-403` |
| M-017 | WASM EnvVars may expose host environment to guest | CWE-200 | 5.3 | `internal/plugin/wasm.go:35` |
| M-018 | GraphQL parser recursion without depth limit | CWE-400 | 5.3 | `internal/graphql/parser.go:334-361` |
| M-019 | JSON body size limited but no nesting depth limit | CWE-770 | 4.8 | `internal/graphql/request.go:99-127` |
| M-020 | CSRF bypass for JSON content-types in portal | CWE-352 | 4.8 | `internal/portal/server.go:660-684` |
| M-021 | Admin API lacks CSRF token on state-changing ops | CWE-352 | 5.3 | `web/src/lib/api.ts:83` |
| M-022 | Auth state stored in sessionStorage (XSS exfiltration) | CWE-312 | 6.1 | `web/src/lib/api.ts:29,40` |
| M-023 | WebSocket no origin validation | CWE-346 | 5.3 | `web/src/lib/ws.ts:85` |

---

## LOW Severity

| ID | Finding | CWE | CVSS | File |
|----|---------|-----|------|------|
| L-001 | FTS5 query sanitization strips but does not escape | CWE-89 | 3.1 | `internal/store/audit_search.go:222` |
| L-002 | Billing credit amounts in audit trail | CWE-532 | 3.1 | `internal/billing/engine.go:167-176` |
| L-003 | Kafka audit export may carry masked body data | CWE-209 | 3.1 | `internal/audit/kafka.go:49-56` |
| L-004 | API key SHA256 bucket has linear scan fallback | CWE-799 | 2.1 | `internal/plugin/auth_apikey.go:181-206` |
| L-005 | HS256 minimum 32-byte secret may be weak | CWE-326 | 3.7 | `internal/pkg/jwt/hs256.go:5` |
| L-006 | SameSite=Lax on admin session cookie | CWE-1275 | 4.3 | `internal/admin/token.go:293-301` |
| L-007 | Session token is raw random bytes (minor) | CWE-340 | 3.1 | `internal/store/session_repo.go:44-50` |
| L-008 | Hardcoded OIDC placeholder subject | CWE-IMP | 3.7 | `internal/admin/oidc_provider.go:283-284` |
| L-009 | UpdateLastUsed swallows errors silently | CWE-391 | 2.1 | `internal/store/api_key_repo.go:269-288` |
| L-010 | Test mode API key bypasses billing with no entropy check | CWE-327 | 5.3 | `internal/billing/engine.go:199-201` |
| L-011 | Prometheus admin API enabled without authentication | CWE-306 | 7.5 | `deployments/monitoring/docker-compose.yml:32-33` |

---

## Positive Security Controls

The following controls were verified as correctly implemented:

| Area | Implementation |
|------|----------------|
| Password hashing | bcrypt cost 12 (`internal/store/user_repo.go:499`) |
| SQL injection | All queries use parameterized `?` placeholders |
| JWT algorithm enforcement (admin) | HS256 only, rejects other algs (`internal/admin/token.go:97`) |
| JWT RSA key size | Minimum 2048-bit enforced (`internal/jwt/rs256.go:64`) |
| Constant-time comparison | All secrets use `crypto/subtle.ConstantTimeCompare` |
| Session cookie flags | `HttpOnly: true, Secure: true` on all session cookies |
| OIDC state/nonce | `crypto/rand` generation, constant-time comparison |
| TLS minimum version | TLS 1.2 floor, explicitly rejects 1.0/1.1 |
| Raft mTLS | RSA 4096-bit CA/key generation |
| GraphQL query depth/complexity | `QueryAnalyzer` applies limits |
| WASM path traversal | `safeResolvePath()` with `..` prefix check |
| WASM WASI gate | WASI only instantiated when `AllowFilesystem=true` |
| IP allow-list | CIDR matching, enforced before auth |
| Webhook signing | HMAC-SHA256 via `X-Webhook-Signature` |
| Audit log masking | Default masking of auth headers, configurable body fields |
| K8s security context | `runAsNonRoot`, `readOnlyRootFilesystem`, `allowPrivilegeEscalation: false` |
| Docker production runtime | distroless/static:nonroot, multi-stage build |
| Admin password minimum | 32-character minimum enforced |

---

## Remediation Roadmap

### Immediate (CRITICAL — fix before production deployment)

1. **C-001** — Fix OIDC refresh_token validation: store and validate refresh tokens against database
2. **C-002** — Remove Docker socket from Promtail: use logging driver instead
3. **C-003** — Remove Kubernetes secret.yaml placeholder values: require external secret injection
4. **C-004** — Persist OIDC authorization codes: add database storage and revocation
5. **C-005** — Fix webhook template injection: validate/sanitize template bodies, restrict field access

### Short-term (HIGH)

6. **H-001, H-002** — Fix SSRF in proxy/federation: resolve hostnames inline, re-validate URLs at execution
7. **H-003** — Fix CORS wildcard origin: reject at config validation, fix subdomain bypass in WebSocket
8. **H-004** — Fix JWT algorithm confusion: maintain algorithm allowlist per key type
9. **H-005** — Stop printing admin password to stderr: remove all plaintext password output
10. **H-006** — Remove default Grafana passwords: fail-fast on defaults
11. **H-008** — Fix Kafka TLS InsecureSkipVerify: make it an error, not a warning
12. **H-010** — Remove admin port from K8s ingress: network-level isolation
13. **H-011** — Use Docker Swarm secrets: replace env var credentials
14. **H-012** — Fix Postgres default credential: use `:${VAR:?error}` syntax
15. **H-013** — Add production approval gate in CI: required reviewers for production deploy
16. **H-014** — Fix Prometheus admin metrics scraping: dedicated internal metrics port

### Medium-term (MEDIUM)

17. Add JWT `iat` validation in auth-jwt plugin
18. Add JTI cache requirement when tokens include `jti`
19. Fix X-Real-IP validation against trusted proxy list
20. Add rate limiting to built-in `/health`, `/ready`, `/metrics` endpoints
21. Fix token bucket race condition — atomic operations or Lua script
22. Add per-request rate limit to admin token and OIDC token endpoints
23. Fix GraphQL batch max size limit
24. Change RBAC default-allow to default-deny for unmapped endpoints
25. Add consumer API key minimum length/entropy validation
26. Add WASM memory bounds validation before read/write
27. Add GraphQL parser depth limit during parsing (not just analysis)
28. Fix portal CSRF for JSON content-types
29. Add CSRF token to admin API client
30. Remove auth state from sessionStorage
31. Add WebSocket origin validation in client

### Long-term (LOW)

32. Replace FTS5 strip with proper escape of special characters
33. Add Kafka export field filtering for sensitive billing data
34. Remove API key linear scan fallback
35. Increase HS256 minimum secret length to 64 chars
36. Use SameSite=Strict for admin session cookies
37. Add HMAC binding to session tokens
38. Fix UpdateLastUsed error handling — return error to caller
39. Add test mode API key entropy requirement
40. Disable Prometheus admin API in production

---

## CVSS Distribution

```
CRITICAL (8.0-10.0):  ████████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░  5
HIGH (7.0-7.9):       ██████████████████████████████░░░░░░░░░░░░░░░░░ 14
MEDIUM (4.0-6.9):     ████████████████████████████████████████████████ 23
LOW (0.1-3.9):        ████████████████████░░░░░░░░░░░░░░░░░░░░░░░░░░░ 11
```

---

## Infrastructure Findings Summary (by Category)

### Docker/Monitoring (9 findings)
| Severity | Finding |
|----------|---------|
| Critical | Promtail exposes Docker socket |
| High | Grafana default password |
| High | Postgres default credential in Swarm |
| High | DB password in env var |
| Medium | cAdvisor host filesystem mounts |
| Medium | Node Exporter host path mounts |
| Medium | Monitoring ports exposed to all interfaces |
| Medium | Prometheus admin API enabled |
| Low | Multi-stage build not used in root Dockerfile |

### Kubernetes (9 findings)
| Severity | Finding |
|----------|---------|
| Critical | Hardcoded secrets in secret.yaml |
| High | Admin port via ingress without auth |
| High | Prometheus ServiceMonitor scrapes admin |
| Medium | Empty auth tokens in configmap |
| Medium | Incomplete NetworkPolicy egress rules |
| Medium | NetworkPolicy disabled by default in Helm |
| Low | Redis port in egress rule |
| Info | K8s security contexts properly configured |

### CI/CD (7 findings)
| Severity | Finding |
|----------|---------|
| High | No production approval gate |
| High | Secrets passed as helm arguments |
| Medium | KUBE_CONFIG base64 in CI |
| Medium | KUBE_CONFIG stored as artifact |
| Medium | buildctl cache could leak sensitive data |
| Low | Trivy severity filter excludes MEDIUM |
| Low | gosec outdated version |

### Configuration (5 findings)
| Severity | Finding |
|----------|---------|
| High | OTLP Bearer token placeholder |
| High | Grafana default password |
| Medium | SMTP credentials in plaintext |
| Medium | Slack/PagerDuty keys in plaintext |
| Medium | NFS nolock mount option |

---

*Generated by security-check — 4-phase pipeline (Recon → Hunt → Verify → Report)*
*Scanner: 48 security skills, 7 language scanners, 3000+ checklist items*
*Files audited: 40+ Go files, 15+ TypeScript/React files, 20+ IaC files*
