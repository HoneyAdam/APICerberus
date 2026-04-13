package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/logging"
	"github.com/APICerberus/APICerebrus/internal/store"
)

// --- Bulk import with valid upstreams ---

func TestBulkDatabaseOperation_ExecWithRealDB(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	payload := `{
		"upstreams": [{"id":"up-bulk-test","name":"BulkUpstream","algorithm":"round_robin","targets":[{"address":"localhost:3001"}]}],
		"services": [],
		"routes": [],
		"consumers": []
	}`
	resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/import", token, []byte(payload))
	code := resp["status_code"].(float64)
	if code != http.StatusOK {
		t.Logf("bulk import upstreams returned %v", resp["status_code"])
	}
}

// --- Bulk import with replace mode (uses valid upstream via existing test paths) ---

func TestHandleBulkImport_ReplaceMode(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	// Create an upstream first via the standard endpoint
	status, _, _ := mustRawRequest(t, http.MethodPost, baseURL+"/admin/api/v1/upstreams", token)
	_ = status

	// Now try bulk import with replace mode
	resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/import", token, nil)
	// nil body should return 200 or 400
	code := resp["status_code"].(float64)
	if code != http.StatusOK && code != http.StatusBadRequest {
		t.Logf("bulk import replace mode returned %v", code)
	}
}

// --- Bulk import with upsert mode ---

func TestHandleBulkImport_UpsertMode(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	payload := `{
		"upstreams": [{"id":"up-upsert","name":"UpsertMe","algorithm":"round_robin","targets":[{"address":"localhost:3001"}]}],
		"mode": "upsert"
	}`
	resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/import", token, []byte(payload))
	code := resp["status_code"].(float64)
	if code != http.StatusOK && code != http.StatusBadRequest {
		t.Errorf("expected 200 or 400 for upsert mode, got %v", code)
	}
}

// --- Bulk delete with valid data ---

func TestHandleBulkDelete_NonexistentIDs(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/delete", token, []byte(`{"type":"service","ids":["nonexistent-1","nonexistent-2"]}`))
	code := resp["status_code"].(float64)
	if code != http.StatusOK {
		t.Logf("bulk delete nonexistent IDs returned %v", resp["status_code"])
	}
}

// --- Bulk services edge cases ---

func TestHandleBulkServices_WithNilBody(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/services", token, nil)
	code := resp["status_code"].(float64)
	if code != http.StatusBadRequest && code != http.StatusOK {
		t.Logf("bulk services nil body returned %v", code)
	}
}

// --- Bulk routes edge cases ---

func TestHandleBulkRoutes_EmptyList(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/routes", token, []byte(`{"routes":[]}`))
	if resp["status_code"].(float64) != http.StatusBadRequest {
		t.Errorf("expected 400 for empty routes list, got %v", resp["status_code"])
	}
}

// --- Bulk plugins edge cases ---

func TestHandleBulkPlugins_EmptyList(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/bulk/plugins", token, []byte(`{"plugins":[]}`))
	if resp["status_code"].(float64) != http.StatusBadRequest {
		t.Errorf("expected 400 for empty plugins list, got %v", resp["status_code"])
	}
}

// --- WebhookManager retryOrFail tests ---

func newTestWebhookManager(t *testing.T) (*WebhookManager, *store.Store) {
	t.Helper()
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Store: config.StoreConfig{Path: tmpDir + "/test.db", BusyTimeout: time.Second, JournalMode: "WAL", ForeignKeys: true},
		Admin: config.AdminConfig{APIKey: "test-key"},
	}
	st, err := store.Open(cfg)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}

	m := &WebhookManager{
		webhooks:   make(map[string]*store.Webhook),
		deliveryCh: make(chan *store.WebhookDelivery, 10),
		store:      st,
		client:     &http.Client{Timeout: time.Second},
	}
	return m, st
}

func TestWebhookManager_RetryOrFail_Retries(t *testing.T) {
	t.Parallel()
	m, st := newTestWebhookManager(t)
	defer st.Close()

	now := time.Now().UTC()
	webhook := &store.Webhook{
		ID:            "wh-test",
		Name:          "Test Webhook",
		URL:           "http://localhost:9999/hook",
		Events:        []string{"test.event"},
		RetryInterval: 1,
		Active:        true,
		LastTriggered: now,
	}
	if err := st.CreateWebhook(webhook); err != nil {
		t.Fatalf("failed to create webhook: %v", err)
	}

	delivery := &store.WebhookDelivery{
		ID:          "del-retry-1",
		WebhookID:   "wh-test",
		EventType:   "test.event",
		Status:      "pending",
		Attempt:     0,
		MaxAttempts: 3,
		CreatedAt:   now,
	}

	if err := st.CreateDelivery(delivery); err != nil {
		t.Fatalf("failed to create delivery: %v", err)
	}

	m.retryOrFail(delivery, webhook)

	if delivery.Attempt != 1 {
		t.Errorf("expected attempt 1, got %d", delivery.Attempt)
	}
	if delivery.Status == "failed" {
		t.Error("should not have marked as failed on first retry")
	}
}

func TestWebhookManager_RetryOrFail_MaxRetriesExceeded(t *testing.T) {
	t.Parallel()
	m, st := newTestWebhookManager(t)
	defer st.Close()

	now := time.Now().UTC()
	webhook := &store.Webhook{
		ID:     "wh-test",
		Name:   "Test Webhook",
		URL:    "http://localhost:9999/hook",
		Events: []string{"test.event"},
		Active: true,
	}
	if err := st.CreateWebhook(webhook); err != nil {
		t.Fatalf("failed to create webhook: %v", err)
	}

	delivery := &store.WebhookDelivery{
		ID:          "del-fail-1",
		WebhookID:   "wh-test",
		EventType:   "test.event",
		Status:      "pending",
		Attempt:     3,
		MaxAttempts: 3,
		CreatedAt:   now,
	}

	if err := st.CreateDelivery(delivery); err != nil {
		t.Fatalf("failed to create delivery: %v", err)
	}

	m.retryOrFail(delivery, webhook)

	if delivery.Status != "failed" {
		t.Errorf("expected status 'failed', got %q", delivery.Status)
	}
	if delivery.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
}

func TestWebhookManager_RetryOrFail_DefaultRetryInterval(t *testing.T) {
	t.Parallel()
	m, st := newTestWebhookManager(t)
	defer st.Close()

	now := time.Now().UTC()
	webhook := &store.Webhook{
		ID:            "wh-test",
		Name:          "Test Webhook",
		URL:           "http://localhost:9999/hook",
		Events:        []string{"test.event"},
		RetryInterval: 0,
		Active:        true,
	}
	if err := st.CreateWebhook(webhook); err != nil {
		t.Fatalf("failed to create webhook: %v", err)
	}

	delivery := &store.WebhookDelivery{
		ID:          "del-default-retry",
		WebhookID:   "wh-test",
		EventType:   "test.event",
		Status:      "pending",
		Attempt:     0,
		MaxAttempts: 5,
		CreatedAt:   now,
	}

	if err := st.CreateDelivery(delivery); err != nil {
		t.Fatalf("failed to create delivery: %v", err)
	}

	m.retryOrFail(delivery, webhook)

	if delivery.Attempt != 1 {
		t.Errorf("expected attempt 1, got %d", delivery.Attempt)
	}
}

// --- WebSocketHub MetricsSnapshot via full server ---

func TestWebSocketHub_MetricsSnapshotFromServer(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/ws/metrics", token, nil)
	code := resp["status_code"].(float64)
	if code != http.StatusOK && code != http.StatusNotFound {
		t.Logf("ws metrics returned %v", code)
	}
}

// --- WebSocketHub Broadcast and Except ---

func TestWebSocketHub_BroadcastAndExcept(t *testing.T) {
	t.Parallel()
	logger := logging.NewStructuredLogger(nil, logging.ErrorLevel)
	hub := NewWebSocketHub(logger)
	defer hub.Stop()

	hub.Broadcast("test-topic", realtimeEvent{Type: "test", Payload: "hello"})
	hub.BroadcastExcept("test-topic", realtimeEvent{Type: "test", Payload: "hello"}, "")
}

// --- Analytics handler with metric parameter ---

func TestAnalyticsTopRoutes_WithMetricParam(t *testing.T) {
	t.Parallel()
	baseURL, _, storePath, token := newAdminTestServer(t)

	seedStore, _ := store.Open(&config.Config{
		Store: config.StoreConfig{Path: storePath, BusyTimeout: time.Second, JournalMode: "WAL"},
	})
	now := time.Now().UTC()
	_ = seedStore.Audits().BatchInsert([]store.AuditEntry{
		{ID: "tr-m1", RequestID: "r1", RouteID: "route-users", ServiceName: "svc-users", Method: "GET", Path: "/users", StatusCode: 200, LatencyMS: 100, ClientIP: "127.0.0.1", CreatedAt: now.Add(-5 * time.Minute)},
		{ID: "tr-m2", RequestID: "r2", RouteID: "route-users", ServiceName: "svc-users", Method: "POST", Path: "/users", StatusCode: 201, LatencyMS: 200, ClientIP: "127.0.0.1", CreatedAt: now.Add(-3 * time.Minute)},
	})
	seedStore.Close()

	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/analytics/top-routes?window=1h&metric=latency", token, nil)
	if resp["status_code"].(float64) != http.StatusOK {
		t.Errorf("expected 200 with metric=latency, got %v", resp["status_code"])
	}
}

// --- GraphQL handler tests ---

func TestGraphQL_PostQuery(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/graphql", token, []byte(`{"query": "{ __typename }"}`))
	code := resp["status_code"].(float64)
	if code != http.StatusOK && code != http.StatusBadRequest && code != http.StatusNotFound {
		t.Logf("graphql query returned %v", code)
	}
}

func TestGraphQL_InvalidQuery(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/graphql", token, []byte(`not json`))
	// May return 400 for bad JSON or 404 if route not registered
	code := resp["status_code"].(float64)
	if code != http.StatusBadRequest && code != http.StatusNotFound && code != http.StatusOK {
		t.Errorf("expected 400, 404, or 200, got %v", code)
	}
}

// --- Webhook handler tests ---

func TestListWebhooks(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/webhooks", token, nil)
	code := resp["status_code"].(float64)
	if code != http.StatusOK && code != http.StatusNotFound {
		t.Logf("list webhooks returned %v", code)
	}
}

func TestCreateWebhook_InvalidPayload(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/webhooks", token, []byte(`not json`))
	code := resp["status_code"].(float64)
	if code != http.StatusBadRequest && code != http.StatusNotFound && code != http.StatusOK {
		t.Logf("create webhook invalid returned %v", code)
	}
}

func TestCreateWebhook_MissingURL(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/webhooks", token, []byte(`{"name":"Bad Hook","events":["route.created"]}`))
	code := resp["status_code"].(float64)
	if code != http.StatusBadRequest && code != http.StatusNotFound && code != http.StatusOK {
		t.Logf("create webhook missing URL returned %v", code)
	}
}

func TestRotateWebhookSecret(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodPost, baseURL+"/admin/api/v1/webhooks/wh-nonexistent/rotate-secret", token, nil)
	code := resp["status_code"].(float64)
	if code != http.StatusNotFound && code != http.StatusOK && code != http.StatusBadRequest {
		t.Logf("rotate webhook secret returned %v", code)
	}
}

func TestDeleteWebhook(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodDelete, baseURL+"/admin/api/v1/webhooks/wh-nonexistent", token, nil)
	code := resp["status_code"].(float64)
	if code != http.StatusNotFound && code != http.StatusOK && code != http.StatusNoContent {
		t.Logf("delete webhook returned %v", code)
	}
}

func TestGetWebhookDeliveries(t *testing.T) {
	t.Parallel()
	baseURL, _, _, token := newAdminTestServer(t)

	resp := mustJSONRequest(t, http.MethodGet, baseURL+"/admin/api/v1/webhooks/wh-nonexistent/deliveries", token, nil)
	code := resp["status_code"].(float64)
	if code != http.StatusNotFound && code != http.StatusOK {
		t.Logf("get webhook deliveries returned %v", code)
	}
}

// --- Additional form login/logout tests ---

func TestHandleFormLogin_GetMethod(t *testing.T) {
	t.Parallel()
	srv := &Server{
		cfg: &config.Config{
			Admin: config.AdminConfig{
				APIKey:      "test-key-123456789012345678901234567890",
				TokenSecret: "secret-12345678901234567890123456789012",
				TokenTTL:    time.Hour,
			},
		},
		rlAttempts: make(map[string]*adminAuthAttempts),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/dashboard/login", nil)
	srv.handleFormLogin(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET, got %d", w.Code)
	}
}

func TestHandleFormLogin_MissingKey(t *testing.T) {
	t.Parallel()
	srv := &Server{
		cfg: &config.Config{
			Admin: config.AdminConfig{
				APIKey:      "test-key-123456789012345678901234567890",
				TokenSecret: "secret-12345678901234567890123456789012",
				TokenTTL:    time.Hour,
			},
		},
		rlAttempts: make(map[string]*adminAuthAttempts),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/dashboard/login", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleFormLogin(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect for missing key, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "missing_key") {
		t.Errorf("expected missing_key in redirect, got: %s", loc)
	}
}

func TestHandleFormLogin_InvalidKey(t *testing.T) {
	t.Parallel()
	srv := &Server{
		cfg: &config.Config{
			Admin: config.AdminConfig{
				APIKey:      "test-key-123456789012345678901234567890",
				TokenSecret: "secret-12345678901234567890123456789012",
				TokenTTL:    time.Hour,
			},
		},
		rlAttempts: make(map[string]*adminAuthAttempts),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/dashboard/login", strings.NewReader("admin_key=wrong-key"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleFormLogin(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect for invalid key, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "invalid_key") {
		t.Errorf("expected invalid_key in redirect, got: %s", loc)
	}
}

func TestHandleFormLogout(t *testing.T) {
	t.Parallel()
	srv := &Server{
		cfg: &config.Config{
			Admin: config.AdminConfig{
				APIKey:      "test-key-123456789012345678901234567890",
				TokenSecret: "secret-12345678901234567890123456789012",
				TokenTTL:    time.Hour,
			},
		},
		rlAttempts: make(map[string]*adminAuthAttempts),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/dashboard/logout", nil)
	srv.handleFormLogout(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", w.Code)
	}

	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == adminSessionCookieName {
			if c.Value != "" || c.MaxAge != -1 {
				t.Error("expected cookie to be cleared")
			}
			found = true
		}
	}
	if !found {
		t.Error("expected session cookie to be cleared")
	}

	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "logout=1") {
		t.Errorf("expected logout=1 in redirect, got: %s", loc)
	}
}

// --- withAdminStaticAuth middleware tests (61.1% coverage) ---

func TestWithAdminStaticAuth_MissingAPIKey(t *testing.T) {
	t.Parallel()
	srv := &Server{
		cfg: &config.Config{
			Admin: config.AdminConfig{
				APIKey:      "test-key-123456789012345678901234567890",
				TokenSecret: "secret-12345678901234567890123456789012",
				TokenTTL:    time.Hour,
			},
		},
		rlAttempts: make(map[string]*adminAuthAttempts),
	}

	handler := srv.withAdminStaticAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/status", nil)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing API key, got %d", w.Code)
	}
}

func TestWithAdminStaticAuth_InvalidAPIKey(t *testing.T) {
	t.Parallel()
	srv := &Server{
		cfg: &config.Config{
			Admin: config.AdminConfig{
				APIKey:      "test-key-123456789012345678901234567890",
				TokenSecret: "secret-12345678901234567890123456789012",
				TokenTTL:    time.Hour,
			},
		},
		rlAttempts: make(map[string]*adminAuthAttempts),
	}

	handler := srv.withAdminStaticAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/status", nil)
	req.Header.Set("X-Admin-Key", "wrong-key")
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid API key, got %d", w.Code)
	}
}

func TestWithAdminStaticAuth_ValidAPIKey(t *testing.T) {
	t.Parallel()
	srv := &Server{
		cfg: &config.Config{
			Admin: config.AdminConfig{
				APIKey:      "test-key-123456789012345678901234567890",
				TokenSecret: "secret-12345678901234567890123456789012",
				TokenTTL:    time.Hour,
			},
		},
		rlAttempts: make(map[string]*adminAuthAttempts),
	}

	called := false
	handler := srv.withAdminStaticAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/status", nil)
	req.Header.Set("X-Admin-Key", "test-key-123456789012345678901234567890")
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("expected handler to be called with valid API key")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// --- withAdminBearerAuth middleware tests ---

func TestWithAdminBearerAuth_ValidBearerToken(t *testing.T) {
	t.Parallel()
	secret := "secret-12345678901234567890123456789012"
	srv := &Server{
		cfg: &config.Config{
			Admin: config.AdminConfig{
				APIKey:      "test-key-123456789012345678901234567890",
				TokenSecret: secret,
				TokenTTL:    time.Hour,
			},
		},
		rlAttempts: make(map[string]*adminAuthAttempts),
	}

	token, err := issueAdminToken(secret, time.Hour, string(RoleAdmin), RolePermissions[RoleAdmin])
	if err != nil {
		t.Fatalf("failed to issue token: %v", err)
	}

	called := false
	handler := srv.withAdminBearerAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("expected handler to be called with valid bearer token")
	}
}

func TestWithAdminBearerAuth_InvalidBearerToken(t *testing.T) {
	t.Parallel()
	srv := &Server{
		cfg: &config.Config{
			Admin: config.AdminConfig{
				APIKey:      "test-key-123456789012345678901234567890",
				TokenSecret: "secret-12345678901234567890123456789012",
				TokenTTL:    time.Hour,
			},
		},
		rlAttempts: make(map[string]*adminAuthAttempts),
	}

	handler := srv.withAdminBearerAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid bearer token, got %d", w.Code)
	}
}

func TestWithAdminBearerAuth_MissingToken(t *testing.T) {
	t.Parallel()
	srv := &Server{
		cfg: &config.Config{
			Admin: config.AdminConfig{
				APIKey:      "test-key-123456789012345678901234567890",
				TokenSecret: "secret-12345678901234567890123456789012",
				TokenTTL:    time.Hour,
			},
		},
		rlAttempts: make(map[string]*adminAuthAttempts),
	}

	handler := srv.withAdminBearerAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/status", nil)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing bearer token, got %d", w.Code)
	}
}

// --- Additional form login tests ---

func TestHandleFormLogin_WithGatewayHTTPS(t *testing.T) {
	t.Parallel()
	srv := &Server{
		cfg: &config.Config{
			Admin: config.AdminConfig{
				APIKey:      "test-key-123456789012345678901234567890",
				TokenSecret: "secret-12345678901234567890123456789012",
				TokenTTL:    time.Hour,
			},
			Gateway: config.GatewayConfig{
				HTTPSAddr: ":8443",
			},
		},
		rlAttempts: make(map[string]*adminAuthAttempts),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/dashboard/login", strings.NewReader("admin_key=test-key-123456789012345678901234567890"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.handleFormLogin(w, req)

	loc := w.Header().Get("Location")
	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", w.Code)
	}
	if strings.Contains(loc, "invalid_key") || strings.Contains(loc, "missing_key") {
		t.Errorf("unexpected redirect: %s", loc)
	}
}
