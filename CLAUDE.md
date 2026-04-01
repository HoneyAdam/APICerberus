# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## ⚠️ MANDATORY LOAD

**Before any work in this project, read and obey `AGENT_DIRECTIVES.md` in the project root.**

All rules in that file are hard overrides. They govern:
- Pre-work protocol (dead code cleanup, phased execution)
- Code quality (senior dev override, forced verification, type safety)
- Context management (sub-agent swarming, decay awareness, read budget)
- Edit safety (re-read before/after edit, grep-based rename, import hygiene)
- Commit discipline (atomic commits, no broken commits)
- Communication (state plan, report honestly, no hallucinated APIs)

**Violation of any rule is a blocking issue.**

---

## Project Overview

APICerebrus is a production-ready API Gateway built in Go with a React-based admin dashboard. It provides routing, authentication, rate limiting, billing/credits, audit logging, and GraphQL Federation with Raft-based clustering.

### Language & Tooling

- **Language**: Go 1.25+
- **Build**: `make build` (includes web dashboard build via npm)
- **Test**: `go test ./...` or `make test` (unit), `make integration` (integration), `make e2e` (e2e)
- **Lint**: `make lint` (runs `go vet` and `golangci-lint` if available)
- **Format**: `go fmt ./...` or `make fmt`
- **Race Detection**: `make test-race`
- **Coverage**: `make coverage` → generates `coverage/coverage.html`

**Web Dashboard** (React + Vite + Tailwind v4 + shadcn/ui):
- Location: `web/`
- Build: `cd web && npm ci && npm run build`
- Dev: `cd web && npm run dev`
- The Makefile's `build` target automatically builds the web dashboard and embeds it

### Architecture Notes

**Module Structure** (`internal/`):
- `gateway/` - HTTP/gRPC/WebSocket servers, router (radix tree), proxy engine, health checker
- `plugin/` - Plugin system with phases: PRE_AUTH → AUTH → POST_AUTH → PRE_PROXY → POST_PROXY
- `ratelimit/` - Token bucket, fixed window, sliding window, leaky bucket algorithms
- `billing/` - Credit system with atomic transactions
- `store/` - SQLite repository layer (users, api_keys, sessions, audit logs)
- `admin/` - REST API for management (port 9876)
- `portal/` - User-facing web portal (port 9877)
- `mcp/` - Model Context Protocol server (stdio + SSE transports)
- `raft/` - Distributed consensus for clustering
- `federation/` - GraphQL Federation (schema composition, query planning)
- `graphql/` - GraphQL query parsing and execution
- `grpc/` - gRPC server and transcoding
- `analytics/` - Metrics collection and time-series data
- `audit/` - Request/response logging with masking
- `cli/` - 40+ CLI commands for management

**Key Files**:
- `cmd/apicerberus/main.go` - Application entrypoint
- `internal/config/` - Configuration parsing and validation
- `apicerberus.example.yaml` - Example configuration
- `web/` - React dashboard (built and embedded via `embed.go`)

**Request Flow**:
```
Client → Gateway (Router) → Plugin Pipeline → Upstream (Load Balancer) → Backend
              ↓                      ↓
        Admin API              Audit/Analytics
```

### Build Requirements

1. Go 1.25+
2. Node.js 20+ (for web dashboard)
3. Make (optional, for convenience)

### Dependency Policy

- **STANDARD**: Use well-maintained packages freely
- Prefer standard library for simple tasks
- SQLite via `modernc.org/sqlite` (pure Go, no CGO)
- Key deps: `google.golang.org/grpc`, `golang.org/x/crypto`, `fsnotify`

### Known Gotchas

- **Web Build Required**: The `build` target always runs `web-build` first. To skip web build during dev, use `go build -o bin/apicerberus ./cmd/apicerberus` directly
- **Port Conflicts**: Default ports are 8080 (gateway), 9876 (admin), 9877 (portal), 50051 (gRPC), 12000 (Raft)
- **SQLite WAL Mode**: The store uses WAL mode for better concurrency; don't delete `-wal` or `-shm` files
- **Embedded Web Assets**: Dashboard is embedded at build time via `embed.go` - changes to `web/` require rebuild
- **API Key Prefixes**: Live keys use `ck_live_`, test keys use `ck_test_`
- **Config Reload**: Send SIGHUP for hot config reload (some changes require restart)
- **Test Tags**: Integration tests need `-tags=integration`, E2E needs `-tags=e2e`
- **gRPC-Web**: Requires `grpc.enable_web: true` for browser gRPC support
- **MCP Server**: Runs on stdio (default) or SSE mode; tools are defined in `internal/mcp/tools.go`

### Testing Patterns

- Unit tests: Standard Go tests alongside source files (`*_test.go`)
- Integration tests: `test/e2e_v*_test.go` with build tags
- Benchmarks: `test/benchmark/` with `-bench=. -benchmem`
- Run single test: `go test -run TestName ./path/to/package`

### Project Layout

```
cmd/apicerberus/     Application entrypoint
internal/           Core implementation
  gateway/          HTTP server, router, proxy
  plugin/           Auth, rate limiting, CORS, transforms
  store/            SQLite repositories
  admin/            REST API handlers
  portal/           User portal handlers
  mcp/              MCP server implementation
  raft/             Consensus and clustering
  federation/       GraphQL Federation
web/                React dashboard (Vite + Tailwind v4)
test/               E2E and integration tests
docs/               Architecture and API documentation
scripts/            Operational scripts (backup, restore)
deployments/        Docker, Helm, Swarm configs
```

### Configuration

- Main config: YAML file (see `apicerberus.example.yaml`)
- Environment vars: `APICERBERUS_*` prefix overrides config values
- Runtime config changes via Admin API (persisted to SQLite)
