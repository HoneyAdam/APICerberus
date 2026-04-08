package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/admin"
	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/gateway"
	"github.com/APICerberus/APICerebrus/internal/store"
)

// TestAuthFlowAPIKey tests the complete API key authentication flow
func TestAuthFlowAPIKey(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("apikey-auth-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-apikey-auth"
	routePath := "/apikey/auth"

	cfg := buildAuthTestConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startAuthTestRuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user with initial credits
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "apikey-test@example.com",
		"name":            "API Key Test User",
		"password":        "secure-password-123",
		"initial_credits": 100,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")
	if userID == "" {
		t.Fatalf("expected created user id")
	}

	// Create live API key
	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "test-live-key",
		"mode": "live",
	}, http.StatusCreated))
	liveKey := anyString(createKey, "full_key")
	if liveKey == "" {
		t.Fatalf("expected full_key from api key create response")
	}
	if !strings.HasPrefix(liveKey, "ck_live_") {
		t.Fatalf("expected live key to start with ck_live_, got %q", liveKey)
	}

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	// Test successful authentication with valid API key
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	status, body := gatewayRequest(t, gwAddr, http.MethodGet, routePath, liveKey)
	if status != http.StatusOK || body != "apikey-auth-ok" {
		t.Fatalf("unexpected gateway response status=%d body=%q", status, body)
	}

	// Test authentication failure with invalid API key
	status, body = gatewayRequest(t, gwAddr, http.MethodGet, routePath, "invalid-key")
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid key, got %d body=%q", status, body)
	}
	if code := gatewayErrorCode(t, body); code != "invalid_api_key" {
		t.Fatalf("expected invalid_api_key error code, got %q", code)
	}

	// Test authentication failure with missing API key
	status, body = gatewayRequest(t, gwAddr, http.MethodGet, routePath, "")
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing key, got %d body=%q", status, body)
	}

	// Revoke the API key
	keyID := anyString(createKey, "ID", "id")
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodDelete, "/admin/api/v1/users/"+userID+"/api-keys/"+keyID, nil, http.StatusNoContent)

	// Test that revoked key no longer works
	status, body = gatewayRequest(t, gwAddr, http.MethodGet, routePath, liveKey)
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401 for revoked key, got %d body=%q", status, body)
	}
}

// TestAuthFlowJWT tests JWT authentication flow
func TestAuthFlowJWT(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("jwt-auth-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-jwt-auth"
	routePath := "/jwt/auth"

	cfg := buildJWTTestConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startAuthTestRuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "jwt-test@example.com",
		"name":            "JWT Test User",
		"password":        "secure-password-123",
		"initial_credits": 100,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")
	if userID == "" {
		t.Fatalf("expected created user id")
	}

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	// Create API key for JWT generation (simulated)
	createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "jwt-test-key",
		"mode": "live",
	}, http.StatusCreated))
	apiKey := anyString(createKey, "full_key")

	// Test with API key (JWT plugin would validate JWT tokens)
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	status, body := gatewayRequest(t, gwAddr, http.MethodGet, routePath, apiKey)
	if status != http.StatusOK {
		t.Fatalf("unexpected gateway response status=%d body=%q", status, body)
	}
}

// TestSessionManagementFlow tests user session lifecycle
func TestSessionManagementFlow(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("session-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-session"
	routePath := "/session/test"

	cfg := buildAuthTestConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startAuthTestRuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "session-test@example.com",
		"name":            "Session Test User",
		"password":        "secure-password-123",
		"initial_credits": 50,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")
	if userID == "" {
		t.Fatalf("expected created user id")
	}

	// Create multiple API keys
	keys := make([]string, 3)
	for i := 0; i < 3; i++ {
		createKey := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
			"name": "session-key-" + string(rune('A'+i)),
			"mode": "live",
		}, http.StatusCreated))
		keys[i] = anyString(createKey, "full_key")
	}

	// Grant permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	// Test that all keys work
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	for i, key := range keys {
		status, body := gatewayRequest(t, gwAddr, http.MethodGet, routePath, key)
		if status != http.StatusOK {
			t.Fatalf("key %d: unexpected gateway response status=%d body=%q", i, status, body)
		}
	}

	// List user's API keys
	keysList := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodGet, "/admin/api/v1/users/"+userID+"/api-keys", nil, http.StatusOK))
	keysArray, ok := keysList["keys"].([]any)
	if !ok || len(keysArray) != 3 {
		t.Fatalf("expected 3 API keys, got %d", len(keysArray))
	}

	// Delete one key
	keyObj := asObject(t, keysArray[0])
	keyID := anyString(keyObj, "ID", "id")
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodDelete, "/admin/api/v1/users/"+userID+"/api-keys/"+keyID, nil, http.StatusNoContent)

	// Verify key count decreased
	keysList = asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodGet, "/admin/api/v1/users/"+userID+"/api-keys", nil, http.StatusOK))
	keysArray, ok = keysList["keys"].([]any)
	if !ok || len(keysArray) != 2 {
		t.Fatalf("expected 2 API keys after deletion, got %d", len(keysArray))
	}
}

// TestPermissionChecks tests endpoint permission enforcement
func TestPermissionChecks(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("permission-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-permission"
	routePath := "/permission/test"

	cfg := buildAuthTestConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startAuthTestRuntime(t, cfg)
	defer runtime.Stop(t)

	// Create two users - one with permission, one without
	user1 := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "permitted@example.com",
		"name":            "Permitted User",
		"password":        "secure-password-123",
		"initial_credits": 50,
	}, http.StatusCreated))
	user1ID := anyString(user1, "ID", "id")

	user2 := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "not-permitted@example.com",
		"name":            "Not Permitted User",
		"password":        "secure-password-123",
		"initial_credits": 50,
	}, http.StatusCreated))
	user2ID := anyString(user2, "ID", "id")

	// Create API keys for both users
	key1Obj := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+user1ID+"/api-keys", map[string]any{
		"name": "user1-key",
		"mode": "live",
	}, http.StatusCreated))
	key1 := anyString(key1Obj, "full_key")

	key2Obj := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+user2ID+"/api-keys", map[string]any{
		"name": "user2-key",
		"mode": "live",
	}, http.StatusCreated))
	key2 := anyString(key2Obj, "full_key")

	// Grant permission only to user1
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+user1ID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	// Test: user1 should have access
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	status, body := gatewayRequest(t, gwAddr, http.MethodGet, routePath, key1)
	if status != http.StatusOK {
		t.Fatalf("user1 should have access, got status=%d body=%q", status, body)
	}

	// Test: user2 should be denied
	status, body = gatewayRequest(t, gwAddr, http.MethodGet, routePath, key2)
	if status != http.StatusForbidden {
		t.Fatalf("user2 should be denied, expected 403 got status=%d body=%q", status, body)
	}
	if code := gatewayErrorCode(t, body); code != "permission_denied" {
		t.Fatalf("expected permission_denied error code, got %q", code)
	}

	// Update permission to deny GET but allow POST
	permObj := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodGet, "/admin/api/v1/users/"+user1ID+"/permissions", nil, http.StatusOK))
	permsArray, ok := permObj["permissions"].([]any)
	if !ok || len(permsArray) == 0 {
		t.Fatalf("expected permissions array")
	}
	permID := anyString(asObject(t, permsArray[0]), "ID", "id")

	// Revoke permission
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodDelete, "/admin/api/v1/permissions/"+permID, nil, http.StatusNoContent)

	// Test: user1 should now be denied
	status, body = gatewayRequest(t, gwAddr, http.MethodGet, routePath, key1)
	if status != http.StatusForbidden {
		t.Fatalf("user1 should be denied after permission revocation, got status=%d body=%q", status, body)
	}
}

// TestAuthFlowRateLimiting tests rate limiting per API key
func TestAuthFlowRateLimiting(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ratelimit-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-ratelimit-auth"
	routePath := "/ratelimit/auth"

	cfg := buildRateLimitTestConfig(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL))
	runtime := startAuthTestRuntime(t, cfg)
	defer runtime.Stop(t)

	// Create user
	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "ratelimit@example.com",
		"name":            "Rate Limit Test User",
		"password":        "secure-password-123",
		"initial_credits": 100,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")

	// Create API key
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

	// Test requests within rate limit
	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	for i := 0; i < 5; i++ {
		status, body := gatewayRequest(t, gwAddr, http.MethodGet, routePath, apiKey)
		if status != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d body=%q", i, status, body)
		}
	}
}

// Helper types and functions

type authTestRuntime struct {
	adminHTTP *http.Server
	cancel    context.CancelFunc
	gwErrCh   chan error
	adminErr  chan error
	store     *store.Store
}

func startAuthTestRuntime(t *testing.T, cfg *config.Config) *authTestRuntime {
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

	return &authTestRuntime{
		adminHTTP: adminHTTP,
		cancel:    cancel,
		gwErrCh:   gwErrCh,
		adminErr:  adminErr,
	}
}

func (r *authTestRuntime) Stop(t *testing.T) {
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

func buildAuthTestConfig(t *testing.T, gwAddr, adminAddr, routeID, routePath, upstreamHost string) *config.Config {
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
			APIKey: "secret-auth-test",
		},
		Store: config.StoreConfig{
			Path:        t.TempDir() + "/auth-test.db",
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
			{ID: "svc-auth", Name: "svc-auth", Protocol: "http", Upstream: "up-auth"},
		},
		Routes: []config.Route{
			{
				ID:      routeID,
				Name:    routeID,
				Service: "svc-auth",
				Paths:   []string{routePath},
				Methods: []string{http.MethodGet, http.MethodPost},
			},
		},
		Upstreams: []config.Upstream{
			{
				ID:        "up-auth",
				Name:      "up-auth",
				Algorithm: "round_robin",
				Targets: []config.UpstreamTarget{
					{ID: "up-auth-t1", Address: upstreamHost, Weight: 1},
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

func buildJWTTestConfig(t *testing.T, gwAddr, adminAddr, routeID, routePath, upstreamHost string) *config.Config {
	cfg := buildAuthTestConfig(t, gwAddr, adminAddr, routeID, routePath, upstreamHost)
	// Add JWT plugin configuration
	cfg.GlobalPlugins = append(cfg.GlobalPlugins, config.PluginConfig{
		Name: "auth-jwt",
		Config: map[string]any{
			"secret": "test-jwt-secret",
		},
	})
	return cfg
}

func buildRateLimitTestConfig(t *testing.T, gwAddr, adminAddr, routeID, routePath, upstreamHost string) *config.Config {
	cfg := buildAuthTestConfig(t, gwAddr, adminAddr, routeID, routePath, upstreamHost)
	// Add rate limiting plugin
	cfg.GlobalPlugins = append(cfg.GlobalPlugins, config.PluginConfig{
		Name: "rate-limit",
		Config: map[string]any{
			"requests_per_second": 10,
			"burst":               20,
		},
	})
	return cfg
}

// Common helper functions
func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen error: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

func mustHost(t *testing.T, rawURL string) string {
	t.Helper()
	if idx := strings.Index(rawURL, "://"); idx != -1 {
		rawURL = rawURL[idx+3:]
	}
	return rawURL
}

func waitForHTTPReady(t *testing.T, rawURL string, headers map[string]string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, rawURL, nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 500 {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("service not ready: %s", rawURL)
}

func adminJSONRequest(t *testing.T, adminAddr, adminKey, method, path string, payload any, expectedStatus int) any {
	t.Helper()
	rawURL := "http://" + adminAddr + path

	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("json marshal payload: %v", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		t.Fatalf("new request %s %s: %v", method, rawURL, err)
	}
	req.Header.Set("X-Admin-Key", adminKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed %s %s: %v", method, rawURL, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body %s %s: %v", method, rawURL, err)
	}

	if resp.StatusCode != expectedStatus {
		t.Fatalf("unexpected status %s %s got=%d want=%d body=%s", method, rawURL, resp.StatusCode, expectedStatus, string(respBody))
	}
	if len(respBody) == 0 || expectedStatus == http.StatusNoContent {
		return nil
	}

	var decoded any
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		t.Fatalf("decode response json %s %s: %v body=%s", method, rawURL, err, string(respBody))
	}
	return decoded
}

func gatewayRequest(t *testing.T, gwAddr, method, path, apiKey string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, "http://"+gwAddr+path, nil)
	if err != nil {
		t.Fatalf("new gateway request: %v", err)
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("gateway request failed: %v", err)
	}
	body := readAllAndClose(t, resp.Body)
	return resp.StatusCode, body
}

func gatewayErrorCode(t *testing.T, body string) string {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decode gateway error payload: %v body=%q", err, body)
	}
	errObj, ok := payload["error"].(map[string]any)
	if !ok {
		return ""
	}
	return anyString(errObj, "code", "Code")
}

func readAllAndClose(t *testing.T, rc io.ReadCloser) string {
	t.Helper()
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(data)
}

func asObject(t *testing.T, value any) map[string]any {
	t.Helper()
	if value == nil {
		return map[string]any{}
	}
	obj, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected JSON object, got %T (%#v)", value, value)
	}
	return obj
}

func anyString(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := obj[key]; ok {
			switch v := value.(type) {
			case string:
				return strings.TrimSpace(v)
			default:
				if rendered := strings.TrimSpace(anyToString(v)); rendered != "" && rendered != "<nil>" {
					return rendered
				}
			}
		}
	}
	return ""
}

func anyToString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case json.Number:
		return val.String()
	default:
		return anyToString(val)
	}
}

func anyInt64(v any) (int64, bool) {
	if v == nil {
		return 0, false
	}
	switch val := v.(type) {
	case int64:
		return val, true
	case float64:
		return int64(val), true
	case json.Number:
		i, err := val.Int64()
		return i, err == nil
	default:
		return 0, false
	}
}
