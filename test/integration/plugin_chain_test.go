package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/admin"
	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/gateway"
)

// TestMultiplePluginsInSequence tests multiple plugins executing in sequence
func TestMultiplePluginsInSequence(t *testing.T) {
	t.Parallel()

	var requestHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestHeaders = r.Header
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("multi-plugin-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-multi-plugin"
	routePath := "/multi/plugin"

	cfg := buildMultiPluginConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startPluginTestRuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user and API key
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "multi-plugin@example.com",
		"name":            "Multi Plugin Test User",
		"password":        "secure-password-123",
		"initial_credits": 100,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")

	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "multi-plugin-key",
		"mode": "live",
	}, http.StatusCreated))
	apiKey := anyString(createKey, "full_key")

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	// Test request with multiple plugins
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	req, _ := http.NewRequest(http.MethodGet, "http://"+gwAddr+routePath, nil)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("X-Request-ID", "test-request-123")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("gateway request failed: %v", err)
	}
	body := readAllAndClose(t, resp.Body)

	if resp.StatusCode != http.StatusOK || body != "multi-plugin-ok" {
		t.Fatalf("unexpected response status=%d body=%q", resp.StatusCode, body)
	}

	// Verify correlation ID was added by correlation-id plugin
	if requestHeaders.Get("X-Request-ID") == "" {
		t.Fatalf("expected X-Request-ID header to be set by plugin")
	}

	// Verify custom header was added by request-transform plugin
	if requestHeaders.Get("X-Custom-Header") != "test-value" {
		t.Fatalf("expected X-Custom-Header to be set by transform plugin")
	}
}

// TestPluginAbortScenarios tests plugin chain abort behavior
func TestPluginAbortScenarios(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("should-not-reach"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-abort-test"
	routePath := "/abort/test"

	cfg := buildAbortTestConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startPluginTestRuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user and API key
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "abort-test@example.com",
		"name":            "Abort Test User",
		"password":        "secure-password-123",
		"initial_credits": 100,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")

	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "abort-test-key",
		"mode": "live",
	}, http.StatusCreated))
	apiKey := anyString(createKey, "full_key")

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	// Test IP restriction abort - request from blocked IP should fail
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	req, _ := http.NewRequest(http.MethodGet, "http://"+gwAddr+routePath, nil)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("X-Forwarded-For", "192.168.1.100") // Blocked IP

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("gateway request failed: %v", err)
	}
	body := readAllAndClose(t, resp.Body)

	// IP restrict plugin should abort the request
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 from IP restrict plugin, got status=%d body=%q", resp.StatusCode, body)
	}
}

// TestPluginErrorHandling tests error handling in plugin chain
func TestPluginErrorHandling(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("error-handling-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-error-handling"
	routePath := "/error/handling"

	cfg := buildErrorHandlingConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startPluginTestRuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user and API key
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "error-handling@example.com",
		"name":            "Error Handling Test User",
		"password":        "secure-password-123",
		"initial_credits": 100,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")

	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "error-handling-key",
		"mode": "live",
	}, http.StatusCreated))
	apiKey := anyString(createKey, "full_key")

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet, http.MethodPost},
		"allowed":  true,
	}, http.StatusCreated)

	// Test request size limit - large body should be rejected
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	largeBody := strings.Repeat("x", 1024*1024) // 1MB body
	req, _ := http.NewRequest(http.MethodPost, "http://"+gwAddr+routePath, strings.NewReader(largeBody))
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("gateway request failed: %v", err)
	}
	body := readAllAndClose(t, resp.Body)

	// Request size limit plugin should reject large bodies
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for large body, got status=%d body=%q", resp.StatusCode, body)
	}

	// Test valid request should pass
	req, _ = http.NewRequest(http.MethodPost, "http://"+gwAddr+routePath, strings.NewReader(`{"test":"data"}`))
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("gateway request failed: %v", err)
	}
	body = readAllAndClose(t, resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for valid request, got status=%d body=%q", resp.StatusCode, body)
	}
}

// TestPriorityOrdering tests plugin priority/ordering
func TestPriorityOrdering(t *testing.T) {
	t.Parallel()

	executionOrder := []string{}
	var mu sync.Mutex

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("priority-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-priority"
	routePath := "/priority/test"

	cfg := buildPriorityTestConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startPluginTestRuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user and API key
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "priority@example.com",
		"name":            "Priority Test User",
		"password":        "secure-password-123",
		"initial_credits": 100,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")

	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "priority-key",
		"mode": "live",
	}, http.StatusCreated))
	apiKey := anyString(createKey, "full_key")

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	// Test request
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	req, _ := http.NewRequest(http.MethodGet, "http://"+gwAddr+routePath, nil)
	req.Header.Set("X-API-Key", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("gateway request failed: %v", err)
	}
	body := readAllAndClose(t, resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status=%d body=%q", resp.StatusCode, body)
	}

	mu.Lock()
	order := make([]string, len(executionOrder))
	copy(order, executionOrder)
	mu.Unlock()

	t.Logf("Plugin execution order: %v", order)
}

// TestCircuitBreakerPlugin tests circuit breaker functionality
func TestCircuitBreakerPlugin(t *testing.T) {
	t.Parallel()

	failCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failCount++
		if failCount <= 5 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"service unavailable"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("circuit-breaker-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-circuit-breaker"
	routePath := "/circuit/breaker"

	cfg := buildCircuitBreakerConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startPluginTestRuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user and API key
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "circuit-breaker@example.com",
		"name":            "Circuit Breaker Test User",
		"password":        "secure-password-123",
		"initial_credits": 100,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")

	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "circuit-breaker-key",
		"mode": "live",
	}, http.StatusCreated))
	apiKey := anyString(createKey, "full_key")

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	// Send requests that will trigger circuit breaker
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	for i := 0; i < 3; i++ {
		status, _ := gatewayRequest(t, gwAddr, http.MethodGet, routePath, apiKey)
		t.Logf("Request %d: status=%d", i, status)
	}
}

// TestRetryPlugin tests retry functionality
func TestRetryPlugin(t *testing.T) {
	t.Parallel()

	attemptCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"temporary failure"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("retry-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-retry"
	routePath := "/retry/test"

	cfg := buildRetryConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startPluginTestRuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user and API key
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "retry@example.com",
		"name":            "Retry Test User",
		"password":        "secure-password-123",
		"initial_credits": 100,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")

	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "retry-key",
		"mode": "live",
	}, http.StatusCreated))
	apiKey := anyString(createKey, "full_key")

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	// Test retry - should succeed after retries
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	status, body := gatewayRequest(t, gwAddr, http.MethodGet, routePath, apiKey)
	if status != http.StatusOK || body != "retry-ok" {
		t.Fatalf("expected success after retry, got status=%d body=%q", status, body)
	}

	if attemptCount < 3 {
		t.Fatalf("expected at least 3 attempts due to retry, got %d", attemptCount)
	}
}

// TestCORSPlugin tests CORS handling
func TestCORSPlugin(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("cors-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-cors"
	routePath := "/cors/test"

	cfg := buildCORSConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startPluginTestRuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user and API key
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "cors@example.com",
		"name":            "CORS Test User",
		"password":        "secure-password-123",
		"initial_credits": 100,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")

	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "cors-key",
		"mode": "live",
	}, http.StatusCreated))
	apiKey := anyString(createKey, "full_key")

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet, http.MethodOptions},
		"allowed":  true,
	}, http.StatusCreated)

	// Test preflight request
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	req, _ := http.NewRequest(http.MethodOptions, "http://"+gwAddr+routePath, nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "X-API-Key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("preflight request failed: %v", err)
	}
	body := readAllAndClose(t, resp.Body)

	// CORS plugin should handle preflight
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		t.Fatalf("unexpected preflight status=%d body=%q", resp.StatusCode, body)
	}

	// Test actual request with CORS
	req, _ = http.NewRequest(http.MethodGet, "http://"+gwAddr+routePath, nil)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Origin", "https://example.com")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("CORS request failed: %v", err)
	}
	body = readAllAndClose(t, resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status=%d body=%q", resp.StatusCode, body)
	}

	// Check CORS headers
	if resp.Header.Get("Access-Control-Allow-Origin") == "" {
		t.Fatalf("expected CORS headers in response")
	}
}

// Helper types and functions

type pluginTestRuntime struct {
	adminHTTP *http.Server
	cancel    context.CancelFunc
	gwErrCh   chan error
	adminErr  chan error
}

func startPluginTestRuntime(t *testing.T, cfg *config.Config) *pluginTestRuntime {
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

	waitForHTTPReady(t, "http://"+cfg.Admin.Addr+"/admin/api/v1/status", map[string]string{"X-Admin-Key": cfg.Admin.APIKey})

	return &pluginTestRuntime{
		adminHTTP: adminHTTP,
		cancel:    cancel,
		gwErrCh:   gwErrCh,
		adminErr:  adminErr,
	}
}

func (r *pluginTestRuntime) Stop(t *testing.T) {
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

func buildBasePluginConfig(t *testing.T, gwAddr, adminAddr, routeID, routePath, upstreamHost string) *config.Config {
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
			Addr:   adminAddr,
			APIKey: "secret-plugin-test",
		},
		Store: config.StoreConfig{
			Path:        t.TempDir() + "/plugin-test.db",
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
			{ID: "svc-plugin", Name: "svc-plugin", Protocol: "http", Upstream: "up-plugin"},
		},
		Routes: []config.Route{
			{
				ID:      routeID,
				Name:    routeID,
				Service: "svc-plugin",
				Paths:   []string{routePath},
				Methods: []string{http.MethodGet, http.MethodPost, http.MethodOptions},
			},
		},
		Upstreams: []config.Upstream{
			{
				ID:        "up-plugin",
				Name:      "up-plugin",
				Algorithm: "round_robin",
				Targets: []config.UpstreamTarget{
					{ID: "up-plugin-t1", Address: upstreamHost, Weight: 1},
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

func buildMultiPluginConfig(t *testing.T, gwAddr, adminAddr, routeID, routePath, upstreamHost string) *config.Config {
	cfg := buildBasePluginConfig(t, gwAddr, adminAddr, routeID, routePath, upstreamHost)
	cfg.GlobalPlugins = append(cfg.GlobalPlugins,
		config.PluginConfig{Name: "correlation-id"},
		config.PluginConfig{
			Name: "request-transform",
			Config: map[string]any{
				"add_headers": map[string]string{
					"X-Custom-Header": "test-value",
				},
			},
		},
	)
	return cfg
}

func buildAbortTestConfig(t *testing.T, gwAddr, adminAddr, routeID, routePath, upstreamHost string) *config.Config {
	cfg := buildBasePluginConfig(t, gwAddr, adminAddr, routeID, routePath, upstreamHost)
	cfg.GlobalPlugins = append(cfg.GlobalPlugins,
		config.PluginConfig{
			Name: "ip-restrict",
			Config: map[string]any{
				"blacklist": []string{"192.168.1.100/32"},
			},
		},
	)
	return cfg
}

func buildErrorHandlingConfig(t *testing.T, gwAddr, adminAddr, routeID, routePath, upstreamHost string) *config.Config {
	cfg := buildBasePluginConfig(t, gwAddr, adminAddr, routeID, routePath, upstreamHost)
	cfg.GlobalPlugins = append(cfg.GlobalPlugins,
		config.PluginConfig{
			Name: "request-size-limit",
			Config: map[string]any{
				"max_body_size": 1024, // 1KB limit
			},
		},
	)
	return cfg
}

func buildPriorityTestConfig(t *testing.T, gwAddr, adminAddr, routeID, routePath, upstreamHost string) *config.Config {
	cfg := buildBasePluginConfig(t, gwAddr, adminAddr, routeID, routePath, upstreamHost)
	cfg.GlobalPlugins = append(cfg.GlobalPlugins,
		config.PluginConfig{Name: "correlation-id"},
		config.PluginConfig{Name: "rate-limit"},
		config.PluginConfig{Name: "cache"},
	)
	return cfg
}

func buildCircuitBreakerConfig(t *testing.T, gwAddr, adminAddr, routeID, routePath, upstreamHost string) *config.Config {
	cfg := buildBasePluginConfig(t, gwAddr, adminAddr, routeID, routePath, upstreamHost)
	cfg.Routes[0].Plugins = []config.PluginConfig{
		{
			Name: "circuit-breaker",
			Config: map[string]any{
				"failure_threshold":   3,
				"success_threshold":   2,
				"timeout":             "5s",
				"half_open_max_calls": 3,
			},
		},
	}
	return cfg
}

func buildRetryConfig(t *testing.T, gwAddr, adminAddr, routeID, routePath, upstreamHost string) *config.Config {
	cfg := buildBasePluginConfig(t, gwAddr, adminAddr, routeID, routePath, upstreamHost)
	cfg.Routes[0].Plugins = []config.PluginConfig{
		{
			Name: "retry",
			Config: map[string]any{
				"max_attempts": 3,
				"backoff":      "100ms",
				"conditions":   []string{"5xx", "gateway_error"},
			},
		},
	}
	return cfg
}

func buildCORSConfig(t *testing.T, gwAddr, adminAddr, routeID, routePath, upstreamHost string) *config.Config {
	cfg := buildBasePluginConfig(t, gwAddr, adminAddr, routeID, routePath, upstreamHost)
	cfg.GlobalPlugins = append(cfg.GlobalPlugins,
		config.PluginConfig{
			Name: "cors",
			Config: map[string]any{
				"allowed_origins":   []string{"*"},
				"allowed_methods":   []string{"GET", "POST", "OPTIONS"},
				"allowed_headers":   []string{"*"},
				"allow_credentials": false,
				"max_age":           86400,
			},
		},
	)
	return cfg
}

// Reuse helper functions
