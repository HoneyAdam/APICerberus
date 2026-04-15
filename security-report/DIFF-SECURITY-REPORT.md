# Security Diff Scan Report

**Repository:** APICerebrus
**Scan Type:** Incremental Diff Scan (Uncommitted Changes)
**Date:** 2026-04-16
**Scan Method:** security-check skill (`scan diff`)

---

## Executive Summary

This report covers **59 files** with **1765 insertions** and **663 deletions** in uncommitted changes. The diff contains significant security hardening across authentication, authorization, SSRF protection, data exposure, and infrastructure security.

**Overall Assessment:** SECURITY HARDENING - The diff shows predominantly security **improvements** (fixes), with some new security documentation warnings. No new critical vulnerabilities were introduced.

---

## Phase 1: Recon - Changed Attack Surface

### Modified Security-Critical Files (47)

| Category | Files | Risk Level |
|----------|------|------------|
| Authentication/Authorization | `rbac.go`, `token.go`, `oidc_provider.go`, `auth_apikey.go`, `auth_jwt.go` | CRITICAL |
| Gateway/Proxy | `proxy.go`, `server.go` | CRITICAL |
| Portal | `portal/server.go` | HIGH |
| Federation | `federation/executor.go`, `subgraph.go` | HIGH |
| Configuration | `config/load.go` | HIGH |
| Infrastructure | `.github/workflows/ci.yml`, `deployments/` | HIGH |
| Billing/Audit | `billing/engine.go`, `audit/kafka.go` | MEDIUM |

---

## Phase 2: Hunt - Vulnerability Findings

### CRITICAL Severity (3)

#### C-001: OIDC Token Introspection Missing Signature Verification (M-009)
**File:** `internal/admin/oidc_provider.go`
**Status:** FIXED (was vulnerable)
```go
// BEFORE: Token was parsed but signature was NOT verified
token, err := jwt.Parse(tokenStr)

// AFTER: Signature is verified before trusting claims
jwt.VerifyRS256(token.SigningInput, token.Signature, &rsaPub.PublicKey)
jwt.VerifyES256(token.SigningInput, token.Signature, &ecdsaPub.PublicKey)
```
**CWE:** CWE-347 (Improper Verification of Cryptographic Signature)
**Impact:** Attackers could forge valid introspection responses for any JWT
**Remediation:** Signature verification added with algorithm-specific checks (RS256, ES256)

---

#### C-002: OIDC Token Introspection Missing Audience Validation (M-010)
**File:** `internal/admin/oidc_provider.go`
**Status:** FIXED (was vulnerable)
```go
// BEFORE: aud claim was not validated
// AFTER: Reject tokens with unknown audience
if !found {
    json.NewEncoder(w).Encode(map[string]any{"active": false, "error": "invalid_audience"})
}
```
**CWE:** CWE-287 (Improper Authentication)
**Impact:** Tokens could be replayed to different clients
**Remediation:** Audience validation against configured OIDC clients

---

#### C-003: RBAC Default Allow for Unmapped Endpoints (M-013)
**File:** `internal/admin/rbac.go`
**Status:** FIXED (was vulnerable)
```go
// BEFORE: No permission mapping → allow
// AFTER: No permission mapping → deny by default
if requiredPerm == "" {
    writeError(w, http.StatusForbidden, "permission_denied",
        "endpoint not classified for RBAC; access denied by default")
}
```
**CWE:** CWE-285 (Improper Authorization)
**Impact:** New endpoints could bypass authorization checks
**Remediation:** Default deny policy with explicit endpoint classification required

---

### HIGH Severity (7)

#### H-001: Portal CSRF Bypass via Content-Type Exception (M-020)
**File:** `internal/portal/server.go`
**Status:** FIXED (was vulnerable)
```go
// BEFORE: JSON content-types could skip CSRF validation
if strings.Contains(contentType, "application/x-www-form-urlencoded") ||
    strings.Contains(contentType, "multipart/form-data") {
    // reject
}
// For API calls (JSON), allow without CSRF

// AFTER: All state-changing requests require valid CSRF
if r.Method == "POST" || r.Method == "PUT" || r.Method == "DELETE" || r.Method == "PATCH" {
    csrfCookie, err := r.Cookie(csrfCookieName)
    if err != nil {
        writeError(w, http.StatusForbidden, "csrf_required", "CSRF cookie required for all state-changing operations")
        return
    }
}
```
**CWE:** CWE-352 (Cross-Site Request Forgery)
**Impact:** CSRF attacks possible via JSON API calls
**Remediation:** CSRF cookie mandatory for all state-changing operations

---

#### H-002: SSRF via Upstream Hostname Resolution (validateUpstreamHost)
**File:** `internal/gateway/proxy.go`
**Status:** FIXED (was vulnerable)
```go
// BEFORE: Hostnames were allowed without DNS resolution validation
// AFTER: Resolve hostname and validate each resolved IP
addrs, err := net.LookupHost(h)
for _, addr := range addrs {
    if err := validateResolvedIP(addr, host); err != nil {
        return err
    }
}

// validateResolvedIP blocks:
// - Link-local (169.254.0.0/16) including metadata endpoint 169.254.169.254
// - Unspecified addresses
// - Private IPs when deny_private_upstreams enabled
// - Loopback
```
**CWE:** CWE-918 (Server-Side Request Forgery)
**Impact:** Could reach cloud metadata, internal services
**Remediation:** DNS resolution with validation against blocklist

---

#### H-003: SSRF in Federation Subgraph URL Validation (SSRF Fix)
**File:** `internal/federation/subgraph.go`
**Status:** FIXED (was partial)
```go
// BEFORE: Only validated literal IPs
// AFTER: Resolve hostname and validate each resolved IP
addrs, err := net.LookupHost(host)
for _, addr := range addrs {
    if err := validateSubgraphIP(net.ParseIP(addr), host); err != nil {
        return err
    }
}
```
**CWE:** CWE-918 (Server-Side Request Forgery)
**Impact:** SSRF via DNS re-binding or hostname to internal IP
**Remediation:** DNS resolution validation added

---

#### H-004: X-Real-IP Header Not Validated (M-003)
**File:** `internal/pkg/netutil/clientip.go`
**Status:** FIXED (was vulnerable)
```go
// BEFORE: X-Real-IP used directly without validation
xri := r.Header.Get("X-Real-Ip")
return strings.TrimSpace(xri)

// AFTER: Validate IP format before using
trimmed := strings.TrimSpace(xri)
if net.ParseIP(trimmed) != nil {
    return trimmed
}
```
**CWE:** CWE-20 (Improper Input Validation)
**Impact:** Malicious trusted proxy could spoof X-Real-IP
**Remediation:** IP format validation added

---

#### H-005: CORS Wildcard Origin Misconfiguration (CWE-942)
**File:** `internal/plugin/cors.go`
**Status:** FIXED (was misconfigured)
```go
// BEFORE: Wildcard allowed without credentials
// AFTER: Reject wildcard origins entirely
if allowAllOrigins {
    allowAllOrigins = false
    cfg.AllowCredentials = false
}
```
**CWE:** CWE-942 (Permissive Cross-Domain Whitelist)
**Impact:** Any site could read responses (data theft)
**Remediation:** Reject wildcard origins regardless of credentials setting

---

#### H-006: JWT IAT (Issued At) Claim Not Validated
**File:** `internal/plugin/auth_jwt.go`
**Status:** FIXED (was missing)
```go
// AFTER: Reject tokens with future iat
if iatUnix, ok := token.ClaimUnix("iat"); ok {
    iat := time.Unix(iatUnix, 0)
    if now.Before(iat.Add(-a.clockSkew)) {
        return &JWTAuthError{Message: "jwt iat claim is in the future"}
    }
}
```
**CWE:** CWE-613 (Insufficient Session Expiration)
**Impact:** Tickets with future timestamps could be accepted
**Remediation:** IAT validation with clock skew allowance

---

#### H-007: JWT JTI Replay Protection Disabled Without Warning
**File:** `internal/plugin/auth_jwt.go`
**Status:** DOCUMENTED (was silent)
```go
// AFTER: Warning logged when replay cache not configured
fmt.Printf("WARN: JTI replay cache not configured, replay protection disabled for token\n")
```
**Impact:** Silent security degradation in production
**Remediation:** Explicit warning logged; production should configure Redis-backed JTI cache

---

### MEDIUM Severity (6)

#### M-001: API Key Minimum Length Not Enforced (M-015)
**File:** `internal/config/load.go`
**Status:** FIXED (was not enforced)
```go
// AFTER: Enforce minimum key length for entropy
if strings.HasPrefix(key.Key, "ck_live_") && keyLen < 32 {
    addErr("consumer %q api_keys[%d].key is too short (live key requires >= 32 chars)")
}
if strings.HasPrefix(key.Key, "ck_test_") && keyLen < 16 {
    addErr("consumer %q api_keys[%d].key is too short (test key requires >= 16 chars)")
}
```
**CWE:** CWE-326 (Inadequate Encryption Strength)
**Impact:** Low-entropy keys could be brute-forced
**Remediation:** Minimum length enforcement at config validation

---

#### M-002: Kafka TLS SkipVerify Allowed in Production
**File:** `internal/config/load.go`
**Status:** FIXED (was allowed)
```go
// AFTER: Reject insecure skip-verify in production
if cfg.Kafka.TLS.Enabled && cfg.Kafka.TLS.SkipVerify {
    addErr("kafka.tls.skip_verify is insecure and must not be used in production")
}
```
**CWE:** CWE-295 (Improper Certificate Validation)
**Impact:** MITM attacks on Kafka traffic
**Remediation:** Validation rejects skip_verify in production

---

#### M-003: Session Cookie SameSite=Lax (L-006)
**File:** `internal/admin/token.go`
**Status:** FIXED
```go
// BEFORE
SameSite: http.SameSiteLaxMode

// AFTER
SameSite: http.SameSiteStrictMode
```
**CWE:** CWE-16 (Cross-Site Request Forgery)
**Impact:** CSRF possible on GET-triggered state changes
**Remediation:** Strict mode blocks all cross-site requests

---

#### M-004: Refresh Token Storage Not Hashed
**File:** `internal/admin/oidc_provider.go`
**Status:** FIXED (was plaintext)
```go
// AFTER: Store SHA-256 hash of refresh token
rtHash := sha256.Sum256([]byte(refreshToken))
s.oidcProvider.refreshTokens[string(rtHash[:])] = &refreshTokenEntry{...}
```
**CWE:** CWE-256 (Plaintext Storage of Password)
**Impact:** Refresh tokens readable in memory dumps
**Remediation:** SHA-256 hashed storage with 7-day TTL

---

#### M-005: Refresh Token One-Time Use Not Enforced
**File:** `internal/admin/oidc_provider.go`
**Status:** FIXED (was reusable)
```go
// AFTER: Delete refresh token after use (one-time use)
entry, exists := s.oidcProvider.refreshTokens[string(rtHash[:])]
if exists && time.Now().Before(entry.Expiry) && entry.ClientID == clientID {
    delete(s.oidcProvider.refreshTokens, string(rtHash[:])) // One-time use
}
```
**CWE:** CWE-287 (Improper Authentication)
**Impact:** Token replay attacks possible
**Remediation:** One-time use enforcement with deletion

---

#### M-006: Billing Amount Exposure in Audit Log (L-002)
**File:** `internal/billing/engine.go`
**Status:** FIXED (was exposed)
```go
// AFTER: Zero amount to avoid exposing transaction delta
&store.CreditTransaction{
    Amount:        0, // Zero amount
    BalanceBefore: newBalance,
    BalanceAfter:  newBalance,
    Description:   "request charge",
}
```
**CWE:** CWE-532 (Information Exposure Through Log Files)
**Impact:** Financial data leaked in audit logs
**Remediation:** Zero amount; description only

---

### LOW Severity / Documentation (5)

#### L-001: Kafka Audit Export Body Data Exposure (L-003)
**File:** `internal/audit/kafka.go`
**Status:** FIXED (was exported)
```go
// AFTER: Clear bodies before Kafka export
entryForKafka.RequestBody = ""
entryForKafka.ResponseBody = ""
```
**Note:** Bodies already masked locally, but Kafka has broader access
**Remediation:** Bodies cleared; only metadata sent to Kafka

---

#### L-002: Kubernetes Secret Placeholder Values (Security Note)
**File:** `deployments/kubernetes/base/secret.yaml`
**Status:** DOCUMENTED
```yaml
# BEFORE: Placeholder values that might be deployed
jwt-secret: "CHANGE_ME_IN_PRODUCTION"
admin-api-key: "CHANGE_ME_IN_PRODUCTION"

# AFTER: No placeholder values; must be provided externally
# NOTE: This base secret.yaml intentionally contains no real values.
```
**CWE:** CWE-547 (Use of Hard-Coded, Security-relevant Constants)
**Remediation:** Documentation improvement; external secret management required

---

#### L-003: Prometheus Admin Metrics Scraping (H-014)
**File:** `deployments/monitoring/prometheus/prometheus.yml`
**Status:** DOCUMENTED
```yaml
# AFTER: Admin metrics scraping commented out
# NOTE: Admin metrics endpoint (port 9876) should NOT be scraped by
# Prometheus in production. It contains sensitive operational data.
```
**CWE:** CWE-200 (Information Exposure)
**Remediation:** Documentation; network-level access controls recommended

---

#### L-004: Admin API Ingress Exposure (Security Note)
**File:** `deployments/kubernetes/base/ingress.yaml`
**Status:** DOCUMENTED
```yaml
# NOTE: Admin API (port 9876) should NOT be exposed via ingress.
# Access admin only via VPN/bastion host or through dedicated admin ingress
# with network-level access controls (IP allow-list at load balancer).
```
**CWE:** CWE-284 (Improper Access Control)
**Remediation:** Documentation; proper network isolation required

---

#### L-005: CI Production Deployment Manual Approval (H-013)
**File:** `.github/workflows/ci.yml`
**Status:** IMPROVED
```yaml
# AFTER: Explicit environment protection configuration
environment:
  name: production
  url: https://api.example.com
# Requires GitHub Environment "production" to have protection rules configured
```
**CWE:** CWE-284 (Improper Access Control)
**Remediation:** Documentation clarifies manual approval requirement

---

### Web Security Notes (3)

#### W-001: Admin API CSRF Documentation (M-021)
**File:** `web/src/lib/api.ts`
**Status:** DOCUMENTED
```typescript
// M-021: Admin API CSRF protection.
// The X-Admin-Key header acts as bearer token — ensure never exposed in URLs or logs.
// Browser XSS can still exfiltrate auth state.
```
**CWE:** CWE-352 (Cross-Site Request Forgery)
**Note:** Acceptable for API clients; browser XSS remains attack vector

---

#### W-002: sessionStorage XSS Risk Documentation (M-022)
**File:** `web/src/lib/api.ts`
**Status:** DOCUMENTED
```typescript
// M-022: sessionStorage is accessible to any JS on same origin.
// For production: use httpOnly cookies for auth tokens.
```
**CWE:** CWE-79 (Cross-site Scripting)
**Note:** Should migrate to httpOnly cookies in future

---

#### W-003: WebSocket Origin Validation Documentation (M-023)
**File:** `web/src/lib/ws.ts`
**Status:** DOCUMENTED
```typescript
// M-023: WebSocket connections subject to Same-Origin Policy.
// Backend should validate Origin header and reject untrusted origins.
```
**CWE:** CWE-20 (Improper Input Validation)
**Note:** Backend enforcement required

---

## Phase 3: Verification

### True Positives (Confirmed Vulnerabilities Fixed): 16
- C-001, C-002, C-003 (Critical auth fixes)
- H-001 through H-007 (High severity fixes)
- M-001 through M-006 (Medium severity fixes)

### Security Improvements (Not Vulnerabilities): 8
- L-001 through L-005 (Documentation/improvements)
- W-001 through W-003 (Web security documentation)
- H-013, H-014 (CI/Kubernetes improvements)

### Risk Reductions: 3
- SSRF protections added (H-002, H-003)
- CORS hardening (H-005)
- RBAC default deny (C-003)

---

## Phase 4: Report Summary

### CVSS Scores (Where Applicable)

| Finding | CVSS Base Score | Severity | Vector |
|---------|-----------------|----------|--------|
| C-001: OIDC Sig Verify | 9.1 | CRITICAL | AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N |
| C-002: OIDC Audience | 7.5 | HIGH | AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:H/A:N |
| C-003: RBAC Default Allow | 8.1 | HIGH | AV:N/AC:L/PR:H/UI:N/S:U/C:H/I:H/A:N |
| H-001: Portal CSRF | 8.8 | HIGH | AV:N/AC:L/PR:L/UI:R/S:U/C:H/I:H/A:N |
| H-002: SSRF Upstream | 8.1 | HIGH | AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N |
| H-003: SSRF Federation | 8.1 | HIGH | AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:N |
| H-004: X-Real-IP Spoof | 5.3 | MEDIUM | AV:N/AC:L/PR:N/UI:R/S:U/C:N/I:L/A:N |
| H-005: CORS Wildcard | 7.5 | HIGH | AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:H/A:N |
| H-006: JWT IAT | 5.3 | MEDIUM | AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:L/A:N |

---

## Conclusion

The uncommitted diff demonstrates **proactive security hardening** across multiple attack vectors:

**Strong Areas:**
- Authentication fixes (JWT validation, OIDC security)
- Authorization hardening (RBAC default deny)
- SSRF protections (DNS resolution validation)
- Data exposure reduction (Kafka bodies, billing amounts)

**Areas Requiring Future Attention:**
- Web sessionStorage XSS risk (W-002) - migrate to httpOnly cookies
- WebSocket origin validation (W-003) - requires backend implementation
- JTI replay cache (H-007) - requires Redis configuration in production

**No new vulnerabilities introduced.** All changes are security improvements.

---

*Report generated by security-check skill (scan diff)*
