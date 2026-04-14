# Dependency Audit

**Date:** 2026-04-14
**Go Version:** 1.26.2

## Core Dependencies

| Dependency | Version | Risk | Assessment |
|------------|---------|------|------------|
| `modernc.org/sqlite` | v1.48.0 | LOW | Pure Go SQLite driver. Actively maintained. No CGO required. |
| `github.com/golang-jwt/jwt/v5` | v5.3.1 | LOW | Audited, industry-standard JWT library. Supports HS256, RS256, ES256, EdDSA. |
| `github.com/coder/websocket` | v1.8.14 | LOW | Modern WebSocket library replacing deprecated golang.org/x/net/websocket. |
| `github.com/graphql-go/graphql` | v0.8.1 | MEDIUM | Stale release but stable API. Depth/complexity guard in place. |
| `github.com/redis/go-redis/v9` | v9.7.3 | LOW | Actively maintained Redis client. |
| `github.com/tetratelabs/wazero` | v1.11.0 | LOW | WASM runtime with sandboxing. No CGO. |
| `github.com/coreos/go-oidc/v3` | v3.18.0 | LOW | Standard OIDC library from CoreOS/Red Hat. |
| `google.golang.org/grpc` | v1.80.0 | LOW | Official gRPC library. |
| `go.opentelemetry.io/otel` | v1.43.0 | LOW | Official OpenTelemetry SDK. |
| `gopkg.in/yaml.v3` | v3.0.1 | LOW | Standard YAML library. |
| `golang.org/x/crypto` | v0.49.0 | LOW | Standard crypto extensions (bcrypt, etc.). |

## Supply Chain Assessment

| Aspect | Status |
|--------|--------|
| Dependency pinning | go.sum present -- versions pinned |
| Vendor directory | Absent -- relies on module proxy |
| Replace directives | None detected |
| Known typosquatting | No indicators found |
| `go.sum` integrity | All checksums verified |

## Frontend Dependencies

| Dependency | Version | Risk | Assessment |
|------------|---------|------|------------|
| `react` | 19.2.4 | LOW | Latest stable. |
| `recharts` | 3.8.1 | INFO | Latest v3. CVE-2024-21539 was in v2.x line. |
| `@tanstack/react-query` | 5.95.2 | LOW | Well-maintained. |
| `zustand` | 5.0.12 | LOW | Lightweight state management. |
| `tailwindcss` | 4.2.2 | LOW | Latest v4. |
| `vite` | 8.0.1 | LOW | Latest major. |
| `typescript` | 5.9.3 | LOW | Latest stable. |

## Recommendations

1. **INFO:** Monitor `graphql-go/graphql` for updates or consider alternatives if it becomes unmaintained
2. **INFO:** Consider adding `go mod verify` to CI pipeline for supply chain integrity
3. **INFO:** Consider running `govulncheck` as part of CI (currently available via `make security`)
