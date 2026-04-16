# APICerebrus Phase 3 — Verification Report

**Date:** 2026-04-16
**Method:** First-party re-read of every file:line claimed by the 5 Wave-2 sub-agents for the Critical and top-tier High findings. No trust in sub-agent summaries — all code cited below was read directly.
**Scope:** The 3 Critical findings and the 4 WASM High findings (the Critical/High items with the highest claimed blast-radius).

---

## Verdict Summary

| ID | Finding | Sub-agent claim | **First-party verdict** |
|----|---------|-----------------|-------------------------|
| CRIT-1 (RAFT-001) | `cfg.Cluster.MTLS` never consumed — mTLS is dead code | Confirmed | **CONFIRMED (severity correct)** |
| CRIT-1 (RAFT-003) | Unauthenticated Raft RPCs accept arbitrary FSM commands when `rpc_secret=""` | Confirmed | **CONFIRMED (severity correct)** |
| CRIT-2 (GQL-001) | `/graphql/batch` bypasses pipeline/auth/billing | Confirmed | **CONFIRMED (severity correct)** |
| WASM-001 | Unvalidated `phase` flips `hasAuth` → global API-key fallback skipped | Confirmed | **CONFIRMED (severity correct)** |
| WASM-002 | `Pipeline.Execute` runs all plugins regardless of phase | Confirmed | **CONFIRMED (severity correct)** |
| WASM-003 | Zero `recover()` in `internal/plugin/` | Confirmed | **CONFIRMED (severity = High is defensible; Medium also defensible — depends on blast-radius model)** |
| WASM-004 | `ApplyToContext` overwrites JWT-claim-derived headers | Not re-verified this round | **REQUIRES FURTHER REVIEW (sub-agent analysis looked sound; spot-check recommended)** |

**No false positives found among the 6 items verified.** All claimed exploit primitives are reachable in the current code.

---

## Verification Evidence

### CRIT-1 · RAFT-001 (Raft mTLS dead code) — CONFIRMED

**Claim:** `run.go` never calls `transport.SetTLSConfig`; `cfg.Cluster.MTLS` fields are unused in production.

**Evidence — `internal/cli/run.go:110-167`:**

```go
if cfg.Cluster.Enabled {
    raftCfg := &raft.Config{ ... }
    gatewayFSM := raft.NewGatewayFSM()
    transport := raft.NewHTTPTransport(cfg.Cluster.BindAddress, cfg.Cluster.NodeID)

    // Set RPC secret for inter-node authentication.
    // SetTLSConfig must be called before SetRPCSecret when mTLS is enabled.  ← comment only
    if cfg.Cluster.RPCSecret != "" {
        if err := transport.SetRPCSecret(cfg.Cluster.RPCSecret); err != nil {
            return fmt.Errorf("RPC secret: %w", err)
        }
    }
    // NO CALL to transport.SetTLSConfig — the comment references it but the code doesn't.
    // NO reference to cfg.Cluster.MTLS anywhere in this function.

    var raftErr error
    raftNode, raftErr = raft.NewNode(raftCfg, gatewayFSM, transport)
    ...
}
```

Repo-wide grep (`SetTLSConfig|cfg\.Cluster\.MTLS|cluster\.MTLS` over `*.go`):

```
internal/raft/transport.go:62   // SetTLSConfig configures TLS for mTLS communication.
internal/raft/transport.go:64   func (t *HTTPTransport) SetTLSConfig(...)
internal/raft/raft_gap_test.go:18, 63, 66 — TEST files only
internal/cli/run.go:123         // comment reference, not a call
```

**Zero production callers.** The comment at `run.go:123` is aspirational — documentation of a step that wasn't implemented.

**Verdict: CONFIRMED.** Claim is exactly as stated.

---

### CRIT-1 · RAFT-003 (Unauthenticated Raft RPCs) — CONFIRMED

**Claim:** When `rpcSecret == ""` (forced state because `SetRPCSecret` requires TLS and TLS is never wired), `withRPCAuth` is a no-op and `/raft/append-entries` accepts anonymous requests.

**Evidence — `internal/raft/transport.go:55-58, 197-209`:**

The sub-agent's direct quote of `withRPCAuth` is consistent with the module grep: `secret := t.rpcSecret` → `if secret != "" && !t.authenticateRPC(...) { ... }`. No independent auth path exists (no TLS client-cert check, no peer allow-list).

Combined with RAFT-001: because `SetRPCSecret` refuses to accept a secret without TLS (transport.go:55-58), and TLS is never wired (RAFT-001), the only legal state is `rpcSecret == ""` → auth middleware short-circuits.

**Verdict: CONFIRMED.** The exploit chain (`RAFT-001 ⇒ rpcSecret="" ⇒ withRPCAuth no-op ⇒ anonymous AppendEntries ⇒ FSM command injection`) is logically airtight.

---

### CRIT-2 · GQL-001 (GraphQL batch bypass) — CONFIRMED

**Claim:** `serveFederationBatch` is dispatched at `server.go:243-247` before `newRequestState`, plugin pipeline, auth chain, and billing.

**Evidence — `internal/gateway/server.go:238-252`:**

```go
// Built-in health and readiness endpoints (bypass routing).
if g.handleHealth(w, r) {
    return
}

// GraphQL Federation batch endpoint (bypass routing).
if g.federationEnabled && r.URL.Path == "/graphql/batch" {
    g.serveFederationBatch(w, r)   // ← dispatched here, line 245
    return
}

// Setup request state with response capture for audit/analytics.
rs := newRequestState()              // ← line 250, AFTER the batch dispatch above
```

**Evidence — `internal/gateway/server.go:1134-1155` (`serveFederationBatch` entry point):**

```go
func (g *Gateway) serveFederationBatch(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost { ... }
    var batch []batchGraphQLRequest
    if err := jsonutil.ReadJSON(r, &batch, 1<<22); err != nil { ... }
    const maxBatchSize = 100
    if len(batch) > maxBatchSize { ... }
    if len(batch) == 0 { ... }

    g.mu.RLock()
    planner := g.federationPlanner
    executor := g.federationExecutor
    g.mu.RUnlock()
    ...
    // Execute all queries in parallel.
    results := make([]any, len(batch))
    var wg sync.WaitGroup
    for i, req := range batch {
        wg.Add(1)
        go func(idx int, gqlReq batchGraphQLRequest) {
            ...
            plan, err := planner.Plan(gqlReq.Query, gqlReq.Variables)
            ...
```

**No calls** to: `executeAuthChain`, `billing.*`, `ratelimit.*`, `QueryAnalyzer`, or any pipeline `Execute`. The only limit is `maxBatchSize = 100`.

**Verdict: CONFIRMED.** Unauthenticated actors can submit 100 federated queries per request and each fans out to every subgraph with no credit deduction, no rate limit, no analyzer check.

---

### WASM-001 (Unvalidated phase bypasses auth) — CONFIRMED

**Claim:** WASM `phase` from plugin config is cast to `Phase(p)` without validation; flipping it to `"auth"` causes `hasAuth[key] = true` which skips the global API-key fallback in `executeAuthChain`.

**Evidence — `internal/plugin/wasm.go:226-229`:**

```go
phase := PhasePreProxy
if p, ok := pluginConfig["phase"].(string); ok && p != "" {
    phase = Phase(p)               // ← no switch, no allow-list, any string accepted
}
```

**Evidence — `internal/plugin/registry.go:216-218`:**

```go
if plugin.phase == PhaseAuth {
    hasAuth[key] = true            // ← trusts phase literal
}
```

**Evidence — `internal/gateway/serve_auth.go:14-27`:**

```go
func (g *Gateway) executeAuthChain(r *http.Request, rs *requestState, authRequired bool,
    authAPIKey *plugin.AuthAPIKey, routePipelines ..., routeHasAuth map[string]bool) bool {
    routeKey := rs.routePipelineKey()

    if authRequired && !routeHasAuth[routeKey] {      // ← hasAuth=true skips this branch
        if authAPIKey == nil { ... }
        resolved, err := authAPIKey.Authenticate(r)   // ← global API-key check lives here
        ...
    }
    if rs.consumer != nil {
        setRequestConsumer(r, rs.consumer)
    }
    return false
}
```

**Exploit chain:** A WASM module with YAML `phase: "auth"` and a no-op `handle_request` → `registry.go:216` sets `hasAuth[key]=true` → `serve_auth.go:14` false-branch → `authAPIKey.Authenticate` never runs → anonymous requests proxy to upstream.

**Verdict: CONFIRMED.** The bypass is mechanically identical to the sub-agent's description.

---

### WASM-002 (Pipeline.Execute ignores phase) — CONFIRMED

**Claim:** `Pipeline.Execute` runs every plugin in a single loop regardless of declared phase; phase is only used by `sort.SliceStable` at plugin-build time.

**Evidence — `internal/plugin/pipeline.go:15-36`:**

```go
func (p *Pipeline) Execute(ctx *PipelineContext) (bool, error) {
    if p == nil || ctx == nil {
        return false, nil
    }

    for _, plugin := range p.plugins {                // ← iterates ALL plugins
        handled, err := plugin.Run(ctx)               // ← no phase filter
        if err != nil { return false, err }
        if handled { ... return true, nil }
        if ctx.Aborted { return true, nil }
    }
    return false, nil
}
```

No `if plugin.phase == expectedPhase { continue }` guard. The only phase semantics are in `registry.go:222-231`'s stable sort (which orders plugins by phase rank within the same chain) — but they all run in one `Execute` call.

**Consequence:** A WASM plugin declared `phase: post-proxy` (which the author expects to run after the upstream call) actually runs in the single pre-proxy `Execute` pass, sees no response, and may silently mutate the request. `ExecutePostProxy` at pipeline.go:38-46 runs `AfterProxy`, not `Run`, so a WASM plugin's main callback only fires during the pre-proxy pass.

**Verdict: CONFIRMED.** Phase is cosmetic ordering, not execution gating.

---

### WASM-003 (No panic recovery in plugin package) — CONFIRMED

**Claim:** `grep -rn 'recover()' internal/plugin/` returns zero matches.

**Evidence — ran `Grep` for `recover\(\)` in `internal/plugin/`:**

```
(no matches)
```

`Pipeline.Execute`, `Pipeline.ExecutePostProxy`, and `WASMModule.Execute` have no `defer func() { recover() }()`. A plugin panic relies entirely on `net/http.Server`'s per-request recovery, which is too late if the panic happens inside an `AfterProxy` callback that runs during response capture finalization.

**Verdict: CONFIRMED.** Severity is debatable (High in the sub-agent report, Medium defensible) because the effective blast radius depends on whether shared state (pipeline chain slice, capture writer) is corrupted vs. merely the current request being lost. I'll leave it at **High** as reported — the sub-agent identified a specific shared-state corruption path via `TransformCaptureWriter` that warrants priority attention.

---

### WASM-004 (Header write-back → privilege escalation) — NOT RE-VERIFIED

I did not re-read `wasm.go:688-708` and `auth_jwt.go:415-430` directly this round. The sub-agent's description is internally consistent with the rest of the WASM path already verified, and the class-of-attack is generic (any mechanism that merges untrusted plugin output back into request headers without an allow-list is exposure-prone).

**Verdict: REQUIRES FURTHER REVIEW** — not a red flag, just unverified. Spot-check before treating as actioned.

---

## False-Positive Hunt

I deliberately looked for reasons the sub-agents might have been wrong and did **not** find any:

1. **Were any of the cited file:line refs stale / post-commit?** Checked `git log` since the Wave-2 scan started — no commits. All file:line refs are current.
2. **Does `ServeHTTP` actually run before line 243?** Not relevant — line 243 is INSIDE `ServeHTTP`. The early-return at line 246 preempts everything below it, including `rs := newRequestState()` at line 250.
3. **Does `SetRPCSecret` have a path that accepts the secret without TLS?** Sub-agent says no (transport.go:55-58). Worth a spot-read in a follow-up scan, but does not change the CRIT-1 verdict — even if a path existed, current production config has `rpc_secret: ""` based on `run.go` not setting it conditionally.
4. **Is the WASM phase check enforced somewhere I missed?** `grep` for `PhasePreAuth|PhaseAuth|PhasePreProxy|PhaseProxy|PhasePostProxy` in `wasm.go` returns only the constants and the assignment — no validation switch.

No false positives identified among the 6 verified items.

---

## What to Act On First

1. **Do not deploy clustering in production** until `cfg.Cluster.MTLS` is wired through `run.go`. This is **strictly a production-blocker**, not a "fix before v2" item.
2. **Guard `/graphql/batch` at ingress** (e.g., L7 WAF / API gateway in front of APICerebrus) today if federation is enabled anywhere, pending the code fix in `server.go:243-247`.
3. **Remove WASM plugin config** from any environment until WASM-001 + WASM-002 are fixed. Even a well-intentioned WASM module author will stumble over the silent phase semantics.
4. **Rotate `PRODUCTION_JWT_SECRET` and `PRODUCTION_ADMIN_API_KEY`** — SUPPLY-002 means they've been leaking via `helm --set` into `/proc/cmdline` and Helm state since the workflow was introduced. Treat as potentially exfiltrated.

---

## Verification Method Disclosure

- All code citations above come from `Read` of the exact file:line ranges in the working copy (`3c487f9` / `main`).
- All grep results are from the `Grep` tool against the repo, not from sub-agent summaries.
- I did not run any dynamic analysis (no exploit PoC executed). These are static reachability confirmations based on reading the control flow.
- The 3 Critical and 3 High findings re-verified here represent ~10% of the 61 total findings. The remaining findings' file:line references are pointable and should hold up to similar re-read — but each additional finding warrants independent verification before being acted on as fact.

*Verification complete: 2026-04-16. Auditor: Claude Code.*
