package portal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- validateCSRFToken Tests (0% coverage) ---

func TestValidateCSRFToken_Match(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(csrfHeaderName, "token-abc")

	if !validateCSRFToken(req, "token-abc") {
		t.Error("expected CSRF validation to pass when tokens match")
	}
}

func TestValidateCSRFToken_Mismatch(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(csrfHeaderName, "token-xyz")

	if validateCSRFToken(req, "token-abc") {
		t.Error("expected CSRF validation to fail when tokens differ")
	}
}

func TestValidateCSRFToken_MissingCookie(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	if validateCSRFToken(req, "") {
		t.Error("expected CSRF validation to fail without cookie")
	}
}

func TestValidateCSRFToken_MissingHeader(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	if validateCSRFToken(req, "token-abc") {
		t.Error("expected CSRF validation to fail without header")
	}
}

func TestValidateCSRFToken_EmptyCookieValue(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(csrfHeaderName, "token-abc")
	if validateCSRFToken(req, "") {
		t.Error("expected CSRF validation to fail with empty cookie value")
	}
}

func TestValidateCSRFToken_XSRFHeaderFallback(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-XSRF-Token", "token-abc")

	if !validateCSRFToken(req, "token-abc") {
		t.Error("expected CSRF validation to pass with X-XSRF-Token header")
	}
}

// --- me handler without user ---

func TestMe_NoUser(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	srv.me(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without user in context, got %d", w.Code)
	}
}

// --- logout without session ---

func TestLogout_NoSession(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()

	srv.logout(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// --- withCSRF middleware ---

func TestWithCSRF_GetSkipsValidation(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	handler := srv.withCSRF(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GET should skip CSRF validation, got %d", w.Code)
	}
}

func TestWithCSRF_PostWithoutCookieBlocked(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	handler := srv.withCSRF(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// M-020 FIX: Without CSRF cookie, all state-changing requests are blocked
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 when no CSRF cookie exists, got %d", w.Code)
	}
}

func TestWithCSRF_PostWithValidToken(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	handler := srv.withCSRF(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "valid-token"})
	req.Header.Set(csrfHeaderName, "valid-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with valid CSRF token, got %d", w.Code)
	}
}

// --- resolveGatewayBaseURL ---

func TestResolveGatewayBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		addr string
		want string
	}{
		{"", "http://127.0.0.1:8080"},
		{":8080", "http://127.0.0.1:8080"},
		{"http://api.example.com", "http://api.example.com"},
		{"https://api.example.com/", "https://api.example.com"},
		{"  :3000  ", "http://127.0.0.1:3000"},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			t.Parallel()
			got := resolveGatewayBaseURL(tt.addr)
			if got != tt.want {
				t.Errorf("resolveGatewayBaseURL(%q) = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}

// --- generateCSRFToken ---

func TestGenerateCSRFToken(t *testing.T) {
	t.Parallel()

	token, err := generateCSRFToken()
	if err != nil {
		t.Fatalf("generateCSRFToken error: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty CSRF token")
	}

	// Generate another - should be different (random)
	token2, err := generateCSRFToken()
	if err != nil {
		t.Fatalf("generateCSRFToken error: %v", err)
	}
	if token == token2 {
		t.Error("expected different tokens from successive calls")
	}
}

// --- portalAssetExists with nil filesystem ---

func TestPortalAssetExists_NilFS(t *testing.T) {
	t.Parallel()

	if portalAssetExists(nil, "anything") {
		t.Error("expected false for nil filesystem")
	}
}

// --- newPortalUIHandler serves index.html for root path ---

func TestNewPortalUIHandler_ServesIndex(t *testing.T) {
	t.Parallel()

	cfg, st := openPortalTestStore(t)
	defer st.Close()

	srv, err := NewServer(cfg, st)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}

	handler := srv.newPortalUIHandler()
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}

	req := httptest.NewRequest(http.MethodGet, "/portal", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Should serve index.html or return 404 if assets not embedded
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound && w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 200, 404, or 503, got %d", w.Code)
	}
}

// --- cloneURL nil ---

func TestCloneURL_NilInput(t *testing.T) {
	t.Parallel()

	result := cloneURL(nil)
	if result == nil {
		t.Error("expected non-nil result for nil input")
	}
}

// --- userFromContext nil ---

func TestUserFromContext_Nil(t *testing.T) {
	t.Parallel()

	result := userFromContext(context.Background())
	if result != nil {
		t.Error("expected nil user from context without user key")
	}
}

// --- sessionFromContext nil ---

func TestSessionFromContext_Nil(t *testing.T) {
	t.Parallel()

	result := sessionFromContext(context.Background())
	if result != nil {
		t.Error("expected nil session from context without session key")
	}
}
