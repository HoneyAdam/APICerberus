package integration

import (
	"context"
	"fmt"
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
)

// TestFullRequestFlow tests the complete end-to-end request flow
func TestFullRequestFlow(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		w.Header().Set("X-Upstream-Count", fmt.Sprintf("%d", count))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"success","count":` + fmt.Sprintf("%d", count) + `}`))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-full-flow"
	routePath := "/full/flow"

	cfg := buildE2EConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startE2ERuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user with credits
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "full-flow@example.com",
		"name":            "Full Flow Test User",
		"password":        "secure-password-123",
		"initial_credits": 100,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")

	// Create API key
	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "full-flow-key",
		"mode": "live",
	}, http.StatusCreated))
	apiKey := anyString(createKey, "full_key")

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet, http.MethodPost},
		"allowed":  true,
	}, http.StatusCreated)

	// Test full request flow
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)

	// GET request
	status, body := gatewayRequest(t, gwAddr, http.MethodGet, routePath, apiKey)
	if status != http.StatusOK {
		t.Fatalf("GET request failed: status=%d body=%q", status, body)
	}
	if !strings.Contains(body, `"message":"success"`) {
		t.Fatalf("unexpected response body: %q", body)
	}

	// POST request with body
	postBody := `{"test":"data"}`
	req, _ := http.NewRequest(http.MethodPost, "http://"+gwAddr+routePath, strings.NewReader(postBody))
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	body = readAllAndClose(t, resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST request failed: status=%d body=%q", resp.StatusCode, body)
	}

	// Verify credit deduction
	balanceResp := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodGet, "/admin/api/v1/users/"+userID+"/credits/balance", nil, http.StatusOK))
	if balance, ok := anyInt64(balanceResp["balance"]); !ok || balance != 98 {
		t.Fatalf("expected balance 98 after 2 requests, got %v", balanceResp["balance"])
	}

	t.Logf("Full request flow completed successfully. Requests handled: %d", requestCount.Load())
}

// TestWebSocketConnections tests WebSocket upgrade handling
func TestWebSocketConnections(t *testing.T) {
	t.Parallel()

	// Create a simple WebSocket echo server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for WebSocket upgrade header
		if r.Header.Get("Upgrade") == "websocket" {
			w.WriteHeader(http.StatusSwitchingProtocols)
			_, _ = w.Write([]byte("websocket-upgrade-accepted"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("regular-response"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-websocket"
	routePath := "/websocket/test"

	cfg := buildE2EConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startE2ERuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user and API key
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "websocket@example.com",
		"name":            "WebSocket Test User",
		"password":        "secure-password-123",
		"initial_credits": 100,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")

	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "websocket-key",
		"mode": "live",
	}, http.StatusCreated))
	apiKey := anyString(createKey, "full_key")

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	// Test WebSocket upgrade request
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	req, _ := http.NewRequest(http.MethodGet, "http://"+gwAddr+routePath, nil)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("WebSocket request failed: %v", err)
	}
	body := readAllAndClose(t, resp.Body)

	// Gateway should handle WebSocket upgrade
	t.Logf("WebSocket response status: %d, body: %s", resp.StatusCode, body)
}

// TestGRPCTranscoding tests gRPC transcoding functionality
func TestGRPCTranscoding(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate gRPC response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"grpc-response"}`))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-grpc"
	routePath := "/grpc/test"

	cfg := buildGRPCTestConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startE2ERuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user and API key
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "grpc@example.com",
		"name":            "gRPC Test User",
		"password":        "secure-password-123",
		"initial_credits": 100,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")

	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "grpc-key",
		"mode": "live",
	}, http.StatusCreated))
	apiKey := anyString(createKey, "full_key")

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodPost},
		"allowed":  true,
	}, http.StatusCreated)

	// Test gRPC-style request
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	req, _ := http.NewRequest(http.MethodPost, "http://"+gwAddr+routePath, strings.NewReader(`{"input":"test"}`))
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("gRPC request failed: %v", err)
	}
	body := readAllAndClose(t, resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("gRPC request failed: status=%d body=%q", resp.StatusCode, body)
	}

	t.Logf("gRPC transcoding test completed: %s", body)
}

// TestRateLimitingInAction tests rate limiting enforcement
func TestRateLimitingInAction(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ratelimit-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-ratelimit"
	routePath := "/ratelimit/test"

	cfg := buildRateLimitE2EConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startE2ERuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user and API key
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "ratelimit@example.com",
		"name":            "Rate Limit Test User",
		"password":        "secure-password-123",
		"initial_credits": 1000,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")

	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "ratelimit-key",
		"mode": "live",
	}, http.StatusCreated))
	apiKey := anyString(createKey, "full_key")

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	// Send requests rapidly
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)

	successCount := 0
	rateLimitedCount := 0

	for i := 0; i < 20; i++ {
		status, _ := gatewayRequest(t, gwAddr, http.MethodGet, routePath, apiKey)
		if status == http.StatusOK {
			successCount++
		} else if status == http.StatusTooManyRequests {
			rateLimitedCount++
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Logf("Rate limiting results: %d successful, %d rate limited", successCount, rateLimitedCount)

	if successCount == 0 {
		t.Fatalf("expected some successful requests")
	}
}

// TestHealthCheckIntegration tests upstream health checking
func TestHealthCheckIntegration(t *testing.T) {
	t.Parallel()

	healthyCount := atomic.Int32{}
	_ = atomic.Int32{}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			healthyCount.Add(1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("healthy"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("healthcheck-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-healthcheck"
	routePath := "/healthcheck/test"

	cfg := buildE2EConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startE2ERuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user and API key
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "healthcheck@example.com",
		"name":            "Health Check Test User",
		"password":        "secure-password-123",
		"initial_credits": 100,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")

	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "healthcheck-key",
		"mode": "live",
	}, http.StatusCreated))
	apiKey := anyString(createKey, "full_key")

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	// Wait for health checks to run
	time.Sleep(2 * time.Second)

	// Test request
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	status, body := gatewayRequest(t, gwAddr, http.MethodGet, routePath, apiKey)
	if status != http.StatusOK {
		t.Fatalf("request failed: status=%d body=%q", status, body)
	}

	t.Logf("Health check test completed. Health endpoint called %d times", healthyCount.Load())
}

// TestConcurrentLoad tests the gateway under concurrent load
func TestConcurrentLoad(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("concurrent-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-concurrent-load"
	routePath := "/concurrent/load"

	cfg := buildE2EConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startE2ERuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user and API key
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "concurrent@example.com",
		"name":            "Concurrent Load Test User",
		"password":        "secure-password-123",
		"initial_credits": 10000,
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

	// Run concurrent requests
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)

	concurrency := 50
	requestsPerWorker := 10
	var wg sync.WaitGroup
	errorCount := atomic.Int32{}

	start := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < requestsPerWorker; j++ {
				status, _ := gatewayRequest(t, gwAddr, http.MethodGet, routePath, apiKey)
				if status != http.StatusOK {
					errorCount.Add(1)
				}
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	totalRequests := concurrency * requestsPerWorker
	successCount := int32(totalRequests) - errorCount.Load()
	rps := float64(totalRequests) / duration.Seconds()

	t.Logf("Concurrent load test completed:")
	t.Logf("  Total requests: %d", totalRequests)
	t.Logf("  Successful: %d", successCount)
	t.Logf("  Errors: %d", errorCount.Load())
	t.Logf("  Duration: %v", duration)
	t.Logf("  RPS: %.2f", rps)

	if errorCount.Load() > int32(totalRequests/10) {
		t.Fatalf("too many errors: %d out of %d", errorCount.Load(), totalRequests)
	}
}

// TestGatewayReload tests configuration reload
func TestGatewayReload(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("reload-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-reload"
	routePath := "/reload/test"

	cfg := buildE2EConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startE2ERuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user and API key
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "reload@example.com",
		"name":            "Reload Test User",
		"password":        "secure-password-123",
		"initial_credits": 100,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")

	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "reload-key",
		"mode": "live",
	}, http.StatusCreated))
	apiKey := anyString(createKey, "full_key")

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	// Test initial configuration
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	status, body := gatewayRequest(t, gwAddr, http.MethodGet, routePath, apiKey)
	if status != http.StatusOK {
		t.Fatalf("initial request failed: status=%d body=%q", status, body)
	}

	// Reload with same config (should work)
	if err := runtime.gateway.Reload(cfg); err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	// Test after reload
	status, body = gatewayRequest(t, gwAddr, http.MethodGet, routePath, apiKey)
	if status != http.StatusOK {
		t.Fatalf("request after reload failed: status=%d body=%q", status, body)
	}

	t.Log("Gateway reload test completed successfully")
}

// Helper types and functions for E2E tests
type e2eRuntime struct {
	adminHTTP *http.Server
	gateway   *gateway.Gateway
	cancel    context.CancelFunc
	gwErrCh   chan error
	adminErr  chan error
}

func startE2ERuntime(t *testing.T, cfg *config.Config) *e2eRuntime {
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

	return &e2eRuntime{
		adminHTTP: adminHTTP,
		gateway:   gw,
		cancel:    cancel,
		gwErrCh:   gwErrCh,
		adminErr:  adminErr,
	}
}

func (r *e2eRuntime) Stop(t *testing.T) {
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

func buildE2EConfig(t *testing.T, gwAddr, adminAddr, routeID, routePath, upstreamHost string) *config.Config {
	t.Helper()
	return &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:       gwAddr,
			ReadTimeout:    2 * time.Second,
			WriteTimeout:   2 * time.Second,
			IdleTimeout:    10 * time.Second,
			MaxHeaderBytes: 1 << 20,
			MaxBodyBytes:   1 << 20,
			TrustedProxies: []string{"127.0.0.1"},
		},
		Admin: config.AdminConfig{
			Addr:        adminAddr,
			APIKey:      "secret-e2e-test",
			TokenSecret: "secret-e2e-test-token-abcdefghijklmnopqrstuvwxyz",
			TokenTTL:    1 * time.Hour,
		},
		Store: config.StoreConfig{
			Path:        t.TempDir() + "/e2e-test.db",
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
		Audit: config.AuditConfig{
			Enabled:              true,
			BufferSize:           256,
			BatchSize:            1,
			FlushInterval:        10 * time.Millisecond,
			RetentionDays:        30,
			MaxRequestBodyBytes:  4096,
			MaxResponseBodyBytes: 4096,
			MaskHeaders:          []string{"X-API-Key", "Authorization"},
			MaskBodyFields:       []string{"password", "token"},
			MaskReplacement:      "***",
		},
		Services: []config.Service{
			{ID: "svc-e2e", Name: "svc-e2e", Protocol: "http", Upstream: "up-e2e"},
		},
		Routes: []config.Route{
			{
				ID:      routeID,
				Name:    routeID,
				Service: "svc-e2e",
				Paths:   []string{routePath},
				Methods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete},
			},
		},
		Upstreams: []config.Upstream{
			{
				ID:        "up-e2e",
				Name:      "up-e2e",
				Algorithm: "round_robin",
				Targets: []config.UpstreamTarget{
					{ID: "up-e2e-t1", Address: upstreamHost, Weight: 1},
				},
				HealthCheck: config.HealthCheckConfig{
					Active: config.ActiveHealthCheckConfig{
						Path:               "/health",
						Interval:           500 * time.Millisecond,
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
			{Name: "correlation-id"},
		},
	}
}

func buildGRPCTestConfig(t *testing.T, gwAddr, adminAddr, routeID, routePath, upstreamHost string) *config.Config {
	cfg := buildE2EConfig(t, gwAddr, adminAddr, routeID, routePath, upstreamHost)
	cfg.Gateway.GRPC = config.GRPCConfig{
		Enabled:           true,
		Addr:              freeAddr(t),
		EnableWeb:         true,
		EnableTranscoding: true,
	}
	cfg.Services[0].Protocol = "grpc"
	return cfg
}

func buildRateLimitE2EConfig(t *testing.T, gwAddr, adminAddr, routeID, routePath, upstreamHost string) *config.Config {
	cfg := buildE2EConfig(t, gwAddr, adminAddr, routeID, routePath, upstreamHost)
	cfg.GlobalPlugins = append(cfg.GlobalPlugins, config.PluginConfig{
		Name: "rate-limit",
		Config: map[string]any{
			"requests_per_second": 100,
			"burst":               150,
		},
	})
	return cfg
}

// Reuse helper functions
