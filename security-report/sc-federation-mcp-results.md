# GraphQL Federation + MCP Audit
**Scope:** internal/federation/**, internal/graphql/**, internal/mcp/**
**Date:** 2026-04-16
**Auditor:** Targeted deep-dive (follow-up to SECURITY-REPORT.md + verified-findings.md)

Only findings not already tracked in `verified-findings.md` / `SECURITY-REPORT.md` are listed. Existing high-confidence items (introspection, executor SSRF re-validation, query plan cache key) are referenced but not re-reported.

---

## Findings

### GQL-001: Federation `/graphql/batch` endpoint bypasses the entire plugin pipeline (auth, rate-limit, GraphQLGuard)
- **Severity:** Critical
- **Confidence:** High
- **CWE:** CWE-862 Missing Authorization / CWE-770 Allocation of Resources Without Limits
- **File:line:** `internal/gateway/server.go:243-247`

```go
// GraphQL Federation batch endpoint (bypass routing).
if g.federationEnabled && r.URL.Path == "/graphql/batch" {
    g.serveFederationBatch(w, r)
    return
}
```

The batch endpoint is matched **before** `newRequestState()`, route matching, the plugin pipeline (line 283 `rs.pipeline.Execute`), the auth chain (line 305 `executeAuthChain`), and billing (line 310). `serveFederationBatch` itself only enforces `maxBatchSize = 100` (line 1146-1150); it does not call `GraphQLGuard`, does not require an API key, and does not credit-check.

**Exploit scenario:**
Unauthenticated attacker submits `POST /graphql/batch` with 100 maximally expensive queries (see GQL-003 alias/fragment expansion). Each query fans out to all configured subgraphs with no auth, rate limit, or billing. One HTTP request triggers hundreds or thousands of upstream subgraph requests.

**Remediation:**
1. Move the batch-endpoint branch *below* auth + pre-proxy pipeline, or invoke the pipeline explicitly inside `serveFederationBatch` before planning.
2. Enforce `GraphQLGuard.Handle` on each element of the batch.
3. Gate the batch endpoint on a dedicated "federation" route with its own ACL.

---

### GQL-002: Federation query endpoint performs zero depth/complexity analysis — `GraphQLGuard` is not invoked
- **Severity:** High
- **Confidence:** High
- **CWE:** CWE-400 Uncontrolled Resource Consumption / CWE-1333 Inefficient Regular Expression Complexity (analog for query expansion)
- **File:line:** `internal/federation/planner.go:42-69`, `internal/gateway/server.go:1075-1123`

The federation handler (`serveFederation`) calls `planner.Plan(query, variables)` directly. `Planner.Plan` → `ParseGraphQLQuery` → `graphql.ParseQuery` whose only safeguard is a parse-time depth ceiling of 50 (`parser.go:184`). There is **no field-count limit, no complexity limit, no alias limit, no fragment expansion check**. The `QueryAnalyzer` in `internal/graphql/analyzer.go` exists but is only wired into the opt-in `GraphQLGuard` plugin (`plugin/graphql_guard.go`), which must be attached to a route by the operator and runs in `PhasePreAuth`. For GraphQL-Protocol services the analyzer is only executed if a user explicitly configures `graphql_guard` on the route — and the batch endpoint (GQL-001) bypasses all plugins anyway.

**Exploit scenario:**
A 49-deep query with 500 wide fields at each level parses cleanly (depth 49 < 50) and has unbounded execution cost. `Planner.planField` at `planner.go:88-145` recursively creates a `PlanStep` per nested field — each step is a fresh HTTP request to a subgraph.

**Remediation:**
1. Call `graphql.NewQueryAnalyzer(...)`.Analyze(query) at the top of `serveFederation` and `serveFederationBatch` before planning; reject queries over configured thresholds.
2. Make GraphQLGuard a mandatory default plugin for `service.Protocol == "graphql"` routes.
3. Add per-query step-count cap in `Planner.Plan` (e.g. refuse plans with >N steps).

---

### GQL-003: Alias and fragment-spread complexity bypass in `QueryAnalyzer`
- **Severity:** High
- **Confidence:** High
- **CWE:** CWE-400 Uncontrolled Resource Consumption
- **File:line:** `internal/graphql/analyzer.go:146-148` and `internal/graphql/parser.go:115-124`

```go
case *FragmentSpread:
    // Fragment spreads are resolved elsewhere
    return a.defaultCost
```

`FragmentSpread.Depth()` returns `1` unconditionally (`parser.go:124`). The complexity visitor counts a spread as `defaultCost` without resolving it against its `FragmentDefinition`. Aliases are parsed (`parser.go:470-478`) but the complexity algorithm treats a field with N aliases the same as a single field — `calculateComplexity` does not multiply by alias count; it simply walks the `Field` tree.

**Exploit scenario — alias bomb:**
```graphql
fragment Big on Query {
  f1: expensiveField { deeply { nested { data } } }
  f2: expensiveField { deeply { nested { data } } }
  ...
  f500: expensiveField { deeply { nested { data } } }
}
query {
  q1: ...Big
  q2: ...Big
  ...
  q20: ...Big
}
```
Analyzer reports complexity ≈ 20 (20 spread defaults). Real cost is 20×500 = 10,000 expensive resolver calls. Depth sees `Depth=1` for each spread instead of the fragment's actual depth.

**Remediation:**
1. Build a fragment map from `Document.Definitions`, then during complexity traversal expand `FragmentSpread` into the referenced `FragmentDefinition.Selections` (with a visited-set to reject cycles).
2. For aliased fields, count each alias as a distinct field (iterate selections — already done — but add a per-field-name-with-alias distinct counter for sibling duplicates to detect abuse).
3. Reject queries where the same field name appears more than K times at the same selection set level.

---

### GQL-004: Unresolved fragment spreads cause silent under-planning (and opens information-disclosure oracle)
- **Severity:** Medium
- **Confidence:** High
- **CWE:** CWE-697 Incorrect Comparison / CWE-754 Improper Check for Unusual Conditions
- **File:line:** `internal/federation/planner.go:300-302`

```go
// Fragment definitions are ignored for now; they would be
// resolved at a higher level before planning.
```

`convertDocument` only copies `*graphql.Operation` nodes and drops `FragmentDefinition`s. `convertSelections` only handles `*graphql.Field`; `*graphql.FragmentSpread` and `*graphql.InlineFragment` are silently discarded. Combined with `findSubgraphForField` returning an error `"no subgraph can resolve field: X"` which is then echoed back to the client (`server.go:1109`), an attacker can enumerate which fields exist on which subgraph by probing with named queries without fragments.

**Exploit scenario:**
A client that supplies a query using only `...FragmentName` spreads receives *no data* but also no planning error — the plan has zero steps, returning `{"data": {}}`. Conversely, a direct `query { someField }` returns the planning error text that leaks the schema (field name, subgraph mapping).

**Remediation:**
1. Expand fragment spreads during `convertDocument` before planning, or reject queries containing fragments the planner cannot expand.
2. Return a generic `{"errors":[{"message":"planning_failed"}]}` without the field name; log details server-side.

---

### GQL-005: Federation `FetchSchema`, `CheckHealth`, and `ExecuteBatch` skip SSRF URL validation
- **Severity:** High
- **Confidence:** High
- **CWE:** CWE-918 Server-Side Request Forgery
- **File:line:**
  - `internal/federation/subgraph.go:237` (`FetchSchema` → `http.NewRequest` directly)
  - `internal/federation/subgraph.go:336` (`CheckHealth` → `http.NewRequest` directly)
  - `internal/federation/executor.go:616` (`ExecuteBatch` → `http.NewRequestWithContext` directly)
  - `internal/federation/executor.go:715` (`runSubscription` → `websocket.Dial` directly)

Only `SubgraphManager.AddSubgraph` (line 161) and `Executor.executeStep` (line 367) call `validateSubgraphURL`. The schema-fetch path used during introspection, the health-check path invoked periodically, the batch execution path, and the subscription WebSocket dial all skip validation. An attacker (or compromised admin) who can set a subgraph URL that passes the initial `AddSubgraph` check but whose DNS record changes (rebinding) or whose schema endpoint differs from the execution endpoint can pivot to internal services.

Additionally `validateSubgraphURL` itself has two gaps:
- **DNS-failure bypass** (`subgraph.go:440-443`): If `net.LookupHost` returns an error, validation is skipped (`return nil`). A malicious DNS server can return NXDOMAIN at validation time and a private IP at connection time (TOCTOU / DNS rebinding).
- **Incomplete IPv6 coverage** (`subgraph.go:454-473`): `IsPrivate()` covers RFC4193 but IPv6 link-local `fe80::/10` is only rejected by `IsLoopback()` partially; IPv6 unique-local and IPv4-mapped IPv6 (`::ffff:10.0.0.1`) pass through because `To4()` is not checked after unmasking.

**Exploit scenario:**
1. Attacker registers subgraph with hostname `evil.example.com` that resolves to `1.2.3.4` initially (passes validation).
2. Attacker changes DNS to `169.254.169.254` after registration.
3. `FetchSchema` / `CheckHealth` / subscription dial pull AWS IMDS credentials.

**Remediation:**
1. Call `validateSubgraphURL` inside `FetchSchema`, `CheckHealth`, `ExecuteBatch`, and `runSubscription` before the network call.
2. On DNS resolution failure in `validateSubgraphURL`, **deny by default** rather than allow.
3. Use a custom `net.Dialer.Control` hook on the HTTP client transport that re-checks the actual dialed IP against the blocklist (pin-the-IP pattern) — defeats DNS rebinding.
4. Add IPv6 link-local (`fe80::/10`) and IPv4-mapped IPv6 unwrap checks.

---

### GQL-006: `@authorized` directive is composed but never enforced at execution time
- **Severity:** High
- **Confidence:** High
- **CWE:** CWE-285 Improper Authorization
- **File:line:** `internal/federation/executor.go:248-295`, `internal/federation/composer.go:279-307`

`Composer.GetAuthorizedFields()` builds a `type.field → required roles` map from `@authorized` directives, and `ExecutionAuthChecker.CheckFieldAuth` validates roles. However, a repo-wide grep shows `CheckFieldAuth` is invoked **only in tests** (`executor_coverage_test.go`); neither `Executor.Execute` nor `Executor.ExecuteParallel` nor `serveFederation`/`serveFederationBatch` constructs or calls the checker. Field-level authorization declared in the schema is effectively cosmetic.

**Exploit scenario:**
A subgraph declares `type User { email: String @authorized(roles: ["admin"]) }`. A non-admin caller queries `{ user(id: 1) { email } }` and receives the email because the gateway never checks the directive — it just forwards the entity query to the subgraph. If the subgraph trusts the gateway for authorization (common Apollo Federation pattern), the data leaks.

**Remediation:**
1. In `Executor.Execute`, walk each plan step's query AST and call `CheckFieldAuth(resultType, fieldName)` for every requested field; on denial, skip the step and emit a structured error.
2. Propagate the caller's roles from the request context into the executor.

---

### GQL-007: No Origin check, no auth enforcement, no rate limit on GraphQL subscription WebSocket upgrade
- **Severity:** High
- **Confidence:** High
- **CWE:** CWE-346 Origin Validation Error / CWE-1385 Missing Origin Validation in WebSockets
- **File:line:** `internal/graphql/subscription.go:65-111` and `internal/graphql/subscription_sse.go:48-157`

`SubscriptionProxy.HandleSubscription` performs only:
- `isWSUpgrade(r)` header check
- protocol hijack
- blind upgrade reply

No `Origin` header validation, no authentication (the upstream is dialed with no auth token propagation), no rate limit. `SSESubscriptionProxy.HandleSSE` likewise has no auth or Origin check. The gateway's main dispatcher does route subscription traffic through its plugin pipeline for routes where `service.Protocol == "graphql"` and `fedEnabled` — but the subscription hand-off at `serveFederation` is HTTP POST only; the standalone `SubscriptionProxy` (used when `graphql.NewProxy` is instantiated) is reachable without plugin-pipeline protections if any caller wires it directly.

**Exploit scenario (cross-site WebSocket hijacking):**
If a browser user is authenticated to the gateway via cookie, an attacker page at `evil.com` can open `new WebSocket("wss://gateway/graphql", "graphql-transport-ws")`. Since subscription handler accepts the upgrade without checking `Origin`, the attacker's JS can subscribe to live data and exfiltrate it.

**Remediation:**
1. In `HandleSubscription` and `HandleSSE`, enforce an `Origin` allow-list from config (similar to admin WebSocket handler).
2. Propagate authenticated consumer context into the subscription (currently `dialUpstream` at `subscription.go:150-156` sends only `Host` and `Sec-WebSocket-*` headers — no forwarded auth).
3. Add per-connection and per-consumer subscription count ceilings.

---

### GQL-008: Planner error messages leak schema structure (field/subgraph enumeration oracle)
- **Severity:** Medium
- **Confidence:** High
- **CWE:** CWE-209 Generation of Error Message Containing Sensitive Information
- **File:line:** `internal/federation/planner.go:95` returned to client via `internal/gateway/server.go:1107-1111`

```go
if subgraph := p.findSubgraphForField(field.Name); subgraph == nil {
    return nil, fmt.Errorf("no subgraph can resolve field: %s", field.Name)
}
```

`serveFederation` echoes this error to the client as `"query planning failed: no subgraph can resolve field: X"`. By enumerating field names, an unauthenticated caller (given GQL-001) or any authenticated caller can map out which fields exist in the composed supergraph even when introspection is disabled.

**Remediation:**
Replace detailed planner errors with a generic response (`{"errors":[{"message":"planning_failed","extensions":{"code":"PLANNING_FAILED"}}]}`) and log details server-side with a correlation ID.

---

### GQL-009: Planner produces colliding step IDs for duplicate field paths, causing dependency-graph corruption
- **Severity:** Medium
- **Confidence:** Medium
- **CWE:** CWE-691 Insufficient Control Flow Management
- **File:line:** `internal/federation/planner.go:101-102` and `126-127`

```go
ID: fmt.Sprintf("step_%s", strings.Join(currentPath, "_")),
```

Step ID is derived from `currentPath` which is built from `field.Name` (not `field.Alias`, see `planner.go:90`). A query with two identically named siblings (legal via aliases: `a: user { id } b: user { id }`) produces two steps with the same ID. `plan.DependsOn` is a `map[string][]string` keyed by step ID, so dependency metadata collides: the second entry overwrites the first (`planner.go:65`). Parallel executor uses the same ID as completion key (`executor.go:510-520`), meaning the second step's completion unblocks the first's dependents incorrectly.

**Exploit scenario:**
Result-shape manipulation: an attacker can craft a federated query whose aliases intentionally produce dependency collisions to force a step to run before its real prerequisite completes, potentially reading stale or nil data for entity references. Could also cause crashes if `completedSteps` is read before the colliding step populates it.

**Remediation:**
Include alias in step ID generation: `strings.Join(currentPath, "_") + "_" + field.Alias`. Or use a monotonic counter.

---

### GQL-010: MCP `system.config.import` enables arbitrary-file-read and file-content oracle via `path` argument
- **Severity:** High
- **Confidence:** High
- **CWE:** CWE-22 Path Traversal / CWE-532 Insertion of Sensitive Info into Error Message
- **File:line:** `internal/mcp/config_import.go:14-17`, invoked from `internal/mcp/call_tool.go:312-325`

```go
if path := strings.TrimSpace(coerce.AsString(args["path"])); path != "" {
    return config.Load(path)
}
```

`config.Load` calls `os.ReadFile(path)` at `internal/config/load.go:17` with no canonicalization, no chroot, no allow-list. A caller authorised for MCP (X-Admin-Key for SSE transport, or inherently local for stdio) can read **any file** the gateway process can read by supplying `path: "/etc/passwd"` or `path: "C:\Windows\System32\config\SAM"`. On YAML-parse failure, the wrapped error message `"parse config: <yaml error>"` often includes the offending line content — turning this into an oracle for non-YAML file contents. On success the returned `*config.Config` is serialised back in the response (`swapRuntime` path imports it; other paths may not). Even failed imports leak data.

Note: even stdio transport is not safe — the MCP server wraps an in-process admin runtime (`server.go:92-105`) that can apply the imported config via `swapRuntime`, giving a remote MCP-SSE client (or a local process that has stolen the admin key) a way to hot-swap the entire gateway config including upstream URLs, TLS certs paths, and admin credentials.

**Exploit scenario:**
```json
{"jsonrpc":"2.0","method":"tools/call","id":1,
 "params":{"name":"system.config.import","arguments":{"path":"/etc/shadow"}}}
```
Response includes `"admin api ... failed: parse config: yaml: line 1: ...hash_here..."`.

**Remediation:**
1. Remove the `path` argument from `system.config.import`; accept only inline `yaml` or structured `config` arguments.
2. If `path` is kept, restrict it to a configured directory (absolute-path canonicalize + prefix check) and strip error details before returning.
3. Add an explicit audit-log entry for every successful `system.config.import` including actor and source.

---

### GQL-011: MCP `/sse` heartbeat endpoint has no authentication — DoS via connection exhaustion
- **Severity:** Medium
- **Confidence:** High
- **CWE:** CWE-770 Allocation of Resources Without Limits or Throttling
- **File:line:** `internal/mcp/server.go:277-303`

The `GET /sse` handler streams heartbeats every 10 seconds for an unbounded number of concurrent clients. No auth check, no per-IP limit, no max-connection cap. An attacker holds N sockets open and consumes file descriptors plus the per-connection goroutine. `POST /mcp` is properly gated with `X-Admin-Key` (line 263) but `/sse` was missed.

**Remediation:**
1. Add the same `X-Admin-Key` check to `GET /sse`.
2. Apply `http.Server.MaxHeaderBytes` + explicit per-IP connection cap (via `net.Listener` wrapper or a middleware like `golang.org/x/net/netutil.LimitListener`).

---

### GQL-012: MCP tool dispatcher forwards args as query-string/body without per-tool schema validation
- **Severity:** Medium
- **Confidence:** High
- **CWE:** CWE-20 Improper Input Validation / CWE-1287 Improper Validation of Specified Type of Input
- **File:line:** `internal/mcp/tools_definitions.go:4` + `internal/mcp/helpers.go:12-62`

Every tool's `InputSchema` is `{"type":"object","additionalProperties":true}` (`anyObj` at tools_definitions.go:4). The server never actually enforces the declared schema; clients can pass any keys. `queryFromArgs` forwards every string-coerced argument as a URL-encoded query parameter to the admin API, and `payloadFromArgs` forwards arbitrary map nesting as JSON body. The admin API is trusted to re-validate, but:
- `audit.search` with arbitrary keys translates to arbitrary query params on `/admin/api/v1/audit-logs`. Any admin-API filter that accepts user-controlled fields (e.g. path fields, SQL `ORDER BY` column names, LIKE patterns, regex) could be abused. The admin API's own validation becomes the only defence.
- `users.create` / `gateway.routes.create` pass through `payloadFromArgs(args, "user")` — if the request omits the nested `user` key, the *entire* args map (minus ignored keys) is sent as the user-create payload, including attacker-controlled `id`, `password_hash`, `role`, etc.

**Exploit scenario — role escalation via create:**
`tools/call users.create` with `{"email":"x@y","role":"admin","permissions":["*"]}` (no `user` nesting). `payloadFromArgs(args, "user")` at `helpers.go:64-96` sees `args["user"]` is absent and falls through to the "promote flat keys" branch, sending `{"email":"x@y","role":"admin","permissions":["*"]}` directly to `POST /admin/api/v1/users`. If the admin API doesn't strip unknown fields, the user is created with attacker-chosen role.

**Remediation:**
1. Define precise JSON schemas per tool and validate incoming `arguments` with `github.com/xeipuuv/gojsonschema` or equivalent.
2. In `payloadFromArgs`, require the nested key — refuse requests missing it rather than promoting flat keys.
3. Lock `queryFromArgs` to an allow-list of query parameter names per tool.

---

### GQL-013: `runSubscription` has no WebSocket frame read deadline — connection can pin a goroutine indefinitely
- **Severity:** Low
- **Confidence:** Medium
- **CWE:** CWE-400 Uncontrolled Resource Consumption
- **File:line:** `internal/federation/executor.go:713-754`

`ctx` is created with a 30-second timeout at line 713 and the same context is used for all `ws.Read` calls. This means **the entire subscription lifetime is capped at 30 s**, which is probably not the intended semantic (subscriptions should be long-lived). If an upstream sends one message then falls silent, the read blocks until 30 s, then the context fires and the subscription dies — whether the client wanted it or not. No per-read deadline, no heartbeat/ping to the upstream.

**Remediation:**
Replace the 30-second timeout with either `context.Background()` plus a per-read deadline, or propagate a caller-controlled `ctx`. Implement ping/pong heartbeats and idle-timeout.

---

### GQL-014: Subgraph `Headers` are forwarded verbatim in every executor step — secret injection risk
- **Severity:** Low
- **Confidence:** Medium
- **CWE:** CWE-522 Insufficiently Protected Credentials / CWE-93 CRLF Injection
- **File:line:** `internal/federation/executor.go:378-380`

```go
for k, v := range step.Subgraph.Headers {
    req.Header.Set(k, v)
}
```

`Subgraph.Headers` comes from admin-API input (`admin/server.go:576-598`) and is set without validation. `http.Header.Set` does validate for CR/LF, so CRLF injection is mitigated. But the broader issue: there is no allow-list of header names, so an admin (or compromised admin) can set `Authorization: Bearer <leaked-token>` or `Cookie: session=<victim>` that leaks across every query to that subgraph regardless of which user originated it. There's no clear separation between admin-configured headers (stable secrets) and per-request headers (user context) — only the former path exists, so per-user auth cannot be forwarded.

**Remediation:**
1. Provide an allow-list for static subgraph headers (config-level).
2. Add a separate per-request header-forwarding mechanism that propagates the caller's Authorization rather than a static token.

---

## Positive Findings

1. `validateSubgraphURL` (subgraph.go:421-451) does resolve hostnames and walk all resolved IPs — correctly handles multi-A-record SSRF for the AddSubgraph path.
2. `Executor.executeStep` re-validates URL before each HTTP dial (`executor.go:365-370`) — addresses the cached-plan SSRF from the earlier HIGH-NEW-2 finding.
3. MCP SSE `POST /mcp` uses `crypto/subtle.ConstantTimeCompare` for admin-key comparison (`server.go:263, 454-456`) — proper timing-safe auth.
4. GraphQL WebSocket frame parser enforces a 1 MiB max frame size (`subscription.go:280, 312-314`) — prevents OOM on oversized frames (CWE-770).
5. GraphQL HTTP JSON body size capped at 10 MiB (`request.go:102`) with a separate JSON-nesting-depth check (`request.go:120, 137-167`).
6. Parser enforces a default max-depth of 50 at parse time (`parser.go:184-189, 353-359`), preventing stack-overflow via deep nesting.
7. Federation executor uses `LoadOrStore` on `circuitBreakers` (`executor.go:239-245`) — thread-safe circuit-breaker creation (no double-init race).
8. `ListLimitReader(body, 50<<20)` on subgraph responses (`executor.go:392, 629`) — prevents memory exhaustion via malicious upstream.
9. MCP admin-token flow uses session cookie rather than response body (`server.go:358-366`) — avoids token leakage in response body (CWE-319).
10. Composer correctly skips `__`-prefixed introspection types during SDL merge (`composer.go:54-57, 254-255`) — prevents introspection-type pollution of composed schema.

---

## Summary

| ID | Severity | CWE | Area |
|----|----------|-----|------|
| GQL-001 | Critical | 862/770 | `/graphql/batch` bypasses entire plugin pipeline (auth, rate-limit, billing) |
| GQL-002 | High | 400 | Federation path does not run `QueryAnalyzer` — no complexity/field-count limits |
| GQL-003 | High | 400 | FragmentSpread counted as cost=1, depth=1 regardless of target; alias duplication not counted |
| GQL-004 | Medium | 697 | Fragments and inline fragments silently dropped by planner; error messages enumerate schema |
| GQL-005 | High | 918 | `FetchSchema`, `CheckHealth`, `ExecuteBatch`, `runSubscription` bypass `validateSubgraphURL`; DNS-failure allow; IPv6 gaps |
| GQL-006 | High | 285 | `@authorized` directive parsed but `ExecutionAuthChecker.CheckFieldAuth` never invoked during execution |
| GQL-007 | High | 346/1385 | Subscription proxies have no Origin check, no auth, no rate limit (XS-WebSocket-hijacking) |
| GQL-008 | Medium | 209 | Planner error messages leak schema structure to clients |
| GQL-009 | Medium | 691 | Step IDs collide on aliased duplicate fields — dependency graph corruption |
| GQL-010 | High | 22/532 | MCP `system.config.import` → `config.Load(path)` is an arbitrary-file-read and error-oracle |
| GQL-011 | Medium | 770 | MCP `/sse` unauthenticated — persistent-connection DoS |
| GQL-012 | Medium | 20 | Every MCP tool uses `additionalProperties:true`; `payloadFromArgs` accepts flat keys — role escalation via `users.create` |
| GQL-013 | Low | 400 | Subscription context capped at 30 s; no per-read deadline or heartbeat |
| GQL-014 | Low | 522 | Subgraph static `Headers` forwarded to every request — no per-user auth forwarding, no allow-list |

**Top remediation priorities:**
1. **GQL-001 + GQL-002**: Move `/graphql/batch` behind auth + pipeline, and wire `QueryAnalyzer` into both `serveFederation` and `serveFederationBatch` as a hard requirement for GraphQL-protocol services.
2. **GQL-005**: Add `validateSubgraphURL` to every federation network entrypoint; deny-on-DNS-fail; use `net.Dialer.Control` to pin IP.
3. **GQL-006**: Wire the `ExecutionAuthChecker` into `Executor.Execute`/`ExecuteParallel` so that `@authorized` is actually enforced.
4. **GQL-010**: Drop `path` support from `system.config.import`, or bound it to a whitelisted directory.
5. **GQL-007**: Add Origin allow-list and auth propagation to both subscription transports.
