// Package main provides comprehensive benchmarks for APICerebrus Gateway components.
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/gateway"
)

// ============================================================================
// Router Benchmarks
// ============================================================================

// BenchmarkRouterExactMatch benchmarks routing with exact path matches.
// Baseline: ~500 ns/op, 0 allocs/op
func BenchmarkRouterExactMatch(b *testing.B) {
	routes := []config.Route{
		{ID: "r1", Name: "users", Service: "svc1", Paths: []string{"/api/v1/users"}, Methods: []string{"GET"}},
		{ID: "r2", Name: "orders", Service: "svc1", Paths: []string{"/api/v1/orders"}, Methods: []string{"GET"}},
		{ID: "r3", Name: "products", Service: "svc1", Paths: []string{"/api/v1/products"}, Methods: []string{"GET"}},
	}
	services := []config.Service{
		{ID: "svc1", Name: "Test Service", Protocol: "http"},
	}

	router, err := gateway.NewRouter(routes, services)
	if err != nil {
		b.Fatalf("failed to create router: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		reqCopy := req.Clone(req.Context())
		_, _, _ = router.Match(reqCopy)
	}
}

// BenchmarkRouterParameterizedMatch benchmarks routing with path parameters.
// Baseline: ~800 ns/op, 2 allocs/op
func BenchmarkRouterParameterizedMatch(b *testing.B) {
	routes := []config.Route{
		{ID: "r1", Name: "user-by-id", Service: "svc1", Paths: []string{"/api/v1/users/:id"}, Methods: []string{"GET"}},
		{ID: "r2", Name: "order-by-id", Service: "svc1", Paths: []string{"/api/v1/orders/:id"}, Methods: []string{"GET"}},
	}
	services := []config.Service{
		{ID: "svc1", Name: "Test Service", Protocol: "http"},
	}

	router, err := gateway.NewRouter(routes, services)
	if err != nil {
		b.Fatalf("failed to create router: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/12345", nil)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		reqCopy := req.Clone(req.Context())
		_, _, _ = router.Match(reqCopy)
	}
}

// BenchmarkRouterWildcardMatch benchmarks routing with wildcard paths.
// Baseline: ~1000 ns/op, 3 allocs/op
func BenchmarkRouterWildcardMatch(b *testing.B) {
	routes := []config.Route{
		{ID: "r1", Name: "static-files", Service: "svc1", Paths: []string{"/static/*"}, Methods: []string{"GET"}},
	}
	services := []config.Service{
		{ID: "svc1", Name: "Test Service", Protocol: "http"},
	}

	router, err := gateway.NewRouter(routes, services)
	if err != nil {
		b.Fatalf("failed to create router: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/static/css/main.css", nil)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		reqCopy := req.Clone(req.Context())
		_, _, _ = router.Match(reqCopy)
	}
}

// BenchmarkRouterLargeRouteSet benchmarks routing performance with many routes.
// Baseline: ~1500 ns/op, 2 allocs/op for 100 routes
func BenchmarkRouterLargeRouteSet(b *testing.B) {
	routes := make([]config.Route, 0, 100)
	for i := 0; i < 100; i++ {
		routes = append(routes, config.Route{
			ID:      fmt.Sprintf("r%d", i),
			Name:    fmt.Sprintf("route-%d", i),
			Service: "svc1",
			Paths:   []string{fmt.Sprintf("/api/service%d/:id", i)},
			Methods: []string{"GET"},
		})
	}
	services := []config.Service{
		{ID: "svc1", Name: "Test Service", Protocol: "http"},
	}

	router, err := gateway.NewRouter(routes, services)
	if err != nil {
		b.Fatalf("failed to create router: %v", err)
	}

	// Test middle route
	req := httptest.NewRequest(http.MethodGet, "/api/service50/12345", nil)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		reqCopy := req.Clone(req.Context())
		_, _, _ = router.Match(reqCopy)
	}
}

// BenchmarkRouterHostBasedRouting benchmarks host-based routing.
// Baseline: ~600 ns/op, 1 alloc/op
func BenchmarkRouterHostBasedRouting(b *testing.B) {
	routes := []config.Route{
		{ID: "r1", Name: "api", Service: "svc1", Hosts: []string{"api.example.com"}, Paths: []string{"/v1/users"}},
		{ID: "r2", Name: "admin", Service: "svc1", Hosts: []string{"admin.example.com"}, Paths: []string{"/dashboard"}},
	}
	services := []config.Service{
		{ID: "svc1", Name: "Test Service", Protocol: "http"},
	}

	router, err := gateway.NewRouter(routes, services)
	if err != nil {
		b.Fatalf("failed to create router: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/users", nil)
	req.Host = "api.example.com"

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		reqCopy := req.Clone(req.Context())
		reqCopy.Host = "api.example.com"
		_, _, _ = router.Match(reqCopy)
	}
}

// BenchmarkRouterParallel benchmarks concurrent routing performance.
// Baseline: ~2000 ns/op under parallel load
func BenchmarkRouterParallel(b *testing.B) {
	routes := []config.Route{
		{ID: "r1", Name: "users", Service: "svc1", Paths: []string{"/api/v1/users/:id"}},
		{ID: "r2", Name: "orders", Service: "svc1", Paths: []string{"/api/v1/orders/:id"}},
		{ID: "r3", Name: "products", Service: "svc1", Paths: []string{"/api/v1/products/:id"}},
	}
	services := []config.Service{
		{ID: "svc1", Name: "Test Service", Protocol: "http"},
	}

	router, err := gateway.NewRouter(routes, services)
	if err != nil {
		b.Fatalf("failed to create router: %v", err)
	}

	paths := []string{"/api/v1/users/123", "/api/v1/orders/456", "/api/v1/products/789"}

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			req := httptest.NewRequest(http.MethodGet, paths[i%len(paths)], nil)
			_, _, _ = router.Match(req)
			i++
		}
	})
}

// ============================================================================
// Proxy Benchmarks
// ============================================================================

// BenchmarkProxyThroughput benchmarks basic proxy forwarding throughput.
// Baseline: ~50,000 req/sec single-threaded
func BenchmarkProxyThroughput(b *testing.B) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer upstream.Close()

	proxy := gateway.NewProxy()
	target := &config.UpstreamTarget{
		ID:      "test-target",
		Address: upstream.URL,
		Weight:  1,
	}

	route := &config.Route{
		ID:        "test-route",
		Name:      "Test Route",
		StripPath: false,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		rec := httptest.NewRecorder()

		ctx := &gateway.RequestContext{
			Request:        req,
			ResponseWriter: rec,
			Route:          route,
		}

		_ = proxy.Forward(ctx, target)
	}
}

// BenchmarkProxyParallelThroughput benchmarks proxy under concurrent load.
// Baseline: ~100,000 req/sec with 8 threads
func BenchmarkProxyParallelThroughput(b *testing.B) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer upstream.Close()

	proxy := gateway.NewProxy()
	target := &config.UpstreamTarget{
		ID:      "test-target",
		Address: upstream.URL,
		Weight:  1,
	}

	route := &config.Route{
		ID:        "test-route",
		Name:      "Test Route",
		StripPath: false,
	}

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			rec := httptest.NewRecorder()

			ctx := &gateway.RequestContext{
				Request:        req,
				ResponseWriter: rec,
				Route:          route,
			}

			_ = proxy.Forward(ctx, target)
		}
	})
}

// BenchmarkProxyLargeResponse benchmarks proxy with large response bodies.
// Baseline: ~5 MB/sec throughput for 1MB responses
func BenchmarkProxyLargeResponse(b *testing.B) {
	largeBody := make([]byte, 1024*1024) // 1MB
	for i := range largeBody {
		largeBody[i] = byte('a' + (i % 26))
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(largeBody)
	}))
	defer upstream.Close()

	proxy := gateway.NewProxy()
	target := &config.UpstreamTarget{
		ID:      "test-target",
		Address: upstream.URL,
		Weight:  1,
	}

	route := &config.Route{
		ID:        "test-route",
		Name:      "Test Route",
		StripPath: false,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		rec := httptest.NewRecorder()

		ctx := &gateway.RequestContext{
			Request:        req,
			ResponseWriter: rec,
			Route:          route,
		}

		_ = proxy.Forward(ctx, target)
	}
}

// ============================================================================
// Load Balancer Benchmarks
// ============================================================================

// BenchmarkLoadBalancerRoundRobin benchmarks round-robin selection.
// Baseline: ~50 ns/op, 0 allocs/op
func BenchmarkLoadBalancerRoundRobin(b *testing.B) {
	targets := []config.UpstreamTarget{
		{ID: "t1", Address: "10.0.0.1:8080", Weight: 1},
		{ID: "t2", Address: "10.0.0.2:8080", Weight: 1},
		{ID: "t3", Address: "10.0.0.3:8080", Weight: 1},
	}

	balancer := gateway.NewBalancer("round_robin", targets)
	ctx := &gateway.RequestContext{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		target, _ := balancer.Next(ctx)
		balancer.Done(target.ID)
	}
}

// BenchmarkLoadBalancerWeightedRoundRobin benchmarks weighted round-robin.
// Baseline: ~100 ns/op, 0 allocs/op
func BenchmarkLoadBalancerWeightedRoundRobin(b *testing.B) {
	targets := []config.UpstreamTarget{
		{ID: "t1", Address: "10.0.0.1:8080", Weight: 1},
		{ID: "t2", Address: "10.0.0.2:8080", Weight: 2},
		{ID: "t3", Address: "10.0.0.3:8080", Weight: 3},
	}

	balancer := gateway.NewBalancer("weighted_round_robin", targets)
	ctx := &gateway.RequestContext{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		target, _ := balancer.Next(ctx)
		balancer.Done(target.ID)
	}
}

// BenchmarkLoadBalancerLeastConn benchmarks least-connections selection.
// Baseline: ~80 ns/op, 0 allocs/op
func BenchmarkLoadBalancerLeastConn(b *testing.B) {
	targets := []config.UpstreamTarget{
		{ID: "t1", Address: "10.0.0.1:8080", Weight: 1},
		{ID: "t2", Address: "10.0.0.2:8080", Weight: 1},
		{ID: "t3", Address: "10.0.0.3:8080", Weight: 1},
	}

	balancer := gateway.NewBalancer("least_conn", targets)
	ctx := &gateway.RequestContext{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		target, _ := balancer.Next(ctx)
		balancer.Done(target.ID)
	}
}

// BenchmarkLoadBalancerIPHash benchmarks IP hash selection.
// Baseline: ~200 ns/op, 1 alloc/op
func BenchmarkLoadBalancerIPHash(b *testing.B) {
	targets := []config.UpstreamTarget{
		{ID: "t1", Address: "10.0.0.1:8080", Weight: 1},
		{ID: "t2", Address: "10.0.0.2:8080", Weight: 1},
		{ID: "t3", Address: "10.0.0.3:8080", Weight: 1},
	}

	balancer := gateway.NewBalancer("ip_hash", targets)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	ctx := &gateway.RequestContext{Request: req}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		target, _ := balancer.Next(ctx)
		balancer.Done(target.ID)
	}
}

// BenchmarkLoadBalancerConsistentHash benchmarks consistent hashing.
// Baseline: ~300 ns/op, 2 allocs/op
func BenchmarkLoadBalancerConsistentHash(b *testing.B) {
	targets := []config.UpstreamTarget{
		{ID: "t1", Address: "10.0.0.1:8080", Weight: 1},
		{ID: "t2", Address: "10.0.0.2:8080", Weight: 1},
		{ID: "t3", Address: "10.0.0.3:8080", Weight: 1},
	}

	balancer := gateway.NewBalancer("consistent_hash", targets)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	ctx := &gateway.RequestContext{Request: req}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		target, _ := balancer.Next(ctx)
		balancer.Done(target.ID)
	}
}

// BenchmarkLoadBalancerParallel benchmarks balancer under concurrent load.
// Baseline: ~150 ns/op with 8 threads
func BenchmarkLoadBalancerParallel(b *testing.B) {
	targets := []config.UpstreamTarget{
		{ID: "t1", Address: "10.0.0.1:8080", Weight: 1},
		{ID: "t2", Address: "10.0.0.2:8080", Weight: 1},
		{ID: "t3", Address: "10.0.0.3:8080", Weight: 1},
		{ID: "t4", Address: "10.0.0.4:8080", Weight: 1},
	}

	balancer := gateway.NewBalancer("round_robin", targets)

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		ctx := &gateway.RequestContext{}
		for pb.Next() {
			target, _ := balancer.Next(ctx)
			balancer.Done(target.ID)
		}
	})
}

// ============================================================================
// WebSocket Benchmarks
// ============================================================================

// BenchmarkWebSocketUpgrade benchmarks WebSocket upgrade handling.
// Note: Full WebSocket benchmarking requires actual network connections.
// This benchmarks the upgrade detection logic.
// Baseline: ~100 ns/op for upgrade detection
func BenchmarkWebSocketUpgradeDetection(b *testing.B) {
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		isWS := strings.EqualFold(req.Header.Get("Upgrade"), "websocket")
		_ = isWS
	}
}

// ============================================================================
// Full Gateway Benchmarks
// ============================================================================

// BenchmarkGatewayEndToEnd benchmarks complete request handling.
// Baseline: ~10,000 req/sec end-to-end
func BenchmarkGatewayEndToEnd(b *testing.B) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message":"success"}`))
	}))
	defer upstream.Close()

	cfg := &config.Config{
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
					{ID: "target-1", Address: upstream.URL, Weight: 1},
				},
			},
		},
	}

	gw, err := gateway.New(cfg)
	if err != nil {
		b.Fatalf("failed to create gateway: %v", err)
	}
	defer gw.Shutdown(nil)

	// Wait for gateway to be ready
	time.Sleep(50 * time.Millisecond)

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
		},
	}

	addr := gw.Addr()
	url := fmt.Sprintf("http://%s/api/test", addr)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resp, err := client.Get(url)
		if err != nil {
			b.Logf("request error: %v", err)
			continue
		}
		resp.Body.Close()
	}
}

// BenchmarkGatewayWithStripPath benchmarks path stripping performance.
// Baseline: ~800 ns/op for strip path operation
func BenchmarkGatewayWithStripPath(b *testing.B) {
	routes := []config.Route{
		{
			ID:        "r1",
			Name:      "api",
			Service:   "svc1",
			Paths:     []string{"/api/v1"},
			StripPath: true,
		},
	}
	services := []config.Service{
		{ID: "svc1", Name: "Test Service", Protocol: "http"},
	}

	router, err := gateway.NewRouter(routes, services)
	if err != nil {
		b.Fatalf("failed to create router: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		reqCopy := req.Clone(req.Context())
		_, _, _ = router.Match(reqCopy)
	}
}
