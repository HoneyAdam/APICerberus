# gRPC + WebSocket + Proxy Audit
**Scope:** internal/grpc/**, internal/gateway/** (ws + proxy)
**Date:** 2026-04-16
**Auditor:** security-review sub-agent (focused pass)
**Prior reports avoided:** SECURITY-REPORT.md MED-NEW-3 (admin WS Origin) and Finding 6 (WS brute force) — already fixed. This audit targets the gateway-side streaming/protocol-confusion surface only.

## Findings

### PROXY-001: gRPC-Web handler emits wildcard CORS on authenticated responses
- **Severity:** High
- **Confidence:** High
- **CWE:** CWE-942 (Permissive CORS), CWE-346 (Origin Validation Error)
- **File:** `internal/grpc/proxy.go:217-219`

```go
w.Header().Set("Content-Type", "application/grpc-web+proto")
w.Header().Set("Access-Control-Allow-Origin", "*")
w.Header().Set("Access-Control-Expose-Headers", "grpc-status, grpc-message")
```

**Description:** `handleGRPCWeb` unconditionally sets `Access-Control-Allow-Origin: *` on every response, including those returned for authenticated gRPC calls. The request-side `metadataFromHeaders` passes through `Cookie` and `Authorization` headers (see PROXY-003), so the response body is fully privileged — yet any origin may read it.

Combined with cookie forwarding, an attacker-hosted page can issue a gRPC-Web call to the gateway and read back session-authenticated data because browsers allow the read when ACAO is `*` (credential-mode aside, same-site cookies, session tokens in response body, etc.).

**Exploit scenario:**
1. User is logged into a gRPC-backed service behind the gateway.
2. User visits `evil.com`.
3. Evil page sends `fetch("https://gateway/grpc.ServiceA/GetSecrets", {method: "POST", headers: {"Content-Type": "application/grpc-web+proto"}, body: <crafted frame>})`.
4. Gateway's upstream gRPC honors session cookie forwarded verbatim, returns secret data.
5. Response has `ACAO: *` → evil page reads the response body and exfiltrates it.

Note this `handleGRPCWeb` handler is in a package that appears currently un-wired (the production H2C server uses `gateway.Gateway` as handler, not `grpc.Proxy`). However the type is public and documented as usable, so the bug will ship the moment an operator wires it in per the README/CLAUDE.md (`grpc.enable_web: true`).

**Remediation:**
- Reflect a validated origin from `cfg.Gateway.AllowedOrigins`, never `*`.
- If `Authorization` or `Cookie` header is present, require `Access-Control-Allow-Credentials: true` AND a specific allowed origin.
- Delete the ACAO/Expose-Headers lines here and delegate CORS to the existing `internal/plugin/cors.go` policy that runs in the pipeline.

---

### PROXY-002: Gateway health checker ignores SSRF protection
- **Severity:** High
- **Confidence:** High
- **CWE:** CWE-918 (SSRF)
- **File:** `internal/gateway/health.go:127-141` (contrast with `proxy.go:315-369 validateUpstreamHost`)

```go
func runHealthCheck(ctx context.Context, client *http.Client, address, path string) (bool, time.Duration) {
    start := time.Now()
    targetPath := normalizePath(path)
    url := "http://" + strings.TrimSpace(address) + targetPath
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)

    resp, err := client.Do(req)
    ...
    return resp.StatusCode >= 200 && resp.StatusCode < 400, latency
}
```

**Description:** The SSRF gate `validateUpstreamHost` (proxy.go:315) blocks link-local (169.254.0.0/16 / AWS-GCP metadata), unspecified addresses, and (when enabled) private/loopback ranges. The active health checker does NOT call this gate. It directly dials whatever address an upstream target is configured with.

Reachability to `applyHealthResult` is trivial — anyone who can add an upstream target via the admin API (the entire admin surface is designed to let operators register upstreams via `POST /admin/api/v1/upstream/targets`) gets a reflective SSRF oracle: the boolean healthy/unhealthy result and observed `latency` are exposed at `GET /admin/api/v1/upstreams/.../health` and in the realtime WebSocket stream.

The oracle lets an attacker with admin-lite access:
- Enumerate the cloud metadata service (`169.254.169.254`) — the normal proxy blocks this, the health path does not.
- Port-scan 127.0.0.1 / RFC1918 (status 200-399 = open). `denyPrivateUpstreams` only affects the proxy path.
- Use sub-second latency timing to infer firewall state.

**Exploit scenario:**
```
POST /admin/api/v1/upstream/targets
{"upstream": "scan", "address": "169.254.169.254:80", "healthcheck": {"path": "/latest/meta-data/iam/security-credentials/"}}
```
Within one health interval, `GET /admin/api/v1/upstreams/scan` reports `healthy=true` iff the metadata path returns 2xx/3xx — confirming existence of IAM role credentials.

**Remediation:**
- Call `validateUpstreamHost(address)` at the top of `runHealthCheck` and return `false` on error.
- Consider a separate, stricter `validateHealthTarget` that always blocks metadata/link-local regardless of `denyPrivateUpstreams`, since health checks can reasonably need LAN access but never need cloud metadata.
- Rate-limit admin API upstream creation or scope to RBAC role.

---

### PROXY-003: gRPC proxy forwards Cookie/Authorization unfiltered as metadata (upstream credential leak)
- **Severity:** Medium
- **Confidence:** High
- **CWE:** CWE-200 (Information Exposure), CWE-345 (Insufficient Verification of Data Authenticity)
- **File:** `internal/grpc/proxy.go:321-349`

```go
func metadataFromHeaders(headers http.Header) metadata.MD {
    md := make(metadata.MD)
    for k, v := range headers {
        if isHTTPHeader(k) { continue }
        key := strings.ToLower(k)
        md[key] = v
    }
    return md
}

func isHTTPHeader(key string) bool {
    httpHeaders := []string{
        "Accept", "Accept-Encoding", "Accept-Language",
        "Connection", "Content-Length", "Content-Type",
        "Host", "Transfer-Encoding", "User-Agent",
        "Upgrade", "Proxy-Authorization", "TE",
    }
    ...
}
```

**Description:** The blocklist omits `Authorization`, `Cookie`, `X-Admin-Key`, `X-Api-Key`, `X-Forwarded-*`, and every custom auth header the admin API uses. These get lowercased and forwarded verbatim into gRPC trailers-metadata to the upstream gRPC backend.

Combined with PROXY-001 (gRPC-Web wildcard CORS), a malicious origin can cause the user's admin cookie or auth token to reach an arbitrary upstream gRPC service as metadata. If the upstream has any logging sidecar or honors the forwarded token for its own auth, the user's gateway credentials are exfiltrated.

**Exploit scenario:**
1. Operator configures an upstream gRPC pointing to a third-party analytics service.
2. User browser has `Cookie: admin_session=...` from prior login to admin UI on same parent domain.
3. Any page (even unrelated one on same eTLD) triggers gRPC-Web call → gateway → forwards `cookie: admin_session=...` as metadata to third-party.

**Remediation:**
- Use an allow-list (e.g., only `x-grpc-*`, `x-trace-id`, custom prefixes) instead of the current tiny deny-list.
- Always strip `authorization`, `cookie`, `set-cookie`, `x-admin-*`, `x-api-key`, `proxy-authorization`.
- Mirror the `internalHeaderPrefixes` stripping from `gateway/proxy.go:493-502`.

---

### PROXY-004: gRPC response headers written from untrusted upstream metadata (response splitting / security-header clobbering)
- **Severity:** Medium
- **Confidence:** Medium
- **CWE:** CWE-113 (HTTP Response Splitting), CWE-346
- **File:** `internal/grpc/proxy.go:139-144`, `proxy.go:160-163`

```go
// Convert gRPC metadata to HTTP headers
for k, v := range headerMD {
    for _, val := range v {
        w.Header().Add(k, val)
    }
}
```

**Description:** The handler blindly writes every key/value from upstream gRPC `headerMD` and `trailerMD` into HTTP response headers without filtering. A malicious or compromised upstream gRPC server can:
- Emit metadata named `Content-Security-Policy`, `Strict-Transport-Security`, or `Set-Cookie` to override the security headers the gateway injects in `addSecurityHeaders`.
- Emit metadata to set arbitrary cookies in the client's browser.
- Emit metadata that shadows `Access-Control-Allow-Origin` (already widened to `*` per PROXY-001).

Go's `net/http` will reject `\r\n` in values, so strict CRLF injection is blocked by stdlib, but semantic clobbering of security headers remains.

Also: `Grpc-Message` is set from `err.Error()` and `st.Message()` unsanitized (line 163). If upstream crafts a status message containing printable non-ASCII control chars or extremely long content, it is reflected in the response header (length is DoS-amplifying but stdlib caps header size, so low impact).

**Remediation:**
- Apply `internalHeaderPrefixes` stripping + an allow-list pattern (e.g., `x-` prefix only, or an explicit config-defined whitelist).
- Reject metadata keys that collide with well-known security headers (`content-security-policy`, `strict-transport-security`, `set-cookie`, `x-frame-options`, `x-content-type-options`, `access-control-*`).
- Truncate `Grpc-Message` to a reasonable length (≤1 KB).

---

### PROXY-005: OptimizedProxy drops Connection-tokened hop-by-hop headers incorrectly (header smuggling)
- **Severity:** Medium
- **Confidence:** High
- **CWE:** CWE-444 (HTTP Request Smuggling — header-handling subclass)
- **File:** `internal/gateway/optimized_proxy.go:662-688` (compare to `proxy.go:454-525`)

```go
func copyHeadersOptimized(dst, src http.Header) {
    ...
    for key, values := range src {
        if isHopByHopHeader(key) { continue }
        for _, value := range values {
            dst.Add(key, value)
        }
    }
}

func isHopByHopHeader(key string) bool {
    switch strings.ToLower(key) {
    case "connection", "proxy-connection", "keep-alive", "proxy-authenticate",
        "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
        return true
    }
    return false
}
```

**Description:** RFC 7230 §6.1 requires a proxy to treat headers *named in* `Connection:` as hop-by-hop and NOT forward them. The sibling `proxy.go` handles this correctly via `parseConnectionTokens` (proxy.go:459), but `OptimizedProxy.copyHeadersOptimized` only drops the canonical RFC hop-by-hop set and ignores custom tokens listed in the client's `Connection` header.

**Exploit scenario (CVE-2022-32148 class):**
- Client sends `Connection: close, X-Forwarded-For\r\nX-Forwarded-For: 127.0.0.1\r\n...`.
- The XFF header is nominally hop-by-hop per the client's Connection token → a compliant proxy strips it → but this gateway forwards it.
- Upstream, seeing the gateway's appended `X-Forwarded-For` AND client's original, may trust the first entry (client-spoofed `127.0.0.1`) for rate-limit bypass / admin allow-list bypass.

- Same issue for client-smuggled `Connection: X-API-Key` — the gateway normally would drop `X-API-Key` as local-hop-only, but this implementation forwards it.

Additionally, `OptimizedProxy` never calls `shouldStripHeader` so all `x-forwarded-*`, `x-amz-*`, `x-envoy-*` from upstream pass through to the client — a parity regression vs. the main `Proxy.copyHeaders` (CWE-200).

**Remediation:**
- Port the `parseConnectionTokens` + `shouldStripHeader` logic from `proxy.go:454-525` into `copyHeadersOptimized`.
- Unify: both proxies should call the same `copyHeaders` implementation — current divergence is a maintenance trap.

---

### PROXY-006: OptimizedProxy request coalescing key omits Cookie — cross-user cache poisoning
- **Severity:** High
- **Confidence:** High
- **CWE:** CWE-345, CWE-524 (Information Exposure Through Caching)
- **File:** `internal/gateway/optimized_proxy.go:539-589`

```go
func (p *OptimizedProxy) isCacheableRequest(req *http.Request) bool {
    if method != http.MethodGet && method != http.MethodHead { return false }
    cacheControl := req.Header.Get("Cache-Control")
    if strings.Contains(cacheControl, "no-cache") || ... { return false }
    return true
}

func (p *OptimizedProxy) coalesceKey(req *http.Request, upstreamURL *url.URL) string {
    ...
    if auth := req.Header.Get("Authorization"); auth != "" { ... }
    else if apiKey := req.Header.Get("X-API-Key"); apiKey != "" { ... }
    // NO Cookie, NO X-Admin-Key, NO custom auth headers
    varyHeaders := []string{"Accept", "Accept-Encoding", "Accept-Language"}
    ...
}
```

**Description:** Request coalescing buffers one response and serves it to every waiter with an identical key. The key partitions by `Authorization` and `X-API-Key` only — it does **not** partition by `Cookie` or any custom auth header.

For a cookie-authenticated GET endpoint (e.g., `/api/me`, `/api/orders`), two concurrent users within the 10ms coalescing window, with the same `Accept` headers, produce the same key. The second user receives the first user's private response.

**Exploit scenario:**
1. Attacker identifies a cookie-authenticated endpoint `/api/me` routed through OptimizedProxy with coalescing on.
2. Attacker blasts GET `/api/me` without `Authorization` header, matching defaults for Accept.
3. A legitimate logged-in user (cookie-only auth) requests `/api/me` within the same 10ms window.
4. The attacker's parallel request is served the logged-in user's buffered response.

Cache-Control: no-cache disables coalescing but most API clients don't send that header on GETs.

**Remediation:**
- Either: include ALL inbound `Cookie` values in the coalesce key, OR
- Disable coalescing entirely for requests with `Cookie`, `Authorization`, `X-API-Key`, or any header whose lowercase name starts with `x-auth-` / `x-admin-` / `x-api-`.
- Preferable: require an explicit opt-in per route (`route.enable_coalescing: true`) rather than coalescing everything by default.

---

### PROXY-007: OptimizedProxy omits `X-Forwarded-Proto` and propagates spoofed XFF
- **Severity:** Low
- **Confidence:** High
- **CWE:** CWE-348 (Use of Less Trusted Source), CWE-290 (Authentication Bypass by Spoofing)
- **File:** `internal/gateway/optimized_proxy.go:633-660`

```go
func (p *OptimizedProxy) appendForwardedHeaders(dst, src *http.Request) {
    ...
    remoteIP := p.clientIP(src.RemoteAddr)
    if remoteIP != "" {
        existing := strings.TrimSpace(src.Header.Get("X-Forwarded-For"))
        if existing != "" {
            dst.Header.Set("X-Forwarded-For", existing+", "+remoteIP)
        } else {
            dst.Header.Set("X-Forwarded-For", remoteIP)
        }
    }
    dst.Header.Set("X-Forwarded-Host", src.Host)
}
```

**Description:**
1. When `gateway.trusted_proxies` is empty (secure-by-default per CLAUDE.md), the gateway's own `netutil.ClientIP` ignores client-supplied XFF. But `OptimizedProxy.appendForwardedHeaders` still reads the raw inbound XFF and concatenates it into the outbound XFF — re-introducing the spoofed IP as the first entry that a naive upstream will trust.
2. Unlike `proxy.go:appendForwardedHeaders`, the Optimized variant never sets `X-Forwarded-Proto` based on `src.TLS`. Any upstream that trusts XFP for security decisions (redirect loops, HSTS, cookie Secure flag inference) will see a wrong or missing value.

**Remediation:**
- Consult the trusted-proxy list: if incoming peer is not trusted, drop client-supplied `X-Forwarded-*` before appending the gateway's own.
- Always emit `X-Forwarded-Proto` based on `src.TLS != nil`.
- Share code with `proxy.go:appendForwardedHeaders`.

---

### PROXY-008: Dead WebSocket proxy code — `ForwardWebSocket` never wired; WS upstreams are effectively broken
- **Severity:** Informational / Medium (depending on advertised support)
- **Confidence:** High
- **CWE:** CWE-754 (Improper Check for Unusual Conditions)
- **File:** `internal/gateway/proxy.go:161-265` (definition); never referenced outside tests

**Description:** The gateway advertises "Full-duplex WebSocket tunneling" (CLAUDE.md "WebSocket Support" section), but `ForwardWebSocket` is not called from the main `ServeHTTP` request path (`server.go:194-322`) nor from `serve_proxy.go`. The normal `executeProxyChain` invokes `g.proxy.Forward` (non-WS), which runs an HTTP round-trip via `http.Transport`. This does not perform a WebSocket handshake to the upstream.

A WS upgrade request arriving at the gateway:
- Matches a route normally.
- `isWebSocketUpgrade(req)` is never checked during proxy dispatch.
- Goes through `http.Transport.RoundTrip` — which supports `Upgrade:` only via `http.Client.Do` with proper dial hijacking; `http.Transport` will return a non-101 response to the client.

Net effect: the advertised WS feature silently fails OR partially tunnels depending on transport quirks. Separately, because `ForwardWebSocket` is dead code, nobody has validated that it:
- Enforces an `Origin` allow-list (**it does not** — no origin check in the function, unlike admin `ws.go:48`).
- Runs the plugin pipeline's `AUTH` phase before hijacking (it does, because `ServeHTTP` runs AUTH before proxy dispatch, but only for non-WS dispatch; the code path is untested and brittle).
- Validates `Sec-WebSocket-Key` (it doesn't).

**Exploit scenario (when feature is actually wired up in future):**
- Cross-Site WebSocket Hijacking: any origin can trigger an authenticated WS upgrade; the gateway proxies to upstream with the victim's cookies and returns a live tunnel to attacker's page. Admin WS fixed this (MED-NEW-3) but the gateway WS path will regress unless PROXY-008 and PROXY-009 are fixed before shipping.

**Remediation:**
1. Either wire `ForwardWebSocket` into `executeProxyChain` (detect `isWebSocketUpgrade(r)` → dispatch to `ForwardWebSocket`) or delete the dead code.
2. Before shipping, mirror `admin/ws.go:isValidWebSocketOrigin` — require `Origin` to match route-scoped allow-list BEFORE upgrade.
3. Add an integration test that exercises a full WS upgrade through the plugin pipeline.

---

### PROXY-009: `ForwardWebSocket` has no request-size / frame-size / read-deadline limits
- **Severity:** Medium (conditional on PROXY-008 being wired)
- **Confidence:** High
- **CWE:** CWE-400 (Uncontrolled Resource Consumption), CWE-770 (Allocation of Resources Without Limits)
- **File:** `internal/gateway/proxy.go:228-264`

```go
clientConn, clientRW, err := hijacker.Hijack()
...
errCh := make(chan error, 2)
go tunnelCopy(upstreamConn, clientRW, errCh)
go tunnelCopy(clientConn, upstreamReader, errCh)

firstErr := <-errCh
_ = clientConn.Close()
_ = upstreamConn.Close()
<-errCh
```

**Description:** Once upgraded:
- `io.Copy` copies indefinitely with no per-frame or per-connection byte ceiling.
- No read deadline is set on `clientConn` or `upstreamConn` — an idle connection is kept open forever, pinning goroutines and sockets.
- No ping/pong heartbeat — dead peers are not detected.
- No cap on pending writes — slow-read attacker can force memory growth in TCP send buffers.

`http.Server`'s `ReadTimeout` / `IdleTimeout` do not apply to a hijacked connection.

**Remediation:**
- Set `SetReadDeadline` / `SetWriteDeadline` on both conns (re-arm on each frame).
- Enforce max frame size from config (e.g., `ws.max_frame_bytes`, default 1 MiB).
- Enforce max session duration (e.g., `ws.max_session_duration`, default 1h).
- Implement WS-level ping on an interval; tear down on pong timeout.

---

### PROXY-010: `handleGRPC` method path not validated — plugin pipeline can be bypassed under direct gRPC port exposure
- **Severity:** Medium
- **Confidence:** Medium
- **CWE:** CWE-285 (Improper Authorization)
- **File:** `internal/grpc/proxy.go:110-114`, `internal/gateway/server.go:397-411`

**Description:** The H2C gRPC server (`grpcpkg.NewH2CServer(grpcConfig, g)`) binds a dedicated port (default `:50051`) with the gateway as handler. When a request hits that port with `Content-Type: application/grpc`, `Gateway.ServeHTTP` still runs the full pipeline, so routes scoped to `/package.Service/Method` gRPC paths must exist.

However, the path resolution uses `r.URL.Path` exactly as the gRPC :path pseudo-header. The radix router has no concept of gRPC service semantics — a route pattern `/api/*` will match a gRPC path `/api/anything.Service/Method` and apply PRE_AUTH/AUTH plugins designed for REST. The consumer-IP-whitelist, API key, and JWT plugins assume HTTP conventions (`Authorization:` header, `?api_key=`), not gRPC metadata (`grpc-metadata-*` / lowercase keys).

If an operator builds a gRPC route without realizing the auth plugins don't match gRPC conventions (no metadata inspection), they effectively deploy an unauthenticated gRPC port.

Combined with PROXY-003, forwarded Cookie/Auth means admin-level gateway cookies can leak into a gRPC session.

**Remediation:**
- Reject `Content-Type: application/grpc*` unless the matched route has `protocol: grpc`.
- Document (and validate at config load) that gRPC routes MUST include an explicit auth plugin that understands gRPC metadata keys.
- Add a content-type-aware pre-auth plugin for gRPC that pulls `authorization` from metadata rather than the HTTP header layer.

---

## Positive Findings

1. **SSRF hardening on main proxy path is solid.** `validateUpstreamHost` (proxy.go:315) + `validateResolvedIP` (proxy.go:373) gate both literal IPs and DNS-resolved hosts against link-local/metadata. `dialUpstreamWebSocket` also validates (proxy.go:402-406). The `denyPrivateUpstreams` flag lets ops tighten further.
2. **Path length / null-byte / segment-count gating in router.** `router.go:133-141` (`maxPathLength=8192`, `maxPathSegments=256`, rejects `\x00`) defuses `%00`-truncation, stack-overflow, and algorithmic-complexity DoS attempts on the radix walker.
3. **Regex route length cap.** `maxRegexLength=1024` in `router.go:116` prevents ReDoS via config-supplied regex patterns — a common oversight (CWE-1333).
4. **Admin WebSocket defense-in-depth.** `internal/admin/ws.go` strictly validates Origin (`isValidWebSocketOrigin`, with wildcard boundary check via `matchAllowedOrigin`), rejects `Origin: null`, ignores Referer, rate-limits static-key fallback, constant-time key comparison, and refuses plaintext fallback when Admin.Addr binds 0.0.0.0.
5. **Body-size gate runs before route match.** `server.go:215-233` checks `Content-Length` and applies `LimitReader` for chunked bodies — blocks memory-exhaustion at HTTP layer before the plugin pipeline allocates.
6. **Transcoder path parsing is defensive.** `parseGRPCMethod` (transcoder.go:185-205) rejects malformed `/svc/` or `/svc/m/extra` paths; no traversal surface.
7. **`denyPrivateUpstreams` is configurable.** The gate flips correctly at boot via `SetDenyPrivateUpstreams` (proxy.go:43).
8. **Main `Proxy.copyHeaders` does handle Connection-tokens.** `proxy.go:454-477` uses `parseConnectionTokens` and `shouldStripHeader` correctly — the smuggling issue in PROXY-005 is isolated to `OptimizedProxy`.
9. **gRPC body limit.** Every gRPC/gRPC-Web/transcoding entry point caps `io.LimitReader(r.Body, 10<<20)` at 10 MiB (proxy.go:117, 193, 254; stream.go:53, 126, 198) — prevents gRPC body bombs.

---

## Summary

**10 findings** across two previously-unaudited surfaces (gRPC + OptimizedProxy) plus one dead-code hazard (`ForwardWebSocket`).

**Highest risk:**
- **PROXY-002** (health-check SSRF oracle bypasses `validateUpstreamHost`) — exposes cloud metadata to any operator with upstream-target create permission.
- **PROXY-006** (coalescing cross-user response leak for cookie-authenticated endpoints) — active data-leak on prod if OptimizedProxy is enabled with default `EnableCoalescing: true`.
- **PROXY-001 + PROXY-003** (gRPC-Web ACAO:* combined with unfiltered Cookie/Auth forwarding) — a one-two credential-exfil via cross-origin gRPC-Web, latent until operator enables gRPC-Web.

**Divergence risk:** `Proxy` vs `OptimizedProxy` header handling (PROXY-005, PROXY-007) is a maintenance bug — two code paths with different security properties. The hardened one is `Proxy`; the one most likely to be enabled in production is `OptimizedProxy`. Consolidate.

**Dead-code liability:** `ForwardWebSocket` (PROXY-008) will regress into a CSWSH (CWE-346) the moment someone wires it in — no origin check, no frame-size cap (PROXY-009). Recommend delete or fix before any WS upstream feature lands.

No HTTP request smuggling (CL.TE / TE.CL) was found — Go's stdlib enforces one-or-the-other, and neither proxy forwards `Transfer-Encoding` or `Content-Length` manually (both are in the hop-by-hop list). No gRPC reflection was found enabled.
