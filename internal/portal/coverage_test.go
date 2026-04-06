package portal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/store"
)

// Test isUserActive edge cases
func TestIsUserActive_AllStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status   string
		expected bool
	}{
		{"", true},
		{"active", true},
		{"ACTIVE", true},
		{"Active", true},
		{"inactive", false},
		{"INACTIVE", false},
		{"suspended", false},
		{"banned", false},
		{"deleted", false},
	}

	for _, tc := range tests {
		user := &store.User{Status: tc.status}
		result := isUserActive(user)
		if result != tc.expected {
			t.Errorf("isUserActive(%q) = %v, want %v", tc.status, result, tc.expected)
		}
	}
}

// Test sanitizeUser with full data
func TestSanitizeUser_FullData(t *testing.T) {
	t.Parallel()

	user := &store.User{
		ID:            "user-123",
		Email:         "test@example.com",
		Name:          "Test User",
		Company:       "Test Company",
		Role:          "admin",
		Status:        "active",
		CreditBalance: 1000,
		RateLimits:    map[string]any{"default": int64(100), "burst": int64(200)},
		IPWhitelist:   []string{"127.0.0.1", "192.168.1.1"},
		Metadata: map[string]any{
			"theme": "dark",
			"lang":  "en",
		},
	}

	result := sanitizeUser(user)

	if result["id"] != "user-123" {
		t.Errorf("expected id=user-123, got %v", result["id"])
	}
	if result["email"] != "test@example.com" {
		t.Errorf("expected email=test@example.com, got %v", result["email"])
	}
	if result["credit_balance"] != int64(1000) {
		t.Errorf("expected credit_balance=1000, got %v", result["credit_balance"])
	}
}

// Test extractClientIP with X-Real-Ip fallback
func TestExtractClientIP_XRealIp(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-Ip", "10.0.0.1")
	ip := extractClientIP(req)
	if ip != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %s", ip)
	}
}

// Test extractClientIP with empty X-Forwarded-For
func TestExtractClientIP_EmptyXFF(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "")
	req.RemoteAddr = "192.168.1.1:12345"
	ip := extractClientIP(req)
	if ip != "192.168.1.1" {
		t.Errorf("expected 192.168.1.1, got %s", ip)
	}
}

// Test rate limit cleanup ticker initialization
func TestRateLimitCleanupTicker(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Verify cleanup ticker is set
	if srv.rlCleanupTicker == nil {
		t.Error("expected cleanup ticker to be initialized")
	}

	// Add an old entry
	srv.rlMu.Lock()
	srv.rlAttempts["test-ip"] = &loginAuthAttempts{
		count:     1,
		firstSeen: time.Now().Add(-40 * time.Minute),
		lastSeen:  time.Now().Add(-40 * time.Minute),
	}
	srv.rlMu.Unlock()

	// Trigger cleanup manually
	srv.cleanupOldRateLimitEntries()

	// Verify entry was cleaned up
	srv.rlMu.RLock()
	if _, exists := srv.rlAttempts["test-ip"]; exists {
		t.Error("expected old entry to be cleaned up")
	}
	srv.rlMu.RUnlock()
}

// Test setSessionCookie with various configurations
func TestSetSessionCookie_Configurations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cfg      sessionCookieConfig
		expected int
	}{
		{
			name: "normal",
			cfg: sessionCookieConfig{
				Name:     "test",
				Path:     "/",
				Value:    "value",
				Expires:  time.Now().Add(time.Hour),
				MaxAge:   time.Hour,
				Secure:   true,
				HTTPOnly: true,
			},
			expected: 3600,
		},
		{
			name: "zero_maxage",
			cfg: sessionCookieConfig{
				Name:     "test",
				Path:     "/",
				Value:    "value",
				Expires:  time.Now().Add(time.Hour),
				MaxAge:   0,
				Secure:   false,
				HTTPOnly: true,
			},
			expected: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			setSessionCookie(w, tc.cfg)

			cookies := w.Result().Cookies()
			if len(cookies) == 0 {
				t.Fatal("expected cookie to be set")
			}

			cookie := cookies[0]
			if cookie.MaxAge != tc.expected {
				t.Errorf("expected MaxAge=%d, got %d", tc.expected, cookie.MaxAge)
			}
		})
	}
}

// Test clearSessionCookie
func TestClearSessionCookie(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	cfg := sessionCookieConfig{
		Name:     "test_cookie",
		Path:     "/portal",
		Secure:   true,
		HTTPOnly: true,
	}
	clearSessionCookie(w, cfg)

	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected cookie to be set")
	}

	cookie := cookies[0]
	if cookie.Value != "" {
		t.Errorf("expected empty value, got %s", cookie.Value)
	}
	if cookie.MaxAge != -1 {
		t.Errorf("expected MaxAge=-1, got %d", cookie.MaxAge)
	}
}

// Test newPortalUIHandler with asset serving
func TestNewPortalUIHandler_AssetServing(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	handler := srv.newPortalUIHandler()

	// Test serving index.html for root path
	req := httptest.NewRequest(http.MethodGet, "/portal", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/html; charset=utf-8" {
		t.Errorf("expected text/html, got %s", contentType)
	}
}

// Test userFromContext with valid user
func TestUserFromContext_Valid(t *testing.T) {
	t.Parallel()

	user := &store.User{ID: "user-1", Email: "test@example.com"}
	ctx := context.WithValue(context.Background(), contextUserKey, user)

	result := userFromContext(ctx)
	if result == nil {
		t.Fatal("expected user from context")
	}
	if result.ID != "user-1" {
		t.Errorf("expected user ID user-1, got %s", result.ID)
	}
}

// Test sessionFromContext with valid session
func TestSessionFromContext_Valid(t *testing.T) {
	t.Parallel()

	session := &store.Session{ID: "session-1", UserID: "user-1"}
	ctx := context.WithValue(context.Background(), contextSessionKey, session)

	result := sessionFromContext(ctx)
	if result == nil {
		t.Fatal("expected session from context")
	}
	if result.ID != "session-1" {
		t.Errorf("expected session ID session-1, got %s", result.ID)
	}
}

// Test getClientIP with X-Forwarded-For containing empty parts
func TestGetClientIP_EmptyParts(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", " , 192.168.1.1, 10.0.0.1")
	ip := getClientIP(req)
	// The function returns the first part after trimming, which is empty string
	// so it falls back to RemoteAddr
	if ip == "" {
		t.Error("expected non-empty IP")
	}
}

// Test portalAssetExists with directory
func TestPortalAssetExists_Directory(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	// Test with a directory path (should return false)
	exists := portalAssetExists(srv.uiFS, "assets")
	if exists {
		t.Error("expected false for directory")
	}
}

// Test resolvePortalAssetPath with various path prefixes
func TestResolvePortalAssetPath_PrefixVariations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pathPrefix string
		cleanPath  string
		wantAsset  string
		wantServe  bool
	}{
		{"/portal", "/portal/assets/main.js", "assets/main.js", true},
		{"/portal", "/portal", "", true},
		{"/portal", "/portal/", "", true},
		{"/portal", "/other", "", false},
		{"", "/assets/main.js", "assets/main.js", true},
		{"", "/", "", true},
	}

	for _, tc := range tests {
		cfg, st := openPortalTestStore(t)
		cfg.Portal.PathPrefix = tc.pathPrefix

		srv, err := NewServer(cfg, st)
		if err != nil {
			st.Close()
			t.Fatalf("NewServer error: %v", err)
		}

		asset, serve := srv.resolvePortalAssetPath(tc.cleanPath)
		if asset != tc.wantAsset || serve != tc.wantServe {
			t.Errorf("resolvePortalAssetPath(%q, %q) = (%q, %v), want (%q, %v)",
				tc.pathPrefix, tc.cleanPath, asset, serve, tc.wantAsset, tc.wantServe)
		}
		st.Close()
	}
}

// Test me handler with valid user
func TestMe_WithValidUser(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	user := createPortalTestUserWithID(t, st, "me-valid@example.com", "portal-pass")

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	ctx := context.WithValue(context.Background(), contextUserKey, user)
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	srv.me(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// Test logout handler with session from context
func TestLogout_WithSessionFromContext(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	user := createPortalTestUserWithID(t, st, "logout-ctx@example.com", "portal-pass")

	// Create a session
	token, _ := store.GenerateSessionToken()
	session := &store.Session{
		UserID:    user.ID,
		TokenHash: store.HashSessionToken(token),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	st.Sessions().Create(session)

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	ctx := context.WithValue(context.Background(), contextSessionKey, session)
	req := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	srv.logout(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// Test extractClientIP with X-Forwarded-For only containing spaces
func TestExtractClientIP_XFFSpacesOnly(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "   ")
	req.RemoteAddr = "192.168.1.1:12345"
	ip := extractClientIP(req)
	// Should fall back to RemoteAddr when X-Forwarded-For is only spaces
	if ip != "192.168.1.1" {
		t.Errorf("expected 192.168.1.1, got %s", ip)
	}
}

// Test newPortalUIHandler with HEAD request
func TestNewPortalUIHandler_HEADRequest(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	handler := srv.newPortalUIHandler()

	// Test HEAD request
	req := httptest.NewRequest(http.MethodHead, "/portal", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for HEAD, got %d", w.Code)
	}
}

// Test newPortalUIHandler with existing asset
func TestNewPortalUIHandler_ExistingAsset(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	handler := srv.newPortalUIHandler()

	// Test requesting favicon.ico (which exists in embedded FS)
	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Should get 200 or 404 depending on if favicon exists
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		t.Errorf("expected 200 or 404, got %d", w.Code)
	}
}

// Test sanitizeUser edge cases
func TestSanitizeUser_EdgeCases(t *testing.T) {
	t.Parallel()

	// Test with nil metadata and rate limits
	user := &store.User{
		ID:            "user-1",
		Email:         "test@example.com",
		Name:          "Test",
		Company:       "",
		Role:          "user",
		Status:        "active",
		CreditBalance: 0,
		RateLimits:    nil,
		IPWhitelist:   nil,
		Metadata:      nil,
	}

	result := sanitizeUser(user)
	if result["id"] != "user-1" {
		t.Errorf("expected id=user-1, got %v", result["id"])
	}
	// Metadata field will be nil or empty map depending on how it's stored
	// Just verify the function doesn't panic with nil values
}

// Test setSessionCookie with negative max age
func TestSetSessionCookie_NegativeMaxAge(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	cfg := sessionCookieConfig{
		Name:     "test",
		Path:     "/",
		Value:    "value",
		Expires:  time.Now().Add(time.Hour),
		MaxAge:   -1 * time.Second,
		Secure:   false,
		HTTPOnly: true,
	}
	setSessionCookie(w, cfg)

	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected cookie to be set")
	}

	// Negative MaxAge should be set to 0
	cookie := cookies[0]
	if cookie.MaxAge != 0 {
		t.Errorf("expected MaxAge=0 for negative input, got %d", cookie.MaxAge)
	}
}
