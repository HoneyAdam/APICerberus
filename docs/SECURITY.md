# Security Policy

## Supported Versions

The following versions of APICerebrus are currently supported with security updates:

| Version | Supported          |
| ------- | ------------------ |
| 1.x.x   | :white_check_mark: |
| < 1.0   | :x:                |

## Security Features

APICerebrus implements comprehensive security measures across multiple layers:

### Authentication & Authorization

- **API Key Authentication**: Configurable API key validation with multiple extraction methods (headers, query params, cookies)
- **JWT Authentication**: RS256 and HS256 signature verification with JWKS support
- **Session Management**: Secure session tokens with configurable expiration
- **Role-Based Access Control**: Admin and user roles with granular permissions
- **Endpoint Permissions**: Per-route, per-user access control with time-based restrictions

### Cryptographic Practices

- **Password Hashing**: bcrypt with default cost (10) for secure password storage
- **API Key Hashing**: SHA-256 for API key storage (raw keys never stored)
- **Session Tokens**: Cryptographically secure random generation using `crypto/rand`
- **TLS Configuration**: Minimum TLS 1.2 with secure cipher suites
- **Certificate Management**: Automatic ACME/Let's Encrypt certificate issuance and renewal

### Input Validation & Injection Prevention

- **SQL Injection Prevention**: Parameterized queries throughout all database operations
- **Path Traversal Protection**: Input sanitization and path validation
- **Request Size Limits**: Configurable maximum body size enforcement
- **JSON Schema Validation**: Request payload validation plugin
- **Header Injection Prevention**: Strict header validation and sanitization

### Transport Security

- **HTTPS Enforcement**: Automatic HTTPS redirection and HSTS headers
- **Security Headers**:
  - `X-Content-Type-Options: nosniff`
  - `X-Frame-Options: DENY`
  - `X-XSS-Protection: 1; mode=block`
  - `Content-Security-Policy: default-src 'self'; frame-ancestors 'none'`
  - `Referrer-Policy: strict-origin-when-cross-origin`
  - `Strict-Transport-Security` (HTTPS only)
  - `Permissions-Policy` for feature restrictions

### Rate Limiting & DDoS Protection

- **Multiple Algorithms**: Token bucket, fixed window, sliding window, leaky bucket
- **Per-Consumer Limits**: Individual rate limit configuration per API consumer
- **IP-Based Restrictions**: Whitelist/blacklist support with CIDR notation
- **Bot Detection**: User-agent pattern matching with configurable actions

### Audit & Monitoring

- **Comprehensive Audit Logging**: All requests logged with full context
- **Sensitive Data Masking**: Automatic PII/sensitive data redaction in logs
- **Real-time Analytics**: Request metrics and performance monitoring
- **Alerting**: Configurable alerts for suspicious activity patterns

## Security Checklist

### Deployment Checklist

- [ ] Change default admin password (`APICERBERUS_ADMIN_PASSWORD` environment variable)
- [ ] Enable HTTPS with valid TLS certificates
- [ ] Configure appropriate CORS policies
- [ ] Set up rate limiting for all routes
- [ ] Enable audit logging
- [ ] Configure IP restrictions if needed
- [ ] Set up monitoring and alerting
- [ ] Enable request/response body size limits
- [ ] Configure session security settings (Secure, HttpOnly, SameSite)
- [ ] Review and customize security headers

### Development Checklist

- [ ] Use parameterized queries for all database operations
- [ ] Validate all user inputs
- [ ] Implement proper error handling without information leakage
- [ ] Use `crypto/rand` for all random generation
- [ ] Apply constant-time comparison for sensitive operations
- [ ] Validate JWT algorithms (explicitly reject "none")
- [ ] Sanitize all data before logging
- [ ] Use secure session management

### Configuration Security

```yaml
# Recommended security settings
gateway:
  max_body_bytes: 1048576  # 1MB limit
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 120s

auth:
  session:
    cookie_secure: true      # HTTPS only
    cookie_http_only: true   # No JavaScript access
    cookie_same_site: strict # CSRF protection

audit:
  enabled: true
  mask_sensitive: true
  max_request_body: 10240   # 10KB
  max_response_body: 10240  # 10KB
```

## Reporting a Vulnerability

We take security vulnerabilities seriously. If you discover a security issue:

1. **Do NOT open a public issue**
2. Email security concerns to: security@apicerberus.local
3. Include:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if any)

We will:
- Acknowledge receipt within 48 hours
- Provide a timeline for resolution
- Notify you when the issue is fixed
- Credit you in the security advisory (with your permission)

## Security Scanning

### Running Security Scans

```bash
# Run all security checks
make security

# Or run individual scanners
gosec ./...                    # Static analysis
govulncheck ./...              # Vulnerability check
trivy fs .                     # Container/filesystem scan
```

### Continuous Security

Security scans are integrated into the CI pipeline:
- Static analysis with gosec on every PR
- Vulnerability scanning with govulncheck
- Dependency scanning with Dependabot

## Known Security Considerations

### Current Limitations

1. **SQLite Database**: Single-node limitation; for multi-node deployments, consider external database options
2. **WebSocket Security**: Ensure proper origin validation for WebSocket connections
3. **GraphQL**: Query depth/complexity limits should be configured for production

### Security Hardening Recommendations

1. **Run as non-root user** in container deployments
2. **Use secrets management** for sensitive configuration
3. **Enable network policies** in Kubernetes deployments
4. **Configure backup encryption** for sensitive data
5. **Implement API versioning** for graceful security updates

## Security Best Practices

### API Key Management

- Rotate API keys regularly (90-day recommended maximum)
- Use different keys for different environments
- Monitor key usage for anomalies
- Revoke compromised keys immediately

### JWT Security

- Use RS256 for production (asymmetric keys)
- Keep clock skew minimal (30 seconds default)
- Validate all claims (exp, iss, aud)
- Use short expiration times (15-60 minutes)
- Implement proper key rotation

### Session Security

- Use secure, random session tokens (256-bit minimum)
- Implement session expiration and cleanup
- Store only token hashes server-side
- Use HttpOnly, Secure, SameSite cookies
- Implement proper logout (server-side invalidation)

### Audit Log Security

- Protect audit logs from tampering
- Implement log rotation and archival
- Mask sensitive data in logs
- Monitor for suspicious patterns
- Retain logs according to compliance requirements

## Compliance

APICerebrus is designed to help meet compliance requirements for:

- **GDPR**: Data masking, audit trails, data retention policies
- **SOC 2**: Access controls, audit logging, monitoring
- **PCI DSS**: Encryption, access control, audit trails (when configured appropriately)

Note: Compliance depends on proper configuration and deployment practices.

## Security Updates

Security updates are released as patch versions (e.g., 1.2.3). Subscribe to:
- GitHub Security Advisories
- Release notifications
- Security mailing list (security-announce@apicerberus.local)

## Contact

- Security Team: security@apicerberus.local
- General Questions: support@apicerberus.local
- Emergency: +1-XXX-XXX-XXXX (24/7 hotline)
