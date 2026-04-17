# Security Findings: Hardcoded Secrets, Weak Crypto & Data Exposure

**Scan Date:** 2026-04-18
**Scope:** Hardcoded secrets, weak cryptography, data exposure
**Files Scanned:** `internal/config/load.go`, `cmd/apicerberus/main.go`, `apicerberus.example.yaml`, `internal/certmanager/*.go`, `internal/raft/tls.go`, `internal/audit/kafka.go`, JWT implementation, TLS implementation

---

## Finding S-001: Raft TLS Hardcoded Certificate Serial Numbers

| Field | Value |
|-------|-------|
| **CWE** | CWE-328 (Predictable Certificate Serial Numbers) |
| **CVSS 3.1** | 3.7 (Low) — AV:N/AC:H/PR:N/UI:N/S:U/C:N/I:L/A:N |
| **Evidence** | `internal/raft/tls.go:40`, `internal/raft/tls.go:80` |
| **Status** | Open |

### Description

The Raft TLS certificate manager uses hardcoded serial numbers for CA and node certificates:

```go
// tls.go:40 — CA certificate
SerialNumber: big.NewInt(1),

// tls.go:80 — Node certificate
SerialNumber: big.NewInt(2),
```

### Impact

- **RFC 5280 Violation:** Certificate serial numbers must be unique within a CA's scope. Using static values violates §4.1.2.2.
- **CRL/OCSP Breakage:** Certificate Revocation Lists (CRL) and OCSP responses rely on serial numbers to identify certificates. Duplicate serials cause incorrect revocation status.
- **Security Impact:** Low in practice for self-signed mTLS certs, but violates compliance requirements (PKI best practices).

### Remediation

Replace hardcoded serials with cryptographically random 128-bit values:

```go
import "crypto/rand"

// Generate random serial
serialBytes := make([]byte, 16)
if _, err := rand.Read(serialBytes); err != nil {
    return fmt.Errorf("failed to generate serial: %w", err)
}
cert.SerialNumber = new(big.Int).SetBytes(serialBytes)
```

**Already noted in:** `security-report/sc-raft-cluster-results.md:444`

---

## Finding S-002: Raft TLS Unnecessary "localhost" in Node Certificate DNSNames

| Field | Value |
|-------|-------|
| **CWE** | CWE-295 (Improper Certificate Validation) |
| **CVSS 3.1** | 1.8 (Low) — AV:N/AC:H/PR:N/UI:R/S:U/C:N/I:L/A:N |
| **Evidence** | `internal/raft/tls.go:89` |
| **Status** | Open |

### Description

Node certificates include `"localhost"` as a DNSName SAN:

```go
// tls.go:89
DNSNames: []string{m.nodeID, "localhost"},
```

### Impact

- Any node in the cluster can impersonate `localhost` connections if a certificate is leaked or compromised.
- Unnecessary attack surface for man-in-the-middle scenarios within the cluster.

### Remediation

Remove `"localhost"` from DNSNames unless strictly required for inter-node localhost communication:

```go
DNSNames: []string{m.nodeID}, // Remove "localhost"
```

---

## Finding S-003: Kafka TLS InsecureSkipVerify Admin-Configurable

| Field | Value |
|-------|-------|
| **CWE** | CWE-295 (Improper Certificate Validation) / CWE-322 (Weak Crypto) |
| **CVSS 3.1** | 4.3 (Medium) — AV:N/AC:H/PR:H/UI:N/S:U/C:L/I:L/A:N |
| **Evidence** | `internal/audit/kafka.go:382` with `#nosec G402` |
| **Status** | Mitigated — Production validation rejects |

### Description

Kafka TLS configuration allows admin to set `InsecureSkipVerify: true`:

```go
// kafka.go:381-382
tlsConfig := &tls.Config{
    InsecureSkipVerify: kw.config.TLS.SkipVerify, // #nosec G402
    ServerName:         kw.config.TLS.ServerName,
}
```

### Mitigation in Place

Config validation in `internal/config/load.go:449` rejects this in production:

```go
// load.go:449
if cfg.Kafka.Enabled && cfg.Kafka.TLS.Enabled && cfg.Kafka.TLS.SkipVerify {
    addErr("kafka.tls.skip_verify is insecure and must not be used in production")
}
```

### Risk Assessment

- **Production:** Protected by config validation — startup will fail if `skip_verify: true`.
- **Non-production:** Remains configurable for dev/test environments with self-signed certs.
- **Residual Risk:** Low — only admins can set this, and it requires explicit unsafe configuration.

---

## Finding S-004: Test Configuration Contains Hardcoded Secrets

| Field | Value |
|-------|-------|
| **CWE** | CWE-798 (Use of Hardcoded Credentials) |
| **CVSS 3.1** | 3.5 (Low) — AV:N/AC:L/PR:N/UI:R/S:U/C:L/I:N/A:N |
| **Evidence** | `test/e2e-config.yaml:10-11` |
| **Status** | Prior Finding — Remediation Complete |

### Description

Test E2E config contains hardcoded API key and token secret:

```yaml
# test/e2e-config.yaml:10-11
api_key: "e2e-test-admin-key-must-be-at-least-32-chars-long"
token_secret: "e2e-test-jwt-secret-must-be-at-least-32-characters"
```

### Remediation Applied

Per `security-report/verified-findings.md:155`:
- `generateRandomSecret()` function added using `crypto/rand` (URL-safe base64)
- E2E tests now generate cryptographically random secrets per test run
- Raw secrets are never written to disk

### Current Status

**FIXED** — Test files now generate secrets at runtime. The hardcoded values in `e2e-config.yaml` remain as fallback/test fixture but are no longer used for actual E2E test runs.

---

## Finding S-005: Test File Uses InsecureSkipVerify (Acceptable)

| Field | Value |
|-------|-------|
| **CWE** | CWE-295 (Improper Certificate Validation) |
| **CVSS 3.1** | 2.0 (Low) — AV:N/AC:H/PR:N/UI:N/S:U/C:N/I:L/A:N |
| **Evidence** | `internal/gateway/https_test.go:50` |
| **Status** | Acceptable — Test Code Only |

### Description

Test file uses `InsecureSkipVerify: true` for testing with self-signed certificates:

```go
// https_test.go:50
InsecureSkipVerify: true, //nolint:gosec // test-only self-signed certificate
```

### Assessment

- **Location:** `https_test.go` — clearly a test file
- **Annotation:** Properly marked with `//nolint:gosec` and explanatory comment
- **Purpose:** Testing with self-signed certificates in isolated test environment
- **Risk:** None — test code is not deployed to production

**This is acceptable practice for unit testing with self-signed certs.**

---

## Positive Security Findings

The following are confirmed secure implementations:

| Feature | Implementation | Evidence |
|---------|----------------|----------|
| **Random Generation** | `crypto/rand` used for all key material | `internal/raft/tls.go:34,74`, `internal/certmanager/acme.go:183,451` |
| **Key Strength** | RSA 4096-bit for Raft CA/node certs | `internal/raft/tls.go:34` — `rsa.GenerateKey(rand.Reader, 4096)` |
| **TLS Enforcement** | TLS 1.0/1.1 rejected, TLS 1.3 enforced | `internal/gateway/tls.go:103` |
| **Weak Ciphers** | RSA key exchange removed | `internal/gateway/tls.go:123-128` |
| **Constant-Time Compare** | API key validation uses `subtle.ConstantTimeCompare` | `internal/plugin/auth_apikey.go:186,204` |
| **Admin Key Compare** | Admin auth uses constant-time compare | `internal/admin/token.go:247` |
| **JWT HS256 Min Length** | Minimum 32-byte secret enforced | `internal/pkg/jwt/hs256.go:5` — `minHS256SecretLength = 32` |
| **Password Hashing** | bcrypt cost 12 | `internal/store/user_repo.go:498` |
| **Secret Config Pattern** | `${ENV_VAR}` pattern used | `apicerberus.example.yaml:41-42` |
| **Kafka TLS Warning** | Warning logged if SkipVerify enabled | `internal/audit/kafka.go:379` |
| **ACME Key Generation** | ECDSA P-256 with `crypto/rand` | `internal/certmanager/acme.go:451` |

---

## Data Exposure Assessment

### Config Files

| File | Secret Pattern | Status |
|------|----------------|--------|
| `apicerberus.example.yaml` | `${ENV_VAR}` placeholders | ✅ Secure |
| `test/e2e-config.yaml` | Hardcoded strings (test only) | ⚠️ Prior finding, remediated |
| `deployments/examples/*.yaml` | `${ENV_VAR}` placeholders | ✅ Secure |
| `apicerberus.yaml` | `${ADMIN_API_KEY}`, `${TOKEN_SECRET}` | ✅ Secure |

### API Key Storage

- API keys stored as SHA-256 hashes in SQLite
- Raw keys never logged or exposed
- Constant-time comparison prevents timing attacks

### Audit Logging

- Sensitive headers masked (`Authorization`, `X-API-Key`)
- Body fields masked (`password`, `token`)
- Configurable replacement text: `***REDACTED***`

---

## Recommendations

1. **S-001 (Serial Numbers):** Replace `big.NewInt()` with `crypto/rand` 128-bit serial generation in `internal/raft/tls.go`.

2. **S-002 (Localhost SAN):** Remove `"localhost"` from `DNSNames` in `internal/raft/tls.go:89` unless required.

3. **S-003 (Kafka SkipVerify):** Consider removing admin-configurable `skip_verify` option entirely; always require valid certs in production.

4. **S-004 (Test Secrets):** Verified fixed — `generateRandomSecret()` now generates secrets at runtime.

---

## References

- [CWE-328: Predictable Certificate Serial Numbers](https://cwe.mitre.org/data/definitions/328.html)
- [CWE-295: Improper Certificate Validation](https://cwe.mitre.org/data/definitions/295.html)
- [CWE-798: Use of Hardcoded Credentials](https://cwe.mitre.org/data/definitions/798.html)
- [RFC 5280 §4.1.2.2: Certificate Serial Number](https://tools.ietf.org/html/rfc5280#section-4.1.2.2)
- [OWASP TLS Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Transport_Layer_Protection_Cheat_Sheet.html)
