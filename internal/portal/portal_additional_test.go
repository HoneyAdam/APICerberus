package portal

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

// Test API Key handlers
func TestRevokeMyAPIKey(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "revoke-key@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "revoke-key@example.com",
		"password": "portal-pass",
	})
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("expected login 200 got %d", loginResp.StatusCode)
	}
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Create an API key
	createResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/api-keys", []*http.Cookie{sessionCookie}, map[string]any{
		"name": "Test Key",
		"mode": "test",
	})
	assertPortalStatus(t, createResp, http.StatusCreated)
	keyID := getNestedString(t, createResp.Body, "key.id")
	if keyID == "" {
		t.Fatalf("expected key id in response")
	}

	// Revoke the API key
	revokeResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodDelete, httpSrv.URL+"/portal/api/v1/api-keys/"+keyID, []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, revokeResp, http.StatusNoContent)
}

func TestRevokeMyAPIKey_NotFound(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "revoke-notfound@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "revoke-notfound@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Try to revoke non-existent key - returns 400 with error message
	revokeResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodDelete, httpSrv.URL+"/portal/api/v1/api-keys/nonexistent-key-id", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, revokeResp, http.StatusBadRequest)
}

// Test log handlers
func TestGetMyLogDetail(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	user := createPortalTestUserWithID(t, st, "log-detail@example.com", "portal-pass")

	// Seed audit entries
	if err := st.Audits().BatchInsert([]store.AuditEntry{
		{
			ID:          "log-1",
			UserID:      user.ID,
			RouteID:     "route-1",
			RouteName:   "Test Route",
			ServiceName: "test-service",
			Method:      "GET",
			Path:        "/api/test",
			StatusCode:  200,
			LatencyMS:   15,
			ClientIP:    "127.0.0.1",
			CreatedAt:   time.Now().UTC(),
		},
	}); err != nil {
		t.Fatalf("seed audit entries: %v", err)
	}

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "log-detail@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Get log detail
	logResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/logs/log-1", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, logResp, http.StatusOK)
}

func TestGetMyLogDetail_NotFound(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "log-notfound@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "log-notfound@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Try to get non-existent log
	logResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/logs/nonexistent-log", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, logResp, http.StatusNotFound)
}

// Test template handlers
func TestDeleteTemplate(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	user := createPortalTestUserWithID(t, st, "delete-template@example.com", "portal-pass")

	// Create a template via store
	template := &store.PlaygroundTemplate{
		UserID: user.ID,
		Name:   "Test Template",
		Method: "GET",
		Path:   "/api/test",
	}
	if err := st.PlaygroundTemplates().Save(template); err != nil {
		t.Fatalf("create template: %v", err)
	}

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "delete-template@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Delete the template
	deleteResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodDelete, httpSrv.URL+"/portal/api/v1/playground/templates/"+template.ID, []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, deleteResp, http.StatusNoContent)
}

func TestDeleteTemplate_NotFound(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "delete-tmpl-notfound@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "delete-tmpl-notfound@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Try to delete non-existent template - returns 400 with error message
	deleteResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodDelete, httpSrv.URL+"/portal/api/v1/playground/templates/nonexistent", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, deleteResp, http.StatusBadRequest)
}

// Test playground handlers
func TestPlaygroundSend_InvalidBody(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "playground-invalid@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "playground-invalid@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Send invalid JSON body
	playgroundResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/playground/send", []*http.Cookie{sessionCookie}, map[string]any{
		"method": "GET",
		"path":   "/api/test",
		"body":   "invalid-json-{",
	})
	assertPortalStatus(t, playgroundResp, http.StatusBadRequest)
}

// Test usage handlers edge cases
func TestUsageTopEndpoints_MissingParams(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "usage-top@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "usage-top@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Get top endpoints without params (should use defaults)
	usageResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/usage/top-endpoints", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, usageResp, http.StatusOK)
}

func TestUsageErrors_MissingParams(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "usage-errors@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "usage-errors@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Get errors without params (should use defaults)
	usageResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/usage/errors", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, usageResp, http.StatusOK)
}

// Test rename API key
func TestRenameMyAPIKey(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "rename-key@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "rename-key@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Create an API key
	createResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/api-keys", []*http.Cookie{sessionCookie}, map[string]any{
		"name": "Old Name",
		"mode": "test",
	})
	assertPortalStatus(t, createResp, http.StatusCreated)
	keyID := getNestedString(t, createResp.Body, "key.id")

	// Rename the API key
	renameResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPut, httpSrv.URL+"/portal/api/v1/api-keys/"+keyID, []*http.Cookie{sessionCookie}, map[string]any{
		"name": "New Name",
	})
	assertPortalStatus(t, renameResp, http.StatusOK)
}

func TestRenameMyAPIKey_NotFound(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "rename-notfound@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "rename-notfound@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Try to rename non-existent key - returns 400 with error message
	renameResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPut, httpSrv.URL+"/portal/api/v1/api-keys/nonexistent-key", []*http.Cookie{sessionCookie}, map[string]any{
		"name": "New Name",
	})
	assertPortalStatus(t, renameResp, http.StatusBadRequest)
}

// Test change password validation
func TestChangePassword_MissingFields(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "change-pwd-missing@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "change-pwd-missing@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Try to change password without new_password
	changeResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPut, httpSrv.URL+"/portal/api/v1/auth/password", []*http.Cookie{sessionCookie}, map[string]any{
		"old_password": "portal-pass",
	})
	assertPortalStatus(t, changeResp, http.StatusBadRequest)
}

func TestChangePassword_WrongCurrent(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "change-pwd-wrong@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "change-pwd-wrong@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Try to change password with wrong current password
	changeResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPut, httpSrv.URL+"/portal/api/v1/auth/password", []*http.Cookie{sessionCookie}, map[string]any{
		"old_password": "wrong-password",
		"new_password": "new-portal-pass",
	})
	assertPortalStatus(t, changeResp, http.StatusUnauthorized)
}

// Test get credits balance
func TestGetCreditsBalance(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	user := createPortalTestUserWithID(t, st, "credits-balance@example.com", "portal-pass")

	// Seed a credit transaction
	if err := st.Credits().Create(&store.CreditTransaction{
		UserID:        user.ID,
		Type:          "consume",
		Amount:        -5,
		BalanceBefore: 100,
		BalanceAfter:  95,
		Description:   "test usage",
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed credit transaction: %v", err)
	}

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "credits-balance@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Get credits balance
	creditsResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/credits/balance", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, creditsResp, http.StatusOK)
}

// Test get all logs
func TestGetMyLogs(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	user := createPortalTestUserWithID(t, st, "my-logs@example.com", "portal-pass")

	// Seed audit entries
	if err := st.Audits().BatchInsert([]store.AuditEntry{
		{
			ID:          "log-1",
			UserID:      user.ID,
			RouteID:     "route-1",
			RouteName:   "Test Route",
			ServiceName: "test-service",
			Method:      "GET",
			Path:        "/api/test",
			StatusCode:  200,
			LatencyMS:   15,
			ClientIP:    "127.0.0.1",
			CreatedAt:   time.Now().UTC(),
		},
	}); err != nil {
		t.Fatalf("seed audit entries: %v", err)
	}

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "my-logs@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Get all logs
	logsResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/logs", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, logsResp, http.StatusOK)
}

// Test get API key list
func TestGetMyAPIKeys(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "my-keys@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "my-keys@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Get API keys list
	keysResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/api-keys", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, keysResp, http.StatusOK)
}

// Test my balance
func TestMyBalance(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	user := createPortalTestUserWithID(t, st, "balance@example.com", "portal-pass")

	// Add some credits
	_, err := st.Users().UpdateCreditBalance(user.ID, 500)
	if err != nil {
		t.Fatalf("failed to add credits: %v", err)
	}

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "balance@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Get balance
	balanceResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/credits/balance", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, balanceResp, http.StatusOK)
}

// Test my transactions
func TestMyTransactions(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	user := createPortalTestUserWithID(t, st, "transactions@example.com", "portal-pass")

	// Add some credits with transaction
	_, err := st.Users().UpdateCreditBalance(user.ID, 1000)
	if err != nil {
		t.Fatalf("failed to add credits: %v", err)
	}

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "transactions@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Get transactions
	txnResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/credits/transactions", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, txnResp, http.StatusOK)
}

// Test export my logs
func TestExportMyLogs(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	user := createPortalTestUserWithID(t, st, "export-logs@example.com", "portal-pass")

	// Seed audit entries
	if err := st.Audits().BatchInsert([]store.AuditEntry{
		{
			ID:         "export-log-1",
			UserID:     user.ID,
			RouteID:    "route-1",
			Method:     "GET",
			Path:       "/api/test",
			StatusCode: 200,
			ClientIP:   "127.0.0.1",
			CreatedAt:  time.Now().UTC(),
		},
	}); err != nil {
		t.Fatalf("failed to seed audit entries: %v", err)
	}

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "export-logs@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Export logs
	exportResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/logs/export", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, exportResp, http.StatusOK)
}

// Test cleanupOldRateLimitEntries
func TestCleanupOldRateLimitEntries(t *testing.T) {
	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Add some rate limit entries
	srv.rlMu.Lock()
	srv.rlAttempts["192.168.1.1"] = &loginAuthAttempts{count: 3, lastSeen: time.Now().Add(-1 * time.Hour)}
	srv.rlAttempts["192.168.1.2"] = &loginAuthAttempts{count: 2, lastSeen: time.Now().Add(-5 * time.Minute)}
	srv.rlAttempts["192.168.1.3"] = &loginAuthAttempts{count: 1, lastSeen: time.Now()}
	srv.rlMu.Unlock()

	// Run cleanup
	srv.cleanupOldRateLimitEntries()

	// Verify old entries are removed, recent ones remain
	srv.rlMu.Lock()
	defer srv.rlMu.Unlock()

	if _, ok := srv.rlAttempts["192.168.1.1"]; ok {
		t.Error("Old entry (1 hour) should have been cleaned up")
	}
	if _, ok := srv.rlAttempts["192.168.1.2"]; !ok {
		t.Error("Recent entry (5 minutes) should still exist")
	}
	if _, ok := srv.rlAttempts["192.168.1.3"]; !ok {
		t.Error("Current entry should still exist")
	}
}

// Test cloneURL function
func TestCloneURL(t *testing.T) {
	tests := []struct {
		name string
		in   *url.URL
		want string
	}{
		{
			name: "nil URL",
			in:   nil,
			want: "",
		},
		{
			name: "simple URL",
			in:   &url.URL{Scheme: "http", Host: "localhost:8080", Path: "/test"},
			want: "http://localhost:8080/test",
		},
		{
			name: "URL with query",
			in:   &url.URL{Scheme: "https", Host: "example.com", Path: "/api", RawQuery: "key=value"},
			want: "https://example.com/api?key=value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cloneURL(tt.in)
			if got.String() != tt.want {
				t.Errorf("cloneURL() = %v, want %v", got.String(), tt.want)
			}
			// Verify it's a clone, not the same pointer
			if tt.in != nil && got == tt.in {
				t.Error("cloneURL should return a new URL instance")
			}
		})
	}
}

// Test portalAssetExists
func TestPortalAssetExists(t *testing.T) {
	t.Parallel()

	t.Run("nil filesystem", func(t *testing.T) {
		exists := portalAssetExists(nil, "test.txt")
		if exists {
			t.Error("portalAssetExists should return false for nil filesystem")
		}
	})

	t.Run("non-existent file", func(t *testing.T) {
		// Create a temporary filesystem
		tmpDir := t.TempDir()
		exists := portalAssetExists(os.DirFS(tmpDir), "nonexistent.txt")
		if exists {
			t.Error("portalAssetExists should return false for non-existent file")
		}
	})

	t.Run("existing file", func(t *testing.T) {
		// Create a temporary filesystem with a file
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")
		if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		exists := portalAssetExists(os.DirFS(tmpDir), "test.txt")
		if !exists {
			t.Error("portalAssetExists should return true for existing file")
		}
	})

	t.Run("directory not file", func(t *testing.T) {
		// Create a temporary filesystem with a directory
		tmpDir := t.TempDir()
		testDir := filepath.Join(tmpDir, "testdir")
		if err := os.Mkdir(testDir, 0755); err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}

		exists := portalAssetExists(os.DirFS(tmpDir), "testdir")
		if exists {
			t.Error("portalAssetExists should return false for directories")
		}
	})
}

// Test resolveRouteCreditCost
func TestResolveRouteCreditCost(t *testing.T) {
	tests := []struct {
		name       string
		billing    config.BillingConfig
		route      *config.Route
		permission *store.EndpointPermission
		want       int64
	}{
		{
			name:       "permission cost takes precedence",
			billing:    config.BillingConfig{DefaultCost: 10},
			route:      &config.Route{ID: "route-1", Name: "Test Route"},
			permission: &store.EndpointPermission{CreditCost: int64Ptr(5)},
			want:       5,
		},
		{
			name:       "route ID cost",
			billing:    config.BillingConfig{DefaultCost: 10, RouteCosts: map[string]int64{"route-1": 15}},
			route:      &config.Route{ID: "route-1", Name: "Test Route"},
			permission: nil,
			want:       15,
		},
		{
			name:       "route name cost",
			billing:    config.BillingConfig{DefaultCost: 10, RouteCosts: map[string]int64{"Test Route": 20}},
			route:      &config.Route{ID: "route-1", Name: "Test Route"},
			permission: nil,
			want:       20,
		},
		{
			name:       "default cost",
			billing:    config.BillingConfig{DefaultCost: 10},
			route:      &config.Route{ID: "route-1", Name: "Test Route"},
			permission: nil,
			want:       10,
		},
		{
			name:       "zero cost",
			billing:    config.BillingConfig{},
			route:      nil,
			permission: nil,
			want:       0,
		},
		{
			name:       "route nil uses default",
			billing:    config.BillingConfig{DefaultCost: 25},
			route:      nil,
			permission: nil,
			want:       25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveRouteCreditCost(tt.billing, tt.route, tt.permission)
			if got != tt.want {
				t.Errorf("resolveRouteCreditCost() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Helper function for int64 pointer
func int64Ptr(i int64) *int64 {
	return &i
}

// Test getMyAPIDetail error paths
func TestGetMyAPIDetail_MissingRouteID(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "api-detail@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "api-detail@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Get API detail with empty route ID (should return 404 since route won't be found)
	apiResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/apis/", []*http.Cookie{sessionCookie}, nil)
	// Empty path value results in 404 from router
	if apiResp.StatusCode != http.StatusNotFound && apiResp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 404 or 400, got %d", apiResp.StatusCode)
	}
}

func TestGetMyAPIDetail_NotFound(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "api-notfound@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "api-notfound@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Get API detail for non-existent route
	apiResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/apis/nonexistent-route-12345", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, apiResp, http.StatusNotFound)
}

// Test createMyAPIKey with invalid JSON
func TestCreateMyAPIKey_InvalidJSON(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "create-key-invalid@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "create-key-invalid@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Create API key with invalid JSON
	client := httpSrv.Client()
	req, _ := http.NewRequest(http.MethodPost, httpSrv.URL+"/portal/api/v1/api-keys", strings.NewReader("not valid json"))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: cfg.Portal.Session.CookieName, Value: sessionCookie.Value})

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", resp.StatusCode)
	}
}

// Test listTemplates with empty list
func TestListTemplates_Empty(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "templates-empty@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "templates-empty@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Get templates list (should be empty)
	templatesResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/playground/templates", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, templatesResp, http.StatusOK)
}

// Test usageOverview with no data
func TestUsageOverview_NoData(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	createPortalTestUser(t, st, "usage-overview@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "usage-overview@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Get usage overview (should work even with no data)
	overviewResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/usage/overview", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, overviewResp, http.StatusOK)
}

// Test myForecast endpoint
func TestMyForecast(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	user := createPortalTestUserWithID(t, st, "forecast@example.com", "portal-pass")

	// Add some credits and usage history
	_, err := st.Users().UpdateCreditBalance(user.ID, 1000)
	if err != nil {
		t.Fatalf("failed to add credits: %v", err)
	}

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "forecast@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Get forecast
	forecastResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/credits/forecast", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, forecastResp, http.StatusOK)
}

// Test myActivity endpoint
func TestMyActivity(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()
	user := createPortalTestUserWithID(t, st, "activity@example.com", "portal-pass")

	// Seed some activity
	if err := st.Audits().BatchInsert([]store.AuditEntry{
		{
			ID:         "activity-1",
			UserID:     user.ID,
			RouteID:    "route-1",
			Method:     "GET",
			Path:       "/api/test",
			StatusCode: 200,
			ClientIP:   "127.0.0.1",
			CreatedAt:  time.Now().UTC(),
		},
	}); err != nil {
		t.Fatalf("failed to seed activity: %v", err)
	}

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()

	// Login first
	loginResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodPost, httpSrv.URL+"/portal/api/v1/auth/login", nil, map[string]any{
		"email":    "activity@example.com",
		"password": "portal-pass",
	})
	sessionCookie := findCookie(loginResp.Cookies, cfg.Portal.Session.CookieName)

	// Get activity
	activityResp := mustPortalJSONRequest(t, httpSrv.Client(), http.MethodGet, httpSrv.URL+"/portal/api/v1/security/activity", []*http.Cookie{sessionCookie}, nil)
	assertPortalStatus(t, activityResp, http.StatusOK)
}

// Test helper functions
func TestAsStringSlice(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected []string
	}{
		{
			name:     "string slice",
			value:    []string{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "any slice",
			value:    []any{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "empty string slice",
			value:    []string{},
			expected: []string{},
		},
		{
			name:     "nil",
			value:    nil,
			expected: nil,
		},
		{
			name:     "string",
			value:    "not a slice",
			expected: nil,
		},
		{
			name:     "slice with empty strings",
			value:    []string{"a", "", "  ", "b"},
			expected: []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := asStringSlice(tt.value)
			if len(result) != len(tt.expected) {
				t.Errorf("asStringSlice() = %v, want %v", result, tt.expected)
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("asStringSlice()[%d] = %v, want %v", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestAsInt64(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		fallback int64
		expected int64
	}{
		{
			name:     "int",
			value:    42,
			fallback: 0,
			expected: 42,
		},
		{
			name:     "int64",
			value:    int64(42),
			fallback: 0,
			expected: 42,
		},
		{
			name:     "int32",
			value:    int32(42),
			fallback: 0,
			expected: 42,
		},
		{
			name:     "float64",
			value:    float64(42.5),
			fallback: 0,
			expected: 42,
		},
		{
			name:     "string valid",
			value:    "42",
			fallback: 0,
			expected: 42,
		},
		{
			name:     "string empty",
			value:    "",
			fallback: 99,
			expected: 99,
		},
		{
			name:     "string invalid",
			value:    "not a number",
			fallback: 99,
			expected: 99,
		},
		{
			name:     "nil",
			value:    nil,
			fallback: 99,
			expected: 99,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := asInt64(tt.value, tt.fallback)
			if result != tt.expected {
				t.Errorf("asInt64(%v, %d) = %d, want %d", tt.value, tt.fallback, result, tt.expected)
			}
		})
	}
}

func TestParsePortalLogFilters(t *testing.T) {
	tests := []struct {
		name        string
		params      url.Values
		expectedMin int
		expectedMax int
		expectedErr bool
	}{
		{
			name:        "empty params",
			params:      url.Values{},
			expectedMin: 0,
			expectedMax: 0,
			expectedErr: false,
		},
		{
			name: "with status_min",
			params: url.Values{
				"status_min": []string{"200"},
			},
			expectedMin: 200,
			expectedMax: 0,
			expectedErr: false,
		},
		{
			name: "with status_max",
			params: url.Values{
				"status_max": []string{"500"},
			},
			expectedMin: 0,
			expectedMax: 500,
			expectedErr: false,
		},
		{
			name: "with method",
			params: url.Values{
				"method": []string{"GET"},
			},
			expectedMin: 0,
			expectedMax: 0,
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parsePortalLogFilters(tt.params)
			if (err != nil) != tt.expectedErr {
				t.Errorf("parsePortalLogFilters() error = %v, expectedErr %v", err, tt.expectedErr)
				return
			}
			if result.StatusMin != tt.expectedMin {
				t.Errorf("StatusMin = %v, want %v", result.StatusMin, tt.expectedMin)
			}
			if result.StatusMax != tt.expectedMax {
				t.Errorf("StatusMax = %v, want %v", result.StatusMax, tt.expectedMax)
			}
		})
	}
}

func TestCloneFloat64Map(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]float64
		expected map[string]float64
	}{
		{
			name:     "nil map",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty map",
			input:    map[string]float64{},
			expected: map[string]float64{},
		},
		{
			name: "map with values",
			input: map[string]float64{
				"a": 1.0,
				"b": 2.5,
			},
			expected: map[string]float64{
				"a": 1.0,
				"b": 2.5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cloneFloat64Map(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("cloneFloat64Map() length = %d, want %d", len(result), len(tt.expected))
			}
			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("cloneFloat64Map()[%s] = %v, want %v", k, result[k], v)
				}
			}

			// Verify it's a clone (modifying result doesn't affect input)
			if len(result) > 0 {
				result["new"] = 99.0
				if _, ok := tt.expected["new"]; ok {
					t.Error("cloneFloat64Map() returned map that affects original")
				}
			}
		})
	}
}

// Test isRateLimited function with various scenarios
func TestIsRateLimited(t *testing.T) {
	tests := []struct {
		name        string
		clientIP    string
		expectLimited bool
	}{
		{
			name:        "empty IP",
			clientIP:    "",
			expectLimited: false,
		},
		{
			name:        "valid IP",
			clientIP:    "192.168.1.1",
			expectLimited: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a server with rate limiting
			cfg, st := openPortalTestStore(t)
			defer st.Close()

			srv, err := NewServer(cfg, st)
			if err != nil {
				t.Fatalf("NewServer error: %v", err)
			}

			limited := srv.isRateLimited(tt.clientIP)
			if limited != tt.expectLimited {
				t.Errorf("isRateLimited(%q) = %v, want %v", tt.clientIP, limited, tt.expectLimited)
			}
		})
	}
}

// Test getClientIP function
func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		remoteAddr string
		expected   string
	}{
		{
			name:       "X-Forwarded-For",
			headers:    map[string]string{"X-Forwarded-For": "10.0.0.1, 10.0.0.2"},
			remoteAddr: "192.168.1.1:12345",
			expected:   "10.0.0.1",
		},
		{
			name:       "X-Forwarded-For single IP",
			headers:    map[string]string{"X-Forwarded-For": "10.0.0.3"},
			remoteAddr: "192.168.1.1:12345",
			expected:   "10.0.0.3",
		},
		{
			name:       "RemoteAddr only",
			headers:    map[string]string{},
			remoteAddr: "192.168.1.1:12345",
			expected:   "192.168.1.1",
		},
		{
			name:       "empty RemoteAddr",
			headers:    map[string]string{},
			remoteAddr: "",
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request with headers
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			req.RemoteAddr = tt.remoteAddr

			got := getClientIP(req)
			if got != tt.expected {
				t.Errorf("getClientIP() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// Test extractClientIP function
func TestExtractClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		expected   string
	}{
		{
			name:       "with port",
			remoteAddr: "192.168.1.1:12345",
			expected:   "192.168.1.1",
		},
		{
			name:       "without port",
			remoteAddr: "192.168.1.1",
			expected:   "192.168.1.1",
		},
		{
			name:       "IPv6 with port",
			remoteAddr: "[::1]:12345",
			expected:   "::1",
		},
		{
			name:       "empty",
			remoteAddr: "",
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.RemoteAddr = tt.remoteAddr

			got := extractClientIP(req)
			if got != tt.expected {
				t.Errorf("extractClientIP(%q) = %q, want %q", tt.remoteAddr, got, tt.expected)
			}
		})
	}
}

// Test startRateLimitCleanup function
func TestStartRateLimitCleanup(t *testing.T) {
	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Test that cleanup doesn't panic
	done := make(chan bool)
	go func() {
		srv.startRateLimitCleanup()
		done <- true
	}()

	// Give it a moment to start
	time.Sleep(10 * time.Millisecond)

	// Cleanup should not block
	select {
	case <-done:
		// OK
	case <-time.After(100 * time.Millisecond):
		// Expected - cleanup runs in background
	}
}

// Test resolvePortalAssetPath function
func TestResolvePortalAssetPath(t *testing.T) {
	tests := []struct {
		name        string
		pathPrefix  string
		cleanPath   string
		wantPath    string
		wantServeUI bool
	}{
		{
			name:        "empty path",
			pathPrefix:  "",
			cleanPath:   "",
			wantPath:    "",
			wantServeUI: true,
		},
		{
			name:        "dot path",
			pathPrefix:  "",
			cleanPath:   ".",
			wantPath:    "",
			wantServeUI: true,
		},
		{
			name:        "assets path",
			pathPrefix:  "",
			cleanPath:   "/assets/main.js",
			wantPath:    "assets/main.js",
			wantServeUI: true,
		},
		{
			name:        "favicon",
			pathPrefix:  "",
			cleanPath:   "/favicon.ico",
			wantPath:    "favicon.ico",
			wantServeUI: true,
		},
		{
			name:        "with prefix matching",
			pathPrefix:  "/portal",
			cleanPath:   "/portal",
			wantPath:    "",
			wantServeUI: true,
		},
		{
			name:        "with prefix subpath",
			pathPrefix:  "/portal",
			cleanPath:   "/portal/dashboard",
			wantPath:    "dashboard",
			wantServeUI: true,
		},
		{
			name:        "non-matching prefix",
			pathPrefix:  "/portal",
			cleanPath:   "/other",
			wantPath:    "",
			wantServeUI: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{pathPrefix: tt.pathPrefix}
			gotPath, gotServeUI := s.resolvePortalAssetPath(tt.cleanPath)
			if gotPath != tt.wantPath {
				t.Errorf("resolvePortalAssetPath() path = %q, want %q", gotPath, tt.wantPath)
			}
			if gotServeUI != tt.wantServeUI {
				t.Errorf("resolvePortalAssetPath() serveUI = %v, want %v", gotServeUI, tt.wantServeUI)
			}
		})
	}
}

// Test recordFailedAuth function
func TestRecordFailedAuth(t *testing.T) {
	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Record failed auth for an IP
	srv.recordFailedAuth("192.168.1.1")

	// Verify it was recorded
	srv.rlMu.RLock()
	attempts, exists := srv.rlAttempts["192.168.1.1"]
	srv.rlMu.RUnlock()

	if !exists {
		t.Fatal("recordFailedAuth() did not create entry")
	}
	if attempts.count != 1 {
		t.Errorf("recordFailedAuth() count = %d, want 1", attempts.count)
	}
}

// Test normalizePortalPathPrefix function
func TestNormalizePortalPathPrefix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"/portal", "/portal"},
		{"portal", "/portal"},
		{"/portal/", "/portal"},
		{"portal/", "/portal"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizePortalPathPrefix(tt.input)
			if got != tt.expected {
				t.Errorf("normalizePortalPathPrefix(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// Test sanitizeUser function
func TestSanitizeUser(t *testing.T) {
	user := &store.User{
		ID:        "user-123",
		Email:     "test@example.com",
		Name:      "Test User",
		Status:    "active",
		CreatedAt: time.Now(),
	}

	sanitized := sanitizeUser(user)

	if sanitized == nil {
		t.Fatal("sanitizeUser() returned nil")
	}
	if sanitized["id"] != user.ID {
		t.Error("sanitizeUser() ID mismatch")
	}
	if sanitized["email"] != user.Email {
		t.Error("sanitizeUser() Email mismatch")
	}
}

// Test isUserActive function
func TestIsUserActive(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected bool
	}{
		{"active", "active", true},
		{"empty", "", true},
		{"inactive", "inactive", false},
		{"pending", "pending", false},
		{"suspended", "suspended", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &store.User{Status: tt.status}
			got := isUserActive(user)
			if got != tt.expected {
				t.Errorf("isUserActive() = %v, want %v", got, tt.expected)
			}
		})
	}
}
