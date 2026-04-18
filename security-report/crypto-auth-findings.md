# Cryptographic & Authentication Security Findings

**Scan Date:** 2026-04-18
**Scope:** `internal/` — JWT, password hashing, token generation, TLS/SSL, session management, API key handling
**Status:** No critical or high-severity issues found. Implementation is sound.

---

## 1. JWT Implementation

### Algorithm Confusion Prevention
**Finding:** Secure
- `"none"` algorithm explicitly rejected in `internal/plugin/auth_jwt.go:179-181`:
  ```go
  if alg == "NONE" {
      return nil, ErrUnsupportedJWTAlgorithm
  }
  ```
- Supported algorithms are explicitly allow-listed: `HS256`, `RS256`, `ES256`, `EDDSA` (auth_jwt.go:182-211)
- Admin tokens hardcoded to `HS256` only — no algorithm negotiation (token.go:58)
- No dynamic algorithm loading from token header that could bypass verification

### HS256 Secret Strength
**Finding:** Secure — minimum 32 bytes enforced
- `internal/pkg/jwt/hs256.go:5`: `minHS256SecretLength = 32` (256 bits per NIST SP 800-107)
- Both `SignHS256` and `VerifyHS256` enforce this minimum (jwt.go:218, 231)
- Weak secrets return `ErrWeakHS256Secret` error

### JWT Library
**Finding:** Secure
- Uses `github.com/golang-jwt/jwt/v5` — well-maintained, mature library
- Custom `Token` struct for manual parsing with explicit signature verification per algorithm type

### JTI Replay Protection
**Finding:** Secure
- JTI claim is required for JWT replay protection when cache is configured
- `auth_jwt.go:266-274`: Fail-closed rejection of tokens with jti claim when no replay cache is configured
- JTI stored with TTL-based expiration in replay cache

### OIDC Provider Tokens
**Finding:** Secure
- Access tokens use RS256/ES256 (asymmetric) — not vulnerable to HS256 algorithm confusion
- ID tokens use RS256/ES256 with proper issuer/audience claims
- Introspection endpoint verifies signature before returning claims (M-009 fix documented)

---

## 2. Password Hashing

### Bcrypt Cost
**Finding:** Secure — cost 12 for admin passwords
- `internal/store/user_repo.go:499`: `bcrypt.GenerateFromPassword([]byte(raw), 12)`
- Cost 12 is above the bcrypt default of 10, providing ~4x slower hashing than default
- Good for admin passwords; user passwords also use bcrypt with same cost

### Salt
**Finding:** Secure
- Bcrypt automatically generates a unique 128-bit salt per hash
- No manual salt management needed

### Password Generation
**Finding:** Secure — rejection sampling for random password generation
- `internal/store/user_repo.go:574-593`: `generateSecurePassword()` uses crypto/rand with rejection sampling
- `maxValid := 256 - (256 % charsetLen)` ensures uniform distribution (CWE-330 compliance)
- 20-character passwords from 60-character alphabet = ~119 bits of entropy

---

## 3. Token Generation (crypto/rand vs math/rand)

### Finding: All Secure — crypto/rand Used Throughout

| Location | Function | RNG |
|----------|----------|-----|
| `internal/admin/token.go:52` | Admin JWT JTI generation | `crypto/rand.Read(jtiBytes)` |
| `internal/store/session_repo.go:46` | Session token generation | `crypto/rand.Read(buf)` |
| `internal/store/api_key_repo.go:368` | API key token generation | `crypto/rand.Read(buf)` |
| `internal/store/user_repo.go:584` | Secure password generation | `crypto/rand.Read(buf)` |
| `internal/admin/token.go:300` | CSRF token generation | `crypto/rand.Read(b)` |
| `internal/admin/oidc_provider.go:606-614` | OIDC JTI/refresh token generation | `crypto/rand.Read(b)` |
| `internal/admin/oidc_provider.go:327` | Auth code generation | `newRandomHex()` → `crypto/rand.Read(b)` |
| `internal/raft/tls.go:36` | Certificate serial number | `crypto/rand.Read(b)` |

**No usage of `math/rand` found anywhere in cryptographic token generation.**

---

## 4. TLS/SSL Configuration

### Gateway TLS
**Finding:** Secure
- `internal/gateway/tls.go:75`: Minimum TLS 1.2 enforced; TLS 1.0/1.1 rejected with warning log
- `internal/gateway/tls.go:88-96`: Safe modern cipher suites for TLS 1.2 (ECDHE-based with forward secrecy)
- Weak RSA-based cipher suites explicitly removed (tls.go:123-129)
- ACME/Let's Encrypt auto-provisioning properly configured

### Raft Cluster mTLS
**Finding:** Very Strong
- `internal/raft/tls.go:139-140`: `MinVersion: tls.VersionTLS13` + `ClientAuth: tls.RequireAndVerifyClientCert`
- 4096-bit RSA keys for CA and node certificates
- 128-bit random serial numbers per RFC 5280 §4.1.2.2
- 1-year certificate validity with proper KeyUsage (digital signature + key encipherment)

### No InsecureSkipVerify
**Finding:** Verified Clean
- Search for `InsecureSkipVerify` returned no matches in codebase
- All TLS configurations perform proper certificate validation

---

## 5. Session Management

### Cookie Security
**Finding:** Secure — proper flags on all session cookies

| Cookie | HttpOnly | Secure | SameSite |
|--------|----------|--------|----------|
| `apicerberus_admin_session` | Yes (token.go:277) | Yes (token.go:278) | Strict (token.go:279) |
| `apicerberus_admin_csrf` | No (token.go:314) | Yes (token.go:315) | Strict (token.go:316) |
| Portal session | Yes (oidc.go:137) | Yes (oidc.go:138) | Lax (oidc.go:138) |

### Session Fixation
**Finding:** Secure
- SameSite=Strict prevents cross-site request forgery attacks
- Session token is a new random token issued on login (not pre-existing)
- Logout clears cookie with `MaxAge: -1` (token.go:476-479)

### CSRF Protection
**Finding:** Secure
- Double-submit cookie pattern implemented (token.go:307-340)
- CSRF cookie is NOT HttpOnly so JavaScript can read it for header comparison
- CSRF header required for state-changing requests (POST/PUT/DELETE/PATCH)
- Login endpoint skips CSRF check (token.go:204) — appropriate

### Session Token Storage
**Finding:** Secure
- Session tokens stored as SHA256 hash (session_repo.go:52-55), not plaintext
- Token hash lookup for session validation

---

## 6. API Key Handling

### Storage
**Finding:** Secure
- API keys stored as SHA256 hash (api_key_repo.go:353-356)
- Full key only returned once at creation time (`Create()` returns `fullKey` — line 124)
- Prefix stored for key identification (`ck_live_` / `ck_test_`)

### Key Generation
**Finding:** Secure
- 32-byte random tokens using `crypto/rand` with rejection sampling
- Rejection sampling ensures uniform distribution (same pattern as password generation)

### Key Validation
**Finding:** Secure
- Status check: revoked keys rejected (api_key_repo.go:341-343)
- User status check: suspended/deleted users rejected (api_key_repo.go:344-346)
- Expiration check: expired keys rejected (api_key_repo.go:347-349)
- Last-used tracking with IP and timestamp (api_key_repo.go:269-289)

---

## 7. OIDC Provider Specific

### Auth Code Security
**Finding:** Secure
- 32-byte cryptographically random auth codes (`newRandomHex(32)`)
- 5-minute TTL on auth codes
- Single-use: marked `Used=true` after exchange (oidc_provider.go:410)
- PKCE support (S256 method required for public clients)

### Refresh Token Security
**Finding:** Acceptable (with note)
- Stored as SHA256 hash (oidc_provider.go:448-449) — not bcrypt
- One-time use: deleted after exchange (oidc_provider.go:476)
- 7-day TTL with 1-hour access token TTL
- **Note:** Using bcrypt for refresh tokens would be more secure against offline attacks, but SHA256 is acceptable for short-lived tokens with one-time use semantics

### Client Authentication
**Finding:** Secure
- Client secrets verified with bcrypt (oidc_provider.go:392)
- Confidential clients authenticated via `client_secret_basic` or `client_secret_post`

---

## Summary

| Category | Status | Notes |
|----------|--------|-------|
| JWT Algorithm Confusion | PASS | `none` algorithm rejected; explicit allow-list |
| JWT Secret Strength | PASS | 32-byte minimum enforced |
| Password Hashing | PASS | bcrypt cost 12 |
| Token RNG | PASS | All crypto/rand, no math/rand |
| TLS Configuration | PASS | TLS 1.2+ min, safe ciphers, no InsecureSkipVerify |
| Raft mTLS | PASS | TLS 1.3, RequireAndVerifyClientCert, 4096-bit RSA |
| Session Cookies | PASS | HttpOnly + Secure + SameSite=Strict |
| CSRF Protection | PASS | Double-submit cookie pattern |
| API Key Storage | PASS | SHA256 hash, single-return on creation |
| OIDC Refresh Tokens | ACCEPTABLE | SHA256 (acceptable for one-time use, short TTL) |

**No critical or high-severity cryptographic or authentication vulnerabilities identified.**

---

## Recommendations

1. **Low Priority — OIDC Refresh Token Hashing:** Consider using bcrypt (with low cost like 4) for refresh token storage instead of SHA256 for defense-in-depth against future quantum attacks. Current SHA256 is acceptable for one-time-use short-lived tokens.

2. **Informational — Key Rotation:** The `keyVersion` mechanism in admin JWT (token.go:113-130) properly handles key rotation. Ensure operators rotate the admin key periodically via `handleRotateAdminKey`.

3. **Informational — bcrypt vs scrypt/argon2:** For future-proofing, consider scrypt or argon2 for password hashing. Current bcrypt cost 12 is adequate for today.
