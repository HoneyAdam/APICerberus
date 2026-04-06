package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// OptimizedPipelineConfig holds configuration for the optimized pipeline.
type OptimizedPipelineConfig struct {
	// Result caching
	EnableResultCache bool
	CacheSize         int
	CacheTTL          time.Duration

	// Parallel execution
	EnableParallel     bool
	MaxParallelPlugins int
	ParallelTimeout    time.Duration

	// Fast path optimization
	EnableFastPath      bool
	FastPathCacheSize   int
}

// DefaultOptimizedPipelineConfig returns sensible defaults.
func DefaultOptimizedPipelineConfig() OptimizedPipelineConfig {
	return OptimizedPipelineConfig{
		EnableResultCache:   true,
		CacheSize:           10_000,
		CacheTTL:            5 * time.Second,
		EnableParallel:      true,
		MaxParallelPlugins:  4,
		ParallelTimeout:     100 * time.Millisecond,
		EnableFastPath:      true,
		FastPathCacheSize:   1000,
	}
}

// OptimizedPipeline is a high-performance plugin pipeline with caching and parallel execution.
type OptimizedPipeline struct {
	plugins         []PipelinePlugin
	config          OptimizedPipelineConfig
	resultCache     *pluginResultCache
	fastPathCache   *fastPathCache
	metrics         *pipelineMetrics
	parallelEnabled bool
}

// pluginResultCache caches plugin execution results.
type pluginResultCache struct {
	mu      sync.RWMutex
	entries map[string]*pluginCacheEntry
	size    int
	ttl     time.Duration
}

type pluginCacheEntry struct {
	result    pluginResult
	expiresAt time.Time
}

type pluginResult struct {
	handled bool
	err     error
}

// fastPathCache caches fast-path decisions for requests.
type fastPathCache struct {
	mu      sync.RWMutex
	entries map[string]*fastPathEntry
	size    int
}

type fastPathEntry struct {
	canSkipPlugins map[string]bool
	expiresAt      time.Time
}

// pipelineMetrics holds performance metrics.
type pipelineMetrics struct {
	executionsTotal   atomic.Uint64
	cacheHits         atomic.Uint64
	cacheMisses       atomic.Uint64
	fastPathHits      atomic.Uint64
	parallelExecutions atomic.Uint64
	errorsTotal       atomic.Uint64
}

// NewOptimizedPipeline creates a high-performance plugin pipeline.
func NewOptimizedPipeline(plugins []PipelinePlugin, cfg OptimizedPipelineConfig) *OptimizedPipeline {
	cloned := make([]PipelinePlugin, len(plugins))
	copy(cloned, plugins)

	p := &OptimizedPipeline{
		plugins:         cloned,
		config:          cfg,
		metrics:         &pipelineMetrics{},
		parallelEnabled: cfg.EnableParallel && len(plugins) > 1,
	}

	if cfg.EnableResultCache {
		p.resultCache = &pluginResultCache{
			entries: make(map[string]*pluginCacheEntry, cfg.CacheSize),
			size:    cfg.CacheSize,
			ttl:     cfg.CacheTTL,
		}
	}

	if cfg.EnableFastPath {
		p.fastPathCache = &fastPathCache{
			entries: make(map[string]*fastPathEntry, cfg.FastPathCacheSize),
			size:    cfg.FastPathCacheSize,
		}
	}

	return p
}

// Execute runs pre-proxy phases with optimizations.
func (p *OptimizedPipeline) Execute(ctx *PipelineContext) (bool, error) {
	if p == nil || ctx == nil {
		return false, nil
	}

	p.metrics.executionsTotal.Add(1)

	// Check fast path cache
	if p.fastPathCache != nil {
		if canSkip, skipPlugins := p.checkFastPath(ctx); canSkip {
			return p.executeWithFastPath(ctx, skipPlugins)
		}
	}

	// Determine which plugins can run in parallel
	if p.parallelEnabled && len(p.plugins) >= 2 {
		return p.executeParallel(ctx)
	}

	// Sequential execution with caching
	return p.executeSequential(ctx)
}

// checkFastPath checks if we can use cached fast-path decisions.
func (p *OptimizedPipeline) checkFastPath(ctx *PipelineContext) (bool, map[string]bool) {
	if p.fastPathCache == nil || ctx.Request == nil {
		return false, nil
	}

	key := p.fastPathKey(ctx)

	p.fastPathCache.mu.RLock()
	entry, exists := p.fastPathCache.entries[key]
	p.fastPathCache.mu.RUnlock()

	if !exists || time.Now().After(entry.expiresAt) {
		return false, nil
	}

	p.metrics.fastPathHits.Add(1)
	return true, entry.canSkipPlugins
}

// executeWithFastPath executes skipping plugins that can be bypassed.
func (p *OptimizedPipeline) executeWithFastPath(ctx *PipelineContext, skipPlugins map[string]bool) (bool, error) {
	for _, plugin := range p.plugins {
		if skipPlugins[plugin.Name()] {
			continue
		}

		handled, err := plugin.Run(ctx)
		if err != nil {
			p.metrics.errorsTotal.Add(1)
			return false, err
		}
		if handled {
			if !ctx.Aborted {
				ctx.Abort(plugin.Name() + ": handled response")
			}
			return true, nil
		}
		if ctx.Aborted {
			return true, nil
		}
	}
	return false, nil
}

// executeSequential runs plugins sequentially with result caching.
func (p *OptimizedPipeline) executeSequential(ctx *PipelineContext) (bool, error) {
	for _, plugin := range p.plugins {
		// Try cache first
		if p.resultCache != nil {
			if cached := p.getCachedResult(plugin.Name(), ctx); cached != nil {
				if cached.err != nil {
					return false, cached.err
				}
				if cached.handled {
					if !ctx.Aborted {
						ctx.Abort(plugin.Name() + ": handled response (cached)")
					}
					return true, nil
				}
				continue
			}
		}

		handled, err := plugin.Run(ctx)

		// Cache result
		if p.resultCache != nil {
			p.cacheResult(plugin.Name(), ctx, handled, err)
		}

		if err != nil {
			p.metrics.errorsTotal.Add(1)
			return false, err
		}
		if handled {
			if !ctx.Aborted {
				ctx.Abort(plugin.Name() + ": handled response")
			}
			return true, nil
		}
		if ctx.Aborted {
			return true, nil
		}
	}
	return false, nil
}

// executeParallel runs independent plugins in parallel.
func (p *OptimizedPipeline) executeParallel(ctx *PipelineContext) (bool, error) {
	// Split plugins into parallelizable and sequential groups
	parallelPlugins, sequentialPlugins := p.splitPlugins(p.plugins)

	if len(parallelPlugins) == 0 {
		// No parallel plugins, run sequentially
		return p.executeSequential(ctx)
	}

	p.metrics.parallelExecutions.Add(1)

	// Execute parallel plugins
	type pluginResult struct {
		plugin  PipelinePlugin
		handled bool
		err     error
	}

	results := make(chan pluginResult, len(parallelPlugins))
	var wg sync.WaitGroup

	// Limit concurrent execution
	semaphore := make(chan struct{}, p.config.MaxParallelPlugins)

	for _, plugin := range parallelPlugins {
		wg.Add(1)
		go func(pl PipelinePlugin) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Create a shallow copy of context for parallel execution
			pluginCtx := &PipelineContext{
				Request:        ctx.Request,
				ResponseWriter: ctx.ResponseWriter,
				Route:          ctx.Route,
				Service:        ctx.Service,
				Consumer:       ctx.Consumer,
				Metadata:       ctx.Metadata,
			}

			handled, err := pl.Run(pluginCtx)
			results <- pluginResult{plugin: pl, handled: handled, err: err}
		}(plugin)
	}

	// Close results channel when all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var firstError error
	var anyHandled bool
	var handledBy string

	for result := range results {
		if result.err != nil {
			if firstError == nil {
				firstError = result.err
			}
			continue
		}
		if result.handled {
			anyHandled = true
			handledBy = result.plugin.Name()
		}
	}

	if firstError != nil {
		p.metrics.errorsTotal.Add(1)
		return false, firstError
	}

	if anyHandled {
		if !ctx.Aborted {
			ctx.Abort(handledBy + ": handled response")
		}
		return true, nil
	}

	// Execute remaining sequential plugins
	for _, plugin := range sequentialPlugins {
		handled, err := plugin.Run(ctx)
		if err != nil {
			p.metrics.errorsTotal.Add(1)
			return false, err
		}
		if handled {
			if !ctx.Aborted {
				ctx.Abort(plugin.Name() + ": handled response")
			}
			return true, nil
		}
		if ctx.Aborted {
			return true, nil
		}
	}

	return false, nil
}

// splitPlugins separates plugins into parallelizable and sequential groups.
func (p *OptimizedPipeline) splitPlugins(plugins []PipelinePlugin) ([]PipelinePlugin, []PipelinePlugin) {
	var parallel, sequential []PipelinePlugin

	for _, plugin := range plugins {
		// Plugins that modify shared state or depend on order must run sequentially
		if p.isParallelSafe(plugin) {
			parallel = append(parallel, plugin)
		} else {
			sequential = append(sequential, plugin)
		}
	}

	return parallel, sequential
}

// isParallelSafe checks if a plugin can run in parallel with others.
func (p *OptimizedPipeline) isParallelSafe(plugin PipelinePlugin) bool {
	// These plugins are generally safe to run in parallel
	safePlugins := map[string]bool{
		"cors":              true,
		"correlation-id":    true,
		"bot-detect":        true,
		"ip-restrict":       true,
		"rate-limit":        true,
		"request-size-limit": true,
		"request-validator": true,
	}

	return safePlugins[strings.ToLower(plugin.Name())]
}

// ExecutePostProxy runs post-proxy callbacks.
func (p *OptimizedPipeline) ExecutePostProxy(ctx *PipelineContext, proxyErr error) {
	if p == nil || ctx == nil {
		return
	}

	// Run post-proxy plugins sequentially (they often depend on order)
	for _, plugin := range p.plugins {
		plugin.AfterProxy(ctx, proxyErr)
	}
}

// Plugins returns a copy of the plugin list.
func (p *OptimizedPipeline) Plugins() []PipelinePlugin {
	if p == nil {
		return nil
	}
	out := make([]PipelinePlugin, len(p.plugins))
	copy(out, p.plugins)
	return out
}

// getCachedResult retrieves a cached plugin result.
func (p *OptimizedPipeline) getCachedResult(pluginName string, ctx *PipelineContext) *pluginResult {
	if p.resultCache == nil {
		return nil
	}

	key := p.cacheKey(pluginName, ctx)

	p.resultCache.mu.RLock()
	entry, exists := p.resultCache.entries[key]
	p.resultCache.mu.RUnlock()

	if !exists || time.Now().After(entry.expiresAt) {
		p.metrics.cacheMisses.Add(1)
		return nil
	}

	p.metrics.cacheHits.Add(1)
	return &entry.result
}

// cacheResult stores a plugin result in the cache.
func (p *OptimizedPipeline) cacheResult(pluginName string, ctx *PipelineContext, handled bool, err error) {
	if p.resultCache == nil {
		return
	}

	key := p.cacheKey(pluginName, ctx)

	p.resultCache.mu.Lock()
	defer p.resultCache.mu.Unlock()

	// Evict oldest if at capacity
	if len(p.resultCache.entries) >= p.resultCache.size {
		p.evictOldest()
	}

	p.resultCache.entries[key] = &pluginCacheEntry{
		result: pluginResult{
			handled: handled,
			err:     err,
		},
		expiresAt: time.Now().Add(p.resultCache.ttl),
	}
}

// evictOldest removes expired and oldest entries from cache.
func (p *OptimizedPipeline) evictOldest() {
	now := time.Now()
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range p.resultCache.entries {
		if now.After(entry.expiresAt) {
			delete(p.resultCache.entries, key)
			continue
		}
		if oldestTime.IsZero() || entry.expiresAt.Before(oldestTime) {
			oldestTime = entry.expiresAt
			oldestKey = key
		}
	}

	// Remove oldest if still at capacity
	if len(p.resultCache.entries) >= p.resultCache.size && oldestKey != "" {
		delete(p.resultCache.entries, oldestKey)
	}
}

// cacheKey generates a cache key for a plugin execution.
func (p *OptimizedPipeline) cacheKey(pluginName string, ctx *PipelineContext) string {
	if ctx.Request == nil {
		return pluginName + ":|"
	}

	var parts []string
	parts = append(parts, pluginName)
	parts = append(parts, strings.ToUpper(ctx.Request.Method))
	parts = append(parts, ctx.Request.URL.Path)

	// Include relevant headers in cache key
	if ctx.Consumer != nil {
		parts = append(parts, ctx.Consumer.ID)
	}

	key := strings.Join(parts, "|")

	// Hash long keys
	if len(key) > 256 {
		hash := sha256.Sum256([]byte(key))
		return hex.EncodeToString(hash[:])
	}

	return key
}

// fastPathKey generates a key for fast-path caching.
func (p *OptimizedPipeline) fastPathKey(ctx *PipelineContext) string {
	if ctx.Request == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(strings.ToUpper(ctx.Request.Method))
	sb.WriteByte('|')
	sb.WriteString(ctx.Request.URL.Path)

	if ctx.Consumer != nil {
		sb.WriteByte('|')
		sb.WriteString(ctx.Consumer.ID)
	}

	return sb.String()
}

// UpdateFastPath updates the fast-path cache based on execution results.
func (p *OptimizedPipeline) UpdateFastPath(ctx *PipelineContext, skipPlugins map[string]bool) {
	if p.fastPathCache == nil || ctx.Request == nil {
		return
	}

	key := p.fastPathKey(ctx)

	p.fastPathCache.mu.Lock()
	defer p.fastPathCache.mu.Unlock()

	// Evict if at capacity
	if len(p.fastPathCache.entries) >= p.fastPathCache.size {
		p.evictOldestFastPath()
	}

	p.fastPathCache.entries[key] = &fastPathEntry{
		canSkipPlugins: skipPlugins,
		expiresAt:      time.Now().Add(30 * time.Second),
	}
}

// evictOldestFastPath removes oldest entries from fast-path cache.
func (p *OptimizedPipeline) evictOldestFastPath() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range p.fastPathCache.entries {
		if oldestTime.IsZero() || entry.expiresAt.Before(oldestTime) {
			oldestTime = entry.expiresAt
			oldestKey = key
		}
	}

	if oldestKey != "" {
		delete(p.fastPathCache.entries, oldestKey)
	}
}

// Metrics returns pipeline metrics.
func (p *OptimizedPipeline) Metrics() PipelineMetricsSnapshot {
	if p == nil || p.metrics == nil {
		return PipelineMetricsSnapshot{}
	}
	return PipelineMetricsSnapshot{
		ExecutionsTotal:    p.metrics.executionsTotal.Load(),
		CacheHits:          p.metrics.cacheHits.Load(),
		CacheMisses:        p.metrics.cacheMisses.Load(),
		FastPathHits:       p.metrics.fastPathHits.Load(),
		ParallelExecutions: p.metrics.parallelExecutions.Load(),
		ErrorsTotal:        p.metrics.errorsTotal.Load(),
	}
}

// PipelineMetricsSnapshot holds pipeline metrics.
type PipelineMetricsSnapshot struct {
	ExecutionsTotal    uint64
	CacheHits          uint64
	CacheMisses        uint64
	FastPathHits       uint64
	ParallelExecutions uint64
	ErrorsTotal        uint64
}

// OptimizedPipelineBuilder builds optimized pipelines for routes.
type OptimizedPipelineBuilder struct {
	config OptimizedPipelineConfig
	cache  map[string]*OptimizedPipeline
	mu     sync.RWMutex
}

// NewOptimizedPipelineBuilder creates a new builder.
func NewOptimizedPipelineBuilder(cfg OptimizedPipelineConfig) *OptimizedPipelineBuilder {
	return &OptimizedPipelineBuilder{
		config: cfg,
		cache:  make(map[string]*OptimizedPipeline),
	}
}

// Build creates or retrieves a cached optimized pipeline.
func (b *OptimizedPipelineBuilder) Build(routeKey string, plugins []PipelinePlugin) *OptimizedPipeline {
	b.mu.RLock()
	if cached := b.cache[routeKey]; cached != nil {
		b.mu.RUnlock()
		return cached
	}
	b.mu.RUnlock()

	// Build new pipeline
	pipeline := NewOptimizedPipeline(plugins, b.config)

	b.mu.Lock()
	b.cache[routeKey] = pipeline
	b.mu.Unlock()

	return pipeline
}

// BuildOptimizedRoutePipelines builds optimized pipelines for all routes.
func BuildOptimizedRoutePipelines(cfg *config.Config, ctx BuilderContext, pipelineCfg OptimizedPipelineConfig) (map[string]*OptimizedPipeline, map[string]bool, error) {
	if cfg == nil {
		return map[string]*OptimizedPipeline{}, map[string]bool{}, nil
	}

	builder := NewOptimizedPipelineBuilder(pipelineCfg)
	registry := NewDefaultRegistry()
	ctx.Consumers = append([]config.Consumer(nil), ctx.Consumers...)
	globalPlugins := append([]config.PluginConfig(nil), cfg.GlobalPlugins...)

	if ctx.PermissionLookup != nil && len(ctx.Consumers) == 0 {
		globalPlugins = ensureEndpointPermissionGlobal(globalPlugins)
	}

	pipelines := make(map[string]*OptimizedPipeline, len(cfg.Routes))
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

		// Sort by phase and priority
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

		pipelines[key] = builder.Build(key, chain)
	}

	return pipelines, hasAuth, nil
}
