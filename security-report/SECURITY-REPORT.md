# APICerebrus Security Report

**Report Date:** 2026-04-16 (Phase 2 Deep Scan — supersedes 12:20 snapshot)
**Project:** APICerebrus API Gateway
**Go Version:** 1.26.2
**Classification:** INTERNAL — Contains unpatched findings
**Scan Type:** Full — 5 specialized parallel agents + diff-scan baseline

---

## 0. How to Read This Report

Two distinct scan waves were performed on 2026-04-16:

| Wave | Time | Scope | Status |
|------|------|-------|--------|
| **Wave 1** (surface scan) | 12:20 | OWASP Top-10 surface review, dependency audit | Superseded. Its fixes remain valid. See §5. |
| **Wave 2** (deep scan) | 17:00 | Targeted deep-dive into 5 under-audited subsystems: WASM pipeline, Raft/mTLS, GraphQL Federation + MCP, gRPC/WebSocket/proxy, supply chain + IaC | **Current — findings in §3–§4.** |

The Wave 1 surface scan correctly identified and fixed 8 items (F-001 through F-011 below). However, it rated WASM, Raft, and Federation as "low risk" without deep inspection of their **integration seams** — and that is where Wave 2 found the serious bugs.

**Net current state:** 3 Critical, 20 High, 21 Medium, 15 Low, 2 Info open — a dramatic delta from the Wave-1 "0 open Critical/High" verdict. Wave 2 findings are all actionable with file:line.

---

## 1. Executive Summary

### Current Risk Profile: **HIGH**

The gateway has strong primitives (bcrypt, constant-time compare, WASM wazero sandbox, SSRF blocks on main proxy path, OIDC state/nonce validation, distroless container image) but several **integration bugs**, **dead-code security features**, and **configuration defaults** combine into production-blocking issues:

1. **Clustering is exploitable by any network-adjacent attacker.** mTLS config is documented but never wired (`cluster.go`+`run.go` path). Raft RPCs accept unauthenticated `AppendEntries` → arbitrary FSM-command injection (routes, credits, certs). Private keys replicate cleartext through the log.
2. **Federation `/graphql/batch` bypasses the entire plugin pipeline** (auth + rate-limit + billing), trivially amplified by alias/fragment complexity bombs.
3. **WASM integration can bypass authentication** by declaring `phase: auth` — unvalidated — to flip `routeHasAuth` while doing nothing. Combined with `Pipeline.Execute` running all phases in one loop, phase isolation is cosmetic.
4. **Health checker is a reflective SSRF oracle** to cloud metadata (`169.254.169.254`) bypassing the main proxy's SSRF gate.
5. **OptimizedProxy request coalescing** leaks cookie-authenticated responses across users inside a 10ms window.
6. **CI/CD** passes `JWT_SECRET` / `ADMIN_API_KEY` via `helm --set` (process-table leak) and pins every third-party Action by mutable tag (CVE-2025-30066 class).

**Recommendation:** Do not roll out clustering, federation, or WASM plugins in production until at least the 3 Critical + top-5 High findings are fixed. Single-node, non-federated, non-WASM deployments are in better shape but still have the health-SSRF oracle and CI/CD secret-leakage issues to address.

### Findings by Severity

| Severity | Count | Subsystems |
|----------|-------|------------|
| **Critical** | 3 | Raft (2), GraphQL Federation (1) |
| **High** | 20 | WASM (4), Raft (5), Federation/MCP (6), Proxy (3), Supply chain (2) |
| **Medium** | 21 | WASM (6), Raft (1), Federation/MCP (5), Proxy (5), Supply chain (4) |
| **Low** | 15 | WASM (2), Raft (4), Federation/MCP (2), Proxy (1), Supply chain (6) |
| **Info** | 2 | Raft (1), Proxy (1) |

Per-subsystem detail files (kept verbatim for engineering reference):
- `sc-wasm-plugin-results.md` — 12 findings
- `sc-raft-cluster-results.md` — 12 findings
- `sc-federation-mcp-results.md` — 14 findings
- `sc-grpc-ws-proxy-results.md` — 10 findings
- `sc-supply-chain-results.md` — 12 findings

---

## 2. Critical Findings (3)

### CRIT-1 · RAFT-001 + RAFT-003: Raft mTLS is dead code — cluster accepts unauthenticated FSM commands

| Field | Value |
|-------|-------|
| **CWE** | CWE-311, CWE-306, CWE-345, CWE-300 |
| **CVSS 3.1** | **9.8** (`AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H`) |
| **File** | `internal/cli/run.go:104-167`; `internal/raft/transport.go:197-209, 55-58`; `internal/raft/node.go:863-949` |
| **Confidence** | High |

Documented `cluster.mtls.*` config is never read. `NewTLSCertificateManager`, `GenerateCA`, `SetTLSConfig` have **no production callers**. Because `SetRPCSecret` refuses to accept a secret without TLS (`transport.go:55-58`), operators who try to enable auth get a startup error and fall back to leaving `rpc_secret` empty → `withRPCAuth` is a no-op → any attacker who can reach port 12000 can `POST /raft/append-entries` with `Term=999999` and inject arbitrary `FSMCommand` entries (credit top-ups via `CmdUpdateCredits`, route mutations, cert swaps).

**Fix:** (§7.1) Wire `cfg.Cluster.MTLS` into `run.go` end-to-end; refuse to start when `cluster.enabled && !mtls.enabled`; validate `cert.IsCA`/`BasicConstraintsValid` in `ImportCACert`; require an out-of-band CA fingerprint pin for auto-generate mode (RAFT-002).

---

### CRIT-2 · GQL-001: `/graphql/batch` endpoint bypasses the entire plugin pipeline

| Field | Value |
|-------|-------|
| **CWE** | CWE-862, CWE-770 |
| **CVSS 3.1** | **9.1** (`AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:N/A:H`) |
| **File** | `internal/gateway/server.go:243-247`; `internal/gateway/server.go:1075-1123` |
| **Confidence** | High |

```go
// server.go:243-247
if g.federationEnabled && r.URL.Path == "/graphql/batch" {
    g.serveFederationBatch(w, r)   // runs BEFORE newRequestState(), auth, rate-limit, billing
    return
}
```

Unauthenticated actors submit 100 maximally-complex federated queries per request. Each element fans out to every configured subgraph with no credit deduction and no rate limit. Compounded by `QueryAnalyzer` never being invoked on federation paths (GQL-002) and `FragmentSpread.Depth()` returning `1` regardless of target (GQL-003), a trivial alias/fragment bomb produces 10,000+ upstream subgraph calls per one unauthenticated HTTP request.

**Fix:** Move the batch-endpoint check below auth/pipeline execution, or wrap `serveFederationBatch` in `executeAuthChain` + `QueryAnalyzer` invocation; gate batch on a dedicated federation ACL.

---

## 3. High Findings (20 — grouped)

Full detail with exploit scenarios and diff-ready remediations lives in the per-subsystem files. Summary grouping below for triage:

### 3.1 WASM & Plugin Pipeline (4 High)

| ID | Title | File |
|----|-------|------|
| WASM-001 | Unvalidated `phase` from WASM config lets a module flip `routeHasAuth=true` → global API-key fallback skipped | `wasm.go:226-229` + `registry.go:216` + `serve_auth.go:14` |
| WASM-002 | `Pipeline.Execute` runs every plugin regardless of declared phase — phase ordering is cosmetic; `post-proxy` WASM fires pre-proxy | `pipeline.go:20-35` |
| WASM-003 | Zero `recover()` calls in `internal/plugin/` — a WASM or plugin panic corrupts shared pipeline state | `pipeline.go:20-35`, `wasm.go:275-352` |
| WASM-004 | `WASMContext.ApplyToContext` merges arbitrary headers back into the live request, overwriting JWT-claim-derived headers (privilege escalation) | `wasm.go:688-708` |

Combined, these give anyone with plugin-config write access a full **AuthN-bypass primitive**.

### 3.2 Raft Clustering (5 High, 2 Crit already listed)

| ID | Title | File |
|----|-------|------|
| RAFT-002 | Auto-generate mTLS has no TOFU protection; `ImportCACert` doesn't verify `cert.IsCA` or pin a fingerprint | `tls.go:25-149` |
| RAFT-004 | Cluster join accepts any `NodeID`/`Address`; no reachability/cert check; SetPeer never called so join is silently broken | `cluster.go:181-232` |
| RAFT-005 | `GatewayFSM.Apply` does `entry.Command.([]byte)` — HTTP transport JSON-round-trips to string → guaranteed follower panic (masked by test-only InmemTransport) | `fsm.go:156-164` |
| RAFT-007 | Private TLS keys (`KeyPEM`) replicated as plaintext JSON in `raft_log` BLOB + in-flight AppendEntries | `certificate_sync.go:16-23`, `fsm.go:367-382` |
| RAFT-008 | `CmdUpdateCredits` FSM path accepts unbounded `int64` deltas; any replicated entry can zero or overflow balances cluster-wide | `fsm.go:329-344` |

### 3.3 GraphQL Federation + MCP (6 High, 1 Crit already listed)

| ID | Title | File |
|----|-------|------|
| GQL-002 | Federation handler never calls `QueryAnalyzer` — no depth/complexity/field-count limit | `planner.go:42-69`, `server.go:1075-1123` |
| GQL-003 | Complexity analyzer: `FragmentSpread` cost hard-coded to `defaultCost=1`, depth `1`; aliased siblings counted once — fragment/alias bomb trivial | `analyzer.go:146-148`, `parser.go:115-124` |
| GQL-005 | 4 subgraph network paths skip `validateSubgraphURL` (FetchSchema, CheckHealth, ExecuteBatch, runSubscription); validator allows on DNS fail; IPv6 gaps | `subgraph.go:237, 336, 440-443, 454-473`, `executor.go:616, 715` |
| GQL-006 | `@authorized` directive parsed but `ExecutionAuthChecker.CheckFieldAuth` never invoked outside tests — field-level authz cosmetic | `executor.go:248-295`, `composer.go:279-307` |
| GQL-007 | Subscription proxies (WS + SSE) have no Origin check, no auth propagation, no rate limit → cross-site WebSocket hijacking | `subscription.go:65-111`, `subscription_sse.go:48-157` |
| GQL-010 | MCP tool `system.config.import` calls `config.Load(path)` with no canonicalization → arbitrary-file-read + YAML-parse error oracle + config hot-swap | `config_import.go:14-17`, `call_tool.go:312-325`, `config/load.go:17` |

### 3.4 gRPC / WebSocket / Proxy (3 High)

| ID | Title | File |
|----|-------|------|
| PROXY-001 | gRPC-Web handler emits `Access-Control-Allow-Origin: *` on authenticated responses | `grpc/proxy.go:217-219` |
| PROXY-002 | Health checker skips `validateUpstreamHost` — reflective SSRF oracle reaching cloud metadata, the main proxy path blocks this | `gateway/health.go:127-141` |
| PROXY-006 | `OptimizedProxy.coalesceKey` partitions only by `Authorization`/`X-API-Key` → cookie-authenticated GETs leak between users inside the 10ms coalesce window | `optimized_proxy.go:539-589` |

### 3.5 Supply Chain / IaC (2 High)

| ID | Title | File |
|----|-------|------|
| SUPPLY-001 | All third-party GitHub Actions pinned by mutable `@vX` tag (CVE-2025-30066 class — tj-actions/changed-files) | `.github/workflows/ci.yml` + `release.yml` |
| SUPPLY-002 | Helm production/staging deploys pass `JWT_SECRET` + `ADMIN_API_KEY` via `helm --set` — `/proc/<pid>/cmdline` leak + Helm release state preserves every rotated value | `ci.yml:491-492, 568-569` |

---

## 4. Medium / Low / Info (38 items)

Full enumerations with file:line and remediation are in the five subsystem files. Representative entries:

### Medium (21 total, selection)

- **WASM-005/006/007/008/011/012** — timeout doesn't cover host marshal/alloc paths (DoS); hot-reload TOCTOU with in-flight requests; shared `Metadata` map leaks `billing.*`/`skip_*` to guest; WASM bypasses native factory registry entirely; marketplace files not re-verified post-install; `Consumer` pointer mutable by any plugin.
- **RAFT-006** — `ClusterManager` binds its own HTTP server on `cfg.Admin.Addr` (port collision with admin API; error swallowed) — cluster admin endpoints are therefore silently unreachable in production today.
- **GQL-004/008/009/011/012** — planner silently drops fragments (and echoes `"no subgraph can resolve field: X"` as a schema oracle); colliding step IDs on aliased dup fields; MCP `/sse` unauthenticated; MCP `payloadFromArgs` accepts flat keys (`users.create` with top-level `role:admin` → privilege escalation if admin API doesn't strip unknown fields).
- **PROXY-003/004/005/009/010** — gRPC metadata forwards `Cookie`/`Authorization` unfiltered upstream; upstream gRPC metadata written unfiltered into HTTP response headers (security-header clobber); `OptimizedProxy.copyHeadersOptimized` ignores `Connection:` tokens (smuggling divergence from `proxy.go`); dead `ForwardWebSocket` with no frame-size/read-deadline; gRPC port matches REST routes with incompatible auth plugins.
- **SUPPLY-003/004/005/006** — `ci.yml` missing top-level `permissions:` → jobs inherit write scope; `:latest` pinned for Prometheus/Grafana/Alertmanager/node-exporter/cAdvisor + own app image; Helm `networkPolicy.enabled: false` default (kustomize base is stricter); Helm SA missing `automountServiceAccountToken: false`.

### Low (15 total, selection)

- WASM-009/010 — `AllowedPaths`/`EnvVars` fields present in config but not wired (latent foot-gun); tar extraction orphans partial files on size-cap breach.
- RAFT-009/010/011/012 — case-sensitive `Bearer` compare on cluster-auth; hardcoded cert serial `=2` on every node; no cert renewal path; `InstallSnapshot` non-chunked.
- GQL-013/014 — subscription context capped at 30 s (breaks long-lived subscriptions); static subgraph `Headers` leak same auth across users.
- PROXY-007 — `OptimizedProxy` propagates spoofed inbound `X-Forwarded-For` and never sets `X-Forwarded-Proto`.
- SUPPLY-007/008/009/010/011/012 — PDB `minAvailable: 2` with default `replicaCount: 1`; orphan Alpine Dockerfile in `deployments/docker/` (with shell); SARIF / benchmark artifact retention; deprecated transitive `glob@10.5.0` + 5 install-script packages; staging `--set` secret duplicate; unsigned Helm chart.

### Info

- RAFT-013 — `subtle.ConstantTimeCompare` early-exit on length in RPC auth (low-impact timing leak).
- PROXY-008 — Dead `ForwardWebSocket` code with no origin check — will regress to CSWSH the moment it's wired in.

---

## 5. Wave 1 Status (Carried Forward — all fixed or accepted)

The 12 items enumerated in the 12:20 report remain in `verified-findings.md`. Status unchanged by Wave 2:

| ID | Title | Status |
|----|-------|--------|
| F-001 | Health endpoints exposure | ✅ Fixed — `allowed_health_ips` config |
| F-002 | Admin API brute-force (WS endpoint) | ✅ Fixed — 2026-04-16 |
| F-003 | Test config hardcoded secrets | ✅ Fixed — `generateRandomSecret()` |
| F-004 | Test config predictable secrets | ✅ Fixed — env-var placeholders |
| F-005 | JWT JTI fail-closed | ✅ Fixed (HIGH-NEW-1) |
| F-006 | Portal sessionStorage XSS exposure | 📝 Accepted (documented M-022) |
| F-007 | OIDC state param CSRF | ✅ Verified correct |
| F-008 | Kafka TLS skip-verify admin-configurable | 📝 Accepted (prod validation rejects) |
| F-009 | GraphQL introspection production | ✅ Fixed — `admin.graphql_introspection` config |
| F-010 | Scope check on admin DELETE routes | ✅ Verified correct |
| F-011 | Portal secret length validation | ✅ Fixed |
| F-012 | Rate-limit on Admin WS upgrade | ✅ Fixed (MED-NEW-3) |

All remain valid. No regressions introduced by the 14 commits between 12:20 and this scan (that period was covered in `DIFF-SECURITY-REPORT.md`; see §6).

---

## 6. Diff-Scan Findings (Since Wave 1)

From `DIFF-SECURITY-REPORT.md` — 5 findings introduced by the 14 commits between 12:20 and 17:00:

| ID | Title | Severity |
|----|-------|----------|
| DIFF-001 | Backup CronJob container missing `securityContext` | Medium |
| DIFF-002 | Backup archives unencrypted, contain password hashes + session tokens | Low |
| DIFF-003 | Helm values spliced unquoted into `sh -c` block | Low |
| DIFF-004 | PG migration v8 uses unsupported `CREATE TRIGGER IF NOT EXISTS` | Info |
| DIFF-005 | `Rollback(version)` allows out-of-order invocation; data-integrity gap | Info |

These are counted in the Medium/Low/Info tallies above.

---

## 7. Remediation Roadmap

### Phase 1 — **STOP-THE-BLEED** (target: within 48 hours if clustering, federation, or WASM is enabled anywhere)

1. **Disable Raft clustering and GraphQL batch endpoint in production until fixed:**
   - Set `cluster.enabled: false` everywhere.
   - Set `federation.enabled: false` if possible, or at minimum refuse traffic to `/graphql/batch` at the ingress layer.
   - Remove WASM plugin config if any WASM modules are loaded in production.
2. **Fix CRIT-1 (RAFT-001/003):** Wire `cfg.Cluster.MTLS` through `run.go`. Require `cluster.mtls.enabled=true` when `cluster.enabled=true`; refuse startup otherwise. (See `sc-raft-cluster-results.md` §RAFT-001.)
3. **Fix CRIT-2 (GQL-001):** Move `/graphql/batch` dispatch below `newRequestState() + rs.pipeline.Execute() + executeAuthChain() + billing`. Invoke `QueryAnalyzer` (GQL-002) at the top of both `serveFederation` and `serveFederationBatch`.
4. **Rotate `PRODUCTION_JWT_SECRET` and `PRODUCTION_ADMIN_API_KEY`** — treat as potentially exposed via SUPPLY-002 `/proc/cmdline` leak. Replace `--set secrets.*` with SealedSecrets/ExternalSecrets pattern.

### Phase 2 — **HIGH-RISK** (target: 2 weeks)

5. **WASM integration hardening (WASM-001/002/004):** Validate `phase` against allow-list; forbid WASM in `PhaseAuth` without a signed-allow-list; split `Pipeline.Execute` per phase; add header deny-list for `ApplyToContext`.
6. **PROXY-002:** Call `validateUpstreamHost` in `runHealthCheck`; add a stricter `validateHealthTarget` that always blocks cloud metadata.
7. **PROXY-006:** Include `Cookie` + `X-Admin-Key` + every configured auth-header name in `coalesceKey`; default coalescing to opt-in per route.
8. **RAFT-005:** Accept both `[]byte` and `string` in `GatewayFSM.Apply`; switch `LogEntry.Command` to `json.RawMessage` long-term; add `recover()` in `Apply`/`applyCommitted` to prevent held-mutex deadlock on panic.
9. **GQL-005:** Add `validateSubgraphURL` to `FetchSchema`, `CheckHealth`, `ExecuteBatch`, `runSubscription`. Deny on DNS failure. Pin IP with `net.Dialer.Control`. Reject IPv4-mapped IPv6 after unwrap.
10. **GQL-006:** Wire `ExecutionAuthChecker.CheckFieldAuth` into `Executor.Execute`/`ExecuteParallel`.
11. **GQL-007:** Add Origin allow-list to `subscription.HandleSubscription` + `HandleSSE`; propagate authenticated consumer into subscription context.
12. **GQL-010:** Remove `path` argument from MCP `system.config.import`; accept only inline YAML/struct. Audit-log every import.
13. **SUPPLY-001:** Pin every third-party Action by full commit SHA; add `.github/dependabot.yml` with `github-actions` ecosystem.
14. **SUPPLY-002:** Remove `--set secrets.*` from workflows; use pre-provisioned Secrets with `resource-policy: keep`.

### Phase 3 — **MEDIUM / HARDENING** (target: 1 month)

15. **WASM-003:** Add `defer recover()` around every plugin call in `Pipeline.Execute` + `ExecutePostProxy` + inside `WASMModule.Execute`; emit `plugin_panic_total` metric.
16. **WASM-005/006/008:** Pass `execCtx` to `allowFn.Call`; cap JSON response size; route WASM through native factory registry; per-module semaphore.
17. **RAFT-006:** Collapse `ClusterManager.Start` into the admin server — register `/admin/api/v1/cluster/*` on the existing mux; unify header scheme to `X-Admin-Key`.
18. **RAFT-007:** Never replicate plaintext private keys; encrypt with KEK before `CmdCertificateUpdate`; chmod `apicerberus.db` to `0600`.
19. **RAFT-008:** Bounds + idempotency (`Nonce`) on `CmdUpdateCredits`/`UpdateRateLimit`/`IncrementCounter`.
20. **GQL-003:** Expand fragments during complexity visit; count aliases as distinct fields.
21. **GQL-008/009/011/012:** Generic planner error message; alias-inclusive step IDs; auth `/sse`; per-tool JSON schema validation with required nesting.
22. **PROXY-003/004/005:** Use allow-list (not deny-list) for gRPC metadata; reject upstream metadata that collides with security headers; port `parseConnectionTokens` into `OptimizedProxy`.
23. **PROXY-008/009:** Delete `ForwardWebSocket` or wire it with Origin check + frame/session limits.
24. **PROXY-010:** Reject `Content-Type: application/grpc*` unless matched route has `protocol: grpc`.
25. **SUPPLY-003/004/005/006:** Add `permissions: contents: read` to `ci.yml` top-level; pin all `:latest` images; default `networkPolicy.enabled: true`; set `automountServiceAccountToken: false` on Helm SA.
26. **DIFF-001 / DIFF-002 / DIFF-003:** Apply `securityContext` to backup CronJob + encrypt archives + quote Helm values (see `DIFF-SECURITY-REPORT.md`).

### Phase 4 — **LOW / INFO** (ongoing)

27. All remaining Low and Info items from the five subsystem files. Track via ticket or changelog; not release-blocking.

---

## 8. Positive Observations (Unchanged from Wave 1, confirmed in Wave 2)

- `gcr.io/distroless/static:nonroot` runtime image with no shell/package-manager.
- `govulncheck` + `trivy` + `gosec` SARIF integration in CI.
- `crypto/subtle.ConstantTimeCompare` correctly used for admin-key and API-key comparison.
- `crypto/rand` used for all key material and nonces.
- RSA 4096 for Raft CA/node certs (exceeds 2048 minimum).
- Radix router has length/segment/null-byte caps (`maxPathLength=8192`, `maxPathSegments=256`, `maxRegexLength=1024`).
- JWT `none` algorithm explicitly rejected; JTI replay fail-closed.
- Bcrypt for passwords; correct storage in SQLite with WAL mode.
- OIDC state + nonce properly validated; HttpOnly cookies.
- Admin WebSocket has strict Origin allow-list, rejects `Origin: null`, rate-limits fallback.
- Body-size gate runs before route match; LimitReader on chunked bodies.
- Kustomize base sets `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem: true`, `capabilities.drop: [ALL]`, `runAsNonRoot: true`, `automountServiceAccountToken: false` correctly.

---

## 9. Files in This Report Set

| File | Purpose |
|------|---------|
| `SECURITY-REPORT.md` | **This file** — consolidated current state |
| `verified-findings.md` | Wave 1 findings (historical; all fixed or accepted) |
| `DIFF-SECURITY-REPORT.md` | 2026-04-16 diff scan (commits since Wave 1) |
| `architecture.md` | Wave 1 architecture map |
| `dependency-audit.md` | Wave 1 Go/NPM dep audit |
| `raw-findings.md` | Wave 1 raw scanner output |
| `sc-wasm-plugin-results.md` | Wave 2 — WASM + plugin pipeline (12 findings) |
| `sc-raft-cluster-results.md` | Wave 2 — Raft + mTLS + cluster (12 findings) |
| `sc-federation-mcp-results.md` | Wave 2 — Federation + MCP (14 findings) |
| `sc-grpc-ws-proxy-results.md` | Wave 2 — gRPC/WS/proxy (10 findings) |
| `sc-supply-chain-results.md` | Wave 2 — Supply chain + IaC (12 findings) |

Every finding in §2–§6 points to a file:line in one of the `sc-*-results.md` files where the exploit scenario, confidence, and patch-level remediation live.

---

*Scan complete: 2026-04-16. Auditor: Claude Code (security-check skill, 5-agent parallel deep-scan).*
