# Server-Side Security Findings — SSRF, Path Traversal, File Upload, Open Redirect

**Scan Date:** 2026-04-18
**Scanner:** Claude Code
**Scope:** SSRF, path traversal, file upload, open redirect
**Files Scanned:** `internal/gateway/router.go`, `internal/gateway/optimized_proxy.go`, `internal/admin/webhook.go`, `internal/admin/webhooks.go`, `internal/plugin/wasm.go`, `internal/plugin/marketplace.go`, `internal/audit/retention.go`, `internal/plugin/redirect.go`, `internal/admin/oidc.go`, `internal/admin/oidc_provider.go`, `internal/federation/subgraph.go`, `internal/federation/executor.go`, `internal/gateway/health.go`, `internal/gateway/proxy.go`

---

## Finding SSRF-001: Health Check Previously Bypassed SSRF Gate (Remediated)

| Field | Value |
|-------|-------|
| **CWE** | CWE-918 (SSRF) |
| **CVSS 3.1** | 6.5 Medium (AV:N/AC:H/PR:L/UI:N/S:U/C:H/I:H/A:N) |
| **Evidence** | `internal/gateway/health.go:128-138` |
| **Status** | REMEDIATED — `validateUpstreamHost` call added at `health.go:136` |

**Description:** Active health probes previously called `validateUpstreamHost` only on the main proxy path. An admin-lite actor who could register an upstream target (or influence its DNS) could probe cloud metadata (169.254.169.254) and RFC1918 addresses via the `/admin/api/v1/upstreams/{name}/health` endpoint — the boolean healthy/unhealthy result and observed latency functioned as a reflective SSRF oracle.

**Evidence (post-fix):**
```go
// internal/gateway/health.go:136
if err := validateUpstreamHost(strings.TrimSpace(address)); err != nil {
    return false, 0
}
```

**Remediation:** The fix applies `validateUpstreamHost` to the `address` parameter before the health probe HTTP request is dispatched (commit `dd68aea`).

---

## Finding SSRF-002: Main Proxy SSRF Protection (Good)

| Field | Value |
|-------|-------|
| **CWE** | CWE-918 (SSRF) |
| **CVSS 3.1** | 4.3 Medium (AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:L/A:N) |
| **Evidence** | `internal/gateway/optimized_proxy.go:465-468` |
| **Status** | GOOD — defenses present |

**Description:** `buildUpstreamURL` calls `validateUpstreamHost` on the upstream host before constructing the proxy URL. This blocks link-local/metadata IPs (169.254.x.x), unspecified addresses (0.0.0.0/::), and private/loopback ranges when `denyPrivateUpstreams` is enabled.

```go
// internal/gateway/optimized_proxy.go:465-468
if err := validateUpstreamHost(base.Host); err != nil {
    return nil, err
}
```

**Remediation:** No action needed. Ensure `denyPrivateUpstreams: true` is set in production config.

---

## Finding SSRF-003: Webhook URL SSRF Protection (Good)

| Field | Value |
|-------|-------|
| **CWE** | CWE-918 (SSRF) |
| **CVSS 3.1** | 4.3 Medium (AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:L/A:N) |
| **Evidence** | `internal/admin/webhooks.go:711-741` |
| **Status** | GOOD |

**Description:** `validateWebhookURL` rejects loopback, link-local/metadata, unspecified, and multicast addresses at webhook registration time. HTTP is allowed for development; only HTTPS for production.

```go
// internal/admin/webhooks.go:711-741
func validateWebhookURL(rawURL string) error {
    // ... rejects loopback, 169.254.x.x, 0.0.0.0, multicast
}
```

**Remediation:** No action needed.

---

## Finding SSRF-004: Federation Subgraph SSRF Protection (Good)

| Field | Value |
|-------|-------|
| **CWE** | CWE-918 (SSRF) |
| **CVSS 3.1** | 4.3 Medium (AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:L/A:N) |
| **Evidence** | `internal/federation/subgraph.go:443-521`, `internal/federation/executor.go:439-444` |
| **Status** | GOOD |

**Description:** `validateSubgraphURL` and `validateSubgraphIP` enforce SSRF blocks for subgraph registration, including explicit IPv6 link-local/unique-local checks and IPv4-mapped-v6 unwrapping. Additionally, `Executor.executeStep` re-validates the subgraph URL before each HTTP dial, closing a cached-plan SSRF window.

```go
// internal/federation/executor.go:439-444
if e.validateURLs {
    if err := validateSubgraphURL(step.Subgraph.URL); err != nil {
        return nil, fmt.Errorf("subgraph URL validation failed: %w", err)
    }
}
```

**Remediation:** No action needed.

---

## Finding REDIR-001: Redirect Plugin Accepts Arbitrary Target URL

| Field | Value |
|-------|-------|
| **CWE** | CWE-601 (URL Redirect to Untrusted Site) |
| **CVSS 3.1** | 5.3 Medium (AV:N/AC:L/PR:N/UI:R/S:U/C:N/I:L/A:N) |
| **Evidence** | `internal/plugin/redirect.go:50-65` |
| **Status** | OPEN |

**Description:** The redirect plugin (`Redirect.Handle`) matches requests by path only, then issues `http.Redirect` to `rule.TargetURL` without validating the target URL scheme or hostname. An attacker who can configure a redirect rule (via admin API) can set `TargetURL` to an arbitrary URI including `//evil.com` (which yields a redirect to `evil.com` due to how `http.Redirect` parses the URL) or `javascript:alert(1)` (which browsers will not follow for GET, but is a risk signal).

**Evidence:**
```go
// internal/plugin/redirect.go:50-65
func (r *Redirect) Handle(w http.ResponseWriter, req *http.Request) bool {
    for _, rule := range r.rules {
        if req.URL.Path != rule.Path {
            continue
        }
        // No validation of rule.TargetURL before redirect
        http.Redirect(w, req, rule.TargetURL, rule.StatusCode)
        return true
    }
    return false
}
```

**Remediation:** Add URL validation to `NewRedirect` or `Handle`:
```go
func isAllowedRedirect(target string) bool {
    u, err := url.Parse(target)
    if err != nil { return false }
    // Only allow absolute HTTPS URLs or absolute paths
    if u.Scheme != "" && u.Scheme != "https" && u.Scheme != "http" {
        return false
    }
    if u.Scheme == "http" {
        // Warn or reject http in production
    }
    return true
}
```
Reject schemes such as `javascript:`, `data:`, `file:`.

---

## Finding REDIR-002: OIDC Logout post_logout_redirect_uri Passed to IdP

| Field | Value |
|-------|-------|
| **CWE** | CWE-601 (URL Redirect to Untrusted Site) |
| **CVSS 3.1** | 4.7 Medium (AV:N/AC:H/PR:H/UI:N/S:U/C:N/I:L/A:N) |
| **Evidence** | `internal/admin/oidc.go:406-410` |
| **Status** | OPEN |

**Description:** When an OIDC provider returns an `end_session_endpoint`, the gateway constructs the logout redirect URL by appending `post_logout_redirect_uri` parameter from the `redirectURL` variable (which originates from the `/admin/api/v1/auth/sso/logout` query parameter `redirect_url`):

```go
// internal/admin/oidc.go:406-410
logoutURL := disc.EndSessionEndpoint +
    "?post_logout_redirect_uri=" + redirectURL +
    "&client_id=" + cfg.OIDC.ClientID
http.Redirect(w, r, logoutURL, http.StatusFound)
```

If a user can control the `redirect_url` query parameter, they can cause the IdP to redirect to an arbitrary URL after logout (IdP-initiated logout open redirect). However, the `redirectURL` default is `/dashboard` (line 365) and is only set from user input when the query parameter is provided. Operator-controlled OIDC configuration (ClientID, Secret) is required to configure an IdP, limiting exploitability.

**Remediation:** Maintain an allow-list of permitted post-logout URIs in the OIDC config, or hard-code the post-logout redirect to `/dashboard`.

---

## Finding REDIR-003: OIDC Authorization redirect_uri Validated Against Client Registry (Good)

| Field | Value |
|-------|-------|
| **CWE** | CWE-601 (URL Redirect to Untrusted Site) |
| **Evidence** | `internal/admin/oidc_provider.go:276-278` |
| **Status** | GOOD |

**Description:** The OIDC authorization endpoint validates `redirect_uri` against the registered `RedirectURIs` for the client before issuing an authorization code. Untrusted redirect URIs are rejected with HTTP 400.

```go
// internal/admin/oidc_provider.go:276-278
if !slices.Contains(client.RedirectURIs, redirectURI) {
    writeError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uri not registered for this client")
    return
}
```

**Remediation:** No action needed.

---

## Finding PT-001: WASM Module Path Traversal Prevention (Good)

| Field | Value |
|-------|-------|
| **CWE** | CWE-22 (Path Traversal) |
| **Evidence** | `internal/plugin/wasm.go:170-190` |
| **Status** | GOOD |

**Description:** `safeResolvePath` uses `filepath.Rel` against `ModuleDir` to ensure resolved paths remain within the plugin module directory. The `../` prefix check via `strings.HasPrefix(rel, "..")` correctly blocks path traversal attempts.

```go
// internal/plugin/wasm.go:185-189
rel, err := filepath.Rel(moduleDir, path)
if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
    return "", fmt.Errorf("wasm module path %q is outside module dir %q", path, moduleDir)
}
```

**Remediation:** No action needed.

---

## Finding PT-002: Marketplace Plugin Extraction Path Traversal (Good)

| Field | Value |
|-------|-------|
| **CWE** | CWE-22 (Path Traversal) |
| **Evidence** | `internal/plugin/marketplace.go:668-672` |
| **Status** | GOOD |

**Description:** The `extractAndInstall` function validates each tar header name via `filepath.Clean` + `filepath.Rel` checks before extraction, preventing files from being written outside the plugin directory.

```go
// internal/plugin/marketplace.go:668-672
targetPath := filepath.Join(pluginDir, filepath.Clean("/"+header.Name))
rel, err := filepath.Rel(pluginDir, targetPath)
if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
    return fmt.Errorf("invalid path in archive: %s", header.Name)
}
```

A test case `TestMarketplace_ExtractAndInstall_PathTraversal` (marketplace_test.go:1080) confirms this defense.

**Remediation:** No action needed.

---

## Finding PT-003: Audit Archive Directory Traversal (Suppressed)

| Field | Value |
|-------|-------|
| **CWE** | CWE-22 (Path Traversal) |
| **Evidence** | `internal/audit/retention.go:192` |
| **Status** | SUPPRESSED — #nosec G304 |

**Description:** `archiveFilePath` constructs the archive file path using administrator-configured `archiveDir`. The `#nosec G304` annotation at line 196 acknowledges the static analysis finding that `os.MkdirAll` and `os.OpenFile` on `filepath.Dir(path)` could theoretically be vulnerable to path traversal if `archiveDir` were user-controlled. However, `archiveDir` is operator-only configuration (loaded at startup from config file), not from request input.

```go
// internal/audit/retention.go:196
// #nosec G304 -- path is within the administrator-configured audit archive directory.
file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
```

**Remediation:** No action needed. Verify `archiveDir` is never populated from untrusted input.

---

## Finding PT-004: Router Path Traversal Prevention (Good)

| Field | Value |
|-------|-------|
| **CWE** | CWE-22 (Path Traversal) |
| **Evidence** | `internal/gateway/router.go:113-141` |
| **Status** | GOOD |

**Description:** Router `Match` rejects:
- Paths longer than 8192 bytes (`maxPathLength`)
- Paths with more than 256 segments (`maxPathSegments`)
- Paths containing null bytes (`\x00`)
- Regex patterns longer than 1024 characters

```go
// internal/gateway/router.go:133-141
if len(path) > maxPathLength {
    return nil, nil, ErrNoRouteMatched
}
if strings.ContainsRune(path, '\x00') {
    return nil, nil, ErrNoRouteMatched
}
if n := strings.Count(path, "/"); n > maxPathSegments {
    return nil, nil, ErrNoRouteMatched
}
```

**Remediation:** No action needed.

---

## Finding FILE-001: WASM Marketplace Download Limit (Good)

| Field | Value |
|-------|-------|
| **CWE** | CWE-400 (Uncontrolled Resource Consumption) |
| **Evidence** | `internal/plugin/marketplace.go:584-589` |
| **Status** | GOOD |

**Description:** Plugin download uses `io.LimitReader` to cap body reads at `MaxPluginSize` (default 100MB). Content-Length header is also checked before reading.

```go
// internal/plugin/marketplace.go:584-589
if resp.ContentLength > mp.config.MaxPluginSize {
    return nil, "", fmt.Errorf("plugin exceeds maximum size")
}
data, err := io.ReadAll(io.LimitReader(resp.Body, mp.config.MaxPluginSize))
```

**Remediation:** No action needed.

---

## Finding FILE-002: WASM Memory Read Size Cap (Good)

| Field | Value |
|-------|-------|
| **CWE** | CWE-400 (Uncontrolled Resource Consumption) |
| **Evidence** | `internal/plugin/wasm.go:448-453` |
| **Status** | GOOD |

**Description:** `readFromWASMMemory` enforces `maxWASMReadSize` of 64MB per read to prevent a malicious module from claiming a huge length to cause OOM during buffer allocation (M-016).

```go
// internal/plugin/wasm.go:448-453
const maxWASMReadSize = 64 * 1024 * 1024
if length > maxWASMReadSize {
    return nil, fmt.Errorf("wasm memory read exceeds maximum size %d bytes", maxWASMReadSize)
}
```

**Remediation:** No action needed.

---

## Summary Table

| ID | Category | CWE | CVSS | Severity | Status | Evidence |
|----|----------|-----|------|----------|--------|----------|
| SSRF-001 | SSRF | CWE-918 | 6.5 | Medium | REMEDIATED | health.go:136 |
| SSRF-002 | SSRF | CWE-918 | 4.3 | Medium | GOOD | optimized_proxy.go:465 |
| SSRF-003 | SSRF | CWE-918 | 4.3 | Medium | GOOD | webhooks.go:711 |
| SSRF-004 | SSRF | CWE-918 | 4.3 | Medium | GOOD | subgraph.go:443, executor.go:439 |
| REDIR-001 | Open Redirect | CWE-601 | 5.3 | Medium | OPEN | redirect.go:61 |
| REDIR-002 | Open Redirect | CWE-601 | 4.7 | Medium | OPEN | oidc.go:406-410 |
| REDIR-003 | Open Redirect | CWE-601 | N/A | Info | GOOD | oidc_provider.go:276 |
| PT-001 | Path Traversal | CWE-22 | N/A | Info | GOOD | wasm.go:185-189 |
| PT-002 | Path Traversal | CWE-22 | N/A | Info | GOOD | marketplace.go:668 |
| PT-003 | Path Traversal | CWE-22 | N/A | Info | SUPPRESSED | retention.go:196 |
| PT-004 | Path Traversal | CWE-22 | N/A | Info | GOOD | router.go:133-141 |
| FILE-001 | File Upload | CWE-400 | N/A | Info | GOOD | marketplace.go:584 |
| FILE-002 | File Upload | CWE-400 | N/A | Info | GOOD | wasm.go:448 |

---

## Open Items Requiring Remediation

1. **REDIR-001 (Medium):** `Redirect.Handle` should validate `rule.TargetURL` — reject non-HTTP(S) schemes, warn on HTTP, and optionally enforce allow-list of domains.
2. **REDIR-002 (Medium):** OIDC logout should either maintain an allow-list of post-logout URIs or hard-code the redirect to `/dashboard` rather than reflecting arbitrary user-supplied values to the IdP.
