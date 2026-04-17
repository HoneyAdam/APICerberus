# Injection Vulnerability Assessment

**Project:** APICerebrus API Gateway
**Date:** 2026-04-18
**Review Scope:** SQL Injection, NoSQL Injection, GraphQL Injection
**Files Analyzed:**
- `internal/store/*.go` (SQLite repositories)
- `internal/admin/*.go` (Admin API handlers)
- `internal/graphql/**/*.go` (GraphQL parsing and execution)
- `internal/federation/**/*.go` (Federation planner and executor)

---

## Executive Summary

| Category | Status | Risk Level |
|----------|--------|------------|
| SQL Injection | Low Risk | See findings below |
| NoSQL Injection | Not Applicable | SQLite-only codebase |
| GraphQL Injection | Medium Risk | 1 finding requires attention |

**Overall Assessment:** The codebase demonstrates strong security posture with parameterized SQL queries throughout. One medium-risk GraphQL injection finding requires remediation.

---

## SQL Injection Assessment

### Findings: NEGLIGIBLE TO LOW RISK

The store layer uses SQLite with proper parameterized queries throughout. All user-controllable values are passed as query arguments, not concatenated into SQL strings.

#### Positive Security Patterns Observed

**1. Parameterized Queries - Consistent Usage**
```go
// internal/store/user_repo.go:150
row := r.db.QueryRow(`
    SELECT id, email, name, company, password_hash, role, status,
           credit_balance, rate_limits, ip_whitelist, metadata, created_at, updated_at
      FROM users
     WHERE id = ?
`, id)
```
- All SQL queries use `?` placeholders with argument arrays
- No raw string concatenation into SQL statements

**2. LIKE Pattern Handling - Safe**
```go
// internal/store/user_repo.go:194-197
if value := strings.TrimSpace(opts.Search); value != "" {
    where = append(where, "(LOWER(email) LIKE ? OR LOWER(name) LIKE ? OR LOWER(company) LIKE ?)")
    pattern := "%" + strings.ToLower(value) + "%"
    args = append(args, pattern, pattern, pattern)
}
```
- LIKE patterns use parameterized queries
- Values wrapped in `%` are passed as arguments, not concatenated

**3. ORDER BY Column Whitelist - Safe**
```go
// internal/store/user_repo.go:622-635
func normalizeUserSortBy(value string) string {
    switch strings.ToLower(strings.TrimSpace(value)) {
    case "email":
        return "email"
    case "name":
        return "name"
    case "updated_at":
        return "updated_at"
    case "credit_balance":
        return "credit_balance"
    default:
        return "created_at"
    }
}
```
- `ORDER BY` columns are validated against a whitelist
- Only predefined column names are allowed

**4. IN Clause with Placeholders - Safe**
```go
// internal/store/audit_repo.go:185-191
placeholders := strings.TrimSuffix(strings.Repeat("?,", len(normalized)), ",")
query := "DELETE FROM audit_logs WHERE id IN (" + placeholders + ")"
args := make([]any, 0, len(normalized))
for _, id := range normalized {
    args = append(args, id)
}
result, r.db.Exec(query, args...)
```
- Dynamic IN clauses built with placeholder repetition
- All values passed via args slice

#### Minor Observation: Audit Search FTS5 Query Sanitization

```go
// internal/store/audit_search.go:222-265
func sanitizeFTS5Query(input string) string {
    // ... strips FTS5 special characters
    // &, | operators are stripped
    // Rejects purely-operator inputs
}
```

The FTS5 sanitization function exists and strips boolean operators. However:
- It has a 500-character input limit (reasonable)
- It wraps tokens in quotes for phrase matching
- Empty inputs return `""` (empty phrase)

**Recommendation:** Consider adding explicit rejection of queries containing GraphQL resonance characters (`{}`, `()`, `:`) since this search is used alongside GraphQL functionality.

---

## NoSQL Injection Assessment

**Not Applicable.** APICerebrus uses SQLite exclusively. There is no MongoDB, Redis (for document storage), or other NoSQL database in the codebase.

---

## GraphQL Injection Assessment

### FINDING GQL-001: Potential Query Injection in Federation Batch Execution

**CWE:** CWE-79 (Cross-Site Scripting) / CWE-943 (Improper Neutralization of Special Elements in GraphQL Query)
**CVSS 3.1 Estimate:** 5.3 (Medium) - AV:N/AC:L/PR:L/UI:N/S:U/C:N/I:L/A:L

**Location:**
- `internal/federation/executor.go:675-677`
- `internal/federation/planner.go:222`

**Evidence:**

```go
// executor.go:675-677
for i, query := range batch.Queries {
    sb.WriteString(fmt.Sprintf("  batch_%d: %s\n", i, query))
}
```

```go
// planner.go:217-225
for name, value := range field.Args {
    if !first {
        sb.WriteString(", ")
    }
    sb.WriteString(fmt.Sprintf("%s: %v", name, value))  // <-- Direct interpolation
    first = false
}
```

**Analysis:**

1. **In `executor.go`:** The `query` string is directly interpolated into a batch GraphQL request without escaping. If a malicious subgraph returns a crafted response containing GraphQL special characters (`}`, `)`, `\n`), it could break the batch query structure.

2. **In `planner.go`:** The `value` from `field.Args` is interpolated directly into the query string using `%v`. While field names come from the parsed AST (controlled by the planner), the values could be user-supplied through query variables.

**Attack Scenario:**
```graphql
# Attacker-controlled query variable: {"input": "test\n  ...on String { __typename }"}
query($input: String!) {
  user(id: $input) {
    name
  }
}
```
The planner would interpolate this as:
```graphql
{
  user(id: test
  ...on String { __typename }: String) {
    name
  }
}
```

**Existing Mitigations:**
- M-018: Query depth limiting (50 levels) in parser
- Query complexity analysis in `internal/graphql/analyzer.go`
- Introspection can be disabled via `admin.graphql_introspection: false`

**Remediation:**

1. **For `executor.go`:** Escape newlines and special characters in query strings:
```go
func escapeGraphQLString(s string) string {
    // Escape backslashes, quotes, newlines, and control characters
    s = strings.ReplaceAll(s, "\\", "\\\\")
    s = strings.ReplaceAll(s, "\"", "\\\"")
    s = strings.ReplaceAll(s, "\n", "\\n")
    s = strings.ReplaceAll(s, "\r", "\\r")
    s = strings.ReplaceAll(s, "\t", "\\t")
    return s
}

// Then use:
sb.WriteString(fmt.Sprintf("  batch_%d: %s\n", i, escapeGraphQLString(query)))
```

2. **For `planner.go`:** Use JSON encoding for argument values instead of `%v`:
```go
import "encoding/json"

for name, value := range field.Args {
    if !first {
        sb.WriteString(", ")
    }
    // Use JSON encoding to properly escape values
    encoded, _ := json.Marshal(value)
    sb.WriteString(fmt.Sprintf("%s: %s", name, string(encoded)))
    first = false
}
```

---

### FINDING GQL-002: String Interpolation in Federation Field Selection

**CWE:** CWE-943 (Improper Neutralization of Special Elements in GraphQL Query)
**CVSS 3.1 Estimate:** 4.3 (Medium) - AV:N/AC:L/PR:L/UI:N/S:U/C:N/I:L/A:N

**Location:** `internal/federation/planner.go:212-226`

**Evidence:**
```go
// buildFieldSelection builds a selection for a field.
func (p *Planner) buildFieldSelection(field GraphQLField, indent int) string {
    var sb strings.Builder
    prefix := strings.Repeat("  ", indent)

    sb.WriteString(fmt.Sprintf("%s%s", prefix, field.Name))

    // Add arguments if any
    if len(field.Args) > 0 {
        sb.WriteString("(")
        first := true
        for name, value := range field.Args {
            if !first {
                sb.WriteString(", ")
            }
            sb.WriteString(fmt.Sprintf("%s: %v", name, value))  // <-- HERE
            first = false
        }
        sb.WriteString(")")
    }
    // ...
}
```

**Analysis:** Field arguments are interpolated with `%v`. If argument values contain GraphQL special characters (`"`, `(`, `)`, `{`, `}`), they could potentially break the generated query or leak data through error messages.

**Remediation:** Same as GQL-001 item 2 - use JSON encoding for argument values.

---

## GraphQL Introspection Security

**Positive Finding:** Introspection can be disabled:
```go
// internal/admin/graphql.go:59-71
h.server.mu.RLock()
introspectionEnabled := h.server.cfg.Admin.GraphQLIntrospection
h.server.mu.RUnlock()
if !introspectionEnabled && isIntrospectionQuery(req.Query) {
    // Returns error, blocks introspection
}
```

**Status:** SECURE - Introspection is controllable via configuration.

---

## Summary of Recommendations

| ID | Severity | Finding | Remediation |
|----|----------|---------|-------------|
| GQL-001 | Medium | Batch query string interpolation | Add string escaping |
| GQL-002 | Medium | Field argument interpolation | Use JSON encoding |
| - | Low | FTS5 query sanitization | Consider GraphQL char filtering |

---

## Test Coverage for Injection Resistances

The following test files cover injection-related scenarios:
- `internal/store/audit_search_test.go` - FTS5 query sanitization tests
- `internal/graphql/*_test.go` - GraphQL parsing and analysis tests
- `internal/federation/*_test.go` - Federation execution tests

**Recommendation:** Add specific test cases for:
1. GraphQL injection payloads in federation batch queries
2. Newline/special character escaping in batch execution
3. FTS5 query with GraphQL special characters (`{}`, `()`)

---

## Conclusion

APICerebrus demonstrates a strong security posture for injection attacks:

1. **SQL Injection:** Effectively mitigated through consistent use of parameterized queries and whitelisting. No exploitable SQL injection vulnerabilities found.

2. **NoSQL Injection:** Not applicable - SQLite-only codebase.

3. **GraphQL Injection:** Two medium-severity findings (GQL-001, GQL-002) require remediation. The issues involve direct string interpolation when building federation queries. Implementing the recommended JSON encoding and string escaping will fully address these findings.

**Priority Remediation:**
1. Fix GQL-001 and GQL-002 in `internal/federation/`
2. Add injection-specific test cases
3. Consider adding FTS5/GraphQL character filtering

---

*Report generated: 2026-04-18*
*APICerebrus version: Current main branch*
