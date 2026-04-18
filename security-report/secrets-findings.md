# Secrets and Credentials Findings Report

**Scan Date:** 2026-04-18
**Scope:** APICerebrus codebase (excluding vendor/, node_modules/)
**Tool:** grep-based pattern matching

---

## Summary

| Severity | Count |
|----------|-------|
| Critical | 0 |
| High | 1 |
| Medium | 2 |
| Low / Info | Multiple |

---

## HIGH Severity

### H-001: Hardcoded Consumer API Key in Configuration

**File:** `apicerberus.yaml:220`

```yaml
consumers:
  - name: "mobile-app"
    api_keys:
      - key: "ck_live_mobile_app_key_12345678901234567890"
```

**Issue:** A production-style API key (`ck_live_*` prefix) is hardcoded in the configuration file. Although this appears to be a sample/development key, hardcoding any `ck_live_*` key in version-controlled configuration files is dangerous.

**Recommendation:** Use environment variable substitution:
```yaml
consumers:
  - name: "mobile-app"
    api_keys:
      - key: "${MOBILE_APP_API_KEY}"
```

---

## MEDIUM Severity

### M-001: Placeholder Credentials in Kubernetes Deployment Example

**File:** `deployments/examples/kubernetes-deployment.yaml:82-83`

```yaml
stringData:
  ADMIN_API_KEY: "change-me-in-production"
  SESSION_SECRET: "change-me-in-production"
```

**Issue:** Example Kubernetes Secret uses weak placeholder values that could be accidentally deployed to production.

**Recommendation:** Remove default values or use clearly marked template values that fail validation if not replaced:
```yaml
stringData:
  ADMIN_API_KEY: ""  # REQUIRED: Set via environment
  SESSION_SECRET: ""  # REQUIRED: Set via environment
```

---

### M-002: Test-Only Credentials in E2E Config

**File:** `test/e2e-config.yaml:10-11`

```yaml
api_key: "e2e-test-admin-key-must-be-at-least-32-chars-long"
token_secret: "e2e-test-jwt-secret-must-be-at-least-32-characters"
```

**Issue:** While these are test-only credentials, they are hardcoded values that could be mistaken for production secrets if the file is used as a template.

**Recommendation:** Consider using environment variables with defaults only for local development:
```yaml
api_key: "${ADMIN_API_KEY:-e2e-test-admin-key-must-be-at-least-32-chars-long}"
```

---

## LOW / INFO Severity

### L-001: Placeholder API Key in Web Admin UI Sample

**File:** `web/src/pages/admin/Config.tsx:16`

```typescript
const SAMPLE_CONFIG = `gateway:
  http_addr: ":8080"
admin:
  addr: ":8081"
  api_key: "change-me-min-32-chars"
```

**Issue:** Sample configuration displayed in the admin UI contains a placeholder API key.

**Recommendation:** This appears to be intentional sample code. No action required, but ensure the sample is clearly marked as non-functional.

---

### L-002: Bearer Token Placeholder in Tracing Config

**File:** `apicerberus.yaml:92`

```yaml
otlp_headers:  # Additional headers for OTLP (optional)
  Authorization: "Bearer token"
```

**Issue:** Placeholder Bearer token in tracing configuration example.

**Recommendation:** Use environment variable:
```yaml
otlp_headers:
  Authorization: "Bearer ${OTLP_AUTH_HEADER}"
```

---

### L-003: README curl Commands Use `change-me`

**Files:** `README.md:233, 425, 430, 441, 446, 455`

Multiple curl examples in README use `-H "X-Admin-Key: change-me"` as placeholder.

**Issue:** Documentation examples with weak credentials could be copy-pasted into production.

**Recommendation:** Update examples to use clearly marked placeholders like `${ADMIN_API_KEY}` or remove the header entirely and note that authentication is required.

---

### L-004: Web E2E Test Passwords

**Files:**
- `web/tests/e2e/crud-flows.spec.ts:105` - `test-password-123`
- `web/tests/e2e/billing-credits.spec.ts:20,40` - `test-password-456`, `test-password-789`
- `web/tests/e2e/api-keys-login.spec.ts:22,53` - `test-password-abc`, `test-password-xyz`

**Issue:** Test passwords in Playwright E2E tests are hardcoded.

**Assessment:** These are test fixtures only, not production credentials. Risk is minimal but should be documented.

---

## Test Fixtures (No Action Required)

The following are test-only fixtures that are intentionally placeholder values:

| File | Pattern | Purpose |
|------|---------|---------|
| `internal/audit/masker_test.go` | `"Bearer token123"` | Test authorization header masking |
| `internal/admin/webhook_test.go` | `"test-secret"` | Webhook delivery tests |
| `internal/pkg/jwt/jwt_test.go` | `"test-secret-long-enough-for-hs256-min!!"` | JWT signing tests |
| `test/benchmark/plugin_bench_test.go` | `"super-secret-key-that-is-32-bytes!"` | Benchmark fixtures |
| `test/e2e_v*.go` | Various `"test-secret-key"` values | E2E test consumers |

These are acceptable for testing purposes.

---

## GOOD PRACTICES OBSERVED

The following patterns demonstrate proper secret management:

1. **docker-compose.prod.yml** uses Docker secrets for sensitive values:
   ```yaml
   secrets:
     jwt_secret:
       file: ./secrets/jwt_secret.txt
     admin_api_key:
       file: ./secrets/admin_api_key.txt
   ```

2. **apicerberus.yaml** uses environment variable substitution for production:
   ```yaml
   admin:
     api_key: "${ADMIN_API_KEY}"
     token_secret: "${TOKEN_SECRET}"
   ```

3. **apicerberus.example.yaml** leaves secrets empty with documentation:
   ```yaml
   admin:
     api_key: ""  # Required: must be a strong, unique secret
   ```

---

## RECOMMENDATIONS

1. **Remove hardcoded `ck_live_*` keys** from all config files - use environment variables
2. **Strengthen Kubernetes example** - fail-fast if secrets are not properly configured
3. **Update README examples** - use environment variable references instead of placeholder strings
4. **Add secret scanning** to CI pipeline to prevent future leaks
5. **Document test fixture policy** - clarify which files contain test-only data

---

## SCAN METHODOLOGY

- Searched for: `api_key`, `apikey`, `secret`, `token`, `password`, `jwt`, `private key`
- Excluded: vendor/, node_modules/, *.sum files
- File types: *.go, *.yaml, *.yml, *.json, *.ts, *.tsx, *.md
- Patterns matched: `ck_live_*`, `ck_test_*`, `-----BEGIN * PRIVATE KEY-----`, database URLs, Bearer tokens
