package gateway

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

func TestGatewayServeHTTPRouteToUpstream(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream", "users")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok-from-upstream"))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 got %d", rr.Code)
	}
	if rr.Body.String() != "ok-from-upstream" {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
	if rr.Header().Get("X-Upstream") != "users" {
		t.Fatalf("missing upstream header")
	}
}

func TestGatewayErrorJSON(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:       "127.0.0.1:0",
			ReadTimeout:    2 * time.Second,
			WriteTimeout:   2 * time.Second,
			IdleTimeout:    10 * time.Second,
			MaxHeaderBytes: 1 << 20,
			MaxBodyBytes:   1 << 20,
		},
	}
	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/no-route", nil)
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 got %d", rr.Code)
	}

	var payload map[string]map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("error response should be valid json: %v", err)
	}
	if payload["error"]["code"] != "route_not_found" {
		t.Fatalf("unexpected error code: %#v", payload)
	}
}

func TestGatewayStartAndShutdown(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("started"))
	}))
	defer upstream.Close()

	addr := freeAddr(t)
	cfg := gatewayTestConfig(t, addr, mustHost(t, upstream.URL))
	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- gw.Start(ctx)
	}()

	waitForHTTPReady(t, "http://"+addr+"/api/users")

	resp, err := http.Get("http://" + addr + "/api/users")
	if err != nil {
		t.Fatalf("request through started gateway failed: %v", err)
	}
	body := readAllAndClose(t, resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 got %d", resp.StatusCode)
	}
	if body != "started" {
		t.Fatalf("unexpected body: %q", body)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("gateway start returned error on shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("gateway did not shutdown after context cancellation")
	}
}

func TestGatewayShutdownDrainsInFlightRequests(t *testing.T) {
	t.Parallel()

	requestStarted := make(chan struct{})
	var requestStartedOnce sync.Once
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestStartedOnce.Do(func() { close(requestStarted) })
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("completed"))
	}))
	defer upstream.Close()

	addr := freeAddr(t)
	cfg := gatewayTestConfig(t, addr, mustHost(t, upstream.URL))
	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startErrCh := make(chan error, 1)
	go func() {
		startErrCh <- gw.Start(ctx)
	}()

	waitForHTTPReady(t, "http://"+addr+"/api/users")

	client := &http.Client{Timeout: 10 * time.Second}

	// Send an in-flight request asynchronously
	responseCh := make(chan *http.Response, 1)
	requestErrCh := make(chan error, 1)
	go func() {
		resp, err := client.Get("http://" + addr + "/api/users")
		if err != nil {
			requestErrCh <- err
			return
		}
		responseCh <- resp
	}()

	<-requestStarted
	// Give the client time to establish the TCP connection before shutting down.
	time.Sleep(50 * time.Millisecond)

	// Initiate graceful shutdown with a 10s timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := gw.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	select {
	case resp := <-responseCh:
		defer resp.Body.Close()
		body := readAllAndClose(t, resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 for in-flight request, got %d body=%q", resp.StatusCode, body)
		}
		if body != "completed" {
			t.Fatalf("unexpected body: %q", body)
		}
	case err := <-requestErrCh:
		t.Fatalf("in-flight request failed during shutdown: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatalf("in-flight request did not complete during graceful shutdown")
	}

	// Cancel the start context to stop the gateway goroutine
	cancel()
	select {
	case <-startErrCh:
	case <-time.After(2 * time.Second):
	}

	// Verify the listener is closed
	resp, err := http.Get("http://" + addr + "/api/users")
	if err == nil {
		_ = resp.Body.Close()
		t.Fatalf("expected listener to be closed after shutdown")
	}
	if !strings.Contains(err.Error(), "connect") && !strings.Contains(err.Error(), "refused") {
		t.Fatalf("expected connection error after shutdown, got: %v", err)
	}
}

func TestGatewayReload(t *testing.T) {
	t.Parallel()

	upstreamA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("a"))
	}))
	defer upstreamA.Close()

	upstreamB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("b"))
	}))
	defer upstreamB.Close()

	cfgA := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstreamA.URL))
	gw, err := New(cfgA)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	reqA := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	rrA := httptest.NewRecorder()
	gw.ServeHTTP(rrA, reqA)
	if rrA.Body.String() != "a" {
		t.Fatalf("expected first config to route to upstream a, got %q", rrA.Body.String())
	}

	cfgB := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstreamB.URL))
	cfgB.Routes[0].Paths = []string{"/api/v2/users"}
	if err := gw.Reload(cfgB); err != nil {
		t.Fatalf("Reload error: %v", err)
	}

	reqOld := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	rrOld := httptest.NewRecorder()
	gw.ServeHTTP(rrOld, reqOld)
	if rrOld.Code != http.StatusNotFound {
		t.Fatalf("expected old route to disappear after reload, got %d", rrOld.Code)
	}

	reqNew := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/v2/users", nil)
	rrNew := httptest.NewRecorder()
	gw.ServeHTTP(rrNew, reqNew)
	if rrNew.Body.String() != "b" {
		t.Fatalf("expected new config to route to upstream b, got %q", rrNew.Body.String())
	}
}

func TestGatewayAuthMissingAPIKey(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.Consumers = []config.Consumer{
		{
			ID:   "c1",
			Name: "mobile-app",
			APIKeys: []config.ConsumerAPIKey{
				{ID: "k1", Key: "ck_live_abc"},
			},
		},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 got %d", rr.Code)
	}

	var payload map[string]map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("error response should be valid json: %v", err)
	}
	if payload["error"]["code"] != "missing_api_key" {
		t.Fatalf("unexpected error payload: %#v", payload)
	}
}

func TestGatewayAuthInvalidAndExpiredAPIKey(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.Consumers = []config.Consumer{
		{
			ID:   "c1",
			Name: "mobile-app",
			APIKeys: []config.ConsumerAPIKey{
				{ID: "k1", Key: "ck_live_valid"},
				{ID: "k2", Key: "ck_live_expired", ExpiresAt: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)},
			},
		},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	reqInvalid := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	reqInvalid.Header.Set("X-API-Key", "wrong")
	reqInvalid.RemoteAddr = "198.51.100.1:1234"
	rrInvalid := httptest.NewRecorder()
	gw.ServeHTTP(rrInvalid, reqInvalid)
	if rrInvalid.Code != http.StatusUnauthorized {
		t.Fatalf("expected invalid key 401 got %d", rrInvalid.Code)
	}

	var invalidPayload map[string]map[string]string
	if err := json.Unmarshal(rrInvalid.Body.Bytes(), &invalidPayload); err != nil {
		t.Fatalf("invalid key response should be json: %v", err)
	}
	if invalidPayload["error"]["code"] != "invalid_api_key" {
		t.Fatalf("unexpected invalid key payload: %#v", invalidPayload)
	}

	reqExpired := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	reqExpired.Header.Set("X-API-Key", "ck_live_expired")
	reqExpired.RemoteAddr = "198.51.100.2:1234"
	rrExpired := httptest.NewRecorder()
	gw.ServeHTTP(rrExpired, reqExpired)
	if rrExpired.Code != http.StatusUnauthorized {
		t.Fatalf("expected expired key 401 got %d", rrExpired.Code)
	}

	var expiredPayload map[string]map[string]string
	if err := json.Unmarshal(rrExpired.Body.Bytes(), &expiredPayload); err != nil {
		t.Fatalf("expired key response should be json: %v", err)
	}
	if expiredPayload["error"]["code"] != "expired_api_key" {
		t.Fatalf("unexpected expired key payload: %#v", expiredPayload)
	}
}

func TestGatewayAuthCustomKeyHeader(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.Consumers = []config.Consumer{
		{
			ID:   "c1",
			Name: "mobile-app",
			APIKeys: []config.ConsumerAPIKey{
				{ID: "k1", Key: "ck_live_custom"},
			},
		},
	}
	cfg.Auth.APIKey.KeyNames = []string{"X-App-Key"}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	req.Header.Set("X-App-Key", "ck_live_custom")
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body=%q", rr.Code, rr.Body.String())
	}
}

func TestGatewayAuthWithSQLiteAPIKeys(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok-from-store-auth"))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.Store = config.StoreConfig{
		Path:        t.TempDir() + "/gateway-auth.db",
		BusyTimeout: time.Second,
		JournalMode: "WAL",
		ForeignKeys: true,
	}
	cfg.GlobalPlugins = []config.PluginConfig{
		{Name: "auth-apikey"},
	}

	st, err := store.Open(cfg)
	if err != nil {
		t.Fatalf("open store error: %v", err)
	}

	passwordHash, err := store.HashPassword("pw")
	if err != nil {
		_ = st.Close()
		t.Fatalf("hash password error: %v", err)
	}
	user := &store.User{
		Email:        "gateway-auth@example.com",
		Name:         "Gateway Auth User",
		PasswordHash: passwordHash,
		Role:         "user",
		Status:       "active",
	}
	if err := st.Users().Create(user); err != nil {
		_ = st.Close()
		t.Fatalf("create user error: %v", err)
	}
	fullKey, _, err := st.APIKeys().Create(user.ID, "gateway", "live")
	if err != nil {
		_ = st.Close()
		t.Fatalf("create api key error: %v", err)
	}
	if err := st.Permissions().Create(&store.EndpointPermission{
		UserID:  user.ID,
		RouteID: "route-1",
		Methods: []string{http.MethodGet},
		Allowed: true,
	}); err != nil {
		_ = st.Close()
		t.Fatalf("create permission error: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close seed store error: %v", err)
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}
	t.Cleanup(func() {
		if gw.store != nil {
			_ = gw.store.Close()
		}
	})

	okReq := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	okReq.Header.Set("X-API-Key", fullKey)
	okResp := httptest.NewRecorder()
	gw.ServeHTTP(okResp, okReq)
	if okResp.Code != http.StatusOK || okResp.Body.String() != "ok-from-store-auth" {
		t.Fatalf("expected sqlite auth success, got status=%d body=%q", okResp.Code, okResp.Body.String())
	}

	missingReq := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	missingResp := httptest.NewRecorder()
	gw.ServeHTTP(missingResp, missingReq)
	if missingResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing key, got %d", missingResp.Code)
	}
	var payload map[string]map[string]string
	if err := json.Unmarshal(missingResp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("missing key response should be json: %v", err)
	}
	if payload["error"]["code"] != "missing_api_key" {
		t.Fatalf("unexpected missing key payload: %#v", payload)
	}
}

func TestGatewayBillingRejectDeductAndTestKeyBypass(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("billing-ok"))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.Store = config.StoreConfig{
		Path:        t.TempDir() + "/gateway-billing.db",
		BusyTimeout: time.Second,
		JournalMode: "WAL",
		ForeignKeys: true,
	}
	cfg.GlobalPlugins = []config.PluginConfig{
		{Name: "auth-apikey"},
	}
	cfg.Billing = config.BillingConfig{
		Enabled:           true,
		DefaultCost:       5,
		ZeroBalanceAction: "reject",
		TestModeEnabled:   true,
	}

	seedStore, err := store.Open(cfg)
	if err != nil {
		t.Fatalf("open seed store error: %v", err)
	}
	user := createGatewayBillingUser(t, seedStore, "billing-user@example.com", 10)
	liveKey, _, err := seedStore.APIKeys().Create(user.ID, "live", "live")
	if err != nil {
		_ = seedStore.Close()
		t.Fatalf("create live key error: %v", err)
	}
	testKey, _, err := seedStore.APIKeys().Create(user.ID, "test", "test")
	if err != nil {
		_ = seedStore.Close()
		t.Fatalf("create test key error: %v", err)
	}
	if err := seedStore.Permissions().Create(&store.EndpointPermission{
		UserID:  user.ID,
		RouteID: "route-1",
		Methods: []string{http.MethodGet},
		Allowed: true,
	}); err != nil {
		_ = seedStore.Close()
		t.Fatalf("create permission error: %v", err)
	}
	if err := seedStore.Close(); err != nil {
		t.Fatalf("close seed store error: %v", err)
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}
	t.Cleanup(func() {
		if gw.store != nil {
			_ = gw.store.Close()
		}
	})

	call := func(key string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
		req.Header.Set("X-API-Key", key)
		rr := httptest.NewRecorder()
		gw.ServeHTTP(rr, req)
		return rr
	}

	liveResp1 := call(liveKey)
	if liveResp1.Code != http.StatusOK || liveResp1.Body.String() != "billing-ok" {
		t.Fatalf("expected first live request success, got status=%d body=%q", liveResp1.Code, liveResp1.Body.String())
	}
	balanceUser, err := gw.store.Users().FindByID(user.ID)
	if err != nil || balanceUser == nil {
		t.Fatalf("find user after first live request error=%v user=%#v", err, balanceUser)
	}
	if balanceUser.CreditBalance != 5 {
		t.Fatalf("expected balance 5 after first live request got %d", balanceUser.CreditBalance)
	}

	testResp := call(testKey)
	if testResp.Code != http.StatusOK || testResp.Body.String() != "billing-ok" {
		t.Fatalf("expected test-key request success, got status=%d body=%q", testResp.Code, testResp.Body.String())
	}
	balanceUser, err = gw.store.Users().FindByID(user.ID)
	if err != nil || balanceUser == nil {
		t.Fatalf("find user after test key request error=%v user=%#v", err, balanceUser)
	}
	if balanceUser.CreditBalance != 5 {
		t.Fatalf("expected balance unchanged for test key, got %d", balanceUser.CreditBalance)
	}

	liveResp2 := call(liveKey)
	if liveResp2.Code != http.StatusOK {
		t.Fatalf("expected second live request success, got %d body=%q", liveResp2.Code, liveResp2.Body.String())
	}
	balanceUser, err = gw.store.Users().FindByID(user.ID)
	if err != nil || balanceUser == nil {
		t.Fatalf("find user after second live request error=%v user=%#v", err, balanceUser)
	}
	if balanceUser.CreditBalance != 0 {
		t.Fatalf("expected balance 0 after second live request got %d", balanceUser.CreditBalance)
	}

	liveResp3 := call(liveKey)
	if liveResp3.Code != http.StatusPaymentRequired {
		t.Fatalf("expected insufficient credits 402 got %d body=%q", liveResp3.Code, liveResp3.Body.String())
	}
	var payload map[string]map[string]string
	if err := json.Unmarshal(liveResp3.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected json payload: %v", err)
	}
	if payload["error"]["code"] != "insufficient_credits" {
		t.Fatalf("unexpected insufficient payload: %#v", payload)
	}
}

func TestGatewayBillingAllowWithFlag(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("billing-allow-ok"))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.Store = config.StoreConfig{
		Path:        t.TempDir() + "/gateway-billing-allow.db",
		BusyTimeout: time.Second,
		JournalMode: "WAL",
		ForeignKeys: true,
	}
	cfg.GlobalPlugins = []config.PluginConfig{
		{Name: "auth-apikey"},
	}
	cfg.Billing = config.BillingConfig{
		Enabled:           true,
		DefaultCost:       5,
		ZeroBalanceAction: "allow_with_flag",
		TestModeEnabled:   true,
	}

	seedStore, err := store.Open(cfg)
	if err != nil {
		t.Fatalf("open seed store error: %v", err)
	}
	user := createGatewayBillingUser(t, seedStore, "billing-allow@example.com", 0)
	liveKey, _, err := seedStore.APIKeys().Create(user.ID, "live", "live")
	if err != nil {
		_ = seedStore.Close()
		t.Fatalf("create live key error: %v", err)
	}
	if err := seedStore.Permissions().Create(&store.EndpointPermission{
		UserID:  user.ID,
		RouteID: "route-1",
		Methods: []string{http.MethodGet},
		Allowed: true,
	}); err != nil {
		_ = seedStore.Close()
		t.Fatalf("create permission error: %v", err)
	}
	if err := seedStore.Close(); err != nil {
		t.Fatalf("close seed store error: %v", err)
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}
	t.Cleanup(func() {
		if gw.store != nil {
			_ = gw.store.Close()
		}
	})

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	req.Header.Set("X-API-Key", liveKey)
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || rr.Body.String() != "billing-allow-ok" {
		t.Fatalf("expected allow_with_flag request success, got status=%d body=%q", rr.Code, rr.Body.String())
	}
	after, err := gw.store.Users().FindByID(user.ID)
	if err != nil || after == nil {
		t.Fatalf("find user error=%v user=%#v", err, after)
	}
	if after.CreditBalance != 0 {
		t.Fatalf("expected balance to remain 0 in allow_with_flag, got %d", after.CreditBalance)
	}
}

func TestGatewayPermissionRateLimitAndCreditOverride(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("perm-override-ok"))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.Store = config.StoreConfig{
		Path:        t.TempDir() + "/gateway-permission-override.db",
		BusyTimeout: time.Second,
		JournalMode: "WAL",
		ForeignKeys: true,
	}
	cfg.GlobalPlugins = []config.PluginConfig{
		{Name: "auth-apikey"},
		{
			Name: "rate-limit",
			Config: map[string]any{
				"algorithm": "fixed_window",
				"scope":     "consumer",
				"limit":     100,
				"window":    "1s",
			},
		},
	}
	cfg.Billing = config.BillingConfig{
		Enabled:           true,
		DefaultCost:       5,
		ZeroBalanceAction: "reject",
		TestModeEnabled:   true,
	}

	seedStore, err := store.Open(cfg)
	if err != nil {
		t.Fatalf("open seed store error: %v", err)
	}
	user := createGatewayBillingUser(t, seedStore, "perm-override@example.com", 10)
	key, _, err := seedStore.APIKeys().Create(user.ID, "live", "live")
	if err != nil {
		_ = seedStore.Close()
		t.Fatalf("create key error: %v", err)
	}
	overrideCost := int64(2)
	if err := seedStore.Permissions().Create(&store.EndpointPermission{
		UserID:  user.ID,
		RouteID: "route-1",
		Methods: []string{http.MethodGet},
		Allowed: true,
		RateLimits: map[string]any{
			"algorithm": "fixed_window",
			"scope":     "consumer",
			"limit":     1,
			"window":    "1s",
		},
		CreditCost: &overrideCost,
	}); err != nil {
		_ = seedStore.Close()
		t.Fatalf("create permission error: %v", err)
	}
	if err := seedStore.Close(); err != nil {
		t.Fatalf("close seed store error: %v", err)
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}
	t.Cleanup(func() {
		if gw.store != nil {
			_ = gw.store.Close()
		}
	})

	req1 := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	req1.Header.Set("X-API-Key", key)
	rr1 := httptest.NewRecorder()
	gw.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK || rr1.Body.String() != "perm-override-ok" {
		t.Fatalf("expected first request success, got status=%d body=%q", rr1.Code, rr1.Body.String())
	}
	afterFirst, err := gw.store.Users().FindByID(user.ID)
	if err != nil || afterFirst == nil {
		t.Fatalf("find user after first request error=%v user=%#v", err, afterFirst)
	}
	if afterFirst.CreditBalance != 8 {
		t.Fatalf("expected credit override deduction to 8, got %d", afterFirst.CreditBalance)
	}

	req2 := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	req2.Header.Set("X-API-Key", key)
	rr2 := httptest.NewRecorder()
	gw.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected permission rate limit override to return 429, got %d body=%q", rr2.Code, rr2.Body.String())
	}
	afterSecond, err := gw.store.Users().FindByID(user.ID)
	if err != nil || afterSecond == nil {
		t.Fatalf("find user after second request error=%v user=%#v", err, afterSecond)
	}
	if afterSecond.CreditBalance != 8 {
		t.Fatalf("expected no extra deduction after rate-limited request, got %d", afterSecond.CreditBalance)
	}
}

func TestGatewayUserIPWhitelistCIDRAllow(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ip-whitelist-ok"))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.Store = config.StoreConfig{
		Path:        t.TempDir() + "/gateway-ip-whitelist-allow.db",
		BusyTimeout: time.Second,
		JournalMode: "WAL",
		ForeignKeys: true,
	}
	cfg.GlobalPlugins = []config.PluginConfig{
		{Name: "auth-apikey"},
	}

	seedStore, err := store.Open(cfg)
	if err != nil {
		t.Fatalf("open seed store error: %v", err)
	}
	user := createGatewayBillingUser(t, seedStore, "ip-allow@example.com", 10)
	user.IPWhitelist = []string{"203.0.113.0/24"}
	if err := seedStore.Users().Update(user); err != nil {
		_ = seedStore.Close()
		t.Fatalf("update user whitelist error: %v", err)
	}
	key, _, err := seedStore.APIKeys().Create(user.ID, "live", "live")
	if err != nil {
		_ = seedStore.Close()
		t.Fatalf("create key error: %v", err)
	}
	if err := seedStore.Permissions().Create(&store.EndpointPermission{
		UserID:  user.ID,
		RouteID: "route-1",
		Methods: []string{http.MethodGet},
		Allowed: true,
	}); err != nil {
		_ = seedStore.Close()
		t.Fatalf("create permission error: %v", err)
	}
	if err := seedStore.Close(); err != nil {
		t.Fatalf("close seed store error: %v", err)
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}
	t.Cleanup(func() {
		if gw.store != nil {
			_ = gw.store.Close()
		}
	})

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	req.Header.Set("X-API-Key", key)
	req.RemoteAddr = "203.0.113.8:1234"
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || rr.Body.String() != "ip-whitelist-ok" {
		t.Fatalf("expected whitelist cidr request success, got status=%d body=%q", rr.Code, rr.Body.String())
	}
}

func TestGatewayUserIPWhitelistDenied(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("should-not-hit"))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.Store = config.StoreConfig{
		Path:        t.TempDir() + "/gateway-ip-whitelist-deny.db",
		BusyTimeout: time.Second,
		JournalMode: "WAL",
		ForeignKeys: true,
	}
	cfg.GlobalPlugins = []config.PluginConfig{
		{Name: "auth-apikey"},
	}

	seedStore, err := store.Open(cfg)
	if err != nil {
		t.Fatalf("open seed store error: %v", err)
	}
	user := createGatewayBillingUser(t, seedStore, "ip-deny@example.com", 10)
	user.IPWhitelist = []string{"203.0.113.0/24"}
	if err := seedStore.Users().Update(user); err != nil {
		_ = seedStore.Close()
		t.Fatalf("update user whitelist error: %v", err)
	}
	key, _, err := seedStore.APIKeys().Create(user.ID, "live", "live")
	if err != nil {
		_ = seedStore.Close()
		t.Fatalf("create key error: %v", err)
	}
	if err := seedStore.Permissions().Create(&store.EndpointPermission{
		UserID:  user.ID,
		RouteID: "route-1",
		Methods: []string{http.MethodGet},
		Allowed: true,
	}); err != nil {
		_ = seedStore.Close()
		t.Fatalf("create permission error: %v", err)
	}
	if err := seedStore.Close(); err != nil {
		t.Fatalf("close seed store error: %v", err)
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}
	t.Cleanup(func() {
		if gw.store != nil {
			_ = gw.store.Close()
		}
	})

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	req.Header.Set("X-API-Key", key)
	req.RemoteAddr = "198.51.100.9:4321"
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-whitelisted ip, got %d body=%q", rr.Code, rr.Body.String())
	}
	var payload map[string]map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected json payload: %v", err)
	}
	if payload["error"]["code"] != "ip_not_allowed" {
		t.Fatalf("unexpected deny payload: %#v", payload)
	}
}

func TestGatewayPluginPipelineAuthRateLimitProxy(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok-from-pipeline"))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.Consumers = []config.Consumer{
		{
			ID:   "consumer-1",
			Name: "consumer-1",
			APIKeys: []config.ConsumerAPIKey{
				{ID: "k1", Key: "ck_live_pipeline"},
			},
		},
	}
	cfg.GlobalPlugins = []config.PluginConfig{
		{
			Name: "auth-apikey",
		},
	}
	cfg.Routes[0].Plugins = []config.PluginConfig{
		{
			Name: "rate-limit",
			Config: map[string]any{
				"algorithm": "fixed_window",
				"scope":     "consumer",
				"limit":     1,
				"window":    "1s",
			},
		},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	req1 := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	req1.Header.Set("X-API-Key", "ck_live_pipeline")
	rr1 := httptest.NewRecorder()
	gw.ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusOK {
		t.Fatalf("expected first request 200 got %d body=%q", rr1.Code, rr1.Body.String())
	}
	if rr1.Body.String() != "ok-from-pipeline" {
		t.Fatalf("unexpected first body: %q", rr1.Body.String())
	}
	if rr1.Header().Get("X-RateLimit-Limit") == "" {
		t.Fatalf("expected rate limit headers on allowed response")
	}

	req2 := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	req2.Header.Set("X-API-Key", "ck_live_pipeline")
	rr2 := httptest.NewRecorder()
	gw.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request 429 got %d body=%q", rr2.Code, rr2.Body.String())
	}
	if rr2.Header().Get("Retry-After") == "" {
		t.Fatalf("expected Retry-After header")
	}
}

func TestGatewayPluginPipelineCircuitBreakerOpen(t *testing.T) {
	t.Parallel()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", "127.0.0.1:1") // closed port -> proxy failure
	cfg.GlobalPlugins = []config.PluginConfig{
		{
			Name: "circuit-breaker",
			Config: map[string]any{
				"error_threshold":    1.0,
				"volume_threshold":   1,
				"sleep_window":       "3s",
				"half_open_requests": 1,
				"window":             "5s",
			},
		},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	req1 := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	rr1 := httptest.NewRecorder()
	gw.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusBadGateway {
		t.Fatalf("expected first request 502 got %d", rr1.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	rr2 := httptest.NewRecorder()
	gw.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected second request 503 got %d body=%q", rr2.Code, rr2.Body.String())
	}
	var payload map[string]map[string]string
	if err := json.Unmarshal(rr2.Body.Bytes(), &payload); err != nil {
		t.Fatalf("error response should be valid json: %v", err)
	}
	if payload["error"]["code"] != "circuit_open" {
		t.Fatalf("unexpected error payload: %#v", payload)
	}
}

func TestGatewayPluginPipelineRetryTransportError(t *testing.T) {
	t.Parallel()

	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("retry-ok"))
	}))
	defer healthy.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", "127.0.0.1:1")
	cfg.Upstreams[0].Targets = []config.UpstreamTarget{
		{ID: "bad", Address: "127.0.0.1:1", Weight: 1},
		{ID: "good", Address: mustHost(t, healthy.URL), Weight: 1},
	}
	cfg.GlobalPlugins = []config.PluginConfig{
		{
			Name: "retry",
			Config: map[string]any{
				"max_retries": 1,
				"base_delay":  "1ms",
				"max_delay":   "2ms",
				"jitter":      false,
			},
		},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || rr.Body.String() != "retry-ok" {
		t.Fatalf("expected retry success, got status=%d body=%q", rr.Code, rr.Body.String())
	}
}

func TestGatewayPluginPipelineRetryRespectsIdempotency(t *testing.T) {
	t.Parallel()

	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("should-not-be-hit"))
	}))
	defer healthy.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", "127.0.0.1:1")
	cfg.Routes[0].Methods = []string{http.MethodPost}
	cfg.Upstreams[0].Targets = []config.UpstreamTarget{
		{ID: "bad", Address: "127.0.0.1:1", Weight: 1},
		{ID: "good", Address: mustHost(t, healthy.URL), Weight: 1},
	}
	cfg.GlobalPlugins = []config.PluginConfig{
		{
			Name: "retry",
			Config: map[string]any{
				"max_retries": 3,
				"base_delay":  "1ms",
				"max_delay":   "2ms",
				"jitter":      false,
			},
		},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "http://gateway.local/api/users", nil)
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for non-idempotent method without retry, got %d", rr.Code)
	}
}

func TestGatewayPluginPipelineRetryOnStatusCode(t *testing.T) {
	t.Parallel()

	up503 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("unavailable"))
	}))
	defer up503.Close()

	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("retry-status-ok"))
	}))
	defer healthy.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, up503.URL))
	cfg.Upstreams[0].Targets = []config.UpstreamTarget{
		{ID: "svc503", Address: mustHost(t, up503.URL), Weight: 1},
		{ID: "good", Address: mustHost(t, healthy.URL), Weight: 1},
	}
	cfg.GlobalPlugins = []config.PluginConfig{
		{
			Name: "retry",
			Config: map[string]any{
				"max_retries": 1,
				"base_delay":  "1ms",
				"max_delay":   "2ms",
				"jitter":      false,
			},
		},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || rr.Body.String() != "retry-status-ok" {
		t.Fatalf("expected retry-on-status success, got status=%d body=%q", rr.Code, rr.Body.String())
	}
}

func TestGatewayPluginPipelineTimeoutCompletesWithinLimit(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(20 * time.Millisecond)
		_, _ = w.Write([]byte("timeout-ok"))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.GlobalPlugins = []config.PluginConfig{
		{
			Name: "timeout",
			Config: map[string]any{
				"timeout": "120ms",
			},
		},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || rr.Body.String() != "timeout-ok" {
		t.Fatalf("expected success within timeout, got status=%d body=%q", rr.Code, rr.Body.String())
	}
}

func TestGatewayPluginPipelineTimeoutExceeded(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(120 * time.Millisecond)
		_, _ = w.Write([]byte("too-late"))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.GlobalPlugins = []config.PluginConfig{
		{
			Name: "timeout",
			Config: map[string]any{
				"timeout": "40ms",
			},
		},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)

	if rr.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504 on timeout, got status=%d body=%q", rr.Code, rr.Body.String())
	}
}

func TestGatewayPluginPipelineRequestTransform(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("method mismatch"))
			return
		}
		if r.URL.Path != "/api/transformed" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("path mismatch"))
			return
		}
		if query.Get("renamed") != "1" || query.Get("added") != "yes" || query.Get("remove") != "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("query mismatch"))
			return
		}
		if r.Header.Get("X-New") != "old-value" || r.Header.Get("X-Remove") != "" || r.Header.Get("X-Added") != "v-added" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("header mismatch"))
			return
		}
		_, _ = w.Write([]byte("transform-ok"))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.Routes[0].Plugins = []config.PluginConfig{
		{
			Name: "request-transform",
			Config: map[string]any{
				"add_headers": map[string]any{
					"X-Added": "v-added",
				},
				"remove_headers": []any{"X-Remove"},
				"rename_headers": map[string]any{
					"X-Old": "X-New",
				},
				"add_query": map[string]any{
					"added": "yes",
				},
				"remove_query": []any{"remove"},
				"rename_query": map[string]any{
					"keep": "renamed",
				},
				"method":           "POST",
				"path_pattern":     "^/api/users$",
				"path_replacement": "/api/transformed",
			},
		},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users?keep=1&remove=2", nil)
	req.Header.Set("X-Old", "old-value")
	req.Header.Set("X-Remove", "remove-value")
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || rr.Body.String() != "transform-ok" {
		t.Fatalf("expected transformed request to succeed, got status=%d body=%q", rr.Code, rr.Body.String())
	}
}

func TestGatewayPluginPipelineResponseTransform(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream", "remove-me")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"original"}`))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.Routes[0].Plugins = []config.PluginConfig{
		{
			Name: "response-transform",
			Config: map[string]any{
				"add_headers": map[string]any{
					"X-Added": "ok",
				},
				"remove_headers": []any{"X-Upstream"},
				"replace_body":   `{"status":"rewritten"}`,
			},
		},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202 got %d body=%q", rr.Code, rr.Body.String())
	}
	if rr.Body.String() != `{"status":"rewritten"}` {
		t.Fatalf("expected transformed response body, got %q", rr.Body.String())
	}
	if rr.Header().Get("X-Upstream") != "" {
		t.Fatalf("expected upstream header removed")
	}
	if rr.Header().Get("X-Added") != "ok" {
		t.Fatalf("expected added response header")
	}
}

func TestGatewayPluginPipelineURLRewrite(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/42" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("path mismatch"))
			return
		}
		if r.URL.Query().Get("foo") != "bar" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("query mismatch"))
			return
		}
		_, _ = w.Write([]byte("rewrite-ok"))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.Routes[0].Paths = []string{"/api/v1/users/*"}
	cfg.Routes[0].Plugins = []config.PluginConfig{
		{
			Name: "url-rewrite",
			Config: map[string]any{
				"pattern":     "^/api/v1/users/(.+)$",
				"replacement": "/users/$1",
			},
		},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/v1/users/42?foo=bar", nil)
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || rr.Body.String() != "rewrite-ok" {
		t.Fatalf("expected rewritten request to succeed, got status=%d body=%q", rr.Code, rr.Body.String())
	}
}

func TestGatewayPluginPipelineFullFlowAuthRateLimitTransformProxyResponseTransform(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/users" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("path mismatch"))
			return
		}
		if r.Header.Get("X-Transformed") != "yes" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("header mismatch"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("upstream-original"))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.Consumers = []config.Consumer{
		{
			ID:   "consumer-flow",
			Name: "consumer-flow",
			APIKeys: []config.ConsumerAPIKey{
				{ID: "k-flow", Key: "ck_flow_1"},
			},
		},
	}
	cfg.GlobalPlugins = []config.PluginConfig{
		{Name: "auth-apikey"},
		{
			Name: "response-transform",
			Config: map[string]any{
				"add_headers": map[string]any{
					"X-Post-Transform": "ok",
				},
				"replace_body": `{"result":"post-transformed"}`,
			},
		},
	}
	cfg.Routes[0].Plugins = []config.PluginConfig{
		{
			Name: "rate-limit",
			Config: map[string]any{
				"algorithm": "fixed_window",
				"scope":     "consumer",
				"limit":     1,
				"window":    "1s",
			},
		},
		{
			Name: "request-transform",
			Config: map[string]any{
				"path_pattern":     "^/api/users$",
				"path_replacement": "/internal/users",
				"add_headers": map[string]any{
					"X-Transformed": "yes",
				},
			},
		},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	req1 := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	req1.Header.Set("X-API-Key", "ck_flow_1")
	rr1 := httptest.NewRecorder()
	gw.ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusOK {
		t.Fatalf("expected first request 200 got %d body=%q", rr1.Code, rr1.Body.String())
	}
	if rr1.Body.String() != `{"result":"post-transformed"}` {
		t.Fatalf("expected response-transform body rewrite, got %q", rr1.Body.String())
	}
	if rr1.Header().Get("X-Post-Transform") != "ok" {
		t.Fatalf("expected post-transform header")
	}

	req2 := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	req2.Header.Set("X-API-Key", "ck_flow_1")
	rr2 := httptest.NewRecorder()
	gw.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request 429 got %d body=%q", rr2.Code, rr2.Body.String())
	}
}

func TestGatewayPluginPipelineRequestSizeLimit(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		_, _ = w.Write(body)
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.Routes[0].Methods = []string{http.MethodPost}
	cfg.Routes[0].Plugins = []config.PluginConfig{
		{
			Name: "request-size-limit",
			Config: map[string]any{
				"max_bytes": 5,
			},
		},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	req1 := httptest.NewRequest(http.MethodPost, "http://gateway.local/api/users", bytes.NewBufferString("12345"))
	rr1 := httptest.NewRecorder()
	gw.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK || rr1.Body.String() != "12345" {
		t.Fatalf("expected allowed request to pass, got status=%d body=%q", rr1.Code, rr1.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPost, "http://gateway.local/api/users", bytes.NewBufferString("123456"))
	rr2 := httptest.NewRecorder()
	gw.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for oversized payload, got %d body=%q", rr2.Code, rr2.Body.String())
	}
	var payload map[string]map[string]string
	if err := json.Unmarshal(rr2.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected json error payload: %v", err)
	}
	if payload["error"]["code"] != "payload_too_large" {
		t.Fatalf("unexpected error code payload: %#v", payload)
	}
}

func TestGatewayPluginPipelineRequestValidator(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("validated-ok"))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.Routes[0].Methods = []string{http.MethodPost}
	cfg.Routes[0].Plugins = []config.PluginConfig{
		{
			Name: "request-validator",
			Config: map[string]any{
				"schema": map[string]any{
					"type":     "object",
					"required": []any{"name", "email"},
					"properties": map[string]any{
						"name":  map[string]any{"type": "string"},
						"email": map[string]any{"type": "string", "format": "email"},
					},
				},
			},
		},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	validReq := httptest.NewRequest(http.MethodPost, "http://gateway.local/api/users", bytes.NewBufferString(`{"name":"Alice","email":"alice@example.com"}`))
	validResp := httptest.NewRecorder()
	gw.ServeHTTP(validResp, validReq)
	if validResp.Code != http.StatusOK || validResp.Body.String() != "validated-ok" {
		t.Fatalf("expected valid request to pass, got status=%d body=%q", validResp.Code, validResp.Body.String())
	}

	invalidReq := httptest.NewRequest(http.MethodPost, "http://gateway.local/api/users", bytes.NewBufferString(`{"name":123,"email":"invalid"}`))
	invalidResp := httptest.NewRecorder()
	gw.ServeHTTP(invalidResp, invalidReq)
	if invalidResp.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid payload 400 got %d body=%q", invalidResp.Code, invalidResp.Body.String())
	}
	var payload map[string]map[string]string
	if err := json.Unmarshal(invalidResp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected json error payload: %v", err)
	}
	if payload["error"]["code"] != "validation_failed" {
		t.Fatalf("unexpected error code payload: %#v", payload)
	}
}

func TestGatewayPluginPipelineCompression(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello-compression"))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.Routes[0].Plugins = []config.PluginConfig{
		{
			Name: "compression",
			Config: map[string]any{
				"min_size": 5,
			},
		},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body=%q", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("expected gzip content encoding")
	}
	reader, err := gzip.NewReader(bytes.NewReader(rr.Body.Bytes()))
	if err != nil {
		t.Fatalf("create gzip reader: %v", err)
	}
	decompressed, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatalf("read gzip body: %v", err)
	}
	if string(decompressed) != "hello-compression" {
		t.Fatalf("unexpected decompressed body %q", string(decompressed))
	}
}

func TestGatewayPluginPipelineCorrelationID(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("missing request id"))
			return
		}
		w.Header().Set("X-Upstream-Request-ID", id)
		_, _ = w.Write([]byte(id))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.Routes[0].Plugins = []config.PluginConfig{
		{Name: "correlation-id"},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	req1 := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	rr1 := httptest.NewRecorder()
	gw.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body=%q", rr1.Code, rr1.Body.String())
	}
	generatedID := strings.TrimSpace(rr1.Body.String())
	if generatedID == "" {
		t.Fatalf("expected generated request id body")
	}
	if rr1.Header().Get("X-Request-ID") != generatedID {
		t.Fatalf("expected response X-Request-ID to equal generated id")
	}
	if rr1.Header().Get("X-Upstream-Request-ID") != generatedID {
		t.Fatalf("expected upstream to receive generated id")
	}

	req2 := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	req2.Header.Set("X-Request-ID", "req-fixed-001")
	rr2 := httptest.NewRecorder()
	gw.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body=%q", rr2.Code, rr2.Body.String())
	}
	if rr2.Body.String() != "req-fixed-001" {
		t.Fatalf("expected existing id to be forwarded, got %q", rr2.Body.String())
	}
	if rr2.Header().Get("X-Request-ID") != "req-fixed-001" {
		t.Fatalf("expected response X-Request-ID to keep provided id")
	}
}

func TestGatewayPluginPipelineBotDetect(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("bot-check-ok"))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.Routes[0].Plugins = []config.PluginConfig{
		{
			Name: "bot-detect",
			Config: map[string]any{
				"action": "block",
			},
		},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	botReq := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	botReq.Header.Set("User-Agent", "Googlebot/2.1")
	botResp := httptest.NewRecorder()
	gw.ServeHTTP(botResp, botReq)
	if botResp.Code != http.StatusForbidden {
		t.Fatalf("expected bot request 403 got %d body=%q", botResp.Code, botResp.Body.String())
	}
	var payload map[string]map[string]string
	if err := json.Unmarshal(botResp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected json error payload: %v", err)
	}
	if payload["error"]["code"] != "bot_blocked" {
		t.Fatalf("unexpected error payload: %#v", payload)
	}

	humanReq := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	humanReq.Header.Set("User-Agent", "Mozilla/5.0")
	humanResp := httptest.NewRecorder()
	gw.ServeHTTP(humanResp, humanReq)
	if humanResp.Code != http.StatusOK || humanResp.Body.String() != "bot-check-ok" {
		t.Fatalf("expected non-bot request to pass, got status=%d body=%q", humanResp.Code, humanResp.Body.String())
	}
}

func TestGatewayPluginPipelineRedirect(t *testing.T) {
	t.Parallel()

	var upstreamHits atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits.Add(1)
		_, _ = w.Write([]byte("should-not-hit"))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.Routes[0].Plugins = []config.PluginConfig{
		{
			Name: "redirect",
			Config: map[string]any{
				"path":        "/api/users",
				"url":         "https://example.com/new",
				"status_code": 307,
			},
		},
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users?foo=bar", nil)
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 got %d body=%q", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Location") != "https://example.com/new?foo=bar" {
		t.Fatalf("unexpected redirect location %q", rr.Header().Get("Location"))
	}
	if upstreamHits.Load() != 0 {
		t.Fatalf("expected upstream not to be called for redirect")
	}
}

func TestGatewayAuditLoggingCapturesAndMasks(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"password":"upstream-secret","ok":true}`))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	cfg.Routes[0].Methods = []string{http.MethodPost}
	cfg.Store = config.StoreConfig{
		Path:        t.TempDir() + "/gateway-audit.db",
		BusyTimeout: time.Second,
		JournalMode: "WAL",
		ForeignKeys: true,
	}
	cfg.Audit = config.AuditConfig{
		Enabled:              true,
		BufferSize:           64,
		BatchSize:            10,
		FlushInterval:        time.Second,
		StoreRequestBody:     true,
		StoreResponseBody:    true,
		MaxRequestBodyBytes:  4096,
		MaxResponseBodyBytes: 4096,
		MaskHeaders:          []string{"Authorization"},
		MaskBodyFields:       []string{"password"},
		MaskReplacement:      "***",
	}

	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}
	t.Cleanup(func() {
		if gw.store != nil {
			_ = gw.store.Close()
		}
	})

	req := httptest.NewRequest(http.MethodPost, "http://gateway.local/api/users", bytes.NewBufferString(`{"password":"client-secret"}`))
	req.Header.Set("Authorization", "Bearer abc")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body=%q", rr.Code, rr.Body.String())
	}

	logs, err := gw.store.Audits().List(store.AuditListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if logs.Total != 1 || len(logs.Entries) != 1 {
		t.Fatalf("expected one audit row, got total=%d len=%d", logs.Total, len(logs.Entries))
	}
	entry := logs.Entries[0]
	if entry.Method != http.MethodPost {
		t.Fatalf("unexpected method in audit row: %s", entry.Method)
	}
	if entry.RequestBody != `{"password":"***"}` {
		t.Fatalf("request body should be masked, got %s", entry.RequestBody)
	}
	if entry.ResponseBody != `{"ok":true,"password":"***"}` && entry.ResponseBody != `{"password":"***","ok":true}` {
		t.Fatalf("response body should be masked, got %s", entry.ResponseBody)
	}
	if entry.RequestHeaders["Authorization"] != "***" {
		t.Fatalf("authorization header should be masked: %#v", entry.RequestHeaders["Authorization"])
	}
}

func TestGatewayAnalyticsRecordsRequestMetrics(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}))
	defer upstream.Close()

	cfg := gatewayTestConfig(t, "127.0.0.1:0", mustHost(t, upstream.URL))
	gw, err := New(cfg)
	if err != nil {
		t.Fatalf("New gateway error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/users", nil)
	rr := httptest.NewRecorder()
	gw.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 got %d body=%q", rr.Code, rr.Body.String())
	}

	engine := gw.Analytics()
	if engine == nil {
		t.Fatalf("expected analytics engine")
	}

	overview := engine.Overview()
	if overview.TotalRequests != 1 {
		t.Fatalf("expected total_requests=1 got %d", overview.TotalRequests)
	}
	if overview.TotalErrors != 0 {
		t.Fatalf("expected total_errors=0 got %d", overview.TotalErrors)
	}
	if overview.ActiveConns != 0 {
		t.Fatalf("expected active_conns=0 got %d", overview.ActiveConns)
	}

	latest := engine.Latest(1)
	if len(latest) != 1 {
		t.Fatalf("expected 1 latest metric got %d", len(latest))
	}
	metric := latest[0]
	if metric.RouteID != "route-1" || metric.RouteName != "users-route" {
		t.Fatalf("unexpected route metric: %#v", metric)
	}
	if metric.Method != http.MethodGet || metric.Path != "/api/users" {
		t.Fatalf("unexpected request metric fields: %#v", metric)
	}
	if metric.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected status code metric: %d", metric.StatusCode)
	}
	if metric.BytesOut <= 0 {
		t.Fatalf("expected bytes_out > 0 got %d", metric.BytesOut)
	}

	now := time.Now().UTC()
	buckets := engine.TimeSeries(now.Add(-time.Minute), now.Add(time.Minute))
	if len(buckets) == 0 {
		t.Fatalf("expected at least one analytics bucket")
	}
	if buckets[len(buckets)-1].Requests <= 0 {
		t.Fatalf("expected bucket requests > 0, buckets=%#v", buckets)
	}
}

func gatewayTestConfig(t *testing.T, addr, upstreamHost string) *config.Config {
	t.Helper()

	return &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:       addr,
			ReadTimeout:    2 * time.Second,
			WriteTimeout:   2 * time.Second,
			IdleTimeout:    10 * time.Second,
			MaxHeaderBytes: 1 << 20,
			MaxBodyBytes:   1 << 20,
		},
		Services: []config.Service{
			{
				ID:       "svc-1",
				Name:     "svc-users",
				Protocol: "http",
				Upstream: "up-users",
			},
		},
		Routes: []config.Route{
			{
				ID:       "route-1",
				Name:     "users-route",
				Service:  "svc-users",
				Paths:    []string{"/api/users"},
				Methods:  []string{http.MethodGet},
				Priority: 100,
			},
		},
		Upstreams: []config.Upstream{
			{
				ID:        "up-1",
				Name:      "up-users",
				Algorithm: "round_robin",
				Targets: []config.UpstreamTarget{
					{ID: "target-1", Address: upstreamHost, Weight: 1},
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
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

func waitForHTTPReady(t *testing.T, rawURL string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(rawURL)
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatalf("gateway did not become ready for %s", rawURL)
}

func readAllAndClose(t *testing.T, r io.ReadCloser) string {
	t.Helper()
	defer r.Close()
	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(body)
}

func createGatewayBillingUser(t *testing.T, st *store.Store, email string, balance int64) *store.User {
	t.Helper()
	pw, err := store.HashPassword("pw")
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	user := &store.User{
		Email:         email,
		Name:          "Gateway Billing User",
		PasswordHash:  pw,
		Role:          "user",
		Status:        "active",
		CreditBalance: balance,
	}
	if err := st.Users().Create(user); err != nil {
		t.Fatalf("create user error: %v", err)
	}
	return user
}
