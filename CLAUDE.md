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

## Build & Development Commands

| Task | Command |
|------|---------|
| Full build (includes web) | `make build` |
| Go only (skip web build) | `go build -o bin/apicerberus ./cmd/apicerberus` |
| Run all tests | `make test` or `go test ./...` |
| Run single test | `go test -run TestName ./path/to/package` |
| Race detection | `make test-race` |
| Coverage report | `make coverage` → opens `coverage/coverage.html` |
| Integration tests | `make integration` (requires `-tags=integration`) |
| E2E tests | `make e2e` (requires `-tags=e2e`) |
| Benchmarks | `make benchmark` |
| Lint | `make lint` (runs `go vet` + `golangci-lint` if available) |
| Format | `make fmt` or `go fmt ./...` |
| Security scan | `make security` |
| CI pipeline | `make ci` (fmt + lint + test-race + security + coverage) |

**Web Dashboard** (React + Vite + Tailwind v4 + shadcn/ui):
- Location: `web/`
- Dev server: `cd web && npm run dev`
- Build: `cd web && npm ci && npm run build`
- The Makefile `build` target embeds web assets automatically

## Architecture

### Core Modules (`internal/`)

| Module | Purpose |
|--------|---------|
| `gateway/` | HTTP/gRPC/WebSocket servers, radix tree router, proxy engine, 10 load balancing algorithms |
| `plugin/` | 6-phase pipeline (PRE_AUTH→AUTH→POST_AUTH→PRE_PROXY→PROXY→POST_PROXY), 20+ plugins |
| `ratelimit/` | Token bucket, fixed/sliding window, leaky bucket; Redis-backed for distributed |
| `billing/` | Credit system with atomic SQLite transactions |
| `store/` | SQLite repositories (WAL mode): users, API keys, sessions, audit logs |
| `admin/` | REST API for management (default port 9876) |
| `portal/` | User-facing web portal (default port 9877) |
| `mcp/` | Model Context Protocol server (stdio + SSE transports) |
| `raft/` | Distributed consensus for clustering |
| `federation/` | GraphQL Federation (Apollo-compatible schema composition, query planning) |
| `graphql/` | GraphQL query parsing, execution, subscriptions |
| `grpc/` | gRPC server, HTTP transcoding, gRPC-Web |
| `analytics/` | Metrics collection with ring buffers and time-series aggregation |
| `audit/` | Request/response logging with field masking, GZIP compression |
| `cli/` | 40+ CLI commands for administration |

### Request Flow

```
Client → Gateway (Radix Router) → Plugin Pipeline → Load Balancer → Upstream
              ↓                         ↓                    ↓
        Admin API                   Audit Logging      Health Checks
                                  Analytics
```

### Key Files

- `cmd/apicerberus/main.go` - Application entrypoint
- `internal/config/load.go` - Configuration parsing and validation
- `internal/config/dynamic_reload.go` - Hot config reload (SIGHUP)
- `apicerberus.example.yaml` - Comprehensive configuration example
- `embed.go` - Embeds web dashboard into Go binary

## Critical Implementation Details

### SQLite (pure Go)
- Uses `modernc.org/sqlite` (no CGO required)
- WAL mode enabled for better concurrency
- **Never delete** `-wal` or `-shm` files while running

### API Key Conventions
- Live keys: `ck_live_*` prefix
- Test keys: `ck_test_*` prefix (bypass credit checks)

### Default Ports
| Service | Port |
|---------|------|
| Gateway HTTP | 8080 |
| Admin API | 9876 |
| User Portal | 9877 |
| gRPC | 50051 |
| Raft | 12000 |

### Testing
- Unit tests: Standard `*_test.go` files alongside source
- Integration tests: `test/*_test.go` with `//go:build integration` tag
- E2E tests: `test/e2e_*_test.go` with `//go:build e2e` tag
- All tests use table-driven patterns with parallel subtests

### Configuration
- Main config: YAML file
- Environment overrides: `APICERBERUS_*` prefix (e.g., `APICERBERUS_ADMIN_API_KEY`)
- Hot reload: Send SIGHUP to running process (some changes require restart)

## Common Tasks

### Run a specific test
```bash
go test -run TestCreditDeduction ./internal/billing/...
```

### Run tests with verbose output
```bash
go test -v ./internal/gateway/...
```

### Build and run locally
```bash
make build
./bin/apicerberus start --config apicerberus.yaml
```

### Check test coverage for a package
```bash
go test -coverprofile=coverage.out ./internal/billing
go tool cover -func=coverage.out
```
