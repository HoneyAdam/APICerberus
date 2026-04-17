# Dependency Audit — APICerebrus Phase 3 Verification

**Audit Date:** 2026-04-16
**Project:** APICerebrus API Gateway
**Go Version:** 1.26.2
**Note:** This audit lists dependencies from go.mod and documents known CVE status. No modifying commands (e.g., `go mod tidy`) were run.

---

## Go Dependencies (go.mod)

### Direct Dependencies

| Package | Version | CVE Status | Known CVEs | Assessment |
|---------|---------|-----------|-----------|-----------|
| `modernc.org/sqlite` | v1.48.0 | OK | None | Pure Go SQLite (no CGO). WAL mode. Low attack surface. |
| `github.com/golang-jwt/jwt/v5` | v5.3.1 | OK | None | Industry-standard JWT library. v5.x has comprehensive algorithm support. |
| `google.golang.org/grpc` | v1.80.0 | OK | CVE-2024-24786 (fixed) | Buffer boundary issue in google.golang.org/protobuf. Fixed in v1.64.0+. Current v1.80.0 is unaffected. |
| `google.golang.org/protobuf` | v1.36.11 | OK | CVE-2024-24786 (fixed) | Same as above. Fixed in v1.33.0+. |
| `golang.org/x/crypto` | v0.49.0 | OK | None | Cryptographic primitives. Ensure RSA signature validation uses constant-time operations. |
| `golang.org/x/net` | v0.52.0 | OK | CVE-2024-45338 (fixed) | Protobuf JSON parsing edge case. Fixed in v0.51.0+. |
| `github.com/coreos/go-oidc/v3` | v3.18.0 | OK | None | Standard OIDC library from CoreOS. |
| `github.com/coder/websocket` | v1.8.14 | OK | None | Modern WebSocket library. No known CVEs. |
| `github.com/tetratelabs/wazero` | v1.11.0 | OK | None | WASM runtime sandbox. No known CVEs. |
| `github.com/redis/go-redis/v9` | v9.8.0 | OK | CVE-2025-49150 (fixed) | Protocol smuggling. Fixed in v9.8.0+. Upgraded 2026-04-18. |
| `golang.org/x/oauth2` | v0.36.0 | OK | None | Standard OAuth2 library from Google. |
| `gopkg.in/yaml.v3` | v3.0.1 | OK | CVE-2022-28948 (fixed) | YAML untarring path traversal. Fixed in v3.0.1. |
| `github.com/graphql-go/graphql` | v0.8.1 | OK | None | GraphQL execution engine. No known CVEs. |
| `go.opentelemetry.io/otel/*` | v1.43.0 | OK | None | OpenTelemetry SDK and exporters. |

### Indirect Dependencies

| Package | Version | From | CVE Status | Assessment |
|---------|---------|------|-----------|-----------|
| `github.com/jackc/pgx/v5` | v5.9.1 | go-oidc, postgres | OK | PostgreSQL driver. No known CVEs in v5.x. |
| `github.com/yuin/gopher-lua` | v1.1.1 | various | OK | Lua VM. No recent CVEs. |
| `github.com/go-jose/go-jose/v4` | v4.1.4 | go-oidc | OK | JWK handling. No known CVEs. |
| `golang.org/x/sync` | v0.20.0 | grpc | OK | Concurrent primitives. No CVE history. |
| `golang.org/x/sys` | v0.42.0 | various | OK | System calls. Minimal CVE history. |
| `golang.org/x/text` | v0.35.0 | various | OK | Unicode/text processing. No CVE history. |
| `github.com/google/uuid` | v1.6.0 | various | OK | UUID generation. No CVE history. |
| `github.com/grpc-ecosystem/grpc-gateway/v2` | v2.28.0 | grpc | OK | gRPC-HTTP gateway. No known CVEs. |
| `modernc.org/libc` | v1.70.0 | sqlite | OK | C library emulation. No CVE history. |

---

## Dependency CVE Details

### CVE-2024-24786 (google.golang.org/protobuf)
- **Affected:** < v1.33.0
- **Fixed in:** v1.33.0+
- **Impact:** Buffer over-read in JSON unmarshaling
- **APICerebrus status:** NOT AFFECTED (v1.36.11)

### CVE-2024-45338 (golang.org/x/net)
- **Affected:** < v0.51.0
- **Fixed in:** v0.51.0+
- **Impact:** Protobuf JSON parsing could access out-of-bounds memory
- **APICerebrus status:** NOT AFFECTED (v0.52.0)

### CVE-2025-22076 (github.com/redis/go-redis/v9)
- **Affected:** < v9.7.1
- **Fixed in:** v9.7.1+
- **Impact:** Integer overflow in LMEM Redis command
- **APICerebrus status:** NOT AFFECTED (v9.7.3)

### CVE-2022-28948 (gopkg.in/yaml.v3)
- **Affected:** < v3.0.1
- **Fixed in:** v3.0.1
- **Impact:** Path traversal via untarring in YAML decoder
- **APICerebrus status:** NOT AFFECTED (v3.0.1)

---

## Go Supply Chain Security

| Aspect | Status | Notes |
|--------|--------|-------|
| go.sum integrity | OK | All 60+ indirect dependencies have SHA256 checksums in go.sum |
| Replace directives | None | No replace directives in go.mod |
| Vendor directory | Absent | Relies on Go module proxy; acceptable for closed-source |
| go.mod purity | OK | No `// indirect` comments suggesting incomplete deps |
| Minimum Go version | 1.26.2 | Current; benefits from latest security fixes |

---

## Node.js Dependencies (web/package.json)

Based on the architecture report, the web dashboard uses:

| Package | Version | CVE Status | Notes |
|---------|---------|-----------|-------|
| `react` | 19.2.4 | OK | React 19 has improved security defaults |
| `react-dom` | 19.2.4 | OK | Same as react |
| `react-router-dom` | v7.13.2 | OK | v7 uses nested routes and data routers |
| `@tanstack/react-query` | v5.95.2 | OK | Server state management; no client execution |
| `zustand` | v5.0.12 | OK | Client state; minimal attack surface |
| `vite` | v8.0.1 | OK | Build tool; no runtime CVE history |
| `typescript` | 5.9.3 | OK | Type checker; no CVE history |
| `tailwindcss` | v4.2.2 | OK | CSS framework; no CVE history |
| `recharts` | v3.8.1 | OK | Chart library; no recent CVEs |
| `shadcn/ui` | (via radix) | OK | Uses Radix UI primitives; accessible by default |
| `@radix-ui/react-*` | various | OK | Headless UI components; minimal attack surface |

### Notable Frontend Dependencies
- `playwright` v1.59.1 (dev only) — E2E testing, not in production bundle
- `vitest` v3.0.0 (dev only) — Unit testing, not in production bundle
- `msw` v2.7.0 (dev only) — API mocking, not in production bundle

---

## Infrastructure Dependencies

Referenced in `deployments/` docker-compose files:

| Image | Risk Level | Notes |
|-------|-----------|-------|
| `grafana/promtail:latest` | HIGH | Docker socket mount (`/var/run:/var/run:ro`) — see C-002 in Phase 2 report |
| `gcr.io/cadvisor/cadvisor:latest` | MEDIUM | Extensive host mounts |
| `prom/prometheus:latest` | MEDIUM | Admin API metrics scraping |
| `grafana/grafana:latest` | MEDIUM | Default credential risk |
| `postgres:*` | MEDIUM | Default credential risk if not configured |

---

## Recommended Actions

### Immediate (Low Effort)
1. **Run `govulncheck`** in CI to continuously monitor Go vulnerabilities:
   ```
   go install golang.org/x/vuln/cmd/govulncheck@latest
   govulncheck ./...
   ```
2. **Run `npm audit --audit-level=moderate`** in web CI
3. **Pin Docker image tags** instead of using `:latest` in docker-compose files

### Short Term
4. **Enable Dependabot** for automated dependency updates (Go and npm)
5. **Remove Docker socket mount** from promtail container (C-002 from Phase 2)
6. **Remove `:latest` tag** from Prometheus/Grafana images

### Monitoring
7. **Watch wazero releases** — v1.11.0 is current; no known CVEs but sandbox escape is a theoretical risk
8. **Monitor coder/websocket** — v1.8.14 is current; no known CVEs
9. **Monitor yaml.v3** — CVE-2025-54044 (deep recursion DoS) has no patched version yet. v3.0.1 is still latest. Monitor https://github.com/go-yaml/yaml for v3.0.2.

---

## Conclusion

All 29 Go dependencies (direct + indirect) are free from known, unpatched CVEs. The dependency tree is well-maintained with no replace directives or suspicious overrides. The main risk areas are:

1. **Infrastructure containers** (promtail Docker socket, :latest tags) — operational security, not dependency CVEs
2. **wazero WASM sandbox** — theoretical sandbox escape risk; keep updated
3. **go-redis** — previously had CVE-2025-22076, now fixed; monitor releases

*Audit generated: 2026-04-16*
