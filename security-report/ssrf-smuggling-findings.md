# SSRF and Request Smuggling Security Assessment

**Date:** 2026-04-18
**Scope:** `internal/gateway/optimized_proxy.go`, `internal/gateway/proxy.go`, `internal/gateway/router.go`, `internal/gateway/health.go`, `internal/raft/transport.go`, `internal/federation/executor.go`, `internal/federation/subgraph.go`, `internal/grpc/proxy.go`
**Note:** `internal/httpbc/` directory does not exist in this codebase.

---

## Executive Summary

The codebase demonstrates **strong SSRF protection** with defense-in-depth measures. Request smuggling risk is **low** due to Go's HTTP library handling and proper header filtering. No critical findings were identified.

---

## 1. SSRF Protection Analysis

### 1.1 Upstream Proxy Validation (`proxy.go`)

**Finding:** Solid protection with defense-in-depth.

The `buildUpstreamURL` function (proxy.go:283) validates upstream targets via `validateUpstreamHost` (proxy.go:315):

| Check | Status | Notes |
|-------|--------|-------|
| Link-local (169.254.0.0/16) | BLOCKED | Includes cloud metadata IPs |
| Unspecified addresses (0.0.0.0/::) | BLOCKED | |
| Loopback (127.x) | BLOCKED | When `denyPrivateUpstreams=true` |
| Private ranges (10.x, 172.16-31.x, 192.168.x) | BLOCKED | When `denyPrivateUpstreams=true` |
| DNS resolution failures | DENIES | Fails closed (M-009 fix) |
| IPv6 private (fc00::/7) | BLOCKED | Via `ip.IsPrivate()` |
| IPv6 link-local (fe80::/10) | BLOCKED | Via `IsLinkLocalUnicast()` |
| IPv4-mapped IPv6 (::ffff:x.x.x.x) | UNWRAPPED | Prevents bypass via v4-mapped format |

**Code snippet (proxy.go:315-370):**
```go
func validateUpstreamHost(host string) error {
    // ... host parsing ...

    ip := net.ParseIP(h)
    if ip == nil {
        // DNS resolution - now DENIES on failure (M-009 fix)
        addrs, err := net.LookupHost(h)
        if err != nil {
            return fmt.Errorf("upstream hostname %q cannot be resolved: %w", h, err)
        }
        for _, addr := range addrs {
            if err := validateResolvedIP(addr, host); err != nil {
                return err
            }
        }
        return nil
    }

    // Block link-local (169.254.0.0/16)
    if ip4 := ip.To4(); ip4 != nil && ip4[0] == 169 && ip4[1] == 254 {
        return fmt.Errorf("upstream address %q is in link-local/metadata range", host)
    }
    // ... additional checks ...
}
```

### 1.2 Optimized Proxy Validation (`optimized_proxy.go`)

**Finding:** Same strong validation applied.

The `OptimizedProxy.buildUpstreamURL` (optimized_proxy.go:446) calls `validateUpstreamHost` at line 466:
```go
if err := validateUpstreamHost(base.Host); err != nil {
    return nil, err
}
```

### 1.3 Health Check SSRF Fix (`health.go`)

**Finding:** SEC-PROXY-002 vulnerability has been patched.

Active health probes now validate the target before connection (health.go:136):
```go
// SEC-PROXY-002: active health probes previously bypassed the SSRF gate
if err := validateUpstreamHost(strings.TrimSpace(address)); err != nil {
    return false, 0
}
```

### 1.4 WebSocket Proxy Validation (`proxy.go`)

**Finding:** SSRF protection applied to WebSocket upgrades.

`dialUpstreamWebSocket` (proxy.go:404) validates before dial:
```go
func dialUpstreamWebSocket(upstreamURL *url.URL) (net.Conn, error) {
    if err := validateUpstreamHost(upstreamURL.Host); err != nil {
        return nil, err
    }
    // ...
}
```

### 1.5 Federation Subgraph URL Validation (`subgraph.go`)

**Finding:** SEC-GQL-005 DNS-rebinding defense implemented.

The `validateSubgraphURL` function (subgraph.go:455) provides comprehensive protection:

```go
// SEC-GQL-005 hardening:
// - DNS resolution failures now DENY rather than ALLOW
// - IPv4-in-IPv6 ("::ffff:10.0.0.1") unwrapped before checks
// - IPv6 link-local and unique-local covered explicitly
func validateSubgraphURL(rawURL string) error {
    // ... URL parsing ...

    addrs, err := net.LookupHost(host)
    if err != nil {
        // Fails closed - previously allowed with "dialer will fail"
        return fmt.Errorf("subgraph host %q cannot be resolved: %w", host, err)
    }
    // ... validates each resolved IP ...
}
```

**Re-validation points (executor.go):**
- `executeStep` (line 440) - before each subgraph dispatch
- `ExecuteBatch` (line 692) - before batch dispatch
- `runSubscription` (line 788) - before WebSocket connection
- `FetchSchema` (subgraph.go:243) - before introspection
- `CheckHealth` (subgraph.go:353) - before health check

### 1.6 Raft Transport (`raft/transport.go`)

**Finding:** Internal cluster communication - limited SSRF risk.

- Raft HTTP endpoints use `MaxBytesHandler` with 10MB limit (line 85)
- Inter-node addresses are configured by cluster admin, not user-controlled
- No SSRF validation on peer addresses (acceptable for internal traffic)

---

## 2. Request Smuggling Analysis

### 2.1 Header Filtering (`proxy.go`, `optimized_proxy.go`)

**Finding:** Properly mitigated.

Hop-by-hop headers are stripped before proxying (proxy.go:481):
```go
var hopByHopHeaders = map[string]bool{
    "connection":          true,
    "proxy-connection":    true,
    "keep-alive":          true,
    "proxy-authenticate":  true,
    "proxy-authorization": true,
    "te":                  true,
    "trailer":             true,
    "transfer-encoding":   true,
    "upgrade":             true,
}
```

Internal headers are also stripped (proxy.go:495):
```go
var internalHeaderPrefixes = []string{
    "x-amzn-",        // AWS internal
    "x-amz-",         // AWS internal
    "x-goog-",        // GCP internal
    "x-go-grpc-",     // gRPC internal
    "x-forwarded-",   // Already set by gateway
    "x-real-ip",      // Already set by gateway
    "x-envoy-",       // Envoy internal
    "x-cloud-trace-", // Cloud trace headers
}
```

### 2.2 Raft Body Limits (`raft/transport.go`)

**Finding:** Mitigated.

```go
const maxRaftRPCBodySize = 10 << 20 // 10 MB — prevents excessive memory allocation (CWE-770)

mux.Handle("/raft/request-vote", http.MaxBytesHandler(rpcHandler(t.handleRequestVote), maxRaftRPCBodySize))
mux.Handle("/raft/append-entries", http.MaxBytesHandler(rpcHandler(t.handleAppendEntries), maxRaftRPCBodySize))
mux.Handle("/raft/install-snapshot", http.MaxBytesHandler(rpcHandler(t.handleInstallSnapshot), maxRaftRPCBodySize))
```

### 2.3 gRPC Header Handling (`grpc/proxy.go`)

**Finding:** Properly mitigated.

HTTP-specific headers are filtered from gRPC metadata (grpc/proxy.go:336):
```go
func isHTTPHeader(key string) bool {
    httpHeaders := []string{
        "Accept", "Accept-Encoding", "Accept-Language",
        "Connection", "Content-Length", "Content-Type",
        "Host", "Transfer-Encoding", "User-Agent",
        "Upgrade", "Proxy-Authorization", "TE",
    }
    // ...
}
```

---

## 3. Host Header Analysis

### 3.1 PreserveHost Behavior (`proxy.go`)

**Finding:** By design, with security awareness.

When `PreserveHost=true`, the client's Host header is forwarded to upstream (proxy.go:126):
```go
if ctx.Route != nil && ctx.Route.PreserveHost {
    proxyReq.Host = ctx.Request.Host
} else {
    // Uses upstream host from config
    proxyReq.Host = u.Host
}
```

This is intentional for scenarios where upstream servers use virtual hosting. The `X-Forwarded-Host` is also set from the original client Host header.

### 3.2 Trusted Proxy Configuration (`pkg/netutil/clientip.go`)

**Finding:** Secure by default.

- When `trusted_proxies` is empty (default), X-Forwarded-For and X-Real-IP are **ignored**
- Right-to-left XFF parsing skips trusted proxies
- X-Real-IP is validated as a valid IP format before use (M-003 fix)

---

## 4. Potential Concerns

### 4.1 `denyPrivateUpstreams` Race Safety

**Severity:** Low

The `denyPrivateUpstreams` variable (proxy.go:39) is set via `SetDenyPrivateUpstreams` at init time. The codebase documentation notes this is not goroutine-safe for concurrent writes, but in practice it's only written once at startup.

```go
var denyPrivateUpstreams bool

func SetDenyPrivateUpstreams(v bool) { denyPrivateUpstreams = v }
```

### 4.2 DNS Rebinding TOCTOU Window

**Severity:** Low (Mitigated)

Between validation and connection, a DNS record could theoretically change. This is mitigated by:
- Short OIDC provider cache (5 minutes, line 94 in admin/oidc.go)
- Re-validation before each federation subgraph request

---

## 5. Findings Summary

| Category | Severity | Status | Notes |
|----------|----------|--------|-------|
| Upstream URL validation | N/A | PASS | `validateUpstreamHost` blocks private/metadata/link-local |
| Health check SSRF | N/A | PASS | SEC-PROXY-002 patched |
| WebSocket SSRF | N/A | PASS | Validation before dial |
| Federation SSRF | N/A | PASS | SEC-GQL-005 re-validation implemented |
| Hop-by-hop headers | N/A | PASS | Properly stripped |
| Raft body limits | N/A | PASS | MaxBytesHandler applied |
| gRPC smuggling | N/A | PASS | HTTP headers filtered |
| Host header handling | N/A | PASS | By design, documented |
| Trusted proxy | N/A | PASS | Secure defaults, XFF validation |

---

## 6. Recommendations

1. **Ensure `deny_private_upstreams: true`** in production configuration to block private IP ranges.

2. **Monitor DNS changes** for subgraph hostnames - the 5-minute OIDC cache could allow brief rebinding windows.

3. **Consider DNS pinning** for critical subgraph URLs if the threat model requires it.

4. **No HTTP bridging module exists** (`internal/httpbc/` was not found) - if this functionality was expected, verify the requirements.

---

## 7. Conclusion

The APICerebrus codebase demonstrates **good SSRF hygiene** with multiple layers of defense:
- DNS resolution validation fails closed
- IPv4-mapped IPv6 addresses are unwrapped
- Health checks and WebSocket upgrades are protected
- Federation subgraph URLs are re-validated before each request

Request smuggling risk is **low** due to Go's HTTP library handling Transfer-Encoding correctly and proper header filtering.

**Overall Assessment:** No critical or high-severity SSRF/request smuggling vulnerabilities identified. Existing mitigations are solid and well-implemented.
