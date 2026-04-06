package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/analytics"
	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/gateway"
	"github.com/APICerberus/APICerebrus/internal/plugin"
)

// BenchmarkOptimizedProxy benchmarks the optimized proxy implementation.
func BenchmarkOptimizedProxy(b *testing.B) {
	cfg := gateway.DefaultOptimizedProxyConfig()
	cfg.EnableCoalescing = false // Disable for fair comparison
	proxy := gateway.NewOptimizedProxy(cfg)
	defer proxy.Close()

	// Create test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	target := &config.UpstreamTarget{
		ID:      "test-target",
		Address: ts.URL,
		Weight:  1,
	}

	route := &config.Route{
		ID:        "test-route",
		Name:      "Test Route",
		StripPath: false,
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest("GET", "/api/test", nil)
			rec := httptest.NewRecorder()

			ctx := &gateway.RequestContext{
				Request:        req,
				ResponseWriter: rec,
				Route:          route,
			}

			err := proxy.Forward(ctx, target)
			if err != nil {
				b.Fatalf("proxy forward failed: %v", err)
			}
		}
	})
}

// BenchmarkStandardProxy benchmarks the standard proxy implementation.
func BenchmarkStandardProxy(b *testing.B) {
	proxy := gateway.NewProxy()

	// Create test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	target := &config.UpstreamTarget{
		ID:      "test-target",
		Address: ts.URL,
		Weight:  1,
	}

	route := &config.Route{
		ID:        "test-route",
		Name:      "Test Route",
		StripPath: false,
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest("GET", "/api/test", nil)
			rec := httptest.NewRecorder()

			ctx := &gateway.RequestContext{
				Request:        req,
				ResponseWriter: rec,
				Route:          route,
			}

			err := proxy.Forward(ctx, target)
			if err != nil {
				b.Fatalf("proxy forward failed: %v", err)
			}
		}
	})
}

// BenchmarkOptimizedAnalytics benchmarks the optimized analytics engine.
func BenchmarkOptimizedAnalytics(b *testing.B) {
	cfg := analytics.DefaultOptimizedEngineConfig()
	cfg.DropOnOverflow = true
	cfg.WorkerCount = 4
	engine := analytics.NewOptimizedEngine(cfg)
	defer engine.Stop()

	metric := analytics.RequestMetric{
		Timestamp:   time.Now().UTC(),
		RouteID:     "test-route",
		RouteName:   "Test Route",
		Method:      "GET",
		Path:        "/api/test",
		StatusCode:  200,
		LatencyMS:   5,
		BytesIn:     100,
		BytesOut:    200,
		Error:       false,
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			engine.Record(metric)
		}
	})
}

// BenchmarkStandardAnalytics benchmarks the standard analytics engine.
func BenchmarkStandardAnalytics(b *testing.B) {
	engine := analytics.NewEngine(analytics.EngineConfig{
		RingBufferSize:  100_000,
		BucketRetention: 24 * time.Hour,
	})

	metric := analytics.RequestMetric{
		Timestamp:   time.Now().UTC(),
		RouteID:     "test-route",
		RouteName:   "Test Route",
		Method:      "GET",
		Path:        "/api/test",
		StatusCode:  200,
		LatencyMS:   5,
		BytesIn:     100,
		BytesOut:    200,
		Error:       false,
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			engine.Record(metric)
		}
	})
}

// BenchmarkOptimizedPipeline benchmarks the optimized pipeline.
func BenchmarkOptimizedPipeline(b *testing.B) {
	plugins := []plugin.PipelinePlugin{
		createMockPlugin("cors", plugin.PhasePreAuth, 10),
		createMockPlugin("correlation-id", plugin.PhasePreAuth, 20),
		createMockPlugin("rate-limit", plugin.PhasePreProxy, 30),
	}

	cfg := plugin.DefaultOptimizedPipelineConfig()
	cfg.EnableParallel = true
	pipeline := plugin.NewOptimizedPipeline(plugins, cfg)

	req := httptest.NewRequest("GET", "/api/test", nil)
	rec := httptest.NewRecorder()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &plugin.PipelineContext{
				Request:        req,
				ResponseWriter: rec,
				Metadata:       make(map[string]any),
			}
			_, _ = pipeline.Execute(ctx)
		}
	})
}

// BenchmarkStandardPipeline benchmarks the standard pipeline.
func BenchmarkStandardPipeline(b *testing.B) {
	plugins := []plugin.PipelinePlugin{
		createMockPlugin("cors", plugin.PhasePreAuth, 10),
		createMockPlugin("correlation-id", plugin.PhasePreAuth, 20),
		createMockPlugin("rate-limit", plugin.PhasePreProxy, 30),
	}

	pipeline := plugin.NewPipeline(plugins)

	req := httptest.NewRequest("GET", "/api/test", nil)
	rec := httptest.NewRecorder()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx := &plugin.PipelineContext{
				Request:        req,
				ResponseWriter: rec,
				Metadata:       make(map[string]any),
			}
			_, _ = pipeline.Execute(ctx)
		}
	})
}

// createMockPlugin creates a mock plugin for benchmarking.
func createMockPlugin(name string, phase plugin.Phase, priority int) plugin.PipelinePlugin {
	return plugin.NewPipelinePlugin(name, phase, priority,
		func(ctx *plugin.PipelineContext) (bool, error) {
			if ctx.Metadata == nil {
				ctx.Metadata = make(map[string]any)
			}
			ctx.Metadata[name+"_executed"] = true
			return false, nil
		},
		func(ctx *plugin.PipelineContext, proxyErr error) {},
	)
}

// BenchmarkFullRequestFlow benchmarks the complete request flow.
func BenchmarkFullRequestFlow(b *testing.B) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message":"success"}`))
	}))
	defer upstream.Close()

	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:       "localhost:0",
			ReadTimeout:    30 * time.Second,
			WriteTimeout:   30 * time.Second,
			IdleTimeout:    120 * time.Second,
			MaxHeaderBytes: 1 << 20,
		},
		Routes: []config.Route{
			{
				ID:      "test-route",
				Name:    "Test Route",
				Paths:   []string{"/api/test"},
				Service: "test-service",
			},
		},
		Services: []config.Service{
			{
				ID:       "test-service",
				Name:     "Test Service",
				Protocol: "http",
				Upstream: "test-upstream",
			},
		},
		Upstreams: []config.Upstream{
			{
				ID:        "test-upstream",
				Name:      "Test Upstream",
				Algorithm: "round_robin",
				Targets: []config.UpstreamTarget{
					{
						ID:      "target-1",
						Address: upstream.URL,
						Weight:  1,
					},
				},
			},
		},
	}

	gw, err := gateway.New(cfg)
	if err != nil {
		b.Fatalf("failed to create gateway: %v", err)
	}
	defer gw.Shutdown(nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := gw.Start(ctx); err != nil {
			b.Logf("gateway start error: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	addr := gw.Addr()
	if addr == "" {
		b.Fatal("gateway address is empty")
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := client.Get(fmt.Sprintf("http://%s/api/test", addr))
			if err != nil {
				b.Logf("request error: %v", err)
				continue
			}
			resp.Body.Close()
		}
	})
}
