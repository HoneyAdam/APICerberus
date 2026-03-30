# API Cerberus â€” Task Breakdown

## TASKS.md

> Granular task list for building API Cerberus.
> Each task is a single implementable unit. Check off as completed.
> Read SPECIFICATION.md and IMPLEMENTATION.md before starting.

---

## v0.0.1 â€” Core Gateway

### 1.1 Project Scaffolding
- [x] Initialize Go module: `go mod init github.com/APICerberus/APICerberus`
- [x] Create directory structure: `cmd/apicerberus/`, `internal/`, `web/`, `test/`
- [x] Create `internal/version/version.go` with Version, Commit, BuildTime vars
- [x] Create `Makefile` with build, clean, test, lint targets
- [x] Create `Dockerfile` (multi-stage: node builder â†’ go builder â†’ alpine runtime)
- [x] Create `.gitignore` (bin/, web/dist/, *.db, *.log)
- [x] Create `LICENSE` (MIT)
- [x] Create `README.md` with project overview and badges placeholder

### 1.2 Custom YAML Parser
- [x] Implement tokenizer: line-by-line scanner, track indentation level
- [x] Implement node types: NodeMap, NodeSequence, NodeScalar
- [x] Implement parser: indentation â†’ nesting, `- item` â†’ sequences, `key: value` â†’ map entries
- [x] Implement comment stripping (`# ...`)
- [x] Implement quoted strings (single + double quotes, escape handling)
- [x] Implement multi-line strings (`|` literal, `>` folded)
- [x] Implement `Unmarshal(data []byte, v any) error` using reflection
- [x] Implement struct tag support (`yaml:"field_name"`)
- [x] Implement type coercion: string â†’ int, float, bool, time.Duration
- [x] Implement `Marshal(v any) ([]byte, error)` for config export
- [x] Write unit tests for all YAML features (20+ test cases)

### 1.3 Configuration System
- [x] Define `Config` struct with all top-level sections (Gateway, Admin, Logging)
- [x] Define `GatewayConfig` struct (HTTPAddr, HTTPSAddr, timeouts, limits)
- [x] Define `AdminConfig` struct (Addr, APIKey, UIEnabled, UIPath)
- [x] Define `LoggingConfig` struct (Level, Format, Output, File rotation)
- [x] Define `Service` struct (ID, Name, Protocol, Upstream, timeouts)
- [x] Define `Route` struct (ID, Name, Service, Hosts, Paths, Methods, StripPath, Priority)
- [x] Define `Upstream` struct (ID, Name, Algorithm, Targets, HealthCheck)
- [x] Define `UpstreamTarget` struct (ID, Address, Weight)
- [x] Define `HealthCheckConfig` struct (Active: path, interval, thresholds)
- [x] Implement `config.Load(path string) (*Config, error)` â€” read file + parse YAML
- [x] Implement `setDefaults(*Config)` â€” fill missing values with defaults
- [x] Implement `validate(*Config)` â€” required fields, value ranges, format checks
- [x] Implement `generateIDs(*Config)` â€” UUID for entities without explicit IDs
- [x] Implement `applyEnvOverrides(*Config)` â€” `APICERBERUS_*` env var mapping via reflection
- [x] Implement `config.Watch(path, onChange)` â€” file poll (2s) + SIGHUP signal handler
- [x] Create `apicerberus.example.yaml` with documented example config
- [x] Write unit tests for config loading, defaults, validation, env override

### 1.4 UUID Generator
- [x] Implement `internal/pkg/uuid/uuid.go` â€” UUID v4 using `crypto/rand`
- [x] Format: `xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx`
- [x] Write unit tests (format validation, uniqueness)

### 1.5 JSON Helpers
- [x] Implement `internal/pkg/json/helpers.go` â€” `writeJSON(w, status, data)` helper
- [x] Implement `readJSON(r, &target)` helper with size limit
- [x] Implement `marshalJSON(v) string` for storing JSON in SQLite text fields

### 1.6 Structured Logging
- [x] Set up `log/slog` with JSON handler
- [x] Configure log level from config (debug/info/warn/error)
- [x] Configure output (stdout/stderr/file)
- [x] Implement file rotation (max size, max backups, compress)
- [x] Add request-scoped logging (correlation ID, route, method)

### 1.7 Router (Radix Tree)
- [x] Implement `radixNode` struct (path, children, route, service, isWild, paramKey)
- [x] Implement `insert(path, route, service)` â€” split nodes, handle wildcards (`*`) and params (`:id`)
- [x] Implement `search(path) (*Route, *Service, params, bool)` â€” walk tree, match segments
- [x] Implement `MethodTree` â€” per-HTTP-method radix trees + wildcard "any" tree
- [x] Implement `Router` struct with host-based routing: `map[host]*MethodTree` + default tree
- [x] Implement `Router.Match(req) (*Route, *Service, error)` â€” host â†’ method â†’ path lookup
- [x] Implement route priority sorting (exact > prefix > regex)
- [x] Implement regex route fallback (compiled at config load)
- [x] Implement path stripping (`strip_path: true`)
- [x] Implement `Router.Rebuild(routes, services)` for hot reload
- [x] Write unit tests: exact match, prefix match, wildcard, params, host routing, priority, strip path

### 1.8 Reverse Proxy
- [x] Implement `Proxy` struct with `http.Transport` (connection pooling settings)
- [x] Implement `sync.Pool` for 32KB copy buffers
- [x] Implement `Proxy.Forward(ctx, target)` â€” build upstream request, copy headers, set X-Forwarded-*, execute, stream response
- [x] Implement header copying (hop-by-hop header filtering: Connection, Keep-Alive, etc.)
- [x] Implement `strip_path` path rewriting in proxy
- [x] Implement `preserve_host` toggle
- [x] Implement error handling (upstream connect failure â†’ 502, timeout â†’ 504)
- [x] Implement response streaming (io.CopyBuffer with pooled buffer)
- [x] Write integration tests with `httptest.Server` as upstream

### 1.9 WebSocket Proxy
- [x] Implement WebSocket upgrade detection (`Upgrade: websocket` header check)
- [x] Implement connection hijacking via `http.Hijacker`
- [x] Implement upstream WebSocket dial + upgrade request forwarding
- [x] Implement bidirectional copy (`io.Copy` in two goroutines)
- [x] Implement graceful close on either side disconnect
- [x] Write integration test with WebSocket echo server

### 1.10 Load Balancing (Round Robin + Weighted)
- [x] Implement `Balancer` interface: `Next(ctx) (*Target, error)`, `UpdateTargets()`, `ReportHealth()`
- [x] Implement `balancer.New(algorithm, targets) Balancer` factory
- [x] Implement `RoundRobin` â€” atomic counter, modulo target count
- [x] Implement `WeightedRoundRobin` â€” expanded target list based on weight, atomic counter
- [x] Implement `UpstreamPool` struct â€” balancer + targets + health state
- [x] Write unit tests for both algorithms (distribution verification)

### 1.11 Active Health Checking
- [x] Implement `health.Checker` struct â€” map of upstream health states
- [x] Implement `TargetHealth` struct (Healthy, ConsecutiveOK, ConsecutiveFail, LastCheck, LastLatency)
- [x] Implement active check loop: ticker â†’ HTTP GET to health path â†’ evaluate status
- [x] Implement healthy/unhealthy threshold logic (consecutive successes/failures)
- [x] Implement `Checker.IsHealthy(upstreamName, targetID) bool`
- [x] Integrate with balancer: skip unhealthy targets in `Next()`
- [x] Write unit tests with mock HTTP health endpoints

### 1.12 Gateway Server
- [x] Implement `Gateway` struct (config, router, proxy, health, httpServer)
- [x] Implement `Gateway.New(cfg) (*Gateway, error)` â€” initialize all subsystems
- [x] Implement `Gateway.ServeHTTP(w, r)` â€” route match â†’ select target â†’ proxy forward
- [x] Implement `Gateway.Start(ctx) error` â€” start HTTP listener
- [x] Implement `Gateway.Reload(newCfg)` â€” rebuild router, update upstreams (RWMutex protected)
- [x] Implement graceful shutdown (context cancellation, drain connections)
- [x] Implement custom error responses (JSON format, consistent error codes)
- [x] Write integration test: start gateway, proxy requests to test upstream, verify response

### 1.13 Admin REST API
- [x] Implement `admin.Server` struct with `http.ServeMux` (Go 1.22+ method patterns)
- [x] Implement admin auth middleware (X-Admin-Key header, constant-time comparison)
- [x] Implement `GET /admin/api/v1/services` â€” list all services
- [x] Implement `POST /admin/api/v1/services` â€” create service (validate, add to config)
- [x] Implement `GET /admin/api/v1/services/{id}` â€” get service detail
- [x] Implement `PUT /admin/api/v1/services/{id}` â€” update service
- [x] Implement `DELETE /admin/api/v1/services/{id}` â€” delete service
- [x] Implement routes CRUD: `GET/POST/PUT/DELETE /admin/api/v1/routes[/{id}]`
- [x] Implement upstreams CRUD: `GET/POST/PUT/DELETE /admin/api/v1/upstreams[/{id}]`
- [x] Implement `POST/DELETE /admin/api/v1/upstreams/{id}/targets[/{tid}]` â€” target management
- [x] Implement `GET /admin/api/v1/upstreams/{id}/health` â€” upstream health status
- [x] Implement `GET /admin/api/v1/status` â€” gateway health check
- [x] Implement `GET /admin/api/v1/info` â€” version, uptime, config summary
- [x] Implement `POST /admin/api/v1/config/reload` â€” trigger hot reload
- [x] Implement in-memory config mutation (Admin API changes â†’ update running config + trigger router rebuild)
- [x] Write integration tests for all admin endpoints

### 1.14 CLI
- [x] Implement `cli.Run(args)` â€” command dispatch (start, stop, version, config)
- [x] Implement `apicerberus start` â€” load config, open store, boot gateway + admin + health checker
- [x] Implement `apicerberus start --config <path>` flag parsing
- [x] Implement `apicerberus stop` â€” send SIGTERM to running process (via PID file)
- [x] Implement `apicerberus version` â€” print version info (JSON)
- [x] Implement `apicerberus config validate <path>` â€” parse + validate config, print errors
- [x] Implement graceful shutdown: `signal.NotifyContext(SIGINT, SIGTERM)`
- [x] Implement startup banner (ASCII logo, version, listen addresses)

### 1.15 Final Integration & Testing (v0.0.1)
- [x] Write E2E test: start full gateway, configure service/route/upstream via admin API, proxy requests, verify
- [x] Write E2E test: hot reload â€” change config file, send SIGHUP, verify new config applied
- [x] Verify binary compiles with `go build` (zero external deps in go.mod)
- [x] Verify Docker build succeeds
- [ ] Tag `v0.0.1`

---

## v0.0.2 â€” Authentication & Rate Limiting

### 2.1 Consumer Entity
- [x] Define `Consumer` config struct (Name, APIKeys, RateLimit, ACLGroups, Metadata)
- [x] Add `consumers` section to YAML config parser
- [x] Add consumer resolution in gateway pipeline (API key â†’ consumer mapping)

### 2.2 API Key Authentication Plugin
- [x] Implement `AuthAPIKey` plugin struct (Phase: Auth, Priority: 10)
- [x] Implement key extraction: header (X-API-Key, Authorization: Bearer), query param, cookie
- [x] Implement configurable key header names (`key_names` config)
- [x] Implement key lookup from consumer config (linear scan, hash for performance)
- [x] Implement key expiration check
- [x] Implement constant-time key comparison (`crypto/subtle.ConstantTimeCompare`)
- [x] Set `ctx.Consumer` on successful auth
- [x] Return 401 with JSON error on missing/invalid/expired key
- [x] Write unit tests (valid key, invalid key, expired key, multiple header sources)

### 2.3 JWT Authentication Plugin
- [x] Implement `internal/pkg/jwt/jwt.go` â€” JWT parsing (split `.`, base64url decode header + payload)
- [x] Implement `internal/pkg/jwt/hs256.go` â€” HMAC-SHA256 verification
- [x] Implement `internal/pkg/jwt/rs256.go` â€” RSA-SHA256 verification using `crypto/rsa`
- [x] Implement JWKS fetching: HTTP GET â†’ parse JSON â†’ extract RSA public keys from `n` + `e`
- [x] Implement JWKS caching with TTL (re-fetch every 1h)
- [x] Implement `AuthJWT` plugin struct (Phase: Auth, Priority: 20)
- [x] Implement claim validation: exp, iss, aud, required_claims
- [x] Implement clock skew tolerance
- [x] Implement claims-to-headers mapping (sub â†’ X-Consumer-ID, etc.)
- [x] Write unit tests: valid HS256, valid RS256, expired, wrong issuer, wrong audience, missing claims

### 2.4 Token Bucket Rate Limiter
- [x] Implement `TokenBucket` struct with `sync.Map` for per-key buckets
- [x] Implement refill logic: elapsed time Ã— refill rate, capped at capacity
- [x] Implement `Allow(key) (bool, remaining, resetAt)`
- [x] Write unit tests: burst, refill timing, multiple keys

### 2.5 Fixed Window Rate Limiter
- [x] Implement `FixedWindow` struct with `sync.Map` for per-key windows
- [x] Implement window ID calculation: `unix_timestamp / window_seconds`
- [x] Implement atomic counter per window
- [x] Implement window reset on ID change
- [x] Write unit tests: within limit, exceed limit, window reset

### 2.6 Rate Limit Plugin
- [x] Implement `RateLimit` plugin struct (Phase: PreProxy, Priority: 20)
- [x] Implement scope resolution: global key, consumer key, IP key, route key, composite keys
- [x] Implement algorithm selection from config (token_bucket, fixed_window)
- [x] Set rate limit response headers: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`
- [x] Return 429 with `Retry-After` header on limit exceeded
- [x] Write integration tests with multiple consumers, different scopes

### 2.7 IP Restrict Plugin
- [x] Implement `IPRestrict` plugin struct (Phase: PreAuth, Priority: 5)
- [x] Implement IP whitelist mode: only listed IPs allowed
- [x] Implement IP blacklist mode: listed IPs blocked
- [x] Implement CIDR range matching (parse `net.ParseCIDR`, `network.Contains`)
- [x] Return 403 with reason on blocked IP
- [x] Write unit tests: whitelist hit/miss, blacklist hit/miss, CIDR ranges

### 2.8 CORS Plugin
- [x] Implement `CORS` plugin struct (Phase: PreAuth, Priority: 1)
- [x] Implement preflight handling: respond to OPTIONS with Access-Control-Allow-* headers
- [x] Implement origin validation (exact match, wildcard `*`)
- [x] Implement configurable: allowed origins, methods, headers, max_age, credentials
- [x] Set response headers on actual requests (Access-Control-Allow-Origin, etc.)
- [x] Write unit tests: preflight, actual request, disallowed origin

### 2.9 Plugin Registry & Pipeline Integration
- [x] Implement `plugin.Registry` â€” map[name]Plugin, register/lookup
- [x] Implement plugin chain builder: parse route plugin configs â†’ instantiate â†’ sort by phase+priority
- [x] Integrate pipeline into `Gateway.ServeHTTP` (before proxy forward)
- [x] Implement per-route plugin configuration via route YAML
- [x] Implement global plugin configuration via `global_plugins` YAML
- [x] Write integration test: request through auth â†’ rate limit â†’ proxy

### 2.10 Final (v0.0.2)
- [x] E2E test: API key auth + rate limiting + CORS working together
- [x] E2E test: JWT auth with RS256 + rate limiting per consumer
- [ ] Tag `v0.0.2`

---

## v0.0.3 â€” Full Load Balancing & Resilience

### 3.1 Additional Load Balancers
- [x] Implement `LeastConn` â€” track active connections per target (atomic.Int64 array)
- [x] Implement `IPHash` â€” FNV hash of client IP, modulo target count
- [x] Implement `Random` â€” `math/rand/v2` selection
- [x] Implement `ConsistentHash` â€” virtual node ring (CRC32), binary search
- [x] Implement `LeastLatency` â€” EWMA latency tracking, select minimum
- [x] Implement `Adaptive` â€” monitor error rate + latency, switch algorithm dynamically
- [x] Implement `GeoAware` â€” placeholder (IP â†’ country mapping, select nearest datacenter)
- [x] Implement `HealthWeighted` â€” score = health_pct Ã— weight, weighted random selection
- [x] Write unit tests for each algorithm (distribution, edge cases)
- [x] Write benchmark tests (`go test -bench`) for selection hot path

### 3.2 Passive Health Checking
- [x] Implement error tracking in proxy: on upstream error â†’ `health.ReportError(upstream, target)`
- [x] Implement error window: count errors within sliding window duration
- [x] Implement success recovery: consecutive successes â†’ mark healthy again
- [x] Integrate passive checks alongside active checks
- [x] Write unit tests: error accumulation, window expiry, recovery

### 3.3 Circuit Breaker Plugin
- [x] Implement `CircuitBreaker` struct â€” states: Closed, Open, HalfOpen
- [x] Implement error rate tracking: `errors / total` within volume threshold
- [x] Implement state transitions: Closedâ†’Open (error_threshold exceeded), Openâ†’HalfOpen (sleep_window elapsed), HalfOpenâ†’Closed (success) or HalfOpenâ†’Open (failure)
- [x] Implement half-open trial requests (configurable count)
- [x] Return 503 when circuit is open
- [x] Write unit tests: state transitions, recovery, volume threshold

### 3.4 Retry Plugin
- [x] Implement `Retry` plugin struct (Phase: Proxy)
- [x] Implement retry logic: on 502/503/504 â†’ retry with next target
- [x] Implement exponential backoff with jitter
- [x] Implement max retries configuration
- [x] Implement idempotency check (only retry safe methods by default: GET, HEAD, OPTIONS)
- [x] Write unit tests: retry on failure, no retry on POST (unless configured), backoff timing

### 3.5 Timeout Plugin
- [x] Implement `Timeout` plugin struct (Phase: Proxy)
- [x] Implement per-route timeout: wrap proxy request with `context.WithTimeout`
- [x] Return 504 Gateway Timeout on context deadline exceeded
- [x] Write unit tests: request completes within timeout, request exceeds timeout

### 3.6 Sliding Window Rate Limiter
- [x] Implement `SlidingWindow` struct with sub-window counters
- [x] Implement counter rotation: remove expired sub-windows
- [x] Implement precise count across window boundary
- [x] Write unit tests: precision accuracy vs fixed window

### 3.7 Leaky Bucket Rate Limiter
- [x] Implement `LeakyBucket` struct with queue depth + drain rate
- [x] Implement drain logic: elapsed time Ã— leak_rate â†’ reduce queue
- [x] Implement capacity check: reject when queue full
- [x] Write unit tests: smooth throughput, burst rejection

### 3.8 Rate Limit Algorithm Selection
- [x] Extend `RateLimit` plugin to support all 4 algorithms
- [x] Implement algorithm factory: `NewRateLimiter(algorithm) RateLimiter`
- [x] Update config to accept `algorithm` field
- [x] Write comparative test: same load â†’ different algorithm behaviors

### 3.9 Final (v0.0.3)
- [x] E2E test: least_latency balancer with health checks + circuit breaker
- [x] E2E test: retry with multiple upstream targets
- [x] Benchmark: 10K req/sec with all features enabled
- [ ] Tag `v0.0.3`

---

## v0.0.4 â€” Transformation & Plugins

### 4.1 Request Transform Plugin
- [x] Implement `RequestTransform` plugin struct (Phase: PreProxy, Priority: 40)
- [x] Implement header manipulation: add, remove, rename headers
- [x] Implement query parameter manipulation: add, remove params
- [x] Implement path rewriting: regex match + replacement (`regexp.ReplaceAllString`)
- [x] Write unit tests for each manipulation type

### 4.2 Response Transform Plugin
- [x] Implement `ResponseTransform` plugin struct (Phase: PostProxy, Priority: 40)
- [x] Implement response header manipulation: add, remove headers
- [x] Implement response body interception (buffer response, transform, re-write)
- [x] Implement `ResponseWriter` wrapper that captures body for transformation
- [x] Write unit tests

### 4.3 Body Template Engine
- [x] Implement `internal/pkg/template/template.go`
- [x] Implement variable substitution: `$body`, `$timestamp_ms`, `$timestamp_iso`, `$upstream_latency_ms`, `$consumer_id`, `$route_name`, `$request_id`, `$remote_addr`, `$header.X-Custom`
- [x] Implement JSON body transformation: `add`, `remove`, `rename` fields
- [x] Implement `template` mode: full body replacement with variable injection
- [x] Implement JSON path traversal for nested field operations
- [x] Write unit tests: each variable type, add/remove/rename, nested fields, full template

### 4.4 URL Rewrite Plugin
- [x] Implement `URLRewrite` plugin struct (Phase: PreProxy, Priority: 35)
- [x] Implement regex-based path rewriting with capture groups
- [x] Implement query string preservation across rewrite
- [x] Write unit tests: simple rewrite, capture groups, query string handling

### 4.5 Plugin Pipeline Architecture (Formalized)
- [x] Implement `Pipeline` struct with global + per-route plugin chains
- [x] Implement `Pipeline.Execute(ctx)` â€” phase-ordered execution with abort support
- [x] Implement `Pipeline.ExecutePostProxy(ctx)` â€” post-proxy phase
- [x] Implement `RequestContext` with all fields (Request, Response, Route, Service, Consumer, Metadata)
- [x] Implement `ctx.Aborted` flag + `ctx.AbortReason` for pipeline short-circuit
- [x] Implement plugin config merging (global + route-level, route overrides global)
- [x] Write integration test: full pipeline with auth â†’ rate limit â†’ transform â†’ proxy â†’ response transform

### 4.6 Request Size Limit Plugin
- [x] Implement `RequestSizeLimit` plugin struct (Phase: PreProxy, Priority: 25)
- [x] Check `Content-Length` header + enforce max body bytes
- [x] Return 413 Payload Too Large on exceed
- [x] Write unit tests

### 4.7 Request Validator Plugin
- [x] Implement `RequestValidator` plugin struct (Phase: PreProxy, Priority: 30)
- [x] Implement basic JSON Schema validation (type checking, required fields, string format)
- [x] Return 400 Bad Request with validation errors
- [x] Write unit tests: valid payload, invalid type, missing required field

### 4.8 Compression Plugin
- [x] Implement `Compression` plugin struct (Phase: PostProxy, Priority: 50)
- [x] Implement gzip compression (`compress/gzip`)
- [x] Implement minimum size threshold (don't compress tiny responses)
- [x] Check `Accept-Encoding` header, set `Content-Encoding` + `Vary`
- [x] Implement `ResponseWriter` wrapper that compresses on write
- [x] Write unit tests: gzip enabled, below threshold, unsupported encoding

### 4.9 Correlation ID Plugin
- [x] Implement `CorrelationID` plugin struct (Phase: PreAuth, Priority: 0)
- [x] Generate UUID if no `X-Request-ID` header present
- [x] Pass through existing `X-Request-ID` if present
- [x] Set `ctx.CorrelationID`
- [x] Add to upstream request headers + response headers
- [x] Write unit tests

### 4.10 Bot Detection Plugin
- [x] Implement `BotDetect` plugin struct (Phase: PreAuth, Priority: 3)
- [x] Implement User-Agent pattern matching (known bot strings)
- [x] Implement configurable: allow list, deny list, action (block/flag)
- [x] Return 403 for blocked bots
- [x] Write unit tests

### 4.11 Redirect Plugin
- [x] Implement `Redirect` plugin struct (Phase: PreProxy)
- [x] Implement configurable redirect rules (path â†’ URL, status code 301/302/307/308)
- [x] Write unit tests

### 4.12 Final (v0.0.4)
- [x] E2E test: request transform â†’ proxy â†’ response transform pipeline
- [x] E2E test: JSON Schema validation + correlation ID propagation
- [ ] Tag `v0.0.4`

---

## v0.0.5 â€” Multi-Tenant Users & Credits

### 5.1 Embedded SQLite
- [x] Bundle SQLite amalgamation (`sqlite3.c` + `sqlite3.h`) in `internal/store/`
- [x] Implement minimal CGO wrapper for `database/sql` driver registration
- [x] Implement `store.Open(cfg) (*Store, error)` â€” open DB, set PRAGMAs (WAL, busy_timeout, foreign_keys)
- [x] Implement `store.migrate()` â€” run schema migrations sequentially
- [x] Implement `schema_migrations` table to track applied migrations
- [x] Write unit tests with in-memory SQLite (`:memory:`)

### 5.2 User Management
- [x] Implement `User` struct (ID, Email, Name, Company, PasswordHash, Role, Status, CreditBalance, RateLimits, IPWhitelist, Metadata, timestamps)
- [x] Implement `UserRepo.Create(user)` â€” insert with generated UUID
- [x] Implement `UserRepo.FindByID(id)` â€” select by primary key
- [x] Implement `UserRepo.FindByEmail(email)` â€” select by unique email
- [x] Implement `UserRepo.List(opts)` â€” paginated list with search (LIKE), filter (status, role), sort, total count
- [x] Implement `UserRepo.Update(user)` â€” update fields, set updated_at
- [x] Implement `UserRepo.Delete(id)` â€” soft delete (set status=deleted) or hard delete
- [x] Implement `UserRepo.UpdateStatus(id, status)` â€” suspend/activate
- [x] Implement password hashing: SHA-256 + random salt (16 bytes) stored as `salt:hash`
- [x] Implement password verification: split salt, re-hash, constant-time compare
- [x] Implement initial admin user creation on first boot (if no admin exists)
- [x] Write unit tests for all UserRepo methods

### 5.3 API Key Management (User-Based)
- [x] Implement `APIKey` struct (ID, UserID, KeyHash, KeyPrefix, Name, Status, ExpiresAt, LastUsedAt, LastUsedIP)
- [x] Implement key generation: `ck_live_` + 32 crypto/rand chars (production), `ck_test_` + 32 chars (test)
- [x] Implement key hashing: SHA-256 of full key (stored in DB)
- [x] Implement key prefix: first 12 chars of full key (for display: `ck_live_ab3x...`)
- [x] Implement `APIKeyRepo.Create(userID, name, mode) (fullKey, *APIKey, error)` â€” return full key ONCE
- [x] Implement `APIKeyRepo.FindByHash(hash)` â€” lookup for auth
- [x] Implement `APIKeyRepo.ListByUser(userID)` â€” user's keys (prefix only, never full key)
- [x] Implement `APIKeyRepo.Revoke(id)` â€” set status=revoked
- [x] Implement `APIKeyRepo.UpdateLastUsed(id, ip)` â€” async update
- [x] Update `AuthAPIKey` plugin to use SQLite store instead of YAML consumers
- [x] Write unit tests

### 5.4 Credit System
- [x] Implement `CreditTransaction` struct (ID, UserID, Type, Amount, BalanceBefore, BalanceAfter, Description, RequestID, RouteID, CreatedAt)
- [x] Implement `UserRepo.UpdateCreditBalance(userID, delta) (newBalance, error)` â€” atomic SQL UPDATE with balance check
- [x] Implement `CreditRepo.Create(txn)` â€” record transaction
- [x] Implement `CreditRepo.ListByUser(userID, opts)` â€” paginated transaction history
- [x] Implement `CreditRepo.OverviewStats()` â€” total distributed, consumed, top consumers
- [x] Implement credit engine: `billing.Engine.CalculateCost(ctx) int64` â€” route cost â†’ method multiplier â†’ default
- [x] Implement credit check in pipeline: after auth, before proxy
- [x] Implement credit deduction in post-proxy: after successful response
- [x] Implement zero-balance handling: reject (402) or allow_with_flag
- [x] Implement test key bypass: `ck_test_` keys skip credit deduction
- [x] Implement `BillingConfig` parsing from YAML (default_cost, route_costs, method_multipliers)
- [x] Write unit tests: cost calculation, deduction, insufficient balance, test key bypass

### 5.5 Endpoint Permissions
- [x] Implement `EndpointPermission` struct (ID, UserID, RouteID, Methods, Allowed, RateLimits, CreditCost, ValidFrom/Until, AllowedDays/Hours)
- [x] Implement `PermissionRepo.Create/Update/Delete/FindByUserAndRoute/ListByUser`
- [x] Implement `PermissionRepo.BulkAssign(userID, permissions)` â€” transaction-based bulk insert
- [x] Implement permission check in pipeline: after auth, check user has access to this route + method
- [x] Implement whitelist mode: user can ONLY access granted endpoints
- [x] Implement time/day restriction check
- [x] Implement per-endpoint rate limit override (use endpoint limit > user limit > global limit)
- [x] Implement per-endpoint credit cost override
- [x] Return 403 with descriptive error on permission denied
- [x] Write unit tests: whitelist hit/miss, method restriction, time restriction, rate limit override

### 5.6 User IP Whitelist
- [x] Implement user-level IP whitelist: stored as JSON array in users.ip_whitelist
- [x] Implement check in pipeline: if user has IP whitelist, verify client IP is in list
- [x] Implement CIDR range support in whitelist
- [x] Return 403 with "IP not allowed" on mismatch
- [x] Write unit tests

### 5.7 Admin API: User/Credit/Permission Endpoints
- [x] Implement `GET /admin/api/v1/users` â€” list users (search, filter, paginate)
- [x] Implement `POST /admin/api/v1/users` â€” create user (email, name, role, initial credits, password)
- [x] Implement `GET /admin/api/v1/users/{id}` â€” user detail
- [x] Implement `PUT /admin/api/v1/users/{id}` â€” update user
- [x] Implement `DELETE /admin/api/v1/users/{id}` â€” delete user
- [x] Implement `POST /admin/api/v1/users/{id}/suspend` â€” suspend
- [x] Implement `POST /admin/api/v1/users/{id}/activate` â€” activate
- [x] Implement `POST /admin/api/v1/users/{id}/reset-password` â€” admin reset
- [x] Implement `GET/POST/DELETE /admin/api/v1/users/{id}/api-keys[/{keyId}]` â€” key CRUD
- [x] Implement `GET/POST/PUT/DELETE /admin/api/v1/users/{id}/permissions[/{pid}]` â€” permission CRUD
- [x] Implement `POST /admin/api/v1/users/{id}/permissions/bulk` â€” bulk assign
- [x] Implement `GET/POST/DELETE /admin/api/v1/users/{id}/ip-whitelist[/{ip}]` â€” IP management
- [x] Implement `GET /admin/api/v1/credits/overview` â€” platform credit overview
- [x] Implement `POST /admin/api/v1/users/{id}/credits/topup` â€” add credits
- [x] Implement `POST /admin/api/v1/users/{id}/credits/deduct` â€” deduct credits
- [x] Implement `GET /admin/api/v1/users/{id}/credits/balance` â€” balance
- [x] Implement `GET /admin/api/v1/users/{id}/credits/transactions` â€” transaction history
- [x] Implement `GET/PUT /admin/api/v1/billing/config` â€” billing configuration
- [x] Implement `GET/PUT /admin/api/v1/billing/route-costs` â€” per-route costs
- [x] Write integration tests for all new endpoints

### 5.8 Final (v0.0.5)
- [x] E2E test: create user â†’ generate API key â†’ make request â†’ credit deducted â†’ verify log
- [x] E2E test: permission denied â†’ 403 with correct reason
- [x] E2E test: zero balance â†’ 402 rejection
- [x] E2E test: test key (ck_test_) â†’ no credit deduction
- [x] Tag `v0.0.5`

---

## v0.0.6 â€” Audit Logging & Analytics

### 6.1 Audit Logger
- [x] Implement `audit.Logger` struct with buffered channel (10K capacity)
- [x] Implement `AuditEntry` struct (all fields from SPECIFICATION Â§19.1)
- [x] Implement `Logger.Log(ctx)` â€” build entry from RequestContext, send to buffer (non-blocking)
- [x] Implement flush loop: batch collect from channel â†’ `AuditRepo.BatchInsert(batch)` every 1s or 100 items
- [x] Implement `ResponseWriter` wrapper that captures response body for audit
- [x] Implement request body capture (read + buffer + re-wrap as new ReadCloser)
- [x] Integrate into pipeline.ExecutePostProxy â€” always log after response

### 6.2 Sensitive Data Masking
- [x] Implement `audit.Masker` struct with header list + body field list + replacement string
- [x] Implement header masking: replace values of configured headers with `***REDACTED***`
- [x] Implement JSON body field masking: traverse JSON, replace values of matching keys
- [x] Implement nested field support (e.g., `user.password`, `payment.credit_card`)
- [x] Write unit tests: mask headers, mask top-level fields, mask nested fields

### 6.3 Audit Log Repository
- [x] Implement `AuditRepo.BatchInsert(entries)` â€” single transaction, prepared statement, batch insert
- [x] Implement `AuditRepo.Search(filters) ([]AuditEntry, total, error)` â€” dynamic WHERE clause builder
- [x] Support filters: user_id, api_key_prefix, route, method, status_code range, client_ip, blocked, block_reason, date range, min_latency, full-text search (path/body LIKE)
- [x] Implement `AuditRepo.FindByID(id)` â€” full detail with req/res bodies
- [x] Implement `AuditRepo.Stats(filters)` â€” aggregate: total requests, error rate, avg latency, top routes, top users
- [x] Implement `AuditRepo.DeleteOlderThan(cutoff, batchSize)` â€” batch deletion
- [x] Implement `AuditRepo.Export(filters, format)` â€” stream results as CSV/JSON/JSONL
- [x] Write unit tests

### 6.4 Log Retention & Cleanup
- [x] Implement retention scheduler: ticker â†’ check retention config â†’ delete expired logs
- [x] Implement per-route retention override (e.g., AI routes â†’ 90d, health checks â†’ 1d)
- [x] Implement batch deletion (configurable batch size, avoid long-running transactions)
- [x] Write unit test: insert old logs â†’ run cleanup â†’ verify deleted

### 6.5 Log Archival
- [x] Implement archive: before deletion, export to JSONL files
- [x] Implement gzip compression for archive files
- [x] Implement archive directory management (date-based filenames)
- [x] Write unit test

### 6.6 Analytics Engine
- [x] Implement `analytics.Engine` struct (ring buffer + time-series store + counters)
- [x] Implement `RingBuffer[RequestMetric]` â€” fixed size (default 100K), lock-free push
- [x] Implement `TimeSeriesStore` â€” per-minute buckets, auto-cleanup of old buckets
- [x] Implement `Bucket` aggregation: requests, errors, latency percentiles (p50/p95/p99), status codes, bytes
- [x] Implement real-time atomic counters: totalRequests, activeConns, totalErrors
- [x] Implement `Engine.Record(metric)` â€” push to ring buffer + update time-series + increment counters
- [x] Integrate into pipeline: after each request, record analytics metric

### 6.7 Analytics API Endpoints
- [x] Implement `GET /admin/api/v1/analytics/overview` â€” KPIs: total req, active conn, error rate, avg latency, credits consumed
- [x] Implement `GET /admin/api/v1/analytics/timeseries` â€” time-series data with configurable window + granularity
- [x] Implement `GET /admin/api/v1/analytics/top-routes` â€” top N routes by request count
- [x] Implement `GET /admin/api/v1/analytics/top-consumers` â€” top N users by request count
- [x] Implement `GET /admin/api/v1/analytics/errors` â€” error breakdown by status code + route
- [x] Implement `GET /admin/api/v1/analytics/latency` â€” percentile data (p50, p95, p99)
- [x] Implement `GET /admin/api/v1/analytics/throughput` â€” requests per second over time
- [x] Implement `GET /admin/api/v1/analytics/status-codes` â€” status code distribution

### 6.8 Audit Log API Endpoints
- [x] Implement `GET /admin/api/v1/audit-logs` â€” search with all filters
- [x] Implement `GET /admin/api/v1/audit-logs/{id}` â€” full request/response detail
- [x] Implement `GET /admin/api/v1/audit-logs/export` â€” download as CSV/JSON/JSONL
- [x] Implement `GET /admin/api/v1/audit-logs/stats` â€” log statistics
- [x] Implement `DELETE /admin/api/v1/audit-logs/cleanup` â€” trigger manual cleanup
- [x] Implement `GET /admin/api/v1/users/{id}/audit-logs` â€” user-scoped log search
- [x] Write integration tests

### 6.9 Final (v0.0.6)
- [x] E2E test: make requests â†’ verify audit logs captured with correct data
- [x] E2E test: masked headers/body fields in audit log
- [x] E2E test: retention cleanup deletes old logs
- [x] E2E test: analytics timeseries returns correct aggregation
- [ ] Tag `v0.0.6`

---

## v0.0.7 â€” Web Dashboard (Admin Panel)

### 7.1 React Project Setup
- [ ] Initialize Vite 6 + React 19 + TypeScript project in `web/`
- [ ] Install Tailwind CSS v4.1 + `@tailwindcss/vite`
- [ ] Initialize shadcn/ui (`components.json`, CSS variables, `cn()` utility)
- [ ] Install Lucide React icons
- [ ] Install Geist font (sans + mono)
- [ ] Install React Router v7
- [ ] Install TanStack Query v5
- [ ] Install TanStack Table v8
- [ ] Install Zustand
- [ ] Install React Hook Form + Zod + @hookform/resolvers
- [ ] Install Recharts 2.x
- [ ] Install CodeMirror 6 (`@codemirror/lang-json`, `@codemirror/lang-yaml`, `@codemirror/theme-one-dark`)
- [ ] Install Sonner (toast notifications)
- [ ] Install date-fns
- [ ] Configure `globals.css` with shadcn/ui CSS variables + dark/light theme colors (purple palette)
- [ ] Configure `vite.config.ts` with build output to `dist/`

### 7.2 Shared Infrastructure
- [ ] Implement `lib/utils.ts` â€” `cn()` class merger, date formatters, byte formatters
- [ ] Implement `lib/api.ts` â€” fetch wrapper with admin API key, error handling, base URL config
- [ ] Implement `lib/ws.ts` â€” WebSocket client with auto-reconnect
- [ ] Implement `lib/constants.ts` â€” route paths, color tokens, config
- [ ] Implement `stores/auth.ts` â€” Zustand: admin session state
- [ ] Implement `stores/theme.ts` â€” Zustand: dark/light toggle + system preference detection
- [ ] Implement `stores/realtime.ts` â€” Zustand: WebSocket live data
- [ ] Implement `hooks/use-media-query.ts` â€” responsive breakpoint hook

### 7.3 shadcn/ui Components
- [ ] Add all required shadcn/ui components: Button, Card, Dialog, Table, Input, Select, Badge, Tabs, Sidebar, Command, Sheet, AlertDialog, DropdownMenu, ScrollArea, Skeleton, Sonner, Resizable, Accordion, Collapsible, Switch, Checkbox, Slider, Popover, HoverCard, Progress, Breadcrumb, Separator, ToggleGroup, Tooltip, RadioGroup, Textarea, Form
- [ ] Configure theme: Deep Purple primary, Crimson destructive, Emerald success, Amber warning

### 7.4 Layout Components
- [ ] Implement `AdminLayout.tsx` â€” sidebar + header + main content + breadcrumb
- [ ] Implement `AppSidebar.tsx` â€” collapsible sidebar with Lucide icons, navigation links, active state
- [ ] Implement `Header.tsx` â€” top bar: search (Command âŒ˜K), theme toggle (Sun/Moon), admin info
- [ ] Implement `ThemeProvider.tsx` â€” dark/light theme context
- [ ] Implement mobile responsive: sidebar collapses at lg:, Sheet menu at md:

### 7.5 Shared UI Components
- [ ] Implement `DataTable.tsx` â€” TanStack Table wrapper with sorting, filtering, column visibility
- [ ] Implement `DataTablePagination.tsx` â€” page controls, per-page selector
- [ ] Implement `DataTableToolbar.tsx` â€” search input, filter dropdowns, column toggle, export button
- [ ] Implement `DataTableExport.tsx` â€” CSV/JSON export from table data
- [ ] Implement `KPICard.tsx` â€” metric card with value, label, trend indicator, Lucide icon
- [ ] Implement `StatusBadge.tsx` â€” color-coded status: active=green, suspended=red, pending=amber
- [ ] Implement `CopyButton.tsx` â€” click-to-copy with Sonner toast feedback
- [ ] Implement `TimeAgo.tsx` â€” relative time display (date-fns)
- [ ] Implement `CreditBadge.tsx` â€” credit cost/balance display
- [ ] Implement `EmptyState.tsx` â€” illustration + description + action button
- [ ] Implement `LoadingState.tsx` â€” Skeleton grid matching content layout
- [ ] Implement `ErrorState.tsx` â€” Alert (destructive) + retry button
- [ ] Implement `ConfirmDialog.tsx` â€” reusable AlertDialog for destructive actions

### 7.6 Chart Components
- [ ] Implement `AreaChart.tsx` â€” Recharts: real-time traffic (responsive, themed)
- [ ] Implement `BarChart.tsx` â€” Recharts: credit usage, error breakdown
- [ ] Implement `LineChart.tsx` â€” Recharts: latency trends
- [ ] Implement `PieChart.tsx` â€” Recharts: status code distribution
- [ ] Implement `HeatmapChart.tsx` â€” Recharts: latency heatmap (ScatterChart based)
- [ ] All charts: dark/light theme aware, responsive, configurable time window

### 7.7 Editor Components
- [ ] Implement `YAMLEditor.tsx` â€” CodeMirror 6 with YAML syntax, dark/light theme
- [ ] Implement `JSONViewer.tsx` â€” CodeMirror 6 read-only with JSON syntax
- [ ] Implement `DiffViewer.tsx` â€” side-by-side config diff

### 7.8 TanStack Query Hooks
- [ ] Implement `use-services.ts` â€” CRUD queries + mutations for services
- [ ] Implement `use-routes.ts` â€” CRUD queries + mutations for routes
- [ ] Implement `use-upstreams.ts` â€” CRUD queries + mutations for upstreams
- [ ] Implement `use-users.ts` â€” CRUD queries + mutations for users
- [ ] Implement `use-credits.ts` â€” balance, topup, transactions queries
- [ ] Implement `use-audit-logs.ts` â€” search, detail, export queries
- [ ] Implement `use-analytics.ts` â€” overview, timeseries, top routes queries

### 7.9 Admin Pages
- [ ] Implement `admin/Dashboard.tsx` â€” KPI cards (requests, users, credits, errors), traffic chart, top routes table, live request tail
- [ ] Implement `admin/Services.tsx` â€” service list (DataTable), create dialog, inline status badges
- [ ] Implement `admin/ServiceDetail.tsx` â€” service config, associated routes, edit form
- [ ] Implement `admin/Routes.tsx` â€” route list, create dialog, plugin badges
- [ ] Implement `admin/RouteDetail.tsx` â€” route config, plugin configuration, edit form
- [ ] Implement `admin/Upstreams.tsx` â€” upstream list, health indicators, target management
- [ ] Implement `admin/UpstreamDetail.tsx` â€” targets list, health check config, algorithm selector
- [ ] Implement `admin/Consumers.tsx` â€” consumer list, API key management
- [ ] Implement `admin/Plugins.tsx` â€” global plugin list, enable/disable, config editor
- [ ] Implement `admin/Users.tsx` â€” user table with Tabs (All/Active/Suspended), search, create dialog
- [ ] Implement `admin/UserDetail.tsx` â€” Tabs: Profile | API Keys | Permissions | Credits | Logs
- [ ] Implement `admin/Credits.tsx` â€” platform overview cards, credit chart, transaction table, pricing editor
- [ ] Implement `admin/AuditLogs.tsx` â€” log table with filters (Combobox + DatePicker), request detail Sheet
- [ ] Implement `admin/AuditLogDetail.tsx` â€” full req/res in Sheet with Tabs (Request | Response | Timing | Credits)
- [ ] Implement `admin/Analytics.tsx` â€” time-series charts, status code pie, latency heatmap, top routes/consumers
- [ ] Implement `admin/Config.tsx` â€” YAML editor with validation, diff view, apply with confirmation
- [ ] Implement `admin/Settings.tsx` â€” portal config, billing settings, retention policies

### 7.10 Go: Embed & Serve Dashboard
- [ ] Implement `embed.go` â€” `//go:embed web/dist/*`
- [ ] Implement SPA serving in admin server: file server + fallback to index.html
- [ ] Implement WebSocket endpoint for real-time dashboard updates
- [ ] Implement WebSocket: broadcast new request metrics, health changes
- [ ] Build pipeline: `npm run build` â†’ embed in Go binary

### 7.11 Final (v0.0.7)
- [ ] Verify dashboard loads in browser (dark + light mode)
- [ ] Verify all CRUD operations work end-to-end through UI
- [ ] Verify responsive layout on mobile/tablet/desktop
- [ ] Tag `v0.0.7`

---

## v0.0.8 â€” User Portal & Playground

### 8.1 Portal Backend
- [ ] Implement `portal.Server` struct with session-based auth
- [ ] Implement session management: `sessions` table, cookie-based, configurable max_age
- [ ] Implement `POST /portal/api/v1/auth/login` â€” email + password â†’ create session â†’ set cookie
- [ ] Implement `POST /portal/api/v1/auth/logout` â€” delete session
- [ ] Implement `GET /portal/api/v1/auth/me` â€” current user info
- [ ] Implement session middleware: cookie â†’ hash â†’ lookup â†’ load user â†’ inject into context
- [ ] Implement all portal API endpoints (API keys, APIs, playground, usage, logs, credits, security, settings)
- [ ] Implement `POST /portal/api/v1/playground/send` â€” proxy test request on behalf of user (using their API key)
- [ ] Implement playground templates CRUD (save/load/delete)
- [ ] Write integration tests for portal auth flow + all endpoints

### 8.2 Portal Frontend Pages
- [ ] Implement `portal/Login.tsx` â€” email + password form, error handling, redirect on success
- [ ] Implement `PortalLayout.tsx` â€” portal sidebar + header (different from admin)
- [ ] Implement `portal/Dashboard.tsx` â€” KPI cards (balance, requests today, error rate), mini usage chart
- [ ] Implement `portal/APIKeys.tsx` â€” key list, generate (Dialog showing key ONCE), rename (inline edit), revoke (AlertDialog)
- [ ] Implement `portal/APIs.tsx` â€” Card grid of available endpoints with method badges, credit cost, rate limit
- [ ] Implement `portal/Playground.tsx` â€” full playground (see below)
- [ ] Implement `portal/Usage.tsx` â€” Recharts: request count, credit consumption, error rate over time
- [ ] Implement `portal/Logs.tsx` â€” DataTable: user's request logs with search/filter
- [ ] Implement `portal/LogDetail.tsx` â€” Sheet: full req/res detail with JSON viewer
- [ ] Implement `portal/Credits.tsx` â€” large balance display, transaction table, usage forecast chart
- [ ] Implement `portal/Security.tsx` â€” IP whitelist editor, activity log (logins, key events)
- [ ] Implement `portal/Settings.tsx` â€” profile form (React Hook Form), change password dialog

### 8.3 API Playground Component
- [ ] Implement `PlaygroundView.tsx` â€” Resizable split pane (request left, response right)
- [ ] Implement `RequestBuilder.tsx` â€” method Select, URL Input with route autocomplete (Command)
- [ ] Implement `HeaderEditor.tsx` â€” dynamic key-value rows, add/remove, auto-fill X-API-Key
- [ ] Implement `BodyEditor.tsx` â€” CodeMirror 6 JSON editor with syntax highlighting
- [ ] Implement query parameter editor (key-value pairs)
- [ ] Implement credit cost preview Badge (updates on endpoint selection)
- [ ] Implement Send button â†’ call portal playground API â†’ display response
- [ ] Implement `ResponseViewer.tsx` â€” Tabs: Body (JSONViewer) | Headers (table) | Timing (latency breakdown)
- [ ] Implement status Badge (color-coded: 2xx=green, 4xx=amber, 5xx=red)
- [ ] Implement `TemplateManager.tsx` â€” save/load/delete request templates Dialog

### 8.4 Go: Portal Embed & Serve
- [ ] Extend `embed.go` to include portal assets (same React app, different routes)
- [ ] Implement SPA serving for `/portal/*` routes
- [ ] Verify portal works on configured port/path

### 8.5 Final (v0.0.8)
- [ ] E2E test: user login â†’ generate key â†’ test in playground â†’ view logs â†’ check credits
- [ ] Verify portal responsive on mobile
- [ ] Tag `v0.0.8`

---

## v0.0.9 â€” Topology & Flow Visualization

### 9.1 React Flow Setup
- [ ] Install `@xyflow/react` (React Flow)
- [ ] Implement custom node types: GatewayNode, ServiceNode, UpstreamNode, PluginNode, ClusterNode
- [ ] Implement custom edge types: TrafficEdge (animated dots), RaftEdge (heartbeat)
- [ ] Implement consistent styling: dark/light theme aware, shadcn/ui color tokens

### 9.2 Plugin Pipeline View
- [ ] Implement `PipelineView.tsx` â€” React Flow showing plugin execution order for a route
- [ ] Node per plugin: icon, name, phase, config summary
- [ ] Animated edge between nodes (request flow direction)
- [ ] Click plugin node â†’ open config editor Dialog
- [ ] Integrate into `admin/RouteDetail.tsx`

### 9.3 Upstream Health Map
- [ ] Implement `UpstreamMap.tsx` â€” React Flow: gateway center, targets in ring
- [ ] Edge thickness proportional to traffic volume
- [ ] Node color: green=healthy, yellow=degraded, red=down
- [ ] Click target â†’ side panel with health history, latency chart
- [ ] Integrate into `admin/UpstreamDetail.tsx`

### 9.4 Service Dependency Graph
- [ ] Implement `ServiceGraph.tsx` â€” React Flow: Services â†’ Routes â†’ Upstreams
- [ ] Auto-layout using dagre/elk algorithm
- [ ] Click any node â†’ navigate to detail page
- [ ] Integrate into `admin/Services.tsx` as toggle view (table/graph)

### 9.5 Cluster Topology (Placeholder)
- [ ] Implement `ClusterTopology.tsx` â€” React Flow: single node (standalone mode)
- [ ] Prepare for v0.5.0: node types for Leader/Follower/Unhealthy
- [ ] Integrate into `admin/Cluster.tsx`

### 9.6 WebSocket Real-Time Feed
- [ ] Implement Go WebSocket endpoint: `/admin/api/v1/ws`
- [ ] Implement server-side: broadcast latest request metrics every 1s
- [ ] Implement server-side: broadcast health check changes immediately
- [ ] Implement client-side: `use-realtime.ts` hook â†’ Zustand store â†’ UI updates
- [ ] Implement live request tail in Dashboard (auto-scroll ScrollArea)
- [ ] Implement real-time chart updates (traffic graph)

### 9.7 Alert Rules Engine
- [ ] Implement `analytics.AlertEngine` â€” evaluate rules against time-series data
- [ ] Implement rule types: error_rate > X%, p99_latency > Xms, upstream_health < X%
- [ ] Implement action types: log, webhook (HTTP POST to configured URL)
- [ ] Implement cooldown: don't re-fire alert within cooldown period
- [ ] Implement alert history: store triggered alerts with timestamp + details
- [ ] Implement Admin API: `GET/POST/PUT/DELETE /admin/api/v1/alerts`
- [ ] Implement admin UI: alert configuration page, alert history table

### 9.8 Final (v0.0.9)
- [ ] Verify React Flow views render correctly (dark + light)
- [ ] Verify WebSocket real-time updates in dashboard
- [ ] Verify alert rules trigger correctly
- [ ] Tag `v0.0.9`

---

## v0.1.0 â€” MCP Server & CLI Completion

### 10.1 MCP Server
- [ ] Implement JSON-RPC 2.0 protocol handler (request parsing, response formatting)
- [ ] Implement `initialize` method (capabilities, server info)
- [ ] Implement `tools/list` â€” return all tool definitions with input schemas
- [ ] Implement `tools/call` â€” dispatch to tool handlers
- [ ] Implement `resources/list` â€” return all resource URIs
- [ ] Implement `resources/read` â€” return resource data
- [ ] Implement all gateway management tools (list/create/update/delete services, routes, upstreams)
- [ ] Implement all user management tools (list/create/update/suspend users, API keys, permissions)
- [ ] Implement all credit tools (overview, balance, topup, deduct, transactions)
- [ ] Implement all audit tools (search, detail, stats, cleanup)
- [ ] Implement all analytics tools (overview, top routes, errors, latency)
- [ ] Implement cluster tools (status, nodes â€” placeholder for v0.5.0)
- [ ] Implement system tools (status, config export/import, reload)
- [ ] Implement stdio transport: read JSON-RPC from stdin, write to stdout
- [ ] Implement SSE transport: HTTP server with Server-Sent Events
- [ ] Write unit tests for each tool

### 10.2 CLI Completion
- [ ] Implement `apicerberus user list` â€” list users (table format)
- [ ] Implement `apicerberus user create --email --name --credits`
- [ ] Implement `apicerberus user get <id>`
- [ ] Implement `apicerberus user update <id> --rate-limit-rps`
- [ ] Implement `apicerberus user suspend/activate <id>`
- [ ] Implement `apicerberus user apikey list/create/revoke`
- [ ] Implement `apicerberus user permission list/grant/revoke`
- [ ] Implement `apicerberus user ip list/add/remove`
- [ ] Implement `apicerberus credit overview/balance/topup/deduct/transactions`
- [ ] Implement `apicerberus audit search/tail/detail/export/stats/cleanup/retention`
- [ ] Implement `apicerberus analytics overview/requests/latency`
- [ ] Implement `apicerberus service/route/upstream list/add/get/update/delete` (if not done in v0.0.1)
- [ ] Implement `apicerberus config export/import/diff`
- [ ] Implement `apicerberus mcp start [--transport stdio|sse] [--port 3000]`
- [ ] Implement CLI table formatter (aligned columns, truncation)
- [ ] Implement CLI JSON output mode (`--output json`)

### 10.3 TLS & ACME
- [ ] Implement `TLSManager` struct with `tls.Config.GetCertificate` callback
- [ ] Implement manual cert loading: cert_file + key_file â†’ `tls.LoadX509KeyPair`
- [ ] Implement cert caching: `sync.Map[domain]*tls.Certificate`
- [ ] Implement cert disk storage: PEM files in acme_dir
- [ ] Implement ACME client: account creation, authorization, challenge solving (tls-alpn-01)
- [ ] Implement cert renewal: check expiry on GetCertificate, renew if <30 days
- [ ] Implement SNI-based virtual hosting (multiple domains)
- [ ] Implement HTTPS listener using `tls.NewListener`
- [ ] Write integration tests with self-signed certs

### 10.4 Config Export/Import
- [ ] Implement `GET /admin/api/v1/config/export` â€” current running config as YAML
- [ ] Implement `POST /admin/api/v1/config/import` â€” upload YAML, validate, apply
- [ ] Implement `apicerberus config diff old.yaml new.yaml` â€” diff two configs (line-by-line diff)
- [ ] Write tests

### 10.5 Final (v0.1.0)
- [ ] Verify MCP server works with Claude Code (`apicerberus mcp start`)
- [ ] Verify all CLI commands work
- [ ] Verify TLS termination with self-signed cert
- [ ] Tag `v0.1.0`

---

## v0.2.0 â€” gRPC Support

- [ ] Implement HTTP/2 prior knowledge listener (h2c) for gRPC
- [ ] Implement gRPC frame proxy: read HTTP/2 frames, forward to upstream
- [ ] Implement gRPC-Web support: translate gRPC-Web framing to native gRPC
- [ ] Implement gRPC health check protocol (grpc.health.v1.Health)
- [ ] Implement gRPC metadata manipulation (headers â†’ gRPC metadata mapping)
- [ ] Implement gRPC streaming support: unary, server-streaming, client-streaming, bidirectional
- [ ] Implement gRPC â†” JSON transcoding: REST request â†’ gRPC call â†’ JSON response
- [ ] Implement protocol auto-detection: content-type `application/grpc` â†’ gRPC path
- [ ] Implement gRPC-specific error mapping (gRPC status codes â†” HTTP status codes)
- [ ] Write integration tests with test gRPC service
- [ ] Tag `v0.2.0`

---

## v0.3.0 â€” GraphQL Support

- [ ] Implement GraphQL request detection (POST with `application/json` body containing `query` field)
- [ ] Implement GraphQL query proxy: parse query, forward to upstream GraphQL service
- [ ] Implement query depth analyzer: recursive AST traversal, enforce max_depth
- [ ] Implement query complexity analyzer: assign cost per field, enforce max_complexity
- [ ] Implement introspection control: block `__schema` and `__type` queries per config
- [ ] Implement field-level authorization: check user permissions against requested fields
- [ ] Implement automatic persisted queries (APQ): hash-based query caching
- [ ] Implement subscription proxying: WebSocket â†’ upstream WebSocket
- [ ] Implement `GraphQLGuard` plugin (depth + complexity + introspection in one plugin)
- [ ] Implement React Flow: GraphQL schema view (placeholder for federation)
- [ ] Write integration tests with test GraphQL service
- [ ] Tag `v0.3.0`

---

## v0.4.0 â€” GraphQL Federation

- [ ] Implement schema federation: fetch schemas from multiple upstream GraphQL services
- [ ] Implement schema composition: merge types, resolve conflicts
- [ ] Implement query planning: split incoming query across federated subgraphs
- [ ] Implement query execution: parallel fetch from subgraphs, merge results
- [ ] Implement query batching: combine multiple queries in single request
- [ ] Implement federated subgraph management (admin API + UI)
- [ ] Implement React Flow: federation schema visualization (subgraph relationships)
- [ ] Write integration tests with multiple test GraphQL services
- [ ] Tag `v0.4.0`

---

## v0.5.0 â€” Raft Clustering & HA

- [ ] Implement Raft node struct: state machine, term, log, commit index
- [ ] Implement leader election: request votes, majority wins, term management
- [ ] Implement log replication: AppendEntries RPC, commit, apply to FSM
- [ ] Implement Raft network transport: TCP connections between nodes
- [ ] Implement GatewayFSM: apply config changes, credit updates, rate limit sync
- [ ] Implement snapshotting: serialize state, compact log
- [ ] Implement snapshot restore: deserialize state on join
- [ ] Implement peer discovery: static config + dynamic join/leave
- [ ] Implement distributed rate limiting: cluster-wide counter sync via Raft
- [ ] Implement distributed credit balance: credit operations through Raft log
- [ ] Implement health check result sharing across cluster
- [ ] Implement cluster-wide analytics aggregation
- [ ] Implement audit log replication (or distributed write to local SQLite)
- [ ] Implement Admin API: cluster status, node list, join, leave, snapshot
- [ ] Implement React Flow: live cluster topology (Leader=purple, Follower=slate, Unhealthy=red, heartbeat animation)
- [ ] Write integration tests: 3-node cluster, leader election, failover, config sync
- [ ] Tag `v0.5.0`

---

## v0.6.0 â€” Advanced Features

- [ ] Implement response caching plugin: in-memory cache, cache-control header aware, key=method+path+query
- [ ] Implement cache invalidation: TTL, max size, manual purge API
- [ ] Implement Geo-aware load balancing: IP â†’ country mapping (bundled GeoIP data), select nearest target
- [ ] Implement Adaptive load balancing: monitor error rate + latency, auto-switch algorithm
- [ ] Implement Prometheus metrics export: `/metrics` endpoint with gateway metrics
- [ ] Implement OpenTelemetry tracing: span creation, context propagation, trace ID in headers
- [ ] Implement webhook notifications: HTTP POST on events (low balance, user created, alert triggered, upstream down)
- [ ] Implement webhook retry with backoff on failure
- [ ] Write tests for each feature
- [ ] Tag `v0.6.0`

---

## v0.7.0 â€” Monetization & Enterprise

- [ ] Implement self-purchase: credit packages config, purchase API endpoint, external payment webhook verification
- [ ] Implement usage-based billing reports: per-user credit consumption, revenue charts in admin dashboard
- [ ] Implement multi-workspace/organization: org entity, users belong to orgs, org-level billing
- [ ] Implement RBAC: custom roles beyond admin/user, configurable permissions per role
- [ ] Implement SSO / OAuth2 login for portal: authorization code flow, token exchange
- [ ] Implement white-label portal: custom logo, colors, domain per deployment
- [ ] Write tests
- [ ] Tag `v0.7.0`

---

## v1.0.0 â€” Production Release

- [ ] Achieve >80% test coverage across all packages
- [ ] Run performance benchmarks: verify 50K+ req/sec on single node
- [ ] Run security audit: check all auth paths, injection vectors, rate limit bypasses
- [ ] Create documentation site (docs.apicerberus.com) with guides, API reference, examples
- [ ] Create migration guides: Kong â†’ API Cerberus, Tyk â†’ API Cerberus, KrakenD â†’ API Cerberus
- [ ] Build multi-arch Docker images: linux/amd64, linux/arm64
- [ ] Create Helm chart for Kubernetes deployment
- [ ] Create docker-compose examples: standalone, 3-node cluster
- [ ] Write CHANGELOG.md (all versions)
- [ ] Create GitHub release with binaries (linux, darwin, windows Ã— amd64, arm64)
- [ ] Final README.md with installation, quickstart, screenshots, badges
- [ ] Tag `v1.0.0`

---

## Task Statistics

| Version | Tasks | Focus |
|---------|-------|-------|
| v0.0.1 | ~75 | Core gateway, proxy, routing, admin API |
| v0.0.2 | ~40 | Auth (API key + JWT), rate limiting, CORS |
| v0.0.3 | ~25 | 10 LB algorithms, circuit breaker, retry |
| v0.0.4 | ~30 | Transformation, plugin pipeline, compression |
| v0.0.5 | ~55 | SQLite, users, credits, permissions |
| v0.0.6 | ~35 | Audit logging, analytics engine |
| v0.0.7 | ~60 | Admin dashboard (React + shadcn/ui) |
| v0.0.8 | ~30 | User portal, API playground |
| v0.0.9 | ~20 | React Flow topologies, WebSocket, alerts |
| v0.1.0 | ~40 | MCP server, CLI, TLS/ACME |
| v0.2.0 | ~12 | gRPC support |
| v0.3.0 | ~12 | GraphQL support |
| v0.4.0 | ~8 | GraphQL federation |
| v0.5.0 | ~18 | Raft clustering |
| v0.6.0 | ~10 | Caching, geo LB, Prometheus, OTel |
| v0.7.0 | ~8 | Monetization, RBAC, SSO |
| v1.0.0 | ~12 | Testing, docs, Docker, Helm |
| **Total** | **~490** | |
