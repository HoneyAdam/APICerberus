# Changelog

All notable changes to APICerebrus will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Comprehensive webhook tests with mock implementations
- Analytics forecast validation tests
- Integration test fixes for auth flow
- Git ignore rules for audit archives

### Fixed
- Integration test: API key ID extraction from nested response
- Integration test: Permission endpoint path correction
- Integration test: Session management array handling
- Config test: Portal default disabled test
- Concurrent map write bug in `optimized_pipeline.go` (Metadata map shared across goroutines)
- Flaky `Login.test.tsx` test (BrandingProvider context not mocked)

### Security
- Metadata map now deep-copied for parallel plugin execution

### Changed
- Updated CLAUDE.md with improved structure
- Improved test coverage for admin package (53.1%)

## [0.1.0] - 2026-04-12

> **First production release with complete WEBUI.md compliance**

### Added

- **Web Dashboard (WEBUI.md compliant)**
  - 3-way theme toggle (Light/Dark/System) with DropdownMenu
  - Flash prevention script to avoid white flash on page load
  - JetBrains Mono font via NPM packages (@fontsource-variable/jetbrains-mono)
  - Inter font via NPM packages (@fontsource-variable/inter)
  - Warm charcoal dark mode (#131316) - NOT pure black
  - Off-white light mode (#f8f8fa) - comfortable contrast
  - WCAG AA compliant color contrast ratios

- **Full WEBUI.md Compliance**
  - No hardcoded hex colors - all use CSS variables
  - No inline `style={}` props - use Tailwind classes
  - No `console.log` statements in production
  - No `any` types in production code
  - Named exports only (no default exports except lazy pages)
  - No barrel exports (no index.ts files)
  - TanStack Query for all data fetching
  - Skeleton loaders instead of spinners
  - Proper z-index scale usage

- **Infrastructure for Release**
  - GitHub Actions CI/CD workflow
  - GitHub Container Registry (ghcr.io) support
  - Semantic versioning with automated releases
  - Docker multi-stage build
  - Binary builds for multiple platforms (linux/amd64, linux/arm64, windows/amd64)

### Changed

- **Web Dashboard**
  - All hardcoded colors → `hsl(var(--success))`, `hsl(var(--destructive))`, etc.
  - All inline styles → Tailwind classes
  - Z-index values aligned with proper layered scale
  - `bg-emerald-500` → `bg-success`
  - `text-amber-600` → `text-warning`

- **Font Stack**
  - Removed Geist Mono references
  - Installed JetBrains Mono via @fontsource-variable
  - Installed Inter via @fontsource-variable
  - No Google Fonts CDN dependency

### Fixed

- CSS assets routing after process restart
- Hardcoded color values in ClusterTopology.tsx
- Hardcoded color values in RateLimitStats.tsx
- Z-index wars (z-[9999]) in TourTooltip.tsx
- Min-height inline styles in JSONViewer, YAMLEditor

### Security

- HttpOnly cookies for admin session authentication
- XSS-safe token transport
- Proper theme persistence with localStorage

## [1.0.0-rc.1] - 2026-04-08

### Added
- Initial release candidate
- HTTP/HTTPS reverse proxy with WebSocket support
- Radix Tree Router with O(k) path matching
- 10 load balancing algorithms
- API Key and JWT authentication
- Rate limiting (token bucket, fixed/sliding window, leaky bucket)
- Credit system with atomic transactions
- Audit logging with field masking
- Analytics engine with time-series data
- GraphQL Federation support
- gRPC support with HTTP transcoding
- Raft clustering for distributed consensus
- MCP Server for AI integration
- WebAssembly plugin support
- React-based admin dashboard
- User portal with API playground
- 40+ CLI commands
- 70+ Admin API endpoints
- 81.2% test coverage

### Security
- TLS 1.3 support with automatic certificate management
- Bot detection and IP restrictions
- CORS configuration
- Request/response transforms
- Field-level masking in audit logs

## [0.9.0] - 2026-04-01

### Added
- WebSocket real-time updates
- Plugin marketplace foundation
- OpenTelemetry tracing integration

## [0.8.0] - 2026-03-25

### Added
- GraphQL subscription support
- Redis-backed distributed rate limiting
- Kafka audit log streaming

## [0.7.0] - 2026-03-18

### Added
- WebAssembly plugin system
- Cache plugin with TTL
- Circuit breaker pattern

## [0.6.0] - 2026-03-11

### Added
- Health checks (active and passive)
- Connection pooling
- Graceful shutdown

## [0.5.0] - 2026-03-04

### Added
- Service mesh integration
- Load balancer health weighting

## [0.4.0] - 2026-02-25

### Added
- GraphQL Federation schema composition
- Query planning and execution

## [0.3.0] - 2026-02-18

### Added
- GraphQL support
- gRPC-Web support

## [0.2.0] - 2026-02-11

### Added
- gRPC server implementation
- HTTP/2 support

## [0.1.0] - 2026-02-04

### Added
- Core gateway implementation
- HTTP reverse proxy
- Basic routing
- Plugin system foundation
