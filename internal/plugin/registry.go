package plugin

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// PipelineContext is shared mutable state passed through plugin chain.
type PipelineContext struct {
	Request        *http.Request
	ResponseWriter http.ResponseWriter
	Response       *http.Response
	Route          *config.Route
	Service        *config.Service
	Consumer       *config.Consumer
	CorrelationID  string
	Metadata       map[string]any
	Aborted        bool
	AbortReason    string
	Retry          *Retry
	Cleanup        []func()
}

func (c *PipelineContext) Abort(reason string) {
	if c == nil {
		return
	}
	c.Aborted = true
	c.AbortReason = strings.TrimSpace(reason)
}

// PipelinePlugin is an executable plugin in route pipeline.
type PipelinePlugin struct {
	name     string
	phase    Phase
	priority int
	run      func(*PipelineContext) (handled bool, err error)
	after    func(*PipelineContext, error)
}

func (p PipelinePlugin) Name() string  { return p.name }
func (p PipelinePlugin) Phase() Phase  { return p.phase }
func (p PipelinePlugin) Priority() int { return p.priority }
func (p PipelinePlugin) Run(ctx *PipelineContext) (bool, error) {
	if p.run == nil {
		return false, nil
	}
	return p.run(ctx)
}

func (p PipelinePlugin) AfterProxy(ctx *PipelineContext, proxyErr error) {
	if p.after != nil {
		p.after(ctx, proxyErr)
	}
}

// NewPipelinePlugin creates a new PipelinePlugin with the given configuration.
// This helper function makes it easier to create plugins programmatically.
func NewPipelinePlugin(name string, phase Phase, priority int, run func(*PipelineContext) (bool, error), after func(*PipelineContext, error)) PipelinePlugin {
	return PipelinePlugin{
		name:     name,
		phase:    phase,
		priority: priority,
		run:      run,
		after:    after,
	}
}

// BuilderContext contains runtime state required while creating plugins.
type BuilderContext struct {
	Consumers        []config.Consumer
	APIKeyLookup     APIKeyLookupFunc
	PermissionLookup EndpointPermissionLookupFunc
}

// PluginFactory builds one executable plugin from config.
type PluginFactory func(spec config.PluginConfig, ctx BuilderContext) (PipelinePlugin, error)

// Registry keeps plugin factories by name.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]PluginFactory
}

func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]PluginFactory),
	}
}

func (r *Registry) Register(name string, factory PluginFactory) error {
	if r == nil {
		return fmt.Errorf("plugin registry is nil")
	}
	name = normalizePluginName(name)
	if name == "" {
		return fmt.Errorf("plugin name is required")
	}
	if factory == nil {
		return fmt.Errorf("plugin factory is nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("plugin %q already registered", name)
	}
	r.factories[name] = factory
	return nil
}

func (r *Registry) Lookup(name string) (PluginFactory, bool) {
	if r == nil {
		return nil, false
	}
	name = normalizePluginName(name)
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.factories[name]
	return factory, ok
}

func (r *Registry) Build(spec config.PluginConfig, ctx BuilderContext) (PipelinePlugin, error) {
	name := normalizePluginName(spec.Name)
	if name == "" {
		return PipelinePlugin{}, fmt.Errorf("plugin name is required")
	}

	factory, ok := r.Lookup(name)
	if !ok {
		return PipelinePlugin{}, fmt.Errorf("plugin %q is not registered", spec.Name)
	}
	plugin, err := factory(spec, ctx)
	if err != nil {
		return PipelinePlugin{}, err
	}
	if plugin.name == "" {
		plugin.name = name
	}
	return plugin, nil
}

func NewDefaultRegistry() *Registry {
	r := NewRegistry()
	_ = r.Register("cors", buildCORSPlugin)
	_ = r.Register("correlation-id", buildCorrelationIDPlugin)
	_ = r.Register("bot-detect", buildBotDetectPlugin)
	_ = r.Register("ip-restrict", buildIPRestrictPlugin)
	_ = r.Register("auth-apikey", buildAuthAPIKeyPlugin)
	_ = r.Register("auth-jwt", buildAuthJWTPlugin)
	_ = r.Register("user-ip-whitelist", buildUserIPWhitelistPlugin)
	_ = r.Register("endpoint-permission", buildEndpointPermissionPlugin)
	_ = r.Register("rate-limit", buildRateLimitPlugin)
	_ = r.Register("request-size-limit", buildRequestSizeLimitPlugin)
	_ = r.Register("request-validator", buildRequestValidatorPlugin)
	_ = r.Register("circuit-breaker", buildCircuitBreakerPlugin)
	_ = r.Register("retry", buildRetryPlugin)
	_ = r.Register("timeout", buildTimeoutPlugin)
	_ = r.Register("url-rewrite", buildURLRewritePlugin)
	_ = r.Register("request-transform", buildRequestTransformPlugin)
	_ = r.Register("response-transform", buildResponseTransformPlugin)
	_ = r.Register("compression", buildCompressionPlugin)
	_ = r.Register("redirect", buildRedirectPlugin)
	_ = r.Register("cache", buildCachePlugin)
	return r
}

// BuildRoutePipelines merges global + route plugin configs, builds and sorts chains.
func BuildRoutePipelines(cfg *config.Config, consumers []config.Consumer) (map[string][]PipelinePlugin, map[string]bool, error) {
	return BuildRoutePipelinesWithContext(cfg, BuilderContext{
		Consumers: append([]config.Consumer(nil), consumers...),
	})
}

// BuildRoutePipelinesWithContext merges global + route plugin configs, builds and sorts chains.
func BuildRoutePipelinesWithContext(cfg *config.Config, ctx BuilderContext) (map[string][]PipelinePlugin, map[string]bool, error) {
	if cfg == nil {
		return map[string][]PipelinePlugin{}, map[string]bool{}, nil
	}

	registry := NewDefaultRegistry()
	ctx.Consumers = append([]config.Consumer(nil), ctx.Consumers...)
	globalPlugins := append([]config.PluginConfig(nil), cfg.GlobalPlugins...)
	if ctx.PermissionLookup != nil && len(ctx.Consumers) == 0 {
		globalPlugins = ensureEndpointPermissionGlobal(globalPlugins)
	}

	pipelines := make(map[string][]PipelinePlugin, len(cfg.Routes))
	hasAuth := make(map[string]bool, len(cfg.Routes))

	for i := range cfg.Routes {
		route := &cfg.Routes[i]
		key := routePipelineKey(route, i)

		specs := mergePluginSpecs(globalPlugins, route.Plugins)

		chain := make([]PipelinePlugin, 0, len(specs))
		for _, spec := range specs {
			if !isPluginEnabled(spec) {
				continue
			}
			plugin, err := registry.Build(spec, ctx)
			if err != nil {
				return nil, nil, fmt.Errorf("build plugin %q for route %q: %w", spec.Name, route.Name, err)
			}
			if plugin.phase == PhaseAuth {
				hasAuth[key] = true
			}
			chain = append(chain, plugin)
		}

		sort.SliceStable(chain, func(i, j int) bool {
			ip, jp := phaseOrder(chain[i].phase), phaseOrder(chain[j].phase)
			if ip != jp {
				return ip < jp
			}
			if chain[i].priority != chain[j].priority {
				return chain[i].priority < chain[j].priority
			}
			return chain[i].name < chain[j].name
		})
		pipelines[key] = chain
	}

	return pipelines, hasAuth, nil
}

func buildCORSPlugin(spec config.PluginConfig, _ BuilderContext) (PipelinePlugin, error) {
	cfgMap := spec.Config
	plugin := NewCORS(CORSConfig{
		AllowedOrigins:   asStringSlice(cfgMap["allowed_origins"]),
		AllowedMethods:   asStringSlice(cfgMap["allowed_methods"]),
		AllowedHeaders:   asStringSlice(cfgMap["allowed_headers"]),
		MaxAge:           asInt(cfgMap["max_age"], 0),
		AllowCredentials: asBool(cfgMap["credentials"], false),
	})
	return PipelinePlugin{
		name:     plugin.Name(),
		phase:    plugin.Phase(),
		priority: plugin.Priority(),
		run: func(ctx *PipelineContext) (bool, error) {
			return plugin.Handle(ctx.ResponseWriter, ctx.Request), nil
		},
	}, nil
}

func buildCorrelationIDPlugin(_ config.PluginConfig, _ BuilderContext) (PipelinePlugin, error) {
	plugin := NewCorrelationID()
	return PipelinePlugin{
		name:     plugin.Name(),
		phase:    plugin.Phase(),
		priority: plugin.Priority(),
		run: func(ctx *PipelineContext) (bool, error) {
			plugin.Apply(ctx)
			return false, nil
		},
	}, nil
}

func buildBotDetectPlugin(spec config.PluginConfig, _ BuilderContext) (PipelinePlugin, error) {
	cfgMap := spec.Config
	plugin := NewBotDetect(BotDetectConfig{
		AllowList: asStringSlice(cfgMap["allow_list"]),
		DenyList:  asStringSlice(cfgMap["deny_list"]),
		Action:    asString(cfgMap["action"]),
	})
	return PipelinePlugin{
		name:     plugin.Name(),
		phase:    plugin.Phase(),
		priority: plugin.Priority(),
		run: func(ctx *PipelineContext) (bool, error) {
			return false, plugin.Evaluate(ctx)
		},
	}, nil
}

func buildIPRestrictPlugin(spec config.PluginConfig, _ BuilderContext) (PipelinePlugin, error) {
	cfgMap := spec.Config
	plugin, err := NewIPRestrict(IPRestrictConfig{
		Whitelist: asStringSlice(cfgMap["whitelist"]),
		Blacklist: asStringSlice(cfgMap["blacklist"]),
	})
	if err != nil {
		return PipelinePlugin{}, err
	}
	return PipelinePlugin{
		name:     plugin.Name(),
		phase:    plugin.Phase(),
		priority: plugin.Priority(),
		run: func(ctx *PipelineContext) (bool, error) {
			return false, plugin.Allow(ctx.Request)
		},
	}, nil
}

func buildAuthAPIKeyPlugin(spec config.PluginConfig, ctx BuilderContext) (PipelinePlugin, error) {
	cfgMap := spec.Config
	plugin := NewAuthAPIKey(ctx.Consumers, AuthAPIKeyOptions{
		KeyNames:    asStringSlice(cfgMap["key_names"]),
		QueryNames:  asStringSlice(cfgMap["query_names"]),
		CookieNames: asStringSlice(cfgMap["cookie_names"]),
		Lookup:      ctx.APIKeyLookup,
	})
	return PipelinePlugin{
		name:     plugin.Name(),
		phase:    plugin.Phase(),
		priority: plugin.Priority(),
		run: func(ctx *PipelineContext) (bool, error) {
			consumer, err := plugin.Authenticate(ctx.Request)
			if err != nil {
				return false, err
			}
			ctx.Consumer = consumer
			return false, nil
		},
	}, nil
}

func buildAuthJWTPlugin(spec config.PluginConfig, _ BuilderContext) (PipelinePlugin, error) {
	cfgMap := spec.Config
	plugin := NewAuthJWT(AuthJWTOptions{
		Secret:          asString(cfgMap["secret"]),
		JWKSURL:         asString(cfgMap["jwks_url"]),
		JWKSTTL:         asDuration(cfgMap["jwks_ttl"], time.Hour),
		Issuer:          asString(cfgMap["issuer"]),
		Audience:        asStringSlice(cfgMap["audience"]),
		RequiredClaims:  asStringSlice(cfgMap["required_claims"]),
		ClaimsToHeaders: asStringMap(cfgMap["claims_to_headers"]),
		ClockSkew:       asDuration(cfgMap["clock_skew"], 30*time.Second),
	})
	return PipelinePlugin{
		name:     plugin.Name(),
		phase:    plugin.Phase(),
		priority: plugin.Priority(),
		run: func(ctx *PipelineContext) (bool, error) {
			claims, err := plugin.Authenticate(ctx.Request)
			if err != nil {
				return false, err
			}
			if ctx.Consumer == nil {
				if sub, ok := claims["sub"].(string); ok {
					sub = strings.TrimSpace(sub)
					if sub != "" {
						ctx.Consumer = &config.Consumer{ID: sub, Name: sub}
					}
				}
			}
			return false, nil
		},
	}, nil
}

func buildRateLimitPlugin(spec config.PluginConfig, _ BuilderContext) (PipelinePlugin, error) {
	cfgMap := spec.Config
	plugin, err := NewRateLimit(RateLimitConfig{
		Algorithm:         asString(cfgMap["algorithm"]),
		Scope:             asString(cfgMap["scope"]),
		RequestsPerSecond: asInt(cfgMap["requests_per_second"], 0),
		Burst:             asInt(cfgMap["burst"], 0),
		Limit:             asInt(cfgMap["limit"], 0),
		Window:            asDuration(cfgMap["window"], time.Second),
		CompositeScopes:   asStringSlice(cfgMap["composite_scopes"]),
	})
	if err != nil {
		return PipelinePlugin{}, err
	}
	return PipelinePlugin{
		name:     plugin.Name(),
		phase:    plugin.Phase(),
		priority: plugin.Priority(),
		run: func(ctx *PipelineContext) (bool, error) {
			if ctx != nil && ctx.Metadata != nil {
				if skip, ok := ctx.Metadata["skip_rate_limit"].(bool); ok && skip {
					return false, nil
				}
			}
			allowed := plugin.Enforce(ctx.ResponseWriter, RateLimitRequest{
				Request:  ctx.Request,
				Route:    ctx.Route,
				Consumer: ctx.Consumer,
				Metadata: ctx.Metadata,
			})
			return !allowed, nil
		},
	}, nil
}

func buildEndpointPermissionPlugin(_ config.PluginConfig, ctx BuilderContext) (PipelinePlugin, error) {
	plugin := NewEndpointPermission(ctx.PermissionLookup)
	return PipelinePlugin{
		name:     plugin.Name(),
		phase:    plugin.Phase(),
		priority: plugin.Priority(),
		run: func(ctx *PipelineContext) (bool, error) {
			return false, plugin.Evaluate(ctx)
		},
	}, nil
}

func buildRequestSizeLimitPlugin(spec config.PluginConfig, _ BuilderContext) (PipelinePlugin, error) {
	cfgMap := spec.Config
	maxBytes := int64(asInt(cfgMap["max_bytes"], 1<<20))
	plugin := NewRequestSizeLimit(RequestSizeLimitConfig{
		MaxBytes: maxBytes,
	})
	return PipelinePlugin{
		name:     plugin.Name(),
		phase:    plugin.Phase(),
		priority: plugin.Priority(),
		run: func(ctx *PipelineContext) (bool, error) {
			return false, plugin.Enforce(ctx)
		},
	}, nil
}

func buildRequestValidatorPlugin(spec config.PluginConfig, _ BuilderContext) (PipelinePlugin, error) {
	cfgMap := spec.Config
	validator, err := NewRequestValidator(RequestValidatorConfig{
		Schema: asAnyMap(cfgMap["schema"]),
	})
	if err != nil {
		return PipelinePlugin{}, err
	}
	return PipelinePlugin{
		name:     validator.Name(),
		phase:    validator.Phase(),
		priority: validator.Priority(),
		run: func(ctx *PipelineContext) (bool, error) {
			return false, validator.Validate(ctx)
		},
	}, nil
}

func buildCircuitBreakerPlugin(spec config.PluginConfig, _ BuilderContext) (PipelinePlugin, error) {
	cfgMap := spec.Config
	plugin := NewCircuitBreaker(CircuitBreakerConfig{
		ErrorThreshold:   asFloat(cfgMap["error_threshold"], 0.5),
		VolumeThreshold:  asInt(cfgMap["volume_threshold"], 20),
		SleepWindow:      asDuration(cfgMap["sleep_window"], 10*time.Second),
		HalfOpenRequests: asInt(cfgMap["half_open_requests"], 1),
		Window:           asDuration(cfgMap["window"], 30*time.Second),
	})
	return PipelinePlugin{
		name:     plugin.Name(),
		phase:    plugin.Phase(),
		priority: plugin.Priority(),
		run: func(ctx *PipelineContext) (bool, error) {
			return false, plugin.Allow()
		},
		after: func(_ *PipelineContext, proxyErr error) {
			plugin.Report(proxyErr == nil)
		},
	}, nil
}

func buildRetryPlugin(spec config.PluginConfig, _ BuilderContext) (PipelinePlugin, error) {
	cfgMap := spec.Config
	plugin := NewRetry(RetryConfig{
		MaxRetries:   asInt(cfgMap["max_retries"], 1),
		BaseDelay:    asDuration(cfgMap["base_delay"], 50*time.Millisecond),
		MaxDelay:     asDuration(cfgMap["max_delay"], 500*time.Millisecond),
		Jitter:       asBool(cfgMap["jitter"], true),
		RetryMethods: asStringSlice(cfgMap["retry_methods"]),
		RetryOnStatus: asIntSlice(cfgMap["retry_on_status"], []int{
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout,
		}),
	})
	return PipelinePlugin{
		name:     plugin.Name(),
		phase:    plugin.Phase(),
		priority: plugin.Priority(),
		run: func(ctx *PipelineContext) (bool, error) {
			ctx.Retry = plugin
			return false, nil
		},
	}, nil
}

func buildTimeoutPlugin(spec config.PluginConfig, _ BuilderContext) (PipelinePlugin, error) {
	cfgMap := spec.Config
	timeout := NewTimeout(TimeoutConfig{
		Duration: asDuration(cfgMap["timeout"], 0),
	})
	return PipelinePlugin{
		name:     timeout.Name(),
		phase:    timeout.Phase(),
		priority: timeout.Priority(),
		run: func(ctx *PipelineContext) (bool, error) {
			timeout.Apply(ctx)
			return false, nil
		},
	}, nil
}

func buildRequestTransformPlugin(spec config.PluginConfig, _ BuilderContext) (PipelinePlugin, error) {
	cfgMap := spec.Config
	bodyHooks := asAnyMap(cfgMap["body_hooks"])
	if len(bodyHooks) == 0 {
		bodyHooks = asAnyMap(cfgMap["body_transform"])
	}
	plugin, err := NewRequestTransform(RequestTransformConfig{
		AddHeaders:      asStringMap(cfgMap["add_headers"]),
		RemoveHeaders:   asStringSlice(cfgMap["remove_headers"]),
		RenameHeaders:   asStringMap(cfgMap["rename_headers"]),
		AddQuery:        asStringMap(cfgMap["add_query"]),
		RemoveQuery:     asStringSlice(cfgMap["remove_query"]),
		RenameQuery:     asStringMap(cfgMap["rename_query"]),
		Method:          asString(cfgMap["method"]),
		Path:            asString(cfgMap["path"]),
		PathPattern:     pickFirstString(cfgMap["path_pattern"], cfgMap["path_regex"]),
		PathReplacement: pickFirstString(cfgMap["path_replacement"], cfgMap["path_replace"]),
		BodyHooks:       bodyHooks,
	})
	if err != nil {
		return PipelinePlugin{}, err
	}
	return PipelinePlugin{
		name:     plugin.Name(),
		phase:    plugin.Phase(),
		priority: plugin.Priority(),
		run: func(ctx *PipelineContext) (bool, error) {
			return false, plugin.Apply(ctx)
		},
	}, nil
}

func buildURLRewritePlugin(spec config.PluginConfig, _ BuilderContext) (PipelinePlugin, error) {
	cfgMap := spec.Config
	plugin, err := NewURLRewrite(URLRewriteConfig{
		Pattern:     pickFirstString(cfgMap["pattern"], cfgMap["regex"]),
		Replacement: pickFirstString(cfgMap["replacement"], cfgMap["replace"]),
	})
	if err != nil {
		return PipelinePlugin{}, err
	}
	return PipelinePlugin{
		name:     plugin.Name(),
		phase:    plugin.Phase(),
		priority: plugin.Priority(),
		run: func(ctx *PipelineContext) (bool, error) {
			return false, plugin.Apply(ctx)
		},
	}, nil
}

func buildResponseTransformPlugin(spec config.PluginConfig, _ BuilderContext) (PipelinePlugin, error) {
	cfgMap := spec.Config
	plugin := NewResponseTransform(ResponseTransformConfig{
		AddHeaders:    asStringMap(cfgMap["add_headers"]),
		RemoveHeaders: asStringSlice(cfgMap["remove_headers"]),
		ReplaceBody:   pickFirstString(cfgMap["replace_body"], cfgMap["body"]),
	})
	return PipelinePlugin{
		name:     plugin.Name(),
		phase:    plugin.Phase(),
		priority: plugin.Priority(),
		run: func(ctx *PipelineContext) (bool, error) {
			plugin.Apply(ctx)
			return false, nil
		},
		after: func(ctx *PipelineContext, proxyErr error) {
			plugin.AfterProxy(ctx, proxyErr)
		},
	}, nil
}

func buildCompressionPlugin(spec config.PluginConfig, _ BuilderContext) (PipelinePlugin, error) {
	cfgMap := spec.Config
	plugin := NewCompression(CompressionConfig{
		MinSize: asInt(cfgMap["min_size"], 0),
	})
	return PipelinePlugin{
		name:     plugin.Name(),
		phase:    plugin.Phase(),
		priority: plugin.Priority(),
		run: func(ctx *PipelineContext) (bool, error) {
			plugin.Apply(ctx)
			return false, nil
		},
		after: func(ctx *PipelineContext, proxyErr error) {
			plugin.AfterProxy(ctx, proxyErr)
		},
	}, nil
}

func buildRedirectPlugin(spec config.PluginConfig, _ BuilderContext) (PipelinePlugin, error) {
	cfgMap := spec.Config
	rules := asRedirectRules(cfgMap["rules"])
	if len(rules) == 0 {
		path := pickFirstString(cfgMap["path"], cfgMap["from"])
		target := pickFirstString(cfgMap["url"], cfgMap["target"], cfgMap["to"])
		if path != "" && target != "" {
			rules = append(rules, RedirectRule{
				Path:       path,
				TargetURL:  target,
				StatusCode: asInt(cfgMap["status_code"], http.StatusFound),
			})
		}
	}
	plugin := NewRedirect(RedirectConfig{Rules: rules})
	return PipelinePlugin{
		name:     plugin.Name(),
		phase:    plugin.Phase(),
		priority: plugin.Priority(),
		run: func(ctx *PipelineContext) (bool, error) {
			if ctx == nil {
				return false, nil
			}
			return plugin.Handle(ctx.ResponseWriter, ctx.Request), nil
		},
	}, nil
}

func normalizePluginName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func isPluginEnabled(spec config.PluginConfig) bool {
	if spec.Enabled == nil {
		return true
	}
	return *spec.Enabled
}

func routePipelineKey(route *config.Route, idx int) string {
	if route == nil {
		return fmt.Sprintf("route-%d", idx)
	}
	if value := strings.TrimSpace(route.ID); value != "" {
		return value
	}
	if value := strings.TrimSpace(route.Name); value != "" {
		return value
	}
	return fmt.Sprintf("route-%d", idx)
}

func mergePluginSpecs(global, route []config.PluginConfig) []config.PluginConfig {
	if len(global) == 0 && len(route) == 0 {
		return nil
	}

	out := make([]config.PluginConfig, 0, len(global)+len(route))
	indexByName := make(map[string]int, len(global)+len(route))

	for _, spec := range global {
		name := normalizePluginName(spec.Name)
		if name == "" {
			continue
		}
		indexByName[name] = len(out)
		out = append(out, spec)
	}

	for _, spec := range route {
		name := normalizePluginName(spec.Name)
		if name == "" {
			continue
		}
		if idx, exists := indexByName[name]; exists {
			out[idx] = spec
			continue
		}
		indexByName[name] = len(out)
		out = append(out, spec)
	}

	return out
}

func ensureEndpointPermissionGlobal(in []config.PluginConfig) []config.PluginConfig {
	needPermission := true
	needUserIPWhitelist := true
	for _, spec := range in {
		switch normalizePluginName(spec.Name) {
		case "endpoint-permission":
			needPermission = false
		case "user-ip-whitelist":
			needUserIPWhitelist = false
		}
	}
	if needPermission {
		in = append(in, config.PluginConfig{Name: "endpoint-permission"})
	}
	if needUserIPWhitelist {
		in = append(in, config.PluginConfig{Name: "user-ip-whitelist"})
	}
	return in
}

func buildUserIPWhitelistPlugin(_ config.PluginConfig, _ BuilderContext) (PipelinePlugin, error) {
	plugin := NewUserIPWhitelist()
	return PipelinePlugin{
		name:     plugin.Name(),
		phase:    plugin.Phase(),
		priority: plugin.Priority(),
		run: func(ctx *PipelineContext) (bool, error) {
			return false, plugin.Evaluate(ctx)
		},
	}, nil
}

func phaseOrder(phase Phase) int {
	switch phase {
	case PhasePreAuth:
		return 1
	case PhaseAuth:
		return 2
	case PhasePreProxy:
		return 3
	case PhaseProxy:
		return 4
	case PhasePostProxy:
		return 5
	default:
		return 999
	}
}

func asString(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		out := strings.TrimSpace(fmt.Sprint(value))
		if strings.EqualFold(out, "<nil>") {
			return ""
		}
		return out
	}
}

func asStringSlice(value any) []string {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			out = append(out, item)
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s := strings.TrimSpace(fmt.Sprint(item))
			if s == "" {
				continue
			}
			out = append(out, s)
		}
		return out
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return nil
		}
		return []string{v}
	default:
		return nil
	}
}

func asStringMap(value any) map[string]string {
	if value == nil {
		return nil
	}
	raw, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(fmt.Sprint(v))
		if key == "" || val == "" {
			continue
		}
		out[key] = val
	}
	return out
}

func asAnyMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	raw, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = v
	}
	return out
}

func asRedirectRules(value any) []RedirectRule {
	if value == nil {
		return nil
	}
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]RedirectRule, 0, len(raw))
	for _, item := range raw {
		ruleMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		path := pickFirstString(ruleMap["path"], ruleMap["from"])
		target := pickFirstString(ruleMap["url"], ruleMap["target"], ruleMap["to"])
		if path == "" || target == "" {
			continue
		}
		out = append(out, RedirectRule{
			Path:       path,
			TargetURL:  target,
			StatusCode: asInt(ruleMap["status_code"], http.StatusFound),
		})
	}
	return out
}

func pickFirstString(values ...any) string {
	for _, value := range values {
		if s := asString(value); s != "" {
			return s
		}
	}
	return ""
}

func asInt(value any, fallback int) int {
	if value == nil {
		return fallback
	}
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case int32:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return fallback
		}
		i, err := strconv.Atoi(v)
		if err != nil {
			return fallback
		}
		return i
	default:
		return fallback
	}
}

func asBool(value any, fallback bool) bool {
	if value == nil {
		return fallback
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		v = strings.TrimSpace(strings.ToLower(v))
		if v == "" {
			return fallback
		}
		return v == "1" || v == "true" || v == "yes" || v == "on"
	default:
		return fallback
	}
}

func asDuration(value any, fallback time.Duration) time.Duration {
	if value == nil {
		return fallback
	}
	switch v := value.(type) {
	case time.Duration:
		return v
	case int:
		return time.Duration(v) * time.Second
	case int64:
		return time.Duration(v) * time.Second
	case float64:
		return time.Duration(v * float64(time.Second))
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return fallback
		}
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
		i, err := strconv.Atoi(v)
		if err != nil {
			return fallback
		}
		return time.Duration(i) * time.Second
	default:
		return fallback
	}
}

func asFloat(value any, fallback float64) float64 {
	if value == nil {
		return fallback
	}
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return fallback
		}
		out, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fallback
		}
		return out
	default:
		return fallback
	}
}

func asIntSlice(value any, fallback []int) []int {
	if value == nil {
		return append([]int(nil), fallback...)
	}
	switch v := value.(type) {
	case []int:
		out := make([]int, 0, len(v))
		for _, item := range v {
			out = append(out, item)
		}
		if len(out) == 0 {
			return append([]int(nil), fallback...)
		}
		return out
	case []any:
		out := make([]int, 0, len(v))
		for _, item := range v {
			out = append(out, asInt(item, -1))
		}
		filtered := make([]int, 0, len(out))
		for _, item := range out {
			if item >= 100 {
				filtered = append(filtered, item)
			}
		}
		if len(filtered) == 0 {
			return append([]int(nil), fallback...)
		}
		return filtered
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return append([]int(nil), fallback...)
		}
		parts := strings.Split(v, ",")
		out := make([]int, 0, len(parts))
		for _, part := range parts {
			n := asInt(strings.TrimSpace(part), -1)
			if n >= 100 {
				out = append(out, n)
			}
		}
		if len(out) == 0 {
			return append([]int(nil), fallback...)
		}
		return out
	default:
		return append([]int(nil), fallback...)
	}
}
