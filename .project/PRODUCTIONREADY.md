# APICerebrus — Production Readiness Verdict

**Audit Date:** 2026-04-08  
**Auditor:** Claude Code (Autonomous Codebase Audit)  
**Scope:** Full-stack audit of Go backend, React frontend, infrastructure, and spec compliance.

---

## Executive Summary

APICerebrus is an **ambitious, feature-dense API gateway** with a surprisingly complete backend implementation. The Go codebase demonstrates real engineering effort across routing, load balancing, authentication, rate limiting, billing/credits, audit logging, GraphQL federation, gRPC proxying, Raft clustering, WebAssembly plugins, and MCP server integration. However, **the project suffers from inflated documentation claims, weak security hygiene, broken frontend tests, and a test suite that optimizes for coverage metrics over maintainability.**

**Final Verdict:**
> **Conditionally Ready for Staged Production Deployments** — *after* addressing critical security and operational blockers. Do not expose the admin API or portal to untrusted networks without remediation.

---

## Dimensional Scores (1–10)

| Dimension | Score | Rationale |
|-----------|-------|-----------|
| **Architecture** | 8/10 | Clean modular boundaries, good separation of concerns, hot-reload implemented, plugin pipeline is real and functional. Minor coupling issues between admin/gateway config mutation. |
| **Backend Code Quality** | 7/10 | Core logic is readable and well-structured. Test files are bloated with auto-generated coverage tests. Some lint issues in production code (unused struct fields, ineffectual assignments). |
| **Frontend Code Quality** | 5/10 | Extensive UI scaffolded with shadcn/ui, TanStack Query, React Router v7, and real data hooks. But 35% of frontend tests fail, indicating either broken components or brittle tests. |
| **Test Coverage & Reliability** | 6/10 | **Actual coverage: 81.2%** (not the claimed 85%+). Go tests pass. Frontend tests are broken. Many "coverage tests" are synthetic padding. |
| **Security Posture** | 4/10 | **186 gosec issues** across 149 files, overwhelmingly G104 (unhandled errors). No evidence of a dedicated security audit. Admin API uses a single static API key. TLS/ACME exists but error paths are ignored. |
| **Spec Compliance** | 7/10 | Core spec features are implemented. Some highly-advertised features (adaptive LB, geo-aware LB) are likely stubs or simplified. MCP server, Federation, Raft, and WASM plugins all have real code. |
| **Operational Readiness** | 6/10 | Docker multi-stage build, distroless base image, GitHub Actions CI, Helm charts, health checks, and graceful shutdown are present. But logging of sensitive data in audit paths and unhandled errors in operators' paths reduce confidence. |
| **Performance & Scalability** | 5/10 | No benchmarks were executed during this audit. The 50K req/sec claim is unverified. SQLite with MaxOpenConns(1) in some paths is a bottleneck. No evidence of load testing. |
| **Documentation Accuracy** | 4/10 | Claims 85%+ test coverage (actual: 81.2%). Claims 1.0.0 production release with 50K req/sec (unverified). TASKS.md marks everything as complete, masking real gaps. |

**Weighted Overall: 6.0/10** — *Above hobby-project grade, below enterprise-production grade.*

---

## Critical Blockers (Do Not Deploy Without Fixing)

1. **Gosec: 186 Unhandled Errors (G104 / CWE-703)**
   - `internal/admin/ws_hub.go`: `conn.Close()`, `sendPong()`, `SetReadDeadline()` errors discarded.
   - `internal/admin/server.go` and gateway paths ignore `WriteJSON`/`Write` errors.
   - **Risk:** Resource leaks, denial-of-service via connection exhaustion, silent failures in operator-facing paths.
   - **Action:** Enforce `errcheck` or `gosec` in CI with zero-tolerance policy for new unhandled errors.

2. **Admin API Authentication is a Single Static API Key**
   - `internal/admin/server.go` uses `subtle.ConstantTimeCompare` against `s.cfg.Admin.APIKey`.
   - No JWT, no RBAC inside admin API, no session rotation, no MFA.
   - **Risk:** Key leakage = total platform compromise.
   - **Action:** Implement scoped admin tokens with TTL, or at minimum IP allow-listing for admin.

3. **SQLite MaxOpenConns(1) in `store.Open`**
   - The store opens with `MaxOpenConns(1)` (confirmed in `internal/store/store.go`).
   - Under concurrent gateway load, this serializes all DB access (auth lookups, credit deductions, audit writes).
   - **Risk:** Gateway throughput collapses under concurrent load.
   - **Action:** Increase `MaxOpenConns` to a sane value (e.g., 25) and benchmark under `wrk`/`k6`.

4. **Frontend Tests: 35% Failure Rate**
   - 6 of 8 test files fail (33 of 94 tests).
   - `UserRoleManager.test.tsx` has assertion mismatches and checkbox state bugs.
   - **Risk:** UI regressions will slip into production undetected.
   - **Action:** Fix or delete brittle tests. Do not claim "production release" with a broken test suite.

5. **Inflated Claims Mask Real Risk**
   - Documentation claims v1.0.0 "production release" with 85%+ coverage and 50K req/sec.
   - These claims are **not substantiated** by the codebase and create a false sense of safety.
   - **Risk:** Stakeholders make deployment decisions based on marketing docs, not engineering reality.
   - **Action:** Update CHANGELOG and docs to reflect actual test coverage and benchmark results.

---

## High-Priority Issues (Fix Before Public Traffic)

6. **Audit Log May Capture Sensitive Data by Default**
   - Config allows `store_request_body: true` and `store_response_body: true` with 10KB limits.
   - Masking exists for headers, but body-field masking is string-replacement only.
   - **Risk:** PII/credentials may be written to SQLite or Kafka streams.
   - **Action:** Default to `store_request_body: false` in production configs; tighten masking rules.

7. **Rate Limiting Cleanup Goroutine Leak Risk**
   - `adminAuthAttempts` cleanup ticker in `internal/admin/server.go` starts in `NewServer` but only one stop channel exists.
   - Rapid admin server restarts in tests or hot-reload scenarios may leak goroutines.
   - **Action:** Add `Close()` method to `Server` and ensure tickers are stopped deterministically.

8. **GraphQL Federation Composer Uses Heuristic Entity Detection**
   - `internal/federation/composer.go` falls back to `"id"` field heuristic for entity detection if `@key` is missing.
   - This can produce incorrect supergraph schemas in production.
   - **Action:** Remove heuristic fallback; fail composition explicitly when `@key` is absent.

9. **WebSocket Connection Handling in Admin Dashboard**
   - The custom WebSocket hub (`ws_hub.go`) reimplements framing. While functional, it handles pong/ping deadlines with discarded errors.
   - **Risk:** Connection leaks and phantom clients.
   - **Action:** Consider migrating to `gorilla/websocket` or `nhooyr/websocket` for battle-tested framing.

10. **No Network Partition Tests for Raft**
    - Raft tests cover basic leader election and log replication, but there are no split-brain or network partition torture tests.
    - **Risk:** Cluster state may diverge under realistic network failure scenarios.
    - **Action:** Add Jepsen-style or `toxiproxy`-based partition tests before offering clustering as production-ready.

---

## Medium-Priority Issues (Fix Within First Sprint)

11. **Test Suite Bloat**
    - Many packages have `additional_test.go`, `coverage_test.go`, `100_test.go` — files that appear generated to push coverage numbers.
    - This creates maintenance drag and false confidence.
    - **Action:** Audit and remove synthetic coverage-padding tests; keep behavior-driven tests only.

12. **Frontend Bundle Size**
    - Build produces a single `index-Byv4lZ-X.js` of **1.87 MB** (550 KB gzipped).
    - Vite warns about chunk size.
    - **Action:** Implement route-based code splitting and dynamic imports before mobile/low-bandwidth usage.

13. **Missing Liveness/Readiness Probes in Deployment Configs**
    - `Dockerfile` has a `HEALTHCHECK`, but Kubernetes Helm charts (if present) may not expose `/admin/api/v1/status` as readiness probe.
    - **Action:** Ensure K8s manifests include explicit readiness/liveness probes.

14. **gRPC Transcoder Untested Edge Cases**
    - `internal/grpc/transcoder.go` is ~200 lines but the transcoder test is minimal.
    - Complex protobuf types (oneofs, nested messages, enums) likely have gaps.
    - **Action:** Expand transcoder test matrix before advertising REST-to-gRPC as production-ready.

---

## What is Actually Working Well

- **Real GraphQL Federation:** The composer, planner, and executor are non-trivial implementations. Schema composition and query planning exist.
- **Real Raft Clustering:** Leader election, log replication, snapshots, and transport layers are implemented with reasonable fidelity.
- **Real gRPC Proxy:** h2c server, unary/streaming proxy, health checks, and transcoding are present.
- **Real WASM Plugin System:** ~560 lines of WASM runtime integration with guest/host communication.
- **Real React Dashboard:** The admin UI has 20+ pages, real data fetching, charts, WebSocket tail, and a functional playground.
- **Credit/Billing System:** Atomic SQLite transactions, per-route pricing, method multipliers, test-key bypass, and transaction history are all functional.
- **CI/CD Pipeline:** GitHub Actions runs lint, race tests, coverage, security scans, docker builds, and Helm linting.

---

## Final Recommendation

**Do not market this as "v1.0.0 Production Release" today.**

The codebase is **too good to be a prototype, but too rough to be enterprise-grade.** Rename the current state to a **v0.9.x Release Candidate** or **v1.0.0-beta**, and commit to resolving the critical blockers above before claiming production readiness.

**Suggested Deployment Path:**
1. **Week 1–2:** Fix gosec errors, address SQLite connection limit, fix frontend tests.
2. **Week 3–4:** Add admin token-scoping or IP restrictions, run perf benchmarks, update docs.
3. **Month 2:** Run Raft partition tests, harden GraphQL federation edge cases, reduce bundle size.
4. **Month 3:** External security audit, chaos engineering on cluster mode, then declare v1.0.0.

If you need this running *this week* for an internal/low-traffic deployment, it will likely function. But for customer-facing, high-throughput, or security-sensitive workloads, **treat the blockers as non-negotiable.**
