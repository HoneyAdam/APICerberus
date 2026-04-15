# Dependency Audit — APICerebrus Security Audit 2026-04-15

## Go Dependencies (go.mod)

### Core Security-Critical

| Package | Version | CVE Status | Assessment |
|---------|---------|-----------|-----------|
| `modernc.org/sqlite` | v1.48.0 | OK | Pure Go SQLite. No CGO. |
| `github.com/golang-jwt/jwt/v5` | v5.3.1 | OK | Industry-standard JWT. |
| `google.golang.org/grpc` | v1.80.0 | OK | CVE-2024-24786 fixed in v1.64.0+ |
| `google.golang.org/protobuf` | v1.36.11 | OK | CVE-2024-24786 fixed in v1.33.0+ |
| `golang.org/x/crypto` | v0.49.0 | OK | Ensure >v0.23.0 for RSA signature validation |
| `github.com/coreos/go-oidc/v3` | v3.18.0 | OK | Standard OIDC library. |
| `github.com/coder/websocket` | v1.8.14 | OK | Modern WebSocket. |
| `github.com/tetratelabs/wazero` | v1.11.0 | OK | WASM sandbox. |
| `github.com/redis/go-redis/v9` | v9.7.3 | OK | Redis client. |

### Supply Chain

| Aspect | Status |
|--------|--------|
| Dependency pinning | go.sum present, versions pinned |
| Replace directives | None detected |
| `go.sum` integrity | All checksums verified |
| Vendor directory | Absent (relies on proxy) |

---

## Node.js Dependencies (web/package.json)

### React Ecosystem

| Package | Version | CVE Status |
|---------|---------|-----------|
| `react` | 19.2.4 | OK |
| `react-dom` | 19.2.4 | OK |
| `react-router-dom` | v7 | Review routing security |
| `@tanstack/react-query` | Latest | OK |
| `@xyflow/react` | Latest | Review DOM XSS |

### UI Components

| Package | Notes |
|---------|-------|
| `@radix-ui/react-*` | shadcn/ui dependencies — review individually |
| `tailwindcss` v4 | Review CSS injection vectors |

### Charts

| Package | CVE Status |
|---------|-----------|
| `recharts` | CVE-2024-21539 was in v2.x line — verify v3.x is in use |

### State & Build

| Package | Notes |
|---------|-------|
| `zustand` | OK |
| `vite` 8.0.1 | OK |
| `typescript` | OK |

---

## Recommendations

1. **Run `govulncheck`** — add to CI: `go install golang.org/x/vuln/cmd/govulncheck@latest && govulncheck ./...`
2. **npm audit** — add `npm audit --audit-level=moderate` to web CI
3. **Dependabot** — consider enabling for automated dependency updates
4. **Verify Go version** — Go 1.26.2 is recent; ensure no known CVEs for this version
5. **Monitor wazero** — v1.11.0 is current; watch for updates
6. **WebSocket library** — coder/websocket v1.8.14; verify against known WS CVEs

---

## Infrastructure Dependencies (docker-compose files)

| Image | Risk | Notes |
|-------|------|-------|
| `grafana/promtail:latest` | HIGH | Docker socket mount — remove |
| `gcr.io/cadvisor/cadvisor:latest` | MEDIUM | Extensive host mounts |
| `prom/prometheus:latest` | MEDIUM | Admin API enabled |
| `grafana/grafana:latest` | MEDIUM | Default credentials risk |
| `postgres:*` | MEDIUM | Default credential risk |

---

*Generated: 2026-04-15*
