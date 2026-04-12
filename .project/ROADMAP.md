# Project Roadmap

> Based on comprehensive codebase analysis performed on 2026-04-11
> This roadmap prioritizes work needed to maintain production quality.

---

## Current State Assessment

APICerebrus is a **feature-complete** API Gateway at v1.0.0-rc.1 with:
- 206 Go source files, 55,400 LOC
- 163 frontend files, ~25,000 LOC
- 32 test packages, all passing
- ~85% overall test coverage
- 70+ Admin API endpoints
- 11 load balancing algorithms
- 30+ plugin types
- Raft clustering, GraphQL Federation, MCP server

**What's working well:**
- All 32 test packages pass with zero failures
- Core proxy path (gateway → router → plugin pipeline → load balancer → upstream) is solid
- Comprehensive plugin architecture with 5-phase pipeline
- Multi-layered security implementation
- Docker, Kubernetes, Docker Swarm deployment support
- Hot config reload with version history
- Graceful shutdown with LIFO hook execution

**Remaining work items for production maturity:**

### High Priority
1. **ServeHTTP refactoring** — `gateway/server.go:191-597` (400+ lines) is the primary maintainability concern
2. **Type coercion cleanup** — Some duplication remains despite `pkg/coerce` package
3. **Audit monitoring** — Expose dropped audit entries counter to `/metrics` endpoint

### Medium Priority
4. **Error type standardization** — Mix of custom error structs and `fmt.Errorf` across packages
5. **Load testing validation** — No production-scale load testing performed yet
6. **GraphQL federation stress test** — Query planner tested but not under heavy load

### Low Priority
7. **Documentation updates** — README stats appear current but verify periodically
8. **Frontend test expansion** — 13 test files good but could expand coverage

---

## Phase 1: Critical Fixes (Week 1-2)

### Must-fix items for production readiness

- [ ] **Extract ServeHTTP handler** — Split `gateway/server.go:191-597` (400 lines) into sub-handlers:
  - `serve_auth.go` — Authentication phase (40 lines)
  - `serve_billing.go` — Billing pre/post proxy (197 lines)
  - `serve_proxy.go` — Proxy forwarding (172 lines)
  - `serve_audit.go` — Audit logging (114 lines)
  - `request_state.go` — Request state management (111 lines)
  - `ServeHTTP` orchestrator reduced to ~124 lines

- [ ] **Expose audit drop counter** — Add `audit_dropped` metric to Prometheus `/metrics` endpoint

- [ ] **Verify JWT replay cache bounds** — Confirm `JTIReplayCache` maxSize and eviction work correctly under load

---

## Phase 2: Core Completion (Week 3-4)

### Complete missing minor features

- [ ] **Type coercion deduplication** — Audit remaining type coercion helpers across packages, consolidate into `pkg/coerce`

- [ ] **Error type standardization** — Audit custom error types vs `fmt.Errorf` usage, document error handling convention

- [ ] **Production load test** — Run sustained load test at 5K-10K req/s, verify:
  - SQLite write performance under load
  - Audit log drop rate
  - Memory growth over time
  - Connection pool behavior

---

## Phase 3: Hardening (Week 5-6)

### Security, error handling, edge cases

- [ ] **Security audit fixes** — Review any `gosec` findings marked "accepted-risk" and document justification

- [ ] **Input validation gaps** — Audit admin API input validation on all endpoints

- [ ] **Circuit breaker tuning** — Document recommended circuit breaker thresholds for production

- [ ] **Rate limit documentation** — Clarify auto-enforcement vs per-route configuration

---

## Phase 4: Testing (Week 7-8)

### Comprehensive test coverage

- [ ] **Frontend test expansion** — Increase from 13 test files to 20+ covering:
  - API client library
  - WebSocket connection
  - Key user flows (login, API key creation, playground)
  - Chart components

- [ ] **Integration test gaps** — Add integration tests for:
  - Raft cluster join/leave
  - Hot reload with config changes
  - Kafka audit streaming
  - OIDC SSO flow

- [ ] **E2E test expansion** — Increase Playwright coverage for:
  - Full admin workflow
  - Portal playground
  - Real-time dashboard updates

---

## Phase 5: Performance & Optimization (Week 9-10)

### Performance tuning and optimization

- [ ] **SQLite write optimization** — If production load tests reveal issues:
  - Consider connection pool tuning
  - Evaluate batch size for audit inserts
  - Consider write-ahead logging settings

- [ ] **Frontend bundle optimization** — Verify bundle analyzer shows acceptable sizes

- [ ] **Cache tuning** — Evaluate cache hit rates and sizing for:
  - GraphQL query cache
  - JWKS cache TTL
  - Plugin config cache

- [ ] **Memory profiling** — Run `pprof` under load to identify any leaks

---

## Phase 6: Documentation & DX (Week 11-12)

### Documentation and developer experience

- [ ] **OpenAPI spec validation** — Verify `docs/openapi.yaml` matches actual API endpoints

- [ ] **Update README** — Verify all stats are current (Go version, file counts, coverage)

- [ ] **Architecture diagrams** — Verify architecture diagrams in docs match actual implementation

- [ ] **Contributing guide** — Expand with:
  - Code review checklist
  - Commit message convention examples
  - Testing requirements

---

## Phase 7: Release Preparation (Week 13-14)

### Final production preparation

- [ ] **CI/CD pipeline completion** — Verify all CI checks pass reliably:
  - `make ci` completes without flaky failures
  - Frontend E2E tests stable

- [ ] **Docker production image** — Final review:
  - Multi-stage build size optimized
  - Non-root user configured
  - Healthcheck working

- [ ] **Release automation** — Verify `.goreleaser.yml` produces clean releases for:
  - Linux amd64/arm64
  - macOS amd64/arm64
  - Windows

- [ ] **Monitoring setup guide** — Document:
  - Prometheus scrape config
  - Key metrics to alert on
  - Dashboard panels for Grafana

---

## Beyond v1.0: Future Enhancements

### Features and improvements for future versions

- [ ] **PostgreSQL backend** — For high-throughput deployments needing horizontal write scaling

- [ ] **gRPC rate limiting** — Apply rate limiting to gRPC endpoints

- [ ] **GraphQL subscription scaling** — Multiple concurrent subscription support optimization

- [ ] **Multi-tenant isolation** — Organization-level resource quotas and billing

- [ ] **API key rotation** — Automatic key rotation with zero downtime

---

## Effort Summary

| Phase | Description | Estimated Hours | Priority | Dependencies |
|-------|-------------|----------------|----------|--------------|
| Phase 1 | Critical Fixes | 16h | CRITICAL | None |
| Phase 2 | Core Completion | 24h | HIGH | Phase 1 |
| Phase 3 | Hardening | 24h | HIGH | Phase 2 |
| Phase 4 | Testing | 32h | HIGH | Phase 3 |
| Phase 5 | Performance | 24h | MEDIUM | Phase 4 |
| Phase 6 | Documentation | 16h | MEDIUM | Phase 5 |
| Phase 7 | Release Prep | 16h | HIGH | Phase 6 |
| **Total** | | **152h (~4 weeks)** | | |

---

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| SQLite write contention under production load | Medium | High | Monitor audit drops, add Redis fallback for high-throughput |
| ServeHTTP refactoring introduces bugs | Low | High | Comprehensive test coverage before refactoring |
| Memory growth under attack (JWT cache) | Low | Medium | Already has bounds, verify under load test |
| Frontend test flakiness | Low | Low | Playwright has improved stability |
| Dependency CVE in third-party libs | Low | High | Already running `govulncheck` in CI |

---

## Go/No-Go Recommendation

**GO — Ready for production deployment** (with standard operational precautions)

APICerebrus has achieved feature completeness at v1.0.0-rc.1 with all 32 test packages passing. The core proxy path is solid, security is well-implemented, and the operational infrastructure (Docker, Kubernetes, monitoring) is in place.

**Recommended operational precautions:**
1. Enable Prometheus monitoring and set up alerts on key metrics
2. Configure audit log streaming to Kafka for production-scale durability
3. Use Redis for distributed rate limiting in multi-node deployments
4. Start with a pilot deployment handling non-critical traffic
5. Monitor SQLite write performance under actual load

**Do NOT deploy without:**
- A valid admin API key (minimum 32 characters, cryptographically random)
- TLS enabled in production (ACME or manual certificates)
- Reasonable audit retention policy configured
- Health check monitoring on `/ready` endpoint
