package test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/admin"
	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/gateway"
	"github.com/APICerberus/APICerebrus/internal/portal"
	"github.com/APICerberus/APICerebrus/internal/store"
)

func TestE2EPortalLoginKeyPlaygroundLogsCreditsFlow(t *testing.T) {
	t.Parallel()

	upstreamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"source":"v008"}`))
	}))
	defer upstreamSrv.Close()

	gatewayAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	portalAddr := freeAddr(t)
	routeID := "route-v008-portal"
	routePath := "/v008/portal"

	cfg := v008Config(t, gatewayAddr, adminAddr, portalAddr, routeID, routePath, mustHost(t, upstreamSrv.URL))
	runtime := startV008Runtime(t, cfg)
	defer runtime.Stop(t)

	createUser := asObject(t, adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users", map[string]any{
		"email":           "v008-portal@example.com",
		"name":            "V008 Portal",
		"password":        "pass-123",
		"initial_credits": 50,
	}, http.StatusCreated))
	userID := anyString(createUser, "ID", "id")
	if userID == "" {
		t.Fatalf("expected created user id")
	}
	_ = adminJSONRequest(t, adminAddr, cfg.Admin.APIKey, http.MethodPost, "/admin/api/v1/users/"+userID+"/permissions", map[string]any{
		"route_id": routeID,
		"methods":  []string{http.MethodGet, http.MethodPost},
		"allowed":  true,
	}, http.StatusCreated)

	client := newPortalHTTPClient(t)
	_ = portalJSONRequest(t, client, portalAddr, http.MethodPost, "/portal/api/v1/auth/login", map[string]any{
		"email":    "v008-portal@example.com",
		"password": "pass-123",
	}, http.StatusOK)

	createKey := asObject(t, portalJSONRequest(t, client, portalAddr, http.MethodPost, "/portal/api/v1/api-keys", map[string]any{
		"name": "v008-live",
		"mode": "live",
	}, http.StatusCreated))
	liveKey := anyString(createKey, "token")
	if liveKey == "" {
		t.Fatalf("expected created portal api key token")
	}

	playground := asObject(t, portalJSONRequest(t, client, portalAddr, http.MethodPost, "/portal/api/v1/playground/send", map[string]any{
		"method":  http.MethodGet,
		"path":    routePath,
		"api_key": liveKey,
		"query": map[string]string{
			"via": "playground",
		},
	}, http.StatusOK))
	playgroundResponse := asObject(t, playground["response"])
	statusCode, ok := anyInt64(playgroundResponse["status_code"])
	if !ok || statusCode != http.StatusOK {
		t.Fatalf("expected playground status_code=200, got %#v", playgroundResponse["status_code"])
	}

	logs := asObject(t, portalJSONRequest(t, client, portalAddr, http.MethodGet, "/portal/api/v1/logs?limit=20", nil, http.StatusOK))
	if _, ok := logs["entries"].([]any); !ok {
		t.Fatalf("expected logs.entries array, got %#v", logs["entries"])
	}

	balance := asObject(t, portalJSONRequest(t, client, portalAddr, http.MethodGet, "/portal/api/v1/credits/balance", nil, http.StatusOK))
	if _, ok := anyInt64(balance["balance"]); !ok {
		t.Fatalf("expected numeric credit balance, got %#v", balance["balance"])
	}
}

type v008Runtime struct {
	adminHTTP   *http.Server
	portalHTTP  *http.Server
	portalStore *store.Store
	cancel      context.CancelFunc
	gwErrCh     chan error
	adminErr    chan error
	portalErr   chan error
}

func startV008Runtime(t *testing.T, cfg *config.Config) *v008Runtime {
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
	portalStore, err := store.Open(cfg)
	if err != nil {
		t.Fatalf("store.Open error: %v", err)
	}
	portalHandler, err := portal.NewServer(cfg, portalStore)
	if err != nil {
		_ = portalStore.Close()
		t.Fatalf("portal.NewServer error: %v", err)
	}

	adminHTTP := &http.Server{
		Addr:           cfg.Admin.Addr,
		Handler:        adminHandler,
		ReadTimeout:    2 * time.Second,
		WriteTimeout:   2 * time.Second,
		IdleTimeout:    10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	portalHTTP := &http.Server{
		Addr:           cfg.Portal.Addr,
		Handler:        portalHandler,
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

	portalErr := make(chan error, 1)
	go func() {
		err := portalHTTP.ListenAndServe()
		if err == http.ErrServerClosed {
			err = nil
		}
		portalErr <- err
	}()

	waitForGatewayListener(t, cfg.Gateway.HTTPAddr)
	waitForTCPReady(t, cfg.Admin.Addr)
	adminToken := getAdminBearerToken(t, cfg.Admin.Addr, cfg.Admin.APIKey)
	waitForHTTPReady(t, "http://"+cfg.Admin.Addr+"/admin/api/v1/status", map[string]string{"Authorization": "Bearer " + adminToken})
	waitForHTTPReady(t, "http://"+cfg.Portal.Addr+"/portal", nil)

	return &v008Runtime{
		adminHTTP:   adminHTTP,
		portalHTTP:  portalHTTP,
		portalStore: portalStore,
		cancel:      cancel,
		gwErrCh:     gwErrCh,
		adminErr:    adminErr,
		portalErr:   portalErr,
	}
}

func (r *v008Runtime) Stop(t *testing.T) {
	t.Helper()
	if r == nil {
		return
	}

	r.cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	_ = r.adminHTTP.Shutdown(shutdownCtx)
	_ = r.portalHTTP.Shutdown(shutdownCtx)
	if r.portalStore != nil {
		_ = r.portalStore.Close()
	}

	if err := <-r.gwErrCh; err != nil {
		t.Fatalf("gateway runtime error: %v", err)
	}
	if err := <-r.adminErr; err != nil {
		t.Fatalf("admin runtime error: %v", err)
	}
	if err := <-r.portalErr; err != nil {
		t.Fatalf("portal runtime error: %v", err)
	}
}

func v008Config(t *testing.T, gatewayAddr, adminAddr, portalAddr, routeID, routePath, upstreamHost string) *config.Config {
	t.Helper()
	return &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:       gatewayAddr,
			ReadTimeout:    2 * time.Second,
			WriteTimeout:   2 * time.Second,
			IdleTimeout:    10 * time.Second,
			MaxHeaderBytes: 1 << 20,
			MaxBodyBytes:   1 << 20,
		},
		Admin: config.AdminConfig{
			Addr:        adminAddr,
			APIKey:      "secret-v008",
			TokenSecret: "secret-v008-token",
			TokenTTL:    1 * time.Hour,
		},
		Portal: config.PortalConfig{
			Enabled:    true,
			Addr:       portalAddr,
			PathPrefix: "/portal",
			Session: config.PortalSessionConfig{
				CookieName: "v008_portal_session",
				MaxAge:     2 * time.Hour,
				Secure:     false,
			},
		},
		Store: config.StoreConfig{
			Path:        t.TempDir() + "/e2e-v008.db",
			BusyTimeout: time.Second,
			JournalMode: "WAL",
			ForeignKeys: true,
		},
		Billing: config.BillingConfig{
			Enabled:           true,
			DefaultCost:       3,
			ZeroBalanceAction: "reject",
			TestModeEnabled:   true,
		},
		Audit: config.AuditConfig{
			Enabled:              true,
			BufferSize:           256,
			BatchSize:            1,
			FlushInterval:        10 * time.Millisecond,
			RetentionDays:        30,
			CleanupInterval:      time.Hour,
			CleanupBatchSize:     100,
			MaxRequestBodyBytes:  4096,
			MaxResponseBodyBytes: 4096,
			MaskHeaders:          []string{"X-API-Key", "Authorization"},
			MaskBodyFields:       []string{"password", "token"},
			MaskReplacement:      "***",
		},
		Services: []config.Service{
			{ID: "svc-v008", Name: "svc-v008", Protocol: "http", Upstream: "up-v008"},
		},
		Routes: []config.Route{
			{
				ID:      routeID,
				Name:    routeID,
				Service: "svc-v008",
				Paths:   []string{routePath},
				Methods: []string{http.MethodGet, http.MethodPost},
			},
		},
		Upstreams: []config.Upstream{
			{
				ID:        "up-v008",
				Name:      "up-v008",
				Algorithm: "round_robin",
				Targets: []config.UpstreamTarget{
					{ID: "up-v008-t1", Address: upstreamHost, Weight: 1},
				},
				HealthCheck: config.HealthCheckConfig{
					Active: config.ActiveHealthCheckConfig{
						Path:               "/health",
						Interval:           time.Second,
						Timeout:            time.Second,
						HealthyThreshold:   1,
						UnhealthyThreshold: 1,
					},
				},
			},
		},
	}
}

func newPortalHTTPClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New error: %v", err)
	}
	return &http.Client{
		Jar:     jar,
		Timeout: 5 * time.Second,
	}
}

func portalJSONRequest(t *testing.T, client *http.Client, portalAddr, method, path string, payload any, expectedStatus int) any {
	t.Helper()
	rawURL := "http://" + portalAddr + path

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
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed %s %s: %v", method, rawURL, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body %s %s: %v", method, rawURL, err)
	}

	if resp.StatusCode != expectedStatus {
		t.Fatalf("unexpected status for %s %s: got %d want %d body=%s", method, rawURL, resp.StatusCode, expectedStatus, string(respBody))
	}
	if len(respBody) == 0 || resp.StatusCode == http.StatusNoContent {
		return map[string]any{}
	}

	var decoded any
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		t.Fatalf("json unmarshal response %s %s: %v body=%s", method, rawURL, err, string(respBody))
	}
	return decoded
}
