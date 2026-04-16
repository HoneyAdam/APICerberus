# WASM & Plugin Pipeline Audit

**Scope:** internal/plugin/**
**Date:** 2026-04-16
**Auditor:** Targeted WASM / pipeline review (supplement to full scan)

---

## Findings

### WASM-001: Unvalidated `phase` from plugin config allows authenticated-phase spoofing and bypass

- **Severity:** High
- **Confidence:** 90
- **CWE:** CWE-863 (Incorrect Authorization), CWE-20 (Improper Input Validation)
- **File:** `internal/plugin/wasm.go:226-229`
- **Description:**
  `WASMRuntime.LoadModule` reads the plugin phase directly from the module author's config with **no validation** against the known `Phase` set:

  ```go
  phase := PhasePreProxy
  if p, ok := pluginConfig["phase"].(string); ok && p != "" {
      phase = Phase(p)                         // arbitrary string accepted
  }
  ```

  The registration path in `registry.go:216` then trusts that phase literally:

  ```go
  if plugin.phase == PhaseAuth {
      hasAuth[key] = true                      // marks the route as "has auth"
  }
  ```

  Because `routeHasAuth[key]` is later consumed in `gateway/serve_auth.go:14` to decide **whether the global API-key fallback runs**, a WASM module that only declares `phase: "auth"` in its YAML config flips that flag to `true` even though the module never authenticates anything.
- **Exploit scenario:**
  An attacker with config-write access (compromised admin token, misconfigured GitOps pipeline, or malicious plugin author) ships a `.wasm` module whose YAML stanza is:

  ```yaml
  name: helper
  phase: auth           # <-- lies about being an auth plugin
  priority: 1
  ```

  The module's `handle_request` trivially returns `{"handled": false}`. At pipeline build time, `hasAuth[route]=true`, so `executeAuthChain` skips calling `authAPIKey.Authenticate(r)`. Requests with **no API key** now reach the upstream. The same trick with `phase: "pre-auth"` lets a WASM plugin mutate headers (e.g. inject a consumer identity via `WASMContext.Headers`, which `ApplyToContext` writes back to `req.Header`) **before** any real AuthN plugin runs — effectively allowing consumer impersonation against downstream services that trust header-injected claims.
- **Remediation:**
  1. Validate `phase` in `LoadModule`:
     ```go
     switch Phase(p) {
     case PhasePreAuth, PhaseAuth, PhasePreProxy, PhaseProxy, PhasePostProxy:
         phase = Phase(p)
     default:
         return nil, fmt.Errorf("wasm module %q has invalid phase %q", id, p)
     }
     ```
  2. Forbid WASM modules in `PhaseAuth` entirely unless an allow-list of signed module IDs permits it — auth is too critical to delegate to guest code.
  3. In `BuildRoutePipelinesWithContext`, only count a plugin toward `hasAuth` if its factory is a known-native auth plugin (track it on the factory, not via `plugin.phase`).

---

### WASM-002: Pipeline `Execute` runs every plugin regardless of declared phase — "phase order" is cosmetic

- **Severity:** High
- **Confidence:** 95
- **CWE:** CWE-693 (Protection Mechanism Failure)
- **File:** `internal/plugin/pipeline.go:20-35`, `internal/gateway/server.go:283`
- **Description:**
  `Pipeline.Execute` iterates *all* chained plugins in one loop:
  ```go
  for _, plugin := range p.plugins {
      handled, err := plugin.Run(ctx)
      ...
  }
  ```
  The phase is only consulted by the stable sort in `registry.go:222-231`; it does not gate execution. Combined with **WASM-001**, this means a WASM plugin declared as `phase: "post-proxy"` still has its `Run` callback invoked **before** the proxy call (during the pre-proxy `Execute`). The documented pipeline contract `PRE_AUTH → AUTH → PRE_PROXY → PROXY → POST_PROXY` is not enforced — only the relative ordering of `Run()` calls is.

  Practical consequence: `after`/`ExecutePostProxy` is the only thing that distinguishes "post-proxy" plugins (`pipeline.go:39-46`), yet a WASM module's `after` is never wired (see `wasm.go:585-588` — no `after` field). So a WASM plugin that authors believe will observe the response actually runs *pre-proxy* and sees no response at all, but may still silently mutate the request.
- **Exploit scenario:**
  A defender writing a "PII-redaction post-proxy" WASM plugin assumes it only sees the response. In reality, the plugin's `Run` fires before the upstream call, the response is never exposed, and any developer who relies on `phase: post-proxy` for a security decision has a false sense of defense-in-depth. An attacker exploiting WASM-001 takes advantage of this ambiguity: the documented separation between phases is not load-bearing.
- **Remediation:**
  1. Split `Pipeline` execution by phase. Each phase gets its own slice; the gateway calls `pipeline.ExecutePhase(PhasePreAuth)`, then after AuthN native-resolution calls `ExecutePhase(PhaseAuth)`, etc.
  2. Explicitly filter in `Execute`:
     ```go
     for _, plugin := range p.plugins {
         if plugin.phase != expectedPhase { continue }
         ...
     }
     ```
  3. Route the WASM `after` callback when `phase == post-proxy` via the existing `PipelinePlugin.after` field.

---

### WASM-003: Plugin/WASM panics crash the entire gateway process

- **Severity:** High
- **Confidence:** 95
- **CWE:** CWE-248 (Uncaught Exception), CWE-400 (Resource Exhaustion)
- **File:** `internal/plugin/pipeline.go:20-35`, `internal/plugin/wasm.go:275-352`
- **Description:**
  Neither `Pipeline.Execute`, `Pipeline.ExecutePostProxy`, nor `WASMModule.Execute` wraps the per-plugin call in `defer recover()`. A grep across the entire `internal/plugin/` tree returns **zero** `recover()` calls:
  ```
  $ grep -rn 'recover()\|panic(' internal/plugin/
  (no matches)
  ```
  The upstream HTTP server (`net/http.Server`) has its own per-request recover for the top-level handler, **but** the Raft FSM, shutdown hooks, and async audit buffer all call through code paths that may fan out from plugins (e.g., `buildCompressionPlugin.after` running during `ExecutePostProxy`). A panic in `after` is not recovered because `ExecutePostProxy` is called from `runAfterProxy` inside `request_state.go:84` — and while `net/http` will recover the serving goroutine, a panic during the compression `after` or a WASM module's deferred close can corrupt shared plugin state (e.g., partially-closed response captures in `request_state.go:85-90`) or skip critical audit flush paths.

  For WASM specifically: `fn.Call(execCtx, …)` at `wasm.go:318` relies on wazero's internal panic-to-error translation, which is generally sound, but any host-side panic (e.g., nil `mod.Memory()` panicking later in `writeToWASMMemory` on an untrusted guest with unusual exports) propagates unwrapped.
- **Exploit scenario:**
  A malicious or buggy WASM module exports an `alloc` function that returns `ptr=0xFFFFFFFF`. `mem.Write(ptr, data)` at `wasm.go:369` returns `false` (handled), but an earlier guest-triggered host panic in an auxiliary plugin's `after` callback (e.g., a compression plugin panicking on a corrupted `Content-Encoding` header the WASM plugin injected) aborts the response flush for the current request and leaves `TransformCaptureWriter` state inconsistent for future requests sharing the pipeline (the pipeline chain slice is shared across requests — see `gateway/server.go:274` `plugin.NewPipeline(chain)`, where `chain` is reused).
- **Remediation:**
  1. Add per-plugin recover in both execution methods:
     ```go
     for _, plugin := range p.plugins {
         handled, err := func() (bool, error) {
             defer func() {
                 if r := recover(); r != nil {
                     log.Printf("[ERROR] plugin %s panic: %v\n%s", plugin.name, r, debug.Stack())
                     // surface as 500 via err
                 }
             }()
             return plugin.Run(ctx)
         }()
         ...
     }
     ```
  2. Same pattern for `ExecutePostProxy` and inside `WASMModule.Execute` around `fn.Call`.
  3. Emit a counter (`plugin_panic_total{plugin=…}`) so ops can alert on runaway modules.

---

### WASM-004: `WASMContext.ApplyToContext` silently overwrites authenticated request headers post-AuthN

- **Severity:** High
- **Confidence:** 85
- **CWE:** CWE-290 (Authentication Bypass by Spoofing), CWE-501 (Trust Boundary Violation)
- **File:** `internal/plugin/wasm.go:688-708`
- **Description:**
  When a WASM module returns a response context, the gateway merges every header back into the live request without filtering:
  ```go
  for k, v := range wc.Headers {
      ctx.Request.Header.Set(k, v)          // unconditional overwrite
  }
  ...
  if wc.CorrelationID != "" {
      ctx.CorrelationID = wc.CorrelationID
  }
  ```
  There is no block-list for sensitive headers (`Authorization`, `X-API-Key`, `X-User-Id`, `X-Admin-Key`, `X-Forwarded-For`, `Cookie`) nor for headers injected by prior auth plugins (e.g., `AuthJWT.applyClaimHeaders` writes claim-derived headers into the request at `auth_jwt.go:428` — a later WASM plugin can overwrite them to forge JWT claims for the upstream).

  `auth_jwt.go:415-430` explicitly converts JWT claims into headers consumed by upstream services. A WASM module running in a later phase can rewrite `X-Claim-Sub`, `X-Claim-Roles`, etc., effectively performing privilege escalation against any downstream service that trusts those headers as proof of authentication.
- **Exploit scenario:**
  Route `/api/admin/*` uses `auth-jwt` with `claims_to_headers: { roles: "X-User-Roles" }`. A benign-looking WASM plugin "url-normalizer" registered with `phase: pre-proxy` returns:
  ```json
  { "context": { "headers": { "X-User-Roles": "admin,superuser" } } }
  ```
  All requests through that route reach the upstream with attacker-chosen role headers, regardless of the actual JWT claims. The upstream believes the gateway authenticated the user as admin.
- **Remediation:**
  1. Introduce a configurable header allow-list per WASM module (`allowed_headers`) and reject writes outside it.
  2. Always block a fixed deny-list: `Authorization`, `Cookie`, `Set-Cookie`, `X-Forwarded-For`, any header beginning with `X-Claim-`, any header in `claims_to_headers` values, `X-Admin-Key`, `X-Webhook-Signature`.
  3. Log/metric every rejected overwrite so tampering is observable.
  4. Consider making `ApplyToContext` additive only (Set only if not already present) unless the plugin declares `can_overwrite_headers: true`.

---

### WASM-005: Execution timeout enforces wall-clock only — unbounded host-side data copying and allocator loops

- **Severity:** Medium
- **Confidence:** 80
- **CWE:** CWE-400 (Uncontrolled Resource Consumption), CWE-770 (Allocation of Resources Without Limits)
- **File:** `internal/plugin/wasm.go:286-318`, `wasm.go:362-390`
- **Description:**
  `Execute` creates a context with `MaxExecution` timeout (`wasm.go:286`) and passes it to `fn.Call`. wazero honors the context to abort the guest at instruction boundaries. However:

  1. The **pre-call marshalling** (`json.Marshal` of the full request context at `wasm.go:291-295`) runs **outside** the timeout. A pathological `Metadata` map (set by an earlier plugin in the chain) can make this expensive before the timer even starts.
  2. `writeToWASMMemory` calls `allocFn.Call(context.Background(), …)` at `wasm.go:364` — a **fresh background context**, not `execCtx`. A guest-defined `alloc` that spin-loops will run forever; only the subsequent `fn.Call(execCtx, ...)` is bounded.
  3. `json.Unmarshal(resultBytes, &resp)` at `wasm.go:338` also runs outside the timeout. Combined with the 64MB `maxWASMReadSize` cap at `wasm.go:401`, a malicious module returning a 64MB JSON bomb (deeply-nested objects) can stall the host-side unmarshal far beyond `MaxExecution`.
  4. There is no per-request memory accounting — `MaxMemory` is a per-**module** memory limit at runtime creation time, not per-invocation. A burst of N concurrent requests to a route with a WASM module consumes N × MaxMemory (default 128MB × concurrent_requests) because each module instance retains its linear memory. Under load this causes OOM.
- **Exploit scenario:**
  An attacker uploads a signed plugin (see marketplace signing) that exports `alloc` as an infinite loop. Any request routed through this plugin hangs indefinitely in `allocFn.Call(context.Background(), ...)`. 100 concurrent requests → 100 goroutines pinned, exhausting the worker pool and denying service for the entire gateway.
- **Remediation:**
  1. Pass `execCtx` (not `context.Background()`) to `allocFn.Call`.
  2. Wrap the pre/post JSON marshal/unmarshal in the same timeout:
     ```go
     execCtx, cancel := context.WithTimeout(context.Background(), timeout)
     defer cancel()
     done := make(chan struct{})
     go func() { defer close(done); reqBytes, err = json.Marshal(...) }()
     select { case <-done: case <-execCtx.Done(): return false, execCtx.Err() }
     ```
     Or use `json.NewDecoder(...).Decode` with a `LimitReader` and enforce a max-size (e.g., 1MB) on response JSON.
  3. Add a semaphore around `Execute()` to cap concurrent WASM invocations per module (e.g., `max_concurrency: 50`).
  4. Reinstantiate the module per call (at cost of performance) **or** reset the memory to zero between calls — wazero `Module.CloseWithExitCode(0)` + recompile is one approach.

---

### WASM-006: Hot-reload TOCTOU — requests in flight observe a half-swapped registry and leaked closed modules

- **Severity:** Medium
- **Confidence:** 80
- **CWE:** CWE-367 (TOCTOU Race Condition), CWE-416 (Use After Free)
- **File:** `internal/plugin/hotreload.go:143-162`, `internal/plugin/wasm.go:512-533`
- **Description:**
  `HotReloader.Reload` swaps the registry under a write lock, but the existing route pipelines built from the *old* registry continue to hold captured references to the old plugins:

  ```go
  func (h *HotReloader) Reload(paths []string, newRegistry *Registry) {
      h.mu.Lock()
      oldRegistry := h.registry
      h.registry = newRegistry      // old plugins still live in existing pipelines
      ...
      h.mu.Unlock()
      ...
  }
  ```

  Pipelines are captured at route-build time in `gateway/server.go:273-274` (`chain := routePipelines[routeKey]; rs.pipeline = plugin.NewPipeline(chain)`). These slices are **not re-fetched per request** under a lock — `server.go:203-205` reads them via `g.routePipelines` and passes them to `handleRequest`, but the Gateway-level swap happens in `g.ReloadConfig` (around `server.go:500-506`) without draining in-flight requests.

  More seriously, `WASMPluginManager.LoadModule` at `wasm.go:520-524` unloads an existing module by the same ID and immediately replaces it:
  ```go
  if existing, ok := m.modules[id]; ok {
      _ = existing.Close()      // frees wazero compiled+instantiated module
      delete(m.modules, id)
  }
  ```
  Any request currently executing `existing.Execute(ctx)` may be mid-call when `existing.Close()` runs because `Close` takes `m.mu.Lock()` (exclusive) while `Execute` holds `m.mu.RLock()` — **but only around reading `mod`** (`wasm.go:280-283`). After the RLock is released, the actual `fn.Call` proceeds on a module that a concurrent `Close` may have already finalized. `api.Module.Close` invalidates the memory, so concurrent reads from `mem.Read`/`mem.Write` can return stale data or trigger host panics.
- **Exploit scenario:**
  Attacker triggers a config reload (legitimate admin action or exploiting webhook/CLI access) that swaps a WASM module while 200 requests are mid-flight on the affected route. Roughly 5-10% of those requests either:
  - Return garbled/empty responses (memory freed mid-read), or
  - Trigger host-level panics that reach the gateway's lack of recover (WASM-003).

  If attacker controls both sides (timing the reload + sending concurrent requests), they can deterministically corrupt response bodies, bypassing response-transform or compression plugins that run afterward.
- **Remediation:**
  1. Use immutable pipeline snapshots per request: on config reload, build new pipelines, atomically swap `g.routePipelines` (already-done), and let **in-flight requests keep using the old pipeline until they complete** via reference counting.
  2. For WASM: don't `Close()` the old module immediately. Keep it in a "retired" set with a timer (`MaxExecution + grace`) before finalizing. Implement a reference count in `WASMModule` incremented/decremented around `Execute`.
  3. Drain inbound requests or use `sync.WaitGroup` when reloading; only close retired modules after `Wait()` returns.
  4. Document that hot-reload has a bounded quiescence window.

---

### WASM-007: Shared `PipelineContext.Metadata` map is an implicit cross-plugin information channel

- **Severity:** Medium
- **Confidence:** 75
- **CWE:** CWE-668 (Exposure of Resource to Wrong Sphere), CWE-200 (Information Exposure)
- **File:** `internal/plugin/registry.go:15-29`, `internal/plugin/wasm.go:680-702`
- **Description:**
  `PipelineContext.Metadata map[string]any` is shared by reference across every plugin in a chain (`registry.go:24`). A WASM module receives the entire metadata map serialized into its sandboxed JSON context (`wasm.go:682-684`):
  ```go
  for k, v := range ctx.Metadata {
      wc.Metadata[k] = v
  }
  ```
  This includes metadata set by prior plugins. For example, the rate-limit plugin checks `ctx.Metadata["skip_rate_limit"]` (`registry.go:388-390`), and the billing layer (`internal/billing/*`) stores per-request cost/credit data in Metadata. If a WASM plugin is placed in the chain, it receives and can modify that state.

  On the way back, `WASMContext.ApplyToContext` (`wasm.go:699-701`) merges WASM-returned Metadata into the live context:
  ```go
  for k, v := range wc.Metadata {
      ctx.Metadata[k] = v
  }
  ```
  This allows a WASM plugin to set `skip_rate_limit=true` (bypassing rate limits even though WASM is configured as `pre-proxy` **after** rate-limit has run? — yes, because ordering depends on priority within the same phase — see also WASM-002). More broadly: there is no namespace/prefix enforcement preventing WASM plugins from reading `billing.cost`, `auth.consumer_id`, or other privileged keys.
- **Exploit scenario:**
  1. A route has `rate-limit` (priority 40) and a later WASM plugin "analytics" (priority 100) both in `pre-proxy`. The WASM plugin runs *after* rate-limit on the current request, but because Metadata persists across plugins, the WASM plugin can set `skip_rate_limit=true`. Rate-limit on **the next request** (if the same context is reused — it is not, but plugins that cache state keyed on metadata keys may be fooled) or on any composite-scope downstream check sees the skip flag.
  2. A WASM plugin reads `Metadata["billing.credits_remaining"]` set by an earlier billing pre-check and exfiltrates it via a response header or custom error message, leaking account balances to end users.
- **Remediation:**
  1. Namespace-scope the WASM-visible metadata: only copy keys with a `wasm.` prefix or an explicitly configured `exposed_metadata_keys` allow-list.
  2. Reject WASM-returned keys that would overwrite reserved namespaces (`auth.*`, `billing.*`, `rate_limit.*`, `skip_*`).
  3. Document that `PipelineContext.Metadata` is a trusted shared bus and must not be blindly exposed to guest code.

---

### WASM-008: WASM plugins are not registered through the native plugin factory system — no config validation, no enable/disable flag, no priority clamping

- **Severity:** Medium
- **Confidence:** 85
- **CWE:** CWE-306 (Missing Authentication for Critical Function), CWE-1188 (Insecure Default Initialization)
- **File:** `internal/plugin/registry.go:150-176`, `internal/plugin/wasm.go:574-589`
- **Description:**
  `NewDefaultRegistry()` lists 24 built-in plugin factories (`cors`, `auth-apikey`, etc.) but **no `wasm` factory**. The only way WASM modules enter a pipeline is through `WASMPluginManager.CreatePipelinePlugin(moduleID)`, which is **not called from `BuildRoutePipelinesWithContext`**. That means WASM plugins are wired in through a completely separate code path with different guarantees:

  - No `isPluginEnabled(spec)` check — the `Enabled *bool` field is unused.
  - No consumer-context injection for auth modules.
  - No factory-level phase validation (reinforces WASM-001).
  - No priority bounds check — a module declaring `priority: -1000` will sort before every native plugin, including auth (in the sort comparator `registry.go:226-228`).
  - The plugin `name` used in the chain (`fmt.Sprintf("wasm-%s", module.Name())` at `wasm.go:582`) is not subject to the `normalizePluginName` pipeline that merges global/route plugin configs, so two WASM modules with the same declared name collide without error.
- **Exploit scenario:**
  A defender adds `- name: auth-apikey` globally. A WASM plugin named `auth-apikey` (declared priority -1000, phase auth) is also loaded. Because WASM bypasses `mergePluginSpecs` and naming collisions are not detected, both run — the WASM one first — and the WASM module's `handle_request` returning `{"handled": true}` aborts the pipeline **before** the native `auth-apikey` authenticates anything. A 200 OK is returned to the client without any authentication check.
- **Remediation:**
  1. Register a `wasm` factory in `NewDefaultRegistry` that takes `module_id`, `phase`, `priority` from plugin config and resolves via the manager — this puts WASM through the same validation path.
  2. Clamp `priority` to a sane range (e.g., 0-1000).
  3. Forbid WASM module names that collide with native plugin names (check against `factories` map).
  4. Honor `PluginConfig.Enabled` for WASM-mapped plugins.

---

### WASM-009: `WASMConfig.AllowedPaths` and `EnvVars` are configured but not wired — a future refactor that uses them would silently expose host secrets

- **Severity:** Low (currently), but a **latent High** bug
- **Confidence:** 95
- **CWE:** CWE-1068 (Inconsistency Between Implementation and Documented Design)
- **File:** `internal/plugin/wasm.go:33-36`, `wasm.go:59-64`
- **Description:**
  The config struct advertises `AllowedPaths map[string]string` (guest→host path mapping for WASI FS) and `EnvVars map[string]string`, but the runtime code never passes either to wazero's `NewModuleConfig()`:
  ```go
  inst, err := r.runtime.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().
      WithName(id).
      WithStartFunctions("_start"))
  ```
  The comment at line 59-64 acknowledges this and warns future maintainers not to use `WithEnvVars` without an allow-list — good hygiene, but the field being present in `WASMConfig` encourages operators to populate it, and the current absence of validation means a YAML like `env_vars: { API_CEREBRUS_ADMIN_KEY: "sk_live_..." }` loads without error and a maintainer could later call `WithEnvVars(cfg.EnvVars)` thinking the entries are validated. The `Validate()` method does nothing with these fields.
- **Exploit scenario:**
  (Latent) A maintainer completes the FS feature in a future PR and adds `runtime.WithFSConfig(...)`. At that moment, every historical deployment with legacy `env_vars` in config is exposed. Even today, operators reading the example config may set these fields expecting them to work, causing drift between configuration documentation and reality.
- **Remediation:**
  1. Either implement the feature with strict allow-listing (deny any env var starting with `APICERBERUS_`, deny any value longer than 256 chars, deny any `*path*` guest mapping that resolves to sensitive host dirs) **or** remove the fields from the exported struct entirely.
  2. Add `Validate()` checks rejecting unexpected `AllowedPaths`/`EnvVars` while the feature is dormant:
     ```go
     if len(c.AllowedPaths) > 0 { return fmt.Errorf("wasm allowed_paths not yet supported; remove from config") }
     if len(c.EnvVars) > 0     { return fmt.Errorf("wasm env_vars not yet supported; remove from config") }
     ```
  3. Remove these fields from example YAML until wired.

---

### WASM-010: Marketplace `extractAndInstall` silently ignores tar symlink/hardlink entries — cannot escape dir, but allows denial-of-service via deeply-nested tar bombs

- **Severity:** Low
- **Confidence:** 70
- **CWE:** CWE-409 (Improper Handling of Highly Compressed Data)
- **File:** `internal/plugin/marketplace.go:658-699`
- **Description:**
  The extraction loop handles only `tar.TypeDir` and `tar.TypeReg`. Symlinks (`tar.TypeSymlink`, `tar.TypeLink`) are silently skipped — that's safe against zip-slip but means a plugin author can ship a manifest that *expects* symlinks to exist and they won't, causing runtime errors post-install (install succeeds but plugin fails to load). More importantly, the extractedSize cap is `maxExtractSize = mp.config.MaxPluginSize` (default 100MB) — a gzip bomb compressed to 100MB can expand to several GB, and the check happens **after** `io.CopyN(outFile, tarReader, maxExtractSize-extractedSize+1)`, meaning up to `maxExtractSize+1` bytes can be written to disk per entry before the counter trips:
  ```go
  written, err := io.CopyN(outFile, tarReader, maxExtractSize-extractedSize+1)
  extractedSize += written
  if extractedSize > maxExtractSize {
      _ = outFile.Close()
      return fmt.Errorf("extracted plugin exceeds maximum size of %d bytes", maxExtractSize)
  }
  ```
  If the first entry is 100MB+1, it's fully written before the size check fires. The file is not deleted on error — orphan residue accumulates at `DataDir/installed/<id>/`.
- **Exploit scenario:**
  An attacker who can publish to the marketplace (or compromise a trusted signer) ships a `.tar.gz` that decompresses to a single 99MB file followed by a 99MB file. First file passes (`extractedSize=99MB`). Second file's `io.CopyN` limit is `100MB - 99MB + 1 = 1MB+1`, so only 1MB+1 is written — but the orphan 99MB file remains on disk. Repeated install attempts fill the disk.
- **Remediation:**
  1. Before each entry, check `extractedSize + header.Size > maxExtractSize` using the tar header's declared size (not trusted, but a first-pass guard) and skip if so.
  2. On any extraction error, `os.RemoveAll(pluginDir)` to clean up partial state.
  3. Reject `header.Typeflag` values other than `TypeDir`/`TypeReg` with an explicit error (so manifest authors know symlinks are unsupported) — currently they are silently dropped.

---

### WASM-011: Marketplace plugin signatures are verified but the **plugin manifest inside the archive is not** — swap-inside-archive attack possible

- **Severity:** Medium
- **Confidence:** 65
- **CWE:** CWE-345 (Insufficient Verification of Data Authenticity)
- **File:** `internal/plugin/marketplace.go:371-385`
- **Description:**
  The Ed25519 signature is verified over the **downloaded bytes** before extraction (`marketplace.go:377-380`), which is correct. However, the `InstalledPlugin` metadata (`marketplace.go:389-396`) and the on-disk extracted files are not re-validated against the manifest inside the archive. If the archive contains, say, a `plugin.yaml` manifest declaring `phase: pre-proxy` but the operator later hand-edits that file to `phase: auth`, the gateway trusts the on-disk manifest (via `LoadModule`'s `pluginConfig` passed by the caller) with no re-verification.

  Additionally, the signature covers the tar.gz as a whole but not individual files' integrity at load time — an attacker with filesystem access (e.g., container escape from another workload sharing a volume, or an insider with dataDir write access) can modify individual extracted `.wasm` bytes after install. `LoadModule` re-validates only the WASM magic header + size (`wasm.go:137-167`), not a per-file checksum.
- **Exploit scenario:**
  Attacker gains write access to `./plugins/installed/<id>/` (a common misconfiguration: sharing the persistent volume read-write across pods). They replace `plugin.wasm` with a malicious variant that keeps the same size and header but exports a backdoor. On next gateway reload, the malicious code runs — the Ed25519 signature was verified once at install time and never re-checked.
- **Remediation:**
  1. On every `LoadModule`, verify the WASM file against a stored per-file SHA-256 that was computed at install time (from `InstalledPlugin.Checksums`).
  2. Store the extracted file hashes in `metadata.json` and verify on load.
  3. Run the marketplace data dir as `0500` (read+execute for owner, nothing for others) and restrict to the gateway service account.

---

### WASM-012: `ctx.Consumer` pointer is mutable by any plugin — WASM and native — enabling post-auth impersonation

- **Severity:** Medium
- **Confidence:** 80
- **CWE:** CWE-639 (Authorization Bypass Through User-Controlled Key)
- **File:** `internal/plugin/registry.go:19, 323-324`, `internal/gateway/server.go:289`
- **Description:**
  `PipelineContext.Consumer *config.Consumer` is a pointer exposed to every plugin. The native auth-apikey plugin sets it (`registry.go:322-324`):
  ```go
  ctx.Consumer = consumer
  ```
  Any subsequent plugin — WASM or native — can reassign it:
  ```go
  ctx.Consumer = &config.Consumer{ID: "admin", Name: "admin"}
  ```
  The gateway then reads the mutated consumer at `server.go:289` (`rs.consumer = rs.pipelineCtx.Consumer`) and uses it for downstream billing, audit logging, and `routeHasAuth` decisions. WASM plugins don't directly manipulate this pointer in the current code because `WASMContext` only exposes `ConsumerID`/`ConsumerName` strings and `ApplyToContext` does not write them back — **but nothing prevents a future refactor from doing so**, and a malicious *native* plugin factory (e.g., a custom third-party one registered via the marketplace) can do this today.

  Additionally, consumer mutation is not logged or audited, making impersonation hard to detect post-hoc.
- **Exploit scenario:**
  A marketplace plugin "consumer-normalizer" claims to canonicalize consumer names. Its actual implementation overwrites `ctx.Consumer.ID = "root"` for selected API keys. Billing and audit now attribute all requests to "root", which either (a) consumes a different account's credits or (b) masks the real actor for audit.
- **Remediation:**
  1. Mark `PipelineContext.Consumer` as "set once after AuthN". Introduce a sentinel that rejects reassignment:
     ```go
     func (c *PipelineContext) SetConsumer(cons *config.Consumer) error {
         if c.Consumer != nil && c.Consumer != cons { return errors.New("consumer already set") }
         c.Consumer = cons
         return nil
     }
     ```
  2. Emit an audit event whenever `Consumer` changes after the AUTH phase.
  3. For WASM specifically, never apply `WASMContext.ConsumerID` changes to `ctx.Consumer` — document that consumer identity is read-only from WASM.

---

## Positive Findings (Good Practices Observed)

- **WASM magic validation** at `wasm.go:137-167` correctly rejects non-WASM files by reading the `\0asm` magic.
- **Path traversal defense** at `wasm.go:170-190` uses `filepath.Rel` against the module directory, blocking `../` escapes.
- **Hard size cap** at `wasm.go:22` (`maxWASMModuleSize = 100MB`) prevents loading oversized modules even if `MaxMemory` is set higher.
- **WASI is gated behind `AllowFilesystem`** (`wasm.go:107-112`). With the default `AllowFilesystem: false`, WASI functions are unavailable to guests — a genuine sandbox win. Plugins cannot perform filesystem I/O without explicit operator opt-in.
- **`maxWASMReadSize = 64MB` cap** at `wasm.go:401` prevents guest-declared huge lengths from triggering host-side OOM during `mem.Read`.
- **Ed25519 signature verification** at `marketplace.go:601-631` is cryptographically sound: correct key size check, fail-closed on decode errors, no accept-any-signature path.
- **Trusted signer allow-list** in `MarketplaceConfig.TrustedSignerKeys` binds each signature to a named author.
- **Constant-time API key comparison** at `auth_apikey.go:186, 204` uses `crypto/subtle.ConstantTimeCompare`, preventing timing leakage. (Already covered in prior scan.)
- **JWT `none` algorithm explicitly rejected** at `auth_jwt.go:179-181`, stopping the classic confusion attack. (Already covered in prior scan.)
- **JTI replay fail-closed** at `auth_jwt.go:266-275` rejects tokens with `jti` when no replay cache is configured — a pleasantly paranoid default. (Already covered in prior scan.)
- **Correlation-ID non-overwrite** on empty WASM response (`wasm.go:705-707`, tested at `wasm_test.go:453-468`) preserves upstream request-id continuity.
- **Marketplace HTTP client timeouts** at `marketplace.go:233-241` (30s total, 10s TLS, 10s response header) prevent slowloris against the registry.

---

## Summary

- **Critical: 0**
- **High: 4** (WASM-001, WASM-002, WASM-003, WASM-004)
- **Medium: 5** (WASM-005, WASM-006, WASM-007, WASM-008, WASM-011, WASM-012 — *Count: WASM-011 and WASM-012 are Medium; corrected: 5*)
- **Low: 2** (WASM-009, WASM-010)
- **Info: 0**

*Tally by ID:* Critical 0 / High 4 / Medium 6 / Low 2.

**Key takeaway:** The WASM sandbox (wazero) is technically sound, but the **integration layer around it** is the problem. Three compounding weaknesses — unvalidated `phase`, single-loop pipeline execution, and unfiltered header write-back — combine into a full **AuthN bypass primitive** for any actor who can register a WASM module (which includes anyone with plugin-config write access via the admin API). Priority fixes in order:

1. **WASM-002** (enforce phase filtering in `Pipeline.Execute`) — fixes the root contract violation.
2. **WASM-001** (validate and allow-list `phase`) — prevents the simplest exploit.
3. **WASM-004** (header write-back filter) — breaks the privilege-escalation payload.
4. **WASM-008** (route WASM through the native factory system) — aligns guarantees.
5. **WASM-003** (recover from panics) — hardens blast radius.

All twelve findings are pointable at specific `file:line`. None overlap with the prior full-scan report (which rated WASM as "low risk — sandboxed wazero"); these gaps are in the **integration**, not the sandbox runtime.
