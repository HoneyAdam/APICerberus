package plugin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// --- AuthJWT metadata methods ---

func TestAuthJWT_Name(t *testing.T) {
	t.Parallel()
	a := &AuthJWT{}
	if a.Name() != "auth-jwt" {
		t.Errorf("Name() = %q, want %q", a.Name(), "auth-jwt")
	}
}

func TestAuthJWT_Phase(t *testing.T) {
	t.Parallel()
	a := &AuthJWT{}
	if a.Phase() != PhaseAuth {
		t.Errorf("Phase() = %v, want %v", a.Phase(), PhaseAuth)
	}
}

func TestAuthJWT_Priority(t *testing.T) {
	t.Parallel()
	a := &AuthJWT{}
	if a.Priority() != 20 {
		t.Errorf("Priority() = %d, want 20", a.Priority())
	}
}

// --- hasClaimValue tests ---

func TestHasClaimValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    any
		expected bool
	}{
		{"nil", nil, false},
		{"empty string", "", false},
		{"whitespace string", "   ", false},
		{"non-empty string", "hello", true},
		{"empty slice", []any{}, false},
		{"non-empty slice", []any{"a"}, true},
		{"empty string slice", []string{}, false},
		{"non-empty string slice", []string{"a"}, true},
		{"number", 42, true},
		{"bool", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := hasClaimValue(tt.input); got != tt.expected {
				t.Errorf("hasClaimValue(%v) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

// --- claimValueToHeader tests ---

func TestClaimValueToHeader(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		input        any
		expectedVal  string
		expectedBool bool
	}{
		{"string", "hello", "hello", true},
		{"empty string", "", "", false},
		{"whitespace string", "  ", "", false},
		{"float64", float64(42), "42", true},
		{"float32", float32(3), "3", true},
		{"int", 99, "99", true},
		{"int64", int64(100), "100", true},
		{"slice of strings", []any{"a", "b"}, "a,b", true},
		{"slice with nil", []any{nil, "x"}, "x", true},
		{"empty slice", []any{}, "", false},
		{"slice of nils", []any{nil, nil}, "", false},
		{"bool default", true, "true", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			val, ok := claimValueToHeader(tt.input)
			if ok != tt.expectedBool {
				t.Errorf("claimValueToHeader(%v) ok = %v, want %v", tt.input, ok, tt.expectedBool)
			}
			if ok && val != tt.expectedVal {
				t.Errorf("claimValueToHeader(%v) val = %q, want %q", tt.input, val, tt.expectedVal)
			}
		})
	}
}

// --- extractBearerToken tests ---

func TestExtractBearerToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		setupReq func() *http.Request
		expected string
	}{
		{"nil request", func() *http.Request { return nil }, ""},
		{"valid bearer", func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("Authorization", "Bearer mytoken")
			return r
		}, "mytoken"},
		{"case insensitive bearer", func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("Authorization", "bearer mytoken")
			return r
		}, "mytoken"},
		{"no auth header", func() *http.Request {
			return httptest.NewRequest(http.MethodGet, "/", nil)
		}, ""},
		{"too short", func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("Authorization", "Basic")
			return r
		}, ""},
		{"wrong prefix", func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("Authorization", "Basic xyz")
			return r
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractBearerToken(tt.setupReq())
			if got != tt.expected {
				t.Errorf("extractBearerToken() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// --- AuthAPIKey.Lookup tests ---

func TestLookup_EmptyKey(t *testing.T) {
	t.Parallel()
	a := NewAuthAPIKey(nil, AuthAPIKeyOptions{})
	_, err := a.Lookup("")
	if err != ErrMissingAPIKey {
		t.Errorf("Lookup('') err = %v, want ErrMissingAPIKey", err)
	}
}

func TestLookup_InvalidKey(t *testing.T) {
	t.Parallel()
	a := NewAuthAPIKey(nil, AuthAPIKeyOptions{})
	_, err := a.Lookup("nonexistent-key")
	if err != ErrInvalidAPIKey {
		t.Errorf("Lookup('nonexistent') err = %v, want ErrInvalidAPIKey", err)
	}
}

func TestLookup_ExternalLookup(t *testing.T) {
	t.Parallel()
	called := false
	lookupFn := func(key string, req *http.Request) (*config.Consumer, error) {
		called = true
		return &config.Consumer{ID: "ext-user"}, nil
	}
	a := NewAuthAPIKey(nil, AuthAPIKeyOptions{Lookup: lookupFn})
	consumer, err := a.Lookup("any-key")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !called {
		t.Error("expected lookup function to be called")
	}
	if consumer.ID != "ext-user" {
		t.Errorf("consumer.ID = %q, want %q", consumer.ID, "ext-user")
	}
}

func TestLookup_ValidKey(t *testing.T) {
	t.Parallel()
	a := NewAuthAPIKey([]config.Consumer{
		{ID: "user-1", APIKeys: []config.ConsumerAPIKey{{Key: "ck_live_testkey123"}}},
	}, AuthAPIKeyOptions{})
	consumer, err := a.Lookup("ck_live_testkey123")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if consumer.ID != "user-1" {
		t.Errorf("consumer.ID = %q, want %q", consumer.ID, "user-1")
	}
}

func TestLookup_ExpiredKey(t *testing.T) {
	t.Parallel()
	past := time.Now().Add(-time.Hour).Format(time.RFC3339)
	a := NewAuthAPIKey([]config.Consumer{
		{ID: "user-1", APIKeys: []config.ConsumerAPIKey{{Key: "ck_live_expired", ExpiresAt: past}}},
	}, AuthAPIKeyOptions{})
	_, err := a.Lookup("ck_live_expired")
	if err != ErrExpiredAPIKey {
		t.Errorf("Lookup expired err = %v, want ErrExpiredAPIKey", err)
	}
}

// --- DebugSummary test ---

func TestDebugSummary(t *testing.T) {
	t.Parallel()
	a := NewAuthAPIKey([]config.Consumer{
		{ID: "user-1", APIKeys: []config.ConsumerAPIKey{{Key: "key1"}}},
		{ID: "user-2", APIKeys: []config.ConsumerAPIKey{{Key: "key2"}}},
	}, AuthAPIKeyOptions{})
	summary := a.DebugSummary()
	if !strings.Contains(summary, "consumers=2") {
		t.Errorf("DebugSummary() = %q, want consumers=2", summary)
	}
	if !strings.Contains(summary, "keys=2") {
		t.Errorf("DebugSummary() = %q, want keys=2", summary)
	}
}

// --- applyClaimHeaders tests ---

func TestApplyClaimHeaders_NilRequest(t *testing.T) {
	t.Parallel()
	a := &AuthJWT{claimsToHeaders: map[string]string{"sub": "X-User"}}
	a.applyClaimHeaders(nil, map[string]any{"sub": "user1"})
}

func TestApplyClaimHeaders_EmptyMapping(t *testing.T) {
	t.Parallel()
	a := &AuthJWT{}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	a.applyClaimHeaders(r, map[string]any{"sub": "user1"})
	if r.Header.Get("X-User") != "" {
		t.Error("expected no header when mapping is empty")
	}
}

func TestApplyClaimHeaders_WithMapping(t *testing.T) {
	t.Parallel()
	a := &AuthJWT{claimsToHeaders: map[string]string{
		"sub":  "X-User-ID",
		"role": "X-User-Role",
	}}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	a.applyClaimHeaders(r, map[string]any{
		"sub":  "user-123",
		"role": "admin",
	})
	if r.Header.Get("X-User-ID") != "user-123" {
		t.Errorf("X-User-ID = %q, want %q", r.Header.Get("X-User-ID"), "user-123")
	}
	if r.Header.Get("X-User-Role") != "admin" {
		t.Errorf("X-User-Role = %q, want %q", r.Header.Get("X-User-Role"), "admin")
	}
}

func TestApplyClaimHeaders_MissingClaim(t *testing.T) {
	t.Parallel()
	a := &AuthJWT{claimsToHeaders: map[string]string{
		"email": "X-Email",
	}}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	a.applyClaimHeaders(r, map[string]any{"sub": "user-123"})
	if r.Header.Get("X-Email") != "" {
		t.Error("expected no header for missing claim")
	}
}

// --- resolveECDSAPublicKey / resolveRSAPublicKey edge cases ---

func TestResolveECDSAPublicKey_NoKeyNoJWKS(t *testing.T) {
	t.Parallel()
	a := &AuthJWT{}
	_, err := a.resolveECDSAPublicKey(nil, nil)
	if err != ErrInvalidJWTSignature {
		t.Errorf("resolveECDSAPublicKey err = %v, want ErrInvalidJWTSignature", err)
	}
}

func TestResolveRSAPublicKey_NoKeyNoJWKS(t *testing.T) {
	t.Parallel()
	a := &AuthJWT{}
	_, err := a.resolveRSAPublicKey(nil, nil)
	if err != ErrInvalidJWTSignature {
		t.Errorf("resolveRSAPublicKey err = %v, want ErrInvalidJWTSignature", err)
	}
}

// --- isExpired tests ---

func TestIsExpired_Nil(t *testing.T) {
	t.Parallel()
	if isExpired(nil) {
		t.Error("expected false for nil expiry")
	}
}

func TestIsExpired_Past(t *testing.T) {
	t.Parallel()
	past := time.Now().Add(-time.Hour)
	if !isExpired(&past) {
		t.Error("expected true for past expiry")
	}
}

func TestIsExpired_Future(t *testing.T) {
	t.Parallel()
	future := time.Now().Add(time.Hour)
	if isExpired(&future) {
		t.Error("expected false for future expiry")
	}
}
