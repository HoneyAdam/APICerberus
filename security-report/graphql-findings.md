# GraphQL Security Assessment Report

**Date:** 2026-04-18
**Scope:** `internal/graphql/` and `internal/federation/`
**Reviewer:** Automated Security Scan

---

## Executive Summary

The APICerebrus GraphQL implementation has several security controls in place, but **multiple significant gaps** were identified that require attention before production deployment.

| Category | Status |
|----------|--------|
| Introspection Control | PARTIAL - Configurable but not enforced at gateway level |
| Query Depth Limiting | IMPLEMENTED |
| Query Complexity Analysis | IMPLEMENTED |
| Batch Query Protection | MISSING |
| Field Cost Analysis | PARTIAL - Only in GraphQLGuard plugin |
| Resolver Authorization | PARTIAL - Federation only |
| Subscription Security | PARTIAL - Origin checking but no auth |

---

## Finding SEC-GQL-001: Introspection Enabled by Default in Admin GraphQL

**Severity:** MEDIUM
**File:** `internal/admin/graphql.go` (lines 59-71)
**Config:** `internal/config/types.go` (line 193)

### Description

The Admin GraphQL API (`/admin/graphql`) has introspection controlled by the `admin.graphql_introspection` config option, which **defaults to `false`** (Go bool zero value). However:

1. The Admin API introspection check happens **after** query execution (line 51 executes first)
2. The check is **not enforced at the gateway/plugin level** - only in the admin GraphQL handler
3. The example config shows the option commented out with a recommendation to disable

### Code Reference

```go
// internal/admin/graphql.go:51-71
result := graphql.Do(graphql.Params{
    Schema: h.schema,
    RequestString: req.Query,
    ...
})

// F-012: Block introspection queries when disabled (default).
h.server.mu.RLock()
introspectionEnabled := h.server.cfg.Admin.GraphQLIntrospection
h.server.mu.RUnlock()
if !introspectionEnabled && isIntrospectionQuery(req.Query) {
    // Returns error but query already executed above
    ...
}
```

### Issue

The query is executed via `graphql.Do()` **before** the introspection check. This means:
- The query executes and potentially returns data
- Only the introspection result is then blocked
- For mutations, this could cause unintended side effects

### Recommendation

1. Parse and check for introspection **before** calling `graphql.Do()`
2. Add `graphql_introspection` check to the gateway-level GraphQLGuard plugin
3. Enable introspection validation at the plugin pipeline level for all GraphQL endpoints

---

## Finding SEC-GQL-002: No Batch Query Limits

**Severity:** HIGH
**File:** `internal/federation/executor.go` (lines 653-732)

### Description

The `ExecuteBatch` function allows executing multiple queries in a single request with **no limit on batch size**:

```go
// internal/federation/executor.go:665
func (e *Executor) ExecuteBatch(ctx context.Context, subgraph *Subgraph, batch *BatchRequest) (*BatchResponse, error) {
    // NO VALIDATION OF len(batch.Queries)
    response := &BatchResponse{
        Results: make([]map[string]any, 0, len(batch.Queries)),
        ...
    }
}
```

### Attack Scenario

An attacker can send a batch request with thousands of queries, causing:
- Resource exhaustion on subgraph servers
- Database connection pool exhaustion
- Denial of Service

### Recommendation

Implement batch size limits:
```go
const MaxBatchSize = 10 // or configurable

if len(batch.Queries) > MaxBatchSize {
    return nil, fmt.Errorf("batch size %d exceeds maximum %d", len(batch.Queries), MaxBatchSize)
}
```

---

## Finding SEC-GQL-003: Admin GraphQL Has No Query Depth/Complexity Limits

**Severity:** MEDIUM
**File:** `internal/admin/graphql.go`

### Description

The Admin GraphQL handler (`/admin/graphql`) does **not** use the `GraphQLGuard` plugin or any depth/complexity limiting. The admin API processes arbitrary GraphQL queries against gateway configuration with no resource limits.

### Attack Scenario

A compromised admin API key could be used to execute deeply nested queries that cause:
- Excessive CPU usage during query planning
- Memory exhaustion
- Extended response times affecting availability

### Recommendation

Apply the same `GraphQLGuard` protection to admin GraphQL endpoints as to gateway GraphQL.

---

## Finding SEC-GQL-004: No Subscription Authentication

**Severity:** HIGH
**Files:** `internal/graphql/subscription.go`, `internal/graphql/subscription_origin.go`

### Description

The WebSocket subscription transport (`graphql-transport-ws` protocol) has **no authentication mechanism**:

1. `connection_init` message accepts arbitrary payload but no token validation
2. Origin checking (SEC-GQL-007) only validates browser origin, not user identity
3. Subscriptions can be started without verifying API key or admin credentials

```go
// internal/graphql/subscription.go:816-823
initMsg := map[string]any{
    "type": "connection_init",
    // No auth token field validated
}
```

### Recommendation

Implement subscription authentication:
1. Require `Authorization` header or `token` in `connection_init` payload
2. Validate tokens before accepting subscription
3. Add subscription rate limiting per connection

---

## Finding SEC-GQL-005: Fragment Spread Complexity Not Fully Calculated

**Severity:** LOW
**File:** `internal/graphql/analyzer.go` (lines 146-148)

### Description

Fragment spreads are assigned a flat cost of `a.defaultCost`:

```go
case *FragmentSpread:
    // Fragment spreads are resolved elsewhere
    return a.defaultCost
```

This underestimates complexity when:
- Fragments contain expensive nested selections
- The same fragment is used multiple times via `...FragmentName`

### Recommendation

Expand the analyzer to:
1. Resolve and inline fragment contents for accurate complexity
2. Count fragment spread occurrences in cost calculation

---

## Finding SEC-GQL-006: Federation Resolver Auth Is Opt-In

**Severity:** MEDIUM
**File:** `internal/federation/executor.go` (lines 403-415)

### Description

The `@authorized` directive enforcement in federation relies on `WithAuthChecker` being called:

```go
// internal/federation/executor.go:411-415
if checker := authCheckerFromContext(ctx); checker != nil {
    if err := enforceFieldAuth(step, checker); err != nil {
        return nil, err
    }
}
```

If `WithAuthChecker` is **not** called, authorization checks are **completely bypassed** for federated queries. The test confirms this permissive behavior:

```go
// internal/federation/auth_enforcement_test.go:115-144
// TestEnforceFieldAuth_NoCheckerInContextIsPermissive
```

### Recommendation

1. Consider making auth checking the **default** rather than opt-in
2. Add a configuration flag to enforce auth on all federated queries
3. Add warning logs when auth checks are skipped

---

## Finding SEC-GQL-007: Subscription Origin Check Has Compatibility Mode

**Severity:** LOW
**File:** `internal/graphql/subscription_origin.go` (lines 39-43)

### Description

When `allowed_origins` is empty, the origin check enters "compat mode" accepting any origin:

```go
// internal/graphql/subscription_origin.go:41-43
if len(allowed) == 0 {
    return true  // Accepts any origin!
}
```

This is documented but could expose subscriptions if operators forget to configure origins.

### Recommendation

1. Log a warning at startup if subscription origins are not configured
2. Consider requiring explicit opt-in via a `subscription_strict_origins` config flag

---

## Positive Security Controls

The following security features are well-implemented:

| Feature | Location | Notes |
|---------|----------|-------|
| Query depth limiting | `internal/graphql/analyzer.go` | Default max 15, configurable |
| Query complexity analysis | `internal/graphql/analyzer.go` | Default max 1000, configurable |
| Field cost tracking | `internal/plugin/graphql_guard.go` | Custom costs per field |
| GraphQLGuard plugin | `internal/plugin/graphql_guard.go` | Integrates with plugin pipeline |
| WebSocket frame size limit | `internal/graphql/subscription.go:332` | 1 MB max frame |
| JSON depth validation | `internal/graphql/request.go:135-167` | 32-level nesting limit |
| APQ cache with TTL | `internal/graphql/apq.go` | Prevents APQ flooding |
| Subgraph URL revalidation | `internal/federation/executor.go:440-444` | Prevents SSRF via DNS rebinding |
| Auth enforcement in federation | `internal/federation/executor.go:405-415` | SEC-GQL-006 implementation |
| API key redaction | `internal/admin/graphql.go:400-408` | Shows only prefix + suffix |

---

## Recommended Priority Fixes

1. **SEC-GQL-002** - Add batch query size limits (High)
2. **SEC-GQL-004** - Add subscription authentication (High)
3. **SEC-GQL-001** - Check introspection before query execution (Medium)
4. **SEC-GQL-003** - Apply GraphQLGuard to admin GraphQL (Medium)
5. **SEC-GQL-006** - Make federation auth default, not opt-in (Medium)

---

## Test Coverage

Security-relevant test files reviewed:
- `internal/plugin/graphql_guard_test.go` - Guard plugin tests
- `internal/graphql/analyzer_test.go` - Depth/complexity analysis tests
- `internal/federation/auth_enforcement_test.go` - Auth enforcement tests
- `internal/graphql/subscription_origin_test.go` - Origin validation tests

Tests are comprehensive for covered scenarios but gaps exist in:
- Batch query boundary conditions
- Subscription authentication bypass attempts
- Introspection query blocking under load
