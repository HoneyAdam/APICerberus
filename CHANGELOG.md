# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Web Dashboard**: Cluster topology visualization with real-time updates
  - Added ClusterTopology component with dagre layout algorithm
  - Added useClusterRealtime hook for WebSocket/SSE-based real-time cluster status
  - Added cluster management page with node visualization
  - Added comprehensive tests for cluster components

- **Federation**: GraphQL subscription and resilience improvements
  - Added WebSocket-based subscription support for federated GraphQL
  - Added query cache for parsed query plans with TTL
  - Added circuit breaker pattern for subgraph calls
  - Added query optimizer with parallel execution groups

- **Metrics**: Prometheus metrics collection
  - Added request latency histograms
  - Added error rate counters by upstream
  - Added health status metrics
  - Added custom metric labels support

- **Testing**: Comprehensive test coverage improvements
  - Added admin handler error path tests (rate limiting, plugin configs)
  - Added federation executor tests
  - Added gateway balancer and helper tests
  - Added MCP server helper function tests
  - Added portal handler tests
  - Added raft CertFSM error path tests
  - Added store repository tests
  - Added plugin auth and claim value tests

### Changed

- **GraphQL Parser**: Improved inline fragment support
  - Added parseInlineFragment function
  - Enhanced error handling for fragment spreads
  - Added type condition validation

- **Web**: Improved test infrastructure
  - Migrated test files to .tsx for proper JSX support
  - Fixed React act() warnings in async tests
  - Added lint and typecheck npm scripts

## [1.0.0] - 2025-03-15

### Added

- Production release with full feature set
- Complete CI/CD pipeline with GitHub Actions
- Security scanning with Trivy, gosec, and govulncheck
- Multi-arch Docker images (amd64, arm64)
- Helm chart for Kubernetes deployment

## [0.1.0] - 2024-12-15

### Added

- Initial release with core gateway features
- Admin API and web dashboard
- Plugin system with 20+ plugins
- CLI with 40+ commands

