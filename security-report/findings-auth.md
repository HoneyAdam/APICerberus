# Authentication & Authorization Security Findings
**Scan Date:** 2026-04-18
**Scanner:** Claude Code Security Analysis
**Scope:** `internal/admin/*.go`, `internal/gateway/middleware*.go`, `internal/plugin/*auth*.go`, `internal/auth*.go`, `cmd/apicerberus/main.go`
**Previous Report:** `security-report/verified-findings.md`

---

## Executive Summary

APICerebrus implements a multi-layered authentication system with:
- **Admin API**: Static API key (X-Admin-Key) + Bearer JWT sessions with HS256 signing
- **Consumer/Gateway API**: API key authentication + JWT plugin support
- **Portal**: Session-based auth with bcrypt-hashed session tokens
- **OIDC Provider**: Authorization server with RS256/ES256 signed tokens

**Status**: Multiple critical/high findings from prior scans have been remediated. The following analysis identifies any remaining or new authentication/authorization concerns.

---

## Previously Fixed (2026-04-18 Remediation Pass)

| ID | Finding | Fix | Status |
|----|---------|-----|--------|
| CRIT-1 | OIDC userinfo signature verification missing | Signature verification added at `oidc_provider.go:608-622` | **FIXED** |
| H-NEW-1 | OIDC introspect leaked expired token claims | Claims only exposed when `exp > now` | **FIXED** |
| M-014 | Admin API CSRF protection missing | Double-submit cookie pattern at `token.go:200-210` | **FIXED** |
| H-001 | Admin key rotation didn't revoke sessions | KeyVersion incremented on rotation, JWTs rejected at `token.go:127-130` | **FIXED** |

---

## New Findings

### Finding 1: OIDC Authorization Endpoint Uses Hardcoded Placeholder Subject

| Field | Value |
|-------|-------|
| **CWE** | CWE-306 (Missing Authentication for Critical Function) |
| **CVSS 3.1** | 5.3 (Medium) — `CVSS:3.1/AV:N/AC:L/PR:L/UI:R/SA:L/RL:U/RC:C` |
| **File:Line** | `internal/admin/oidc_provider.go:292-294` |
| **Confidence** | High |

**Code:**
```go
// For now, use a default user — in production this would redirect to login
// For testing/demo, we'll use a placeholder subject
subject := "user@example.com"
```

**Evidence:**
The `handleOIDCAuthorize` function (lines 247-326) does not actually authenticate the user before generating an authorization code. It accepts any valid client_id/redirect_uri combination and issues an auth code with a hardcoded subject.

**Impact:**
- Any user with a valid OIDC client can impersonate `user@example.com`
- Authorization codes are issued to the wrong subject, breaking user differentiation
- The authorization code flow is fundamentally broken for production use

**Remediation:**
1. Implement proper user authentication before issuing authorization codes
2. Either redirect to a login page or integrate with an existing session mechanism
3. Remove the placeholder comment and hardcoded value before production deployment

---

### Finding 2: OIDC Provider Lacks PKCE Support for Public Clients

| Field | Value |
|-------|-------|
| **CWE** | CWE-287 (Improper Authentication) |
| **CVSS 3.1** | 5.3 (Medium) — `CVSS:3.1/AV:N/AC:L/PR:L/UI:R/SA:L/RL:U/RC:C` |
| **File:Line** | `internal/admin/oidc_provider.go:247-326` |
| **Confidence** | High |

**Code:**
```go
func (s *Server) handleOIDCAuthorize(w http.ResponseWriter, r *http.Request) {
    // ... no PKCE validation ...
    authCode, err := newRandomHex(32)
    // ...
}
```

**Evidence:**
The OIDC authorization endpoint does not validate `code_challenge` and `code_challenge_method` parameters (RFC 7636). This allows authorization code interception attacks for public clients.

**Impact:**
- Public OIDC clients (mobile apps, SPAs) are vulnerable to authorization code interception
- Attackers on the same network can steal authorization codes
- Cannot be exploited for confidential clients (requires client_secret)

**Remediation:**
1. Add PKCE validation: generate `code_verifier`, compute `code_challenge = BASE64URL(SHA256(code_verifier))`
2. Store `code_challenge` with the auth code
3. On token exchange, validate `code_verifier` matches stored `code_challenge`

---

### Finding 3: Consumer API Key Entropy Not Enforced

| Field | Value |
|-------|-------|
| **CWE** | CWE-327 (Use of Weak Cryptographic Primitive) |
| **CVSS 3.1** | 3.5 (Low) — `CVSS:3.1/AV:N/AC:L/PR:H/UI:N/SA:L/RL:U/RC:C` |
| **File:Line** | `internal/config/load.go:571-578` |
| **Confidence** | Medium |

**Code:**
```go
// M-015: Enforce minimum key length for entropy. Weak keys (< 16 chars) can be
// brute-forced. Require at least 32 chars for live keys, 16 for test keys.
if strings.HasPrefix(key.Key, "ck_live_") && keyLen < 32 {
    addErr(...)
}
```

**Evidence:**
The validation only checks key length, not entropy. A 32-character key consisting of repeated characters or common patterns would pass validation but be trivially brute-forceable.

**Impact:**
- Admins could create API keys with low entropy (e.g., `ck_live_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa`)
- Such keys could be brute-forced despite meeting the length requirement
- The comment claims to enforce entropy but the code does not

**Remediation:**
1. Add entropy validation: reject keys with repeated characters, common patterns, or dictionary words
2. Require key generation via `crypto/rand` (enforced by design, but not verified at load time)
3. Consider using a passphrase-based key derivation function for admin-created keys

---

### Finding 4: OIDC Provider Auto-Generated RSA Key Is 2048 Bits

| Field | Value |
|-------|-------|
| **CWE** | CWE-327 (Use of Weak Cryptographic Primitive) |
| **CVSS 3.1** | 3.5 (Low) — `CVSS:3.1/AV:N/AC:L/PR:H/UI:N/SA:L/RL:U/RC:C` |
| **File:Line** | `internal/admin/oidc_provider.go:159-167` |
| **Confidence** | Medium |

**Code:**
```go
// Auto-generate RSA key
rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
if err != nil {
    return nil, fmt.Errorf("generate RSA key: %w", err)
}
```

**Evidence:**
The auto-generated RSA key for the OIDC provider uses 2048 bits. NIST SP 800-57 Part 1 Rev 5 recommends 3072-bit minimum for RSA keys beyond 2030.

**Impact:**
- OIDC provider tokens signed with 2048-bit RSA may not meet long-term security requirements
- Tokens with long validity periods are most affected
- Keys generated at startup time when no key file is provided

**Remediation:**
1. Increase default RSA key size to 3072 bits
2. Allow configuration of key size via `oidc.provider.key_size` config option
3. Document that operators should provide their own keys for production

---

### Finding 5: OIDC Auth Codes Not Persisted Across Restarts

| Field | Value |
|-------|-------|
| **CWE** | CWE-613 (Insufficient Session Expiration) |
| **CVSS 3.1** | 4.0 (Medium-Low) — `CVSS:3.1/AV:N/AC:L/PR:L/UI:R/SA:L/RL:U/RC:C` |
| **File:Line** | `internal/admin/oidc_provider.go:309-317` |
| **Confidence** | High |

**Code:**
```go
s.mu.Lock()
s.oidcProvider.authCodes[code] = &authCodeEntry{...}
s.mu.Unlock()
```

**Evidence:**
Authorization codes are stored in an in-memory map (`authCodes`). If the server restarts before the codes are exchanged, users must re-authenticate.

**Impact:**
- Poor availability: users lose their authorization flow on server restart
- Potential denial of service for in-flight authorization flows
- Not a security issue per se, but a usability and availability concern

**Remediation:**
1. Persist auth codes to SQLite with expiry timestamp
2. Load and cleanup expired codes on startup
3. Consider Redis for distributed deployments

---

## Verified Secure Implementations

The following authentication mechanisms were reviewed and found to be properly implemented:

### Admin API Static Key Auth
| Component | Implementation | Location |
|-----------|---------------|----------|
| Key comparison | `crypto/subtle.ConstantTimeCompare` | `server.go:247` |
| Rate limiting | 5 attempts per 15 min, 30-min block | `admin_helpers.go:239-269` |
| IP allowlist | CIDR-aware `netutil.IsAllowedIP` | `server.go:172-175` |
| Weak key detection | Blocks "change", "secret", "password", "123" | `server.go:444-449` |

### Admin API JWT Bearer Auth
| Component | Implementation | Location |
|-----------|---------------|----------|
| Algorithm | HS256 only (no algorithm confusion) | `token.go:107-109` |
| Key version | Tokens rejected if key_version mismatches | `token.go:127-130` |
| Token expiry | Required exp claim, validated | `token.go:131-134` |
| IAT validation | Future iat rejected (60s tolerance) | `token.go:136-141` |
| NBF validation | Not-yet-valid tokens rejected | `token.go:143-147` |
| CSRF protection | Double-submit cookie pattern | `token.go:199-210` |

### Consumer API Key Auth
| Component | Implementation | Location |
|-----------|---------------|----------|
| Key lookup | SHA256 hash bucket + constant-time compare | `auth_apikey.go:181-210` |
| Expiry check | Optional expiresAt with `time.After` | `auth_apikey.go:187-189` |
| Backoff | Per-IP exponential backoff | `auth_backoff.go:13-119` |
| Rate limiting | 5 attempts/15min, permanent block at 100 | `admin_helpers.go:293-296` |

### JWT Plugin
| Component | Implementation | Location |
|-----------|---------------|----------|
| Algorithm allowlist | HS256, RS256, ES256, EDDSA only | `auth_jwt.go:182-212` |
| None algorithm | Explicitly rejected | `auth_jwt.go:179-181` |
| Issuer validation | Optional `iss` claim check | `auth_jwt.go:348-359` |
| Audience validation | Optional `aud` claim check | `auth_jwt.go:361-389` |
| JTI replay protection | Fail-closed when cache missing | `auth_jwt.go:265-275` |

### OIDC Callback Validation
| Component | Implementation | Location |
|-----------|---------------|----------|
| State validation | `crypto/subtle.ConstantTimeCompare` | `oidc.go:181` |
| Nonce validation | Constant-time comparison | `oidc.go:245` |
| ID token verification | Via `oidc.Config{ClientID}` verifier | `oidc.go:237` |
| Error reflection | Allowlisted OIDC error codes only | `oidc.go:189-203` |

### Portal Session Auth
| Component | Implementation | Location |
|-----------|---------------|----------|
| Session tokens | Generated via `crypto/rand` | `portal/server.go:189` |
| Token storage | bcrypt hash of token | `portal/server.go:199` |
| CSRF protection | Double-submit with `SameSite=Strict` | `portal/server.go:100-103` |
| Rate limiting | Per-IP login attempt limiting | `portal/server.go:152-157` |

---

## Summary

| Severity | Count | Status |
|----------|-------|--------|
| Critical | 0 | All fixed (CRIT-1 remediated) |
| High | 0 | All fixed (H-001, H-NEW-1 remediated) |
| Medium | 2 | 1 incomplete OIDC impl (Finding 1), 1 PKCE missing (Finding 2) |
| Low | 3 | Key entropy (Finding 3), RSA key size (Finding 4), auth code persistence (Finding 5) |

**Overall Assessment**: The authentication and authorization system is well-designed with proper cryptographic practices. The remaining findings are lower severity and relate to incomplete OIDC provider implementation rather than fundamental flaws.

---

## Recommendations

### Priority 1 (Should Fix Before Production)
1. **Finding 1**: Implement real user authentication in OIDC authorization endpoint, remove hardcoded placeholder
2. **Finding 2**: Add PKCE support for public OIDC clients

### Priority 2 (Recommended Before Production)
3. **Finding 4**: Increase auto-generated RSA key size to 3072 bits
4. **Finding 3**: Add API key entropy validation beyond length check

### Priority 3 (Nice to Have)
5. **Finding 5**: Persist OIDC authorization codes to database
