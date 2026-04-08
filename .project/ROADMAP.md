# APICerebrus — Prioritized Roadmap to Production

**Audit Date:** 2026-04-08  
**Status:** Post-Audit Action Plan  
**Target:** Achieve defensible v1.0.0 production readiness within 8–10 weeks.

---

## Phase 1: Critical Blockers (Weeks 1–2)

**Goal:** Remove all P0 issues that would cause immediate production incidents or security compromises.

### 1.1 Security Hardening Sprint

| # | Task | Owner | Acceptance Criteria |
|---|------|-------|---------------------|
| 1.1.1 | Fix all G104 (unhandled error) issues in production code | Backend Team | `gosec ./...` returns ≤ 10 issues (focusing on non-test code) |
| 1.1.2 | Handle `conn.Close()`, `SetReadDeadline()`, and `sendPong()` errors in `internal/admin/ws_hub.go` | Backend Team | Zero gosec issues in `ws_hub.go` |
| 1.1.3 | Handle all `WriteJSON` / `w.Write` errors in `internal/admin/` handlers | Backend Team | `golangci-lint` clean on `internal/admin/*.go` (excluding tests) |
| 1.1.4 | Add scoped admin API tokens with TTL (minimum: JWT tokens with `exp` claim, replace static key for non-bootstrap ops) | Backend Team | Admin endpoints accept `Authorization: Bearer <token>`; static key restricted to bootstrap only |
| 1.1.5 | Add IP allow-list option for admin API | Backend Team | Config field `admin.allowed_ips: []` enforced before auth |

### 1.2 Database Scalability Fix

| # | Task | Owner | Acceptance Criteria |
|---|------|-------|---------------------|
| 1.2.1 | Remove `MaxOpenConns(1)` from `store.Open` | Backend Team | `MaxOpenConns` set to `cfg.Store.MaxOpenConns` (default 25) |
| 1.2.2 | Verify WAL mode and busy_timeout still prevent "database is locked" under load | Backend Team | Run `k6` or `wrk` with 100 concurrent connections for 60s; zero lock errors |
| 1.2.3 | Add connection pool metrics (open conns, wait duration) to `/admin/api/v1/status` | Backend Team | Status endpoint includes `store.connections` object |

### 1.3 Frontend Test Stability

| # | Task | Owner | Acceptance Criteria |
|---|------|-------|---------------------|
| 1.3.1 | Fix `UserRoleManager.test.tsx` failures (permission count mismatch, checkbox state) | Frontend Team | `npm run test:run` passes for this file |
| 1.3.2 | Fix `ClusterTopology.test.tsx` React Flow rendering issues | Frontend Team | File passes; if unsolvable, mock React Flow and test logic only |
| 1.3.3 | Fix remaining 4 failing test files | Frontend Team | `npm run test:run` returns 0 failures |
| 1.3.4 | Add CI gate: block merge on web test failures | DevOps | GitHub Actions `web-test` job is required status check |

### 1.4 Documentation Honesty

| # | Task | Owner | Acceptance Criteria |
|---|------|-------|---------------------|
| 1.4.1 | Update CHANGELOG.md to reflect actual 81.2% test coverage | Docs/PM | All coverage claims are evidence-based |
| 1.4.2 | Remove "zero external dependencies" claim from README, SPECIFICATION.md, and IMPLEMENTATION.md | Docs/PM | Replaced with "minimal, curated dependencies" or accurate list |
| 1.4.3 | Change version narrative from "v1.0.0 Production Release" to "v1.0.0-rc.1" or "v0.9.5" | Docs/PM | No production assertions until Phase 4 complete |

---

## Phase 2: Operational Readiness (Weeks 3–4)

**Goal:** Ensure the system behaves predictably under operator and edge-case scenarios.

### 2.1 Audit Log Safety

| # | Task | Owner | Acceptance Criteria |
|---|------|-------|---------------------|
| 2.1.1 | Change default `store_request_body` and `store_response_body` to `false` in example configs | Backend Team | `apicerberus.example.yaml` defaults to safe settings |
| 2.1.2 | Implement recursive JSON body field masking (not just top-level string replacement) | Backend Team | Audit log tests verify nested `user.password` is masked |
| 2.1.3 | Add PII detection warning banner in admin UI when body logging is enabled | Frontend Team | Admin dashboard shows alert: "Sensitive data may be logged" |

### 2.2 Admin Server Resource Management

| # | Task | Owner | Acceptance Criteria |
|---|------|-------|---------------------|
| 2.2.1 | Add `Server.Close()` method that stops rate-limit cleanup ticker and WebSocket hubs | Backend Team | `goleak` test confirms no goroutine leaks after server shutdown |
| 2.2.2 | Ensure `Gateway.Shutdown()` closes all listeners and drains in-flight requests within 10s | Backend Team | Integration test verifies graceful shutdown |

### 2.3 Performance Benchmarking

| # | Task | Owner | Acceptance Criteria |
|---|------|-------|---------------------|
| 2.3.1 | Write `BenchmarkServeHTTP` in `internal/gateway/` | Backend Team | Benchmark runs `httptest.Server` → gateway proxy for 10s |
| 2.3.2 | Run `wrk` or `k6` against local gateway with 1,000 concurrent connections | Backend Team | Document throughput and p99 latency for simple proxy path |
| 2.3.3 | Update SPECIFICATION.md with *measured* performance numbers, not targets | Docs/PM | Replace "50,000+ req/sec" with actual observed throughput |

### 2.4 Frontend Bundle Optimization

| # | Task | Owner | Acceptance Criteria |
|---|------|-------|---------------------|
| 2.4.1 | Implement route-based code splitting with `React.lazy()` and dynamic imports | Frontend Team | No single chunk > 500KB after build |
| 2.4.2 | Verify lazy-loaded routes do not break `@tanstack/react-query` prefetching | Frontend Team | All admin/portal routes load correctly in preview build |

---

## Phase 3: Subsystem Hardening (Weeks 5–6)

**Goal:** Harden complex subsystems (Raft, GraphQL Federation, gRPC) for real-world failure modes.

### 3.1 Raft Cluster Torture Tests

| # | Task | Owner | Acceptance Criteria |
|---|------|-------|---------------------|
| 3.1.1 | Add network partition simulation using `toxiproxy` or custom transport partitioning | Backend Team | Test: partition leader from majority → verify new leader elected, no split-brain |
| 3.1.2 | Add test for log divergence recovery after partition heals | Backend Team | Divergent follower catches up via AppendEntries or snapshot |
| 3.1.3 | Add chaotic node restart test (kill random node, restart after random delay) | Backend Team | Cluster remains available with quorum; no data loss |
| 3.1.4 | Document known Raft limitations in `docs/raft-limitations.md` | Docs/PM | List unhandled edge cases (e.g., membership changes) |

### 3.2 GraphQL Federation Hardening

| # | Task | Owner | Acceptance Criteria |
|---|------|-------|---------------------|
| 3.2.1 | Remove `@key` heuristic fallback in `internal/federation/composer.go` | Backend Team | `Compose()` returns error if entity lacks `@key` directive |
| 3.2.2 | Add query plan cache with TTL | Backend Team | Repeated identical queries use cached plan; cache hit metrics exposed |
| 3.2.3 | Add federation error boundary tests (missing subgraph, schema conflict, circular reference) | Backend Team | 10+ error-case tests in `federation/` |
| 3.2.4 | Validate subscription federation over WebSocket end-to-end | Backend Team | E2E test: federated subscription streams events |

### 3.3 gRPC Transcoder Depth

| # | Task | Owner | Acceptance Criteria |
|---|------|-------|---------------------|
| 3.3.1 | Add transcoder tests for `oneof`, nested messages, enums, and maps | Backend Team | 15+ transcoder test cases covering all protobuf scalar/complex types |
| 3.3.2 | Add transcoder error handling for unknown fields and type mismatches | Backend Team | Returns 400 with descriptive JSON error body |

### 3.4 Rate Limiter Edge Cases

| # | Task | Owner | Acceptance Criteria |
|---|------|-------|---------------------|
| 3.4.1 | Add Redis rate limiter reconnection logic with exponential backoff | Backend Team | Redis disconnection does not crash gateway; falls back to memory or rejects gracefully |
| 3.4.2 | Add sliding window precision tests with sub-second granularity | Backend Team | Tests verify behavior at window boundaries |

---

## Phase 4: Production Certification (Weeks 7–8)

**Goal:** Complete validation, security review, and operational runbooks.

### 4.1 Security Audit

| # | Task | Owner | Acceptance Criteria |
|---|------|-------|---------------------|
| 4.1.1 | Run `govulncheck` and fix all HIGH/CRITICAL vulnerabilities | Security/Dev | Clean `govulncheck ./...` |
| 4.1.2 | Run Trivy container scan on built Docker image | Security/Dev | Zero CRITICAL vulnerabilities; document all HIGH findings |
| 4.1.3 | Perform manual pentest on admin API (auth bypass, IDOR, injection) | Security/Dev | Report with findings and remediation status |
| 4.1.4 | Review audit log encryption-at-rest strategy | Security/Dev | Decision recorded: encrypt SQLite files via OS-level encryption or application-layer |

### 4.2 Operational Runbooks

| # | Task | Owner | Acceptance Criteria |
|---|------|-------|---------------------|
| 4.2.1 | Write runbook for "Gateway returning 502 to all routes" | SRE | Includes health check, upstream verification, config reload steps |
| 4.2.2 | Write runbook for "Raft cluster loses quorum" | SRE | Includes manual intervention, forced snapshot, node rejoin steps |
| 4.2.3 | Write runbook for "SQLite database locked errors" | SRE | Includes connection pool check, WAL file cleanup, restart procedure |
| 4.2.4 | Add Kubernetes readiness/liveness probes to Helm charts | DevOps | Probes hit `/admin/api/v1/status` with documented thresholds |

### 4.3 Chaos Engineering Lite

| # | Task | Owner | Acceptance Criteria |
|---|------|-------|---------------------|
| 4.3.1 | Run gateway under `k6` with random upstream failures | Backend Team | p99 latency and error rate recorded; no gateway crashes |
| 4.3.2 | Run admin API under rapid config reload stress | Backend Team | 100 reloads in 60s; no memory leaks or goroutine Growth |
| 4.3.3 | Test WebSocket tail with 1,000 concurrent dashboard clients | Frontend+Backend | No hub crashes; message delivery stable |

### 4.4 Test Suite Cleanup

| # | Task | Owner | Acceptance Criteria |
|---|------|-------|---------------------|
| 4.4.1 | Identify and remove synthetic coverage-padding tests | Backend Team | Delete or consolidate `additional_test.go`, `100_test.go`, `coverage_test.go` where tests are trivial |
| 4.4.2 | Ensure total coverage stays ≥ 75% after cleanup | Backend Team | Run `go test -cover ./...` and verify total |
| 4.4.3 | Add behavior-driven integration tests for critical paths | Backend Team | Auth → rate limit → billing → proxy → audit E2E |

---

## Phase 5: Public Release (Week 9–10)

**Goal:** Declare v1.0.0 with confidence.

### 5.1 Release Criteria Checklist

- [ ] All P0 and P1 issues resolved
- [ ] `gosec` issues in production code ≤ 5
- [ ] Go test coverage ≥ 80% (actual, not inflated)
- [ ] Frontend tests 100% passing
- [ ] Performance benchmarks documented and reproducible
- [ ] Raft partition tests pass
- [ ] Security scan (Trivy + gosec + govulncheck) clean
- [ ] Helm charts include readiness/liveness probes
- [ ] Runbooks written and reviewed
- [ ] CHANGELOG, README, and SPECIFICATION.md claims verified

### 5.2 Release Activities

| # | Task | Owner | Acceptance Criteria |
|---|------|-------|---------------------|
| 5.2.1 | Tag `v1.0.0` in Git | Release Lead | Signed tag with release notes |
| 5.2.2 | Publish multi-arch Docker images (amd64, arm64) | CI/CD | Images pushed to registry with `v1.0.0` tag |
| 5.2.3 | Publish migration guide from Kong/Tyk (lightweight) | Docs/PM | At least one migration scenario documented |
| 5.2.4 | Announce release with accurate feature list and known limitations | Marketing/PM | No unverified performance claims |

---

## Quick Reference: Priority Definitions

| Priority | Response Time | Owner Examples |
|----------|--------------|----------------|
| **P0 — Critical** | Fix immediately before any deployment | Security bugs, data loss risks, total availability blockers |
| **P1 — High** | Fix within 1 sprint | Performance regressions, significant edge-case failures |
| **P2 — Medium** | Fix within 2–4 sprints | UX debt, bundle size, convenience improvements |
| **P3 — Low** | Backlog / nice-to-have | Refactoring, experimental features, docs polish |

---

## Summary Timeline

```
Week 1–2  ████████████████████  Phase 1: Critical Blockers
Week 3–4  ████████████████████  Phase 2: Operational Readiness
Week 5–6  ████████████████████  Phase 3: Subsystem Hardening
Week 7–8  ████████████████████  Phase 4: Production Certification
Week 9–10 ████████████          Phase 5: Public Release
```

**Bottom Line:** APICerebrus has the bones of a production-grade gateway. With 8–10 weeks of focused hardening on security, operations, and test discipline, it can credibly wear the v1.0.0 badge.

---

*End of Roadmap*
