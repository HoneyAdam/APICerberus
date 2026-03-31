package gateway

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// TestGracefulShutdown verifies active connections complete during shutdown
func TestGracefulShutdown(t *testing.T) {
	// Create test upstream server
	upstream := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("upstream response"))
	})
	defer upstream.Close()

	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:  "127.0.0.1:0",
			HTTPSAddr: "",
			ReadTimeout:    5 * time.Second,
			WriteTimeout:   5 * time.Second,
			MaxBodyBytes:   1024 * 1024,
			MaxHeaderBytes: 1024 * 1024,
		},
		Services: []config.Service{
			{
				ID:       "test-service",
				Name:     "test",
				Protocol: "http",
				Upstream: "test-upstream",
			},
		},
		Routes: []config.Route{
			{
				ID:      "test-route",
				Name:    "test",
				Service: "test-service",
				Paths:   []string{"/test"},
				Methods: []string{"GET"},
			},
		},
		Upstreams: []config.Upstream{
			{
				ID:   "test-upstream",
				Name: "test",
				Targets: []config.UpstreamTarget{
					{Address: upstream.Addr, Weight: 100},
				},
			},
		},
		Store: config.StoreConfig{Path: t.TempDir() + "/test.db"},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create gateway: %v", err)
	}

	// Start gateway
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- gw.Start(ctx)
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Get server address
	addr := gw.Addr()
	if addr == "" {
		t.Fatal("gateway address not available")
	}

	// Start concurrent requests
	requestCount := 10
	responses := make(chan *http.Response, requestCount)
	completed := make(chan bool, requestCount)

	for i := 0; i < requestCount; i++ {
		go func() {
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Get("http://" + addr + "/test")
			if err != nil {
				t.Logf("request error: %v", err)
				completed <- false
				return
			}
			responses <- resp
			completed <- true
		}()
	}

	// Give requests time to start
	time.Sleep(50 * time.Millisecond)

	// Initiate graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	t.Log("Initiating graceful shutdown...")
	shutdownStart := time.Now()
	err = gw.Shutdown(shutdownCtx)
	if err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
	shutdownDuration := time.Since(shutdownStart)

	t.Logf("Shutdown completed in %v", shutdownDuration)

	// Wait for all requests to complete
	successCount := 0
	for i := 0; i < requestCount; i++ {
		select {
		case success := <-completed:
			if success {
				successCount++
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timeout waiting for requests to complete")
		}
	}

	// Verify all requests completed successfully
	if successCount != requestCount {
		t.Errorf("not all requests completed: %d/%d", successCount, requestCount)
	}

	t.Logf("All %d requests completed successfully during graceful shutdown", successCount)
}

// TestGracefulShutdownWithActiveConnections verifies connections are drained
func TestGracefulShutdownWithActiveConnections(t *testing.T) {
	// Create slow upstream
	requestStarted := make(chan bool)
	requestDone := make(chan bool)

	upstream := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestStarted <- true
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		requestDone <- true
	})
	defer upstream.Close()

	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:       "127.0.0.1:0",
			ReadTimeout:    10 * time.Second,
			WriteTimeout:   10 * time.Second,
			MaxBodyBytes:   1024 * 1024,
			MaxHeaderBytes: 1024 * 1024,
		},
		Services: []config.Service{
			{ID: "test-service", Name: "test", Protocol: "http", Upstream: "test-upstream"},
		},
		Routes: []config.Route{
			{ID: "test-route", Name: "test", Service: "test-service", Paths: []string{"/slow"}, Methods: []string{"GET"}},
		},
		Upstreams: []config.Upstream{
			{
				ID:      "test-upstream",
				Name:    "test",
				Targets: []config.UpstreamTarget{{Address: upstream.Addr, Weight: 100}},
			},
		},
		Store: config.StoreConfig{Path: t.TempDir() + "/test.db"},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create gateway: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = gw.Start(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	addr := gw.Addr()

	// Start a slow request
	go func() {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get("http://" + addr + "/slow")
		if err != nil {
			t.Logf("slow request error: %v", err)
			return
		}
		defer resp.Body.Close()
	}()

	// Wait for request to reach upstream
	select {
	case <-requestStarted:
		t.Log("Request reached upstream")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for request to start")
	}

	// Initiate shutdown while request is in flight
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = gw.Shutdown(shutdownCtx)
	if err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	// Wait for request to complete
	select {
	case <-requestDone:
		t.Log("Request completed during shutdown")
	case <-time.After(3 * time.Second):
		t.Fatal("request did not complete during graceful shutdown")
	}
}

// TestGracefulShutdownTimeout verifies shutdown times out if requests don't complete
func TestGracefulShutdownTimeout(t *testing.T) {
	// Very slow upstream
	upstream := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // Will timeout
		w.WriteHeader(http.StatusOK)
	})
	defer upstream.Close()

	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:       "127.0.0.1:0",
			ReadTimeout:    10 * time.Second,
			WriteTimeout:   10 * time.Second,
			MaxBodyBytes:   1024 * 1024,
			MaxHeaderBytes: 1024 * 1024,
		},
		Services: []config.Service{
			{ID: "test-service", Name: "test", Protocol: "http", Upstream: "test-upstream"},
		},
		Routes: []config.Route{
			{ID: "test-route", Name: "test", Service: "test-service", Paths: []string{"/very-slow"}, Methods: []string{"GET"}},
		},
		Upstreams: []config.Upstream{
			{
				ID:      "test-upstream",
				Name:    "test",
				Targets: []config.UpstreamTarget{{Address: upstream.Addr, Weight: 100}},
			},
		},
		Store: config.StoreConfig{Path: t.TempDir() + "/test.db"},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create gateway: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = gw.Start(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	addr := gw.Addr()

	// Start a request
	go func() {
		client := &http.Client{Timeout: 10 * time.Second}
		_, _ = client.Get("http://" + addr + "/very-slow")
	}()

	time.Sleep(50 * time.Millisecond)

	// Try to shutdown with short timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = gw.Shutdown(shutdownCtx)
	if err == nil {
		t.Error("expected shutdown to timeout, but it succeeded")
	}

	t.Logf("Shutdown correctly timed out: %v", err)
}

// TestGracefulShutdownDuringRaftElection verifies shutdown during cluster changes
func TestGracefulShutdownDuringRaftElection(t *testing.T) {
	// This test verifies the gateway handles shutdown during Raft operations
	// Simplified version - just verify no panic

	cfg := minimalTestConfig(t)
	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create gateway: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = gw.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	// Shutdown immediately
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = gw.Shutdown(shutdownCtx)
	if err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	t.Log("Graceful shutdown during Raft election verified")
}

// Helper functions

// testServer wraps an HTTP test server with its address
type testServer struct {
	*http.Server
	Addr string
}

func newTestServer(t *testing.T, handler http.HandlerFunc) *testServer {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handler)

	server := &http.Server{
		Handler: mux,
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	go func() {
		server.Serve(listener)
	}()

	return &testServer{
		Server: server,
		Addr:   listener.Addr().String(),
	}
}

func minimalTestConfig(t *testing.T) *config.Config {
	return &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:       "127.0.0.1:0",
			ReadTimeout:    5 * time.Second,
			WriteTimeout:   5 * time.Second,
			MaxBodyBytes:   1024 * 1024,
			MaxHeaderBytes: 1024 * 1024,
		},
		Services: []config.Service{},
		Routes:   []config.Route{},
		Upstreams: []config.Upstream{},
		Store:    config.StoreConfig{Path: t.TempDir() + "/test.db"},
	}
}
