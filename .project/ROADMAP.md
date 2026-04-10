# Project Roadmap

> Based on comprehensive codebase analysis performed on 2026-04-10
> This roadmap prioritizes work needed to bring the project to production quality.

## Current State Assessment

APICerebrus is a feature-rich API Gateway at v1.0.0-rc.1 with 170K+ Go LOC, 24K+ frontend LOC, 3,398 passing tests, and 100+ admin API endpoints. The core functionality is solid — routing, proxying (HTTP/gRPC/GraphQL), authentication, rate limiting, billing, audit logging, Raft clustering, and MCP server are all implemented and mostly working.

**Key blockers for production readiness:**
1. 17 failing tests (including critical E2E flows for billing, audit, permissions)
2. SQLite write contention causing silent data loss (audit drops, API key tracking failures)
3. No database migration framework — schema changes are risky
4. E2E test infrastructure is unstable

**What's working well:**
- Core gateway proxy with radix tree router
- All 10 load balancing algorithms
- 4 rate limiting algorithms + Redis distributed
- Plugin pipeline with 20+ plugins
- User management with credit billing
- OpenTelemetry tracing, Prometheus metrics
- Raft clustering with mTLS
- React 19 dashboard with modern stack

## Phase 1: Critical Fixes (Week 1-2) — ✅ COMPLETE

### Must-fix items blocking basic functionality

- [x] **Fix SQLite write contention** — Retry with exponential backoff added to billing deduction (`internal/gateway/server.go:1075-1088`); busy timeout increased to 5s.
- [x] **Fix 5 admin unit test failures** — All passing.
- [x] **Fix `TestGatewayBillingRejectDeductAndTestKeyBypass`** — Removed `t.Parallel()`, added SQLITE_BUSY retry, increased busy timeout.
- [x] **Fix `TestPluginAbortScenarios`** — Passing.
- [x] **Remove WASM plugin claim from README** — Feature IS implemented (`internal/plugin/wasm.go`); README corrected.
- [x] **Remove Plugin Marketplace claim from README** — Feature IS implemented (`internal/plugin/marketplace.go`); README corrected.
- [x] **Security: remediate 11 Dependabot vulnerabilities** — Go 1.26.2 (6 stdlib vulns), gRPC v1.79.3 (auth bypass), go-redis v9.7.3, Vite v8.0.5 (3 CVEs).

## Phase 2: Core Completion (Week 3-6)

### Complete missing core features from specification

- [x] **Stabilize E2E test infrastructure** — All E2E tests passing including:
  - TestE2EAdminConfigureAndProxy
  - TestE2EHotReloadWithConfigWatch
  - TestChaosUpstreamPanicRecovery
  - TestChaosRateLimiterLocalFallback
  - TestChaosCorruptedDatabase
  - TestChaosUpstreamConnectionFailure
  - All integration tests (auth, cluster, gateway, plugins, lifecycle)

- [ ] **Implement database migration framework** — Integrate `golang-migrate/migrate` or equivalent. Create initial migration from current SQLite schema. Add migration commands to CLI. Files: new `internal/migrations/` package. Spec reference: data model in SPEC §16-19. Effort: 8h.

- [ ] **Improve `internal/pkg/jwt` test coverage** — Currently 61.8%. Add tests for ES256, EdDSA, nbf validation, jti replay cache. Effort: 4h.

- [ ] **Improve `internal/portal` test coverage** — Currently 76.3%. Add tests for portal handlers (login, API key management, playground). Effort: 4h.

- [ ] **Audit log retry on SQLite busy** — When batch insert fails, retry with exponential backoff before dropping. Add dead-letter queue for permanently failed entries. Files: `internal/audit/logger.go`. Effort: 4h.

## Phase 3: Hardening (Week 7-8)

### Security, error handling, edge cases

- [ ] **Add input validation to all admin API path/query parameters** — Currently some endpoints may not validate ID formats, pagination params, date ranges. Effort: 8h.
- [ ] **Add request ID to all error responses** — Ensure every error response includes a correlation/request ID for audit trail lookup. Effort: 2h.
- [ ] **Implement rate limiting on admin API endpoints** — Currently admin API may not have rate limits. Protect against brute force on auth endpoints. Effort: 4h.
- [ ] **Add graceful degradation for Redis unavailability** — When Redis is down, distributed rate limiting should fall back to local mode without errors. Files: `internal/ratelimit/`. Effort: 4h.
- [ ] **Add circuit breaker for upstream health check dependencies** — Prevent cascading failures when many upstreams go down simultaneously. Effort: 2h.
- [ ] ** Harden CSP headers** — Review and tighten Content-Security-Policy for admin and portal. Effort: 2h.
- [ ] **Add security headers to all responses** — HSTS, X-Content-Type-Options, X-Frame-Options, Referrer-Policy. Effort: 2h.

## Phase 4: Testing (Week 9-10)

### Comprehensive test coverage

- [ ] **Add fuzz tests for router** — Fuzz the radix tree router with malformed paths, Unicode, null bytes, path traversal attempts. Files: `internal/gateway/router_test.go`. Effort: 4h.
- [ ] **Add fuzz tests for YAML parser** — Fuzz config parsing with malformed YAML. Files: `internal/pkg/yaml/`. Effort: 2h.
- [ ] **Add fuzz tests for JSON parser** — Fuzz JSON request body parsing. Files: `internal/pkg/json/`. Effort: 2h.
- [ ] **Add load testing framework** — Use `vegeta` or `k6` for sustained load testing. Target: 50K req/s per spec. Effort: 8h.
- [ ] **Add frontend component tests** — Vitest tests for key React components (data tables, forms, charts). Target: 60% frontend coverage. Effort: 16h.
- [ ] **Add frontend E2E tests** — Playwright tests for admin dashboard flows (login, create service/route, view analytics). Effort: 16h.
- [ ] **Raise overall coverage to 80%+** — Focus on `internal/pkg/jwt` (61.8%), `test/helpers` (48.0%), `internal/cli` (76.8%), `internal/portal` (76.3%). Effort: 8h.

## Phase 5: Performance & Optimization (Week 11-12)

### Performance tuning and optimization

- [ ] **Parallelize plugin execution within phases** — Plugins in the same phase that don't share state can run concurrently. Files: `internal/plugin/pipeline.go`. Effort: 8h.
- [ ] **Object pool for audit log entries** — Reduce allocations in hot path. Files: `internal/audit/logger.go`. Effort: 2h.
- [ ] **Optimize JSON marshaling in admin API** — Use `json.Encoder` streaming instead of `json.Marshal` + `write`. Files: `internal/pkg/json/`. Effort: 4h.
- [ ] **Add SQLite connection pool tuning** — Configure `SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime` based on workload. Effort: 2h.
- [ ] **Frontend bundle analysis** — Add `rollup-plugin-visualizer` to Vite config. Identify and eliminate dead code. Effort: 2h.
- [ ] **Frontend code splitting** — Ensure React Flow, CodeMirror, and Recharts are lazy-loaded only when needed. Effort: 4h.
- [ ] **Benchmark critical paths** — Add benchmarks for router matching, plugin pipeline, proxy forwarding. Track regression in CI. Effort: 4h.

## Phase 6: Documentation & DX (Week 13-14)

### Documentation and developer experience

- [ ] **Generate OpenAPI/Swagger spec** — Add `swaggo/swag` annotations to admin handlers or use `ogen` for code-first spec generation. Effort: 16h.
- [ ] **Update API.md** — Reflect all 100+ current endpoints (currently documents ~70). Effort: 4h.
- [ ] **Add architecture decision records (ADRs)** — Document key decisions: SQLite vs PostgreSQL, custom YAML parser, 5-phase pipeline, SubnetAware vs Geo-aware. Effort: 4h.
- [ ] **Add contributing guide with dev environment setup** — Docker compose for local development with SQLite, Redis, upstream mock. Effort: 4h.
- [ ] **Add troubleshooting guide** — Common issues: SQLite locked, Redis connection, certificate renewal, Raft cluster join failures. Effort: 4h.
- [ ] **Add `.goreleaser.yml`** — Multi-platform binary releases with version info embedded. Effort: 4h.
- [ ] **Add GitHub Actions CI workflow** — Auto-run tests, lint, security scans on PR. Effort: 4h.

## Phase 7: Release Preparation (Week 15-16)

### Final production preparation

- [ ] **Run full security audit** — Re-run gosec, govulncheck, trivy. Verify all findings resolved. Effort: 4h.
- [ ] **Docker image optimization** — Use `distroless/static` or `alpine` base. Minimize image size. Add non-root user. Effort: 2h.
- [ ] **Add health check endpoint** — Comprehensive `/health` that checks DB, Redis (if configured), Raft status. Effort: 2h.
- [ ] **Add readiness probe endpoint** — `/ready` that returns 503 until all dependencies are initialized. Effort: 2h.
- [ ] **Configure log rotation** — Ensure production log rotation is documented and tested. Effort: 2h.
- [ ] **Test backup/restore procedure** — Verify `scripts/backup.sh` and `scripts/restore.sh` work end-to-end. Effort: 4h.
- [ ] **Zero-downtime deployment test** — Verify rolling update with Raft cluster works without request loss. Effort: 4h.
- [ ] **Release candidate validation** — Full test suite pass, security clean, performance targets met. Effort: 8h.

## Beyond v1.0: Future Enhancements

- [ ] **SSO/OIDC Integration** (v0.7.0) — OAuth2/OIDC provider support for enterprise SSO
- [ ] **RBAC** (v0.7.0) — Role-based access control beyond admin/user binary
- [ ] **White-label Branding** (v0.7.0) — Customizable dashboard branding
- [ ] **WASM Plugin Runtime** — `wazero`-based WASM plugin execution for user-defined plugins
- [ ] **Plugin Marketplace** — Discovery and installation mechanism for community plugins
- [ ] **True Geographic Load Balancing** — MaxMind GeoIP2 integration for location-aware routing
- [ ] **gRPC Streaming Health Checks** — Native gRPC health checking protocol
- [ ] **GraphQL APQ** — Automatic Persisted Queries for production GraphQL
- [ ] **ACME DNS-01 Challenge** — Wildcard certificate support
- [ ] **Multi-tenant Clustering** — Per-tenant Raft groups for isolation

## Effort Summary

| Phase | Estimated Hours | Priority | Dependencies |
|-------|----------------|----------|-------------|
| Phase 1: Critical Fixes | 13h | CRITICAL | None |
| Phase 2: Core Completion | 38h | HIGH | Phase 1 |
| Phase 3: Hardening | 26h | HIGH | Phase 1 |
| Phase 4: Testing | 50h | HIGH | Phase 2 |
| Phase 5: Performance & Optimization | 28h | MEDIUM | Phase 2 |
| Phase 6: Documentation & DX | 38h | MEDIUM | Phase 1 |
| Phase 7: Release Preparation | 28h | HIGH | Phase 4, 5 |
| **Total** | **221h** | | |

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| SQLite write contention worsens under production load | High | High | Implement retry + backoff immediately (Phase 1); consider PostgreSQL migration for v2.0 |
| E2E tests remain flaky after fixes | Medium | Medium | Isolate tests with dedicated fixtures; consider testcontainers for clean DB per test |
| Migration framework introduction breaks existing deployments | Medium | High | Test migrations extensively; provide rollback scripts; document migration process |
| Performance targets (50K req/s) not met with sequential plugin pipeline | Medium | Medium | Parallelize plugins (Phase 5); benchmark early to identify bottlenecks |
| Frontend bundle grows beyond acceptable size | Low | Low | Add bundle analysis (Phase 5); implement code splitting |
| Raft cluster instability under network partition | Low | High | Test partition scenarios; improve Raft transport resilience |
| Security regression after fixes | Medium | High | Add security scanning to CI (Phase 7); run gosec/govulncheck on every PR |
