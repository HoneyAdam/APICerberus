# API Cerberus

<p align="center">
  <img src="assets/banner.jpeg" alt="API Cerberus" width="100%">
</p>

[![Go](https://img.shields.io/badge/go-1.26%2B-00ADD8.svg)](https://go.dev/)
[![Release](https://img.shields.io/badge/release-v0.1.0-blue.svg)](#release-status)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](./LICENSE)

API Cerberus is an API gateway and API management platform written in Go.
It combines gateway routing/proxy features with authentication, rate limiting,
user management, credits/billing, and an admin REST API.

## Release Status

- Current tagged release: `v1.0.0`
- Implemented milestones: `v0.0.1` to `v1.0.0`
- Status: Production Ready

Progress is tracked in [`.project/TASKS.md`](./.project/TASKS.md).

## What Is Implemented (v0.0.1 - v0.1.0)

### Core Features
- Core gateway: routing, reverse proxy, websocket proxy
- Load balancing: 10 algorithms (round robin, weighted, least_conn, ip_hash, consistent_hash, adaptive, least_latency, health_weighted, random)
- Health checks: active and passive
- Plugin pipeline with 20+ plugins
- Authentication: API key (SQLite-backed) and JWT (HS256, RS256, JWKS)
- Rate limiting: 4 algorithms (token bucket, fixed window, sliding window, leaky bucket)
- Traffic controls: circuit breaker, retry, timeout, IP restrict, CORS, bot detection
- Transform plugins: request/response transform, URL rewrite, validation, size limits, correlation IDs, compression

### Data & Management
- Embedded SQLite-backed data model
- User management with roles and IP whitelist
- API key management with `ck_live_`/`ck_test_` prefixes
- Credit system with atomic transactions and test key bypass
- Endpoint permissions with time/day restrictions
- Audit logging with masking and retention policies
- Analytics engine with real-time metrics

### Interfaces
- Admin REST API (40+ endpoints)
- Web Dashboard (React + shadcn/ui, 35+ components)
- User Portal with API Playground
- WebSocket real-time updates
- MCP Server (stdio + SSE transports, 25+ tools)
- CLI with 40+ commands

### Operations
- TLS with ACME auto-provisioning
- Config export/import with diff
- Hot reload (SIGHUP)
- Graceful shutdown

## Documentation

- Product specification: [`.project/SPECIFICATION.md`](./.project/SPECIFICATION.md)
- Implementation guide: [`.project/IMPLEMENTATION.md`](./.project/IMPLEMENTATION.md)
- Task roadmap and milestones: [`.project/TASKS.md`](./.project/TASKS.md)
- Example config: [`apicerberus.example.yaml`](./apicerberus.example.yaml)

### Architecture Documentation

Comprehensive architecture documentation is available in [`docs/architecture/`](./docs/architecture/):

- [Overview](docs/architecture/README.md) - High-level system overview and principles
- [System Design](docs/architecture/system-design.md) - Architecture patterns and component interactions
- [Components](docs/architecture/components.md) - Detailed component architecture
- [Deployment](docs/architecture/deployment.md) - Deployment patterns and topology
- [Data Flow](docs/architecture/data-flow.md) - Request/response lifecycle
- [Security](docs/architecture/security.md) - Security architecture and threat model

### API Documentation

- OpenAPI 3.0 Specification: [`docs/api/openapi.yaml`](./docs/api/openapi.yaml)
- View with Swagger UI or import into Postman

## Contributing

Please see [CONTRIBUTING.md](./CONTRIBUTING.md) for development guidelines, including:

- Setup and prerequisites
- Branching strategy and commit conventions
- Testing requirements
- Code quality standards
- Security checklist

## Requirements

- Go `1.26+`
- Make (optional, for convenience commands)

## Quick Start

1. Copy the example configuration:

```bash
cp apicerberus.example.yaml apicerberus.yaml
```

PowerShell:

```powershell
Copy-Item apicerberus.example.yaml apicerberus.yaml
```

2. Build:

```bash
make build
```

3. Validate config:

```bash
./bin/apicerberus config validate apicerberus.yaml
```

4. Edit `apicerberus.yaml` for your local environment:

- Set `admin.api_key` to a secure value.
- Update upstream targets (`upstreams[].targets[].address`) to reachable services.
- If you keep route host filters from the example config, send matching `Host` headers in requests.

5. Start gateway and admin API:

```bash
./bin/apicerberus start --config apicerberus.yaml
```

6. Check admin status:

```bash
curl -H "X-Admin-Key: change-me" http://127.0.0.1:9876/admin/api/v1/status
```

7. Stop process (from another terminal):

```bash
./bin/apicerberus stop
```

## Local Request Example

After configuring a reachable upstream and starting the server:

```bash
curl \
  -H "Host: api.example.com" \
  -H "X-API-Key: ck_live_mobile_abc123" \
  http://127.0.0.1:8080/api/v1/users
```

## CLI Commands

```text
# Core
apicerberus start [--config path] [--pid-file path]
apicerberus stop [--pid-file path]
apicerberus version
apicerberus config validate <path>

# Config Management
apicerberus config export [--config path] [--out path]
apicerberus config import [--target path] <source>
apicerberus config diff <path1> <path2>

# User Management
apicerberus user list [--config path] [--output json]
apicerberus user create --email --name [--credits] [--role]
apicerberus user get <id> [--config path] [--output json]
apicerberus user update <id> [--name] [--rate-limit-rps]
apicerberus user suspend|activate <id>
apicerberus user apikey list --user <id>
apicerberus user apikey create --user <id> --name <name> [--mode test|live]
apicerberus user apikey revoke --user <id> --key <key-id>
apicerberus user permission list --user <id>
apicerberus user permission grant --user <id> --route <route> --methods <methods>
apicerberus user permission revoke --user <id> --permission <id>
apicerberus user ip list --user <id>
apicerberus user ip add --user <id> --ip <cidr>
apicerberus user ip remove --user <id> --ip <cidr>

# Credit Management
apicerberus credit overview [--config path] [--output json]
apicerberus credit balance --user <id>
apicerberus credit topup --user <id> --amount <n> --reason <text>
apicerberus credit deduct --user <id> --amount <n> --reason <text>
apicerberus credit transactions --user <id>

# Audit & Analytics
apicerberus audit search [--config path] [--output json] [--user] [--route] [--since]
apicerberus audit tail [--config path] [--follow]
apicerberus audit detail <id>
apicerberus audit export [--format csv|json|jsonl]
apicerberus audit stats
apicerberus audit cleanup --older-than-days <n>
apicerberus audit retention show|set --days <n>
apicerberus analytics overview [--config path] [--output json]
apicerberus analytics requests [--config path]
apicerberus analytics latency [--config path]

# Gateway Entities
apicerberus service list|add|get|update|delete
apicerberus route list|add|get|update|delete
apicerberus upstream list|add|get|update|delete

# MCP Server
apicerberus mcp start [--transport stdio|sse] [--port 3000] [--config path]
```

## Admin API Overview

The admin server is protected by `X-Admin-Key` header.

### Core Endpoints
- System: `/admin/api/v1/status`, `/info`, `/config/reload`, `/config/export`, `/config/import`
- Real-time: `/admin/api/v1/ws` (WebSocket)

### Gateway Management
- Services CRUD: `/admin/api/v1/services`
- Routes CRUD: `/admin/api/v1/routes`
- Upstreams CRUD + targets + health: `/admin/api/v1/upstreams`

### User Management
- Users CRUD: `/admin/api/v1/users`
- User operations: `/admin/api/v1/users/{id}/suspend`, `/activate`, `/reset-password`
- API keys: `/admin/api/v1/users/{id}/api-keys`
- Permissions: `/admin/api/v1/users/{id}/permissions`
- IP whitelist: `/admin/api/v1/users/{id}/ip-whitelist`
- Credit: `/admin/api/v1/users/{id}/credits/*`

### Audit & Analytics
- Audit logs: `/admin/api/v1/audit-logs` (search, export, cleanup)
- Analytics: `/admin/api/v1/analytics/*` (overview, timeseries, top routes, latency, etc.)

### Alerts
- Alert rules: `/admin/api/v1/alerts`
- Alert history: `/admin/api/v1/alerts/history`

## Tests

Run the full test suite:

```bash
go test ./...
```

Run only end-to-end tests:

```bash
go test ./test
```

Run with race detection:

```bash
go test -race ./...
```

## CI/CD

This project uses GitHub Actions for continuous integration and deployment.

### Workflows

- **CI** (`.github/workflows/ci.yml`) - Runs on every PR and push to main:
  - Lint with golangci-lint
  - Unit tests with coverage
  - Build for multiple platforms (Linux, macOS, Windows)
  - Integration tests
  - Security scanning (Trivy, gosec, govulncheck)

- **Release** (`.github/workflows/release.yml`) - Triggered on version tags:
  - Creates GitHub release with changelog
  - Builds multi-arch binaries (amd64, arm64)
  - Publishes Docker images to GitHub Container Registry
  - Updates Helm chart repository

### Automated Security Scanning

Security scans run on every build:
- **Trivy** - Container image vulnerability scanning
- **gosec** - Go security code analysis
- **govulncheck** - Go vulnerability database check

### Dependabot

Automated dependency updates are configured for:
- Go modules (weekly)
- npm packages (weekly)
- GitHub Actions (weekly)
- Docker images (weekly)

## Docker

Build and run using Docker:

```bash
docker build -t apicerberus:local .
docker run --rm -p 8080:8080 -p 9876:9876 apicerberus:local
```

## Repository Layout

- `cmd/apicerberus` - application entrypoint
- `internal` - gateway, plugins, admin API, billing, store, config
- `test` - E2E and integration tests
- `web` - dashboard assets
- `.project` - product docs, roadmap, and task breakdown

## Roadmap

Completed milestones:

- `v0.2.0`: gRPC support (HTTP/2, gRPC-Web, transcoding)
- `v0.3.0`: GraphQL support (query depth, complexity, subscriptions)
- `v0.4.0`: GraphQL Federation (schema composition, query planning)
- `v0.5.0`: Raft Clustering (HA, distributed rate limiting)
- `v0.6.0`: Advanced features (caching, Prometheus, OpenTelemetry)
- `v0.7.0`: Enterprise (RBAC, SSO, white-label)
- `v1.0.0`: Production release with CI/CD and documentation

See the full plan in [`.project/TASKS.md`](./.project/TASKS.md).
