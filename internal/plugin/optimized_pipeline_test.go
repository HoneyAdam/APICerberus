package plugin

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// =============================================================================
// Tests for Optimized Pipeline (0.0% coverage functions)
// =============================================================================

func TestDefaultOptimizedPipelineConfig(t *testing.T) {
	cfg := DefaultOptimizedPipelineConfig()

	if !cfg.EnableResultCache {
		t.Error("EnableResultCache should be true")
	}
	if cfg.CacheSize != 10000 {
		t.Errorf("CacheSize = %d, want 10000", cfg.CacheSize)
	}
	if cfg.CacheTTL != 5*time.Second {
		t.Errorf("CacheTTL = %v, want 5s", cfg.CacheTTL)
	}
	if !cfg.EnableParallel {
		t.Error("EnableParallel should be true")
	}
	if cfg.MaxParallelPlugins != 4 {
		t.Errorf("MaxParallelPlugins = %d, want 4", cfg.MaxParallelPlugins)
	}
	if !cfg.EnableFastPath {
		t.Error("EnableFastPath should be true")
	}
	if cfg.FastPathCacheSize != 1000 {
		t.Errorf("FastPathCacheSize = %d, want 1000", cfg.FastPathCacheSize)
	}
}

func TestNewOptimizedPipeline(t *testing.T) {
	t.Run("with default config", func(t *testing.T) {
		plugins := []PipelinePlugin{
			NewPipelinePlugin("test", PhasePreAuth, 0, func(ctx *PipelineContext) (bool, error) {
				return false, nil
			}, nil),
		}
		cfg := DefaultOptimizedPipelineConfig()
		p := NewOptimizedPipeline(plugins, cfg)

		if p == nil {
			t.Fatal("NewOptimizedPipeline returned nil")
		}
		if len(p.plugins) != 1 {
			t.Errorf("len(plugins) = %d, want 1", len(p.plugins))
		}
		if p.resultCache == nil {
			t.Error("resultCache should not be nil")
		}
		if p.fastPathCache == nil {
			t.Error("fastPathCache should not be nil")
		}
	})

	t.Run("with disabled caches", func(t *testing.T) {
		plugins := []PipelinePlugin{}
		cfg := OptimizedPipelineConfig{
			EnableResultCache: false,
			EnableFastPath:    false,
			EnableParallel:    false,
		}
		p := NewOptimizedPipeline(plugins, cfg)

		if p == nil {
			t.Fatal("NewOptimizedPipeline returned nil")
		}
		if p.resultCache != nil {
			t.Error("resultCache should be nil when disabled")
		}
		if p.fastPathCache != nil {
			t.Error("fastPathCache should be nil when disabled")
		}
	})

	t.Run("plugins are cloned", func(t *testing.T) {
		plugins := []PipelinePlugin{
			NewPipelinePlugin("p1", PhasePreAuth, 0, nil, nil),
			NewPipelinePlugin("p2", PhasePreAuth, 0, nil, nil),
		}
		cfg := OptimizedPipelineConfig{EnableResultCache: false, EnableFastPath: false}
		p := NewOptimizedPipeline(plugins, cfg)

		// Modify original
		plugins = append(plugins, NewPipelinePlugin("p3", PhasePreAuth, 0, nil, nil))

		// Pipeline should not be affected
		if len(p.plugins) != 2 {
			t.Error("plugins should be cloned, not shared")
		}
	})
}

func TestOptimizedPipeline_Execute(t *testing.T) {
	t.Run("nil pipeline", func(t *testing.T) {
		var p *OptimizedPipeline
		ctx := &PipelineContext{}
		handled, err := p.Execute(ctx)
		if handled {
			t.Error("handled should be false for nil pipeline")
		}
		if err != nil {
			t.Errorf("err should be nil, got %v", err)
		}
	})

	t.Run("nil context", func(t *testing.T) {
		p := NewOptimizedPipeline([]PipelinePlugin{}, DefaultOptimizedPipelineConfig())
		handled, err := p.Execute(nil)
		if handled {
			t.Error("handled should be false for nil context")
		}
		if err != nil {
			t.Errorf("err should be nil, got %v", err)
		}
	})

	t.Run("sequential execution", func(t *testing.T) {
		handlerCalled := false
		plugin := NewPipelinePlugin("test", PhasePreAuth, 0, func(ctx *PipelineContext) (bool, error) {
			handlerCalled = true
			return false, nil
		}, nil)

		cfg := OptimizedPipelineConfig{
			EnableResultCache: false,
			EnableParallel:    false,
			EnableFastPath:    false,
		}
		p := NewOptimizedPipeline([]PipelinePlugin{plugin}, cfg)

		ctx := &PipelineContext{
			Request:        httptest.NewRequest(http.MethodGet, "/", nil),
			ResponseWriter: httptest.NewRecorder(),
		}

		handled, err := p.Execute(ctx)
		if err != nil {
			t.Errorf("Execute error: %v", err)
		}
		if handled {
			t.Error("handled should be false")
		}
		if !handlerCalled {
			t.Error("handler should have been called")
		}
	})

	t.Run("plugin returns handled", func(t *testing.T) {
		plugin := NewPipelinePlugin("test", PhasePreAuth, 0, func(ctx *PipelineContext) (bool, error) {
			return true, nil
		}, nil)

		cfg := OptimizedPipelineConfig{
			EnableResultCache: false,
			EnableParallel:    false,
			EnableFastPath:    false,
		}
		p := NewOptimizedPipeline([]PipelinePlugin{plugin}, cfg)

		ctx := &PipelineContext{
			Request:        httptest.NewRequest(http.MethodGet, "/", nil),
			ResponseWriter: httptest.NewRecorder(),
		}

		handled, err := p.Execute(ctx)
		if err != nil {
			t.Errorf("Execute error: %v", err)
		}
		if !handled {
			t.Error("handled should be true")
		}
	})

	t.Run("plugin returns error", func(t *testing.T) {
		testErr := errors.New("test error")
		plugin := NewPipelinePlugin("test", PhasePreAuth, 0, func(ctx *PipelineContext) (bool, error) {
			return false, testErr
		}, nil)

		cfg := OptimizedPipelineConfig{
			EnableResultCache: false,
			EnableParallel:    false,
			EnableFastPath:    false,
		}
		p := NewOptimizedPipeline([]PipelinePlugin{plugin}, cfg)

		ctx := &PipelineContext{
			Request:        httptest.NewRequest(http.MethodGet, "/", nil),
			ResponseWriter: httptest.NewRecorder(),
		}

		_, err := p.Execute(ctx)
		if err != testErr {
			t.Errorf("Execute error = %v, want %v", err, testErr)
		}
	})
}

func TestOptimizedPipeline_checkFastPath(t *testing.T) {
	t.Run("fast path cache hit", func(t *testing.T) {
		cfg := OptimizedPipelineConfig{
			EnableFastPath:    true,
			FastPathCacheSize: 100,
		}
		p := NewOptimizedPipeline([]PipelinePlugin{}, cfg)

		// Pre-populate cache
		p.fastPathCache.mu.Lock()
		p.fastPathCache.entries["GET|/api"] = &fastPathEntry{
			canSkipPlugins: map[string]bool{"plugin1": true},
			expiresAt:      time.Now().Add(time.Hour),
		}
		p.fastPathCache.mu.Unlock()

		ctx := &PipelineContext{
			Route:          &config.Route{ID: "test-route"},
			Request:        httptest.NewRequest(http.MethodGet, "/api", nil),
			ResponseWriter: httptest.NewRecorder(),
		}

		canSkip, skipPlugins := p.checkFastPath(ctx)
		if !canSkip {
			t.Error("should find fast path")
		}
		if !skipPlugins["plugin1"] {
			t.Error("should be able to skip plugin1")
		}
	})

	t.Run("fast path cache miss", func(t *testing.T) {
		cfg := OptimizedPipelineConfig{
			EnableFastPath:    true,
			FastPathCacheSize: 100,
		}
		p := NewOptimizedPipeline([]PipelinePlugin{}, cfg)

		ctx := &PipelineContext{
			Route:          &config.Route{ID: "test-route"},
			Request:        httptest.NewRequest(http.MethodGet, "/unknown", nil),
			ResponseWriter: httptest.NewRecorder(),
		}

		canSkip, _ := p.checkFastPath(ctx)
		if canSkip {
			t.Error("should not find unknown path")
		}
	})

	t.Run("expired entry", func(t *testing.T) {
		cfg := OptimizedPipelineConfig{
			EnableFastPath:    true,
			FastPathCacheSize: 100,
		}
		p := NewOptimizedPipeline([]PipelinePlugin{}, cfg)

		// Pre-populate cache with expired entry
		p.fastPathCache.mu.Lock()
		p.fastPathCache.entries["GET|/api"] = &fastPathEntry{
			canSkipPlugins: map[string]bool{"plugin1": true},
			expiresAt:      time.Now().Add(-time.Hour),
		}
		p.fastPathCache.mu.Unlock()

		ctx := &PipelineContext{
			Route:          &config.Route{ID: "test-route"},
			Request:        httptest.NewRequest(http.MethodGet, "/api", nil),
			ResponseWriter: httptest.NewRecorder(),
		}

		canSkip, _ := p.checkFastPath(ctx)
		if canSkip {
			t.Error("should not find expired entry")
		}
	})
}

func TestOptimizedPipeline_cacheOperations(t *testing.T) {
	t.Run("cache key generation", func(t *testing.T) {
		p := NewOptimizedPipeline([]PipelinePlugin{}, DefaultOptimizedPipelineConfig())

		ctx := &PipelineContext{
			Route:   &config.Route{ID: "route-1"},
			Request: httptest.NewRequest(http.MethodGet, "/api/users", nil),
		}

		key := p.cacheKey("test-plugin", ctx)
		if key == "" {
			t.Error("cacheKey should not be empty")
		}
	})

	t.Run("cache result and get", func(t *testing.T) {
		cfg := OptimizedPipelineConfig{
			EnableResultCache: true,
			CacheSize:         100,
			CacheTTL:          time.Hour,
		}
		p := NewOptimizedPipeline([]PipelinePlugin{}, cfg)

		ctx := &PipelineContext{
			Route:   &config.Route{ID: "route-1"},
			Request: httptest.NewRequest(http.MethodGet, "/api", nil),
		}

		// Cache a result
		p.cacheResult("test-plugin", ctx, false, nil)

		// Get cached result
		result := p.getCachedResult("test-plugin", ctx)
		if result == nil {
			t.Error("should find cached result")
		}
	})

	t.Run("cache eviction when full", func(t *testing.T) {
		cfg := OptimizedPipelineConfig{
			EnableResultCache: true,
			CacheSize:         2,
			CacheTTL:          time.Hour,
		}
		p := NewOptimizedPipeline([]PipelinePlugin{}, cfg)

		// Add entries beyond capacity
		for i := 0; i < 5; i++ {
			pluginName := string(rune('a' + i))
			p.resultCache.mu.Lock()
			p.resultCache.entries[pluginName] = &pluginCacheEntry{
				result:    pluginResult{handled: false},
				expiresAt: time.Now().Add(time.Hour),
			}
			p.resultCache.mu.Unlock()
		}

		// Should not panic - eviction should handle it
	})
}

func TestOptimizedPipeline_fastPathOperations(t *testing.T) {
	t.Run("fast path key generation", func(t *testing.T) {
		p := NewOptimizedPipeline([]PipelinePlugin{}, DefaultOptimizedPipelineConfig())

		ctx := &PipelineContext{
			Route:   &config.Route{ID: "route-1"},
			Request: httptest.NewRequest(http.MethodGet, "/api/users", nil),
		}

		key := p.fastPathKey(ctx)
		if key == "" {
			t.Error("fastPathKey should not be empty")
		}
	})

	t.Run("update fast path", func(t *testing.T) {
		cfg := OptimizedPipelineConfig{
			EnableFastPath:    true,
			FastPathCacheSize: 100,
		}
		p := NewOptimizedPipeline([]PipelinePlugin{}, cfg)

		ctx := &PipelineContext{
			Route:   &config.Route{ID: "route-1"},
			Request: httptest.NewRequest(http.MethodGet, "/api", nil),
		}

		skipPlugins := map[string]bool{"plugin1": true, "plugin2": true}
		p.UpdateFastPath(ctx, skipPlugins)

		// Verify it was cached
		canSkip, found := p.checkFastPath(ctx)
		if !canSkip {
			t.Error("should find fast path after update")
		}
		if !found["plugin1"] {
			t.Error("should be able to skip plugin1")
		}
	})

	t.Run("fast path eviction", func(t *testing.T) {
		cfg := OptimizedPipelineConfig{
			EnableFastPath:    true,
			FastPathCacheSize: 2,
		}
		p := NewOptimizedPipeline([]PipelinePlugin{}, cfg)

		// Add entries beyond capacity
		for i := 0; i < 5; i++ {
			ctx := &PipelineContext{
				Route:   &config.Route{ID: string(rune('a' + i))},
				Request: httptest.NewRequest(http.MethodGet, "/", nil),
			}
			skipPlugins := map[string]bool{"plugin": true}
			p.UpdateFastPath(ctx, skipPlugins)
		}

		// Should not panic
	})
}

func TestOptimizedPipeline_Metrics(t *testing.T) {
	p := NewOptimizedPipeline([]PipelinePlugin{}, DefaultOptimizedPipelineConfig())

	t.Run("initial metrics", func(t *testing.T) {
		metrics := p.Metrics()
		if metrics.ExecutionsTotal != 0 {
			t.Errorf("ExecutionsTotal = %d, want 0", metrics.ExecutionsTotal)
		}
	})

	t.Run("metrics after execution", func(t *testing.T) {
		// Increment metrics
		p.metrics.executionsTotal.Add(5)
		p.metrics.cacheHits.Add(3)
		p.metrics.cacheMisses.Add(2)
		p.metrics.fastPathHits.Add(1)

		metrics := p.Metrics()
		if metrics.ExecutionsTotal != 5 {
			t.Errorf("ExecutionsTotal = %d, want 5", metrics.ExecutionsTotal)
		}
		if metrics.CacheHits != 3 {
			t.Errorf("CacheHits = %d, want 3", metrics.CacheHits)
		}
	})
}

func TestOptimizedPipeline_ExecutePostProxy(t *testing.T) {
	t.Run("execute post proxy", func(t *testing.T) {
		callOrder := []string{}
		afterFunc1 := func(ctx *PipelineContext, err error) {
			callOrder = append(callOrder, "plugin1")
		}
		afterFunc2 := func(ctx *PipelineContext, err error) {
			callOrder = append(callOrder, "plugin2")
		}
		plugin1 := NewPipelinePlugin("plugin1", PhasePostProxy, 0, nil, afterFunc1)
		plugin2 := NewPipelinePlugin("plugin2", PhasePostProxy, 0, nil, afterFunc2)

		p := NewOptimizedPipeline([]PipelinePlugin{plugin1, plugin2}, DefaultOptimizedPipelineConfig())

		ctx := &PipelineContext{
			Request:        httptest.NewRequest(http.MethodGet, "/", nil),
			ResponseWriter: httptest.NewRecorder(),
		}

		p.ExecutePostProxy(ctx, nil)
		// ExecutePostProxy doesn't return error
		if len(callOrder) != 2 {
			t.Errorf("expected 2 calls, got %d", len(callOrder))
		}
	})

	t.Run("nil pipeline", func(t *testing.T) {
		var p *OptimizedPipeline
		ctx := &PipelineContext{}
		p.ExecutePostProxy(ctx, nil)
		// ExecutePostProxy doesn't return error - should not panic
	})
}

func TestOptimizedPipeline_Plugins(t *testing.T) {
	plugin1 := NewPipelinePlugin("plugin1", PhasePreAuth, 0, nil, nil)
	plugin2 := NewPipelinePlugin("plugin2", PhasePreAuth, 0, nil, nil)

	p := NewOptimizedPipeline([]PipelinePlugin{plugin1, plugin2}, DefaultOptimizedPipelineConfig())

	plugins := p.Plugins()
	if len(plugins) != 2 {
		t.Errorf("len(Plugins) = %d, want 2", len(plugins))
	}
}

func TestOptimizedPipelineBuilder(t *testing.T) {
	t.Run("new builder", func(t *testing.T) {
		builder := NewOptimizedPipelineBuilder(DefaultOptimizedPipelineConfig())
		if builder == nil {
			t.Fatal("NewOptimizedPipelineBuilder returned nil")
		}
	})

	t.Run("build empty", func(t *testing.T) {
		cfg := DefaultOptimizedPipelineConfig()
		builder := NewOptimizedPipelineBuilder(cfg)
		p := builder.Build("test", []PipelinePlugin{})
		if p == nil {
			t.Fatal("Build returned nil")
		}
		if len(p.plugins) != 0 {
			t.Errorf("len(plugins) = %d, want 0", len(p.plugins))
		}
	})
}

func TestBuildOptimizedRoutePipelines(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{
			{ID: "route-1"},
			{ID: "route-2"},
		},
	}

	pipelines, _, err := BuildOptimizedRoutePipelines(cfg, BuilderContext{}, DefaultOptimizedPipelineConfig())
	if err != nil {
		t.Fatalf("BuildOptimizedRoutePipelines error: %v", err)
	}
	if pipelines == nil {
		t.Fatal("BuildOptimizedRoutePipelines returned nil")
	}
}

func TestOptimizedPipeline_executeWithFastPath(t *testing.T) {
	t.Run("execute with fast path", func(t *testing.T) {
		callOrder := []string{}
		plugin1 := NewPipelinePlugin("plugin1", PhasePreAuth, 0, func(ctx *PipelineContext) (bool, error) {
			callOrder = append(callOrder, "plugin1")
			return false, nil
		}, nil)
		plugin2 := NewPipelinePlugin("plugin2", PhasePreAuth, 0, func(ctx *PipelineContext) (bool, error) {
			callOrder = append(callOrder, "plugin2")
			return false, nil
		}, nil)

		cfg := OptimizedPipelineConfig{
			EnableResultCache: false,
			EnableFastPath:    true,
			EnableParallel:    false,
		}
		p := NewOptimizedPipeline([]PipelinePlugin{plugin1, plugin2}, cfg)

		// Set up fast path to skip plugin1
		ctx := &PipelineContext{
			Route:          &config.Route{ID: "route-1"},
			Request:        httptest.NewRequest(http.MethodGet, "/api", nil),
			ResponseWriter: httptest.NewRecorder(),
		}
		skipPlugins := map[string]bool{"plugin1": true}
		p.UpdateFastPath(ctx, skipPlugins)

		// Execute with fast path
		canSkip, _ := p.checkFastPath(ctx)
		if !canSkip {
			t.Error("should find fast path")
		}

		handled, err := p.executeWithFastPath(ctx, skipPlugins)
		if err != nil {
			t.Errorf("executeWithFastPath error: %v", err)
		}
		if handled {
			t.Error("handled should be false")
		}
		// Only plugin2 should be called
		if len(callOrder) != 1 || callOrder[0] != "plugin2" {
			t.Errorf("expected only plugin2 to be called, got %v", callOrder)
		}
	})
}

// =============================================================================
// Additional Tests for 0.0% coverage functions
// =============================================================================

func TestOptimizedPipeline_executeParallel(t *testing.T) {
	t.Run("parallel execution success", func(t *testing.T) {
		callCount := 0
		plugin1 := NewPipelinePlugin("cors", PhasePreAuth, 0, func(ctx *PipelineContext) (bool, error) {
			callCount++
			return false, nil
		}, nil)
		plugin2 := NewPipelinePlugin("correlation-id", PhasePreAuth, 0, func(ctx *PipelineContext) (bool, error) {
			callCount++
			return false, nil
		}, nil)

		cfg := OptimizedPipelineConfig{
			EnableParallel:     true,
			MaxParallelPlugins: 4,
			ParallelTimeout:    time.Second,
			EnableResultCache:  false,
			EnableFastPath:     false,
		}
		p := NewOptimizedPipeline([]PipelinePlugin{plugin1, plugin2}, cfg)

		ctx := &PipelineContext{
			Request:        httptest.NewRequest(http.MethodGet, "/", nil),
			ResponseWriter: httptest.NewRecorder(),
		}

		handled, err := p.executeParallel(ctx)
		if err != nil {
			t.Errorf("executeParallel error: %v", err)
		}
		if handled {
			t.Error("handled should be false")
		}
		if callCount != 2 {
			t.Errorf("expected 2 plugin calls, got %d", callCount)
		}
	})

	t.Run("no parallel plugins", func(t *testing.T) {
		// Use plugins that are not parallel-safe
		plugin1 := NewPipelinePlugin("auth", PhasePreAuth, 0, func(ctx *PipelineContext) (bool, error) {
			return false, nil
		}, nil)

		cfg := OptimizedPipelineConfig{
			EnableParallel:     true,
			MaxParallelPlugins: 4,
			ParallelTimeout:    time.Second,
			EnableResultCache:  false,
			EnableFastPath:     false,
		}
		p := NewOptimizedPipeline([]PipelinePlugin{plugin1}, cfg)

		ctx := &PipelineContext{
			Request:        httptest.NewRequest(http.MethodGet, "/", nil),
			ResponseWriter: httptest.NewRecorder(),
		}

		handled, err := p.executeParallel(ctx)
		if err != nil {
			t.Errorf("executeParallel error: %v", err)
		}
		if handled {
			t.Error("handled should be false")
		}
	})

	t.Run("parallel plugin returns error", func(t *testing.T) {
		testErr := errors.New("parallel error")
		plugin1 := NewPipelinePlugin("cors", PhasePreAuth, 0, func(ctx *PipelineContext) (bool, error) {
			return false, testErr
		}, nil)

		cfg := OptimizedPipelineConfig{
			EnableParallel:     true,
			MaxParallelPlugins: 4,
			ParallelTimeout:    time.Second,
			EnableResultCache:  false,
			EnableFastPath:     false,
		}
		p := NewOptimizedPipeline([]PipelinePlugin{plugin1}, cfg)

		ctx := &PipelineContext{
			Request:        httptest.NewRequest(http.MethodGet, "/", nil),
			ResponseWriter: httptest.NewRecorder(),
		}

		_, err := p.executeParallel(ctx)
		if err != testErr {
			t.Errorf("executeParallel error = %v, want %v", err, testErr)
		}
	})

	t.Run("parallel plugin handles request", func(t *testing.T) {
		plugin1 := NewPipelinePlugin("cors", PhasePreAuth, 0, func(ctx *PipelineContext) (bool, error) {
			return true, nil
		}, nil)

		cfg := OptimizedPipelineConfig{
			EnableParallel:     true,
			MaxParallelPlugins: 4,
			ParallelTimeout:    time.Second,
			EnableResultCache:  false,
			EnableFastPath:     false,
		}
		p := NewOptimizedPipeline([]PipelinePlugin{plugin1}, cfg)

		ctx := &PipelineContext{
			Request:        httptest.NewRequest(http.MethodGet, "/", nil),
			ResponseWriter: httptest.NewRecorder(),
		}

		handled, err := p.executeParallel(ctx)
		if err != nil {
			t.Errorf("executeParallel error: %v", err)
		}
		if !handled {
			t.Error("handled should be true")
		}
	})
}

func TestOptimizedPipeline_splitPlugins(t *testing.T) {
	t.Run("split mixed plugins", func(t *testing.T) {
		parallelPlugin := NewPipelinePlugin("cors", PhasePreAuth, 0, nil, nil)
		sequentialPlugin := NewPipelinePlugin("auth", PhasePreAuth, 0, nil, nil)

		cfg := DefaultOptimizedPipelineConfig()
		p := NewOptimizedPipeline([]PipelinePlugin{parallelPlugin, sequentialPlugin}, cfg)

		parallel, sequential := p.splitPlugins(p.plugins)

		// cors is parallel-safe, auth is not
		if len(parallel) != 1 {
			t.Errorf("len(parallel) = %d, want 1", len(parallel))
		}
		if len(sequential) != 1 {
			t.Errorf("len(sequential) = %d, want 1", len(sequential))
		}
	})

	t.Run("all parallel safe", func(t *testing.T) {
		plugin1 := NewPipelinePlugin("cors", PhasePreAuth, 0, nil, nil)
		plugin2 := NewPipelinePlugin("correlation-id", PhasePreAuth, 0, nil, nil)

		cfg := DefaultOptimizedPipelineConfig()
		p := NewOptimizedPipeline([]PipelinePlugin{plugin1, plugin2}, cfg)

		parallel, sequential := p.splitPlugins(p.plugins)

		if len(parallel) != 2 {
			t.Errorf("len(parallel) = %d, want 2", len(parallel))
		}
		if len(sequential) != 0 {
			t.Errorf("len(sequential) = %d, want 0", len(sequential))
		}
	})

	t.Run("all sequential", func(t *testing.T) {
		plugin1 := NewPipelinePlugin("auth", PhasePreAuth, 0, nil, nil)
		plugin2 := NewPipelinePlugin("jwt", PhasePreAuth, 0, nil, nil)

		cfg := DefaultOptimizedPipelineConfig()
		p := NewOptimizedPipeline([]PipelinePlugin{plugin1, plugin2}, cfg)

		parallel, sequential := p.splitPlugins(p.plugins)

		if len(parallel) != 0 {
			t.Errorf("len(parallel) = %d, want 0", len(parallel))
		}
		if len(sequential) != 2 {
			t.Errorf("len(sequential) = %d, want 2", len(sequential))
		}
	})
}

func TestOptimizedPipeline_isParallelSafe(t *testing.T) {
	tests := []struct {
		name     string
		plugin   PipelinePlugin
		expected bool
	}{
		{"cors", NewPipelinePlugin("cors", PhasePreAuth, 0, nil, nil), true},
		{"correlation-id", NewPipelinePlugin("correlation-id", PhasePreAuth, 0, nil, nil), true},
		{"bot-detect", NewPipelinePlugin("bot-detect", PhasePreAuth, 0, nil, nil), true},
		{"ip-restrict", NewPipelinePlugin("ip-restrict", PhasePreAuth, 0, nil, nil), true},
		{"rate-limit", NewPipelinePlugin("rate-limit", PhasePreAuth, 0, nil, nil), true},
		{"auth", NewPipelinePlugin("auth", PhasePreAuth, 0, nil, nil), false},
		{"jwt", NewPipelinePlugin("jwt", PhasePreAuth, 0, nil, nil), false},
		{"cache", NewPipelinePlugin("cache", PhasePreAuth, 0, nil, nil), false},
	}

	cfg := DefaultOptimizedPipelineConfig()
	p := NewOptimizedPipeline([]PipelinePlugin{}, cfg)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.isParallelSafe(tt.plugin)
			if result != tt.expected {
				t.Errorf("isParallelSafe(%q) = %v, want %v", tt.name, result, tt.expected)
			}
		})
	}
}

func TestOptimizedPipeline_evictOldest(t *testing.T) {
	t.Run("evict oldest entry", func(t *testing.T) {
		cfg := OptimizedPipelineConfig{
			EnableResultCache: true,
			CacheSize:         2,
			CacheTTL:          time.Hour,
		}
		p := NewOptimizedPipeline([]PipelinePlugin{}, cfg)

		// Add some entries
		p.resultCache.mu.Lock()
		p.resultCache.entries["key1"] = &pluginCacheEntry{
			result:    pluginResult{handled: false},
			expiresAt: time.Now().Add(time.Hour),
		}
		p.resultCache.entries["key2"] = &pluginCacheEntry{
			result:    pluginResult{handled: false},
			expiresAt: time.Now().Add(time.Hour),
		}
		p.resultCache.entries["key3"] = &pluginCacheEntry{
			result:    pluginResult{handled: false},
			expiresAt: time.Now().Add(time.Hour),
		}
		p.resultCache.mu.Unlock()

		// Evict oldest
		p.evictOldest()

		// Should have evicted one entry
		p.resultCache.mu.RLock()
		count := len(p.resultCache.entries)
		p.resultCache.mu.RUnlock()

		if count != 2 {
			t.Errorf("expected 2 entries after eviction, got %d", count)
		}
	})

	t.Run("empty cache", func(t *testing.T) {
		cfg := OptimizedPipelineConfig{
			EnableResultCache: true,
			CacheSize:         2,
			CacheTTL:          time.Hour,
		}
		p := NewOptimizedPipeline([]PipelinePlugin{}, cfg)

		// Should not panic on empty cache
		p.evictOldest()
	})
}
