package gateway

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	stdpath "path"
	"strings"
	"sync"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// RequestContext carries state for forwarding a single request.
type RequestContext struct {
	Request         *http.Request
	ResponseWriter  http.ResponseWriter
	Route           *config.Route
	Consumer        *config.Consumer
	UpstreamTimeout time.Duration // Per-request upstream timeout (0 = use transport defaults)
}

// Proxy handles upstream forwarding.
type Proxy struct {
	transport *http.Transport
	bufPool   sync.Pool
}

// denyPrivateUpstreams controls whether private/loopback upstream IPs are rejected.
// It is set once at gateway initialization from cfg.Gateway.DenyPrivateUpstreams.
var denyPrivateUpstreams bool

// SetDenyPrivateUpstreams configures SSRF protection for upstream IPs.
// When true, private (10.x, 172.16.x, 192.168.x) and loopback (127.x) IPs are rejected.
func SetDenyPrivateUpstreams(v bool) { denyPrivateUpstreams = v }

// NewProxy creates a reverse proxy with sensible transport pooling defaults.
func NewProxy(pool config.PoolConfig) *Proxy {
	maxIdleConns := 1000
	if pool.MaxIdleConns > 0 {
		maxIdleConns = pool.MaxIdleConns
	}
	maxIdleConnsPerHost := 100
	if pool.MaxIdleConnsPerHost > 0 {
		maxIdleConnsPerHost = pool.MaxIdleConnsPerHost
	}
	idleConnTimeout := 90 * time.Second
	if pool.IdleConnTimeout > 0 {
		idleConnTimeout = pool.IdleConnTimeout
	}

	return &Proxy{
		transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			MaxIdleConns:          maxIdleConns,
			MaxIdleConnsPerHost:   maxIdleConnsPerHost,
			IdleConnTimeout:       idleConnTimeout,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ForceAttemptHTTP2:     true,
		},
		bufPool: sync.Pool{
			New: func() any {
				return make([]byte, 32*1024)
			},
		},
	}
}

// Forward sends the incoming request to selected upstream target and streams response.
func (p *Proxy) Forward(ctx *RequestContext, target *config.UpstreamTarget) error {
	resp, err := p.Do(ctx, target)
	if err != nil {
		writeProxyError(ctx.ResponseWriter, proxyErrorStatus(err))
		return err
	}
	defer resp.Body.Close()
	return p.WriteResponse(ctx.ResponseWriter, resp)
}

// Do performs upstream request and returns raw upstream response without writing to client.
func (p *Proxy) Do(ctx *RequestContext, target *config.UpstreamTarget) (*http.Response, error) {
	if p == nil {
		return nil, errors.New("proxy is nil")
	}
	if ctx == nil || ctx.Request == nil {
		return nil, errors.New("invalid request context")
	}
	if target == nil || strings.TrimSpace(target.Address) == "" {
		return nil, errors.New("invalid upstream target")
	}

	upstreamPath := normalizePath(ctx.Request.URL.Path)
	if ctx.Route != nil && ctx.Route.StripPath {
		upstreamPath = stripPathForProxy(ctx.Route, upstreamPath)
	}

	upstreamURL, err := buildUpstreamURL(target.Address, upstreamPath, ctx.Request.URL.RawQuery)
	if err != nil {
		return nil, err
	}

	body := ctx.Request.Body
	if ctx.Request.GetBody != nil {
		if rc, getErr := ctx.Request.GetBody(); getErr == nil {
			body = rc
		}
	}
	proxyReq, err := http.NewRequestWithContext(ctx.Request.Context(), ctx.Request.Method, upstreamURL, body)
	if err != nil {
		return nil, err
	}

	copyHeaders(proxyReq.Header, ctx.Request.Header)
	appendForwardedHeaders(proxyReq, ctx.Request)

	if ctx.Route != nil && ctx.Route.PreserveHost {
		proxyReq.Host = ctx.Request.Host
	} else {
		u, parseErr := url.Parse(upstreamURL)
		if parseErr == nil && u.Host != "" {
			proxyReq.Host = u.Host
		}
	}

	// Apply per-request upstream timeout if configured
	if ctx.UpstreamTimeout > 0 {
		reqCtx, cancel := context.WithTimeout(proxyReq.Context(), ctx.UpstreamTimeout)
		defer cancel()
		proxyReq = proxyReq.WithContext(reqCtx)
	}

	return p.transport.RoundTrip(proxyReq)
}

// WriteResponse streams an upstream response to downstream client.
func (p *Proxy) WriteResponse(w http.ResponseWriter, resp *http.Response) error {
	if p == nil {
		return errors.New("proxy is nil")
	}
	if w == nil || resp == nil {
		return errors.New("invalid write response args")
	}
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	buf := p.bufPool.Get().([]byte)
	defer p.bufPool.Put(buf) //nolint
	_, err := io.CopyBuffer(w, resp.Body, buf)
	return err
}

// ForwardWebSocket proxies an upgraded websocket connection and tunnels both directions.
func (p *Proxy) ForwardWebSocket(ctx *RequestContext, target *config.UpstreamTarget) error {
	if p == nil {
		return errors.New("proxy is nil")
	}
	if ctx == nil || ctx.Request == nil || ctx.ResponseWriter == nil {
		return errors.New("invalid request context")
	}
	if !isWebSocketUpgrade(ctx.Request) {
		return errors.New("request is not websocket upgrade")
	}
	if target == nil || strings.TrimSpace(target.Address) == "" {
		return errors.New("invalid upstream target")
	}

	hijacker, ok := ctx.ResponseWriter.(http.Hijacker)
	if !ok {
		return errors.New("response writer does not support hijacking")
	}

	upstreamPath := normalizePath(ctx.Request.URL.Path)
	if ctx.Route != nil && ctx.Route.StripPath {
		upstreamPath = stripPathForProxy(ctx.Route, upstreamPath)
	}
	upstreamURL, err := buildUpstreamURL(target.Address, upstreamPath, ctx.Request.URL.RawQuery)
	if err != nil {
		return err
	}
	upstreamParsed, err := url.Parse(upstreamURL)
	if err != nil {
		return err
	}

	upstreamConn, err := dialUpstreamWebSocket(upstreamParsed)
	if err != nil {
		return err
	}

	upstreamReq := ctx.Request.Clone(ctx.Request.Context())
	upstreamReq.URL = upstreamParsed
	upstreamReq.RequestURI = ""
	upstreamReq.Host = ctx.Request.Host
	if ctx.Route == nil || !ctx.Route.PreserveHost {
		upstreamReq.Host = upstreamParsed.Host
	}

	if err := upstreamReq.Write(upstreamConn); err != nil {
		_ = upstreamConn.Close()
		return err
	}

	upstreamReader := bufio.NewReader(upstreamConn)
	upstreamResp, err := http.ReadResponse(upstreamReader, upstreamReq)
	if err != nil {
		_ = upstreamConn.Close()
		return err
	}

	if upstreamResp.StatusCode != http.StatusSwitchingProtocols {
		defer upstreamResp.Body.Close()
		copyHeaders(ctx.ResponseWriter.Header(), upstreamResp.Header)
		ctx.ResponseWriter.WriteHeader(upstreamResp.StatusCode)
		_, _ = io.Copy(ctx.ResponseWriter, upstreamResp.Body)
		_ = upstreamConn.Close()
		return fmt.Errorf("upstream websocket upgrade rejected with status %d", upstreamResp.StatusCode)
	}

	clientConn, clientRW, err := hijacker.Hijack()
	if err != nil {
		_ = upstreamConn.Close()
		return err
	}

	// Flush the 101 response from upstream to the client.
	if err := upstreamResp.Write(clientRW.Writer); err != nil {
		_ = clientConn.Close()
		_ = upstreamConn.Close()
		return err
	}
	if err := clientRW.Writer.Flush(); err != nil {
		_ = clientConn.Close()
		_ = upstreamConn.Close()
		return err
	}

	if buffered := clientRW.Reader.Buffered(); buffered > 0 {
		tmp := make([]byte, buffered)
		_, _ = io.ReadFull(clientRW.Reader, tmp)
		_, _ = upstreamConn.Write(tmp)
	}

	errCh := make(chan error, 2)
	go tunnelCopy(upstreamConn, clientRW, errCh)
	go tunnelCopy(clientConn, upstreamReader, errCh)

	firstErr := <-errCh
	_ = clientConn.Close()
	_ = upstreamConn.Close()
	<-errCh

	if isBenignTunnelClose(firstErr) {
		return nil
	}
	return firstErr
}

func stripPathForProxy(route *config.Route, requestPath string) string {
	prefix, ok := bestStripPrefix(route.Paths, requestPath)
	if !ok || prefix == "/" {
		return requestPath
	}

	rewritten := strings.TrimPrefix(requestPath, prefix)
	if rewritten == "" {
		return "/"
	}
	if !strings.HasPrefix(rewritten, "/") {
		return "/" + rewritten
	}
	return rewritten
}

func buildUpstreamURL(targetAddress, pathValue, rawQuery string) (string, error) {
	target := strings.TrimSpace(targetAddress)
	if target == "" {
		return "", errors.New("empty target address")
	}

	if !strings.Contains(target, "://") {
		target = "http://" + target
	}

	base, err := url.Parse(target)
	if err != nil {
		return "", err
	}
	if base.Host == "" {
		return "", fmt.Errorf("missing host in target address %q", targetAddress)
	}

	// SSRF protection: block private, loopback, link-local, and metadata IPs
	if err := validateUpstreamHost(base.Host); err != nil {
		return "", err
	}

	base.Path = joinURLPath(base.Path, pathValue)
	base.RawQuery = rawQuery
	return base.String(), nil
}

// validateUpstreamHost rejects cloud metadata (169.254.0.0/16) and
// unspecified (0.0.0.0/::) addresses to prevent SSRF via config.
// When denyPrivateUpstreams is true (set via SetDenyPrivateUpstreams),
// private (10.x, 172.16-31.x, 192.168.x) and loopback (127.x) IPs are also rejected.
func validateUpstreamHost(host string) error {
	// Strip port if present
	h := host
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		// Check if this is an IPv6 bracket address or IPv4:port
		if strings.HasPrefix(host, "[") {
			// IPv6: [::1]:8080
			if end := strings.Index(host, "]"); end != -1 && end+1 == idx {
				h = host[1:end]
			}
		} else {
			h = host[:idx]
		}
	}

	ip := net.ParseIP(h)
	if ip == nil {
		// Not a literal IP — resolve hostname and validate each resolved IP
		addrs, err := net.LookupHost(h)
		if err != nil {
			// Cannot resolve — allow with warning (dialer will fail if unreachable)
			return nil
		}
		for _, addr := range addrs {
			if err := validateResolvedIP(addr, host); err != nil {
				return err
			}
		}
		return nil
	}

	// Block link-local (169.254.0.0/16) — includes cloud metadata (169.254.169.254)
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 169 && ip4[1] == 254 {
		return fmt.Errorf("upstream address %q is in link-local/metadata range", host)
	}
	// Block unspecified addresses
	if ip.IsUnspecified() {
		return fmt.Errorf("upstream address %q is unspecified", host)
	}
	// Block private and loopback IPs when deny_private_upstreams is enabled
	if denyPrivateUpstreams {
		if ip.IsLoopback() {
			return fmt.Errorf("upstream address %q is loopback", host)
		}
		// Private ranges: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
		if ip4 := ip.To4(); ip4 != nil {
			if ip4[0] == 10 ||
				(ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) ||
				(ip4[0] == 192 && ip4[1] == 168) {
				return fmt.Errorf("upstream address %q is in private range", host)
			}
		}
	}
	return nil
}

// validateResolvedIP checks a resolved IP against SSRF blocklist.
// Called after DNS resolution of a hostname in validateUpstreamHost.
func validateResolvedIP(addr, originalHost string) error {
	ip := net.ParseIP(addr)
	if ip == nil {
		return nil
	}
	// Block link-local (169.254.0.0/16) — includes cloud metadata (169.254.169.254)
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 169 && ip4[1] == 254 {
		return fmt.Errorf("upstream host %q resolves to link-local/metadata IP %q", originalHost, addr)
	}
	// Block unspecified addresses
	if ip.IsUnspecified() {
		return fmt.Errorf("upstream host %q resolves to unspecified IP %q", originalHost, addr)
	}
	// Block private and loopback when deny_private_upstreams is enabled
	if denyPrivateUpstreams {
		if ip.IsLoopback() {
			return fmt.Errorf("upstream host %q resolves to loopback IP %q", originalHost, addr)
		}
		if ip4 := ip.To4(); ip4 != nil {
			if ip4[0] == 10 ||
				(ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) ||
				(ip4[0] == 192 && ip4[1] == 168) {
				return fmt.Errorf("upstream host %q resolves to private IP %q", originalHost, addr)
			}
		}
	}
	return nil
}

func dialUpstreamWebSocket(upstreamURL *url.URL) (net.Conn, error) {
	// Validate upstream host to prevent SSRF attacks
	if err := validateUpstreamHost(upstreamURL.Host); err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	switch strings.ToLower(upstreamURL.Scheme) {
	case "https", "wss":
		return tls.DialWithDialer(dialer, "tcp", upstreamURL.Host, &tls.Config{
			MinVersion: tls.VersionTLS12,
		})
	case "http", "ws":
		return dialer.Dial("tcp", upstreamURL.Host)
	default:
		return nil, fmt.Errorf("unsupported upstream scheme %q for websocket", upstreamURL.Scheme)
	}
}

func joinURLPath(basePath, requestPath string) string {
	base := normalizePath(basePath)
	req := normalizePath(requestPath)
	if base == "/" {
		return req
	}
	if req == "/" {
		return base
	}
	return stdpath.Clean(base + "/" + strings.TrimPrefix(req, "/"))
}

func appendForwardedHeaders(dst, src *http.Request) {
	if dst == nil || src == nil {
		return
	}

	remoteIP := clientIP(src.RemoteAddr)
	if remoteIP != "" {
		existing := strings.TrimSpace(src.Header.Get("X-Forwarded-For"))
		if existing != "" {
			dst.Header.Set("X-Forwarded-For", existing+", "+remoteIP)
		} else {
			dst.Header.Set("X-Forwarded-For", remoteIP)
		}
	}
	dst.Header.Set("X-Forwarded-Host", src.Host)
	if src.TLS != nil {
		dst.Header.Set("X-Forwarded-Proto", "https")
	} else {
		dst.Header.Set("X-Forwarded-Proto", "http")
	}
}

func copyHeaders(dst, src http.Header) {
	if dst == nil || src == nil {
		return
	}

	connectionTokens := parseConnectionTokens(src)
	for key, values := range src {
		canonical := textproto.CanonicalMIMEHeaderKey(key)
		lower := strings.ToLower(canonical)
		if hopByHopHeaders[lower] {
			continue
		}
		if _, blocked := connectionTokens[lower]; blocked {
			continue
		}
		// Strip internal upstream headers that should not leak to clients (CWE-200)
		if shouldStripHeader(lower) {
			continue
		}
		for _, value := range values {
			dst.Add(canonical, value)
		}
	}
}

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

// internalHeaderPrefixes lists header prefixes used internally by upstream
// services that should not be exposed to downstream clients.
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

func shouldStripHeader(lower string) bool {
	for _, prefix := range internalHeaderPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func parseConnectionTokens(headers http.Header) map[string]struct{} {
	out := make(map[string]struct{})
	for _, value := range headers.Values("Connection") {
		for _, token := range strings.Split(value, ",") {
			t := strings.ToLower(strings.TrimSpace(token))
			if t == "" {
				continue
			}
			out[t] = struct{}{}
		}
	}
	return out
}

func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(remoteAddr)
}

func isTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func writeProxyError(w http.ResponseWriter, status int) {
	http.Error(w, http.StatusText(status), status)
}

func proxyErrorStatus(err error) int {
	if isTimeoutError(err) {
		return http.StatusGatewayTimeout
	}
	return http.StatusBadGateway
}

func isWebSocketUpgrade(req *http.Request) bool {
	if req == nil || req.Method != http.MethodGet {
		return false
	}

	hasUpgradeToken := false
	for _, token := range strings.Split(req.Header.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
			hasUpgradeToken = true
			break
		}
	}
	return hasUpgradeToken && strings.EqualFold(strings.TrimSpace(req.Header.Get("Upgrade")), "websocket")
}

func tunnelCopy(dst net.Conn, src io.Reader, errCh chan<- error) {
	_, err := io.Copy(dst, src)
	if tcp, ok := dst.(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
	}
	errCh <- err
}

func isBenignTunnelClose(err error) bool {
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "use of closed network connection")
}
