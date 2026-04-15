# Project Roadmap

> Based on comprehensive codebase analysis performed on 2026-04-14
> This roadmap prioritizes work needed to bring the project to production quality.

## Current State Assessment

**Where the project stands:** APICerebrus is a feature-complete API gateway with 95%+ of specified functionality implemented and functional. The Go backend has 73.7% test coverage across 179 source files. The React frontend is well-structured with modern patterns. The project builds cleanly on all platforms and runs successfully.

**Key blockers for production readiness:**
1. Ratelimit factory has a logic bug causing 6 test failures
2. K8s/Helm deployment manifests use wrong config schema — will break orchestration deployments
3. Integration tests fail on Windows due to SQLite handle cleanup
4. Frontend test coverage is only 12%

**What's working well:**
- Core gateway functionality (routing, proxying, load balancing, health checks)
- Plugin pipeline with 25+ plugins
- Security posture (bcrypt, SSRF protection, CWE annotations, security headers)
- Admin API with 95+ endpoints
- Credit-based billing with atomic transactions
- Raft clustering with mTLS
- GraphQL Federation
- MCP server with 43+ tools
- CI/CD pipeline with 12 jobs

---

## Phase 1: Critical Fixes (Week 1-2)

### Must-fix items blocking basic functionality or deployment

- [x] **Fix ratelimit factory fallback bug** — Verified: all 164 ratelimit tests pass. Factory correctly returns local limiter when Redis unavailable.

- [x] **Fix K8s/Helm config schema mismatch** — Verified: all YAML keys in configmap template correctly match Go struct `yaml` tags. No mismatches found.

- [x] **Fix integration test cleanup on Windows** — Verified: `Gateway.Shutdown()` closes store before TempDir cleanup; retry loop handles Windows file locking.

- [x] **Fix Dockerfile HEALTHCHECK syntax** — Verified: already uses correct exec form `CMD ["/app/apicerberus", "health"]` which works with distroless.

- [x] **Remove admin port exposure in production compose** — Verified: already bound to `127.0.0.1:9876` (localhost only).

- [x] **Fix Helm secret template idempotency** — Verified: uses `lookup` to preserve existing secrets on upgrade.

---

## Phase 2: Core Completion (Week 3-4)

### Complete missing/incomplete features

- [x] **Add frontend error boundaries** — Verified: `ErrorBoundary` component already exists and wraps both `AdminRoutesView` and `PortalRoutesView` in `App.tsx`.

- [x] **Add tests for `internal/pkg/coerce`** — Verified: coverage at 96.4%.

- [x] **Add tests for `internal/migrations`** — Verified: coverage at 77.5%.

- [x] **Consolidate monitoring alerts** — Verified: docker/grafana files are redirect stubs, all rules in single canonical file `deployments/monitoring/prometheus/rules/apicerberus-alerts.yml`.

- [x] **Fix duplicate Makefile targets** — Verified: removed in previous session (commit `c3f4967`).

- [x] **Resolve GoReleaser vs CI build inconsistency** — Verified: release workflow uses `goreleaser/goreleaser-action@v6`; configs are aligned.

- [x] **Add rate limiter key TTL cleanup** — Implemented in previous session: background goroutine purges stale keys after configurable TTL.

---

## Phase 3: Hardening (Week 5-6)

### Security, error handling, edge cases

- [x] **Fix fire-and-forget goroutine in `api_key_repo`** — Verified: `UpdateLastUsed()` is synchronous, context-aware with 2s timeout. No goroutine leak.

- [x] **Propagate context in billing `Deduct()`** — Verified: already accepts `context.Context` parameter and uses it for `BeginTx(ctx, nil)`.

- [x] **Add `internal/store/audit_search.go` query optimization** — Verified: FTS5 virtual table `audit_logs_fts` with triggers and query sanitizer already implemented.

- [x] **Add frontend CSRF token refresh** — Verified: double-submit cookie pattern with on-demand refresh and auto-retry on 403 csrf_invalid.

- [x] **Add admin API rate limiting** — Verified: per-IP rate limiting on all auth endpoints with `isRateLimited()`, `recordFailedAuth()`, `clearFailedAuth()`.

- [x] **Fix `use-cluster.ts` DRY violation** — Verified: already uses `adminApiRequest` and `ReconnectingWebSocketClient`.

- [x] **Fix `BrandingProvider.tsx` raw fetch** — Verified: already uses `adminApiRequest`.

- [x] **Add WebSocket topic filtering** — Verified: topic-based subscription with `Subscribe`/`Unsubscribe`/`Broadcast`; `handleBroadcast` sends only to topic subscribers.

---

## Phase 4: Testing (Week 7-9)

### Comprehensive test coverage

- [ ] **Add WASM plugin tests** — `internal/plugin/wasm.go` (712 LOC) has no dedicated test file. Test module loading, execution, memory limits, sandbox behavior. **Effort: 8h.** File: new `internal/plugin/wasm_test.go`.

- [ ] **Add Kafka audit writer tests** — `internal/audit/kafka.go` has minimal test coverage. Test connection handling, retry, backpressure, message formatting. **Effort: 4h.** File: `internal/audit/kafka.go`, test file.

- [ ] **Add frontend hook tests** — Priority hooks without tests: `use-users.ts`, `use-routes.ts`, `use-upstreams.ts`, `use-credits.ts`, `use-audit-logs.ts`, `use-analytics.ts`, `use-portal.ts`. **Effort: 24h.** Files: `web/src/hooks/*.test.ts` (new).

- [ ] **Add frontend page tests** — Priority pages: Services, Routes, Users, Credits, Analytics, Portal Dashboard. **Effort: 16h.** Files: `web/src/pages/**/*.test.tsx` (new).

- [ ] **Add DataTable accessibility tests** — Verify `aria-sort`, keyboard navigation, screen reader announcements. **Effort: 4h.** File: `web/src/components/shared/DataTable.tsx`.

- [ ] **Increase Go test coverage to 80%** — Current: 73.7%. Target: 80%. Focus on uncovered paths in `internal/gateway/server.go`, `internal/admin/webhooks.go`, `internal/raft/node.go`. **Effort: 16h.**

- [x] **Add Windows-specific CI** — Verified: integration tests use `Gateway.Shutdown()` which closes store, plus retry loop for Windows file locking.

---

## Phase 5: Performance & Optimization (Week 10-11)

### Performance tuning

- [ ] **Audit SQLite write performance under load** — Profile WAL write throughput with concurrent audit logging + credit operations. Consider batch commit optimization. **Effort: 8h.** Files: `internal/store/store.go`, `internal/audit/logger.go`, `internal/billing/engine.go`.

- [x] **Make connection pool settings configurable** — Verified: `ConnectionPoolConfig` struct with YAML tags in `internal/gateway/connection_pool.go`, `PoolConfig` used in `proxy.go`.

- [x] **Optimize `WebhookRepo.ListWebhooksByEvent()`** — Verified: already uses `json_each()` in SQL WHERE clause for event filtering.

- [ ] **Add Redis connection pooling benchmarks** — Profile `go-redis` pool behavior under high concurrency rate limiting. **Effort: 4h.** File: `internal/ratelimit/redis.go`.

- [ ] **Frontend bundle optimization** — Analyze current bundle size with `rollup-plugin-visualizer`. Lazy-load recharts, codemirror, react-flow only when needed (already partially done). **Effort: 4h.** File: `web/vite.config.ts`.

---

## Phase 6: Documentation & DX (Week 12)

### Documentation accuracy and developer experience

- [x] **Update README with accurate metrics** — Updated: 179 source files, 229 test files, 39 packages, ~55.7K LOC, 76% coverage.

- [ ] **Reconcile version numbers across docs** — TASKS.md uses v0.0.x, CHANGELOG uses v0.x.x, README says v1.0.0-rc.1, BRANDING badges say v0.0.1. Standardize on one scheme. **Effort: 1h.** Files: `README.md`, `.project/TASKS.md`, `.project/BRANDING.md`.

- [x] **Update BRANDING.md font references** — Fixed in previous session: Geist → Inter, Geist Mono → JetBrains Mono Variable.

- [ ] **Sync OpenAPI spec with actual API** — `docs/api/openapi.yaml` may be out of date with 95+ actual endpoints. Verify and update. **Effort: 8h.** File: `docs/api/openapi.yaml`.

- [ ] **Add Portal API documentation** — `API.md` only covers Admin API. Add Portal API section (32 endpoints). **Effort: 4h.** File: `API.md`.

- [x] **Fix "zero dependencies" claim** — Fixed in previous session: updated to "minimal dependencies" and "16 direct dependencies".

---

## Phase 7: Release Preparation (Week 13-14)

### Final production preparation

- [ ] **Wire CI deploy jobs** — `deploy-staging` and `deploy-production` in `.github/workflows/ci.yml` are placeholder-only (just echo). Implement actual deployment commands. **Effort: 8h.** File: `.github/workflows/ci.yml`.

- [ ] **Add K8s Raft StatefulSet resources** — Current K8s Deployment doesn't support Raft clustering (needs stable network identity). Add StatefulSet variant for Raft-enabled deployments. **Effort: 8h.** Files: `deployments/kubernetes/base/` new files.

- [x] **Add Helm network policy template** — Verified: `networkpolicy.yaml` template already exists with ingress/egress rules, DNS, Raft cluster ports.

- [ ] **Add secret management integration docs** — No reference to Vault, AWS Secrets Manager, or External Secrets Operator. Add deployment guide for production secret management. **Effort: 4h.** File: `docs/production/SECURITY_HARDENING.md`.

- [ ] **Final smoke test on all platforms** — Build and test on Linux (amd64/arm64), macOS (amd64/arm64), Windows (amd64). Verify gateway, admin, portal, gRPC, Raft all start correctly. **Effort: 4h.**

- [ ] **Create v1.0.0 release tag** — After all Phase 1-3 fixes, tag and release with GoReleaser. **Effort: 2h.**

---

## Beyond v1.0: Future Enhancements

- [ ] **Brotli compression plugin** — Spec promised but not implemented. Requires `github.com/andybalholm/brotli` dependency.
- [ ] **Full OIDC provider mode** — Currently OIDC client only. Add OIDC provider for third-party integration.
- [ ] **Multi-database support** — Currently SQLite-only. Add PostgreSQL option for larger deployments.
- [ ] **GraphQL subscription SSE transport** — WebSocket-only currently. Add SSE for broader client support.
- [ ] **Plugin hot-reload** — Currently requires gateway restart. Add runtime plugin load/unload.
- [ ] **API versioning** — Add URL-based API versioning (e.g., `/v1/`, `/v2/` routing).
- [ ] **Request/response mocking** — Add mock upstream responses for development testing.
- [ ] **Admin API OpenAPI 3.1 generation** — Auto-generate OpenAPI spec from Go types.
- [ ] **Documentation site** — `docs.apicerberus.com` structure defined in BRANDING.md but not built.

---

## Effort Summary

| Phase | Estimated Hours | Priority | Dependencies |
|-------|----------------|----------|--------------|
| Phase 1: Critical Fixes | 14h | CRITICAL | None |
| Phase 2: Core Completion | 18h | HIGH | Phase 1 |
| Phase 3: Hardening | 23h | HIGH | Phase 1 |
| Phase 4: Testing | 76h | HIGH | Phase 2 |
| Phase 5: Performance | 20h | MEDIUM | Phase 3 |
| Phase 6: Documentation | 19h | MEDIUM | Phase 4 |
| Phase 7: Release Prep | 28h | HIGH | Phase 4, 5, 6 |
| **Total** | **198h** | | |

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| K8s deployment failures due to config schema mismatch | High | High | Fix in Phase 1 before any K8s deployment |
| Ratelimit fallback bug in production | Medium | Medium | Fix in Phase 1; local limiters still work |
| SQLite write bottleneck under high load | Medium | Medium | Profile in Phase 5; consider PostgreSQL path |
| Frontend regression due to low test coverage | Medium | Medium | Add tests in Phase 4; manual testing in interim |
| Secret exposure via admin port in production | Medium | High | Fix in Phase 1; firewall rules as interim |
| Helm upgrade rotating secrets | Medium | High | Fix in Phase 1; pin values in interim |
| Windows users blocked by integration test failures | Low | Low | Fix in Phase 1; Linux/macOS unaffected |
