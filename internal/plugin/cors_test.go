package plugin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSPreflight(t *testing.T) {
	t.Parallel()

	cors := NewCORS(CORSConfig{
		AllowedOrigins: []string{"https://app.example.com"},
		AllowedMethods: []string{"GET", "POST"},
		AllowedHeaders: []string{"Authorization", "Content-Type"},
		MaxAge:         600,
	})

	req := httptest.NewRequest(http.MethodOptions, "http://gateway.local/users", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rr := httptest.NewRecorder()

	handled := cors.Handle(rr, req)
	if !handled {
		t.Fatalf("preflight should be handled")
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204 got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Fatalf("unexpected allow origin header")
	}
	if rr.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Fatalf("missing Access-Control-Allow-Methods")
	}
	if rr.Header().Get("Access-Control-Allow-Headers") == "" {
		t.Fatalf("missing Access-Control-Allow-Headers")
	}
}

func TestCORSActualRequest(t *testing.T) {
	t.Parallel()

	// M-003 (CORS wildcard) security fix: wildcard origins are rejected at config time.
	// Test actual requests with a proper non-wildcard origin configuration.
	cors := NewCORS(CORSConfig{
		AllowedOrigins: []string{"https://app.example.com"},
	})

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/users", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rr := httptest.NewRecorder()

	handled := cors.Handle(rr, req)
	if handled {
		t.Fatalf("actual request should continue in pipeline")
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Fatalf("expected specific allow origin")
	}
}

func TestCORSWildcardOriginRejected(t *testing.T) {
	t.Parallel()

	// SECURITY TEST: Wildcard origins are rejected at config time (M-003 fix).
	// When wildcard is rejected, the plugin denies requests with origins not in the
	// explicit list (which now only contains the literal "*" string, not the wildcard).
	cors := NewCORS(CORSConfig{
		AllowedOrigins: []string{"*"},
	})

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/users", nil)
	req.Header.Set("Origin", "https://other.example.com")
	rr := httptest.NewRecorder()

	handled := cors.Handle(rr, req)
	if !handled {
		t.Fatalf("CORS plugin should handle (block) disallowed origin")
	}
	// With M-003 fix, non-matching origin on a config with "*" (but no wildcard flag)
	// should be blocked since "*" in origins list requires exact match
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-matching origin, got %d", rr.Code)
	}
}

func TestCORSDisallowedOrigin(t *testing.T) {
	t.Parallel()

	cors := NewCORS(CORSConfig{
		AllowedOrigins: []string{"https://app.example.com"},
	})

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/users", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rr := httptest.NewRecorder()

	handled := cors.Handle(rr, req)
	if !handled {
		t.Fatalf("disallowed origin should be handled with rejection")
	}
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 got %d", rr.Code)
	}
}
