package admin

import (
	"net/http"
	"testing"
	"time"
)

// --- extractAdminTokenFromCookie Tests ---

func TestExtractAdminTokenFromCookie(t *testing.T) {
	req := newRequestWithCookie(http.MethodGet, "/", adminSessionCookieName, "test-token-value")
	token := extractAdminTokenFromCookie(req)
	if token != "test-token-value" {
		t.Errorf("expected 'test-token-value', got %q", token)
	}
}

func TestExtractAdminTokenFromCookie_Empty(t *testing.T) {
	req := newRequestWithoutCookie(http.MethodGet, "/")
	token := extractAdminTokenFromCookie(req)
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}

func TestExtractAdminTokenFromCookie_Whitespace(t *testing.T) {
	req := newRequestWithCookie(http.MethodGet, "/", adminSessionCookieName, "  test-token  ")
	token := extractAdminTokenFromCookie(req)
	if token != "test-token" {
		t.Errorf("expected trimmed 'test-token', got %q", token)
	}
}

// --- extractBearerToken Tests ---

func TestExtractBearerToken(t *testing.T) {
	req := newRequestWithBearer(http.MethodGet, "/", "my-token-value")
	token := extractBearerToken(req)
	if token != "my-token-value" {
		t.Errorf("expected 'my-token-value', got %q", token)
	}
}

func TestExtractBearerToken_Missing(t *testing.T) {
	req := newRequestWithoutCookie(http.MethodGet, "/")
	token := extractBearerToken(req)
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}

func TestExtractBearerToken_Whitespace(t *testing.T) {
	req := newRequestWithBearer(http.MethodGet, "/", "   my-token   ")
	token := extractBearerToken(req)
	if token != "my-token" {
		t.Errorf("expected trimmed 'my-token', got %q", token)
	}
}

func TestExtractBearerToken_NotBearer(t *testing.T) {
	req := newRequestWithHeader(http.MethodGet, "/", "Authorization", "Basic dXNlcjpwYXNz")
	token := extractBearerToken(req)
	if token != "" {
		t.Errorf("expected empty for non-Bearer auth, got %q", token)
	}
}

// --- verifyAdminToken Tests ---

func TestVerifyAdminToken_EmptySecret(t *testing.T) {
	err := verifyAdminToken("some.token.here", "")
	if err == nil {
		t.Error("expected error for empty secret")
	}
}

func TestVerifyAdminToken_InvalidFormat(t *testing.T) {
	err := verifyAdminToken("not-a-jwt", "secret-at-least-32-chars-long!")
	if err == nil {
		t.Error("expected error for invalid JWT format")
	}
}

func TestVerifyAdminToken_WrongSignature(t *testing.T) {
	token, _ := issueAdminToken("secret-one-secret-at-least-32-chars!", 1*time.Hour)
	err := verifyAdminToken(token, "secret-two-secret-at-least-32-chars!!")
	if err == nil {
		t.Error("expected error for wrong signature")
	}
}

func TestIssueAdminToken_EmptySecret(t *testing.T) {
	_, err := issueAdminToken("", 0)
	if err == nil {
		t.Error("expected error for empty secret")
	}
}

func TestIssueAdminToken_DefaultTTL(t *testing.T) {
	token, err := issueAdminToken("secret-at-least-32-chars-long!!!", -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}
}

// --- handleTokenLogout Tests ---

func TestTokenLogout_ReturnsJSON(t *testing.T) {
	t.Parallel()
	serverURL, _, _, token := newAdminTestServer(t)

	req, _ := http.NewRequest(http.MethodPost, serverURL+"/admin/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	cookies := resp.Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == "apicerberus_admin_session" {
			found = true
			if c.Path != "/" {
				t.Errorf("expected cookie path /, got: %s", c.Path)
			}
		}
	}
	if !found {
		t.Error("expected admin session cookie to be cleared")
	}
}

// --- withAdminStaticAuth WrongKey ---

func TestAdminStaticAuth_WrongKey(t *testing.T) {
	t.Parallel()
	serverURL, _, _, _ := newAdminTestServer(t)

	req, _ := http.NewRequest(http.MethodGet, serverURL+"/admin/api/v1/status", nil)
	req.Header.Set("X-Admin-Key", "wrong-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong admin key, got %d", resp.StatusCode)
	}
}

// --- helper functions for creating requests with cookies/headers ---

func newRequestWithCookie(method, url, cookieName, cookieValue string) *http.Request {
	req, _ := http.NewRequest(method, url, nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: cookieValue})
	return req
}

func newRequestWithoutCookie(method, url string) *http.Request {
	req, _ := http.NewRequest(method, url, nil)
	return req
}

func newRequestWithBearer(method, url, token string) *http.Request {
	req, _ := http.NewRequest(method, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func newRequestWithHeader(method, url, key, value string) *http.Request {
	req, _ := http.NewRequest(method, url, nil)
	req.Header.Set(key, value)
	return req
}
