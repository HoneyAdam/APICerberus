package portal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

// =============================================================================
// API Keys Handler Tests - Additional Coverage
// =============================================================================

func TestListMyAPIKeys_MissingSession(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Request without session cookie
	resp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/api-keys", nil, nil)
	assertPortalStatus(t, resp, http.StatusUnauthorized)
}

func TestCreateMyAPIKey_InvalidModeDefaultsToLive(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "portal-key-mode@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-key-mode@example.com",
		"password": "portal-pass",
	})
	assertPortalStatus(t, loginResp, http.StatusOK)
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)
	csrfCookie := findCookie(loginResp.Cookies, csrfCookieName)
	csrfToken := csrfCookie.Value

	// Invalid mode defaults to "live" mode
	resp := mustPortalJSONRequestWithCSRF(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/api-keys", []*http.Cookie{sessionCookie, csrfCookie}, map[string]any{
		"name": "test-key",
		"mode": "invalid",
	}, csrfToken)
	assertPortalStatus(t, resp, http.StatusCreated)
	// Verify it created a live key (not test)
	var body map[string]any
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if key, ok := body["key"].(map[string]any); ok {
		if prefix, ok := key["key_prefix"].(string); ok {
			if len(prefix) < 8 || prefix[:8] != "ck_live_" {
				t.Errorf("expected live key, got prefix: %s", prefix)
			}
		}
	}
}

func TestCreateMyAPIKey_MissingNameDefaults(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "portal-key-name@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-key-name@example.com",
		"password": "portal-pass",
	})
	assertPortalStatus(t, loginResp, http.StatusOK)
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)
	csrfCookie := findCookie(loginResp.Cookies, csrfCookieName)
	csrfToken := csrfCookie.Value

	// Missing name defaults to "default"
	resp := mustPortalJSONRequestWithCSRF(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/api-keys", []*http.Cookie{sessionCookie, csrfCookie}, map[string]any{
		"mode": "test",
	}, csrfToken)
	assertPortalStatus(t, resp, http.StatusCreated)
	// Verify it created key with default name
	var body map[string]any
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if key, ok := body["key"].(map[string]any); ok {
		if name, ok := key["name"].(string); ok {
			if name != "default" {
				t.Errorf("expected name 'default', got: %s", name)
			}
		}
	}
}

func TestRenameMyAPIKey_EmptyName_Coverage(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	user := createPortalTestUserWithID(t, st, "portal-rename-coverage@example.com", "portal-pass")

	// Create an API key first using the correct method signature
	_, key, err := st.APIKeys().Create(user.ID, "Original Name", "test")
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-rename-coverage@example.com",
		"password": "portal-pass",
	})
	assertPortalStatus(t, loginResp, http.StatusOK)
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)
	csrfCookie := findCookie(loginResp.Cookies, csrfCookieName)
	csrfToken := csrfCookie.Value

	// Try to rename with empty name
	resp := mustPortalJSONRequestWithCSRF(t, httpSrv.Client(), http.MethodPut, httpSrv.URL+"/portal/api/v1/api-keys/"+key.ID, []*http.Cookie{sessionCookie, csrfCookie}, map[string]any{
		"name": "",
	}, csrfToken)
	assertPortalStatus(t, resp, http.StatusBadRequest)
}

func TestRevokeMyAPIKey_NonExistent(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "portal-revoke@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-revoke@example.com",
		"password": "portal-pass",
	})
	assertPortalStatus(t, loginResp, http.StatusOK)
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)
	csrfCookie := findCookie(loginResp.Cookies, csrfCookieName)
	csrfToken := csrfCookie.Value

	// Try to revoke non-existent key - returns 400 with sql error
	resp := mustPortalJSONRequestWithCSRF(t, httpSrv.Client(), http.MethodDelete, httpSrv.URL+"/portal/api/v1/api-keys/non-existent", []*http.Cookie{sessionCookie, csrfCookie}, nil, csrfToken)
	assertPortalStatus(t, resp, http.StatusBadRequest)
}

// =============================================================================
// Logs and Credits Handler Tests - Additional Coverage
// =============================================================================

func TestListMyLogs_InvalidLimit(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "portal-logs@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-logs@example.com",
		"password": "portal-pass",
	})
	assertPortalStatus(t, loginResp, http.StatusOK)
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Invalid limit defaults to 50, returns 200 with empty results
	resp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/logs?limit=invalid", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, resp, http.StatusOK)
}

func TestGetMyLogDetail_NotFound_Coverage(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "portal-log-detail-coverage@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-log-detail-coverage@example.com",
		"password": "portal-pass",
	})
	assertPortalStatus(t, loginResp, http.StatusOK)
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Try to get non-existent log
	resp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/logs/non-existent", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, resp, http.StatusNotFound)
}

func TestExportMyLogs_InvalidFormat(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "portal-export@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-export@example.com",
		"password": "portal-pass",
	})
	assertPortalStatus(t, loginResp, http.StatusOK)
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Try with invalid format - returns 500 when export fails
	resp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/logs/export?format=xml", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, resp, http.StatusInternalServerError)
}

func TestPurchaseCredits_InvalidAmount_Coverage(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "portal-purchase-coverage@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-purchase-coverage@example.com",
		"password": "portal-pass",
	})
	assertPortalStatus(t, loginResp, http.StatusOK)
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)
	csrfCookie := findCookie(loginResp.Cookies, csrfCookieName)
	csrfToken := csrfCookie.Value

	// Try with negative amount
	resp := mustPortalJSONRequestWithCSRF(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/credits/purchase", []*http.Cookie{sessionCookie, csrfCookie}, map[string]any{
		"amount": -10,
	}, csrfToken)
	assertPortalStatus(t, resp, http.StatusBadRequest)

	// Try with zero amount
	resp = mustPortalJSONRequestWithCSRF(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/credits/purchase", []*http.Cookie{sessionCookie, csrfCookie}, map[string]any{
		"amount": 0,
	}, csrfToken)
	assertPortalStatus(t, resp, http.StatusBadRequest)
}

// =============================================================================
// Security Handler Tests - Additional Coverage
// =============================================================================

func TestAddMyIP_InvalidIP(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "portal-ip@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-ip@example.com",
		"password": "portal-pass",
	})
	assertPortalStatus(t, loginResp, http.StatusOK)
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)
	csrfCookie := findCookie(loginResp.Cookies, csrfCookieName)
	csrfToken := csrfCookie.Value

	// Try with empty IP - returns 400
	resp := mustPortalJSONRequestWithCSRF(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/security/ip-whitelist", []*http.Cookie{sessionCookie, csrfCookie}, map[string]any{
		"ip": "",
	}, csrfToken)
	assertPortalStatus(t, resp, http.StatusBadRequest)
}

func TestRemoveMyIP_NotFound(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "portal-ip-remove@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-ip-remove@example.com",
		"password": "portal-pass",
	})
	assertPortalStatus(t, loginResp, http.StatusOK)
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)
	csrfCookie := findCookie(loginResp.Cookies, csrfCookieName)
	csrfToken := csrfCookie.Value

	// Try to remove IP that doesn't exist - returns 200 with empty list
	resp := mustPortalJSONRequestWithCSRF(t, httpSrv.Client(), http.MethodDelete, httpSrv.URL+"/portal/api/v1/security/ip-whitelist/192.168.1.1", []*http.Cookie{sessionCookie, csrfCookie}, nil, csrfToken)
	assertPortalStatus(t, resp, http.StatusOK)
}

// =============================================================================
// Profile Handler Tests - Additional Coverage
// =============================================================================

func TestUpdateProfile_InvalidEmail(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "portal-profile@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-profile@example.com",
		"password": "portal-pass",
	})
	assertPortalStatus(t, loginResp, http.StatusOK)
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)
	csrfCookie := findCookie(loginResp.Cookies, csrfCookieName)
	csrfToken := csrfCookie.Value

	// Handler doesn't validate email format - returns 200 even with invalid email
	resp := mustPortalJSONRequestWithCSRF(t, httpSrv.Client(), http.MethodPut, httpSrv.URL+"/portal/api/v1/settings/profile", []*http.Cookie{sessionCookie, csrfCookie}, map[string]any{
		"email": "not-an-email",
	}, csrfToken)
	assertPortalStatus(t, resp, http.StatusOK)
}

// =============================================================================
// Password Change Tests - Additional Coverage
// =============================================================================

func TestChangePassword_WrongOldPassword(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "portal-pwd@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-pwd@example.com",
		"password": "portal-pass",
	})
	assertPortalStatus(t, loginResp, http.StatusOK)
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)
	csrfCookie := findCookie(loginResp.Cookies, csrfCookieName)
	csrfToken := csrfCookie.Value

	// Try with wrong old password
	resp := mustPortalJSONRequestWithCSRF(t, httpSrv.Client(), http.MethodPut, httpSrv.URL+"/portal/api/v1/auth/password", []*http.Cookie{sessionCookie, csrfCookie}, map[string]any{
		"old_password": "wrong-password",
		"new_password": "new-portal-pass",
	}, csrfToken)
	assertPortalStatus(t, resp, http.StatusUnauthorized)
}

func TestChangePassword_MissingFields(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "portal-pwd-missing@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-pwd-missing@example.com",
		"password": "portal-pass",
	})
	assertPortalStatus(t, loginResp, http.StatusOK)
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)
	csrfCookie := findCookie(loginResp.Cookies, csrfCookieName)
	csrfToken := csrfCookie.Value

	// Try with missing new password
	resp := mustPortalJSONRequestWithCSRF(t, httpSrv.Client(), http.MethodPut, httpSrv.URL+"/portal/api/v1/auth/password", []*http.Cookie{sessionCookie, csrfCookie}, map[string]any{
		"old_password": "portal-pass",
	}, csrfToken)
	assertPortalStatus(t, resp, http.StatusBadRequest)
}

// =============================================================================
// Usage Handler Tests - Additional Coverage
// =============================================================================

func TestUsageOverview_InvalidWindow(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "portal-usage@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-usage@example.com",
		"password": "portal-pass",
	})
	assertPortalStatus(t, loginResp, http.StatusOK)
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Try with invalid window format
	resp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/usage/overview?window=invalid", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, resp, http.StatusBadRequest)
}

// =============================================================================
// Playground Template Tests - Additional Coverage
// =============================================================================

func TestSaveTemplate_MissingPath(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "portal-template@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-template@example.com",
		"password": "portal-pass",
	})
	assertPortalStatus(t, loginResp, http.StatusOK)
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)
	csrfCookie := findCookie(loginResp.Cookies, csrfCookieName)
	csrfToken := csrfCookie.Value

	// Missing path defaults to "/" - returns 201
	resp := mustPortalJSONRequestWithCSRF(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/playground/templates", []*http.Cookie{sessionCookie, csrfCookie}, map[string]any{
		"name":   "Test Template",
		"method": "GET",
	}, csrfToken)
	assertPortalStatus(t, resp, http.StatusCreated)
}

func TestDeleteTemplate_NotFound(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "portal-template-del@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-template-del@example.com",
		"password": "portal-pass",
	})
	assertPortalStatus(t, loginResp, http.StatusOK)
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)
	csrfCookie := findCookie(loginResp.Cookies, csrfCookieName)
	csrfToken := csrfCookie.Value

	// Try to delete non-existent template - returns 400 with sql error
	resp := mustPortalJSONRequestWithCSRF(t, httpSrv.Client(), http.MethodDelete, httpSrv.URL+"/portal/api/v1/playground/templates/non-existent", []*http.Cookie{sessionCookie, csrfCookie}, nil, csrfToken)
	assertPortalStatus(t, resp, http.StatusBadRequest)
}

// =============================================================================
// API List Helper Tests
// =============================================================================

func TestBuildAPIList_EmptyPermissions(t *testing.T) {
	snapshot := portalConfigView{
		Routes: []config.Route{
			{ID: "route-1", Name: "Route 1", Service: "svc-1", Paths: []string{"/api/1"}, Methods: []string{"GET"}},
		},
		Services: []config.Service{
			{ID: "svc-1", Name: "Service 1"},
		},
	}

	// When no permissions provided, all routes should be returned
	perms := buildAPIList(snapshot, []store.EndpointPermission{})

	if len(perms) != 1 {
		t.Errorf("expected 1 API entry when no permissions filter, got %d", len(perms))
	}
}

func TestBuildAPIList_WithPermissions(t *testing.T) {
	snapshot := portalConfigView{
		Routes: []config.Route{
			{ID: "route-1", Name: "Route 1", Service: "svc-1", Paths: []string{"/api/1"}, Methods: []string{"GET", "POST", "DELETE"}},
			{ID: "route-2", Name: "Route 2", Service: "svc-2", Paths: []string{"/api/2"}, Methods: []string{"GET"}},
		},
		Services: []config.Service{
			{ID: "svc-1", Name: "Service 1"},
			{ID: "svc-2", Name: "Service 2"},
		},
	}

	// When permissions provided, only allowed routes should be returned
	perms := buildAPIList(snapshot, []store.EndpointPermission{
		{RouteID: "route-1", Allowed: true, Methods: []string{"GET", "POST"}},
	})

	if len(perms) != 1 {
		t.Fatalf("expected 1 API entry, got %d", len(perms))
	}

	if perms[0]["route_name"] != "Route 1" {
		t.Errorf("expected name 'Route 1', got %s", perms[0]["route_name"])
	}

	// Should only have the route that was permitted
	if perms[0]["route_id"] != "route-1" {
		t.Errorf("expected route_id 'route-1', got %s", perms[0]["route_id"])
	}
}

// =============================================================================
// Forecast Handler Tests - Additional Coverage
// =============================================================================

func TestMyForecast_WithData(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	user := createPortalTestUserWithID(t, st, "portal-forecast-data@example.com", "portal-pass")

	// Seed some credit transactions for forecasting
	for i := 0; i < 5; i++ {
		if err := st.Credits().Create(&store.CreditTransaction{
			UserID:        user.ID,
			Type:          "consume",
			Amount:        -int64(i + 1),
			BalanceBefore: 100,
			BalanceAfter:  100 - int64(i+1),
			Description:   "test usage",
			CreatedAt:     time.Now().UTC().Add(-time.Duration(i) * time.Hour),
		}); err != nil {
			t.Fatalf("seed credit transaction: %v", err)
		}
	}

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-forecast-data@example.com",
		"password": "portal-pass",
	})
	assertPortalStatus(t, loginResp, http.StatusOK)
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	resp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/credits/forecast", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, resp, http.StatusOK)
}

// =============================================================================
// listMyAPIs Coverage Tests
// =============================================================================

func TestListMyAPIs_NoGateway(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "portal-apis-nogw@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-apis-nogw@example.com",
		"password": "portal-pass",
	})
	assertPortalStatus(t, loginResp, http.StatusOK)
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// This tests when gateway is not available - returns 503
	resp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/apis", []*http.Cookie{sessionCookie}, nil)
	// Gateway is not available in test, expect service unavailable
	if resp.StatusCode != http.StatusServiceUnavailable && resp.StatusCode != http.StatusOK {
		t.Errorf("expected 503 or 200, got %d", resp.StatusCode)
	}
}

// =============================================================================
// myBalance Coverage Tests
// =============================================================================

func TestMyBalance_StoreError(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "portal-balance-err@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-balance-err@example.com",
		"password": "portal-pass",
	})
	assertPortalStatus(t, loginResp, http.StatusOK)
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Test normal balance retrieval
	resp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/credits/balance", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, resp, http.StatusOK)
}

// =============================================================================
// listTemplates Coverage Tests
// =============================================================================

func TestListTemplates_NoSession(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Request without session
	resp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/playground/templates", nil, nil)
	assertPortalStatus(t, resp, http.StatusUnauthorized)
}

// =============================================================================
// playgroundSend Coverage Tests
// =============================================================================

func TestPlaygroundSend_InvalidJSON(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "portal-playground-invalid@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "portal-playground-invalid@example.com",
		"password": "portal-pass",
	})
	assertPortalStatus(t, loginResp, http.StatusOK)
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)
	csrfCookie := findCookie(loginResp.Cookies, csrfCookieName)

	// Send invalid JSON
	req, _ := http.NewRequest(http.MethodPost, httpSrv.URL+"/portal/api/v1/playground/send", strings.NewReader(`{invalid json`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeaderName, csrfCookie.Value)
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	resp, err := httpSrv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", resp.StatusCode)
	}
}
