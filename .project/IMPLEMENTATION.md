# API Cerberus — Implementation Guide

## IMPLEMENTATION.md

> Technical implementation details for building API Cerberus.
> This document is the engineering blueprint — read SPECIFICATION.md first.

---

## 1. Go Project Foundation

### 1.1 Module Setup

```go
// go.mod — minimal, curated external dependencies
module github.com/APICerberus/APICerberus

go 1.25
```

All functionality is implemented using Go standard library only. The following `stdlib` packages are the primary building blocks:

```
net/http           — HTTP server, reverse proxy, HTTP/2
net                — TCP/UDP listeners, raw connections
crypto/tls         — TLS termination, certificate management
crypto/rand        — Secure random generation
crypto/sha256      — API key hashing
crypto/hmac        — HMAC-SHA256 for JWT HS256
crypto/rsa         — RSA for JWT RS256
crypto/x509        — Certificate parsing, JWKS
encoding/json      — JSON marshal/unmarshal
encoding/base64    — Base64 for JWT, API keys
encoding/pem       — PEM certificate parsing
database/sql       — SQLite interface (via custom driver)
compress/gzip      — Response compression
compress/flate     — Deflate compression
net/url            — URL parsing, query manipulation
net/http/httputil  — ReverseProxy base
io                 — Stream handling
os                 — File operations, signals
os/signal          — SIGHUP for hot reload
sync               — Mutex, RWMutex, WaitGroup, Pool, Once, Map
sync/atomic        — Lock-free counters
context            — Request context, cancellation
time               — Timers, tickers, duration
fmt                — Formatted output
log/slog           — Structured logging (Go 1.21+)
regexp             — Route path matching, URL rewriting
strings            — String manipulation
strconv            — String/number conversion
sort               — Sorting algorithms
math/rand/v2       — Non-crypto random (load balancing)
hash/crc32         — Consistent hashing
hash/fnv           — Fast hashing for IP hash
embed              — Embed web assets in binary
path               — URL path operations
bytes              — Buffer operations
bufio              — Buffered I/O
errors             — Error wrapping
maps               — Map operations (Go 1.21+)
slices             — Slice operations (Go 1.21+)
cmp                — Comparison (Go 1.21+)
```

### 1.2 Entry Point

```go
// cmd/apicerberus/main.go
package main

import (
    "os"
    "github.com/APICerberus/APICerberus/internal/cli"
)

func main() {
    if err := cli.Run(os.Args[1:]); err != nil {
        os.Exit(1)
    }
}
```

### 1.3 Build System

```makefile
# Makefile
VERSION     := 0.0.1
GIT_COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME  := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS     := -s -w \
    -X github.com/APICerberus/APICerberus/internal/version.Version=$(VERSION) \
    -X github.com/APICerberus/APICerberus/internal/version.Commit=$(GIT_COMMIT) \
    -X github.com/APICerberus/APICerberus/internal/version.BuildTime=$(BUILD_TIME)

.PHONY: build build-web clean test lint

# Build web assets first, then Go binary
build: build-web
	CGO_ENABLED=1 go build -ldflags="$(LDFLAGS)" -o bin/apicerberus ./cmd/apicerberus

# CGO_ENABLED=1 is required for SQLite (pure Go SQLite driver uses CGO)
# Alternative: use modernc.org/sqlite which is pure Go — but that's an external dep
# Solution: We embed a minimal SQLite implementation via CGO with the C amalgamation
# bundled in the repo (see section 7 for SQLite strategy)

build-web:
	cd web && npm run build

# Cross-compilation
build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -ldflags="$(LDFLAGS)" -o bin/apicerberus-linux-amd64 ./cmd/apicerberus

build-darwin:
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build -ldflags="$(LDFLAGS)" -o bin/apicerberus-darwin-arm64 ./cmd/apicerberus

# Docker build (uses multi-stage)
docker:
	docker build -t apicerberus:$(VERSION) .

clean:
	rm -rf bin/ web/dist/

test:
	go test -race -cover ./...

lint:
	go vet ./...
```

### 1.4 Version Package

```go
// internal/version/version.go
package version

var (
    Version   = "0.0.1"
    Commit    = "unknown"
    BuildTime = "unknown"
)

type Info struct {
    Version   string `json:"version"`
    Commit    string `json:"commit"`
    BuildTime string `json:"build_time"`
    GoVersion string `json:"go_version"`
}

func Get() Info {
    return Info{
        Version:   Version,
        Commit:    Commit,
        BuildTime: BuildTime,
        GoVersion: runtime.Version(),
    }
}
```

---

## 2. Configuration System

### 2.1 Custom YAML Parser

Since we can't use `gopkg.in/yaml.v3`, we implement a minimal YAML parser that handles the subset we need:

```go
// internal/pkg/yaml/parser.go
package yaml

// Strategy: Our config YAML is well-structured and predictable.
// We parse a USEFUL SUBSET of YAML, not full YAML spec.
//
// Supported:
// - Key-value pairs (string, int, float, bool)
// - Nested maps (indentation-based)
// - Lists (- item syntax)
// - Quoted strings (single and double)
// - Comments (# ...)
// - Multi-line strings (| and >)
// - Anchors/aliases NOT supported (not needed)
//
// Parse flow:
// 1. Tokenize (line-by-line, track indentation)
// 2. Build node tree (map, sequence, scalar)
// 3. Marshal to Go structs via reflection

type NodeKind int

const (
    NodeMap NodeKind = iota
    NodeSequence
    NodeScalar
)

type Node struct {
    Kind     NodeKind
    Value    string            // For scalars
    Map      map[string]*Node  // For maps
    Sequence []*Node           // For sequences
    Line     int               // Source line for error reporting
}

// Unmarshal parses YAML bytes into a Go struct
func Unmarshal(data []byte, v any) error {
    root, err := parse(data)
    if err != nil {
        return err
    }
    return decode(root, reflect.ValueOf(v))
}

// Marshal serializes a Go struct to YAML bytes
func Marshal(v any) ([]byte, error) {
    // Reflection-based encoder
    // Walk struct fields, emit key: value with proper indentation
}
```

**Implementation approach**: The YAML parser is ~800-1000 lines. It handles indentation-based nesting, type coercion (string→int/bool/duration), and struct tag mapping (`yaml:"field_name"`). Edge cases like multi-line strings and empty values are handled.

### 2.2 Configuration Types

```go
// internal/config/config.go
package config

import "time"

type Config struct {
    Gateway   GatewayConfig   `yaml:"gateway"`
    Admin     AdminConfig     `yaml:"admin"`
    Portal    PortalConfig    `yaml:"portal"`
    Services  []Service       `yaml:"services"`
    Routes    []Route         `yaml:"routes"`
    Upstreams []Upstream      `yaml:"upstreams"`
    Consumers []Consumer      `yaml:"consumers"`
    Plugins   []PluginConfig  `yaml:"global_plugins"`
    Billing   BillingConfig   `yaml:"billing"`
    AuditLog  AuditLogConfig  `yaml:"audit_log"`
    Logging   LoggingConfig   `yaml:"logging"`
    Cluster   ClusterConfig   `yaml:"cluster"`
    Store     StoreConfig     `yaml:"store"`
    Alerts    []AlertConfig   `yaml:"alerts"`
}

type GatewayConfig struct {
    HTTPAddr       string        `yaml:"http_addr"`       // ":8080"
    HTTPSAddr      string        `yaml:"https_addr"`      // ":8443"
    GRPCAddr       string        `yaml:"grpc_addr"`       // ":9090"
    TLS            TLSConfig     `yaml:"tls"`
    ReadTimeout    time.Duration `yaml:"read_timeout"`    // default 30s
    WriteTimeout   time.Duration `yaml:"write_timeout"`   // default 30s
    IdleTimeout    time.Duration `yaml:"idle_timeout"`    // default 120s
    MaxHeaderBytes int           `yaml:"max_header_bytes"` // default 1MB
    MaxBodyBytes   int64         `yaml:"max_body_bytes"`  // default 10MB
}

type AdminConfig struct {
    Addr      string `yaml:"addr"`       // ":9876"
    APIKey    string `yaml:"api_key"`
    UIEnabled bool   `yaml:"ui_enabled"` // default true
    UIPath    string `yaml:"ui_path"`    // "/dashboard"
}

type PortalConfig struct {
    Enabled      bool            `yaml:"enabled"`
    Addr         string          `yaml:"addr"`         // ":9877"
    PathPrefix   string          `yaml:"path_prefix"`  // "/portal"
    Session      SessionConfig   `yaml:"session"`
    Registration RegistrationConfig `yaml:"registration"`
    Password     PasswordConfig  `yaml:"password"`
}

type Service struct {
    ID             string        `yaml:"id" json:"id"`
    Name           string        `yaml:"name" json:"name"`
    Protocol       string        `yaml:"protocol" json:"protocol"`       // http | grpc | graphql
    Upstream       string        `yaml:"upstream" json:"upstream"`
    PathPrefix     string        `yaml:"path_prefix" json:"path_prefix"`
    Retries        int           `yaml:"retries" json:"retries"`
    ConnectTimeout time.Duration `yaml:"connect_timeout" json:"connect_timeout"`
    ReadTimeout    time.Duration `yaml:"read_timeout" json:"read_timeout"`
    WriteTimeout   time.Duration `yaml:"write_timeout" json:"write_timeout"`
    GraphQL        *GraphQLConfig `yaml:"graphql,omitempty" json:"graphql,omitempty"`
}

type Route struct {
    ID           string         `yaml:"id" json:"id"`
    Name         string         `yaml:"name" json:"name"`
    Service      string         `yaml:"service" json:"service"`
    Hosts        []string       `yaml:"hosts" json:"hosts"`
    Paths        []string       `yaml:"paths" json:"paths"`
    Methods      []string       `yaml:"methods" json:"methods"`
    Headers      map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
    StripPath    bool           `yaml:"strip_path" json:"strip_path"`
    PreserveHost bool           `yaml:"preserve_host" json:"preserve_host"`
    Plugins      []PluginConfig `yaml:"plugins" json:"plugins"`
    Priority     int            `yaml:"priority" json:"priority"`   // Higher = matched first
}

type Upstream struct {
    ID           string           `yaml:"id" json:"id"`
    Name         string           `yaml:"name" json:"name"`
    Algorithm    string           `yaml:"algorithm" json:"algorithm"`
    Targets      []UpstreamTarget `yaml:"targets" json:"targets"`
    HealthCheck  HealthCheckConfig `yaml:"health_check" json:"health_check"`
    CircuitBreaker *CircuitBreakerConfig `yaml:"circuit_breaker,omitempty" json:"circuit_breaker,omitempty"`
}

type UpstreamTarget struct {
    ID      string `yaml:"id" json:"id"`
    Address string `yaml:"address" json:"address"` // host:port
    Weight  int    `yaml:"weight" json:"weight"`   // default 100
}

type StoreConfig struct {
    Type        string        `yaml:"type"`         // "sqlite"
    Path        string        `yaml:"path"`         // "/var/apicerberus/data.db"
    WALMode     bool          `yaml:"wal_mode"`     // default true
    BusyTimeout time.Duration `yaml:"busy_timeout"` // default 5s
    MaxOpenConns int          `yaml:"max_open_conns"` // default 25
}

type BillingConfig struct {
    Enabled             bool              `yaml:"enabled"`
    DefaultCost         int64             `yaml:"default_cost"`       // default 1
    RouteCosts          map[string]int64  `yaml:"route_costs"`
    MethodMultipliers   map[string]float64 `yaml:"method_multipliers"`
    TestModeEnabled     bool              `yaml:"test_mode_enabled"`  // default true
    ZeroBalanceAction   string            `yaml:"zero_balance_action"` // reject | allow_with_flag
    ZeroBalanceResponse *CustomResponse   `yaml:"zero_balance_response"`
    LowBalanceThreshold int64            `yaml:"low_balance_threshold"`
    LowBalanceWebhook   string           `yaml:"low_balance_webhook"`
    SelfPurchase        SelfPurchaseConfig `yaml:"self_purchase"`
}

type AuditLogConfig struct {
    Enabled              bool     `yaml:"enabled"`
    StoreRequestHeaders  bool     `yaml:"store_request_headers"`
    StoreRequestBody     bool     `yaml:"store_request_body"`
    StoreResponseHeaders bool     `yaml:"store_response_headers"`
    StoreResponseBody    bool     `yaml:"store_response_body"`
    MaxRequestBodySize   int      `yaml:"max_request_body_size"`  // bytes
    MaxResponseBodySize  int      `yaml:"max_response_body_size"` // bytes
    MaskHeaders          []string `yaml:"mask_headers"`
    MaskBodyFields       []string `yaml:"mask_body_fields"`
    MaskReplacement      string   `yaml:"mask_replacement"`       // "***REDACTED***"
    Retention            RetentionConfig `yaml:"retention"`
    Archive              ArchiveConfig   `yaml:"archive"`
}
```

### 2.3 Config Loading Pipeline

```
1. Load YAML file (custom parser)
2. Apply environment variable overrides (APICERBERUS_*)
3. Set defaults for missing values
4. Validate (required fields, value ranges, format checks)
5. Generate IDs for entities without explicit IDs
6. Build runtime config (resolve references: route→service→upstream)
7. Return immutable config snapshot
```

```go
// internal/config/loader.go
func Load(path string) (*Config, error) {
    // 1. Read file
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("read config: %w", err)
    }
    
    // 2. Parse YAML
    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("parse config: %w", err)
    }
    
    // 3. Apply env overrides
    applyEnvOverrides(&cfg)
    
    // 4. Set defaults
    setDefaults(&cfg)
    
    // 5. Validate
    if err := validate(&cfg); err != nil {
        return nil, fmt.Errorf("validate config: %w", err)
    }
    
    // 6. Generate missing IDs
    generateIDs(&cfg)
    
    return &cfg, nil
}

// internal/config/env.go
func applyEnvOverrides(cfg *Config) {
    // Convention: APICERBERUS_SECTION_FIELD
    // Uses reflection to map env vars to struct fields
    // e.g., APICERBERUS_GATEWAY_HTTP_ADDR → cfg.Gateway.HTTPAddr
    applyEnvToStruct("APICERBERUS", reflect.ValueOf(cfg).Elem())
}

// internal/config/watcher.go  
func Watch(path string, onChange func(*Config)) {
    // Strategy: poll file stat every 2 seconds
    // On modification time change → re-parse → call onChange
    // Also handle SIGHUP signal → re-parse → call onChange
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGHUP)
    
    ticker := time.NewTicker(2 * time.Second)
    var lastModTime time.Time
    
    for {
        select {
        case <-sigCh:
            // SIGHUP received — reload
        case <-ticker.C:
            info, _ := os.Stat(path)
            if info != nil && info.ModTime() != lastModTime {
                lastModTime = info.ModTime()
                // reload
            }
        }
    }
}
```

---

## 3. Gateway Core (Reverse Proxy)

### 3.1 Gateway Server

```go
// internal/gateway/gateway.go
package gateway

type Gateway struct {
    config    *config.Config
    router    *Router
    pipeline  *pipeline.Pipeline
    proxy     *Proxy
    health    *health.Checker
    analytics *analytics.Engine
    store     *store.Store
    
    httpServer  *http.Server
    httpsServer *http.Server
    grpcServer  *http.Server // gRPC runs on HTTP/2
    
    mu sync.RWMutex // protects config hot-reload
}

func New(cfg *config.Config, st *store.Store) (*Gateway, error) {
    g := &Gateway{
        config: cfg,
        store:  st,
    }
    
    // Initialize subsystems
    g.router = NewRouter(cfg.Routes, cfg.Services)
    g.proxy = NewProxy(cfg.Upstreams)
    g.health = health.NewChecker(cfg.Upstreams)
    g.analytics = analytics.NewEngine(cfg)
    g.pipeline = pipeline.New(cfg, st)
    
    return g, nil
}

func (g *Gateway) Start(ctx context.Context) error {
    errCh := make(chan error, 3)
    
    // Start health checker
    go g.health.Start(ctx)
    
    // Start analytics engine
    go g.analytics.Start(ctx)
    
    // HTTP listener
    if g.config.Gateway.HTTPAddr != "" {
        go func() {
            errCh <- g.listenHTTP(ctx)
        }()
    }
    
    // HTTPS listener
    if g.config.Gateway.HTTPSAddr != "" {
        go func() {
            errCh <- g.listenHTTPS(ctx)
        }()
    }
    
    // gRPC listener
    if g.config.Gateway.GRPCAddr != "" {
        go func() {
            errCh <- g.listenGRPC(ctx)
        }()
    }
    
    return <-errCh
}

// ServeHTTP is the main request handler
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    startTime := time.Now()
    
    // 1. Create request context
    ctx := pipeline.NewRequestContext(r, w, startTime)
    
    // 2. Route matching
    route, service, err := g.router.Match(r)
    if err != nil {
        g.writeError(w, http.StatusNotFound, "no_route", "No route matched")
        return
    }
    ctx.Route = route
    ctx.Service = service
    
    // 3. Resolve upstream
    upstream := g.proxy.GetUpstream(service.Upstream)
    if upstream == nil {
        g.writeError(w, http.StatusBadGateway, "no_upstream", "Upstream not found")
        return
    }
    ctx.Upstream = upstream
    
    // 4. Execute plugin pipeline (auth, rate limit, transform, etc.)
    if err := g.pipeline.Execute(ctx); err != nil {
        // Pipeline already wrote error response
        return
    }
    
    // 5. Select target via load balancer
    target, err := upstream.Balancer.Next(ctx)
    if err != nil {
        g.writeError(w, http.StatusBadGateway, "no_target", "No healthy upstream target")
        return
    }
    ctx.UpstreamTarget = target
    
    // 6. Proxy request to upstream
    g.proxy.Forward(ctx, target)
    
    // 7. Execute post-proxy plugins (response transform, analytics, logging)
    g.pipeline.ExecutePostProxy(ctx)
}
```

### 3.2 Router (Radix Tree)

```go
// internal/gateway/router.go
package gateway

// Router uses a radix tree for fast path matching.
// Routes are indexed by host → method → path for O(log n) lookup.

type Router struct {
    // Host-based routing: map[host]*MethodTree
    hosts map[string]*MethodTree
    
    // Default tree for routes without host restriction
    defaultTree *MethodTree
    
    // Compiled regex routes (evaluated after exact/prefix match)
    regexRoutes []*compiledRoute
    
    mu sync.RWMutex
}

type MethodTree struct {
    // Per-method radix trees
    trees map[string]*radixNode // GET, POST, PUT, DELETE, PATCH, etc.
    any   *radixNode            // Routes matching all methods
}

type radixNode struct {
    path     string
    children []*radixNode
    route    *config.Route    // nil for intermediate nodes
    service  *config.Service  // resolved service reference
    isWild   bool             // /path/* wildcard
    paramKey string           // /path/:id parameter
}

func (r *Router) Match(req *http.Request) (*config.Route, *config.Service, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    
    host := stripPort(req.Host)
    method := req.Method
    path := req.URL.Path
    
    // 1. Try host-specific routes
    if tree, ok := r.hosts[host]; ok {
        if route, svc := tree.match(method, path); route != nil {
            return route, svc, nil
        }
    }
    
    // 2. Try default routes (no host restriction)
    if route, svc := r.defaultTree.match(method, path); route != nil {
        return route, svc, nil
    }
    
    // 3. Try regex routes
    for _, cr := range r.regexRoutes {
        if cr.matches(host, method, path) {
            return cr.route, cr.service, nil
        }
    }
    
    return nil, nil, ErrNoRouteMatched
}
```

### 3.3 Reverse Proxy

```go
// internal/gateway/proxy.go
package gateway

type Proxy struct {
    upstreams map[string]*UpstreamPool
    transport *http.Transport
    bufPool   sync.Pool // buffer pool for body copying
}

func NewProxy(upstreams []config.Upstream) *Proxy {
    p := &Proxy{
        upstreams: make(map[string]*UpstreamPool),
        transport: &http.Transport{
            MaxIdleConns:        1000,
            MaxIdleConnsPerHost: 100,
            IdleConnTimeout:     90 * time.Second,
            TLSHandshakeTimeout: 10 * time.Second,
            // HTTP/2 support
            ForceAttemptHTTP2: true,
        },
        bufPool: sync.Pool{
            New: func() any {
                buf := make([]byte, 32*1024) // 32KB buffers
                return &buf
            },
        },
    }
    
    for _, u := range upstreams {
        p.upstreams[u.Name] = NewUpstreamPool(u)
    }
    
    return p
}

func (p *Proxy) Forward(ctx *pipeline.RequestContext, target *config.UpstreamTarget) {
    startTime := time.Now()
    
    // Build upstream request
    upstreamURL := fmt.Sprintf("http://%s%s", target.Address, ctx.Request.URL.Path)
    if ctx.Route.StripPath {
        upstreamURL = fmt.Sprintf("http://%s%s", target.Address, 
            stripPrefix(ctx.Request.URL.Path, ctx.Route.Paths[0]))
    }
    
    proxyReq, err := http.NewRequestWithContext(
        ctx.Request.Context(),
        ctx.Request.Method,
        upstreamURL,
        ctx.Request.Body,
    )
    if err != nil {
        ctx.SetError(http.StatusBadGateway, "proxy_error", err.Error())
        return
    }
    
    // Copy headers
    copyHeaders(proxyReq.Header, ctx.Request.Header)
    
    // Set proxy headers
    proxyReq.Header.Set("X-Forwarded-For", ctx.ClientIP)
    proxyReq.Header.Set("X-Forwarded-Proto", ctx.Scheme)
    proxyReq.Header.Set("X-Request-ID", ctx.CorrelationID)
    
    if !ctx.Route.PreserveHost {
        proxyReq.Host = target.Address
    }
    
    // Execute request
    resp, err := p.transport.RoundTrip(proxyReq)
    if err != nil {
        ctx.SetError(http.StatusBadGateway, "upstream_error", err.Error())
        return
    }
    defer resp.Body.Close()
    
    ctx.UpstreamLatency = time.Since(startTime)
    ctx.Response.StatusCode = resp.StatusCode
    ctx.Response.Headers = resp.Header
    
    // Copy response headers
    copyHeaders(ctx.ResponseWriter.Header(), resp.Header)
    
    // Write status code
    ctx.ResponseWriter.WriteHeader(resp.StatusCode)
    
    // Stream response body (using pooled buffer)
    bufPtr := p.bufPool.Get().(*[]byte)
    defer p.bufPool.Put(bufPtr)
    
    ctx.Response.Size, _ = io.CopyBuffer(ctx.ResponseWriter, resp.Body, *bufPtr)
}
```

### 3.4 WebSocket Proxy

```go
// internal/gateway/websocket.go
func (p *Proxy) ForwardWebSocket(ctx *pipeline.RequestContext, target *config.UpstreamTarget) {
    // Detect WebSocket upgrade
    if !isWebSocketUpgrade(ctx.Request) {
        return // not a websocket request
    }
    
    // Hijack the connection
    hijacker, ok := ctx.ResponseWriter.(http.Hijacker)
    if !ok {
        ctx.SetError(http.StatusInternalServerError, "ws_error", "hijack not supported")
        return
    }
    
    clientConn, clientBuf, err := hijacker.Hijack()
    if err != nil {
        return
    }
    defer clientConn.Close()
    
    // Connect to upstream
    upstreamURL := fmt.Sprintf("ws://%s%s", target.Address, ctx.Request.URL.Path)
    upstreamConn, err := net.DialTimeout("tcp", target.Address, 10*time.Second)
    if err != nil {
        return
    }
    defer upstreamConn.Close()
    
    // Send upgrade request to upstream
    upgradeReq := buildUpgradeRequest(ctx.Request, target)
    upgradeReq.Write(upstreamConn)
    
    // Bidirectional copy
    errCh := make(chan error, 2)
    go func() {
        _, err := io.Copy(upstreamConn, clientBuf)
        errCh <- err
    }()
    go func() {
        _, err := io.Copy(clientConn, upstreamConn)
        errCh <- err
    }()
    
    <-errCh // Wait for either direction to close
}
```

---

## 4. Plugin Pipeline

### 4.1 Pipeline Architecture

```go
// internal/pipeline/pipeline.go
package pipeline

type Pipeline struct {
    globalPlugins []plugin.Plugin  // Execute on every request
    routePlugins  map[string][]plugin.Plugin // Per-route plugins
    registry      *Registry
    store         *store.Store
    config        *config.Config
}

func New(cfg *config.Config, st *store.Store) *Pipeline {
    p := &Pipeline{
        routePlugins: make(map[string][]plugin.Plugin),
        registry:     NewRegistry(),
        store:        st,
        config:       cfg,
    }
    
    // Register all built-in plugins
    p.registry.Register(&plugin.AuthAPIKey{Store: st})
    p.registry.Register(&plugin.AuthJWT{})
    p.registry.Register(&plugin.RateLimit{})
    p.registry.Register(&plugin.CORS{})
    p.registry.Register(&plugin.IPRestrict{})
    p.registry.Register(&plugin.RequestTransform{})
    p.registry.Register(&plugin.ResponseTransform{})
    p.registry.Register(&plugin.CorrelationID{})
    p.registry.Register(&plugin.Logger{})
    p.registry.Register(&plugin.Analytics{})
    p.registry.Register(&plugin.Compression{})
    p.registry.Register(&plugin.Cache{})
    p.registry.Register(&plugin.CircuitBreaker{})
    p.registry.Register(&plugin.Retry{})
    p.registry.Register(&plugin.Timeout{})
    p.registry.Register(&plugin.RequestSizeLimit{})
    p.registry.Register(&plugin.URLRewrite{})
    p.registry.Register(&plugin.BotDetect{})
    p.registry.Register(&plugin.RequestValidator{})
    p.registry.Register(&plugin.GraphQLGuard{})
    // ... additional built-in plugins
    
    // Build global plugin chain
    p.buildGlobalChain(cfg.Plugins)
    
    // Build per-route plugin chains
    for _, route := range cfg.Routes {
        p.buildRouteChain(route)
    }
    
    return p
}

func (p *Pipeline) Execute(ctx *RequestContext) error {
    // Phase 1: Pre-auth global plugins
    for _, pl := range p.globalPlugins {
        if pl.Phase() == plugin.PhasePreAuth {
            if err := pl.Execute(ctx); err != nil {
                return err
            }
            if ctx.Aborted {
                return nil // Plugin wrote response, stop pipeline
            }
        }
    }
    
    // Phase 2: Auth plugins (route-specific)
    routePlugins := p.routePlugins[ctx.Route.ID]
    for _, pl := range routePlugins {
        if pl.Phase() == plugin.PhaseAuth {
            if err := pl.Execute(ctx); err != nil {
                return err
            }
            if ctx.Aborted {
                return nil
            }
        }
    }
    
    // Phase 2.5: User validation (credit check, permission check)
    if ctx.User != nil {
        if err := p.checkUserPermission(ctx); err != nil {
            return err
        }
        if err := p.checkUserCredits(ctx); err != nil {
            return err
        }
    }
    
    // Phase 3: Pre-proxy plugins
    allPlugins := mergePlugins(p.globalPlugins, routePlugins)
    for _, pl := range allPlugins {
        if pl.Phase() == plugin.PhasePreProxy {
            if err := pl.Execute(ctx); err != nil {
                return err
            }
            if ctx.Aborted {
                return nil
            }
        }
    }
    
    return nil
}

func (p *Pipeline) ExecutePostProxy(ctx *RequestContext) {
    allPlugins := mergePlugins(p.globalPlugins, p.routePlugins[ctx.Route.ID])
    
    // Phase 5: Post-proxy plugins (response transform, analytics, logging)
    for _, pl := range allPlugins {
        if pl.Phase() == plugin.PhasePostProxy {
            pl.Execute(ctx) // Post-proxy errors are logged but don't abort
        }
    }
    
    // Always: deduct credits and write audit log
    if ctx.User != nil && !ctx.IsTestKey {
        p.deductCredits(ctx)
    }
    p.writeAuditLog(ctx)
}
```

### 4.2 Request Context

```go
// internal/pipeline/context.go
package pipeline

type RequestContext struct {
    // HTTP
    Request        *http.Request
    ResponseWriter http.ResponseWriter
    
    // Routing
    Route          *config.Route
    Service        *config.Service
    Upstream       *UpstreamPool
    UpstreamTarget *config.UpstreamTarget
    
    // Auth
    Consumer       *config.Consumer  // Legacy consumer (from YAML config)
    User           *store.User       // Multi-tenant user (from SQLite)
    APIKey         *store.APIKey     // Which API key was used
    IsTestKey      bool              // ck_test_ key — no credit deduction
    
    // Request state
    ClientIP       string
    Scheme         string            // http | https
    CorrelationID  string
    StartTime      time.Time
    
    // Response state
    Response       ResponseData
    UpstreamLatency time.Duration
    GatewayLatency  time.Duration
    
    // Credit
    CreditCost     int64
    CreditBalance  int64             // Balance after deduction
    
    // Pipeline control
    Aborted        bool
    AbortReason    string
    
    // Plugin data exchange
    Metadata       map[string]any
    
    // Body capture (for audit log)
    CapturedRequestBody  []byte
    CapturedResponseBody []byte
}

type ResponseData struct {
    StatusCode int
    Headers    http.Header
    Size       int64
    Body       []byte // Captured if audit logging enabled
}
```

### 4.3 Plugin Interface

```go
// internal/plugin/plugin.go
package plugin

type Phase int

const (
    PhasePreAuth  Phase = 10
    PhaseAuth     Phase = 20
    PhasePreProxy Phase = 30
    PhaseProxy    Phase = 40
    PhasePostProxy Phase = 50
)

type Plugin interface {
    Name() string
    Phase() Phase
    Priority() int // Lower = executes first within same phase
    Execute(ctx *pipeline.RequestContext) error
    Configure(cfg map[string]any) error
}

// BasePlugin provides shared functionality
type BasePlugin struct {
    name     string
    phase    Phase
    priority int
    config   map[string]any
}

func (b *BasePlugin) Name() string   { return b.name }
func (b *BasePlugin) Phase() Phase   { return b.phase }
func (b *BasePlugin) Priority() int  { return b.priority }
```

---

## 5. Authentication Plugins

### 5.1 API Key Authentication

```go
// internal/plugin/auth_apikey.go
package plugin

type AuthAPIKey struct {
    BasePlugin
    store    *store.Store
    keyNames []string // Header names to check: ["X-API-Key", "Authorization"]
}

func (a *AuthAPIKey) Execute(ctx *pipeline.RequestContext) error {
    // 1. Extract API key from request
    key := a.extractKey(ctx.Request)
    if key == "" {
        ctx.Aborted = true
        ctx.AbortReason = "missing_api_key"
        writeJSON(ctx.ResponseWriter, 401, map[string]string{
            "error":   "unauthorized",
            "message": "API key is required",
        })
        return nil
    }
    
    // 2. Hash the key and look up in store
    keyHash := sha256Hex(key)
    apiKey, err := a.store.APIKeys().FindByHash(keyHash)
    if err != nil || apiKey == nil {
        ctx.Aborted = true
        ctx.AbortReason = "invalid_api_key"
        writeJSON(ctx.ResponseWriter, 401, map[string]string{
            "error":   "unauthorized",
            "message": "Invalid API key",
        })
        return nil
    }
    
    // 3. Check key status
    if apiKey.Status != "active" {
        ctx.Aborted = true
        ctx.AbortReason = "revoked_api_key"
        writeJSON(ctx.ResponseWriter, 401, map[string]string{
            "error":   "unauthorized",
            "message": "API key has been revoked",
        })
        return nil
    }
    
    // 4. Check expiration
    if apiKey.ExpiresAt != nil && time.Now().After(*apiKey.ExpiresAt) {
        ctx.Aborted = true
        ctx.AbortReason = "expired_api_key"
        writeJSON(ctx.ResponseWriter, 401, map[string]string{
            "error":   "unauthorized",
            "message": "API key has expired",
        })
        return nil
    }
    
    // 5. Load user
    user, err := a.store.Users().FindByID(apiKey.UserID)
    if err != nil || user == nil || user.Status != "active" {
        ctx.Aborted = true
        ctx.AbortReason = "user_inactive"
        writeJSON(ctx.ResponseWriter, 403, map[string]string{
            "error":   "forbidden",
            "message": "User account is not active",
        })
        return nil
    }
    
    // 6. Set context
    ctx.User = user
    ctx.APIKey = apiKey
    ctx.IsTestKey = strings.HasPrefix(key, "ck_test_")
    
    // 7. Update last used
    go a.store.APIKeys().UpdateLastUsed(apiKey.ID, ctx.ClientIP)
    
    return nil
}

func (a *AuthAPIKey) extractKey(r *http.Request) string {
    // Check configured header names
    for _, name := range a.keyNames {
        if v := r.Header.Get(name); v != "" {
            // Handle "Bearer <key>" format
            if strings.HasPrefix(v, "Bearer ") {
                return strings.TrimPrefix(v, "Bearer ")
            }
            return v
        }
    }
    // Check query parameter
    if v := r.URL.Query().Get("apikey"); v != "" {
        return v
    }
    return ""
}
```

### 5.2 JWT Authentication

```go
// internal/plugin/auth_jwt.go
package plugin

type AuthJWT struct {
    BasePlugin
    algorithms     []string          // ["RS256", "HS256"]
    hmacSecret     []byte            // For HS256
    rsaPublicKeys  []*rsa.PublicKey  // For RS256 (from JWKS)
    jwksURL        string
    consumerClaim  string            // "sub"
    claimsToHeaders map[string]string
    requiredClaims []string
    audience       string
    issuer         string
    clockSkew      time.Duration
}

func (j *AuthJWT) Execute(ctx *pipeline.RequestContext) error {
    // 1. Extract JWT from Authorization header
    token := extractBearerToken(ctx.Request)
    if token == "" {
        ctx.Aborted = true
        writeJSON(ctx.ResponseWriter, 401, map[string]string{
            "error": "unauthorized", "message": "JWT token required",
        })
        return nil
    }
    
    // 2. Parse JWT (header.payload.signature)
    parts := strings.Split(token, ".")
    if len(parts) != 3 {
        ctx.Aborted = true
        writeJSON(ctx.ResponseWriter, 401, map[string]string{
            "error": "unauthorized", "message": "Malformed JWT",
        })
        return nil
    }
    
    // 3. Decode header to get algorithm
    header, err := decodeJWTSegment(parts[0])
    if err != nil {
        return j.reject(ctx, "Invalid JWT header")
    }
    
    alg, _ := header["alg"].(string)
    
    // 4. Verify signature based on algorithm
    switch alg {
    case "HS256":
        if !j.verifyHS256(parts, j.hmacSecret) {
            return j.reject(ctx, "Invalid JWT signature")
        }
    case "RS256":
        if !j.verifyRS256(parts) {
            return j.reject(ctx, "Invalid JWT signature")
        }
    default:
        return j.reject(ctx, "Unsupported algorithm: "+alg)
    }
    
    // 5. Decode and validate claims
    claims, err := decodeJWTSegment(parts[1])
    if err != nil {
        return j.reject(ctx, "Invalid JWT payload")
    }
    
    // Check expiration
    if exp, ok := claims["exp"].(float64); ok {
        if time.Now().After(time.Unix(int64(exp), 0).Add(j.clockSkew)) {
            return j.reject(ctx, "JWT expired")
        }
    }
    
    // Check issuer
    if j.issuer != "" {
        if iss, _ := claims["iss"].(string); iss != j.issuer {
            return j.reject(ctx, "Invalid issuer")
        }
    }
    
    // Check audience
    if j.audience != "" {
        if !j.checkAudience(claims) {
            return j.reject(ctx, "Invalid audience")
        }
    }
    
    // Check required claims
    for _, c := range j.requiredClaims {
        if _, ok := claims[c]; !ok {
            return j.reject(ctx, "Missing required claim: "+c)
        }
    }
    
    // 6. Map claims to headers
    for claim, header := range j.claimsToHeaders {
        if v, ok := claims[claim]; ok {
            ctx.Request.Header.Set(header, fmt.Sprint(v))
        }
    }
    
    // 7. Set consumer from claim
    if j.consumerClaim != "" {
        if sub, ok := claims[j.consumerClaim].(string); ok {
            ctx.Metadata["jwt_subject"] = sub
        }
    }
    
    return nil
}
```

### 5.3 JWT Cryptography (Pure Go)

```go
// internal/pkg/jwt/hs256.go
package jwt

func VerifyHS256(signingInput string, signature []byte, secret []byte) bool {
    mac := hmac.New(sha256.New, secret)
    mac.Write([]byte(signingInput))
    expected := mac.Sum(nil)
    return hmac.Equal(signature, expected)
}

// internal/pkg/jwt/rs256.go
func VerifyRS256(signingInput string, signature []byte, publicKey *rsa.PublicKey) bool {
    hash := sha256.Sum256([]byte(signingInput))
    err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hash[:], signature)
    return err == nil
}

// JWKS fetching (for RS256 key rotation)
func FetchJWKS(url string) ([]*rsa.PublicKey, error) {
    resp, err := http.Get(url)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var jwks struct {
        Keys []struct {
            Kty string `json:"kty"`
            N   string `json:"n"`   // RSA modulus (base64url)
            E   string `json:"e"`   // RSA exponent (base64url)
            Kid string `json:"kid"`
        } `json:"keys"`
    }
    
    json.NewDecoder(resp.Body).Decode(&jwks)
    
    var keys []*rsa.PublicKey
    for _, k := range jwks.Keys {
        if k.Kty == "RSA" {
            pub, err := parseRSAPublicKey(k.N, k.E)
            if err == nil {
                keys = append(keys, pub)
            }
        }
    }
    return keys, nil
}
```

---

## 6. Rate Limiting

### 6.1 Rate Limiter Interface

```go
// internal/plugin/rate_limit.go
package plugin

type RateLimiter interface {
    Allow(key string) (allowed bool, remaining int64, resetAt time.Time)
    Configure(cfg RateLimitConfig) error
}

type RateLimitConfig struct {
    Algorithm string `json:"algorithm"` // token_bucket, sliding_window, fixed_window, leaky_bucket
    // Token Bucket
    Capacity   int64 `json:"capacity"`
    RefillRate int64 `json:"refill_rate"` // per second
    // Window-based
    Requests  int64         `json:"requests"`
    Window    time.Duration `json:"window"`
    Precision time.Duration `json:"precision"` // for sliding window
    // Leaky Bucket
    LeakRate int64 `json:"leak_rate"` // per second
    // Common
    Scope string `json:"scope"` // global, consumer, ip, route, consumer+route
}
```

### 6.2 Token Bucket

```go
// internal/plugin/rate_limit_token_bucket.go
type TokenBucket struct {
    buckets sync.Map // map[string]*bucket
}

type bucket struct {
    tokens    float64
    capacity  float64
    refillRate float64 // tokens per second
    lastRefill time.Time
    mu         sync.Mutex
}

func (tb *TokenBucket) Allow(key string) (bool, int64, time.Time) {
    val, _ := tb.buckets.LoadOrStore(key, &bucket{
        tokens:     float64(tb.capacity),
        capacity:   float64(tb.capacity),
        refillRate: float64(tb.refillRate),
        lastRefill: time.Now(),
    })
    
    b := val.(*bucket)
    b.mu.Lock()
    defer b.mu.Unlock()
    
    // Refill tokens based on elapsed time
    now := time.Now()
    elapsed := now.Sub(b.lastRefill).Seconds()
    b.tokens = min(b.capacity, b.tokens+(elapsed*b.refillRate))
    b.lastRefill = now
    
    if b.tokens >= 1 {
        b.tokens--
        return true, int64(b.tokens), time.Time{}
    }
    
    // Calculate when next token will be available
    waitTime := time.Duration((1 - b.tokens) / b.refillRate * float64(time.Second))
    return false, 0, now.Add(waitTime)
}
```

### 6.3 Sliding Window

```go
// internal/plugin/rate_limit_sliding_window.go
type SlidingWindow struct {
    windows sync.Map // map[string]*slidingWindowState
    requests int64
    window   time.Duration
    precision time.Duration
}

type slidingWindowState struct {
    counters []int64     // sub-window counters
    times    []time.Time // sub-window timestamps
    mu       sync.Mutex
}

func (sw *SlidingWindow) Allow(key string) (bool, int64, time.Time) {
    val, _ := sw.windows.LoadOrStore(key, &slidingWindowState{})
    state := val.(*slidingWindowState)
    
    state.mu.Lock()
    defer state.mu.Unlock()
    
    now := time.Now()
    windowStart := now.Add(-sw.window)
    
    // Remove expired sub-windows
    var total int64
    validIdx := 0
    for i, t := range state.times {
        if t.After(windowStart) {
            if validIdx != i {
                state.times[validIdx] = state.times[i]
                state.counters[validIdx] = state.counters[i]
            }
            total += state.counters[validIdx]
            validIdx++
        }
    }
    state.times = state.times[:validIdx]
    state.counters = state.counters[:validIdx]
    
    if total >= sw.requests {
        resetAt := state.times[0].Add(sw.window)
        return false, 0, resetAt
    }
    
    // Add to current sub-window
    currentWindow := now.Truncate(sw.precision)
    if len(state.times) > 0 && state.times[len(state.times)-1].Equal(currentWindow) {
        state.counters[len(state.counters)-1]++
    } else {
        state.times = append(state.times, currentWindow)
        state.counters = append(state.counters, 1)
    }
    
    remaining := sw.requests - total - 1
    return true, remaining, time.Time{}
}
```

### 6.4 Fixed Window & Leaky Bucket

```go
// internal/plugin/rate_limit_fixed_window.go
type FixedWindow struct {
    windows sync.Map // map[string]*fixedWindowState
}

type fixedWindowState struct {
    count    atomic.Int64
    windowID int64 // unix timestamp / window_seconds
}

func (fw *FixedWindow) Allow(key string) (bool, int64, time.Time) {
    windowID := time.Now().Unix() / int64(fw.window.Seconds())
    
    val, _ := fw.windows.LoadOrStore(key, &fixedWindowState{})
    state := val.(*fixedWindowState)
    
    // Reset if window changed
    if state.windowID != windowID {
        state.count.Store(0)
        state.windowID = windowID
    }
    
    count := state.count.Add(1)
    if count > fw.requests {
        resetAt := time.Unix((windowID+1)*int64(fw.window.Seconds()), 0)
        return false, 0, resetAt
    }
    
    return true, fw.requests - count, time.Time{}
}

// internal/plugin/rate_limit_leaky_bucket.go
type LeakyBucket struct {
    buckets sync.Map
}

type leakyState struct {
    queue    int64     // current queue depth
    capacity int64
    leakRate float64   // requests per second
    lastLeak time.Time
    mu       sync.Mutex
}

func (lb *LeakyBucket) Allow(key string) (bool, int64, time.Time) {
    val, _ := lb.buckets.LoadOrStore(key, &leakyState{
        capacity: lb.capacity,
        leakRate: float64(lb.leakRate),
        lastLeak: time.Now(),
    })
    
    state := val.(*leakyState)
    state.mu.Lock()
    defer state.mu.Unlock()
    
    // Drain based on elapsed time
    now := time.Now()
    elapsed := now.Sub(state.lastLeak).Seconds()
    drained := int64(elapsed * state.leakRate)
    state.queue = max(0, state.queue-drained)
    state.lastLeak = now
    
    if state.queue >= state.capacity {
        waitTime := time.Duration(float64(time.Second) / state.leakRate)
        return false, 0, now.Add(waitTime)
    }
    
    state.queue++
    remaining := state.capacity - state.queue
    return true, remaining, time.Time{}
}
```

---

## 7. Embedded SQLite (Pure Go Strategy)

### 7.1 SQLite Approach

For embedded database support, we have two options:

**Option A: CGO with bundled SQLite amalgamation** (recommended for v0.0.5+)
- Bundle `sqlite3.c` + `sqlite3.h` amalgamation in the repository
- Use Go's `database/sql` with a minimal CGO wrapper
- Single binary still achievable with static linking
- Best performance, full SQLite feature set

**Option B: Pure Go SQLite alternative**
- Implement a minimal key-value + SQL layer on top of a B+Tree
- Similar to what CobaltDB was going to be
- More work but truly zero CGO

**Chosen: Option A** — CGO with bundled amalgamation is pragmatic and proven.

```go
// internal/store/sqlite.go
package store

/*
#cgo CFLAGS: -DSQLITE_THREADSAFE=1 -DSQLITE_ENABLE_WAL=1
#include "sqlite3.h"
*/
import "C"

// The sqlite3.c and sqlite3.h files are bundled in internal/store/
// This keeps zero external Go dependencies while using battle-tested SQLite
```

### 7.2 Schema Migrations

```go
// internal/store/migrations.go
package store

var migrations = []string{
    // v1: Users table
    `CREATE TABLE IF NOT EXISTS users (
        id TEXT PRIMARY KEY,
        email TEXT UNIQUE NOT NULL,
        name TEXT NOT NULL DEFAULT '',
        company TEXT NOT NULL DEFAULT '',
        password_hash TEXT NOT NULL,
        role TEXT NOT NULL DEFAULT 'user' CHECK(role IN ('admin','user')),
        status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','suspended','pending')),
        credit_balance INTEGER NOT NULL DEFAULT 0,
        rate_limit_rps INTEGER NOT NULL DEFAULT 0,
        rate_limit_rpm INTEGER NOT NULL DEFAULT 0,
        rate_limit_rpd INTEGER NOT NULL DEFAULT 0,
        ip_whitelist TEXT NOT NULL DEFAULT '[]',
        metadata TEXT NOT NULL DEFAULT '{}',
        created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
        updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
        last_login_at TEXT,
        last_active_at TEXT
    )`,
    
    `CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)`,
    `CREATE INDEX IF NOT EXISTS idx_users_status ON users(status)`,
    
    // v2: API Keys table
    `CREATE TABLE IF NOT EXISTS api_keys (
        id TEXT PRIMARY KEY,
        user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
        key_hash TEXT UNIQUE NOT NULL,
        key_prefix TEXT NOT NULL,
        name TEXT NOT NULL DEFAULT '',
        status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','revoked','expired')),
        expires_at TEXT,
        last_used_at TEXT,
        last_used_ip TEXT NOT NULL DEFAULT '',
        created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
    )`,
    
    `CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash)`,
    `CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys(user_id)`,
    
    // v3: Endpoint Permissions table
    `CREATE TABLE IF NOT EXISTS endpoint_permissions (
        id TEXT PRIMARY KEY,
        user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
        route_id TEXT NOT NULL,
        methods TEXT NOT NULL DEFAULT '["*"]',
        allowed INTEGER NOT NULL DEFAULT 1,
        rate_limit_rps INTEGER,
        rate_limit_rpm INTEGER,
        rate_limit_rpd INTEGER,
        credit_cost INTEGER,
        valid_from TEXT,
        valid_until TEXT,
        allowed_days TEXT NOT NULL DEFAULT '[]',
        allowed_hours TEXT NOT NULL DEFAULT '',
        created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
    )`,
    
    `CREATE INDEX IF NOT EXISTS idx_permissions_user ON endpoint_permissions(user_id)`,
    `CREATE INDEX IF NOT EXISTS idx_permissions_route ON endpoint_permissions(route_id)`,
    `CREATE UNIQUE INDEX IF NOT EXISTS idx_permissions_user_route ON endpoint_permissions(user_id, route_id)`,
    
    // v4: Credit Transactions table
    `CREATE TABLE IF NOT EXISTS credit_transactions (
        id TEXT PRIMARY KEY,
        user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
        type TEXT NOT NULL CHECK(type IN ('topup','consume','refund','admin_adjust','purchase')),
        amount INTEGER NOT NULL,
        balance_before INTEGER NOT NULL,
        balance_after INTEGER NOT NULL,
        description TEXT NOT NULL DEFAULT '',
        request_id TEXT NOT NULL DEFAULT '',
        route_id TEXT NOT NULL DEFAULT '',
        created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
    )`,
    
    `CREATE INDEX IF NOT EXISTS idx_credit_txns_user ON credit_transactions(user_id)`,
    `CREATE INDEX IF NOT EXISTS idx_credit_txns_created ON credit_transactions(created_at)`,
    
    // v5: Audit Log table
    `CREATE TABLE IF NOT EXISTS audit_logs (
        id TEXT PRIMARY KEY,
        request_id TEXT NOT NULL,
        user_id TEXT NOT NULL DEFAULT '',
        api_key_id TEXT NOT NULL DEFAULT '',
        api_key_prefix TEXT NOT NULL DEFAULT '',
        method TEXT NOT NULL,
        path TEXT NOT NULL,
        host TEXT NOT NULL DEFAULT '',
        query_params TEXT NOT NULL DEFAULT '',
        request_headers TEXT NOT NULL DEFAULT '{}',
        request_body TEXT NOT NULL DEFAULT '',
        request_size INTEGER NOT NULL DEFAULT 0,
        client_ip TEXT NOT NULL DEFAULT '',
        user_agent TEXT NOT NULL DEFAULT '',
        route_id TEXT NOT NULL DEFAULT '',
        route_name TEXT NOT NULL DEFAULT '',
        service_id TEXT NOT NULL DEFAULT '',
        service_name TEXT NOT NULL DEFAULT '',
        upstream_target TEXT NOT NULL DEFAULT '',
        status_code INTEGER NOT NULL DEFAULT 0,
        response_headers TEXT NOT NULL DEFAULT '{}',
        response_body TEXT NOT NULL DEFAULT '',
        response_size INTEGER NOT NULL DEFAULT 0,
        gateway_latency_ms INTEGER NOT NULL DEFAULT 0,
        upstream_latency_ms INTEGER NOT NULL DEFAULT 0,
        credits_consumed INTEGER NOT NULL DEFAULT 0,
        credit_balance_after INTEGER NOT NULL DEFAULT 0,
        blocked INTEGER NOT NULL DEFAULT 0,
        block_reason TEXT NOT NULL DEFAULT '',
        protocol TEXT NOT NULL DEFAULT 'http',
        tls_version TEXT NOT NULL DEFAULT '',
        timestamp TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%f','now'))
    )`,
    
    `CREATE INDEX IF NOT EXISTS idx_audit_user ON audit_logs(user_id)`,
    `CREATE INDEX IF NOT EXISTS idx_audit_route ON audit_logs(route_id)`,
    `CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_logs(timestamp)`,
    `CREATE INDEX IF NOT EXISTS idx_audit_status ON audit_logs(status_code)`,
    `CREATE INDEX IF NOT EXISTS idx_audit_blocked ON audit_logs(blocked)`,
    `CREATE INDEX IF NOT EXISTS idx_audit_client_ip ON audit_logs(client_ip)`,
    
    // v6: Sessions table (for portal auth)
    `CREATE TABLE IF NOT EXISTS sessions (
        id TEXT PRIMARY KEY,
        user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
        token_hash TEXT UNIQUE NOT NULL,
        ip_address TEXT NOT NULL DEFAULT '',
        user_agent TEXT NOT NULL DEFAULT '',
        expires_at TEXT NOT NULL,
        created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
    )`,
    
    `CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token_hash)`,
    `CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id)`,
    `CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at)`,
    
    // v7: Playground templates table
    `CREATE TABLE IF NOT EXISTS playground_templates (
        id TEXT PRIMARY KEY,
        user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
        name TEXT NOT NULL,
        method TEXT NOT NULL,
        path TEXT NOT NULL,
        headers TEXT NOT NULL DEFAULT '{}',
        query_params TEXT NOT NULL DEFAULT '',
        body TEXT NOT NULL DEFAULT '',
        created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
        updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
    )`,
    
    `CREATE INDEX IF NOT EXISTS idx_templates_user ON playground_templates(user_id)`,
    
    // v8: Migration version tracker
    `CREATE TABLE IF NOT EXISTS schema_migrations (
        version INTEGER PRIMARY KEY,
        applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
    )`,
}
```

### 7.3 Repository Pattern

```go
// internal/store/store.go
package store

type Store struct {
    db *sql.DB
}

func Open(cfg config.StoreConfig) (*Store, error) {
    dsn := cfg.Path + "?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON"
    db, err := sql.Open("sqlite3", dsn)
    if err != nil {
        return nil, err
    }
    
    db.SetMaxOpenConns(cfg.MaxOpenConns)
    
    s := &Store{db: db}
    if err := s.migrate(); err != nil {
        return nil, fmt.Errorf("migrate: %w", err)
    }
    
    return s, nil
}

func (s *Store) Users() *UserRepo         { return &UserRepo{db: s.db} }
func (s *Store) APIKeys() *APIKeyRepo     { return &APIKeyRepo{db: s.db} }
func (s *Store) Permissions() *PermissionRepo { return &PermissionRepo{db: s.db} }
func (s *Store) Credits() *CreditRepo     { return &CreditRepo{db: s.db} }
func (s *Store) AuditLogs() *AuditRepo    { return &AuditRepo{db: s.db} }
func (s *Store) Sessions() *SessionRepo   { return &SessionRepo{db: s.db} }
func (s *Store) Templates() *TemplateRepo { return &TemplateRepo{db: s.db} }

// internal/store/user_repo.go
type UserRepo struct {
    db *sql.DB
}

func (r *UserRepo) Create(u *User) error {
    u.ID = generateUUID()
    u.CreatedAt = time.Now().UTC()
    u.UpdatedAt = u.CreatedAt
    
    _, err := r.db.Exec(`
        INSERT INTO users (id, email, name, company, password_hash, role, status,
            credit_balance, rate_limit_rps, rate_limit_rpm, rate_limit_rpd,
            ip_whitelist, metadata, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        u.ID, u.Email, u.Name, u.Company, u.PasswordHash, u.Role, u.Status,
        u.CreditBalance, u.RateLimitRPS, u.RateLimitRPM, u.RateLimitRPD,
        marshalJSON(u.IPWhitelist), marshalJSON(u.Metadata),
        formatTime(u.CreatedAt), formatTime(u.UpdatedAt),
    )
    return err
}

func (r *UserRepo) FindByID(id string) (*User, error) {
    // ... standard SQL scan
}

func (r *UserRepo) FindByEmail(email string) (*User, error) {
    // ... standard SQL scan
}

func (r *UserRepo) List(opts ListOptions) ([]*User, int, error) {
    // Paginated list with search, filter, sort
    // Returns users + total count
}

func (r *UserRepo) UpdateCreditBalance(userID string, delta int64) (newBalance int64, err error) {
    // Atomic credit update using SQL:
    // UPDATE users SET credit_balance = credit_balance + ? WHERE id = ? AND credit_balance + ? >= 0
    // Returns new balance
    // Returns error if balance would go negative
    tx, err := r.db.Begin()
    if err != nil {
        return 0, err
    }
    defer tx.Rollback()
    
    var balance int64
    err = tx.QueryRow(`
        UPDATE users SET credit_balance = credit_balance + ?, updated_at = ?
        WHERE id = ? AND credit_balance + ? >= 0
        RETURNING credit_balance`,
        delta, formatTime(time.Now().UTC()), userID, delta,
    ).Scan(&balance)
    
    if err != nil {
        return 0, fmt.Errorf("insufficient credits or user not found")
    }
    
    return balance, tx.Commit()
}
```

---

## 8. Load Balancing

### 8.1 Balancer Interface

```go
// internal/balancer/balancer.go
package balancer

type Balancer interface {
    Next(ctx *pipeline.RequestContext) (*config.UpstreamTarget, error)
    UpdateTargets(targets []config.UpstreamTarget)
    ReportHealth(targetID string, healthy bool, latency time.Duration)
}

func New(algorithm string, targets []config.UpstreamTarget) Balancer {
    switch algorithm {
    case "round_robin":
        return NewRoundRobin(targets)
    case "weighted_round_robin":
        return NewWeightedRoundRobin(targets)
    case "least_conn":
        return NewLeastConn(targets)
    case "ip_hash":
        return NewIPHash(targets)
    case "random":
        return NewRandom(targets)
    case "consistent_hash":
        return NewConsistentHash(targets)
    case "least_latency":
        return NewLeastLatency(targets)
    case "adaptive":
        return NewAdaptive(targets)
    case "geo_aware":
        return NewGeoAware(targets)
    case "health_weighted":
        return NewHealthWeighted(targets)
    default:
        return NewRoundRobin(targets) // default fallback
    }
}
```

### 8.2 Key Implementations

```go
// Round Robin — atomic counter
type RoundRobin struct {
    targets []config.UpstreamTarget
    counter atomic.Uint64
}

func (rr *RoundRobin) Next(_ *pipeline.RequestContext) (*config.UpstreamTarget, error) {
    n := rr.counter.Add(1)
    idx := int(n) % len(rr.targets)
    return &rr.targets[idx], nil
}

// Least Connections — track active connections per target
type LeastConn struct {
    targets []config.UpstreamTarget
    active  []atomic.Int64 // active connections per target
    mu      sync.RWMutex
}

func (lc *LeastConn) Next(_ *pipeline.RequestContext) (*config.UpstreamTarget, error) {
    lc.mu.RLock()
    defer lc.mu.RUnlock()
    
    minIdx := 0
    minConn := lc.active[0].Load()
    for i := 1; i < len(lc.targets); i++ {
        conn := lc.active[i].Load()
        if conn < minConn {
            minConn = conn
            minIdx = i
        }
    }
    
    lc.active[minIdx].Add(1)
    return &lc.targets[minIdx], nil
}

// Consistent Hash — ring-based for cache affinity
type ConsistentHash struct {
    ring     []hashEntry // sorted by hash value
    replicas int         // virtual nodes per target (default 150)
}

type hashEntry struct {
    hash   uint32
    target *config.UpstreamTarget
}

func (ch *ConsistentHash) Next(ctx *pipeline.RequestContext) (*config.UpstreamTarget, error) {
    // Hash the request key (URL path, or specific header)
    key := ctx.Request.URL.Path
    h := crc32.ChecksumIEEE([]byte(key))
    
    // Binary search in ring
    idx := sort.Search(len(ch.ring), func(i int) bool {
        return ch.ring[i].hash >= h
    })
    if idx >= len(ch.ring) {
        idx = 0
    }
    
    return ch.ring[idx].target, nil
}

// Least Latency — exponentially weighted moving average
type LeastLatency struct {
    targets   []config.UpstreamTarget
    latencies []atomic.Int64 // EWMA latency in microseconds
    alpha     float64        // smoothing factor (default 0.3)
}

func (ll *LeastLatency) Next(_ *pipeline.RequestContext) (*config.UpstreamTarget, error) {
    minIdx := 0
    minLat := ll.latencies[0].Load()
    for i := 1; i < len(ll.targets); i++ {
        lat := ll.latencies[i].Load()
        if lat < minLat {
            minLat = lat
            minIdx = i
        }
    }
    return &ll.targets[minIdx], nil
}

func (ll *LeastLatency) ReportHealth(targetID string, _ bool, latency time.Duration) {
    for i, t := range ll.targets {
        if t.ID == targetID {
            // EWMA update: new = alpha * sample + (1-alpha) * old
            old := float64(ll.latencies[i].Load())
            sample := float64(latency.Microseconds())
            updated := ll.alpha*sample + (1-ll.alpha)*old
            ll.latencies[i].Store(int64(updated))
            break
        }
    }
}
```

---

## 9. Analytics Engine

```go
// internal/analytics/engine.go
package analytics

type Engine struct {
    // Ring buffer for recent requests (fixed memory)
    requestBuffer *RingBuffer[RequestMetric]
    
    // Time-series buckets (per-minute aggregation)
    timeSeries *TimeSeriesStore
    
    // Real-time counters
    totalRequests  atomic.Int64
    activeConns    atomic.Int64
    totalErrors    atomic.Int64
    
    // Subscribers for real-time WebSocket updates
    subscribers map[string]chan<- Metric
    subMu       sync.RWMutex
}

type RequestMetric struct {
    Timestamp       time.Time
    RouteID         string
    ServiceID       string
    UserID          string
    Method          string
    StatusCode      int
    RequestSize     int64
    ResponseSize    int64
    GatewayLatency  time.Duration
    UpstreamLatency time.Duration
    CreditConsumed  int64
    Blocked         bool
    BlockReason     string
}

// RingBuffer is a fixed-size circular buffer (lock-free for single writer)
type RingBuffer[T any] struct {
    items []T
    size  int
    head  atomic.Uint64
}

func (rb *RingBuffer[T]) Push(item T) {
    idx := rb.head.Add(1) % uint64(rb.size)
    rb.items[idx] = item
}

func (rb *RingBuffer[T]) Recent(n int) []T {
    // Return last n items
}

// TimeSeriesStore maintains per-minute bucketed aggregations
type TimeSeriesStore struct {
    buckets map[int64]*Bucket // key = unix minute timestamp
    mu      sync.RWMutex
    maxAge  time.Duration     // retention (default 7 days)
}

type Bucket struct {
    Timestamp   time.Time
    Requests    int64
    Errors      int64
    TotalLatency int64 // microseconds, divide by Requests for avg
    P50Latency  int64
    P95Latency  int64
    P99Latency  int64
    StatusCodes map[int]int64
    BytesIn     int64
    BytesOut    int64
    Credits     int64
}
```

---

## 10. Admin API Server

```go
// internal/admin/server.go
package admin

type Server struct {
    config  *config.Config
    store   *store.Store
    gateway *gateway.Gateway
    mux     *http.ServeMux
}

func New(cfg *config.Config, st *store.Store, gw *gateway.Gateway) *Server {
    s := &Server{config: cfg, store: st, gateway: gw}
    s.mux = http.NewServeMux()
    s.registerRoutes()
    return s
}

func (s *Server) registerRoutes() {
    // Admin auth middleware wraps all /admin/api/ routes
    adminAPI := s.withAuth(s.mux)
    
    // Services
    s.mux.HandleFunc("GET /admin/api/v1/services", s.listServices)
    s.mux.HandleFunc("POST /admin/api/v1/services", s.createService)
    s.mux.HandleFunc("GET /admin/api/v1/services/{id}", s.getService)
    s.mux.HandleFunc("PUT /admin/api/v1/services/{id}", s.updateService)
    s.mux.HandleFunc("DELETE /admin/api/v1/services/{id}", s.deleteService)
    
    // Routes
    s.mux.HandleFunc("GET /admin/api/v1/routes", s.listRoutes)
    s.mux.HandleFunc("POST /admin/api/v1/routes", s.createRoute)
    // ... all route endpoints
    
    // Users
    s.mux.HandleFunc("GET /admin/api/v1/users", s.listUsers)
    s.mux.HandleFunc("POST /admin/api/v1/users", s.createUser)
    s.mux.HandleFunc("GET /admin/api/v1/users/{id}", s.getUser)
    s.mux.HandleFunc("PUT /admin/api/v1/users/{id}", s.updateUser)
    s.mux.HandleFunc("DELETE /admin/api/v1/users/{id}", s.deleteUser)
    s.mux.HandleFunc("POST /admin/api/v1/users/{id}/suspend", s.suspendUser)
    s.mux.HandleFunc("POST /admin/api/v1/users/{id}/activate", s.activateUser)
    
    // User API Keys
    s.mux.HandleFunc("GET /admin/api/v1/users/{id}/api-keys", s.listUserAPIKeys)
    s.mux.HandleFunc("POST /admin/api/v1/users/{id}/api-keys", s.createUserAPIKey)
    s.mux.HandleFunc("DELETE /admin/api/v1/users/{id}/api-keys/{keyId}", s.revokeUserAPIKey)
    
    // User Permissions
    s.mux.HandleFunc("GET /admin/api/v1/users/{id}/permissions", s.listUserPermissions)
    s.mux.HandleFunc("POST /admin/api/v1/users/{id}/permissions", s.grantPermission)
    s.mux.HandleFunc("PUT /admin/api/v1/users/{id}/permissions/{pid}", s.updatePermission)
    s.mux.HandleFunc("DELETE /admin/api/v1/users/{id}/permissions/{pid}", s.revokePermission)
    
    // Credits
    s.mux.HandleFunc("GET /admin/api/v1/credits/overview", s.creditOverview)
    s.mux.HandleFunc("POST /admin/api/v1/users/{id}/credits/topup", s.topupCredits)
    s.mux.HandleFunc("POST /admin/api/v1/users/{id}/credits/deduct", s.deductCredits)
    s.mux.HandleFunc("GET /admin/api/v1/users/{id}/credits/transactions", s.listCreditTxns)
    
    // Audit Logs
    s.mux.HandleFunc("GET /admin/api/v1/audit-logs", s.searchAuditLogs)
    s.mux.HandleFunc("GET /admin/api/v1/audit-logs/{id}", s.getAuditLogDetail)
    s.mux.HandleFunc("GET /admin/api/v1/audit-logs/export", s.exportAuditLogs)
    
    // Analytics
    s.mux.HandleFunc("GET /admin/api/v1/analytics/overview", s.analyticsOverview)
    s.mux.HandleFunc("GET /admin/api/v1/analytics/timeseries", s.analyticsTimeSeries)
    // ... all analytics endpoints
    
    // System
    s.mux.HandleFunc("GET /admin/api/v1/status", s.status)
    s.mux.HandleFunc("GET /admin/api/v1/info", s.info)
    s.mux.HandleFunc("POST /admin/api/v1/config/reload", s.reloadConfig)
    
    // WebSocket for real-time updates
    s.mux.HandleFunc("GET /admin/api/v1/ws", s.handleWebSocket)
    
    // Embedded web dashboard (static files)
    s.mux.Handle("/dashboard/", http.StripPrefix("/dashboard/",
        http.FileServer(http.FS(web.DashboardAssets))))
}

// Admin auth middleware
func (s *Server) withAuth(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Skip auth for static assets
        if strings.HasPrefix(r.URL.Path, "/dashboard/") {
            next.ServeHTTP(w, r)
            return
        }
        
        apiKey := r.Header.Get("X-Admin-Key")
        if apiKey == "" {
            apiKey = r.Header.Get("Authorization")
            apiKey = strings.TrimPrefix(apiKey, "Bearer ")
        }
        
        if !constantTimeEqual(apiKey, s.config.Admin.APIKey) {
            writeJSON(w, 401, map[string]string{"error": "unauthorized"})
            return
        }
        
        next.ServeHTTP(w, r)
    })
}
```

---

## 11. User Portal API Server

```go
// internal/portal/server.go
package portal

type Server struct {
    config *config.Config
    store  *store.Store
    mux    *http.ServeMux
}

func (s *Server) registerRoutes() {
    // Public routes (no auth)
    s.mux.HandleFunc("POST /portal/api/v1/auth/login", s.login)
    
    // Protected routes (session auth)
    s.mux.HandleFunc("POST /portal/api/v1/auth/logout", s.withSession(s.logout))
    s.mux.HandleFunc("GET /portal/api/v1/auth/me", s.withSession(s.me))
    
    // API Keys
    s.mux.HandleFunc("GET /portal/api/v1/api-keys", s.withSession(s.listMyAPIKeys))
    s.mux.HandleFunc("POST /portal/api/v1/api-keys", s.withSession(s.createMyAPIKey))
    s.mux.HandleFunc("PUT /portal/api/v1/api-keys/{id}", s.withSession(s.renameMyAPIKey))
    s.mux.HandleFunc("DELETE /portal/api/v1/api-keys/{id}", s.withSession(s.revokeMyAPIKey))
    
    // Available APIs
    s.mux.HandleFunc("GET /portal/api/v1/apis", s.withSession(s.listMyAPIs))
    s.mux.HandleFunc("GET /portal/api/v1/apis/{routeId}", s.withSession(s.getAPIDetail))
    
    // Playground
    s.mux.HandleFunc("POST /portal/api/v1/playground/send", s.withSession(s.playgroundSend))
    s.mux.HandleFunc("GET /portal/api/v1/playground/templates", s.withSession(s.listTemplates))
    s.mux.HandleFunc("POST /portal/api/v1/playground/templates", s.withSession(s.saveTemplate))
    s.mux.HandleFunc("DELETE /portal/api/v1/playground/templates/{id}", s.withSession(s.deleteTemplate))
    
    // Usage & Logs
    s.mux.HandleFunc("GET /portal/api/v1/usage/overview", s.withSession(s.usageOverview))
    s.mux.HandleFunc("GET /portal/api/v1/logs", s.withSession(s.listMyLogs))
    s.mux.HandleFunc("GET /portal/api/v1/logs/{id}", s.withSession(s.getMyLogDetail))
    
    // Credits
    s.mux.HandleFunc("GET /portal/api/v1/credits/balance", s.withSession(s.myBalance))
    s.mux.HandleFunc("GET /portal/api/v1/credits/transactions", s.withSession(s.myTransactions))
    
    // Security
    s.mux.HandleFunc("GET /portal/api/v1/security/ip-whitelist", s.withSession(s.listMyIPs))
    s.mux.HandleFunc("POST /portal/api/v1/security/ip-whitelist", s.withSession(s.addMyIP))
    s.mux.HandleFunc("DELETE /portal/api/v1/security/ip-whitelist/{ip}", s.withSession(s.removeMyIP))
    
    // Settings
    s.mux.HandleFunc("GET /portal/api/v1/settings/profile", s.withSession(s.getProfile))
    s.mux.HandleFunc("PUT /portal/api/v1/settings/profile", s.withSession(s.updateProfile))
    s.mux.HandleFunc("PUT /portal/api/v1/auth/password", s.withSession(s.changePassword))
    
    // Static portal UI
    s.mux.Handle("/portal/", http.StripPrefix("/portal/",
        http.FileServer(http.FS(web.PortalAssets))))
}

// Session middleware
func (s *Server) withSession(handler http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        cookie, err := r.Cookie(s.config.Portal.Session.CookieName)
        if err != nil {
            writeJSON(w, 401, map[string]string{"error": "unauthorized"})
            return
        }
        
        tokenHash := sha256Hex(cookie.Value)
        session, err := s.store.Sessions().FindByToken(tokenHash)
        if err != nil || session == nil || time.Now().After(session.ExpiresAt) {
            writeJSON(w, 401, map[string]string{"error": "session_expired"})
            return
        }
        
        user, err := s.store.Users().FindByID(session.UserID)
        if err != nil || user == nil || user.Status != "active" {
            writeJSON(w, 403, map[string]string{"error": "user_inactive"})
            return
        }
        
        // Set user in context
        ctx := context.WithValue(r.Context(), ctxKeyUser, user)
        handler(w, r.WithContext(ctx))
    }
}
```

---

## 12. Web Dashboard Embedding

```go
// embed.go (root of project)
package apicerberus

import "embed"

//go:embed web/dist/*
var WebAssets embed.FS

// internal/admin/server.go — serve embedded assets
// The React app is a SPA, so we need to handle client-side routing:

func (s *Server) serveSPA(assets embed.FS, prefix string) http.Handler {
    fsys, _ := fs.Sub(assets, "web/dist")
    fileServer := http.FileServer(http.FS(fsys))
    
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Try to serve the file directly
        path := strings.TrimPrefix(r.URL.Path, prefix)
        if path == "" {
            path = "index.html"
        }
        
        // Check if file exists
        f, err := fsys.Open(path)
        if err != nil {
            // File not found — serve index.html for SPA routing
            r.URL.Path = prefix + "index.html"
        } else {
            f.Close()
        }
        
        fileServer.ServeHTTP(w, r)
    })
}
```

---

## 13. Audit Log System

```go
// internal/audit/logger.go
package audit

type Logger struct {
    store   *store.Store
    config  config.AuditLogConfig
    buffer  chan *AuditEntry // Buffered write channel
    masker  *Masker
}

func NewLogger(cfg config.AuditLogConfig, st *store.Store) *Logger {
    l := &Logger{
        store:  st,
        config: cfg,
        buffer: make(chan *AuditEntry, cfg.BufferSize), // default 10000
        masker: NewMasker(cfg.MaskHeaders, cfg.MaskBodyFields, cfg.MaskReplacement),
    }
    
    // Start background writer (batch inserts for performance)
    go l.flushLoop()
    
    return l
}

func (l *Logger) Log(ctx *pipeline.RequestContext) {
    entry := &AuditEntry{
        ID:            generateUUID(),
        RequestID:     ctx.CorrelationID,
        UserID:        safeUserID(ctx.User),
        APIKeyID:      safeKeyID(ctx.APIKey),
        APIKeyPrefix:  safeKeyPrefix(ctx.APIKey),
        Method:        ctx.Request.Method,
        Path:          ctx.Request.URL.Path,
        Host:          ctx.Request.Host,
        QueryParams:   ctx.Request.URL.RawQuery,
        ClientIP:      ctx.ClientIP,
        UserAgent:     ctx.Request.UserAgent(),
        RouteID:       safeID(ctx.Route),
        RouteName:     safeName(ctx.Route),
        ServiceID:     safeID(ctx.Service),
        ServiceName:   safeName(ctx.Service),
        StatusCode:    ctx.Response.StatusCode,
        RequestSize:   ctx.Request.ContentLength,
        ResponseSize:  ctx.Response.Size,
        GatewayLatency: ctx.GatewayLatency.Milliseconds(),
        UpstreamLatency: ctx.UpstreamLatency.Milliseconds(),
        CreditsConsumed: ctx.CreditCost,
        CreditBalance:   ctx.CreditBalance,
        Blocked:         ctx.Aborted,
        BlockReason:     ctx.AbortReason,
        Timestamp:       ctx.StartTime,
    }
    
    // Capture and mask headers
    if l.config.StoreRequestHeaders {
        entry.RequestHeaders = l.masker.MaskHeaders(ctx.Request.Header)
    }
    if l.config.StoreResponseHeaders {
        entry.ResponseHeaders = l.masker.MaskHeaders(ctx.Response.Headers)
    }
    
    // Capture and mask bodies (truncated to max size)
    if l.config.StoreRequestBody && len(ctx.CapturedRequestBody) > 0 {
        body := truncate(ctx.CapturedRequestBody, l.config.MaxRequestBodySize)
        entry.RequestBody = l.masker.MaskBody(string(body))
    }
    if l.config.StoreResponseBody && len(ctx.CapturedResponseBody) > 0 {
        body := truncate(ctx.CapturedResponseBody, l.config.MaxResponseBodySize)
        entry.ResponseBody = l.masker.MaskBody(string(body))
    }
    
    // Non-blocking send to buffer
    select {
    case l.buffer <- entry:
    default:
        // Buffer full — drop entry (log warning)
        slog.Warn("audit log buffer full, dropping entry", "request_id", entry.RequestID)
    }
}

func (l *Logger) flushLoop() {
    ticker := time.NewTicker(l.config.FlushInterval) // default 1s
    batch := make([]*AuditEntry, 0, 100)
    
    for {
        select {
        case entry := <-l.buffer:
            batch = append(batch, entry)
            if len(batch) >= 100 {
                l.store.AuditLogs().BatchInsert(batch)
                batch = batch[:0]
            }
        case <-ticker.C:
            if len(batch) > 0 {
                l.store.AuditLogs().BatchInsert(batch)
                batch = batch[:0]
            }
        }
    }
}

// internal/audit/retention.go
func (l *Logger) StartRetentionScheduler(ctx context.Context) {
    ticker := time.NewTicker(l.config.Retention.CleanupInterval) // default 1h
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            cutoff := time.Now().Add(-l.config.Retention.Default) // default 30d
            
            // Archive before delete (if enabled)
            if l.config.Archive.Enabled {
                l.archiveOlderThan(cutoff)
            }
            
            deleted, err := l.store.AuditLogs().DeleteOlderThan(cutoff, l.config.Retention.CleanupBatchSize)
            if err != nil {
                slog.Error("audit cleanup failed", "error", err)
            } else if deleted > 0 {
                slog.Info("audit cleanup", "deleted", deleted, "cutoff", cutoff)
            }
        }
    }
}
```

---

## 14. Credit Engine

```go
// internal/billing/credit.go
package billing

type Engine struct {
    store  *store.Store
    config config.BillingConfig
}

func (e *Engine) CalculateCost(ctx *pipeline.RequestContext) int64 {
    if !e.config.Enabled {
        return 0
    }
    
    // 1. Check per-route override in permissions
    if ctx.User != nil {
        perm, _ := e.store.Permissions().FindByUserAndRoute(ctx.User.ID, ctx.Route.ID)
        if perm != nil && perm.CreditCost != nil {
            return *perm.CreditCost
        }
    }
    
    // 2. Check route-specific cost
    if cost, ok := e.config.RouteCosts[ctx.Route.Name]; ok {
        return e.applyMethodMultiplier(cost, ctx.Request.Method)
    }
    
    // 3. Default cost
    return e.applyMethodMultiplier(e.config.DefaultCost, ctx.Request.Method)
}

func (e *Engine) applyMethodMultiplier(baseCost int64, method string) int64 {
    if mult, ok := e.config.MethodMultipliers[method]; ok {
        return int64(float64(baseCost) * mult)
    }
    return baseCost
}

func (e *Engine) CheckAndDeduct(ctx *pipeline.RequestContext) error {
    if !e.config.Enabled || ctx.IsTestKey {
        return nil
    }
    
    cost := e.CalculateCost(ctx)
    ctx.CreditCost = cost
    
    if cost == 0 {
        return nil
    }
    
    // Atomic deduction
    newBalance, err := e.store.Users().UpdateCreditBalance(ctx.User.ID, -cost)
    if err != nil {
        // Insufficient credits
        ctx.CreditBalance = ctx.User.CreditBalance // unchanged
        return e.handleZeroBalance(ctx)
    }
    
    ctx.CreditBalance = newBalance
    
    // Record transaction
    go e.store.Credits().Create(&store.CreditTransaction{
        UserID:        ctx.User.ID,
        Type:          "consume",
        Amount:        -cost,
        BalanceBefore: newBalance + cost,
        BalanceAfter:  newBalance,
        Description:   fmt.Sprintf("API call: %s %s", ctx.Request.Method, ctx.Request.URL.Path),
        RequestID:     ctx.CorrelationID,
        RouteID:       ctx.Route.ID,
    })
    
    // Check low balance warning
    if newBalance <= e.config.LowBalanceThreshold && e.config.LowBalanceWebhook != "" {
        go e.sendLowBalanceWebhook(ctx.User, newBalance)
    }
    
    return nil
}

func (e *Engine) handleZeroBalance(ctx *pipeline.RequestContext) error {
    switch e.config.ZeroBalanceAction {
    case "reject":
        ctx.Aborted = true
        ctx.AbortReason = "insufficient_credits"
        resp := e.config.ZeroBalanceResponse
        if resp != nil {
            writeJSON(ctx.ResponseWriter, resp.StatusCode, json.RawMessage(resp.Body))
        } else {
            writeJSON(ctx.ResponseWriter, 402, map[string]string{
                "error":   "insufficient_credits",
                "message": "Your credit balance is exhausted",
            })
        }
        return fmt.Errorf("insufficient credits")
    case "allow_with_flag":
        ctx.Metadata["zero_balance"] = true
        return nil
    default:
        return fmt.Errorf("insufficient credits")
    }
}
```

---

## 15. Health Checking

```go
// internal/health/checker.go
package health

type Checker struct {
    upstreams map[string]*UpstreamHealth
    mu        sync.RWMutex
}

type UpstreamHealth struct {
    config  config.HealthCheckConfig
    targets map[string]*TargetHealth // keyed by target ID
}

type TargetHealth struct {
    Healthy          bool
    ConsecutiveOK    int
    ConsecutiveFail  int
    LastCheck        time.Time
    LastLatency      time.Duration
    // Passive health
    RecentErrors     int
    ErrorWindowStart time.Time
}

// Active health check loop
func (c *Checker) Start(ctx context.Context) {
    for name, uh := range c.upstreams {
        if uh.config.Active.Interval > 0 {
            go c.activeCheckLoop(ctx, name, uh)
        }
    }
}

func (c *Checker) activeCheckLoop(ctx context.Context, name string, uh *UpstreamHealth) {
    ticker := time.NewTicker(uh.config.Active.Interval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            for _, target := range uh.targets {
                go c.checkTarget(uh, target)
            }
        }
    }
}

func (c *Checker) checkTarget(uh *UpstreamHealth, th *TargetHealth) {
    client := &http.Client{Timeout: uh.config.Active.Timeout}
    
    start := time.Now()
    resp, err := client.Get(fmt.Sprintf("http://%s%s", th.Address, uh.config.Active.Path))
    latency := time.Since(start)
    
    th.LastCheck = time.Now()
    th.LastLatency = latency
    
    healthy := err == nil && contains(uh.config.Active.ExpectedStatus, resp.StatusCode)
    
    if healthy {
        th.ConsecutiveOK++
        th.ConsecutiveFail = 0
        if th.ConsecutiveOK >= uh.config.Active.HealthyThreshold {
            th.Healthy = true
        }
    } else {
        th.ConsecutiveFail++
        th.ConsecutiveOK = 0
        if th.ConsecutiveFail >= uh.config.Active.UnhealthyThreshold {
            th.Healthy = false
        }
    }
    
    if resp != nil {
        resp.Body.Close()
    }
}

// Passive health check (called from proxy on upstream errors)
func (c *Checker) ReportError(upstreamName, targetID string) {
    c.mu.RLock()
    uh, ok := c.upstreams[upstreamName]
    c.mu.RUnlock()
    if !ok {
        return
    }
    
    th := uh.targets[targetID]
    if th == nil {
        return
    }
    
    // Reset error window if expired
    if time.Since(th.ErrorWindowStart) > uh.config.Passive.ErrorWindow {
        th.RecentErrors = 0
        th.ErrorWindowStart = time.Now()
    }
    
    th.RecentErrors++
    if th.RecentErrors >= uh.config.Passive.ErrorThreshold {
        th.Healthy = false
    }
}
```

---

## 16. MCP Server

```go
// internal/mcp/server.go
package mcp

type Server struct {
    store   *store.Store
    gateway *gateway.Gateway
    config  *config.Config
}

// MCP protocol: JSON-RPC 2.0 over stdio or SSE

type JSONRPCRequest struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      any             `json:"id"`
    Method  string          `json:"method"`
    Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
    JSONRPC string `json:"jsonrpc"`
    ID      any    `json:"id"`
    Result  any    `json:"result,omitempty"`
    Error   *Error `json:"error,omitempty"`
}

func (s *Server) HandleRequest(req JSONRPCRequest) JSONRPCResponse {
    switch req.Method {
    case "initialize":
        return s.initialize(req)
    case "tools/list":
        return s.listTools(req)
    case "tools/call":
        return s.callTool(req)
    case "resources/list":
        return s.listResources(req)
    case "resources/read":
        return s.readResource(req)
    default:
        return errorResponse(req.ID, -32601, "method not found")
    }
}

func (s *Server) listTools(_ JSONRPCRequest) JSONRPCResponse {
    tools := []Tool{
        {Name: "apicerberus_list_users", Description: "List all platform users"},
        {Name: "apicerberus_create_user", Description: "Create a new user"},
        {Name: "apicerberus_topup_credits", Description: "Add credits to a user"},
        {Name: "apicerberus_search_audit_logs", Description: "Search request audit logs"},
        {Name: "apicerberus_analytics_overview", Description: "Get analytics dashboard data"},
        {Name: "apicerberus_list_services", Description: "List all gateway services"},
        {Name: "apicerberus_create_route", Description: "Create a new route"},
        {Name: "apicerberus_upstream_health", Description: "Check upstream health status"},
        {Name: "apicerberus_cluster_status", Description: "Get cluster status"},
        // ... all tools from SPECIFICATION.md §8.1
    }
    // Return with input schemas for each tool
}

// stdio transport
func (s *Server) RunStdio() error {
    scanner := bufio.NewScanner(os.Stdin)
    encoder := json.NewEncoder(os.Stdout)
    
    for scanner.Scan() {
        var req JSONRPCRequest
        if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
            continue
        }
        resp := s.HandleRequest(req)
        encoder.Encode(resp)
    }
    return scanner.Err()
}

// SSE transport
func (s *Server) RunSSE(addr string) error {
    mux := http.NewServeMux()
    mux.HandleFunc("/sse", s.handleSSE)
    return http.ListenAndServe(addr, mux)
}
```

---

## 17. CLI Implementation

```go
// internal/cli/cli.go
package cli

func Run(args []string) error {
    if len(args) == 0 {
        printUsage()
        return nil
    }
    
    switch args[0] {
    case "start":
        return cmdStart(args[1:])
    case "stop":
        return cmdStop(args[1:])
    case "reload":
        return cmdReload(args[1:])
    case "version":
        return cmdVersion()
    case "service":
        return cmdService(args[1:])
    case "route":
        return cmdRoute(args[1:])
    case "upstream":
        return cmdUpstream(args[1:])
    case "consumer":
        return cmdConsumer(args[1:])
    case "plugin":
        return cmdPlugin(args[1:])
    case "user":
        return cmdUser(args[1:])
    case "credit":
        return cmdCredit(args[1:])
    case "audit":
        return cmdAudit(args[1:])
    case "analytics":
        return cmdAnalytics(args[1:])
    case "cluster":
        return cmdCluster(args[1:])
    case "config":
        return cmdConfig(args[1:])
    case "mcp":
        return cmdMCP(args[1:])
    default:
        return fmt.Errorf("unknown command: %s", args[0])
    }
}

// internal/cli/cmd_start.go
func cmdStart(args []string) error {
    configPath := "apicerberus.yaml"
    // Parse flags
    for i := 0; i < len(args); i++ {
        switch args[i] {
        case "--config", "-c":
            if i+1 < len(args) {
                configPath = args[i+1]
                i++
            }
        }
    }
    
    // Load config
    cfg, err := config.Load(configPath)
    if err != nil {
        return fmt.Errorf("load config: %w", err)
    }
    
    // Open store
    st, err := store.Open(cfg.Store)
    if err != nil {
        return fmt.Errorf("open store: %w", err)
    }
    
    // Create gateway
    gw, err := gateway.New(cfg, st)
    if err != nil {
        return fmt.Errorf("create gateway: %w", err)
    }
    
    // Create admin API server
    adminSrv := admin.New(cfg, st, gw)
    
    // Create portal server
    portalSrv := portal.New(cfg, st)
    
    // Create audit logger
    auditLogger := audit.NewLogger(cfg.AuditLog, st)
    
    // Context for graceful shutdown
    ctx, cancel := signal.NotifyContext(context.Background(),
        os.Interrupt, syscall.SIGTERM)
    defer cancel()
    
    // Start all servers
    errCh := make(chan error, 4)
    
    go func() { errCh <- gw.Start(ctx) }()
    go func() { errCh <- adminSrv.Start(ctx) }()
    go func() { errCh <- portalSrv.Start(ctx) }()
    go auditLogger.StartRetentionScheduler(ctx)
    
    // Start config watcher
    go config.Watch(configPath, func(newCfg *config.Config) {
        gw.Reload(newCfg)
        slog.Info("config reloaded")
    })
    
    slog.Info("API Cerberus started",
        "http", cfg.Gateway.HTTPAddr,
        "admin", cfg.Admin.Addr,
        "portal", cfg.Portal.Addr,
        "version", version.Version,
    )
    
    return <-errCh
}
```

---

## 18. TLS & ACME

```go
// internal/gateway/tls.go
package gateway

type TLSManager struct {
    config   config.TLSConfig
    certs    sync.Map // map[domain]*tls.Certificate
    acmeDir  string
}

// GetCertificate is used as tls.Config.GetCertificate callback
func (tm *TLSManager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
    // 1. Check cached certificate
    if cert, ok := tm.certs.Load(hello.ServerName); ok {
        c := cert.(*tls.Certificate)
        if time.Now().Before(c.Leaf.NotAfter.Add(-30 * 24 * time.Hour)) {
            return c, nil // Valid and not expiring within 30 days
        }
    }
    
    // 2. Check stored certificate on disk
    cert, err := tm.loadFromDisk(hello.ServerName)
    if err == nil {
        tm.certs.Store(hello.ServerName, cert)
        return cert, nil
    }
    
    // 3. Issue new certificate via ACME
    if tm.config.Auto {
        cert, err := tm.issueCertificate(hello.ServerName)
        if err != nil {
            return nil, err
        }
        tm.certs.Store(hello.ServerName, cert)
        return cert, nil
    }
    
    return nil, fmt.Errorf("no certificate for %s", hello.ServerName)
}

// ACME implementation using tls-alpn-01 challenge
func (tm *TLSManager) issueCertificate(domain string) (*tls.Certificate, error) {
    // 1. Generate RSA key pair
    key, _ := rsa.GenerateKey(rand.Reader, 2048)
    
    // 2. Create ACME account (if not exists)
    // 3. Request authorization for domain
    // 4. Solve tls-alpn-01 challenge
    // 5. Finalize order and get certificate
    // 6. Save to disk
    
    // This is ~500 lines implementing RFC 8555 (ACME) + RFC 8737 (tls-alpn-01)
    // Using only crypto/tls, crypto/rsa, net/http, encoding/json
}
```

---

## 19. Raft Clustering (v0.5.0)

```go
// internal/cluster/raft.go
package cluster

// Pure Go Raft implementation following the Raft paper (Ongaro & Ousterhout)
// Implements: leader election, log replication, snapshotting

type RaftNode struct {
    id       string
    state    atomic.Int32 // Follower=0, Candidate=1, Leader=2
    
    // Persistent state
    currentTerm atomic.Uint64
    votedFor    string
    log         []LogEntry
    
    // Volatile state
    commitIndex uint64
    lastApplied uint64
    
    // Leader state
    nextIndex  map[string]uint64
    matchIndex map[string]uint64
    
    // FSM
    fsm FSM
    
    // Transport
    transport Transport
    peers     []string
    
    // Channels
    applyCh   chan LogEntry
    
    mu sync.RWMutex
}

type FSM interface {
    Apply(entry LogEntry) error
    Snapshot() ([]byte, error)
    Restore(data []byte) error
}

// Gateway FSM — applies config changes to the running gateway
type GatewayFSM struct {
    gateway *gateway.Gateway
    store   *store.Store
}

func (g *GatewayFSM) Apply(entry LogEntry) error {
    switch entry.Type {
    case LogTypeConfigUpdate:
        return g.gateway.Reload(entry.Data.(*config.Config))
    case LogTypeCreditUpdate:
        // Distributed credit balance sync
    case LogTypeRateLimitSync:
        // Distributed rate limit counter sync
    }
    return nil
}
```

---

## 20. Performance & Optimization Notes

### Memory Management
- **sync.Pool** for buffer reuse (proxy body copying, JSON encoding)
- **Ring buffers** for analytics (fixed memory, no allocations after init)
- **String interning** for repeated route names, header keys
- **Pre-allocated slices** for plugin chains (avoid append allocations)

### Concurrency
- **sync.RWMutex** for config reads (many goroutines read, rare writes)
- **sync.Map** for rate limit buckets (high contention, many keys)
- **atomic.Int64** for counters (active connections, request count)
- **Channel-based** audit log buffering (decouple hot path from I/O)
- **Connection pooling** via http.Transport (reuse upstream connections)

### Hot Path Optimization
The request processing hot path (ServeHTTP) should avoid:
- Heap allocations (use sync.Pool for buffers)
- Map lookups with string keys (use pre-computed hashes)
- Reflection (all config parsed at startup)
- Locks on shared state (use atomic/lock-free where possible)
- Logging in hot path (use async ring buffer)

### SQLite Performance
- WAL mode (concurrent reads + single writer)
- Prepared statements (reused across requests)
- Batch inserts for audit logs (collect → flush every 1s)
- Connection pool (25 connections default)
- PRAGMA settings: `journal_mode=WAL`, `busy_timeout=5000`, `synchronous=NORMAL`

---

## 21. Testing Strategy

```
Unit tests:        internal/pkg/* (YAML parser, JWT, UUID, crypto)
                   internal/plugin/* (each plugin in isolation)
                   internal/balancer/* (each algorithm)
                   internal/config/* (parsing, validation, env override)
                   internal/store/* (SQLite repos with in-memory DB)

Integration tests: internal/gateway/* (full proxy pipeline with httptest)
                   internal/admin/* (API endpoints with test store)
                   internal/portal/* (portal endpoints with test session)

E2E tests:         test/ (start full gateway, proxy real requests, verify)

Benchmark tests:   internal/gateway/ (req/sec throughput)
                   internal/balancer/ (selection speed)
                   internal/plugin/rate_limit* (contention under load)
                   internal/store/ (SQLite batch insert speed)
```

```go
// Test pattern for plugins:
func TestRateLimitTokenBucket(t *testing.T) {
    tb := &TokenBucket{}
    tb.Configure(RateLimitConfig{
        Algorithm: "token_bucket",
        Capacity:  10,
        RefillRate: 5,
    })
    
    // Should allow initial burst
    for i := 0; i < 10; i++ {
        allowed, _, _ := tb.Allow("test-key")
        if !allowed {
            t.Fatalf("request %d should be allowed", i)
        }
    }
    
    // 11th should be rejected
    allowed, _, resetAt := tb.Allow("test-key")
    if allowed {
        t.Fatal("11th request should be rejected")
    }
    if resetAt.IsZero() {
        t.Fatal("resetAt should be set")
    }
    
    // Wait for refill
    time.Sleep(200 * time.Millisecond) // 5/sec * 0.2s = 1 token
    allowed, _, _ = tb.Allow("test-key")
    if !allowed {
        t.Fatal("should be allowed after refill")
    }
}
```

---

## 22. Docker & Deployment

```dockerfile
# Dockerfile — multi-stage build
FROM node:22-alpine AS web-builder
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.23-alpine AS go-builder
RUN apk add --no-cache gcc musl-dev  # For CGO (SQLite)
WORKDIR /app
COPY go.mod ./
COPY . .
COPY --from=web-builder /app/web/dist ./web/dist
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o /apicerberus ./cmd/apicerberus

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=go-builder /apicerberus /usr/local/bin/apicerberus
RUN mkdir -p /var/apicerberus /etc/apicerberus
EXPOSE 8080 8443 9090 9876 9877 7946
ENTRYPOINT ["apicerberus", "start"]
CMD ["--config", "/etc/apicerberus/apicerberus.yaml"]
```

---

## 23. Key Implementation Decisions Summary

| Decision | Choice | Reason |
|----------|--------|--------|
| YAML parser | Custom (~1000 LOC) | Zero deps, only need config subset |
| JSON handling | `encoding/json` | Standard library, sufficient performance |
| SQLite | CGO with bundled amalgamation | Battle-tested, ACID, zero external Go deps |
| HTTP router | Custom radix tree | Fast prefix matching, Go 1.22+ mux patterns |
| Connection pool | `http.Transport` | Built-in, proven, HTTP/2 support |
| Rate limit storage | `sync.Map` + per-key mutex | High concurrency, many distinct keys |
| Analytics buffer | Lock-free ring buffer | Fixed memory, no GC pressure |
| Audit log writes | Buffered channel → batch insert | Decouple hot path from disk I/O |
| Config hot reload | `SIGHUP` + file poll + admin API | Multiple trigger mechanisms |
| Password hashing | `golang.org/x/crypto/bcrypt`... NO | Use stdlib `crypto/sha256` + salt (or bundle bcrypt source) |
| UUID generation | `crypto/rand` + format | No external dep needed |
| Web embedding | `embed.FS` | Go 1.16+ native, single binary |
| Structured logging | `log/slog` | Go 1.21+ native, JSON output |
| gRPC proxy | Raw HTTP/2 + `net/http` | No gRPC library needed for proxying |
| WebSocket proxy | `net.Conn` hijack + bidirectional copy | No gorilla/websocket dependency |

---

## 24. File Count & Size Estimates

```
Go source files:           ~120 files
Go source lines:           ~25,000-30,000 LOC
Test files:                ~50 files
Test lines:                ~10,000-12,000 LOC
React/TypeScript files:    ~80 files
Frontend lines:            ~15,000-20,000 LOC
Config/build files:        ~15 files
Documentation:             ~5 files (SPEC, IMPL, TASKS, BRANDING, README)

Total estimated:           ~270 files, ~60,000-70,000 LOC
Binary size target:        < 30MB (with embedded UI)
```
