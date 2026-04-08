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

### Changed
- Updated CLAUDE.md with improved structure
- Improved test coverage for admin package (53.1%)

## [1.0.0] - 2026-04-08

### Added
- Initial production release
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
- 85%+ test coverage

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
