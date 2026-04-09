package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
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

var adminTokenCache sync.Map // key: adminAddr|adminKey, value: token string

func getAdminBearerToken(t *testing.T, adminAddr, adminKey string) string {
	t.Helper()
	cacheKey := adminAddr + "|" + adminKey
	if v, ok := adminTokenCache.Load(cacheKey); ok {
		return v.(string)
	}
	req, err := http.NewRequest(http.MethodPost, "http://"+adminAddr+"/admin/api/v1/auth/token", nil)
	if err != nil {
		t.Fatalf("create token request: %v", err)
	}
	req.Header.Set("X-Admin-Key", adminKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get admin token: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("get admin token status=%d body=%s", resp.StatusCode, string(body))
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode admin token: %v", err)
	}
	adminTokenCache.Store(cacheKey, result.Token)
	return result.Token
}

func waitForTCPReady(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("tcp not ready: %s", addr)
}

func TestE2EUserCreateKeyRequestDeductAndTransactionLog(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		_, _ = w.Write([]byte("v005-credit-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-v005-credit"
	routePath := "/v005/credit"
	cfg := v005Config(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL), config.BillingConfig{
		Enabled:           true,
		DefaultCost:       5,
		ZeroBalanceAction: "reject",
		TestModeEnabled:   true,
	})

	runtime := startV005Runtime(t, cfg)
	defer runtime.Stop(t)

	createUser := asObject(t, adminJSONRequest(t, adminAddr, "secret-v005", http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "v005-credit@example.com",
		"name":            "V005 Credit",
		"password":        "pass-123",
		"initial_credits": 20,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")
	if userID == "" {
		t.Fatalf("expected created user id")
	}

	createKey := asObject(t, adminJSONRequest(t, adminAddr, "secret-v005", http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "credit-live",
		"mode": "live",
	}, http.StatusCreated))
	liveKey := anyString(createKey, "full_key")
	if liveKey == "" {
		t.Fatalf("expected full_key from api key create response")
	}

	_ = adminJSONRequest(t, adminAddr, "secret-v005", http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	status, body := gatewayRequest(t, gwAddr, http.MethodGet, routePath, liveKey)
	if status != http.StatusOK || body != "v005-credit-ok" {
		t.Fatalf("unexpected gateway response status=%d body=%q", status, body)
	}

	balanceResp := asObject(t, adminJSONRequest(t, adminAddr, "secret-v005", http.MethodGet, "/admin/api/v1/users/"+userID+"/credits/balance", nil, http.StatusOK))
	if balance, ok := anyInt64(balanceResp["balance"]); !ok || balance != 15 {
		t.Fatalf("expected credit balance 15 after request, got %#v", balanceResp["balance"])
	}

	txnResp := asObject(t, adminJSONRequest(t, adminAddr, "secret-v005", http.MethodGet, "/admin/api/v1/users/"+userID+"/credits/transactions", nil, http.StatusOK))
	txns, ok := txnResp["Transactions"].([]any)
	if !ok || len(txns) == 0 {
		t.Fatalf("expected at least one credit transaction, got %#v", txnResp)
	}
	firstTxn, ok := txns[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected transaction payload: %#v", txns[0])
	}
	if anyString(firstTxn, "Type", "type") != "consume" {
		t.Fatalf("expected first transaction type consume, got %#v", firstTxn["Type"])
	}
	if amount, ok := anyInt64(firstTxn["Amount"]); !ok || amount != -5 {
		t.Fatalf("expected first transaction amount -5, got %#v", firstTxn["Amount"])
	}
	if anyString(firstTxn, "RouteID", "route_id") != routeID {
		t.Fatalf("expected transaction route id %q, got %#v", routeID, firstTxn["RouteID"])
	}
}

func TestE2EPermissionDeniedReturns403Reason(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		_, _ = w.Write([]byte("should-not-pass"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-v005-perm"
	routePath := "/v005/permission"
	cfg := v005Config(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL), config.BillingConfig{
		Enabled:           true,
		DefaultCost:       1,
		ZeroBalanceAction: "reject",
		TestModeEnabled:   true,
	})

	runtime := startV005Runtime(t, cfg)
	defer runtime.Stop(t)

	createUser := asObject(t, adminJSONRequest(t, adminAddr, "secret-v005", http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "v005-perm@example.com",
		"name":            "V005 Permission",
		"password":        "pass-123",
		"initial_credits": 50,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")
	if userID == "" {
		t.Fatalf("expected created user id")
	}

	createKey := asObject(t, adminJSONRequest(t, adminAddr, "secret-v005", http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "perm-live",
		"mode": "live",
	}, http.StatusCreated))
	liveKey := anyString(createKey, "full_key")
	if liveKey == "" {
		t.Fatalf("expected full_key from api key create response")
	}

	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	status, body := gatewayRequest(t, gwAddr, http.MethodGet, routePath, liveKey)
	if status != http.StatusForbidden {
		t.Fatalf("expected 403 for permission denied, got %d body=%q", status, body)
	}
	if code := gatewayErrorCode(t, body); code != "permission_denied" {
		t.Fatalf("expected permission_denied error code, got %q body=%q", code, body)
	}
}

func TestE2EZeroBalanceRejectedWith402(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		_, _ = w.Write([]byte("should-not-pass-zero-balance"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-v005-zero"
	routePath := "/v005/zero"
	cfg := v005Config(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL), config.BillingConfig{
		Enabled:           true,
		DefaultCost:       5,
		ZeroBalanceAction: "reject",
		TestModeEnabled:   true,
	})

	runtime := startV005Runtime(t, cfg)
	defer runtime.Stop(t)

	createUser := asObject(t, adminJSONRequest(t, adminAddr, "secret-v005", http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "v005-zero@example.com",
		"name":            "V005 Zero",
		"password":        "pass-123",
		"initial_credits": 0,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")
	if userID == "" {
		t.Fatalf("expected created user id")
	}

	createKey := asObject(t, adminJSONRequest(t, adminAddr, "secret-v005", http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "zero-live",
		"mode": "live",
	}, http.StatusCreated))
	liveKey := anyString(createKey, "full_key")
	if liveKey == "" {
		t.Fatalf("expected full_key from api key create response")
	}

	_ = adminJSONRequest(t, adminAddr, "secret-v005", http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	status, body := gatewayRequest(t, gwAddr, http.MethodGet, routePath, liveKey)
	if status != http.StatusPaymentRequired {
		t.Fatalf("expected 402 for zero balance, got %d body=%q", status, body)
	}
	if code := gatewayErrorCode(t, body); code != "insufficient_credits" {
		t.Fatalf("expected insufficient_credits error code, got %q body=%q", code, body)
	}
}

func TestE2ETestKeySkipsCreditDeduction(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		_, _ = w.Write([]byte("v005-test-key-ok"))
	}))
	defer upstream.Close()

	gwAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	routeID := "route-v005-test-key"
	routePath := "/v005/test-key"
	cfg := v005Config(t, gwAddr, adminAddr, routeID, routePath, mustHost(t, upstream.URL), config.BillingConfig{
		Enabled:           true,
		DefaultCost:       5,
		ZeroBalanceAction: "reject",
		TestModeEnabled:   true,
	})

	runtime := startV005Runtime(t, cfg)
	defer runtime.Stop(t)

	createUser := asObject(t, adminJSONRequest(t, adminAddr, "secret-v005", http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "v005-test-key@example.com",
		"name":            "V005 Test Key",
		"password":        "pass-123",
		"initial_credits": 5,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")
	if userID == "" {
		t.Fatalf("expected created user id")
	}

	createKey := asObject(t, adminJSONRequest(t, adminAddr, "secret-v005", http.MethodPost, "/admin/api/v1/users/"+userID+"/api-keys", map[string]any{
		"name": "test-mode-key",
		"mode": "test",
	}, http.StatusCreated))
	testKey := anyString(createKey, "full_key")
	if testKey == "" {
		t.Fatalf("expected full_key from api key create response")
	}
	if !strings.HasPrefix(testKey, "ck_test_") {
		t.Fatalf("expected ck_test_ key prefix, got %q", testKey)
	}

	_ = adminJSONRequest(t, adminAddr, "secret-v005", http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet},
		"allowed":  true,
	}, http.StatusCreated)

	waitForHTTPReady(t, "http://"+gwAddr+routePath, nil)
	status, body := gatewayRequest(t, gwAddr, http.MethodGet, routePath, testKey)
	if status != http.StatusOK || body != "v005-test-key-ok" {
		t.Fatalf("unexpected gateway response status=%d body=%q", status, body)
	}

	balanceResp := asObject(t, adminJSONRequest(t, adminAddr, "secret-v005", http.MethodGet, "/admin/api/v1/users/"+userID+"/credits/balance", nil, http.StatusOK))
	if balance, ok := anyInt64(balanceResp["balance"]); !ok || balance != 5 {
		t.Fatalf("expected unchanged credit balance 5 for test key, got %#v", balanceResp["balance"])
	}

	txnResp := asObject(t, adminJSONRequest(t, adminAddr, "secret-v005", http.MethodGet, "/admin/api/v1/users/"+userID+"/credits/transactions", nil, http.StatusOK))
	txns, ok := txnResp["Transactions"].([]any)
	if !ok {
		t.Fatalf("unexpected transactions payload: %#v", txnResp)
	}
	if len(txns) != 0 {
		t.Fatalf("expected no credit transactions for test key request, got %d", len(txns))
	}
}

type v005Runtime struct {
	adminHTTP *http.Server
	cancel    context.CancelFunc
	gwErrCh   chan error
	adminErr  chan error
}

func startV005Runtime(t *testing.T, cfg *config.Config) *v005Runtime {
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

	return &v005Runtime{
		adminHTTP: adminHTTP,
		cancel:    cancel,
		gwErrCh:   gwErrCh,
		adminErr:  adminErr,
	}
}

func (r *v005Runtime) Stop(t *testing.T) {
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

func v005Config(t *testing.T, gwAddr, adminAddr, routeID, routePath, upstreamHost string, billingCfg config.BillingConfig) *config.Config {
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
			APIKey:      "secret-v005",
			TokenSecret: "secret-v005-token-abcdefghijklmnopqrstuvwxyz",
			TokenTTL:    1 * time.Hour,
		},
		Store: config.StoreConfig{
			Path:        t.TempDir() + "/e2e-v005.db",
			BusyTimeout: time.Second,
			JournalMode: "WAL",
			ForeignKeys: true,
		},
		Billing: billingCfg,
		Services: []config.Service{
			{ID: "svc-v005", Name: "svc-v005", Protocol: "http", Upstream: "up-v005"},
		},
		Routes: []config.Route{
			{
				ID:      routeID,
				Name:    routeID,
				Service: "svc-v005",
				Paths:   []string{routePath},
				Methods: []string{http.MethodGet},
			},
		},
		Upstreams: []config.Upstream{
			{
				ID:        "up-v005",
				Name:      "up-v005",
				Algorithm: "round_robin",
				Targets: []config.UpstreamTarget{
					{ID: "up-v005-t1", Address: upstreamHost, Weight: 1},
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
		},
	}
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
	if adminKey != "" {
		token := getAdminBearerToken(t, adminAddr, adminKey)
		req.Header.Set("Authorization", "Bearer "+token)
	}
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
				if rendered := strings.TrimSpace(fmt.Sprint(v)); rendered != "" && rendered != "<nil>" {
					return rendered
				}
			}
		}
	}
	return ""
}

func anyInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		return int64(v), true
	case float32:
		return int64(v), true
	case json.Number:
		parsed, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}
