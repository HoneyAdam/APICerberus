package plugin

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	coerce "github.com/APICerberus/APICerebrus/internal/pkg/coerce"
	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/pkg/netutil"
	"github.com/APICerberus/APICerebrus/internal/ratelimit"
)

const (
	rateLimitAlgoTokenBucket   = "token_bucket"
	rateLimitAlgoFixedWindow   = "fixed_window"
	rateLimitAlgoSlidingWindow = "sliding_window"
	rateLimitAlgoLeakyBucket   = "leaky_bucket"
)

// RateLimitConfig configures RateLimit plugin behavior.
type RateLimitConfig struct {
	Algorithm         string
	Scope             string
	RequestsPerSecond int
	Burst             int
	Limit             int
	Window            time.Duration
	CompositeScopes   []string
}

// RateLimitRequest contains context used for scope-key resolution.
type RateLimitRequest struct {
	Request  *http.Request
	Route    *config.Route
	Consumer *config.Consumer
	Metadata map[string]any
}

// RateLimitDecision includes result metadata for headers and responses.
type RateLimitDecision struct {
	Allowed   bool
	Limit     int
	Remaining int
	ResetAt   time.Time
	ScopeKey  string
}

// RateLimitError indicates the request exceeded the configured limit.
type RateLimitError struct {
	PluginError
	RetryAfter time.Duration
}

type rateLimiter interface {
	Allow(key string) (bool, int, time.Time)
}

// stalePurger is implemented by in-memory rate limiters that support key eviction.
type stalePurger interface {
	PurgeStale(now time.Time)
}

// RateLimit plugin enforces token/fixed-window request limits.
type RateLimit struct {
	algorithm string
	scope     string
	limiter   rateLimiter
	limit     int
	composite []string
	now       func() time.Time

	mu      sync.RWMutex
	dynamic map[string]dynamicRateLimiter

	cleanupMu   sync.Mutex
	cleanupStop chan struct{}
}

const defaultPurgeInterval = 5 * time.Minute

type dynamicRateLimiter struct {
	limiter   rateLimiter
	limit     int
	scope     string
	composite []string
}

func NewRateLimit(cfg RateLimitConfig) (*RateLimit, error) {
	algo := strings.ToLower(strings.TrimSpace(cfg.Algorithm))
	if algo == "" {
		algo = rateLimitAlgoTokenBucket
	}

	scope := strings.ToLower(strings.TrimSpace(cfg.Scope))
	if scope == "" {
		scope = "global"
	}

	var (
		limiter rateLimiter
		limit   int
	)
	limiter, limit, err := NewRateLimiter(algo, cfg)
	if err != nil {
		return nil, err
	}

	composite := make([]string, 0, len(cfg.CompositeScopes))
	for _, c := range cfg.CompositeScopes {
		c = strings.ToLower(strings.TrimSpace(c))
		if c == "" {
			continue
		}
		composite = append(composite, c)
	}
	if len(composite) == 0 {
		composite = []string{"consumer", "ip", "route"}
	}

	return &RateLimit{
		algorithm: algo,
		scope:     scope,
		limiter:   limiter,
		limit:     limit,
		composite: composite,
		now:       time.Now,
		dynamic:   map[string]dynamicRateLimiter{},
	}, nil
}

// NewRateLimiter selects rate limit algorithm implementation from config.
func NewRateLimiter(algorithm string, cfg RateLimitConfig) (rateLimiter, int, error) {
	algo := strings.ToLower(strings.TrimSpace(algorithm))
	if algo == "" {
		algo = rateLimitAlgoTokenBucket
	}

	switch algo {
	case rateLimitAlgoTokenBucket:
		rps := cfg.RequestsPerSecond
		if rps <= 0 {
			rps = 10
		}
		burst := cfg.Burst
		if burst <= 0 {
			burst = rps
		}
		return ratelimit.NewTokenBucket(rps, burst), burst, nil
	case rateLimitAlgoFixedWindow:
		limit := cfg.Limit
		if limit <= 0 {
			limit = 10
		}
		window := cfg.Window
		if window <= 0 {
			window = time.Second
		}
		return ratelimit.NewFixedWindow(limit, window), limit, nil
	case rateLimitAlgoSlidingWindow:
		limit := cfg.Limit
		if limit <= 0 {
			limit = 10
		}
		window := cfg.Window
		if window <= 0 {
			window = time.Second
		}
		return ratelimit.NewSlidingWindow(limit, window), limit, nil
	case rateLimitAlgoLeakyBucket:
		capacity := cfg.Burst
		if capacity <= 0 {
			capacity = cfg.Limit
		}
		if capacity <= 0 {
			capacity = 10
		}
		leakRate := cfg.RequestsPerSecond
		if leakRate <= 0 {
			leakRate = capacity
		}
		return ratelimit.NewLeakyBucket(capacity, leakRate), capacity, nil
	default:
		return nil, 0, fmt.Errorf("unsupported rate limit algorithm %q", cfg.Algorithm)
	}
}

func (r *RateLimit) Name() string  { return "rate-limit" }
func (r *RateLimit) Phase() Phase  { return PhasePreProxy }
func (r *RateLimit) Priority() int { return 20 }

// StartCleanup launches a background goroutine that periodically purges stale
// rate limiter keys to prevent unbounded memory growth. Call StopCleanup when
// the plugin is no longer needed.
func (r *RateLimit) StartCleanup(interval time.Duration) {
	if r == nil {
		return
	}
	if interval <= 0 {
		interval = defaultPurgeInterval
	}
	r.cleanupMu.Lock()
	defer r.cleanupMu.Unlock()
	if r.cleanupStop != nil {
		return // already running
	}
	r.cleanupStop = make(chan struct{})
	go r.cleanupLoop(interval)
}

// StopCleanup terminates the background purge goroutine.
func (r *RateLimit) StopCleanup() {
	if r == nil {
		return
	}
	r.cleanupMu.Lock()
	defer r.cleanupMu.Unlock()
	if r.cleanupStop != nil {
		close(r.cleanupStop)
		r.cleanupStop = nil
	}
}

func (r *RateLimit) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-r.cleanupStop:
			return
		case <-ticker.C:
			r.purgeAll()
		}
	}
}

func (r *RateLimit) purgeAll() {
	now := r.now()
	r.purgeLimiter(r.limiter, now)

	r.mu.RLock()
	dynamicCopy := make([]rateLimiter, 0, len(r.dynamic))
	for _, d := range r.dynamic {
		dynamicCopy = append(dynamicCopy, d.limiter)
	}
	r.mu.RUnlock()

	for _, l := range dynamicCopy {
		r.purgeLimiter(l, now)
	}
}

func (r *RateLimit) purgeLimiter(l rateLimiter, now time.Time) {
	if p, ok := l.(stalePurger); ok {
		p.PurgeStale(now)
	}
}

// Check evaluates request without writing response.
func (r *RateLimit) Check(in RateLimitRequest) (*RateLimitDecision, error) {
	if r == nil || r.limiter == nil {
		return nil, fmt.Errorf("rate limiter is not initialized")
	}
	if in.Metadata != nil {
		if skip, ok := in.Metadata["skip_rate_limit"].(bool); ok && skip {
			return &RateLimitDecision{
				Allowed:   true,
				Limit:     0,
				Remaining: 0,
				ResetAt:   r.now(),
				ScopeKey:  "skipped",
			}, nil
		}
	}

	limiter, limit, scope, composite, err := r.effectiveLimiter(in)
	if err != nil {
		return nil, err
	}
	if limiter == nil {
		limiter = r.limiter
		limit = r.limit
		scope = r.scope
		composite = r.composite
	}

	scopeKey := scopeKeyFor(scope, composite, in)
	allowed, remaining, resetAt := limiter.Allow(scopeKey)
	decision := &RateLimitDecision{
		Allowed:   allowed,
		Limit:     limit,
		Remaining: remaining,
		ResetAt:   resetAt,
		ScopeKey:  scopeKey,
	}
	if allowed {
		return decision, nil
	}

	retryAfter := resetAt.Sub(r.now())
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	return decision, &RateLimitError{
		PluginError: PluginError{
			Code:    "rate_limit_exceeded",
			Message: "Rate limit exceeded",
			Status:  http.StatusTooManyRequests,
		},
		RetryAfter: retryAfter,
	}
}

// Enforce checks rate limit and writes headers + 429 response when exceeded.
func (r *RateLimit) Enforce(w http.ResponseWriter, in RateLimitRequest) bool {
	decision, err := r.Check(in)
	if decision != nil {
		r.writeHeaders(w, decision)
	}
	if err == nil {
		return true
	}

	rlErr, ok := err.(*RateLimitError)
	if !ok {
		if writeErr := jsonutil.WriteJSON(w, http.StatusTooManyRequests, map[string]any{
			"error": map[string]any{
				"code":    "rate_limit_failed",
				"message": "Rate limit check failed",
			},
		}); writeErr != nil {
			log.Printf("[ERROR] rate_limit: failed to write rate limit error response: %v", writeErr)
		}
		return false
	}

	retryAfterSeconds := int(math.Ceil(rlErr.RetryAfter.Seconds()))
	if retryAfterSeconds < 1 {
		retryAfterSeconds = 1
	}
	w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfterSeconds))
	if writeErr := jsonutil.WriteJSON(w, rlErr.Status, map[string]any{
		"error": map[string]any{
			"code":    rlErr.Code,
			"message": rlErr.Message,
		},
	}); writeErr != nil {
		log.Printf("[ERROR] rate_limit: failed to write rate limit response: %v", writeErr)
	}
	return false
}

func (r *RateLimit) writeHeaders(w http.ResponseWriter, decision *RateLimitDecision) {
	if w == nil || decision == nil {
		return
	}
	w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", decision.Limit))
	w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", decision.Remaining))
	w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", decision.ResetAt.Unix()))
}

func scopeKeyFor(scope string, composite []string, in RateLimitRequest) string {
	switch scope {
	case "global":
		return "global"
	case "consumer":
		return "consumer:" + consumerKey(in.Consumer)
	case "ip":
		return "ip:" + requestIP(in.Request)
	case "route":
		return "route:" + routeKey(in.Route, in.Request)
	case "composite":
		parts := make([]string, 0, len(composite))
		for _, item := range composite {
			switch item {
			case "global":
				parts = append(parts, "global")
			case "consumer":
				parts = append(parts, "consumer:"+consumerKey(in.Consumer))
			case "ip":
				parts = append(parts, "ip:"+requestIP(in.Request))
			case "route":
				parts = append(parts, "route:"+routeKey(in.Route, in.Request))
			}
		}
		if len(parts) == 0 {
			return "global"
		}
		return strings.Join(parts, "|")
	default:
		return "global"
	}
}

func (r *RateLimit) effectiveLimiter(in RateLimitRequest) (rateLimiter, int, string, []string, error) {
	if r == nil {
		return nil, 0, "", nil, nil
	}
	override := rateLimitConfigFromMetadata(in.Metadata)
	if len(override) > 0 {
		return r.dynamicLimiter(override)
	}
	userCfg := rateLimitConfigFromConsumer(in.Consumer)
	if len(userCfg) > 0 {
		return r.dynamicLimiter(userCfg)
	}
	return r.limiter, r.limit, r.scope, append([]string(nil), r.composite...), nil
}

func (r *RateLimit) dynamicLimiter(cfgMap map[string]any) (rateLimiter, int, string, []string, error) {
	key := normalizeRateLimitConfigKey(cfgMap)

	r.mu.RLock()
	if dynamic, ok := r.dynamic[key]; ok && dynamic.limiter != nil {
		r.mu.RUnlock()
		return dynamic.limiter, dynamic.limit, dynamic.scope, append([]string(nil), dynamic.composite...), nil
	}
	r.mu.RUnlock()

	cfg := RateLimitConfig{
		Algorithm:         coerce.AsString(cfgMap["algorithm"]),
		Scope:             coerce.AsString(cfgMap["scope"]),
		RequestsPerSecond: coerce.AsInt(cfgMap["requests_per_second"], 0),
		Burst:             coerce.AsInt(cfgMap["burst"], 0),
		Limit:             coerce.AsInt(cfgMap["limit"], 0),
		Window:            coerce.AsDuration(cfgMap["window"], 0),
		CompositeScopes:   coerce.AsStringSlice(cfgMap["composite_scopes"]),
	}
	if strings.TrimSpace(cfg.Algorithm) == "" {
		cfg.Algorithm = r.algorithm
	}
	if strings.TrimSpace(cfg.Scope) == "" {
		cfg.Scope = r.scope
	}
	if cfg.Window <= 0 {
		if coerce.AsInt(cfgMap["window_seconds"], 0) > 0 {
			cfg.Window = time.Duration(coerce.AsInt(cfgMap["window_seconds"], 0)) * time.Second
		}
	}
	if len(cfg.CompositeScopes) == 0 {
		cfg.CompositeScopes = append([]string(nil), r.composite...)
	}

	limiter, limit, err := NewRateLimiter(cfg.Algorithm, cfg)
	if err != nil {
		return nil, 0, "", nil, err
	}
	scope := strings.ToLower(strings.TrimSpace(cfg.Scope))
	if scope == "" {
		scope = "global"
	}
	composite := cfg.CompositeScopes
	if len(composite) == 0 {
		composite = []string{"consumer", "ip", "route"}
	}

	r.mu.Lock()
	r.dynamic[key] = dynamicRateLimiter{
		limiter:   limiter,
		limit:     limit,
		scope:     scope,
		composite: append([]string(nil), composite...),
	}
	r.mu.Unlock()
	return limiter, limit, scope, append([]string(nil), composite...), nil
}

func rateLimitConfigFromMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	raw, ok := metadata[metadataPermissionRateLimitOverride]
	if !ok {
		return nil
	}
	cfg, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	for k, v := range cfg {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func rateLimitConfigFromConsumer(consumer *config.Consumer) map[string]any {
	if consumer == nil || len(consumer.Metadata) == 0 {
		return nil
	}
	raw, ok := consumer.Metadata["rate_limits"]
	if !ok {
		return nil
	}
	cfg, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]any{}
	for k, v := range cfg {
		key := strings.TrimSpace(strings.ToLower(k))
		if key == "" {
			continue
		}
		out[key] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeRateLimitConfigKey(cfg map[string]any) string {
	if len(cfg) == 0 {
		return "empty"
	}
	keys := make([]string, 0, len(cfg))
	for k := range cfg {
		keys = append(keys, strings.ToLower(strings.TrimSpace(k)))
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+normalizeAny(cfg[k]))
	}
	return strings.Join(parts, ";")
}

func normalizeAny(value any) string {
	switch v := value.(type) {
	case nil:
		return "nil"
	case string:
		return strings.TrimSpace(strings.ToLower(v))
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 64)
	case []any:
		items := make([]string, 0, len(v))
		for _, item := range v {
			items = append(items, normalizeAny(item))
		}
		return "[" + strings.Join(items, ",") + "]"
	case []string:
		items := make([]string, 0, len(v))
		for _, item := range v {
			items = append(items, normalizeAny(item))
		}
		return "[" + strings.Join(items, ",") + "]"
	case map[string]any:
		return normalizeRateLimitConfigKey(v)
	default:
		return strings.TrimSpace(strings.ToLower(fmt.Sprint(value)))
	}
}

func consumerKey(consumer *config.Consumer) string {
	if consumer == nil {
		return "anonymous"
	}
	if value := strings.TrimSpace(consumer.ID); value != "" {
		return value
	}
	if value := strings.TrimSpace(consumer.Name); value != "" {
		return value
	}
	return "anonymous"
}

func routeKey(route *config.Route, req *http.Request) string {
	if route != nil {
		if value := strings.TrimSpace(route.ID); value != "" {
			return value
		}
		if value := strings.TrimSpace(route.Name); value != "" {
			return value
		}
	}
	if req != nil && strings.TrimSpace(req.URL.Path) != "" {
		return req.URL.Path
	}
	return "unknown"
}

func requestIP(req *http.Request) string {
	if req == nil {
		return "unknown"
	}
	if ip := netutil.ExtractClientIP(req); ip != "" {
		return ip
	}
	return "unknown"
}
