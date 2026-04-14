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

- [ ] **Fix ratelimit factory fallback bug** — `internal/ratelimit/redis.go` factory returns `*DistributedTokenBucket` wrapper instead of unwrapping to local `*TokenBucket` when Redis unavailable. Affects 6 tests in `internal/ratelimit`. **Effort: 2h.** Files: `internal/ratelimit/redis.go`, `internal/ratelimit/redis_coverage_test.go`, `internal/ratelimit/ratelimit_extended_test.go`.

- [ ] **Fix K8s/Helm config schema mismatch** — `deployments/kubernetes/base/configmap.yaml` and `deployments/helm/apicerberus/values.yaml` use `server.address`, `auth.jwt.secret` while the application expects `gateway.http_addr`, `admin.token_secret`. All K8s and Helm deployments will produce invalid config. **Effort: 4-8h.** Files: `deployments/kubernetes/base/configmap.yaml`, `deployments/helm/apicerberus/values.yaml`, `deployments/helm/apicerberus/templates/configmap.yaml`, `deployments/examples/*.yaml`.

- [ ] **Fix integration test cleanup on Windows** — `test/integration/*_test.go` fails TempDir cleanup because SQLite file handles aren't released before removal. Add explicit `db.Close()` + short wait in test cleanup. **Effort: 2-4h.** Files: `test/integration/auth_flow_test.go`, `test/integration/request_lifecycle_test.go`, `test/integration/plugin_chain_test.go`.

- [ ] **Fix Dockerfile HEALTHCHECK syntax** — Line 102 uses exec form with shell operator `|| exit 1`, which won't work. Use shell form: `HEALTHCHECK CMD /app/apicerberus health || exit 1`. **Effort: 15min.** File: `Dockerfile`.

- [ ] **Remove admin port exposure in production compose** — `docker-compose.prod.yml` publishes port 9876 with `mode: host`, exposing admin API on all interfaces. Change to `127.0.0.1:9876:9876` or remove. **Effort: 15min.** File: `docker-compose.prod.yml`.

- [ ] **Fix Helm secret template idempotency** — `deployments/helm/apicerberus/templates/secret.yaml` uses `randAlphaNum 32` which generates new secrets on every `helm upgrade`. Use `lookup` to preserve existing secrets. **Effort: 1h.** File: `deployments/helm/apicerberus/templates/secret.yaml`.

---

## Phase 2: Core Completion (Week 3-4)

### Complete missing/incomplete features

- [ ] **Add frontend error boundaries** — No React error boundaries wrapping route subtrees. A single component crash takes down the page. Add `ErrorBoundary` component wrapping each route group in `App.tsx`. **Effort: 2h.** Files: `web/src/App.tsx`, new `web/src/components/ErrorBoundary.tsx`.

- [ ] **Add tests for `internal/pkg/coerce`** — Type coercion package has zero test coverage but is used by admin API for input processing. Add table-driven tests for all coercion functions. **Effort: 2h.** File: `internal/pkg/coerce/coerce.go`, new `internal/pkg/coerce/coerce_test.go`.

- [ ] **Add tests for `internal/migrations`** — Migration runner has zero test coverage. Test up/down migrations, version tracking, error handling. **Effort: 2h.** File: `internal/migrations/migrations.go`, new `internal/migrations/migrations_test.go`.

- [ ] **Consolidate monitoring alerts** — Three separate files define overlapping Prometheus alert rules with different thresholds: `deployments/docker/prometheus-alerts.yml`, `deployments/grafana/alerts.yml`, `deployments/monitoring/prometheus/rules/apicerberus-alerts.yml`. Consolidate into one canonical file. **Effort: 3h.**

- [ ] **Fix duplicate Makefile targets** — `docker-compose-up`, `docker-compose-down`, `docker-compose-logs`, `docker-compose-prod-up`, `docker-compose-prod-down` are each defined twice. Remove duplicates. **Effort: 15min.** File: `Makefile`.

- [ ] **Resolve GoReleaser vs CI build inconsistency** — `.goreleaser.yml` is maintained but CI builds binaries manually via shell scripts. Either integrate GoReleaser into CI (`goreleaser/goreleaser-action`) or remove `.goreleaser.yml`. **Effort: 2-4h.** Files: `.goreleaser.yml`, `.github/workflows/release.yml`.

- [ ] **Add rate limiter key TTL cleanup** — `internal/ratelimit/token_bucket.go`, `fixed_window.go`, `sliding_window.go` use `sync.Map` that grows unbounded as new keys are added. Add background goroutine to purge stale keys after configurable TTL. **Effort: 4h.**

---

## Phase 3: Hardening (Week 5-6)

### Security, error handling, edge cases

- [ ] **Fix fire-and-forget goroutine in `api_key_repo`** — `UpdateLastLast()` spawns unmanaged goroutine that could leak during shutdown. Replace with managed worker pool or context-aware goroutine. **Effort: 2h.** File: `internal/store/api_key_repo.go`.

- [ ] **Propagate context in billing `Deduct()`** — Uses `context.Background()` instead of request context, blocking indefinitely if SQLite is stuck. Accept context parameter. **Effort: 1h.** File: `internal/billing/engine.go`.

- [ ] **Add `internal/store/audit_search.go` query optimization** — Uses `LIKE` patterns on `request_body`/`response_body` columns. Add FTS5 virtual table or GIN-like indexing for audit search. **Effort: 8h.**

- [ ] **Add frontend CSRF token refresh** — Portal API client fetches CSRF token once. Add periodic refresh and handle token expiry gracefully. **Effort: 2h.** File: `web/src/lib/portal-api.ts`.

- [ ] **Add admin API rate limiting** — No rate limiting on admin endpoints by default. Add configurable rate limiting on `/admin/api/v1/auth/token` to prevent brute force. **Effort: 2h.** File: `internal/admin/server.go`.

- [ ] **Fix `use-cluster.ts` DRY violation** — Creates its own WebSocket and raw `fetch` instead of using `ReconnectingWebSocketClient` and `adminApiRequest`. Refactor to use shared utilities. **Effort: 1h.** File: `web/src/hooks/use-cluster.ts`.

- [ ] **Fix `BrandingProvider.tsx` raw fetch** — Should use `adminApiRequest` for consistency. **Effort: 30min.** File: `web/src/components/layout/BrandingProvider.tsx`.

- [ ] **Add WebSocket topic filtering** — `ws_hub.go` broadcasts all events to all connections. Add topic-based subscription (e.g., `analytics:*`, `config:*`). **Effort: 4h.** File: `internal/admin/ws_hub.go`.

---

## Phase 4: Testing (Week 7-9)

### Comprehensive test coverage

- [ ] **Add WASM plugin tests** — `internal/plugin/wasm.go` (712 LOC) has no dedicated test file. Test module loading, execution, memory limits, sandbox behavior. **Effort: 8h.** File: new `internal/plugin/wasm_test.go`.

- [ ] **Add Kafka audit writer tests** — `internal/audit/kafka.go` has minimal test coverage. Test connection handling, retry, backpressure, message formatting. **Effort: 4h.** File: `internal/audit/kafka.go`, test file.

- [ ] **Add frontend hook tests** — Priority hooks without tests: `use-users.ts`, `use-routes.ts`, `use-upstreams.ts`, `use-credits.ts`, `use-audit-logs.ts`, `use-analytics.ts`, `use-portal.ts`. **Effort: 24h.** Files: `web/src/hooks/*.test.ts` (new).

- [ ] **Add frontend page tests** — Priority pages: Services, Routes, Users, Credits, Analytics, Portal Dashboard. **Effort: 16h.** Files: `web/src/pages/**/*.test.tsx` (new).

- [ ] **Add DataTable accessibility tests** — Verify `aria-sort`, keyboard navigation, screen reader announcements. **Effort: 4h.** File: `web/src/components/shared/DataTable.tsx`.

- [ ] **Increase Go test coverage to 80%** — Current: 73.7%. Target: 80%. Focus on uncovered paths in `internal/gateway/server.go`, `internal/admin/webhooks.go`, `internal/raft/node.go`. **Effort: 16h.**

- [ ] **Add Windows-specific CI** — Integration tests fail on Windows. Add Windows runner to CI matrix with appropriate SQLite cleanup. **Effort: 4h.** File: `.github/workflows/ci.yml`.

---

## Phase 5: Performance & Optimization (Week 10-11)

### Performance tuning

- [ ] **Audit SQLite write performance under load** — Profile WAL write throughput with concurrent audit logging + credit operations. Consider batch commit optimization. **Effort: 8h.** Files: `internal/store/store.go`, `internal/audit/logger.go`, `internal/billing/engine.go`.

- [ ] **Make connection pool settings configurable** — Currently hardcoded: `maxIdleConns=100`, `idleConnTimeout=90s`. Add to YAML config. **Effort: 2h.** Files: `internal/gateway/proxy.go`, `internal/config/types.go`, `apicerberus.example.yaml`.

- [ ] **Optimize `WebhookRepo.ListWebhooksByEvent()`** — Fetches ALL active webhooks then filters in Go. Add SQL WHERE clause for event filtering. **Effort: 2h.** File: `internal/store/webhook_repo.go`.

- [ ] **Add Redis connection pooling benchmarks** — Profile `go-redis` pool behavior under high concurrency rate limiting. **Effort: 4h.** File: `internal/ratelimit/redis.go`.

- [ ] **Frontend bundle optimization** — Analyze current bundle size with `rollup-plugin-visualizer`. Lazy-load recharts, codemirror, react-flow only when needed (already partially done). **Effort: 4h.** File: `web/vite.config.ts`.

---

## Phase 6: Documentation & DX (Week 12)

### Documentation accuracy and developer experience

- [ ] **Update README with accurate metrics** — Current: "150,000+ LOC" (actual: 55K non-test), "85% coverage" (actual: 73.7%), "Go 1.26+" (verify correct Go version). **Effort: 1h.** File: `README.md`.

- [ ] **Reconcile version numbers across docs** — TASKS.md uses v0.0.x, CHANGELOG uses v0.x.x, README says v1.0.0-rc.1, BRANDING badges say v0.0.1. Standardize on one scheme. **Effort: 1h.** Files: `README.md`, `.project/TASKS.md`, `.project/BRANDING.md`.

- [ ] **Update BRANDING.md font references** — Says "Geist Sans/Mono" but frontend uses "Inter/JetBrains Mono". **Effort: 15min.** File: `.project/BRANDING.md`.

- [ ] **Sync OpenAPI spec with actual API** — `docs/api/openapi.yaml` may be out of date with 95+ actual endpoints. Verify and update. **Effort: 8h.** File: `docs/api/openapi.yaml`.

- [ ] **Add Portal API documentation** — `API.md` only covers Admin API. Add Portal API section (32 endpoints). **Effort: 4h.** File: `API.md`.

- [ ] **Fix "zero dependencies" claim** — README and BRANDING claim zero dependencies but go.mod has 16 direct deps. Update to "minimal dependencies" or "16 dependencies." **Effort: 30min.** Files: `README.md`, `.project/BRANDING.md`.

---

## Phase 7: Release Preparation (Week 13-14)

### Final production preparation

- [ ] **Wire CI deploy jobs** — `deploy-staging` and `deploy-production` in `.github/workflows/ci.yml` are placeholder-only (just echo). Implement actual deployment commands. **Effort: 8h.** File: `.github/workflows/ci.yml`.

- [ ] **Add K8s Raft StatefulSet resources** — Current K8s Deployment doesn't support Raft clustering (needs stable network identity). Add StatefulSet variant for Raft-enabled deployments. **Effort: 8h.** Files: `deployments/kubernetes/base/` new files.

- [ ] **Add Helm network policy template** — `values.yaml` has `networkPolicy.enabled: false` but no template implements it. Create `networkpolicy.yaml` template. **Effort: 2h.** File: `deployments/helm/apicerberus/templates/networkpolicy.yaml`.

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
