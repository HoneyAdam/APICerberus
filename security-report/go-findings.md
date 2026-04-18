# Go Security Scan Report - APICerebrus

**Scan Date:** 2026-04-18
**Scanner:** security-check (sc-lang-go)
**Scope:** internal/gateway/, internal/plugin/, internal/store/, internal/admin/, internal/raft/, internal/billing/
**Go Version:** 1.22+ (implied by math/rand/v2 usage)

---

## Executive Summary

The codebase demonstrates **strong security posture** overall. SQL injection protection is robust with parameterized queries throughout, cryptographic operations properly use `crypto/rand` and `crypto/subtle`, and HTTP servers have appropriate timeouts configured. A few medium and low severity issues were identified related to error handling and edge cases.

**Critical Findings:** 0
**High Findings:** 0
**Medium Findings:** 2
**Low Findings:** 1
**Informational:** 1

---

## Findings by Severity

### [MEDIUM] Ignored strconv.Atoi Errors in Pagination

- **Category:** Input Validation & Error Handling
- **Location:**
  - `internal/admin/admin_users.go:51-52`
  - `internal/admin/admin_billing.go:237-238`
  - `internal/admin/graphql.go:216-217`
- **Pattern Matched:** `limit, _ := strconv.Atoi(...)` and `offset, _ := strconv.Atoi(...)`
- **Description:** The `strconv.Atoi` conversion errors are silently discarded using the blank identifier. If a non-numeric string is passed for `limit` or `offset`, the value defaults to 0, which can cause unexpected behavior such as returning zero results or using default pagination limits inconsistently.

**Exploitability:** Medium - An attacker could craft requests with non-numeric `limit`/`offset` parameters. While the code has defaults (e.g., `limit = 50` when `limit <= 0` in `user_repo.go:220-221`), the error is silently swallowed, making debugging difficult and potentially causing subtle behavior differences.

**Remediation:** Check the error return value and return a 400 Bad Request with an appropriate error message when parsing fails:

```go
limit, err := strconv.Atoi(strings.TrimSpace(query.Get("limit")))
if err != nil {
    writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be numeric")
    return
}
```

- **Reference:** CWE-20 (Improper Input Validation), CWE-703 (Handler Dis不对Error Condition or Action)

---

### [MEDIUM] Dynamic Sort Column Concatenation in User List Query

- **Category:** SQL Injection (Defended)
- **Location:** `internal/store/user_repo.go:213-241`
- **Pattern Matched:** `ORDER BY ` + sortBy + ` ` + sortDir
- **Description:** The sort column and direction are concatenated into the SQL query string. While the `normalizeUserSortBy()` function uses an allowlist (only "email", "name", "updated_at", "credit_balance", defaulting to "created_at"), and `sortDir` is either "ASC" or "DESC" from a ternary, this pattern is risky.

**Exploitability:** Low - The allowlist prevents SQL injection, but the concatenation pattern could become a vulnerability if `normalizeUserSortBy` is modified incorrectly or if new columns are added without proper validation. The `sortDir` variable is hardcoded to "ASC" or "DESC" based on a boolean, so it's safe.

**Remediation:** Continue using the allowlist approach but ensure it's validated at the function entry point and document the security rationale. Consider using a prepared statement with column name validation:

```go
func normalizeUserSortBy(value string) string {
    allowed := map[string]struct{}{
        "email":          {},
        "name":           {},
        "updated_at":     {},
        "credit_balance": {},
        "created_at":     {},
    }
    if _, ok := allowed[strings.ToLower(strings.TrimSpace(value))]; ok {
        return value
    }
    return "created_at"
}
```

- **Reference:** CWE-89 (SQL Injection - Protected by Allowlist), SC-GO-143

---

### [LOW] Panic on crypto/rand Unavailability in Password Generation

- **Category:** Memory Safety / Error Handling
- **Location:** `internal/store/user_repo.go:585`
- **Pattern Matched:** `panic(fmt.Sprintf("crypto/rand unavailable: %v", err))`
- **Description:** The `generateSecurePassword()` function calls `panic()` if `crypto/rand.Read()` fails. While `crypto/rand` failure is extremely rare in practice (only possible in environments with no entropy source), panicking will crash the entire goroutine and could impact the service if this path is ever exercised.

**Exploitability:** Very Low - `crypto/rand` failures are essentially impossible on any system with a proper entropy source. However, in constrained environments (some embedded systems, containers without proper entropy configuration), this could cause issues.

**Remediation:** Return an error instead of panicking:

```go
func generateSecurePassword() (string, error) {
    // ... existing code ...
    if _, err := rand.Read(buf); err != nil {
        return "", fmt.Errorf("crypto/rand unavailable: %w", err)
    }
    // ... existing code ...
}
```

- **Reference:** CWE-248 (Uncaught Exception), SC-GO-091, SC-GO-393

---

### [INFO] math/rand/v2 Usage for Non-Cryptographic Purposes

- **Category:** Cryptography (Acceptable)
- **Location:**
  - `internal/analytics/engine.go:6` - math/rand
  - `internal/analytics/optimized_engine.go:4` - math/rand/v2
  - `internal/gateway/balancer_extra.go:6,214,761` - math/rand/v2 (load balancing)
  - `internal/raft/node.go:7,678` - math/rand/v2 (Raft election timeout jitter)
  - `internal/plugin/retry.go:5,132` - math/rand/v2 (retry backoff)
- **Pattern Matched:** `math/rand`, `math/rand/v2`
- **Description:** The codebase uses `math/rand` and `math/rand/v2` for non-security-sensitive operations like load balancing selection, retry backoff jitter, and Raft election timeout randomization. Code includes `#nosec G404` annotations acknowledging this.

**Assessment:** **ACCEPTABLE** - In Go 1.22+, `math/rand/v2` is automatically seeded from `crypto/rand` at program startup, making it suitable for non-cryptographic randomization. Load balancer selection, retry jitter, and election timeout randomization do not require cryptographically secure randomness. The security checklist (SC-GO-297) specifically notes this is acceptable for non-security purposes.

**Reference:** CWE-338 (Use of Cryptographically Weak PRNG), Go 1.22 Release Notes

---

## Positive Security Findings

The following security patterns were verified as **CORRECTLY IMPLEMENTED**:

### SQL Injection Protection
All database queries use parameterized queries with `?` placeholders. No string concatenation in SQL queries was found in application code (sqlite3.c is vendored SQLite).

**Verified in:**
- `internal/store/api_key_repo.go:67,95-108,136-141` - All queries use `?` placeholders
- `internal/store/user_repo.go:230-241` - Parameterized queries with proper allowlist for ORDER BY
- `internal/store/audit_repo.go:186-191` - Dynamic IN clause uses proper placeholders

### Command Injection Prevention
No usage of `os/exec` with shell commands was found in the codebase.

### HTTP Server Timeouts
All HTTP servers have appropriate timeouts configured:
- Gateway: `ReadTimeout`, `WriteTimeout`, `IdleTimeout` (defaults: 30s, 30s, 120s)
- Admin API: Same timeouts as gateway
- gRPC: Configured timeouts
- Raft transport: 30s read/write, 120s idle
- MCP server: 30s read/write, 120s idle

**Verified in:**
- `internal/config/load.go:57-64` - Default timeout configuration
- `internal/gateway/server.go:185-187,401-403` - Applied to servers
- `internal/raft/transport.go:103-105` - Raft RPC timeouts

### Cryptographic Best Practices
- **API key hashing:** Uses SHA-256 (api_key_repo.go)
- **Admin authentication:** Uses `subtle.ConstantTimeCompare` for timing-attack-resistant comparison
- **Session tokens:** Uses `crypto/rand` for generation
- **Password hashing:** Uses bcrypt with proper cost factor

**Verified in:**
- `internal/admin/token.go:374,424` - Constant-time admin key comparison
- `internal/mcp/server.go:478` - `secureCompare()` wrapper
- `internal/plugin/auth_apikey.go:186,204` - API key constant-time comparison
- `internal/store/api_key_repo.go:107` - SHA-256 key hashing
- `internal/store/user_repo.go:574-594` - Secure password generation with rejection sampling

### Client IP Extraction (Path Traversal Prevention)
The `ExtractClientIP()` function in `internal/pkg/netutil/clientip.go` is **secure by default**:
- When no trusted proxies are configured, X-Forwarded-For and X-Real-IP are **ignored**
- Right-to-left XFF parsing to find rightmost untrusted IP
- IP format validation before use

### Race Condition Protection
Proper use of synchronization primitives throughout:
- `sync.Mutex` / `sync.RWMutex` for protecting shared state
- `sync.Map` for concurrent map access
- `sync/atomic` for simple counters

**Verified in:**
- `internal/plugin/cache.go:148,160` - Cache protection
- `internal/ratelimit/token_bucket.go:11,20` - Rate limit bucket maps
- `internal/analytics/optimized_engine.go:356,368,373` - Analytics protection
- `internal/gateway/router.go:46` - Router protection

### Path Traversal in Config Import
Config import securely handles file operations:
- Uses `os.CreateTemp()` in a restricted temp directory
- Sets file permissions to `0o600`
- Strips sensitive fields before import
- Defers removal of temp files

**Verified in:**
- `internal/admin/server.go:470-489` - Secure temp file handling
- `internal/admin/server.go:516-533` - `stripSensitiveFields()` function

### Input Validation
- Email validation using `looksLikeEmail()` function
- CIDR/IP validation for trusted proxies
- JSON body size limits using `http.MaxBytesReader`
- GraphQL query depth/complexity planning

---

## Recommendations

1. **High Priority:** Add error checking for `strconv.Atoi` calls in pagination handlers to return proper 400 Bad Request responses.

2. **Medium Priority:** Consider refactoring `generateSecurePassword()` to return an error instead of panicking, though the practical risk is extremely low.

3. **Low Priority:** Continue maintaining the allowlist approach for sort column validation and ensure any future additions are reviewed for security implications.

---

## Scan Methodology

This scan followed the security-check sc-lang-go skill checklist (415+ items) including:

1. **Input Validation & Sanitization** - SQL injection, path traversal, integer conversion
2. **Authentication & Session Management** - Constant-time comparison, crypto/rand usage
3. **Cryptography** - Proper use of crypto libraries, key generation
4. **Error Handling** - Panic recovery, error propagation
5. **Concurrency & Race Conditions** - Mutex usage, sync.Map patterns
6. **Network & HTTP Security** - Timeout configuration, header validation
7. **Memory Safety** - No unsafe.Pointer usage found

---

*Report generated by security-check skill. For questions, contact the security team.*
