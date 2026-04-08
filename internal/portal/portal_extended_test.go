package portal

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/store"
)

// =============================================================================
// Additional Handler Tests
// =============================================================================

func TestListMyLogs_WithFilters(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	user := createPortalTestUserWithID(t, st, "logs-test@example.com", "portal-pass")

	// Seed audit logs
	if err := st.Audits().BatchInsert([]store.AuditEntry{
		{
			ID:          "audit-1",
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
		t.Fatalf("seed audit logs: %v", err)
	}

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Test with user context
	ctx := context.WithValue(context.Background(), contextUserKey, user)
	req := httptest.NewRequest(http.MethodGet, "/portal/api/v1/logs?limit=10", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	srv.listMyLogs(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetMyLogDetail_NotFound(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	user := createPortalTestUserWithID(t, st, "log-detail@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Test with user context but non-existent log
	ctx := context.WithValue(context.Background(), contextUserKey, user)
	req := httptest.NewRequest(http.MethodGet, "/portal/api/v1/logs/non-existent", nil).WithContext(ctx)
	req.SetPathValue("id", "non-existent")
	w := httptest.NewRecorder()

	srv.getMyLogDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestExportMyLogs(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	user := createPortalTestUserWithID(t, st, "export-logs@example.com", "portal-pass")

	// Seed audit logs
	if err := st.Audits().BatchInsert([]store.AuditEntry{
		{
			ID:          "audit-export-1",
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
		t.Fatalf("seed audit logs: %v", err)
	}

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Test export
	ctx := context.WithValue(context.Background(), contextUserKey, user)
	req := httptest.NewRequest(http.MethodGet, "/portal/api/v1/logs/export?format=json", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	srv.exportMyLogs(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("expected JSON content type, got %s", contentType)
	}
}

func TestMyForecast(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	user := createPortalTestUserWithID(t, st, "forecast@example.com", "portal-pass")

	// Seed credit transactions
	if err := st.Credits().Create(&store.CreditTransaction{
		UserID:        user.ID,
		Type:          "consume",
		Amount:        -10,
		BalanceBefore: 100,
		BalanceAfter:  90,
		Description:   "test consumption",
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed credit transaction: %v", err)
	}

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Update user credit balance
	user.CreditBalance = 90

	ctx := context.WithValue(context.Background(), contextUserKey, user)
	req := httptest.NewRequest(http.MethodGet, "/portal/api/v1/credits/forecast", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	srv.myForecast(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if _, ok := response["balance"]; !ok {
		t.Error("expected balance in response")
	}
}

func TestPurchaseCredits_InvalidAmount(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	user := createPortalTestUserWithID(t, st, "purchase@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Test with invalid amount
	body := map[string]any{"amount": -10}
	jsonBytes, _ := json.Marshal(body)
	ctx := context.WithValue(context.Background(), contextUserKey, user)
	req := httptest.NewRequest(http.MethodPost, "/portal/api/v1/credits/purchase", bytes.NewReader(jsonBytes)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.purchaseCredits(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestMyTransactions_WithTypeFilter(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	user := createPortalTestUserWithID(t, st, "transactions@example.com", "portal-pass")

	// Seed credit transactions
	if err := st.Credits().Create(&store.CreditTransaction{
		UserID:        user.ID,
		Type:          "purchase",
		Amount:        50,
		BalanceBefore: 0,
		BalanceAfter:  50,
		Description:   "test purchase",
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed credit transaction: %v", err)
	}

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Test with type filter
	ctx := context.WithValue(context.Background(), contextUserKey, user)
	req := httptest.NewRequest(http.MethodGet, "/portal/api/v1/credits/transactions?type=purchase", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	srv.myTransactions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// =============================================================================
// Additional Session Management Tests
// =============================================================================

func TestWithSession_NoCookie(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Create a test handler that should not be called
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called without valid session")
	})

	// Test with no cookie
	req := httptest.NewRequest(http.MethodGet, "/portal/api/v1/test", nil)
	w := httptest.NewRecorder()

	srv.withSession(testHandler)(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestWithSession_InvalidCookie(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called with invalid session")
	})

	// Test with invalid cookie
	req := httptest.NewRequest(http.MethodGet, "/portal/api/v1/test", nil)
	req.AddCookie(&http.Cookie{
		Name:  cfg.Portal.Session.CookieName,
		Value: "invalid-token",
	})
	w := httptest.NewRecorder()

	srv.withSession(testHandler)(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestWithSession_ExpiredSession(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	user := createPortalTestUserWithID(t, st, "expired-session@example.com", "portal-pass")

	// Create an expired session
	token, _ := store.GenerateSessionToken()
	session := &store.Session{
		UserID:    user.ID,
		TokenHash: store.HashSessionToken(token),
		ExpiresAt: time.Now().UTC().Add(-time.Hour), // Expired
	}
	st.Sessions().Create(session)

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called with expired session")
	})

	// Test with expired session cookie
	req := httptest.NewRequest(http.MethodGet, "/portal/api/v1/test", nil)
	req.AddCookie(&http.Cookie{
		Name:  cfg.Portal.Session.CookieName,
		Value: token,
	})
	w := httptest.NewRecorder()

	srv.withSession(testHandler)(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestWithSession_InactiveUser(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	// Create inactive user
	hash, _ := store.HashPassword("portal-pass")
	user := &store.User{
		Email:        "inactive@example.com",
		Name:         "Inactive User",
		PasswordHash: hash,
		Role:         "user",
		Status:       "suspended",
	}
	if err := st.Users().Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Create session for inactive user
	token, _ := store.GenerateSessionToken()
	session := &store.Session{
		UserID:    user.ID,
		TokenHash: store.HashSessionToken(token),
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	st.Sessions().Create(session)

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for inactive user")
	})

	// Test with inactive user session
	req := httptest.NewRequest(http.MethodGet, "/portal/api/v1/test", nil)
	req.AddCookie(&http.Cookie{
		Name:  cfg.Portal.Session.CookieName,
		Value: token,
	})
	w := httptest.NewRecorder()

	srv.withSession(testHandler)(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// =============================================================================
// Additional Login Tests
// =============================================================================

func TestLogin_InvalidJSON(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Test with invalid JSON
	req := httptest.NewRequest(http.MethodPost, "/portal/api/v1/auth/login", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.login(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestLogin_MissingCredentials(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Test with missing credentials
	body := map[string]any{"email": "", "password": ""}
	jsonBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/portal/api/v1/auth/login", bytes.NewReader(jsonBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.login(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestLogin_UserLookupError(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Test with non-existent user
	body := map[string]any{"email": "nonexistent@example.com", "password": "password123"}
	jsonBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/portal/api/v1/auth/login", bytes.NewReader(jsonBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.login(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestLogin_SuccessClearsRateLimit(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	user := createPortalTestUserWithID(t, st, "login-success@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Record some failed attempts
	clientIP := "192.168.1.100"
	for i := 0; i < 3; i++ {
		srv.recordFailedAuth(clientIP)
	}

	// Verify attempts were recorded
	if !srv.isRateLimited(clientIP) && srv.rlAttempts[clientIP].count != 3 {
		t.Error("Failed attempts should be recorded")
	}

	// Now login successfully
	body := map[string]any{"email": user.Email, "password": "portal-pass"}
	jsonBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/portal/api/v1/auth/login", bytes.NewReader(jsonBytes))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = clientIP + ":12345"
	w := httptest.NewRecorder()

	srv.login(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify rate limit was cleared
	srv.rlMu.RLock()
	if _, exists := srv.rlAttempts[clientIP]; exists {
		t.Error("Rate limit should be cleared after successful login")
	}
	srv.rlMu.RUnlock()
}

// =============================================================================
// Additional Helper Function Tests
// =============================================================================

func TestNormalizePortalPathPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"/", ""},
		{"/portal", "/portal"},
		{"portal", "/portal"},
		{"/portal/", "/portal"},
		{"  /portal  ", "/portal"},
	}

	for _, tc := range tests {
		result := normalizePortalPathPrefix(tc.input)
		if result != tc.expected {
			t.Errorf("normalizePortalPathPrefix(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestSessionCookieMethods(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	cfg.Portal.Session.CookieName = ""
	cfg.Portal.Session.MaxAge = 0
	cfg.Portal.Session.Secure = true

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	t.Run("default cookie name", func(t *testing.T) {
		name := srv.sessionCookieName()
		if name != "apicerberus_session" {
			t.Errorf("expected default cookie name, got %s", name)
		}
	})

	t.Run("default max age", func(t *testing.T) {
		maxAge := srv.sessionMaxAge()
		if maxAge != 24*time.Hour {
			t.Errorf("expected 24h max age, got %v", maxAge)
		}
	})

	t.Run("session secure", func(t *testing.T) {
		secure := srv.sessionSecure()
		if !secure {
			t.Error("expected secure to be true")
		}
	})
}

func TestGetClientIP_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("nil request", func(t *testing.T) {
		ip := getClientIP(nil)
		if ip != "" {
			t.Errorf("expected empty string for nil request, got %s", ip)
		}
	})

	t.Run("empty remote addr", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = ""
		ip := getClientIP(req)
		if ip != "" {
			t.Errorf("expected empty string, got %s", ip)
		}
	})

	t.Run("remote addr without port", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.168.1.1"
		ip := getClientIP(req)
		if ip != "192.168.1.1" {
			t.Errorf("expected 192.168.1.1, got %s", ip)
		}
	})
}

func TestUserFromContext_NilContext(t *testing.T) {
	t.Parallel()

	user := userFromContext(nil)
	if user != nil {
		t.Error("expected nil user from nil context")
	}
}

func TestSessionFromContext_NilContext(t *testing.T) {
	t.Parallel()

	session := sessionFromContext(nil)
	if session != nil {
		t.Error("expected nil session from nil context")
	}
}

func TestSanitizeUser_Nil(t *testing.T) {
	t.Parallel()

	result := sanitizeUser(nil)
	if result == nil {
		t.Error("expected non-nil map")
	}
	if len(result) != 0 {
		t.Error("expected empty map for nil user")
	}
}

// =============================================================================
// Additional Rate Limiting Tests
// =============================================================================

func TestIsRateLimited_ExpiredBlock(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	clientIP := "192.168.1.200"

	// Create a blocked entry that has expired (more than 30 min old)
	srv.rlMu.Lock()
	srv.rlAttempts[clientIP] = &loginAuthAttempts{
		count:     10,
		firstSeen: time.Now().Add(-40 * time.Minute),
		lastSeen:  time.Now().Add(-40 * time.Minute),
		blocked:   true,
	}
	srv.rlMu.Unlock()

	// Should not be rate limited anymore since block expired
	if srv.isRateLimited(clientIP) {
		t.Error("should not be rate limited after block expires")
	}
}

func TestIsRateLimited_WindowExpired(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	clientIP := "192.168.1.201"

	// Create an entry that is outside the 15-minute window
	srv.rlMu.Lock()
	srv.rlAttempts[clientIP] = &loginAuthAttempts{
		count:     10,
		firstSeen: time.Now().Add(-20 * time.Minute),
		lastSeen:  time.Now().Add(-20 * time.Minute),
		blocked:   false,
	}
	srv.rlMu.Unlock()

	// Should not be rate limited since window expired
	if srv.isRateLimited(clientIP) {
		t.Error("should not be rate limited after window expires")
	}
}

func TestRecordFailedAuth_UpdatesExisting(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	clientIP := "192.168.1.202"

	// Record first attempt
	srv.recordFailedAuth(clientIP)

	// Record more attempts
	for i := 0; i < 3; i++ {
		srv.recordFailedAuth(clientIP)
	}

	// Verify count was updated
	srv.rlMu.RLock()
	attempts, exists := srv.rlAttempts[clientIP]
	srv.rlMu.RUnlock()

	if !exists {
		t.Fatal("attempts should exist")
	}

	if attempts.count != 4 {
		t.Errorf("expected count 4, got %d", attempts.count)
	}
}

func TestCleanupOldRateLimitEntries_NoOldEntries(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	clientIP := "192.168.1.203"

	// Create a recent entry
	srv.rlMu.Lock()
	srv.rlAttempts[clientIP] = &loginAuthAttempts{
		count:     1,
		firstSeen: time.Now(),
		lastSeen:  time.Now(),
	}
	srv.rlMu.Unlock()

	// Run cleanup
	srv.cleanupOldRateLimitEntries()

	// Entry should still exist since it's recent
	srv.rlMu.RLock()
	_, exists := srv.rlAttempts[clientIP]
	srv.rlMu.RUnlock()

	if !exists {
		t.Error("recent entry should not be cleaned up")
	}
}

// =============================================================================
// Additional API Handler Tests
// =============================================================================

func TestListMyAPIs_NoPermissions(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	user := createPortalTestUserWithID(t, st, "no-perms@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	ctx := context.WithValue(context.Background(), contextUserKey, user)
	req := httptest.NewRequest(http.MethodGet, "/portal/api/v1/apis", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	srv.listMyAPIs(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetMyAPIDetail_NotFound(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	user := createPortalTestUserWithID(t, st, "api-detail@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	ctx := context.WithValue(context.Background(), contextUserKey, user)
	req := httptest.NewRequest(http.MethodGet, "/portal/api/v1/apis/non-existent", nil).WithContext(ctx)
	req.SetPathValue("routeId", "non-existent")
	w := httptest.NewRecorder()

	srv.getMyAPIDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestRenameMyAPIKey_MissingName(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	user := createPortalTestUserWithID(t, st, "rename-key@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Create an API key
	_, key, err := st.APIKeys().Create(user.ID, "test-key", "test")
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Try to rename with empty name
	body := map[string]any{"name": ""}
	jsonBytes, _ := json.Marshal(body)
	ctx := context.WithValue(context.Background(), contextUserKey, user)
	req := httptest.NewRequest(http.MethodPut, "/portal/api/v1/api-keys/"+key.ID, bytes.NewReader(jsonBytes)).WithContext(ctx)
	req.SetPathValue("id", key.ID)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.renameMyAPIKey(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestPlaygroundSend_MissingFields(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	user := createPortalTestUserWithID(t, st, "playground@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Try with missing path
	body := map[string]any{"method": "GET"}
	jsonBytes, _ := json.Marshal(body)
	ctx := context.WithValue(context.Background(), contextUserKey, user)
	req := httptest.NewRequest(http.MethodPost, "/portal/api/v1/playground/send", bytes.NewReader(jsonBytes)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.playgroundSend(w, req)

	// Should fail validation
	if w.Code == http.StatusOK {
		t.Error("expected error for missing path")
	}
}

func TestSaveTemplate_MissingFields(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	user := createPortalTestUserWithID(t, st, "template@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Try with missing name
	body := map[string]any{"method": "GET", "path": "/test"}
	jsonBytes, _ := json.Marshal(body)
	ctx := context.WithValue(context.Background(), contextUserKey, user)
	req := httptest.NewRequest(http.MethodPost, "/portal/api/v1/playground/templates", bytes.NewReader(jsonBytes)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.saveTemplate(w, req)

	// Should fail validation
	if w.Code == http.StatusCreated {
		t.Error("expected error for missing name")
	}
}

func TestUpdateProfile_NoChanges(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	user := createPortalTestUserWithID(t, st, "profile@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Update with same values
	body := map[string]any{"name": user.Name, "company": user.Company}
	jsonBytes, _ := json.Marshal(body)
	ctx := context.WithValue(context.Background(), contextUserKey, user)
	req := httptest.NewRequest(http.MethodPut, "/portal/api/v1/settings/profile", bytes.NewReader(jsonBytes)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.updateProfile(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateNotifications_InvalidPayload(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	user := createPortalTestUserWithID(t, st, "notifications@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Invalid JSON
	ctx := context.WithValue(context.Background(), contextUserKey, user)
	req := httptest.NewRequest(http.MethodPut, "/portal/api/v1/settings/notifications", strings.NewReader("invalid")).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.updateNotifications(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAddMyIP_EmptyIP(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	user := createPortalTestUserWithID(t, st, "add-ip@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Empty IP should be rejected
	body := map[string]any{"ip": ""}
	jsonBytes, _ := json.Marshal(body)
	ctx := context.WithValue(context.Background(), contextUserKey, user)
	req := httptest.NewRequest(http.MethodPost, "/portal/api/v1/security/ip-whitelist", bytes.NewReader(jsonBytes)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.addMyIP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAddMyIP_ValidIP(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	user := createPortalTestUserWithID(t, st, "add-ip-valid@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Valid IP
	body := map[string]any{"ip": "192.168.1.100"}
	jsonBytes, _ := json.Marshal(body)
	ctx := context.WithValue(context.Background(), contextUserKey, user)
	req := httptest.NewRequest(http.MethodPost, "/portal/api/v1/security/ip-whitelist", bytes.NewReader(jsonBytes)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.addMyIP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
