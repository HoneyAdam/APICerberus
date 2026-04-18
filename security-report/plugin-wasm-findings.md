# Plugin & WASM Security Findings

**Scope:** internal/plugin/ (wasm.go, pipeline.go, registry.go, marketplace.go, hotreload.go, types.go)
**Date:** 2026-04-18
**Context:** Follow-up scan after fixes applied to SEC-WASM-001/002/003/004 findings from 2026-04-16 audit

---

## Verified Fixes (Previously Reported — Now Confirmed)

### SEC-WASM-001: Phase Validation Implemented
**File:** `internal/plugin/wasm.go:211-226`

Phase strings are now validated against the known set. `PhaseAuth` and `PhasePostProxy` are explicitly rejected for WASM modules:

```go
switch candidate := Phase(raw); candidate {
case PhasePreAuth, PhasePreProxy, PhaseProxy:
    return candidate, nil
case PhaseAuth:
    return "", fmt.Errorf("wasm module %q: phase %q is not permitted for WASM plugins")
case PhasePostProxy:
    return "", fmt.Errorf("wasm module %q: phase %q is not permitted for WASM plugins")
```

**Status:** FIXED. `TestResolveWASMPhase` (wasm_test.go:858-935) verifies all rejection cases.

---

### SEC-WASM-003: Panic Recovery in Execute
**File:** `internal/plugin/wasm.go:318-327`

```go
defer func() {
    if r := recover(); r != nil {
        handled = false
        err = fmt.Errorf("wasm module %q panicked: %v", m.id, r)
    }
}()
```

**Status:** FIXED in `WASMModule.Execute`. Native plugin `Run`/`AfterProxy` also have recover guards (pipeline.go:67-73, 88-91).

---

### SEC-WASM-004: Protected Headers Deny-List
**File:** `internal/plugin/wasm.go:756-789`

`wasmProtectedHeaders` maps 14 sensitive headers (Authorization, Proxy-Authorization, Cookie, Set-Cookie, X-Api-Key, X-Admin-Key, X-Forwarded-For, X-Real-IP, X-Forwarded-Host, X-Forwarded-Proto, Host, X-Consumer-Id, X-Consumer-Name, X-Correlation-Id, X-Trace-Id).

`ApplyToContext` uses `http.CanonicalHeaderKey` before the membership check, so casing variants are caught.

**Status:** FIXED for listed headers. `TestWASMContext_ApplyToContext_ProtectedHeadersDenied` (wasm_test.go:292-343) verifies all 18 test cases.

---

## New / Remaining Findings

### M-016: maxWASMReadSize Hard Cap (64MB)
**File:** `internal/plugin/wasm.go:448-453`
**Severity:** Low (defense-in-depth)
**Confidence:** High

```go
const maxWASMReadSize = 64 * 1024 * 1024 // 64MB hard cap per read
if length > maxWASMReadSize {
    return nil, fmt.Errorf("wasm memory read exceeds maximum size %d bytes", maxWASMReadSize)
}
```

Prevents a malicious module from claiming a huge length to trigger OOM during buffer allocation in `mem.Read`. Applied consistently in `readFromWASMMemory`.

**Note:** This is a host-side cap. The actual linear memory is still bounded by `WithMemoryLimitPages` at module instantiation time.

---

### M-017: EnvVars Field Acknowledged but Unwired
**File:** `internal/plugin/wasm.go:60-64`

```go
// M-017: EnvVars field exists but is NOT currently wired to wazero runtime.
// If WithEnvVars is used in the future, only allow known-safe variables
// (e.g., no API keys, secrets, or host paths).
```

`WASMConfig.Validate()` does not reject `EnvVars` or `AllowedPaths` — it returns nil unconditionally. If an operator populates these fields expecting functionality, the config passes validation but the feature does nothing.

**Severity:** Low (latent). Feature is explicitly unimplemented.

---

### Finding M-018: allocFn.Call Uses Unbounded Context
**File:** `internal/plugin/wasm.go:411-416`
**Severity:** Medium
**CWE:** CWE-400 (Resource Exhaustion)

```go
allocFn := mod.ExportedFunction("alloc")
if allocFn != nil {
    results, err := allocFn.Call(context.Background(), uint64(len(data)))
    //                                                     ^^^^^^^^^^^^^
    //                                    execCtx is NOT used here
```

`writeToWASMMemory` calls `allocFn.Call(context.Background(), ...)` — a fresh background context — rather than `execCtx` which carries the `MaxExecution` timeout. A guest-exported `alloc` that spin-loops runs forever; only the subsequent `fn.Call(execCtx, ...)` is bounded.

The JSON marshal of the request context also runs outside the timeout window (line 339-347).

**Remediation:**
```go
// Pass execCtx to alloc, not context.Background()
results, err := allocFn.Call(execCtx, uint64(len(data)))
```

---

### Finding M-019: Pipeline Phase Filtering Not Enforced at Execution
**File:** `internal/plugin/pipeline.go:15-36`
**Severity:** Medium (remains from WASM-002)
**CWE:** CWE-693 (Protection Mechanism Failure)

`Pipeline.Execute` iterates all plugins in a single loop with no phase filtering:

```go
for _, plugin := range p.plugins {
    handled, err := plugin.Run(ctx)
    ...
}
```

The phase is only used for sorting at build time (registry.go:253-262). `BuildRoutePipelinesWithContext` does not split plugins into per-phase slices — all plugins (pre-auth, auth, pre-proxy, proxy) are mixed in one slice and `Execute` runs them all sequentially.

WASM-002 from the prior audit described this as a contract violation. The fix described was to split execution by phase. This has not been implemented.

**Note:** With SEC-WASM-001 now forbidding `PhaseAuth` and `PhasePostProxy` for WASM, the practical impact is reduced — WASM plugins can only occupy phases that *would* run in a single-pass Execute anyway. However, the architectural issue remains: `phase` is cosmetic at runtime.

---

### Finding M-020: wasmProtectedHeaders Missing JWT Claim Headers
**File:** `internal/plugin/wasm.go:756`
**Severity:** Medium
**CWE:** CWE-290 (Authentication Bypass by Spoofing)

`wasmProtectedHeaders` blocks `Authorization`, `Cookie`, `X-Api-Key`, `X-Admin-Key`, etc., but does not include headers written by `auth_jwt.go` when `ClaimsToHeaders` is configured.

`auth_jwt.go:369` maps JWT claims to arbitrary header names via `ClaimsToHeaders`. A WASM plugin can overwrite these arbitrary headers:

```go
// auth_jwt.go
ClaimsToHeaders: map[string]string{
  "sub":   "X-Claim-Sub",
  "roles": "X-User-Roles",
  "email": "X-Claim-Email",
}
```

`wasmProtectedHeaders` has no entry for `X-Claim-*` headers. A WASM plugin declared in `pre-proxy` (which runs *after* `auth-jwt` because `auth-jwt` is PhaseAuth at priority 10 and WASM defaults to PhasePreProxy at priority 100) can overwrite claim-derived headers to escalate privileges against an upstream that trusts header-injected claims.

**Remediation:** Add `X-Claim-*` wildcard or enumerate all possible `ClaimsToHeaders` values to `wasmProtectedHeaders`.

---

### Finding M-021: Hot-Reload TOCTOU (Residual from WASM-006)
**File:** `internal/plugin/hotreload.go:143-162`, `internal/plugin/wasm.go:570-573`
**Severity:** Medium
**CWE:** CWE-367 (TOCTOU Race Condition)

`HotReloader.Reload` atomically swaps the registry under a write lock. Existing route pipelines hold captured references to old plugins. The `WASMPluginManager.LoadModule` unloads and closes existing modules without quiescing in-flight `Execute` calls.

The prior audit recommended reference-counting with a grace period before closing. This has not been implemented.

**Note:** The prior scan noted `m.mu.RUnlock()` at `wasm.go:332` releases the lock before `fn.Call` at line 367 — a concurrent `Close` can finalize the module while `fn.Call` is in progress. No mechanism prevents this.

---

### Finding M-022: Marketplace Checksum Covers Archive, Not Per-File Contents
**File:** `internal/plugin/marketplace.go:358-369`, `internal/plugin/wasm.go:138-168`
**Severity:** Medium
**CWE:** CWE-345 (Insufficient Verification of Data Authenticity)

At install time, the SHA-256 of the downloaded `.tar.gz` bytes is verified against `listing.Checksums[version]` (line 366-369). The signature is over the archive, not its contents.

Individual extracted files are not checksummed. On every `LoadModule`, only the WASM magic header + size is revalidated (`validateWASMModule` at wasm.go:138-168). If an attacker modifies an extracted `.wasm` file post-install (e.g., via a separate container escape), the gateway loads the tampered module on next reload.

**Remediation:** Store per-file SHA-256 hashes in `metadata.json` at install time and verify on `LoadModule`.

---

### Finding M-023: Marketplace Extraction Size Check Off-by-One
**File:** `internal/plugin/marketplace.go:685-694`
**Severity:** Low
**CWE:** CWE-409 (Improper Handling of Highly Compressed Data)

```go
written, err := io.CopyN(outFile, tarReader, maxExtractSize-extractedSize+1)
extractedSize += written
if extractedSize > maxExtractSize {
    return fmt.Errorf("extracted plugin exceeds maximum size")
}
```

`io.CopyN` copies at most `maxExtractSize-extractedSize+1` bytes per entry. If the total written exceeds `maxExtractSize`, the error is returned but the file remains on disk (`outFile` is only closed, not deleted). Orphan partial files accumulate in `DataDir/installed/<id>/`.

---

### Finding M-024: WASM Plugin Manager Bypasses Native Factory System
**File:** `internal/plugin/registry.go:181-207`, `internal/plugin/wasm.go:623-638`
**Severity:** Low (reduced by SEC-WASM-001 fix)
**CWE:** CWE-1188 (Insecure Default Initialization)

`NewDefaultRegistry()` lists 24 native plugin factories. WASM modules are not registered as a factory — `WASMPluginManager.CreatePipelinePlugin` is called outside `BuildRoutePipelinesWithContext`.

Consequences (partially mitigated by SEC-WASM-001 phase validation):
- `PluginConfig.Enabled *bool` is not honored for WASM plugins
- No consumer-context injection for WASM auth
- No priority bounds check
- WASM module name collision with native plugins not detected

**Note:** With `PhaseAuth` and `PhasePostProxy` now forbidden for WASM, the most dangerous exploit scenarios (auth bypass) are blocked. This finding remains as an architectural concern.

---

## Positive Findings (Good Practices Confirmed)

- **Memory limit pages** at wasm.go:106 (`WithMemoryLimitPages`) bounds linear memory
- **Magic header validation** at wasm.go:160-166 rejects non-WASM files
- **Path traversal check** at wasm.go:186 (`strings.HasPrefix(rel, "..")`) prevents directory escape
- **Hard module size cap** at wasm.go:23 (`maxWASMModuleSize = 100MB`) limits loading oversized modules
- **maxWASMReadSize 64MB cap** at wasm.go:450 prevents large-read OOM
- **Panic recovery** in WASMModule.Execute at wasm.go:322-327
- **WASI opt-in** at wasm.go:111 — WASI only instantiated when `AllowFilesystem: true`
- **Marketplace Ed25519 verification** at marketplace.go:626-628 with constant-time comparison
- **Marketplace HTTP client timeouts** at marketplace.go:233-241 prevent slowloris
- **Constant-time API key comparison** in auth_apikey.go (confirmed from prior scan)
- **JWT `none` algorithm rejection** in auth_jwt.go (confirmed from prior scan)

---

## Summary

| ID | Severity | Status |
|----|----------|--------|
| SEC-WASM-001 | High | FIXED |
| SEC-WASM-003 | High | FIXED |
| SEC-WASM-004 | High | PARTIALLY FIXED (M-020 remaining) |
| M-016 | Low | CONFIRMED |
| M-017 | Low | CONFIRMED (latent) |
| M-018 | Medium | OPEN — alloc unbounded |
| M-019 | Medium | OPEN — phase filtering not enforced |
| M-020 | Medium | OPEN — missing claim header protection |
| M-021 | Medium | OPEN — hot-reload TOCTOU |
| M-022 | Medium | OPEN — per-file checksum missing |
| M-023 | Low | OPEN — extraction orphan files |
| M-024 | Low | OPEN — bypasses factory system |

**Critical:** 0 | **High:** 0 | **Medium:** 5 | **Low:** 3

**Top priorities:**
1. **M-020** — Add `X-Claim-*` to `wasmProtectedHeaders` or enumerate `ClaimsToHeaders` values
2. **M-018** — Pass `execCtx` to `allocFn.Call` in `writeToWASMMemory`
3. **M-019** — Implement phase-filtered execution in `Pipeline.Execute` (architectural fix)
4. **M-021** — Add reference-counting grace period before closing WASM modules on reload
