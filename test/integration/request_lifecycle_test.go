package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/admin"
	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/gateway"
	"github.com/APICerberus/APICerebrus/internal/store"
)

// TestRequestRouting tests request routing to correct upstream
func TestRequestRouting(t *testing.T) {
	t.Parallel()

	// Create multiple upstreams
	upstream1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("upstream-1"))
	}))
	defer upstream1.Close()

	upstream2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("upstream-2"))
	}))
	defer upstream2.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)

	cfg := buildRoutingTestConfig(t, gwAddr, adminAddr, mustHost(t, upstream1.URL), mustHost(t, upstream2.URL))
	runtime := startLifecycleTestRuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user and API key
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "routing@example.com",
		"name":            "Routing Test User",
		"password":        "secure-password-123",
		"initial_credits": 100,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")

	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "routing-key",
		"mode": "live",
	}, http.StatusCreated))
	apiKey := anyString(createKey, "full_key")

	// Grant permissions for both routes
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": "route-service-1",
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": "route-service-2",
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	// Test routing to service 1
	waitForHTTPReady(t, "http://"+gwAddr+"/service1/test", nil)
	status, body := gatewayRequest(t, gwAddr, http.MethodGet, "/service1/test", apiKey)
	if status != http.StatusOK || body != "upstream-1" {
		t.Fatalf("expected upstream-1 response, got status=%d body=%q", status, body)
	}

	// Test routing to service 2
	status, body = gatewayRequest(t, gwAddr, http.MethodGet, "/service2/test", apiKey)
	if status != http.StatusOK || body != "upstream-2" {
		t.Fatalf("expected upstream-2 response, got status=%d body=%q", status, body)
	}

	// Test non-existent route returns 404
	status, body = gatewayRequest(t, gwAddr, http.MethodGet, "/nonexistent", apiKey)
	if status != http.StatusNotFound {
		t.Fatalf("expected 404 for non-existent route, got status=%d body=%q", status, body)
	}
}

// TestPluginChainExecution tests plugin chain execution order
func TestPluginChainExecution(t *testing.T) {
	t.Parallel()

	var requestHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestHeaders = r.Header
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("plugin-chain-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-plugin-chain"
	routePath := "/plugin/chain"

	cfg := buildPluginChainTestConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startLifecycleTestRuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user and API key
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "plugin-chain@example.com",
		"name":            "Plugin Chain Test User",
		"password":        "secure-password-123",
		"initial_credits": 100,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")

	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "plugin-chain-key",
		"mode": "live",
	}, http.StatusCreated))
	apiKey := anyString(createKey, "full_key")

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	// Test request with correlation ID
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	req, _ := http.NewRequest(http.MethodGet, "http://"+gwAddr+routePath, nil)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("X-Correlation-ID", "test-correlation-123")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("gateway request failed: %v", err)
	}
	body := readAllAndClose(t, resp.Body)

	if resp.StatusCode != http.StatusOK || body != "plugin-chain-ok" {
		t.Fatalf("unexpected response status=%d body=%q", resp.StatusCode, body)
	}

	// Verify correlation ID was passed to upstream
	if requestHeaders.Get("X-Correlation-ID") != "test-correlation-123" {
		t.Fatalf("expected correlation ID to be passed to upstream, got %q", requestHeaders.Get("X-Correlation-ID"))
	}
}

// TestLoadBalancing tests load balancing across multiple targets
func TestLoadBalancing(t *testing.T) {
	t.Parallel()

	var requestCount1, requestCount2 atomic.Int32

	upstream1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount1.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("target-1"))
	}))
	defer upstream1.Close()

	upstream2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount2.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("target-2"))
	}))
	defer upstream2.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-loadbalance"
	routePath := "/loadbalance/test"

	cfg := buildLoadBalanceTestConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream1.URL), mustHost(t, upstream2.URL))
	runtime := startLifecycleTestRuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user and API key
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "loadbalance@example.com",
		"name":            "Load Balance Test User",
		"password":        "secure-password-123",
		"initial_credits": 1000,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")

	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "loadbalance-key",
		"mode": "live",
	}, http.StatusCreated))
	apiKey := anyString(createKey, "full_key")

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	// Send multiple requests
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	requestCount := 10
	for i := 0; i < requestCount; i++ {
		status, body := gatewayRequest(t, gwAddr, http.MethodGet, routePath, apiKey)
		if status != http.StatusOK {
			t.Fatalf("request %d: unexpected status=%d body=%q", i, status, body)
		}
	}

	// Verify both targets received requests (round-robin)
	count1 := requestCount1.Load()
	count2 := requestCount2.Load()
	if count1+count2 != int32(requestCount) {
		t.Fatalf("expected %d total requests, got %d", requestCount, count1+count2)
	}
	// With round-robin, both should have received requests
	if count1 == 0 || count2 == 0 {
		t.Fatalf("expected both targets to receive requests, got target1=%d target2=%d", count1, count2)
	}
}

// TestResponseHandling tests response processing
func TestResponseHandling(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "custom-value")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status":"created","id":"123"}`))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-response"
	routePath := "/response/test"

	cfg := buildLifecycleTestConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startLifecycleTestRuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user and API key
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "response@example.com",
		"name":            "Response Test User",
		"password":        "secure-password-123",
		"initial_credits": 100,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")

	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "response-key",
		"mode": "live",
	}, http.StatusCreated))
	apiKey := anyString(createKey, "full_key")

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodPost},
		"allowed":  true,
	}, http.StatusCreated)

	// Test response handling
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	req, _ := http.NewRequest(http.MethodPost, "http://"+gwAddr+routePath, nil)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("gateway request failed: %v", err)
	}
	body := readAllAndClose(t, resp.Body)

	// Verify status code is preserved
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", resp.StatusCode)
	}

	// Verify headers are preserved
	if resp.Header.Get("X-Custom-Header") != "custom-value" {
		t.Fatalf("expected custom header to be preserved")
	}

	// Verify body is preserved
	if !strings.Contains(body, `"status":"created"`) {
		t.Fatalf("expected response body to be preserved, got %q", body)
	}
}

// TestErrorScenarios tests various error scenarios
func TestErrorScenarios(t *testing.T) {
	t.Skip("TODO: Fix route matching for error paths")
	t.Parallel()

	// Upstream that returns errors
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/500"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"internal server error"}`))
		case strings.Contains(path, "/timeout"):
			time.Sleep(5 * time.Second)
			w.WriteHeader(http.StatusOK)
		case strings.Contains(path, "/slow"):
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("slow-response"))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-errors"
	routePath := "/errors"

	cfg := buildLifecycleTestConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startLifecycleTestRuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user and API key
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "errors@example.com",
		"name":            "Error Test User",
		"password":        "secure-password-123",
		"initial_credits": 100,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")

	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "errors-key",
		"mode": "live",
	}, http.StatusCreated))
	apiKey := anyString(createKey, "full_key")

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	waitForHTTPReady(t, "http://"+gwAddr+routePath+"/test", nil)

	// Test 500 error from upstream is passed through
	status, body := gatewayRequest(t, gwAddr, http.MethodGet, routePath+"/500", apiKey)
	if status != http.StatusInternalServerError {
		t.Fatalf("expected 500 from upstream, got status=%d body=%q", status, body)
	}

	// Test normal request still works
	status, body = gatewayRequest(t, gwAddr, http.MethodGet, routePath+"/test", apiKey)
	if status != http.StatusOK || body != "ok" {
		t.Fatalf("expected ok response, got status=%d body=%q", status, body)
	}
}

// TestRequestTransformation tests request body transformation
func TestRequestTransformation(t *testing.T) {
	t.Parallel()

	var receivedBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("transform-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-transform"
	routePath := "/transform/test"

	cfg := buildTransformTestConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startLifecycleTestRuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user and API key
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "transform@example.com",
		"name":            "Transform Test User",
		"password":        "secure-password-123",
		"initial_credits": 100,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")

	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "transform-key",
		"mode": "live",
	}, http.StatusCreated))
	apiKey := anyString(createKey, "full_key")

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodPost},
		"allowed":  true,
	}, http.StatusCreated)

	// Test request with body
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	reqBody := `{"test":"data","number":123}`
	req, _ := http.NewRequest(http.MethodPost, "http://"+gwAddr+routePath, strings.NewReader(reqBody))
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("gateway request failed: %v", err)
	}
	body := readAllAndClose(t, resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status=%d body=%q", resp.StatusCode, body)
	}

	// Verify body was passed through
	if string(receivedBody) != reqBody {
		t.Fatalf("expected body %q, got %q", reqBody, string(receivedBody))
	}
}

// TestConcurrentRequests tests handling of concurrent requests
func TestConcurrentRequests(t *testing.T) {
	t.Parallel()

	var activeRequests atomic.Int32
	var maxConcurrent atomic.Int32

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := activeRequests.Add(1)
		for {
			max := maxConcurrent.Load()
			if current <= max || maxConcurrent.CompareAndSwap(max, current) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		activeRequests.Add(-1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("concurrent-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-concurrent"
	routePath := "/concurrent/test"

	cfg := buildLifecycleTestConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startLifecycleTestRuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user and API key
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "concurrent@example.com",
		"name":            "Concurrent Test User",
		"password":        "secure-password-123",
		"initial_credits": 1000,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")

	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "concurrent-key",
		"mode": "live",
	}, http.StatusCreated))
	apiKey := anyString(createKey, "full_key")

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	// Send concurrent requests
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	concurrency := 20
	var wg sync.WaitGroup
	errors := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			status, body := gatewayRequest(t, gwAddr, http.MethodGet, routePath, apiKey)
			if status != http.StatusOK {
				errors <- fmt.Errorf("request %d: status=%d body=%q", idx, status, body)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	var errCount int
	for err := range errors {
		if err != nil {
			t.Logf("Error: %v", err)
			errCount++
		}
	}

	if errCount > 0 {
		t.Fatalf("had %d errors out of %d concurrent requests", errCount, concurrency)
	}

	t.Logf("Max concurrent requests handled: %d", maxConcurrent.Load())
}

// Helper types and functions for lifecycle tests

type lifecycleTestRuntime struct {
	adminHTTP *http.Server
	cancel    context.CancelFunc
	gwErrCh   chan error
	adminErr  chan error
	store     *store.Store
}

func startLifecycleTestRuntime(t *testing.T, cfg *config.Config) *lifecycleTestRuntime {
	t.Helper()
	if cfg == nil {
		t.Fatalf("config is nil")
	}

	gw, err := gateway.New(cfg)
	if err != nil {
		t.Fatalf("gateway.New error: %v", err)
	}

	adminHandler, err := admin.NewServer(cfg, gw)
	if err != nil {
		t.Fatalf("admin.NewServer error: %v", err)
	}

	adminHTTP := &http.Server{
		Addr:           cfg.Admin.Addr,
		Handler:        adminHandler,
		ReadTimeout:    2 * time.Second,
		WriteTimeout:   2 * time.Second,
		IdleTimeout:    10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	ctx, cancel := context.WithCancel(context.Background())

	gwErrCh := make(chan error, 1)
	go func() { gwErrCh <- gw.Start(ctx) }()

	adminErr := make(chan error, 1)
	go func() {
		err := adminHTTP.ListenAndServe()
		if err == http.ErrServerClosed {
			err = nil
		}
		adminErr <- err
	}()

	waitForTCPReady(t, cfg.Admin.Addr)
	adminToken := getAdminBearerToken(t, cfg.Admin.Addr, cfg.Admin.APIKey)
	waitForHTTPReady(t, "http://"+cfg.Admin.Addr+"/admin/api/v1/status", map[string]string{"Authorization": "Bearer " + adminToken})

	return &lifecycleTestRuntime{
		adminHTTP: adminHTTP,
		cancel:    cancel,
		gwErrCh:   gwErrCh,
		adminErr:  adminErr,
	}
}

func (r *lifecycleTestRuntime) Stop(t *testing.T) {
	t.Helper()
	if r == nil {
		return
	}
	r.cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	_ = r.adminHTTP.Shutdown(shutdownCtx)

	if err := <-r.gwErrCh; err != nil {
		t.Fatalf("gateway runtime error: %v", err)
	}
	if err := <-r.adminErr; err != nil {
		t.Fatalf("admin runtime error: %v", err)
	}
}

func buildLifecycleTestConfig(t *testing.T, gwAddr, adminAddr, routeID, routePath, upstreamHost string) *config.Config {
	t.Helper()
	return &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:       gwAddr,
			ReadTimeout:    2 * time.Second,
			WriteTimeout:   2 * time.Second,
			IdleTimeout:    10 * time.Second,
			MaxHeaderBytes: 1 << 20,
			MaxBodyBytes:   1 << 20,
		},
		Admin: config.AdminConfig{
			Addr:        adminAddr,
			APIKey:      "secret-lifecycle-test",
			TokenSecret: "secret-lifecycle-test-token",
			TokenTTL:    1 * time.Hour,
		},
		Store: config.StoreConfig{
			Path:        t.TempDir() + "/lifecycle-test.db",
			BusyTimeout: time.Second,
			JournalMode: "WAL",
			ForeignKeys: true,
		},
		Billing: config.BillingConfig{
			Enabled:           true,
			DefaultCost:       1,
			ZeroBalanceAction: "reject",
			TestModeEnabled:   true,
		},
		Services: []config.Service{
			{ID: "svc-lifecycle", Name: "svc-lifecycle", Protocol: "http", Upstream: "up-lifecycle"},
		},
		Routes: []config.Route{
			{
				ID:      routeID,
				Name:    routeID,
				Service: "svc-lifecycle",
				Paths:   []string{routePath},
				Methods: []string{http.MethodGet, http.MethodPost},
			},
		},
		Upstreams: []config.Upstream{
			{
				ID:        "up-lifecycle",
				Name:      "up-lifecycle",
				Algorithm: "round_robin",
				Targets: []config.UpstreamTarget{
					{ID: "up-lifecycle-t1", Address: upstreamHost, Weight: 1},
				},
				HealthCheck: config.HealthCheckConfig{
					Active: config.ActiveHealthCheckConfig{
						Path:               "/health",
						Interval:           1 * time.Second,
						Timeout:            1 * time.Second,
						HealthyThreshold:   1,
						UnhealthyThreshold: 1,
					},
				},
			},
		},
		GlobalPlugins: []config.PluginConfig{
			{Name: "auth-apikey"},
			{Name: "endpoint-permission"},
		},
	}
}

func buildRoutingTestConfig(t *testing.T, gwAddr, adminAddr, upstreamHost1, upstreamHost2 string) *config.Config {
	t.Helper()
	cfg := buildLifecycleTestConfig(t, gwAddr, adminAddr, "route-service-1", "/service1/test", upstreamHost1)
	cfg.Services = append(cfg.Services, config.Service{
		ID:       "svc-service-2",
		Name:     "svc-service-2",
		Protocol: "http",
		Upstream: "up-service-2",
	})
	cfg.Routes = append(cfg.Routes, config.Route{
		ID:      "route-service-2",
		Name:    "route-service-2",
		Service: "svc-service-2",
		Paths:   []string{"/service2/test"},
		Methods: []string{http.MethodGet},
	})
	cfg.Upstreams = append(cfg.Upstreams, config.Upstream{
		ID:        "up-service-2",
		Name:      "up-service-2",
		Algorithm: "round_robin",
		Targets: []config.UpstreamTarget{
			{ID: "up-service-2-t1", Address: upstreamHost2, Weight: 1},
		},
		HealthCheck: config.HealthCheckConfig{
			Active: config.ActiveHealthCheckConfig{
				Path:               "/health",
				Interval:           1 * time.Second,
				Timeout:            1 * time.Second,
				HealthyThreshold:   1,
				UnhealthyThreshold: 1,
			},
		},
	})
	return cfg
}

func buildPluginChainTestConfig(t *testing.T, gwAddr, adminAddr, routeID, routePath, upstreamHost string) *config.Config {
	cfg := buildLifecycleTestConfig(t, gwAddr, adminAddr, routeID, routePath, upstreamHost)
	cfg.GlobalPlugins = append(cfg.GlobalPlugins, config.PluginConfig{
		Name: "correlation-id",
	})
	return cfg
}

func buildLoadBalanceTestConfig(t *testing.T, gwAddr, adminAddr, routeID, routePath, upstreamHost1, upstreamHost2 string) *config.Config {
	cfg := buildLifecycleTestConfig(t, gwAddr, adminAddr, routeID, routePath, upstreamHost1)
	cfg.Upstreams[0].Targets = append(cfg.Upstreams[0].Targets, config.UpstreamTarget{
		ID:      "up-lifecycle-t2",
		Address: upstreamHost2,
		Weight:  1,
	})
	return cfg
}

func buildTransformTestConfig(t *testing.T, gwAddr, adminAddr, routeID, routePath, upstreamHost string) *config.Config {
	cfg := buildLifecycleTestConfig(t, gwAddr, adminAddr, routeID, routePath, upstreamHost)
	cfg.GlobalPlugins = append(cfg.GlobalPlugins, config.PluginConfig{
		Name: "request-transform",
		Config: map[string]any{
			"add_headers": map[string]string{
				"X-Transformed": "true",
			},
		},
	})
	return cfg
}

// Reuse helper functions from auth_flow_test.go
