package gateway

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// OptimizedProxy is a high-performance reverse proxy with connection pooling,
// zero-copy buffers, and request coalescing for maximum throughput.
type OptimizedProxy struct {
	transport      *http.Transport
	reverseProxy   *httputil.ReverseProxy
	bufPool        *sync.Pool
	coalescingPool *requestCoalescingPool
	metrics        *proxyMetrics
	config         OptimizedProxyConfig
}

// OptimizedProxyConfig holds configuration for the optimized proxy.
type OptimizedProxyConfig struct {
	// Connection pool settings
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	MaxConnsPerHost       int
	IdleConnTimeout       time.Duration
	TLSHandshakeTimeout   time.Duration
	ExpectContinueTimeout time.Duration
	ResponseHeaderTimeout time.Duration

	// HTTP/2 settings
	ForceHTTP2         bool
	HTTP2ReadIdleTimeout time.Duration
	HTTP2PingTimeout     time.Duration

	// Buffer settings
	BufferSize         int
	BufferPoolCapacity int

	// Request coalescing
	EnableCoalescing    bool
	CoalescingWindow    time.Duration

	// Timeouts
	ProxyTimeout       time.Duration
	DialTimeout        time.Duration
	KeepAlive          time.Duration
}

// DefaultOptimizedProxyConfig returns sensible defaults for high throughput.
func DefaultOptimizedProxyConfig() OptimizedProxyConfig {
	return OptimizedProxyConfig{
		MaxIdleConns:          10_000,
		MaxIdleConnsPerHost:   1_000,
		MaxConnsPerHost:       2_000,
		IdleConnTimeout:       5 * time.Minute,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ForceHTTP2:            true,
		HTTP2ReadIdleTimeout:  30 * time.Second,
		HTTP2PingTimeout:      5 * time.Second,
		BufferSize:            64 * 1024, // 64KB buffers
		BufferPoolCapacity:    10_000,
		EnableCoalescing:      true,
		CoalescingWindow:      10 * time.Millisecond,
		ProxyTimeout:          30 * time.Second,
		DialTimeout:           10 * time.Second,
		KeepAlive:             30 * time.Second,
	}
}

// proxyMetrics holds performance metrics for the proxy.
type proxyMetrics struct {
	requestsTotal    atomic.Uint64
	requestsActive   atomic.Int64
	requestsCoalesced atomic.Uint64
	bytesTransferred atomic.Uint64
	errorsTotal      atomic.Uint64
}

// requestCoalescingPool deduplicates concurrent identical requests.
type requestCoalescingPool struct {
	mu      sync.RWMutex
	pending map[string]*coalescedRequest
	window  time.Duration
}

type coalescedRequest struct {
	mu       sync.Mutex
	key      string
	resp     *http.Response
	err      error
	done     chan struct{}
	waiters  int
	created  time.Time
}

// NewRequestCoalescingPool creates a new request coalescing pool.
func NewRequestCoalescingPool(window time.Duration) *requestCoalescingPool {
	return &requestCoalescingPool{
		pending: make(map[string]*coalescedRequest),
		window:  window,
	}
}

// Get attempts to join an in-flight request or creates a new one.
func (p *requestCoalescingPool) Get(key string) (*coalescedRequest, bool) {
	p.mu.RLock()
	req, exists := p.pending[key]
	p.mu.RUnlock()

	if exists {
		req.mu.Lock()
		// Check if request is still valid
		if time.Since(req.created) < p.window {
			req.waiters++
			req.mu.Unlock()
			return req, true // Join existing request
		}
		req.mu.Unlock()
	}

	// Create new request
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if req, exists = p.pending[key]; exists {
		req.mu.Lock()
		if time.Since(req.created) < p.window {
			req.waiters++
			req.mu.Unlock()
			return req, true
		}
		req.mu.Unlock()
	}

	newReq := &coalescedRequest{
		key:     key,
		done:    make(chan struct{}),
		created: time.Now(),
		waiters: 1,
	}
	p.pending[key] = newReq
	return newReq, false
}

// Complete marks a request as complete and notifies waiters.
func (p *requestCoalescingPool) Complete(key string, resp *http.Response, err error) {
	p.mu.Lock()
	req, exists := p.pending[key]
	if !exists {
		p.mu.Unlock()
		return
	}
	delete(p.pending, key)
	p.mu.Unlock()

	req.mu.Lock()
	req.resp = resp
	req.err = err
	close(req.done)
	req.mu.Unlock()
}

// NewOptimizedProxy creates a high-performance reverse proxy.
func NewOptimizedProxy(cfg OptimizedProxyConfig) *OptimizedProxy {
	if cfg.MaxIdleConns == 0 {
		cfg = DefaultOptimizedProxyConfig()
	}

	// Create optimized transport
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   cfg.DialTimeout,
			KeepAlive: cfg.KeepAlive,
			DualStack: true,
		}).DialContext,
		ForceAttemptHTTP2:     cfg.ForceHTTP2,
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
		MaxConnsPerHost:       cfg.MaxConnsPerHost,
		IdleConnTimeout:       cfg.IdleConnTimeout,
		TLSHandshakeTimeout:   cfg.TLSHandshakeTimeout,
		ExpectContinueTimeout: cfg.ExpectContinueTimeout,
		ResponseHeaderTimeout: cfg.ResponseHeaderTimeout,
		DisableKeepAlives:     false,
		DisableCompression:    false,
	}

	// Create buffer pool
	bufPool := &sync.Pool{
		New: func() interface{} {
			b := make([]byte, cfg.BufferSize)
			return &b
		},
	}

	// Pre-populate buffer pool
	for i := 0; i < cfg.BufferPoolCapacity; i++ {
		b := make([]byte, cfg.BufferSize)
		bufPool.Put(&b)
	}

	var coalescingPool *requestCoalescingPool
	if cfg.EnableCoalescing {
		coalescingPool = NewRequestCoalescingPool(cfg.CoalescingWindow)
	}

	p := &OptimizedProxy{
		transport:      transport,
		bufPool:        bufPool,
		coalescingPool: coalescingPool,
		metrics:        &proxyMetrics{},
		config:         cfg,
	}

	// Create reverse proxy with custom director and transport
	p.reverseProxy = &httputil.ReverseProxy{
		Director:       p.director,
		Transport:      transport,
		ModifyResponse: p.modifyResponse,
		ErrorHandler:   p.errorHandler,
		BufferPool:     p,
	}

	return p
}

// Get implements httputil.BufferPool interface.
func (p *OptimizedProxy) Get() []byte {
	if p.bufPool == nil {
		return make([]byte, p.config.BufferSize)
	}
	buf := p.bufPool.Get().(*[]byte)
	return *buf
}

// Put implements httputil.BufferPool interface.
func (p *OptimizedProxy) Put(buf []byte) {
	if p.bufPool == nil || cap(buf) < p.config.BufferSize {
		return
	}
	p.bufPool.Put(&buf)
}

// director modifies the outgoing request.
func (p *OptimizedProxy) director(req *http.Request) {
	// The actual URL and headers are set in Forward method
	// This is a no-op director as we handle everything in Forward
}

// modifyResponse modifies the response before sending to client.
func (p *OptimizedProxy) modifyResponse(resp *http.Response) error {
	// Add performance headers
	resp.Header.Set("X-Proxy-Optimized", "true")
	return nil
}

// errorHandler handles proxy errors.
func (p *OptimizedProxy) errorHandler(w http.ResponseWriter, req *http.Request, err error) {
	p.metrics.errorsTotal.Add(1)
	status := http.StatusBadGateway
	if errors.Is(err, context.DeadlineExceeded) {
		status = http.StatusGatewayTimeout
	}
	http.Error(w, http.StatusText(status), status)
}

// Forward proxies a request to the target with optimizations.
func (p *OptimizedProxy) Forward(ctx *RequestContext, target *config.UpstreamTarget) error {
	if p == nil {
		return errors.New("proxy is nil")
	}
	if ctx == nil || ctx.Request == nil {
		return errors.New("invalid request context")
	}
	if target == nil || strings.TrimSpace(target.Address) == "" {
		return errors.New("invalid upstream target")
	}

	p.metrics.requestsTotal.Add(1)
	p.metrics.requestsActive.Add(1)
	defer p.metrics.requestsActive.Add(-1)

	// Build upstream URL
	upstreamURL, err := p.buildUpstreamURL(ctx.Request, target.Address, ctx.Route)
	if err != nil {
		return err
	}

	// Create proxy request
	proxyReq, err := p.createProxyRequest(ctx.Request, upstreamURL)
	if err != nil {
		return err
	}

	// Request coalescing for cacheable requests
	if p.coalescingPool != nil && p.isCacheableRequest(ctx.Request) {
		coalesceKey := p.coalesceKey(ctx.Request, upstreamURL)
		coalescedReq, isWaiter := p.coalescingPool.Get(coalesceKey)

		if isWaiter {
			// Wait for the in-flight request to complete
			<-coalescedReq.done
			p.metrics.requestsCoalesced.Add(1)
			return p.serveCoalescedResponse(ctx.ResponseWriter, coalescedReq.resp, coalescedReq.err)
		}

		// Execute the request
		resp, err := p.executeRequest(proxyReq)
		p.coalescingPool.Complete(coalesceKey, resp, err)

		if err != nil {
			return err
		}
		defer resp.Body.Close()

		return p.writeResponse(ctx.ResponseWriter, resp)
	}

	// Execute request without coalescing
	resp, err := p.executeRequest(proxyReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return p.writeResponse(ctx.ResponseWriter, resp)
}

// Do performs the upstream request and returns the raw response.
func (p *OptimizedProxy) Do(ctx *RequestContext, target *config.UpstreamTarget) (*http.Response, error) {
	if p == nil {
		return nil, errors.New("proxy is nil")
	}
	if ctx == nil || ctx.Request == nil {
		return nil, errors.New("invalid request context")
	}
	if target == nil || strings.TrimSpace(target.Address) == "" {
		return nil, errors.New("invalid upstream target")
	}

	upstreamURL, err := p.buildUpstreamURL(ctx.Request, target.Address, ctx.Route)
	if err != nil {
		return nil, err
	}

	proxyReq, err := p.createProxyRequest(ctx.Request, upstreamURL)
	if err != nil {
		return nil, err
	}

	return p.executeRequest(proxyReq)
}

// WriteResponse writes the upstream response to the client with zero-copy.
func (p *OptimizedProxy) WriteResponse(w http.ResponseWriter, resp *http.Response) error {
	return p.writeResponse(w, resp)
}

// buildUpstreamURL constructs the upstream URL.
func (p *OptimizedProxy) buildUpstreamURL(req *http.Request, targetAddress string, route *config.Route) (*url.URL, error) {
	target := strings.TrimSpace(targetAddress)
	if target == "" {
		return nil, errors.New("empty target address")
	}

	if !strings.Contains(target, "://") {
		target = "http://" + target
	}

	base, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	if base.Host == "" {
		return nil, errors.New("missing host in target address")
	}

	// Build path
	upstreamPath := req.URL.Path
	if route != nil && route.StripPath {
		upstreamPath = p.stripPath(route, upstreamPath)
	}

	base.Path = p.joinPath(base.Path, upstreamPath)
	base.RawQuery = req.URL.RawQuery

	return base, nil
}

// createProxyRequest creates the outgoing proxy request.
func (p *OptimizedProxy) createProxyRequest(req *http.Request, upstreamURL *url.URL) (*http.Request, error) {
	ctx := req.Context()
	// Note: We don't set a timeout here because it would require managing the cancel function
	// The transport has its own timeouts configured

	proxyReq, err := http.NewRequestWithContext(ctx, req.Method, upstreamURL.String(), req.Body)
	if err != nil {
		return nil, err
	}

	// Copy headers
	copyHeadersOptimized(proxyReq.Header, req.Header)

	// Set forwarded headers
	p.appendForwardedHeaders(proxyReq, req)

	// Set host header
	if req.URL.Scheme == "https" {
		proxyReq.Header.Set("X-Forwarded-Proto", "https")
	} else {
		proxyReq.Header.Set("X-Forwarded-Proto", "http")
	}

	return proxyReq, nil
}

// executeRequest executes the HTTP request.
func (p *OptimizedProxy) executeRequest(req *http.Request) (*http.Response, error) {
	return p.transport.RoundTrip(req)
}

// writeResponse writes the response with zero-copy where possible.
func (p *OptimizedProxy) writeResponse(w http.ResponseWriter, resp *http.Response) error {
	// Copy headers
	copyHeadersOptimized(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	// Use buffer pool for copying
	buf := p.Get()
	defer p.Put(buf)

	n, err := io.CopyBuffer(w, resp.Body, buf)
	p.metrics.bytesTransferred.Add(uint64(n))

	return err
}

// serveCoalescedResponse serves a response from a coalesced request.
func (p *OptimizedProxy) serveCoalescedResponse(w http.ResponseWriter, resp *http.Response, err error) error {
	if err != nil {
		return err
	}
	if resp == nil {
		return errors.New("nil response from coalesced request")
	}

	// Clone the response for this waiter
	copyHeadersOptimized(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	// Read and write body
	buf := p.Get()
	defer p.Put(buf)

	// For coalesced requests, we need to read the body into a buffer
	// since the original response body can only be read once
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	_, err = w.Write(body)
	return err
}

// isCacheableRequest checks if a request can be coalesced.
func (p *OptimizedProxy) isCacheableRequest(req *http.Request) bool {
	// Only GET and HEAD requests are cacheable for coalescing
	method := strings.ToUpper(req.Method)
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}

	// Check Cache-Control headers
	cacheControl := req.Header.Get("Cache-Control")
	if strings.Contains(cacheControl, "no-cache") ||
		strings.Contains(cacheControl, "no-store") ||
		strings.Contains(cacheControl, "max-age=0") {
		return false
	}

	return true
}

// coalesceKey generates a key for request coalescing.
func (p *OptimizedProxy) coalesceKey(req *http.Request, upstreamURL *url.URL) string {
	var sb strings.Builder
	sb.Grow(256)
	sb.WriteString(strings.ToUpper(req.Method))
	sb.WriteByte('|')
	sb.WriteString(upstreamURL.String())

	// Include vary headers in key
	varyHeaders := []string{"Accept", "Accept-Encoding", "Accept-Language"}
	for _, header := range varyHeaders {
		if value := req.Header.Get(header); value != "" {
			sb.WriteByte('|')
			sb.WriteString(header)
			sb.WriteByte('=')
			sb.WriteString(value)
		}
	}

	return sb.String()
}

// stripPath removes the route prefix from the path.
func (p *OptimizedProxy) stripPath(route *config.Route, path string) string {
	if route == nil || len(route.Paths) == 0 {
		return path
	}

	// Find the best matching prefix
	var bestPrefix string
	for _, p := range route.Paths {
		if strings.HasPrefix(path, p) && len(p) > len(bestPrefix) {
			bestPrefix = p
		}
	}

	if bestPrefix == "" || bestPrefix == "/" {
		return path
	}

	rewritten := strings.TrimPrefix(path, bestPrefix)
	if rewritten == "" {
		return "/"
	}
	if !strings.HasPrefix(rewritten, "/") {
		return "/" + rewritten
	}
	return rewritten
}

// joinPath joins two URL paths.
func (p *OptimizedProxy) joinPath(base, req string) string {
	base = strings.TrimSuffix(base, "/")
	if base == "" {
		base = "/"
	}
	if req == "" || req == "/" {
		return base
	}
	req = strings.TrimPrefix(req, "/")
	return base + "/" + req
}

// appendForwardedHeaders adds X-Forwarded-* headers.
func (p *OptimizedProxy) appendForwardedHeaders(dst, src *http.Request) {
	if dst == nil || src == nil {
		return
	}

	// X-Forwarded-For
	remoteIP := p.clientIP(src.RemoteAddr)
	if remoteIP != "" {
		existing := strings.TrimSpace(src.Header.Get("X-Forwarded-For"))
		if existing != "" {
			dst.Header.Set("X-Forwarded-For", existing+", "+remoteIP)
		} else {
			dst.Header.Set("X-Forwarded-For", remoteIP)
		}
	}

	// X-Forwarded-Host
	dst.Header.Set("X-Forwarded-Host", src.Host)
}

// clientIP extracts the client IP from RemoteAddr.
func (p *OptimizedProxy) clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(remoteAddr)
}

// copyHeadersOptimized copies headers efficiently.
func copyHeadersOptimized(dst, src http.Header) {
	if dst == nil || src == nil {
		return
	}

	for key, values := range src {
		// Skip hop-by-hop headers
		if isHopByHopHeader(key) {
			continue
		}

		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

// isHopByHopHeader checks if a header is hop-by-hop.
func isHopByHopHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "proxy-connection", "keep-alive", "proxy-authenticate",
		"proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	}
	return false
}

// Metrics returns current proxy metrics.
func (p *OptimizedProxy) Metrics() ProxyMetricsSnapshot {
	if p == nil || p.metrics == nil {
		return ProxyMetricsSnapshot{}
	}
	return ProxyMetricsSnapshot{
		RequestsTotal:     p.metrics.requestsTotal.Load(),
		RequestsActive:    p.metrics.requestsActive.Load(),
		RequestsCoalesced: p.metrics.requestsCoalesced.Load(),
		BytesTransferred:  p.metrics.bytesTransferred.Load(),
		ErrorsTotal:       p.metrics.errorsTotal.Load(),
	}
}

// ProxyMetricsSnapshot is a point-in-time copy of proxy metrics.
type ProxyMetricsSnapshot struct {
	RequestsTotal     uint64
	RequestsActive    int64
	RequestsCoalesced uint64
	BytesTransferred  uint64
	ErrorsTotal       uint64
}

// Close cleans up resources.
func (p *OptimizedProxy) Close() error {
	if p.transport != nil {
		p.transport.CloseIdleConnections()
	}
	return nil
}
