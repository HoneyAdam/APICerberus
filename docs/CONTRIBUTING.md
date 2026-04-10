# Contributing Guide

## Development Environment Setup

### Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.26+ | Backend development |
| Node.js | 20+ | Frontend dashboard |
| make | any | Build automation |
| SQLite CLI | any | Debugging database (optional) |
| Redis | 7+ | Distributed rate limiting (optional) |

### Quick Start

```bash
# Clone the repository
git clone https://github.com/APICerberus/APICerebrus.git
cd APICerebrus

# Download Go dependencies
go mod download

# Install frontend dependencies
cd web && npm ci && cd ..

# Build the full binary (includes embedded web assets)
make build

# Run with example configuration
./bin/apicerberus start --config apicerberus.example.yaml
```

### Running Tests

```bash
# All unit tests
make test

# Single package
go test ./internal/gateway/... -v

# Race detection
make test-race

# Coverage report
make coverage  # Opens coverage/coverage.html

# Integration tests (require full environment)
make integration

# E2E tests
make e2e

# Benchmarks
make benchmark

# Full CI pipeline
make ci
```

### Frontend Development

```bash
cd web

# Start dev server with hot reload
npm run dev  # Available at http://localhost:5173

# Type check
npm run lint

# Run frontend tests
npm run test:run

# Production build
npm run build  # Outputs to web/dist/
```

The Go binary embeds `web/dist/` via `embed.go`. After rebuilding the frontend, run `make build` to create a new binary with updated assets.

## Project Structure

```
APICerebrus/
├── cmd/apicerberus/          # Application entrypoint
├── internal/
│   ├── gateway/              # HTTP/gRPC/WebSocket servers, radix router, proxy
│   ├── plugin/               # 5-phase plugin pipeline + 20+ plugins
│   ├── ratelimit/            # Token bucket, sliding window, leaky bucket
│   ├── billing/              # Credit system with atomic transactions
│   ├── store/                # SQLite repositories (WAL mode)
│   ├── admin/                # Admin REST API + WebSocket
│   ├── portal/               # User-facing web portal
│   ├── raft/                 # Distributed consensus (Hashicorp Raft)
│   ├── federation/           # GraphQL Federation
│   ├── grpc/                 # gRPC server + HTTP transcoding
│   ├── analytics/            # Metrics collection with ring buffers
│   ├── audit/                # Request/response logging with masking
│   ├── mcp/                  # Model Context Protocol server
│   ├── cli/                  # CLI commands
│   ├── config/               # Configuration loading + hot reload
│   ├── logging/              # Structured logging with rotation
│   └── pkg/                  # Shared utilities (JWT, YAML, JSON, UUID)
├── web/                      # React 19 admin dashboard (TypeScript + Vite)
├── test/                     # Integration, E2E, load, and benchmark tests
├── scripts/                  # Backup, restore, health check
└── docs/                     # Architecture decisions, troubleshooting
```

## Architecture Overview

APICerebrus is an API Gateway built as a single binary with embedded dashboard:

```
Client → Gateway (Radix Router) → Plugin Pipeline → Load Balancer → Upstream
              ↓                         ↓                    ↓
        Admin API                   Audit Logging      Health Checks
                                  Analytics
```

### Key Design Decisions

See [docs/ARCHITECTURE_DECISIONS.md](docs/ARCHITECTURE_DECISIONS.md) for detailed rationale on:
- SQLite as primary database (ADR-001)
- Custom YAML parser (ADR-002)
- 5-phase plugin pipeline (ADR-003)
- SubnetAware load balancing (ADR-004)

### Plugin System

Plugins execute in 5 phases: `PRE_AUTH → AUTH → PRE_PROXY → PROXY → POST_PROXY`. Each plugin implements the `Plugin` interface and registers for specific phases. See `internal/plugin/types.go` and `internal/plugin/pipeline.go`.

### Request Flow

1. Client request arrives at gateway
2. Radix tree router matches route by method + path + host
3. PRE_AUTH plugins run (correlation ID, IP restrictions)
4. AUTH plugins run (API key, JWT validation)
5. PRE_PROXY plugins run (rate limiting, transforms)
6. PROXY plugins run (circuit breaker, retry)
7. Request forwarded to upstream (load balanced)
8. Response captured by `ResponseCaptureWriter`
9. POST_PROXY plugins run (response transforms)
10. Audit log entry queued asynchronously

## Code Style

### Go

- Follow [Effective Go](https://go.dev/doc/effective_go) and [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- Run `make fmt` before committing
- Run `make lint` before submitting PRs
- Use table-driven tests with `t.Parallel()` in subtests
- Add fuzz tests for parsers and routers

### Frontend

- TypeScript strict mode — no `any` without justification
- Components: functional with hooks, no class components
- State: React Query for server state, Zustand for client state
- Styling: Tailwind v4 utility classes + shadcn/ui components

## Pull Request Process

1. Create a feature branch from `main`
2. Make changes with tests
3. Run `make ci` locally — must pass before PR
4. Open PR with description of what changed and why
5. Address review feedback
6. Squash merge to `main`

### Commit Message Convention

Use conventional commits:
```
type(scope): description

feat(gateway): add WebSocket proxy support
fix(billing): retry on SQLITE_BUSY with backoff
docs: add architecture decision records
test(router): add fuzz tests for path traversal
perf(audit): reduce allocations with object pool
```

## Local Development with Docker

```bash
# Build image
docker build -t apicerberus:dev .

# Run with local config
docker run -p 8080:8080 -p 9876:9876 -p 9877:9877 \
  -v $(pwd)/apicerberus.yaml:/config/apicerberus.yaml \
  -v $(pwd)/data:/data \
  apicerberus:dev
```

## Troubleshooting

See [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) for common issues:
- SQLite locked errors
- Redis connection failures
- Certificate renewal problems
- Raft cluster join failures
- Plugin execution timeouts
