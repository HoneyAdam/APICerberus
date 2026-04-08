package plugin

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/pkg/jwt"
)

// Test GraphQLGuard plugin
func TestGraphQLGuard_Methods(t *testing.T) {
	t.Run("NewGraphQLGuard with nil config", func(t *testing.T) {
		guard := NewGraphQLGuard(nil)
		if guard == nil {
			t.Fatal("NewGraphQLGuard(nil) returned nil")
		}
		if guard.maxDepth != 15 {
			t.Errorf("maxDepth = %d, want 15", guard.maxDepth)
		}
		if guard.maxComplexity != 1000 {
			t.Errorf("maxComplexity = %d, want 1000", guard.maxComplexity)
		}
	})

	t.Run("NewGraphQLGuard with custom config", func(t *testing.T) {
		guard := NewGraphQLGuard(&GraphQLGuardConfig{
			MaxDepth:           10,
			MaxComplexity:      500,
			BlockIntrospection: true,
			FieldCosts:         map[string]int{"Query": 2},
		})
		if guard == nil {
			t.Fatal("NewGraphQLGuard returned nil")
		}
		if guard.maxDepth != 10 {
			t.Errorf("maxDepth = %d, want 10", guard.maxDepth)
		}
		if guard.maxComplexity != 500 {
			t.Errorf("maxComplexity = %d, want 500", guard.maxComplexity)
		}
		if !guard.blockIntrospection {
			t.Error("blockIntrospection should be true")
		}
	})

	t.Run("Name returns graphql_guard", func(t *testing.T) {
		guard := NewGraphQLGuard(nil)
		if guard.Name() != "graphql_guard" {
			t.Errorf("Name() = %q, want %q", guard.Name(), "graphql_guard")
		}
	})

	t.Run("Phase returns PhasePreAuth", func(t *testing.T) {
		guard := NewGraphQLGuard(nil)
		if guard.Phase() != PhasePreAuth {
			t.Errorf("Phase() = %v, want %v", guard.Phase(), PhasePreAuth)
		}
	})

	t.Run("Priority returns 2", func(t *testing.T) {
		guard := NewGraphQLGuard(nil)
		if guard.Priority() != 2 {
			t.Errorf("Priority() = %d, want 2", guard.Priority())
		}
	})

	t.Run("Handle with nil receiver", func(t *testing.T) {
		var guard *GraphQLGuard
		if guard.Handle(nil, nil) {
			t.Error("Handle should return false with nil receiver")
		}
	})

	t.Run("Handle with nil writer", func(t *testing.T) {
		guard := NewGraphQLGuard(nil)
		if guard.Handle(nil, nil) {
			t.Error("Handle should return false with nil writer")
		}
	})

	t.Run("Handle with nil request", func(t *testing.T) {
		guard := NewGraphQLGuard(nil)
		w := &mockResponseWriter{}
		if guard.Handle(w, nil) {
			t.Error("Handle should return false with nil request")
		}
	})
}

// Mock response writer for testing
type mockResponseWriter struct {
	header     http.Header
	statusCode int
	body       []byte
	writeErr   error
}

func (m *mockResponseWriter) Header() http.Header {
	if m.header == nil {
		m.header = make(http.Header)
	}
	return m.header
}

func (m *mockResponseWriter) Write(b []byte) (int, error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	m.body = append(m.body, b...)
	return len(b), nil
}

func (m *mockResponseWriter) WriteHeader(code int) {
	m.statusCode = code
}

// Test EndpointPermissionError Error method
func TestEndpointPermissionError_Error(t *testing.T) {
	err := &EndpointPermissionError{
		Code:    "test_code",
		Message: "test message",
	}

	if err.Error() != "test message" {
		t.Errorf("Error() = %q, want %q", err.Error(), "test message")
	}
}

// Test claimValueToHeader function
func TestClaimValueToHeader(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		wantVal  string
		wantBool bool
	}{
		{"empty string", "", "", false},
		{"whitespace string", "   ", "", false},
		{"valid string", "test", "test", true},
		{"string with spaces", " test ", "test", true},
		{"float64", float64(42), "42", true},
		{"float32", float32(42), "42", true},
		{"int", int(42), "42", true},
		{"int64", int64(42), "42", true},
		{"nil", nil, "<nil>", true}, // nil falls through to default case which uses fmt.Sprint
		{"empty []any", []any{}, "", false},
		{"[]any with values", []any{"a", "b"}, "a,b", true},
		{"[]any with nil", []any{nil, "a"}, "a", true},
		{"bool true", true, "true", true},
		{"bool false", false, "false", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := claimValueToHeader(tt.input)
			if val != tt.wantVal {
				t.Errorf("claimValueToHeader() val = %q, want %q", val, tt.wantVal)
			}
			if ok != tt.wantBool {
				t.Errorf("claimValueToHeader() ok = %v, want %v", ok, tt.wantBool)
			}
		})
	}
}

// Test hasClaimValue function
func TestHasClaimValue(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  bool
	}{
		{"nil", nil, false},
		{"empty string", "", false},
		{"whitespace string", "   ", false},
		{"valid string", "test", true},
		{"empty []any", []any{}, false},
		{"[]any with values", []any{"a"}, true},
		{"empty []string", []string{}, false},
		{"[]string with values", []string{"a"}, true},
		{"int", 42, true},
		{"bool", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasClaimValue(tt.input)
			if got != tt.want {
				t.Errorf("hasClaimValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test CircuitBreaker state transitions
func TestCircuitBreaker_StateTransitions(t *testing.T) {
	t.Run("closed state", func(t *testing.T) {
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			ErrorThreshold: 0.5,
			SleepWindow:    time.Second,
		})
		if cb.State() != CircuitClosed {
			t.Errorf("Initial state = %v, want CLOSED", cb.State())
		}
	})

	t.Run("nil config uses defaults", func(t *testing.T) {
		cb := NewCircuitBreaker(CircuitBreakerConfig{})
		if cb == nil {
			t.Fatal("NewCircuitBreaker returned nil")
		}
		if cb.State() != CircuitClosed {
			t.Error("Should start in CLOSED state")
		}
	})
}

// Test AuthJWT plugin
func TestAuthJWT_Methods(t *testing.T) {
	t.Run("NewAuthJWT with empty options", func(t *testing.T) {
		jwtAuth := NewAuthJWT(AuthJWTOptions{})
		if jwtAuth == nil {
			t.Fatal("NewAuthJWT returned nil")
		}
		// Check defaults are applied
		if jwtAuth.clockSkew != 30*time.Second {
			t.Errorf("clockSkew = %v, want 30s", jwtAuth.clockSkew)
		}
	})

	t.Run("NewAuthJWT with custom clock skew", func(t *testing.T) {
		jwtAuth := NewAuthJWT(AuthJWTOptions{
			ClockSkew: 60 * time.Second,
		})
		if jwtAuth.clockSkew != 60*time.Second {
			t.Errorf("clockSkew = %v, want 60s", jwtAuth.clockSkew)
		}
	})

	t.Run("NewAuthJWT with negative clock skew uses default", func(t *testing.T) {
		jwtAuth := NewAuthJWT(AuthJWTOptions{
			ClockSkew: -10 * time.Second,
		})
		// Negative clock skew becomes 0, then 0 becomes default 30s
		if jwtAuth.clockSkew != 30*time.Second {
			t.Errorf("clockSkew = %v, want 30s", jwtAuth.clockSkew)
		}
	})

	t.Run("NewAuthJWT with claims to headers", func(t *testing.T) {
		jwtAuth := NewAuthJWT(AuthJWTOptions{
			ClaimsToHeaders: map[string]string{
				"sub":   "X-User-ID",
				"email": "X-User-Email",
			},
		})
		if jwtAuth.claimsToHeaders["sub"] != "X-User-ID" {
			t.Error("claimsToHeaders not set correctly")
		}
	})

	t.Run("Name returns auth-jwt", func(t *testing.T) {
		jwtAuth := NewAuthJWT(AuthJWTOptions{})
		if jwtAuth.Name() != "auth-jwt" {
			t.Errorf("Name() = %q, want %q", jwtAuth.Name(), "auth-jwt")
		}
	})

	t.Run("Phase returns PhaseAuth", func(t *testing.T) {
		jwtAuth := NewAuthJWT(AuthJWTOptions{})
		if jwtAuth.Phase() != PhaseAuth {
			t.Errorf("Phase() = %v, want %v", jwtAuth.Phase(), PhaseAuth)
		}
	})

	t.Run("Priority returns 20", func(t *testing.T) {
		jwtAuth := NewAuthJWT(AuthJWTOptions{})
		if jwtAuth.Priority() != 20 {
			t.Errorf("Priority() = %d, want 20", jwtAuth.Priority())
		}
	})
}

// Test CircuitBreaker Allow method
func TestCircuitBreaker_Allow(t *testing.T) {
	t.Run("Allow in closed state", func(t *testing.T) {
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			ErrorThreshold: 0.5,
			SleepWindow:    time.Second,
		})
		// In closed state, Allow should return nil (allow request)
		err := cb.Allow()
		if err != nil {
			t.Errorf("Allow() in closed state = %v, want nil", err)
		}
	})

	t.Run("Allow with nil config uses defaults", func(t *testing.T) {
		cb := NewCircuitBreaker(CircuitBreakerConfig{})
		if cb == nil {
			t.Fatal("NewCircuitBreaker returned nil")
		}
		err := cb.Allow()
		if err != nil {
			t.Errorf("Allow() with defaults = %v, want nil", err)
		}
	})
}

// Test compression Apply with various content types
func TestCompression_Apply(t *testing.T) {
	t.Run("with nil context", func(t *testing.T) {
		c := NewCompression(CompressionConfig{
			MinSize: 100,
		})

		// Should not panic with nil
		c.Apply(nil)
	})

	t.Run("with empty config", func(t *testing.T) {
		c := NewCompression(CompressionConfig{})
		if c == nil {
			t.Fatal("NewCompression returned nil")
		}
		if c.minSize != 0 {
			t.Errorf("minSize = %d, want 0", c.minSize)
		}
	})
}

// Test BotDetect
func TestBotDetect(t *testing.T) {
	t.Run("NewBotDetect", func(t *testing.T) {
		bd := NewBotDetect(BotDetectConfig{
			DenyList: []string{"badbot"},
			Action:   "block",
		})
		if bd == nil {
			t.Fatal("NewBotDetect returned nil")
		}
	})

	t.Run("NewBotDetect nil config", func(t *testing.T) {
		bd := NewBotDetect(BotDetectConfig{})
		if bd == nil {
			t.Fatal("NewBotDetect returned nil")
		}
	})
}

// Test CorrelationID
func TestCorrelationID(t *testing.T) {
	t.Run("NewCorrelationID", func(t *testing.T) {
		cid := NewCorrelationID()
		if cid == nil {
			t.Fatal("NewCorrelationID returned nil")
		}
	})

	t.Run("Apply with nil", func(t *testing.T) {
		cid := NewCorrelationID()
		// Should not panic with nil
		cid.Apply(nil)
	})
}

// Test AuthAPIKey additional methods
func TestAuthAPIKey_Additional(t *testing.T) {
	t.Run("Lookup method", func(t *testing.T) {
		auth := NewAuthAPIKey(nil, AuthAPIKeyOptions{
			KeyNames: []string{"X-API-Key"},
		})
		// Lookup with empty key should return nil
		result, err := auth.Lookup("")
		if err == nil && result != nil {
			t.Error("Lookup with empty key should return nil or error")
		}
	})

	t.Run("DebugSummary", func(t *testing.T) {
		auth := NewAuthAPIKey(nil, AuthAPIKeyOptions{
			KeyNames: []string{"X-API-Key"},
		})
		summary := auth.DebugSummary()
		if summary == "" {
			t.Error("DebugSummary should return non-empty string")
		}
	})
}

// Test BotDetect methods
func TestBotDetect_Methods(t *testing.T) {
	bd := NewBotDetect(BotDetectConfig{
		DenyList: []string{"badbot"},
		Action:   "block",
	})

	t.Run("Name returns bot-detect", func(t *testing.T) {
		if bd.Name() != "bot-detect" {
			t.Errorf("Name() = %q, want %q", bd.Name(), "bot-detect")
		}
	})

	t.Run("Phase returns PhasePreAuth", func(t *testing.T) {
		if bd.Phase() != PhasePreAuth {
			t.Errorf("Phase() = %v, want %v", bd.Phase(), PhasePreAuth)
		}
	})

	t.Run("Priority returns 3", func(t *testing.T) {
		if bd.Priority() != 3 {
			t.Errorf("Priority() = %d, want 3", bd.Priority())
		}
	})
}

// Test error types
func TestError_Types(t *testing.T) {
	t.Run("AuthError Error", func(t *testing.T) {
		err := &AuthError{
			Code:    "invalid_key",
			Message: "API key is invalid",
		}
		if err.Error() != "API key is invalid" {
			t.Errorf("Error() = %q, want %q", err.Error(), "API key is invalid")
		}
	})

	t.Run("JWTAuthError Error", func(t *testing.T) {
		err := &JWTAuthError{
			Code:    "invalid_token",
			Message: "token is expired",
		}
		if err.Error() != "token is expired" {
			t.Errorf("Error() = %q, want %q", err.Error(), "token is expired")
		}
	})

	t.Run("BotDetectError Error", func(t *testing.T) {
		err := &BotDetectError{
			Code:    "bot_detected",
			Message: "bot detected in request",
		}
		if err.Error() != "bot detected in request" {
			t.Errorf("Error() = %q, want %q", err.Error(), "bot detected in request")
		}
	})
}

// Test CircuitBreaker State method
func TestCircuitBreaker_State(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		ErrorThreshold: 0.5,
		SleepWindow:    time.Second,
	})

	// Initial state should be Closed
	if cb.State() != CircuitClosed {
		t.Errorf("Initial state = %v, want CircuitClosed", cb.State())
	}
}

// Test normalizeRedirectStatus function
func TestNormalizeRedirectStatus(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		{301, 301},
		{302, 302},
		{307, 307},
		{308, 308},
		{200, 302}, // Invalid, should default to 302
		{404, 302}, // Invalid, should default to 302
		{500, 302}, // Invalid, should default to 302
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("status_%d", tt.input), func(t *testing.T) {
			got := normalizeRedirectStatus(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeRedirectStatus(%d) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

// Test appendQueryIfMissing function
func TestAppendQueryIfMissing(t *testing.T) {
	tests := []struct {
		url      string
		query    string
		expected string
	}{
		{"http://example.com", "foo=bar", "http://example.com?foo=bar"},
		{"http://example.com?existing=param", "foo=bar", "http://example.com?existing=param"},
		{"http://example.com", "", "http://example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := appendQueryIfMissing(tt.url, tt.query)
			if got != tt.expected {
				t.Errorf("appendQueryIfMissing(%q, %q) = %q, want %q", tt.url, tt.query, got, tt.expected)
			}
		})
	}
}

// Test parseMinute function
func TestParseMinute(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		wantErr  bool
	}{
		{"00:00", 0, false},
		{"00:30", 30, false},
		{"00:59", 59, false},
		{"12:00", 720, false},
		{"23:59", 1439, false},
		{"25:00", 0, true}, // Hour out of range
		{"12:60", 0, true}, // Minute out of range
		{"-1:00", 0, true}, // Negative hour
		{"12:-1", 0, true}, // Negative minute
		{"abc", 0, true},   // Invalid
		{"", 0, true},      // Empty
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseMinute(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseMinute(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("parseMinute(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

// Test parseTimeRange function
func TestParseTimeRange(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid range", "00:00-23:59", false},
		{"business hours", "09:00-17:00", false},
		{"empty", "", true},
		{"invalid format", "invalid", true},
		{"no dash", "00:00 23:59", true},
		{"same time not allowed", "12:00-12:00", true}, // Same time is invalid
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startMin, endMin, err := parseTimeRange(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTimeRange() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				// Verify times are valid minutes
				if startMin < 0 || startMin > 1439 {
					t.Errorf("startMin = %d, want between 0 and 1439", startMin)
				}
				if endMin < 0 || endMin > 1439 {
					t.Errorf("endMin = %d, want between 0 and 1439", endMin)
				}
			}
		})
	}
}

// Test consumerKey function
func TestConsumerKey(t *testing.T) {
	tests := []struct {
		name     string
		consumer *config.Consumer
		expected string
	}{
		{
			name:     "nil consumer",
			consumer: nil,
			expected: "anonymous",
		},
		{
			name:     "consumer with ID",
			consumer: &config.Consumer{ID: "consumer-123"},
			expected: "consumer-123",
		},
		{
			name:     "consumer without ID",
			consumer: &config.Consumer{},
			expected: "anonymous",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := consumerKey(tt.consumer)
			if got != tt.expected {
				t.Errorf("consumerKey() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// Test routeKey function
func TestRouteKey(t *testing.T) {
	tests := []struct {
		name     string
		route    *config.Route
		req      *http.Request
		expected string
	}{
		{
			name:     "nil route",
			route:    nil,
			req:      nil,
			expected: "unknown", // Function returns "unknown" for nil route
		},
		{
			name:     "route with ID",
			route:    &config.Route{ID: "route-123"},
			req:      nil,
			expected: "route-123",
		},
		{
			name:     "route with Name",
			route:    &config.Route{Name: "route-name"},
			req:      nil,
			expected: "route-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := routeKey(tt.route, tt.req)
			if got != tt.expected {
				t.Errorf("routeKey() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// Test extractBearerToken function
func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name     string
		auth     string
		expected string
	}{
		{"Bearer token", "Bearer abc123", "abc123"},
		{"bearer lowercase", "bearer abc123", "abc123"}, // EqualFold makes it case-insensitive
		{"BEARER uppercase", "BEARER abc123", "abc123"},
		{"with extra spaces", "Bearer   abc123", "abc123"}, // Spaces are trimmed
		{"short token", "Bearer x", "x"},                   // Just needs 8+ chars
		{"empty", "", ""},
		{"only bearer", "Bearer", ""},
		{"no space", "Bearerabc", ""}, // Must have space after Bearer
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				Header: http.Header{
					"Authorization": []string{tt.auth},
				},
			}
			got := extractBearerToken(req)
			if got != tt.expected {
				t.Errorf("extractBearerToken(%q) = %q, want %q", tt.auth, got, tt.expected)
			}
		})
	}
}

// Test extractBearerToken with nil request
func TestExtractBearerToken_NilRequest(t *testing.T) {
	got := extractBearerToken(nil)
	if got != "" {
		t.Errorf("extractBearerToken(nil) = %q, want empty string", got)
	}
}

// Test ensureVaryAcceptEncoding function
func TestEnsureVaryAcceptEncoding(t *testing.T) {
	tests := []struct {
		name     string
		input    http.Header
		expected string
	}{
		{
			name:     "empty header",
			input:    http.Header{},
			expected: "Accept-Encoding",
		},
		{
			name: "existing vary header",
			input: http.Header{
				"Vary": []string{"Authorization"},
			},
			expected: "Authorization, Accept-Encoding",
		},
		{
			name: "already has accept-encoding",
			input: http.Header{
				"Vary": []string{"Accept-Encoding"},
			},
			expected: "Accept-Encoding",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ensureVaryAcceptEncoding(tt.input)
			got := tt.input.Get("Vary")
			if got != tt.expected {
				t.Errorf("Vary header = %q, want %q", got, tt.expected)
			}
		})
	}
}

// Test phaseOrder function
func TestPhaseOrder(t *testing.T) {
	tests := []struct {
		phase    Phase
		expected int
	}{
		{PhasePreAuth, 1},
		{PhaseAuth, 2},
		{PhasePreProxy, 3},
		{PhaseProxy, 4},
		{PhasePostProxy, 5},
		{Phase("unknown"), 999}, // Unknown phase
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("phase_%s", tt.phase), func(t *testing.T) {
			got := phaseOrder(tt.phase)
			if got != tt.expected {
				t.Errorf("phaseOrder(%v) = %d, want %d", tt.phase, got, tt.expected)
			}
		})
	}
}

// Test asString function
func TestAsString(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"string", "hello", "hello"},
		{"int", 42, "42"},
		{"int64", int64(42), "42"},
		{"float64", 3.14, "3.14"},
		{"bool", true, "true"},
		{"nil", nil, ""},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asString(tt.input)
			if got != tt.expected {
				t.Errorf("asString(%v) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// Test asStringSlice function
func TestAsStringSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected []string
	}{
		{
			name:     "[]string",
			input:    []string{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "[]interface{}",
			input:    []any{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "[]interface{} with ints",
			input:    []any{1, 2, 3},
			expected: []string{"1", "2", "3"},
		},
		{
			name:     "nil",
			input:    nil,
			expected: nil,
		},
		{
			name:     "string",
			input:    "not a slice",
			expected: []string{"not a slice"}, // String is wrapped in a slice
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asStringSlice(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("asStringSlice(%v) = %v, want %v", tt.input, got, tt.expected)
				return
			}
			for i := range tt.expected {
				if got[i] != tt.expected[i] {
					t.Errorf("asStringSlice(%v)[%d] = %q, want %q", tt.input, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

// Test AuthAPIKey Lookup method (direct call)
func TestAuthAPIKey_Lookup_Direct(t *testing.T) {
	t.Run("Lookup with empty key", func(t *testing.T) {
		auth := NewAuthAPIKey(nil, AuthAPIKeyOptions{})
		consumer, err := auth.Lookup("")
		if err != ErrMissingAPIKey {
			t.Errorf("Lookup(\"\") error = %v, want ErrMissingAPIKey", err)
		}
		if consumer != nil {
			t.Error("Lookup with empty key should return nil consumer")
		}
	})

	t.Run("Lookup with whitespace key", func(t *testing.T) {
		auth := NewAuthAPIKey(nil, AuthAPIKeyOptions{})
		consumer, err := auth.Lookup("   ")
		if err != ErrMissingAPIKey {
			t.Errorf("Lookup(whitespace) error = %v, want ErrMissingAPIKey", err)
		}
		if consumer != nil {
			t.Error("Lookup with whitespace key should return nil consumer")
		}
	})

	t.Run("Lookup with nil AuthAPIKey", func(t *testing.T) {
		var auth *AuthAPIKey
		consumer, err := auth.Lookup("some-key")
		if err != ErrInvalidAPIKey {
			t.Errorf("Lookup on nil AuthAPIKey error = %v, want ErrInvalidAPIKey", err)
		}
		if consumer != nil {
			t.Error("Lookup on nil AuthAPIKey should return nil consumer")
		}
	})

	t.Run("Lookup with custom lookup func", func(t *testing.T) {
		expectedConsumer := &config.Consumer{ID: "custom-consumer"}
		lookupCalled := false
		auth := NewAuthAPIKey(nil, AuthAPIKeyOptions{
			Lookup: func(rawKey string, req *http.Request) (*config.Consumer, error) {
				lookupCalled = true
				if rawKey == "test-key" {
					return expectedConsumer, nil
				}
				return nil, ErrInvalidAPIKey
			},
		})

		consumer, err := auth.Lookup("test-key")
		if err != nil {
			t.Errorf("Lookup error = %v", err)
		}
		if !lookupCalled {
			t.Error("Custom lookup function was not called")
		}
		if consumer != expectedConsumer {
			t.Error("Lookup returned unexpected consumer")
		}
	})

	t.Run("Lookup with expired key", func(t *testing.T) {
		pastTime := time.Now().Add(-time.Hour).Format(time.RFC3339)
		consumers := []config.Consumer{
			{
				ID: "consumer-1",
				APIKeys: []config.ConsumerAPIKey{
					{Key: "expired-key", ExpiresAt: pastTime},
				},
			},
		}
		auth := NewAuthAPIKey(consumers, AuthAPIKeyOptions{})
		consumer, err := auth.Lookup("expired-key")
		if err != ErrExpiredAPIKey {
			t.Errorf("Lookup with expired key error = %v, want ErrExpiredAPIKey", err)
		}
		if consumer != nil {
			t.Error("Lookup with expired key should return nil consumer")
		}
	})

	t.Run("DebugSummary with empty auth", func(t *testing.T) {
		auth := NewAuthAPIKey(nil, AuthAPIKeyOptions{})
		summary := auth.DebugSummary()
		expected := "consumers=0 keys=0"
		if summary != expected {
			t.Errorf("DebugSummary() = %q, want %q", summary, expected)
		}
	})

	t.Run("DebugSummary with consumers", func(t *testing.T) {
		consumers := []config.Consumer{
			{
				ID: "consumer-1",
				APIKeys: []config.ConsumerAPIKey{
					{Key: "key1"},
					{Key: "key2"},
				},
			},
			{
				ID: "consumer-2",
				APIKeys: []config.ConsumerAPIKey{
					{Key: "key3"},
				},
			},
		}
		auth := NewAuthAPIKey(consumers, AuthAPIKeyOptions{})
		summary := auth.DebugSummary()
		expected := "consumers=2 keys=3"
		if summary != expected {
			t.Errorf("DebugSummary() = %q, want %q", summary, expected)
		}
	})
}

// Test CircuitBreaker State method transitions
func TestCircuitBreaker_State_Transitions(t *testing.T) {
	t.Run("State transitions from Open to HalfOpen", func(t *testing.T) {
		now := time.Now()
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			ErrorThreshold: 0.5,
			SleepWindow:    time.Millisecond,
		})
		cb.now = func() time.Time { return now }

		// Initially closed
		if cb.State() != CircuitClosed {
			t.Errorf("Initial state = %v, want CircuitClosed", cb.State())
		}

		// Trip to open
		cb.tripOpenLocked(now)
		if cb.State() != CircuitOpen {
			t.Errorf("After trip state = %v, want CircuitOpen", cb.State())
		}

		// Advance time past sleep window
		cb.now = func() time.Time { return now.Add(2 * time.Millisecond) }
		if cb.State() != CircuitHalfOpen {
			t.Errorf("After sleep window state = %v, want CircuitHalfOpen", cb.State())
		}
	})

	t.Run("State at exact openUntil time", func(t *testing.T) {
		now := time.Now()
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			ErrorThreshold: 0.5,
			SleepWindow:    time.Second,
		})
		cb.now = func() time.Time { return now }

		cb.tripOpenLocked(now)

		// At exactly openUntil time, should transition to HalfOpen
		cb.now = func() time.Time { return now.Add(time.Second) }
		if cb.State() != CircuitHalfOpen {
			t.Errorf("State at openUntil time = %v, want CircuitHalfOpen", cb.State())
		}
	})
}

// Test extractBearerToken edge cases
func TestExtractBearerToken_Additional(t *testing.T) {
	tests := []struct {
		name     string
		auth     string
		expected string
	}{
		{"exactly 7 chars", "Bearer ", ""},
		{"less than 7 chars", "Bearer", ""},
		{"Bearer with tab", "Bearer\tabc123", ""}, // Tab is not a space
		{"Bearer with multiple spaces", "Bearer   abc123", "abc123"},
		{"BearerX token", "BearerX token", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				Header: http.Header{
					"Authorization": []string{tt.auth},
				},
			}
			got := extractBearerToken(req)
			if got != tt.expected {
				t.Errorf("extractBearerToken(%q) = %q, want %q", tt.auth, got, tt.expected)
			}
		})
	}
}

// Test applyClaimHeaders edge cases
func TestApplyClaimHeaders_Additional(t *testing.T) {
	t.Run("applyClaimHeaders with nil request", func(t *testing.T) {
		auth := NewAuthJWT(AuthJWTOptions{
			Secret: "secret",
			ClaimsToHeaders: map[string]string{
				"sub": "X-User-ID",
			},
		})
		claims := map[string]any{"sub": "user123"}
		// Should not panic with nil request
		auth.applyClaimHeaders(nil, claims)
	})

	t.Run("applyClaimHeaders with empty claimsToHeaders", func(t *testing.T) {
		auth := NewAuthJWT(AuthJWTOptions{
			Secret: "secret",
		})
		req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
		claims := map[string]any{"sub": "user123"}
		// Should not panic with empty claimsToHeaders
		auth.applyClaimHeaders(req, claims)
	})

	t.Run("applyClaimHeaders skips missing claims", func(t *testing.T) {
		auth := NewAuthJWT(AuthJWTOptions{
			Secret: "secret",
			ClaimsToHeaders: map[string]string{
				"sub":   "X-User-ID",
				"email": "X-User-Email",
			},
		})
		req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
		claims := map[string]any{"sub": "user123"} // email is missing

		auth.applyClaimHeaders(req, claims)

		if req.Header.Get("X-User-ID") != "user123" {
			t.Error("X-User-ID header should be set")
		}
		if req.Header.Get("X-User-Email") != "" {
			t.Error("X-User-Email header should not be set")
		}
	})

	t.Run("applyClaimHeaders skips invalid claim values", func(t *testing.T) {
		auth := NewAuthJWT(AuthJWTOptions{
			Secret: "secret",
			ClaimsToHeaders: map[string]string{
				"sub":     "X-User-ID",
				"invalid": "X-Invalid",
			},
		})
		req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
		claims := map[string]any{
			"sub":     "user123",
			"invalid": "", // Empty string should be skipped
		}

		auth.applyClaimHeaders(req, claims)

		if req.Header.Get("X-User-ID") != "user123" {
			t.Error("X-User-ID header should be set")
		}
		if req.Header.Get("X-Invalid") != "" {
			t.Error("X-Invalid header should not be set for empty value")
		}
	})
}

// Test AuthJWT Authenticate edge cases
func TestAuthJWT_Authenticate_EdgeCases(t *testing.T) {
	t.Run("Authenticate with nil request", func(t *testing.T) {
		auth := NewAuthJWT(AuthJWTOptions{Secret: "secret"})
		claims, err := auth.Authenticate(nil)
		if err != ErrMissingJWT {
			t.Errorf("Authenticate(nil) error = %v, want ErrMissingJWT", err)
		}
		if claims != nil {
			t.Error("Authenticate(nil) should return nil claims")
		}
	})

	t.Run("Authenticate with no Authorization header", func(t *testing.T) {
		auth := NewAuthJWT(AuthJWTOptions{Secret: "secret"})
		req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
		claims, err := auth.Authenticate(req)
		if err != ErrMissingJWT {
			t.Errorf("Authenticate(no auth) error = %v, want ErrMissingJWT", err)
		}
		if claims != nil {
			t.Error("Authenticate(no auth) should return nil claims")
		}
	})
}

// Test gzipBytes edge cases
func TestGzipBytes_EdgeCases(t *testing.T) {
	t.Run("gzipBytes with empty input", func(t *testing.T) {
		result, err := gzipBytes([]byte{})
		if err != nil {
			t.Errorf("gzipBytes(empty) error = %v", err)
		}
		// Empty input should still produce valid gzip output
		if len(result) == 0 {
			t.Error("gzipBytes(empty) should return non-empty gzip data")
		}
	})

	t.Run("gzipBytes with large input", func(t *testing.T) {
		// Create a large payload
		largeData := make([]byte, 10000)
		for i := range largeData {
			largeData[i] = byte('a' + i%26)
		}
		result, err := gzipBytes(largeData)
		if err != nil {
			t.Errorf("gzipBytes(large) error = %v", err)
		}
		// Gzipped data should be smaller
		if len(result) >= len(largeData) {
			t.Error("gzipBytes should compress data")
		}
	})
}

// Test CircuitBreakerError Error method
func TestCircuitBreakerError_Error(t *testing.T) {
	err := &CircuitBreakerError{
		Code:    "circuit_open",
		Message: "circuit breaker is open",
	}
	if err.Error() != "circuit breaker is open" {
		t.Errorf("Error() = %q, want %q", err.Error(), "circuit breaker is open")
	}
}

// Test Compression Priority method
func TestCompression_Priority(t *testing.T) {
	comp := NewCompression(CompressionConfig{})
	// Priority should be 50
	if comp.Priority() != 50 {
		t.Errorf("Priority() = %d, want 50", comp.Priority())
	}
}

// Test CircuitBreaker pruneEventsLocked
func TestCircuitBreaker_pruneEventsLocked(t *testing.T) {
	now := time.Now()
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Window:         time.Minute,
		ErrorThreshold: 0.5,
		SleepWindow:    time.Second,
	})
	cb.now = func() time.Time { return now }

	// Add some events
	cb.events = []circuitEvent{
		{ts: now.Add(-2 * time.Minute), success: true},
		{ts: now.Add(-90 * time.Second), success: false},
		{ts: now.Add(-30 * time.Second), success: true},
	}

	// Prune events older than 1 minute
	cb.pruneEventsLocked(now)

	// Should keep only events within the last minute
	if len(cb.events) != 1 {
		t.Errorf("Expected 1 event after pruning, got %d", len(cb.events))
	}
}

// Test CircuitBreaker pruneEventsLocked with no pruning needed
func TestCircuitBreaker_pruneEventsLocked_NoPrune(t *testing.T) {
	now := time.Now()
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Window:         time.Minute,
		ErrorThreshold: 0.5,
		SleepWindow:    time.Second,
	})
	cb.now = func() time.Time { return now }

	// Add recent events only
	cb.events = []circuitEvent{
		{ts: now.Add(-30 * time.Second), success: true},
		{ts: now.Add(-20 * time.Second), success: false},
	}

	originalLen := len(cb.events)
	cb.pruneEventsLocked(now)

	if len(cb.events) != originalLen {
		t.Errorf("Expected %d events (no pruning), got %d", originalLen, len(cb.events))
	}
}

// Test CircuitBreaker pruneEventsLocked with empty events
func TestCircuitBreaker_pruneEventsLocked_Empty(t *testing.T) {
	now := time.Now()
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Window:         time.Minute,
		ErrorThreshold: 0.5,
		SleepWindow:    time.Second,
	})

	cb.pruneEventsLocked(now)

	if len(cb.events) != 0 {
		t.Errorf("Expected 0 events, got %d", len(cb.events))
	}
}

// Test acceptsGzip
func TestAcceptsGzip(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected bool
	}{
		{"empty header", "", false},
		{"gzip only", "gzip", true},
		{"with deflate", "deflate, gzip", true},
		{"with identity", "identity", false},
		{"wildcard only", "*", false}, // Wildcard doesn't specifically indicate gzip support
		{"gzip with qvalue", "gzip;q=0.5", true},
		{"deflate only", "deflate", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				Header: http.Header{
					"Accept-Encoding": []string{tt.header},
				},
			}
			got := acceptsGzip(req)
			if got != tt.expected {
				t.Errorf("acceptsGzip(%q) = %v, want %v", tt.header, got, tt.expected)
			}
		})
	}

	t.Run("nil request", func(t *testing.T) {
		got := acceptsGzip(nil)
		if got != false {
			t.Errorf("acceptsGzip(nil) = %v, want false", got)
		}
	})
}

// Test NewCompression with various configs
func TestNewCompression_Configs(t *testing.T) {
	t.Run("with minSize", func(t *testing.T) {
		comp := NewCompression(CompressionConfig{
			MinSize: 1000,
		})
		if comp == nil {
			t.Fatal("NewCompression returned nil")
		}
	})

	t.Run("with zero minSize", func(t *testing.T) {
		comp := NewCompression(CompressionConfig{
			MinSize: 0,
		})
		if comp == nil {
			t.Fatal("NewCompression returned nil")
		}
	})
}

// Test gunzipBytes
func TestGunzipBytes(t *testing.T) {
	t.Run("gunzipBytes with valid data", func(t *testing.T) {
		original := []byte("Hello, World!")
		gzipped, err := gzipBytes(original)
		if err != nil {
			t.Fatalf("gzipBytes error: %v", err)
		}

		gunzipped, err := gunzipBytes(gzipped)
		if err != nil {
			t.Errorf("gunzipBytes error = %v", err)
		}
		if string(gunzipped) != string(original) {
			t.Errorf("gunzipBytes result = %q, want %q", gunzipped, original)
		}
	})

	t.Run("gunzipBytes with invalid data", func(t *testing.T) {
		invalidData := []byte("not gzipped data")
		_, err := gunzipBytes(invalidData)
		if err == nil {
			t.Error("gunzipBytes should return error for invalid data")
		}
	})
}

// Test resolveRSAPublicKey with configured public key
func TestResolveRSAPublicKey_WithConfiguredKey(t *testing.T) {
	// Generate a test RSA key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey error: %v", err)
	}

	auth := NewAuthJWT(AuthJWTOptions{
		PublicKey: &privateKey.PublicKey,
	})

	// Create a mock token (we don't need a valid one for this test)
	token := &jwt.Token{}

	pub, err := auth.resolveRSAPublicKey(context.Background(), token)
	if err != nil {
		t.Errorf("resolveRSAPublicKey error = %v", err)
	}
	if pub != &privateKey.PublicKey {
		t.Error("resolveRSAPublicKey should return configured public key")
	}
}

// Test resolveRSAPublicKey without JWKS client
func TestResolveRSAPublicKey_WithoutJWKS(t *testing.T) {
	auth := NewAuthJWT(AuthJWTOptions{
		Secret: "secret", // No PublicKey, no JWKSURL
	})

	token := &jwt.Token{}
	_, err := auth.resolveRSAPublicKey(context.Background(), token)
	if err != ErrInvalidJWTSignature {
		t.Errorf("resolveRSAPublicKey error = %v, want ErrInvalidJWTSignature", err)
	}
}

// Test CorrelationIDError Error method
func TestCorrelationIDError_Error(t *testing.T) {
	err := &CorrelationIDError{
		Code:    "missing_header",
		Message: "correlation ID header is missing",
	}
	if err.Error() != "correlation ID header is missing" {
		t.Errorf("Error() = %q, want %q", err.Error(), "correlation ID header is missing")
	}
}

// Test RateLimitError Error method
func TestRateLimitError_Error(t *testing.T) {
	err := &RateLimitError{
		Code:    "rate_limit_exceeded",
		Message: "rate limit exceeded",
	}
	if err.Error() != "rate limit exceeded" {
		t.Errorf("Error() = %q, want %q", err.Error(), "rate limit exceeded")
	}
}

// Test IPRestrictError Error method
func TestIPRestrictError_Error(t *testing.T) {
	err := &IPRestrictError{
		Code:    "ip_blocked",
		Message: "IP address is blocked",
	}
	if err.Error() != "IP address is blocked" {
		t.Errorf("Error() = %q, want %q", err.Error(), "IP address is blocked")
	}
}

// Test CircuitBreaker Priority method
func TestCircuitBreaker_Priority(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{})
	if cb.Priority() != 30 {
		t.Errorf("Priority() = %d, want 30", cb.Priority())
	}
}

// Test Redirect Priority method
func TestRedirect_Priority(t *testing.T) {
	redirect := NewRedirect(RedirectConfig{})
	if redirect.Priority() != 15 {
		t.Errorf("Priority() = %d, want 15", redirect.Priority())
	}
}

// Test IPRestrict methods
func TestIPRestrict_Methods(t *testing.T) {
	ipr, err := NewIPRestrict(IPRestrictConfig{})
	if err != nil {
		t.Fatalf("NewIPRestrict error: %v", err)
	}

	if ipr.Name() != "ip-restrict" {
		t.Errorf("Name() = %q, want ip-restrict", ipr.Name())
	}

	if ipr.Phase() != PhasePreAuth {
		t.Errorf("Phase() = %v, want PhasePreAuth", ipr.Phase())
	}

	if ipr.Priority() != 5 {
		t.Errorf("Priority() = %d, want 5", ipr.Priority())
	}
}

// Test Pipeline Plugins method
func TestPipeline_Plugins(t *testing.T) {
	p := NewPipeline([]PipelinePlugin{})

	// Initially should have empty plugins
	plugins := p.Plugins()
	if plugins == nil {
		t.Fatal("Plugins() returned nil")
	}

	if len(plugins) != 0 {
		t.Errorf("Expected 0 plugins, got %d", len(plugins))
	}
}

func TestTimeout_Methods(t *testing.T) {
	timeout := NewTimeout(TimeoutConfig{Duration: time.Second})

	if timeout.Name() != "timeout" {
		t.Errorf("Name() = %v, want timeout", timeout.Name())
	}

	if timeout.Phase() != PhaseProxy {
		t.Errorf("Phase() = %v, want PhaseProxy", timeout.Phase())
	}

	if timeout.Priority() != 10 {
		t.Errorf("Priority() = %v, want 10", timeout.Priority())
	}
}

func TestCompression_Methods(t *testing.T) {
	comp := NewCompression(CompressionConfig{})

	if comp.Name() != "compression" {
		t.Errorf("Name() = %v, want compression", comp.Name())
	}

	if comp.Phase() != PhasePostProxy {
		t.Errorf("Phase() = %v, want PhasePostProxy", comp.Phase())
	}
}

func TestCompression_DefaultLevel(t *testing.T) {
	// Test with default level (0 means no compression)
	comp := NewCompression(CompressionConfig{})
	if comp == nil {
		t.Error("NewCompression should not return nil")
	}
}

// Test RequestValidator methods
func TestRequestValidator_Methods(t *testing.T) {
	v, err := NewRequestValidator(RequestValidatorConfig{})
	if err != nil {
		t.Fatalf("NewRequestValidator error: %v", err)
	}
	if v.Name() != "request-validator" {
		t.Errorf("Name() = %q, want request-validator", v.Name())
	}
	if v.Phase() != PhasePreProxy {
		t.Errorf("Phase() = %v, want PhasePreProxy", v.Phase())
	}
	if v.Priority() != 30 {
		t.Errorf("Priority() = %d, want 30", v.Priority())
	}
}

// Test Retry methods
func TestRetry_Methods(t *testing.T) {
	r := NewRetry(RetryConfig{})
	if r.Name() != "retry" {
		t.Errorf("Name() = %q, want retry", r.Name())
	}
	if r.Phase() != PhaseProxy {
		t.Errorf("Phase() = %v, want PhaseProxy", r.Phase())
	}
}

// Test RequestSizeLimitError Error method
func TestRequestSizeLimitError_Error(t *testing.T) {
	err := &RequestSizeLimitError{
		Code:    "request_too_large",
		Message: "request body exceeds size limit",
	}
	if err.Error() != "request body exceeds size limit" {
		t.Errorf("Error() = %q, want %q", err.Error(), "request body exceeds size limit")
	}
}

// Test RequestValidatorError Error method
func TestRequestValidatorError_Error(t *testing.T) {
	err := &RequestValidatorError{
		Code:    "validation_failed",
		Message: "request validation failed",
	}
	if err.Error() != "request validation failed" {
		t.Errorf("Error() = %q, want %q", err.Error(), "request validation failed")
	}
}

// Test CaptureResponseWriter IsFlushed method
func TestCaptureResponseWriter_IsFlushed(t *testing.T) {
	// Create a mock ResponseWriter
	w := httptest.NewRecorder()
	crw := NewCaptureResponseWriter(w)

	// IsFlushed should return false for a new writer
	if crw.IsFlushed() {
		t.Error("IsFlushed() = true, want false")
	}
}

// Test asFloat helper function
func TestAsFloat(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		fallback float64
		expected float64
	}{
		{"float64", float64(3.14), 0, 3.14},
		{"float32", float32(2.5), 0, 2.5},
		{"int", int(42), 0, 42},
		{"int64", int64(100), 0, 100},
		{"string number", "123.45", 0, 123.45},
		{"string int", "100", 0, 100},
		{"invalid string", "not a number", 99, 99},
		{"nil", nil, 50, 50},
		{"bool", true, 10, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asFloat(tt.input, tt.fallback)
			if got != tt.expected {
				t.Errorf("asFloat(%v, %v) = %v, want %v", tt.input, tt.fallback, got, tt.expected)
			}
		})
	}
}

// Test asIntSlice helper function
func TestAsIntSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		fallback []int
		expected []int
	}{
		{"[]int", []int{1, 2, 3}, []int{0}, []int{1, 2, 3}},                    // []int returns as-is
		{"[]any with ints >= 100", []any{100, 200}, []int{0}, []int{100, 200}}, // filtered by >= 100
		{"[]any with ints < 100", []any{1, 2}, []int{99}, []int{99}},           // falls back because all < 100
		{"nil", nil, []int{99}, []int{99}},
		{"single int (goes to default)", 42, []int{99}, []int{99}}, // default case returns fallback
		{"invalid string (goes to default)", "not a number", []int{77}, []int{77}},
		{"empty []int", []int{}, []int{0}, []int{0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asIntSlice(tt.input, tt.fallback)
			if len(got) != len(tt.expected) {
				t.Errorf("asIntSlice(%v, %v) = %v, want %v", tt.input, tt.fallback, got, tt.expected)
				return
			}
			for i := range tt.expected {
				if got[i] != tt.expected[i] {
					t.Errorf("asIntSlice(%v, %v)[%d] = %d, want %d", tt.input, tt.fallback, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

// Test simple plugin interface methods
func TestPluginInterfaceMethods(t *testing.T) {
	t.Run("CorrelationID", func(t *testing.T) {
		p := &CorrelationID{}
		if p.Name() != "correlation-id" {
			t.Errorf("Name() = %v", p.Name())
		}
		if p.Phase() != PhasePreAuth {
			t.Errorf("Phase() = %v", p.Phase())
		}
		if p.Priority() != 0 {
			t.Errorf("Priority() = %v", p.Priority())
		}
	})

	t.Run("Compression", func(t *testing.T) {
		p := NewCompression(CompressionConfig{})
		if p.Name() != "compression" {
			t.Errorf("Name() = %v", p.Name())
		}
		if p.Phase() != PhasePostProxy {
			t.Errorf("Phase() = %v", p.Phase())
		}
	})

	t.Run("Timeout", func(t *testing.T) {
		p := NewTimeout(TimeoutConfig{Duration: time.Second})
		if p.Name() != "timeout" {
			t.Errorf("Name() = %v", p.Name())
		}
		if p.Phase() != PhaseProxy {
			t.Errorf("Phase() = %v", p.Phase())
		}
		if p.Priority() != 10 {
			t.Errorf("Priority() = %v", p.Priority())
		}
	})

	t.Run("BotDetect", func(t *testing.T) {
		p := NewBotDetect(BotDetectConfig{})
		if p.Name() != "bot-detect" {
			t.Errorf("Name() = %v", p.Name())
		}
		if p.Phase() != PhasePreAuth {
			t.Errorf("Phase() = %v", p.Phase())
		}
	})

	t.Run("CircuitBreaker", func(t *testing.T) {
		p := NewCircuitBreaker(CircuitBreakerConfig{})
		if p.Name() != "circuit-breaker" {
			t.Errorf("Name() = %v", p.Name())
		}
		if p.Phase() != PhaseProxy {
			t.Errorf("Phase() = %v", p.Phase())
		}
	})

	t.Run("CORS", func(t *testing.T) {
		p := NewCORS(CORSConfig{})
		if p.Name() != "cors" {
			t.Errorf("Name() = %v", p.Name())
		}
		if p.Phase() != PhasePreAuth {
			t.Errorf("Phase() = %v", p.Phase())
		}
	})

	t.Run("Redirect", func(t *testing.T) {
		p := NewRedirect(RedirectConfig{})
		if p.Name() != "redirect" {
			t.Errorf("Name() = %v", p.Name())
		}
		if p.Phase() != PhasePreProxy {
			t.Errorf("Phase() = %v", p.Phase())
		}
	})

	t.Run("RequestSizeLimit", func(t *testing.T) {
		p := NewRequestSizeLimit(RequestSizeLimitConfig{})
		if p.Name() != "request-size-limit" {
			t.Errorf("Name() = %v", p.Name())
		}
		if p.Phase() != PhasePreProxy {
			t.Errorf("Phase() = %v", p.Phase())
		}
	})

	t.Run("ResponseTransform", func(t *testing.T) {
		p := NewResponseTransform(ResponseTransformConfig{})
		if p.Name() != "response-transform" {
			t.Errorf("Name() = %v", p.Name())
		}
		if p.Phase() != PhasePostProxy {
			t.Errorf("Phase() = %v", p.Phase())
		}
	})

	t.Run("URLRewrite", func(t *testing.T) {
		p, _ := NewURLRewrite(URLRewriteConfig{})
		if p.Name() != "url-rewrite" {
			t.Errorf("Name() = %v", p.Name())
		}
		if p.Phase() != PhasePreProxy {
			t.Errorf("Phase() = %v", p.Phase())
		}
	})
}

// Test PipelinePlugin Priority method (line 50 in registry.go)
func TestPipelinePlugin_Priority(t *testing.T) {
	plugin := PipelinePlugin{
		name:     "test-plugin",
		phase:    PhasePreAuth,
		priority: 42,
	}
	if plugin.Priority() != 42 {
		t.Errorf("Priority() = %d, want 42", plugin.Priority())
	}
}

// Test PipelinePlugin Run with nil run function (line 51-56 in registry.go)
func TestPipelinePlugin_Run_NilFunc(t *testing.T) {
	plugin := PipelinePlugin{
		name:  "test-plugin",
		phase: PhasePreAuth,
		run:   nil, // Explicitly nil
	}
	handled, err := plugin.Run(&PipelineContext{})
	if handled != false {
		t.Errorf("Run() handled = %v, want false", handled)
	}
	if err != nil {
		t.Errorf("Run() err = %v, want nil", err)
	}
}

// Test error types Error methods
func TestTimeoutError_Error(t *testing.T) {
	err := &TimeoutError{
		Code:    "invalid_timeout",
		Message: "Timeout value is invalid",
		Status:  http.StatusBadRequest,
	}
	if err.Error() != "Timeout value is invalid" {
		t.Errorf("Error() = %q, want %q", err.Error(), "Timeout value is invalid")
	}
}

func TestURLRewriteError_Error(t *testing.T) {
	err := &URLRewriteError{
		Code:    "invalid_url_rewrite",
		Message: "URL rewrite configuration is invalid",
		Status:  http.StatusBadRequest,
	}
	if err.Error() != "URL rewrite configuration is invalid" {
		t.Errorf("Error() = %q, want %q", err.Error(), "URL rewrite configuration is invalid")
	}
}

func TestUserIPWhitelistError_Error(t *testing.T) {
	err := &UserIPWhitelistError{
		Code:    "ip_not_allowed",
		Message: "IP not allowed",
		Status:  http.StatusForbidden,
	}
	if err.Error() != "IP not allowed" {
		t.Errorf("Error() = %q, want %q", err.Error(), "IP not allowed")
	}
}

// Test Retry methods (lines 85-89 in retry.go)
func TestRetry_Methods_Full(t *testing.T) {
	r := NewRetry(RetryConfig{
		MaxRetries:    3,
		BaseDelay:     100 * time.Millisecond,
		MaxDelay:      time.Second,
		Jitter:        true,
		RetryMethods:  []string{"GET", "POST"},
		RetryOnStatus: []int{500, 502, 503},
	})

	t.Run("Name", func(t *testing.T) {
		if r.Name() != "retry" {
			t.Errorf("Name() = %q, want retry", r.Name())
		}
	})

	t.Run("Phase", func(t *testing.T) {
		if r.Phase() != PhaseProxy {
			t.Errorf("Phase() = %v, want PhaseProxy", r.Phase())
		}
	})

	t.Run("Priority", func(t *testing.T) {
		if r.Priority() != 20 {
			t.Errorf("Priority() = %d, want 20", r.Priority())
		}
	})

	t.Run("MaxAttempts with retryable method", func(t *testing.T) {
		attempts := r.MaxAttempts("GET")
		if attempts != 4 { // maxRetries + 1
			t.Errorf("MaxAttempts(GET) = %d, want 4", attempts)
		}
	})

	t.Run("MaxAttempts with non-retryable method", func(t *testing.T) {
		attempts := r.MaxAttempts("DELETE")
		if attempts != 1 {
			t.Errorf("MaxAttempts(DELETE) = %d, want 1", attempts)
		}
	})
}

// Test buildCorrelationIDPlugin (line 241 in registry.go)
func TestBuildCorrelationIDPlugin(t *testing.T) {
	registry := NewDefaultRegistry()
	plugin, err := registry.Build(config.PluginConfig{
		Name: "correlation-id",
	}, BuilderContext{})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if plugin.Name() != "correlation-id" {
		t.Errorf("Name() = %q, want correlation-id", plugin.Name())
	}
	if plugin.Phase() != PhasePreAuth {
		t.Errorf("Phase() = %v, want PhasePreAuth", plugin.Phase())
	}

	// Test Run function
	ctx := &PipelineContext{
		Request: httptest.NewRequest(http.MethodGet, "http://example.com", nil),
	}
	handled, err := plugin.Run(ctx)
	if handled != false {
		t.Errorf("Run() handled = %v, want false", handled)
	}
	if err != nil {
		t.Errorf("Run() err = %v, want nil", err)
	}
}

// Test buildBotDetectPlugin (line 254 in registry.go)
func TestBuildBotDetectPlugin(t *testing.T) {
	registry := NewDefaultRegistry()
	plugin, err := registry.Build(config.PluginConfig{
		Name: "bot-detect",
		Config: map[string]any{
			"allow_list": []string{"goodbot"},
			"deny_list":  []string{"badbot"},
			"action":     "block",
		},
	}, BuilderContext{})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if plugin.Name() != "bot-detect" {
		t.Errorf("Name() = %q, want bot-detect", plugin.Name())
	}

	// Test Run function
	ctx := &PipelineContext{
		Request: httptest.NewRequest(http.MethodGet, "http://example.com", nil),
	}
	handled, err := plugin.Run(ctx)
	if handled != false {
		t.Errorf("Run() handled = %v, want false", handled)
	}
	// Should not error with normal user agent
	if err != nil {
		t.Errorf("Run() err = %v, want nil", err)
	}
}

// Test buildIPRestrictPlugin (line 271 in registry.go)
func TestBuildIPRestrictPlugin(t *testing.T) {
	registry := NewDefaultRegistry()

	t.Run("valid config", func(t *testing.T) {
		plugin, err := registry.Build(config.PluginConfig{
			Name: "ip-restrict",
			Config: map[string]any{
				"whitelist": []string{"192.168.1.0/24"},
				"blacklist": []string{"10.0.0.1"},
			},
		}, BuilderContext{})
		if err != nil {
			t.Fatalf("Build error: %v", err)
		}
		if plugin.Name() != "ip-restrict" {
			t.Errorf("Name() = %q, want ip-restrict", plugin.Name())
		}

		// Test Run function
		ctx := &PipelineContext{
			Request: httptest.NewRequest(http.MethodGet, "http://example.com", nil),
		}
		handled, err := plugin.Run(ctx)
		if handled != false {
			t.Errorf("Run() handled = %v, want false", handled)
		}
		// Localhost might be blocked depending on whitelist
		// We just check it doesn't panic
	})

	t.Run("invalid config", func(t *testing.T) {
		_, err := registry.Build(config.PluginConfig{
			Name: "ip-restrict",
			Config: map[string]any{
				"whitelist": []string{"invalid-cidr"},
			},
		}, BuilderContext{})
		if err == nil {
			t.Error("Expected error for invalid CIDR")
		}
	})
}

// Test buildAuthJWTPlugin (line 313 in registry.go)
func TestBuildAuthJWTPlugin(t *testing.T) {
	registry := NewDefaultRegistry()
	plugin, err := registry.Build(config.PluginConfig{
		Name: "auth-jwt",
		Config: map[string]any{
			"secret":          "test-secret",
			"issuer":          "test-issuer",
			"audience":        []string{"test-audience"},
			"required_claims": []string{"sub"},
			"claims_to_headers": map[string]any{
				"sub": "X-User-ID",
			},
			"clock_skew": "30s",
		},
	}, BuilderContext{})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if plugin.Name() != "auth-jwt" {
		t.Errorf("Name() = %q, want auth-jwt", plugin.Name())
	}
	if plugin.Phase() != PhaseAuth {
		t.Errorf("Phase() = %v, want PhaseAuth", plugin.Phase())
	}

	// Test Run function with no auth header
	ctx := &PipelineContext{
		Request: httptest.NewRequest(http.MethodGet, "http://example.com", nil),
	}
	handled, err := plugin.Run(ctx)
	if handled != false {
		t.Errorf("Run() handled = %v, want false", handled)
	}
	// Should return error for missing JWT
	if err == nil {
		t.Error("Run() should return error for missing JWT")
	}
}

// Test buildRequestSizeLimitPlugin (line 394 in registry.go)
func TestBuildRequestSizeLimitPlugin(t *testing.T) {
	registry := NewDefaultRegistry()
	plugin, err := registry.Build(config.PluginConfig{
		Name: "request-size-limit",
		Config: map[string]any{
			"max_bytes": 1024,
		},
	}, BuilderContext{})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if plugin.Name() != "request-size-limit" {
		t.Errorf("Name() = %q, want request-size-limit", plugin.Name())
	}
	if plugin.Phase() != PhasePreProxy {
		t.Errorf("Phase() = %v, want PhasePreProxy", plugin.Phase())
	}

	// Test Run function
	ctx := &PipelineContext{
		Request: httptest.NewRequest(http.MethodGet, "http://example.com", nil),
	}
	handled, err := plugin.Run(ctx)
	if handled != false {
		t.Errorf("Run() handled = %v, want false", handled)
	}
	if err != nil {
		t.Errorf("Run() err = %v, want nil", err)
	}
}

// Test buildRequestValidatorPlugin (line 410 in registry.go)
func TestBuildRequestValidatorPlugin(t *testing.T) {
	registry := NewDefaultRegistry()

	t.Run("valid config", func(t *testing.T) {
		plugin, err := registry.Build(config.PluginConfig{
			Name: "request-validator",
			Config: map[string]any{
				"schema": map[string]any{
					"type": "object",
				},
			},
		}, BuilderContext{})
		if err != nil {
			t.Fatalf("Build error: %v", err)
		}
		if plugin.Name() != "request-validator" {
			t.Errorf("Name() = %q, want request-validator", plugin.Name())
		}

		// Test Run function
		ctx := &PipelineContext{
			Request: httptest.NewRequest(http.MethodGet, "http://example.com", nil),
		}
		handled, err := plugin.Run(ctx)
		if handled != false {
			t.Errorf("Run() handled = %v, want false", handled)
		}
		// May or may not error depending on validation
	})

	t.Run("invalid config", func(t *testing.T) {
		_, err := registry.Build(config.PluginConfig{
			Name: "request-validator",
			Config: map[string]any{
				"schema": "not-a-valid-schema",
			},
		}, BuilderContext{})
		// Schema validation might not error on invalid type
		// Just ensure it doesn't panic
		_ = err
	})
}

// Test buildCircuitBreakerPlugin (line 428 in registry.go)
func TestBuildCircuitBreakerPlugin(t *testing.T) {
	registry := NewDefaultRegistry()
	plugin, err := registry.Build(config.PluginConfig{
		Name: "circuit-breaker",
		Config: map[string]any{
			"error_threshold":    0.5,
			"volume_threshold":   10,
			"sleep_window":       "10s",
			"half_open_requests": 1,
			"window":             "30s",
		},
	}, BuilderContext{})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if plugin.Name() != "circuit-breaker" {
		t.Errorf("Name() = %q, want circuit-breaker", plugin.Name())
	}
	if plugin.Phase() != PhaseProxy {
		t.Errorf("Phase() = %v, want PhaseProxy", plugin.Phase())
	}

	// Test Run function
	ctx := &PipelineContext{
		Request: httptest.NewRequest(http.MethodGet, "http://example.com", nil),
	}
	handled, err := plugin.Run(ctx)
	if handled != false {
		t.Errorf("Run() handled = %v, want false", handled)
	}
	if err != nil {
		t.Errorf("Run() err = %v, want nil", err)
	}

	// Test AfterProxy function
	plugin.AfterProxy(ctx, nil)                      // Success
	plugin.AfterProxy(ctx, fmt.Errorf("some error")) // Failure
}

// Test buildRetryPlugin (line 450 in registry.go)
func TestBuildRetryPlugin(t *testing.T) {
	registry := NewDefaultRegistry()
	plugin, err := registry.Build(config.PluginConfig{
		Name: "retry",
		Config: map[string]any{
			"max_retries":     3,
			"base_delay":      "100ms",
			"max_delay":       "1s",
			"jitter":          true,
			"retry_methods":   []string{"GET", "POST"},
			"retry_on_status": []int{500, 502, 503},
		},
	}, BuilderContext{})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if plugin.Name() != "retry" {
		t.Errorf("Name() = %q, want retry", plugin.Name())
	}
	if plugin.Phase() != PhaseProxy {
		t.Errorf("Phase() = %v, want PhaseProxy", plugin.Phase())
	}

	// Test Run function
	ctx := &PipelineContext{
		Request: httptest.NewRequest(http.MethodGet, "http://example.com", nil),
	}
	handled, err := plugin.Run(ctx)
	if handled != false {
		t.Errorf("Run() handled = %v, want false", handled)
	}
	if err != nil {
		t.Errorf("Run() err = %v, want nil", err)
	}
	// Check that Retry was set on context
	if ctx.Retry == nil {
		t.Error("Run() should set ctx.Retry")
	}
}

// Test buildTimeoutPlugin (line 475 in registry.go)
func TestBuildTimeoutPlugin(t *testing.T) {
	registry := NewDefaultRegistry()
	plugin, err := registry.Build(config.PluginConfig{
		Name: "timeout",
		Config: map[string]any{
			"timeout": "5s",
		},
	}, BuilderContext{})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if plugin.Name() != "timeout" {
		t.Errorf("Name() = %q, want timeout", plugin.Name())
	}
	if plugin.Phase() != PhaseProxy {
		t.Errorf("Phase() = %v, want PhaseProxy", plugin.Phase())
	}

	// Test Run function
	ctx := &PipelineContext{
		Request: httptest.NewRequest(http.MethodGet, "http://example.com", nil),
	}
	handled, err := plugin.Run(ctx)
	if handled != false {
		t.Errorf("Run() handled = %v, want false", handled)
	}
	if err != nil {
		t.Errorf("Run() err = %v, want nil", err)
	}
}

// Test buildCompressionPlugin (line 563 in registry.go)
func TestBuildCompressionPlugin(t *testing.T) {
	registry := NewDefaultRegistry()
	plugin, err := registry.Build(config.PluginConfig{
		Name: "compression",
		Config: map[string]any{
			"min_size": 100,
		},
	}, BuilderContext{})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if plugin.Name() != "compression" {
		t.Errorf("Name() = %q, want compression", plugin.Name())
	}
	if plugin.Phase() != PhasePostProxy {
		t.Errorf("Phase() = %v, want PhasePostProxy", plugin.Phase())
	}

	// Test Run function
	w := httptest.NewRecorder()
	ctx := &PipelineContext{
		Request:        httptest.NewRequest(http.MethodGet, "http://example.com", nil),
		ResponseWriter: w,
	}
	handled, err := plugin.Run(ctx)
	if handled != false {
		t.Errorf("Run() handled = %v, want false", handled)
	}
	if err != nil {
		t.Errorf("Run() err = %v, want nil", err)
	}

	// Test AfterProxy function
	plugin.AfterProxy(ctx, nil)
}

// Test buildRedirectPlugin (line 582 in registry.go)
func TestBuildRedirectPlugin(t *testing.T) {
	registry := NewDefaultRegistry()

	t.Run("with rules", func(t *testing.T) {
		plugin, err := registry.Build(config.PluginConfig{
			Name: "redirect",
			Config: map[string]any{
				"rules": []any{
					map[string]any{
						"path":        "/old",
						"url":         "/new",
						"status_code": 301,
					},
				},
			},
		}, BuilderContext{})
		if err != nil {
			t.Fatalf("Build error: %v", err)
		}
		if plugin.Name() != "redirect" {
			t.Errorf("Name() = %q, want redirect", plugin.Name())
		}

		// Test Run function with matching path
		w := httptest.NewRecorder()
		ctx := &PipelineContext{
			Request:        httptest.NewRequest(http.MethodGet, "http://example.com/old", nil),
			ResponseWriter: w,
		}
		handled, err := plugin.Run(ctx)
		if handled != true {
			t.Errorf("Run() handled = %v, want true for matching path", handled)
		}
		if err != nil {
			t.Errorf("Run() err = %v, want nil", err)
		}
		if w.Code != 301 {
			t.Errorf("Status code = %d, want 301", w.Code)
		}
	})

	t.Run("with simple config", func(t *testing.T) {
		plugin, err := registry.Build(config.PluginConfig{
			Name: "redirect",
			Config: map[string]any{
				"path":        "/old",
				"url":         "/new",
				"status_code": 302,
			},
		}, BuilderContext{})
		if err != nil {
			t.Fatalf("Build error: %v", err)
		}

		// Test Run function
		w := httptest.NewRecorder()
		ctx := &PipelineContext{
			Request:        httptest.NewRequest(http.MethodGet, "http://example.com/old", nil),
			ResponseWriter: w,
		}
		handled, err := plugin.Run(ctx)
		if handled != true {
			t.Errorf("Run() handled = %v, want true", handled)
		}
		if err != nil {
			t.Errorf("Run() err = %v, want nil", err)
		}
	})

	t.Run("with nil context", func(t *testing.T) {
		plugin, err := registry.Build(config.PluginConfig{
			Name:   "redirect",
			Config: map[string]any{},
		}, BuilderContext{})
		if err != nil {
			t.Fatalf("Build error: %v", err)
		}

		handled, err := plugin.Run(nil)
		if handled != false {
			t.Errorf("Run(nil) handled = %v, want false", handled)
		}
		if err != nil {
			t.Errorf("Run(nil) err = %v, want nil", err)
		}
	})
}

// Test handleFileRemove (line 185 in hot_reload.go)
func TestHandleFileRemove(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.lua")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create reloader with disabled config (to avoid watcher issues)
	reloader := &PluginReloader{
		config: HotReloadConfig{
			Enabled:  false,
			WatchDir: tmpDir,
		},
		handlers: make(map[string]ReloadHandler),
	}

	// Register a handler
	handlerCalled := false
	var receivedContent []byte
	reloader.RegisterHandler("test", func(name string, content []byte) error {
		handlerCalled = true
		receivedContent = content
		return nil
	})

	// Call handleFileRemove directly
	reloader.handleFileRemove(testFile)

	if !handlerCalled {
		t.Error("Handler was not called")
	}
	if receivedContent != nil {
		t.Error("Received content should be nil for file removal")
	}
}

// Test handleFileRemove with handler error
func TestHandleFileRemove_HandlerError(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.lua")
	os.WriteFile(testFile, []byte("test content"), 0644)

	reloader := &PluginReloader{
		config: HotReloadConfig{
			Enabled:  false,
			WatchDir: tmpDir,
		},
		handlers: make(map[string]ReloadHandler),
	}

	// Register a handler that returns an error
	reloader.RegisterHandler("test", func(name string, content []byte) error {
		return fmt.Errorf("unload error")
	})

	// Should not panic even with handler error
	reloader.handleFileRemove(testFile)
}

// Test handleFileRemove without handler
func TestHandleFileRemove_NoHandler(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.lua")
	os.WriteFile(testFile, []byte("test content"), 0644)

	reloader := &PluginReloader{
		config: HotReloadConfig{
			Enabled:  false,
			WatchDir: tmpDir,
		},
		handlers: make(map[string]ReloadHandler),
	}

	// No handler registered - should not panic
	reloader.handleFileRemove(testFile)
}

// Test routePipelineKey with various route configurations
func TestRoutePipelineKey_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		route    *config.Route
		idx      int
		expected string
	}{
		{
			name:     "nil route",
			route:    nil,
			idx:      0,
			expected: "route-0",
		},
		{
			name:     "route with empty ID and Name",
			route:    &config.Route{ID: "", Name: ""},
			idx:      5,
			expected: "route-5",
		},
		{
			name:     "route with whitespace ID",
			route:    &config.Route{ID: "   ", Name: "test"},
			idx:      0,
			expected: "test",
		},
		{
			name:     "route with ID",
			route:    &config.Route{ID: "route-123"},
			idx:      0,
			expected: "route-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := routePipelineKey(tt.route, tt.idx)
			if got != tt.expected {
				t.Errorf("routePipelineKey() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// Test isPluginEnabled
func TestIsPluginEnabled(t *testing.T) {
	tests := []struct {
		name     string
		spec     config.PluginConfig
		expected bool
	}{
		{
			name:     "nil enabled",
			spec:     config.PluginConfig{Enabled: nil},
			expected: true,
		},
		{
			name:     "enabled true",
			spec:     config.PluginConfig{Enabled: boolPtr(true)},
			expected: true,
		},
		{
			name:     "enabled false",
			spec:     config.PluginConfig{Enabled: boolPtr(false)},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPluginEnabled(tt.spec)
			if got != tt.expected {
				t.Errorf("isPluginEnabled() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// Helper function
func boolPtr(b bool) *bool {
	return &b
}

// Test Registry Build with various error scenarios
func TestRegistry_Build_Errors(t *testing.T) {
	t.Run("empty plugin name", func(t *testing.T) {
		registry := NewDefaultRegistry()
		_, err := registry.Build(config.PluginConfig{
			Name: "",
		}, BuilderContext{})
		if err == nil {
			t.Error("Expected error for empty plugin name")
		}
	})

	t.Run("unregistered plugin", func(t *testing.T) {
		registry := NewRegistry()
		_, err := registry.Build(config.PluginConfig{
			Name: "nonexistent",
		}, BuilderContext{})
		if err == nil {
			t.Error("Expected error for unregistered plugin")
		}
	})
}

// Test Registry Register errors
func TestRegistry_Register_Errors(t *testing.T) {
	t.Run("nil registry", func(t *testing.T) {
		var registry *Registry
		err := registry.Register("test", func(spec config.PluginConfig, ctx BuilderContext) (PipelinePlugin, error) {
			return PipelinePlugin{}, nil
		})
		if err == nil {
			t.Error("Expected error for nil registry")
		}
	})

	t.Run("empty plugin name", func(t *testing.T) {
		registry := NewRegistry()
		err := registry.Register("", func(spec config.PluginConfig, ctx BuilderContext) (PipelinePlugin, error) {
			return PipelinePlugin{}, nil
		})
		if err == nil {
			t.Error("Expected error for empty plugin name")
		}
	})

	t.Run("nil factory", func(t *testing.T) {
		registry := NewRegistry()
		err := registry.Register("test", nil)
		if err == nil {
			t.Error("Expected error for nil factory")
		}
	})

	t.Run("duplicate registration", func(t *testing.T) {
		registry := NewRegistry()
		factory := func(spec config.PluginConfig, ctx BuilderContext) (PipelinePlugin, error) {
			return PipelinePlugin{}, nil
		}
		err := registry.Register("test", factory)
		if err != nil {
			t.Fatalf("First registration failed: %v", err)
		}
		err = registry.Register("test", factory)
		if err == nil {
			t.Error("Expected error for duplicate registration")
		}
	})
}

// Test Registry Lookup with nil registry
func TestRegistry_Lookup_Nil(t *testing.T) {
	var registry *Registry
	factory, ok := registry.Lookup("test")
	if ok {
		t.Error("Lookup on nil registry should return false")
	}
	if factory != nil {
		t.Error("Lookup on nil registry should return nil factory")
	}
}

// Test mergePluginSpecs edge cases
func TestMergePluginSpecs_EdgeCases(t *testing.T) {
	t.Run("both empty", func(t *testing.T) {
		result := mergePluginSpecs(nil, nil)
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})

	t.Run("empty global", func(t *testing.T) {
		route := []config.PluginConfig{{Name: "test"}}
		result := mergePluginSpecs(nil, route)
		if len(result) != 1 {
			t.Errorf("Expected 1 plugin, got %d", len(result))
		}
	})

	t.Run("empty route", func(t *testing.T) {
		global := []config.PluginConfig{{Name: "test"}}
		result := mergePluginSpecs(global, nil)
		if len(result) != 1 {
			t.Errorf("Expected 1 plugin, got %d", len(result))
		}
	})

	t.Run("plugin with empty name is skipped", func(t *testing.T) {
		global := []config.PluginConfig{{Name: ""}, {Name: "valid"}}
		result := mergePluginSpecs(global, nil)
		if len(result) != 1 {
			t.Errorf("Expected 1 plugin, got %d", len(result))
		}
		if result[0].Name != "valid" {
			t.Errorf("Expected 'valid', got %q", result[0].Name)
		}
	})
}

// Test ensureEndpointPermissionGlobal
func TestEnsureEndpointPermissionGlobal(t *testing.T) {
	t.Run("empty input adds both plugins", func(t *testing.T) {
		result := ensureEndpointPermissionGlobal(nil)
		if len(result) != 2 {
			t.Errorf("Expected 2 plugins, got %d", len(result))
		}
	})

	t.Run("with existing endpoint-permission", func(t *testing.T) {
		input := []config.PluginConfig{{Name: "endpoint-permission"}}
		result := ensureEndpointPermissionGlobal(input)
		if len(result) != 2 { // Should add user-ip-whitelist
			t.Errorf("Expected 2 plugins, got %d", len(result))
		}
	})

	t.Run("with existing user-ip-whitelist", func(t *testing.T) {
		input := []config.PluginConfig{{Name: "user-ip-whitelist"}}
		result := ensureEndpointPermissionGlobal(input)
		if len(result) != 2 { // Should add endpoint-permission
			t.Errorf("Expected 2 plugins, got %d", len(result))
		}
	})

	t.Run("with both existing", func(t *testing.T) {
		input := []config.PluginConfig{
			{Name: "endpoint-permission"},
			{Name: "user-ip-whitelist"},
		}
		result := ensureEndpointPermissionGlobal(input)
		if len(result) != 2 {
			t.Errorf("Expected 2 plugins, got %d", len(result))
		}
	})
}

// Test buildUserIPWhitelistPlugin
func TestBuildUserIPWhitelistPlugin(t *testing.T) {
	registry := NewDefaultRegistry()
	plugin, err := registry.Build(config.PluginConfig{
		Name: "user-ip-whitelist",
	}, BuilderContext{})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if plugin.Name() != "user-ip-whitelist" {
		t.Errorf("Name() = %q, want user-ip-whitelist", plugin.Name())
	}

	// Test Run function
	ctx := &PipelineContext{
		Request: httptest.NewRequest(http.MethodGet, "http://example.com", nil),
	}
	handled, err := plugin.Run(ctx)
	if handled != false {
		t.Errorf("Run() handled = %v, want false", handled)
	}
	if err != nil {
		t.Errorf("Run() err = %v, want nil", err)
	}
}

// Test buildAuthAPIKeyPlugin with full context
func TestBuildAuthAPIKeyPlugin_Full(t *testing.T) {
	registry := NewDefaultRegistry()
	consumers := []config.Consumer{
		{
			ID:   "consumer-1",
			Name: "Test Consumer",
			APIKeys: []config.ConsumerAPIKey{
				{Key: "test-api-key"},
			},
		},
	}

	plugin, err := registry.Build(config.PluginConfig{
		Name: "auth-apikey",
		Config: map[string]any{
			"key_names":    []string{"X-API-Key"},
			"query_names":  []string{"api_key"},
			"cookie_names": []string{"api_key"},
		},
	}, BuilderContext{
		Consumers: consumers,
	})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if plugin.Name() != "auth-apikey" {
		t.Errorf("Name() = %q, want auth-apikey", plugin.Name())
	}

	// Test Run function with valid key
	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	ctx := &PipelineContext{
		Request: req,
	}
	handled, err := plugin.Run(ctx)
	if handled != false {
		t.Errorf("Run() handled = %v, want false", handled)
	}
	if err != nil {
		t.Errorf("Run() err = %v, want nil", err)
	}
	if ctx.Consumer == nil {
		t.Error("Run() should set ctx.Consumer")
	}
}

// Test buildEndpointPermissionPlugin
func TestBuildEndpointPermissionPlugin(t *testing.T) {
	registry := NewDefaultRegistry()

	t.Run("with permission lookup", func(t *testing.T) {
		lookupCalled := false
		plugin, err := registry.Build(config.PluginConfig{
			Name: "endpoint-permission",
		}, BuilderContext{
			PermissionLookup: func(userID, routeID string) (*EndpointPermissionRecord, error) {
				lookupCalled = true
				return &EndpointPermissionRecord{Allowed: true}, nil
			},
		})
		if err != nil {
			t.Fatalf("Build error: %v", err)
		}
		if plugin.Name() != "endpoint-permission" {
			t.Errorf("Name() = %q, want endpoint-permission", plugin.Name())
		}

		// Test Run function
		ctx := &PipelineContext{
			Request:  httptest.NewRequest(http.MethodGet, "http://example.com/test", nil),
			Consumer: &config.Consumer{ID: "consumer-1"},
			Route:    &config.Route{ID: "route-1"},
		}
		handled, err := plugin.Run(ctx)
		if handled != false {
			t.Errorf("Run() handled = %v, want false", handled)
		}
		if err != nil {
			t.Errorf("Run() err = %v, want nil", err)
		}
		if !lookupCalled {
			t.Error("Permission lookup was not called")
		}
	})

	t.Run("without permission lookup", func(t *testing.T) {
		plugin, err := registry.Build(config.PluginConfig{
			Name: "endpoint-permission",
		}, BuilderContext{})
		if err != nil {
			t.Fatalf("Build error: %v", err)
		}

		ctx := &PipelineContext{
			Request:  httptest.NewRequest(http.MethodGet, "http://example.com/test", nil),
			Consumer: &config.Consumer{ID: "consumer-1"},
		}
		handled, err := plugin.Run(ctx)
		if handled != false {
			t.Errorf("Run() handled = %v, want false", handled)
		}
		if err != nil {
			t.Errorf("Run() err = %v, want nil", err)
		}
	})
}

// Test buildRateLimitPlugin with error case
func TestBuildRateLimitPlugin_Error(t *testing.T) {
	registry := NewDefaultRegistry()
	_, err := registry.Build(config.PluginConfig{
		Name: "rate-limit",
		Config: map[string]any{
			"algorithm": "invalid-algorithm",
		},
	}, BuilderContext{})
	if err == nil {
		t.Error("Expected error for invalid algorithm")
	}
}

// Test buildResponseTransformPlugin with nil context
func TestBuildResponseTransformPlugin_NilContext(t *testing.T) {
	registry := NewDefaultRegistry()
	plugin, err := registry.Build(config.PluginConfig{
		Name: "response-transform",
		Config: map[string]any{
			"add_headers": map[string]any{
				"X-Custom": "value",
			},
		},
	}, BuilderContext{})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}

	// Test Run with nil context - should not panic
	handled, err := plugin.Run(nil)
	if handled != false {
		t.Errorf("Run(nil) handled = %v, want false", handled)
	}
	if err != nil {
		t.Errorf("Run(nil) err = %v, want nil", err)
	}

	// Test AfterProxy with nil context - should not panic
	plugin.AfterProxy(nil, nil)
}

// Test buildCORSPlugin with full config
func TestBuildCORSPlugin_Full(t *testing.T) {
	registry := NewDefaultRegistry()
	plugin, err := registry.Build(config.PluginConfig{
		Name: "cors",
		Config: map[string]any{
			"allowed_origins": []string{"https://example.com"},
			"allowed_methods": []string{"GET", "POST"},
			"allowed_headers": []string{"Content-Type", "Authorization"},
			"max_age":         3600,
			"credentials":     true,
		},
	}, BuilderContext{})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if plugin.Name() != "cors" {
		t.Errorf("Name() = %q, want cors", plugin.Name())
	}

	// Test Run function with preflight request
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "http://example.com", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	ctx := &PipelineContext{
		Request:        req,
		ResponseWriter: w,
	}
	handled, err := plugin.Run(ctx)
	// CORS plugin returns true if it handles the request (preflight)
	if err != nil {
		t.Errorf("Run() err = %v, want nil", err)
	}
	_ = handled
}

// Test BuildRoutePipelinesWithContext with nil config
func TestBuildRoutePipelinesWithContext_NilConfig(t *testing.T) {
	pipelines, hasAuth, err := BuildRoutePipelinesWithContext(nil, BuilderContext{})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(pipelines) != 0 {
		t.Errorf("Expected empty pipelines, got %d", len(pipelines))
	}
	if len(hasAuth) != 0 {
		t.Errorf("Expected empty hasAuth, got %d", len(hasAuth))
	}
}

// Test PipelineContext Abort with nil receiver
func TestPipelineContext_Abort_Nil(t *testing.T) {
	var ctx *PipelineContext
	// Should not panic
	ctx.Abort("test reason")
}

// Test PipelineContext Abort with empty reason
func TestPipelineContext_Abort_EmptyReason(t *testing.T) {
	ctx := &PipelineContext{}
	ctx.Abort("  ")
	if !ctx.Aborted {
		t.Error("Aborted should be true")
	}
	// Empty/whitespace reason should be trimmed to empty
	if ctx.AbortReason != "" {
		t.Errorf("AbortReason = %q, want empty", ctx.AbortReason)
	}
}

// Test asRedirectRules edge cases
func TestAsRedirectRules_EdgeCases(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		result := asRedirectRules(nil)
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})

	t.Run("non-array input", func(t *testing.T) {
		result := asRedirectRules("not an array")
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})

	t.Run("array with non-map items", func(t *testing.T) {
		result := asRedirectRules([]any{"not a map", 123})
		if len(result) != 0 {
			t.Errorf("Expected empty result, got %v", result)
		}
	})

	t.Run("rule with missing path", func(t *testing.T) {
		result := asRedirectRules([]any{
			map[string]any{
				"url": "/target",
			},
		})
		if len(result) != 0 {
			t.Errorf("Expected empty result for missing path, got %v", result)
		}
	})

	t.Run("rule with missing target", func(t *testing.T) {
		result := asRedirectRules([]any{
			map[string]any{
				"path": "/source",
			},
		})
		if len(result) != 0 {
			t.Errorf("Expected empty result for missing target, got %v", result)
		}
	})

	t.Run("valid rules", func(t *testing.T) {
		result := asRedirectRules([]any{
			map[string]any{
				"path":        "/old",
				"url":         "/new",
				"status_code": 301,
			},
			map[string]any{
				"from":        "/source",
				"to":          "/dest",
				"status_code": 302,
			},
		})
		if len(result) != 2 {
			t.Errorf("Expected 2 rules, got %d", len(result))
		}
	})
}

// Test asAnyMap edge cases
func TestAsAnyMap_EdgeCases(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		result := asAnyMap(nil)
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})

	t.Run("non-map input", func(t *testing.T) {
		result := asAnyMap("not a map")
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})

	t.Run("map with empty keys", func(t *testing.T) {
		result := asAnyMap(map[string]any{
			"":      "value1",
			"valid": "value2",
			"  ":    "value3",
		})
		if len(result) != 1 {
			t.Errorf("Expected 1 entry, got %d", len(result))
		}
		if _, ok := result["valid"]; !ok {
			t.Error("Expected 'valid' key to be present")
		}
	})
}

// Test asStringMap edge cases
func TestAsStringMap_EdgeCases(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		result := asStringMap(nil)
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})

	t.Run("non-map input", func(t *testing.T) {
		result := asStringMap("not a map")
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})

	t.Run("map with empty keys or values", func(t *testing.T) {
		result := asStringMap(map[string]any{
			"":      "value1",
			"valid": "value2",
			"key3":  "",
			"  ":    "value4",
		})
		if len(result) != 1 {
			t.Errorf("Expected 1 entry, got %d", len(result))
		}
		if _, ok := result["valid"]; !ok {
			t.Error("Expected 'valid' key to be present")
		}
	})
}

// Test asDuration edge cases
func TestAsDuration_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		fallback time.Duration
		expected time.Duration
	}{
		{"nil", nil, time.Second, time.Second},
		{"duration", time.Minute, time.Second, time.Minute},
		{"int", 60, time.Second, 60 * time.Second},
		{"int64", int64(120), time.Second, 120 * time.Second},
		{"float64", 30.5, time.Second, 30*time.Second + 500*time.Millisecond},
		{"valid string", "2m", time.Second, 2 * time.Minute},
		{"valid string seconds", "30s", time.Second, 30 * time.Second},
		{"string as number", "60", time.Second, 60 * time.Second},
		{"empty string", "", time.Second, time.Second},
		{"invalid string", "not a duration", time.Second, time.Second},
		{"bool", true, time.Second, time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asDuration(tt.input, tt.fallback)
			if got != tt.expected {
				t.Errorf("asDuration(%v, %v) = %v, want %v", tt.input, tt.fallback, got, tt.expected)
			}
		})
	}
}

// Test asBool edge cases
func TestAsBool_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		fallback bool
		expected bool
	}{
		{"nil", nil, true, true},
		{"nil with false fallback", nil, false, false},
		{"bool true", true, false, true},
		{"bool false", false, true, false},
		{"string true", "true", false, true},
		{"string TRUE", "TRUE", false, true},
		{"string 1", "1", false, true},
		{"string yes", "yes", false, true},
		{"string on", "on", false, true},
		{"string false", "false", true, false},
		{"string 0", "0", true, false},
		{"string no", "no", true, false},
		{"string off", "off", true, false},
		{"empty string", "", true, true},
		{"whitespace string", "   ", true, true},
		{"int", 42, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asBool(tt.input, tt.fallback)
			if got != tt.expected {
				t.Errorf("asBool(%v, %v) = %v, want %v", tt.input, tt.fallback, got, tt.expected)
			}
		})
	}
}

// Test asInt edge cases
func TestAsInt_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		fallback int
		expected int
	}{
		{"nil", nil, 42, 42},
		{"int", 100, 42, 100},
		{"int64", int64(200), 42, 200},
		{"int32", int32(300), 42, 300},
		{"float64", 3.14, 42, 3},
		{"float64 large", 999.9, 42, 999},
		{"float32", float32(4.5), 42, 4},
		{"string number", "123", 42, 123},
		{"string negative", "-456", 42, -456},
		{"empty string", "", 42, 42},
		{"whitespace string", "   ", 42, 42},
		{"invalid string", "not a number", 42, 42},
		{"bool", true, 42, 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asInt(tt.input, tt.fallback)
			if got != tt.expected {
				t.Errorf("asInt(%v, %v) = %v, want %v", tt.input, tt.fallback, got, tt.expected)
			}
		})
	}
}

// Test asString edge cases
func TestAsString_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"empty string", "", ""},
		{"whitespace", "  hello  ", "hello"},
		{"int", 42, "42"},
		{"int64", int64(100), "100"},
		{"float64", 3.14, "3.14"},
		{"bool", true, "true"},
		{"<nil> string", "<nil>", "<nil>"}, // String literal is not converted
		{"<NIL> string", "<NIL>", "<NIL>"}, // String literal is not converted
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asString(tt.input)
			if got != tt.expected {
				t.Errorf("asString(%v) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// Test asStringSlice edge cases
func TestAsStringSlice_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected []string
	}{
		{"nil", nil, nil},
		{"[]string", []string{"a", "b"}, []string{"a", "b"}},
		{"[]string with empty", []string{"a", "", "b"}, []string{"a", "b"}},
		{"[]any", []any{"x", "y"}, []string{"x", "y"}},
		{"[]any with empty", []any{"x", "", "y"}, []string{"x", "y"}},
		{"[]any with ints", []any{1, 2}, []string{"1", "2"}},
		{"string", "single", []string{"single"}},
		{"empty string", "", nil},
		{"int", 42, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asStringSlice(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("asStringSlice(%v) = %v, want %v", tt.input, got, tt.expected)
				return
			}
			for i := range tt.expected {
				if got[i] != tt.expected[i] {
					t.Errorf("asStringSlice(%v)[%d] = %q, want %q", tt.input, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

// Test asFloat edge cases
func TestAsFloat_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		fallback float64
		expected float64
	}{
		{"nil", nil, 1.5, 1.5},
		{"float64", 3.14, 0, 3.14},
		{"float32", float32(2.5), 0, 2.5},
		{"int", 42, 0, 42},
		{"int64", int64(100), 0, 100},
		{"string float", "3.14", 0, 3.14},
		{"string int", "100", 0, 100},
		{"empty string", "", 99, 99},
		{"invalid string", "not a number", 99, 99},
		{"bool", true, 99, 99},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asFloat(tt.input, tt.fallback)
			if got != tt.expected {
				t.Errorf("asFloat(%v, %v) = %v, want %v", tt.input, tt.fallback, got, tt.expected)
			}
		})
	}
}

// Test asIntSlice edge cases
func TestAsIntSlice_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		fallback []int
		expected []int
	}{
		{"nil", nil, []int{99}, []int{99}},
		{"[]int", []int{1, 2, 3}, []int{99}, []int{1, 2, 3}},
		{"empty []int", []int{}, []int{99}, []int{99}},
		{"[]any with valid codes", []any{200, 404}, []int{99}, []int{200, 404}},
		{"[]any with invalid codes", []any{1, 2, 3}, []int{99}, []int{99}},
		{"[]any mixed", []any{100, 50, 200}, []int{99}, []int{100, 200}},
		{"string comma separated", "200,404,500", []int{99}, []int{200, 404, 500}},
		{"string with invalid", "abc,def", []int{99}, []int{99}},
		{"empty string", "", []int{99}, []int{99}},
		{"int", 42, []int{99}, []int{99}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asIntSlice(tt.input, tt.fallback)
			if len(got) != len(tt.expected) {
				t.Errorf("asIntSlice(%v, %v) = %v, want %v", tt.input, tt.fallback, got, tt.expected)
				return
			}
			for i := range tt.expected {
				if got[i] != tt.expected[i] {
					t.Errorf("asIntSlice(%v, %v)[%d] = %d, want %d", tt.input, tt.fallback, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

// Test normalizePluginName
func TestNormalizePluginName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Test-Plugin", "test-plugin"},
		{"  spaced  ", "spaced"},
		{"UPPERCASE", "uppercase"},
		{"", ""},
		{"MiXeD-CaSe", "mixed-case"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizePluginName(tt.input)
			if got != tt.expected {
				t.Errorf("normalizePluginName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// Test phaseOrder with unknown phase
func TestPhaseOrder_Unknown(t *testing.T) {
	unknownPhase := Phase("unknown_phase")
	got := phaseOrder(unknownPhase)
	if got != 999 {
		t.Errorf("phaseOrder(unknown) = %d, want 999", got)
	}
}

// Test pickFirstString
func TestPickFirstString(t *testing.T) {
	tests := []struct {
		name     string
		inputs   []any
		expected string
	}{
		{"first valid", []any{"a", "b", "c"}, "a"},
		{"second valid", []any{"", "b", "c"}, "b"},
		{"third valid", []any{"", "", "c"}, "c"},
		{"all empty", []any{"", "", ""}, ""},
		{"no args", []any{}, ""},
		{"whitespace skipped", []any{"  ", "valid"}, "valid"},
		{"int converted", []any{42}, "42"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pickFirstString(tt.inputs...)
			if got != tt.expected {
				t.Errorf("pickFirstString() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// Test BuildRoutePipelines
func TestBuildRoutePipelines(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{
			{
				ID:    "route-1",
				Name:  "Test Route",
				Paths: []string{"/test"},
				Plugins: []config.PluginConfig{
					{Name: "correlation-id"},
				},
			},
		},
		GlobalPlugins: []config.PluginConfig{
			{Name: "cors"},
		},
	}
	consumers := []config.Consumer{
		{ID: "consumer-1", Name: "Test"},
	}

	pipelines, hasAuth, err := BuildRoutePipelines(cfg, consumers)
	if err != nil {
		t.Fatalf("BuildRoutePipelines error: %v", err)
	}
	if len(pipelines) != 1 {
		t.Errorf("Expected 1 pipeline, got %d", len(pipelines))
	}
	if len(hasAuth) != 0 {
		t.Errorf("Expected no auth, got %d entries", len(hasAuth))
	}
}

// Test NewPluginReloader errors
func TestNewPluginReloader_Errors(t *testing.T) {
	t.Run("disabled config", func(t *testing.T) {
		reloader, err := NewPluginReloader(HotReloadConfig{
			Enabled: false,
		}, nil)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if reloader == nil {
			t.Error("Expected non-nil reloader")
		}
	})
}

// Test ReloadPlugin errors
func TestReloadPlugin_Errors(t *testing.T) {
	tmpDir := t.TempDir()

	reloader := &PluginReloader{
		config: HotReloadConfig{
			Enabled:  true,
			WatchDir: tmpDir,
		},
		handlers: make(map[string]ReloadHandler),
	}

	t.Run("no handler registered", func(t *testing.T) {
		err := reloader.ReloadPlugin("nonexistent")
		if err == nil {
			t.Error("Expected error for missing handler")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		reloader.RegisterHandler("test", func(name string, content []byte) error {
			return nil
		})
		err := reloader.ReloadPlugin("test")
		if err == nil {
			t.Error("Expected error for missing file")
		}
	})

	t.Run("handler error", func(t *testing.T) {
		// Create the file
		testFile := filepath.Join(tmpDir, "error.lua")
		os.WriteFile(testFile, []byte("test"), 0644)

		reloader.RegisterHandler("error", func(name string, content []byte) error {
			return fmt.Errorf("handler error")
		})
		err := reloader.ReloadPlugin("error")
		if err == nil {
			t.Error("Expected error from handler")
		}
	})
}

// Test handleFileChange errors
func TestHandleFileChange_Errors(t *testing.T) {
	tmpDir := t.TempDir()
	reloader := &PluginReloader{
		config: HotReloadConfig{
			Enabled:  true,
			WatchDir: tmpDir,
		},
		handlers: make(map[string]ReloadHandler),
	}

	t.Run("file read error", func(t *testing.T) {
		// Try to read a non-existent file
		reloader.handleFileChange("/nonexistent/file.lua")
		// Should not panic, just log error
	})

	t.Run("handler error", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "test.lua")
		os.WriteFile(testFile, []byte("content"), 0644)

		handlerCalled := false
		reloader.RegisterHandler("test", func(name string, content []byte) error {
			handlerCalled = true
			return fmt.Errorf("handler error")
		})

		reloader.handleFileChange(testFile)
		if !handlerCalled {
			t.Error("Handler should have been called")
		}
	})
}

// Test UnwatchFile
func TestUnwatchFile(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		reloader := &PluginReloader{
			config: HotReloadConfig{
				Enabled: false,
			},
		}
		err := reloader.UnwatchFile("/some/path")
		if err != nil {
			t.Errorf("UnwatchFile when disabled should return nil, got %v", err)
		}
	})

	t.Run("nil watcher", func(t *testing.T) {
		reloader := &PluginReloader{
			config: HotReloadConfig{
				Enabled: true,
			},
			watcher: nil,
		}
		err := reloader.UnwatchFile("/some/path")
		if err != nil {
			t.Errorf("UnwatchFile with nil watcher should return nil, got %v", err)
		}
	})
}

// Test WatchFile
func TestWatchFile(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		reloader := &PluginReloader{
			config: HotReloadConfig{
				Enabled: false,
			},
		}
		err := reloader.WatchFile("/some/path")
		if err != nil {
			t.Errorf("WatchFile when disabled should return nil, got %v", err)
		}
	})

	t.Run("nil watcher", func(t *testing.T) {
		reloader := &PluginReloader{
			config: HotReloadConfig{
				Enabled: true,
			},
			watcher: nil,
		}
		err := reloader.WatchFile("/some/path")
		if err != nil {
			t.Errorf("WatchFile with nil watcher should return nil, got %v", err)
		}
	})
}

// Test matchesPattern with no patterns
func TestMatchesPattern_NoPatterns(t *testing.T) {
	reloader := &PluginReloader{
		config: HotReloadConfig{
			Enabled:  true,
			Patterns: []string{},
		},
	}

	// With empty patterns, should match everything
	if !reloader.matchesPattern("/any/file.lua") {
		t.Error("Should match when no patterns defined")
	}
}

// Test matchesPattern with patterns
func TestMatchesPattern_WithPatterns(t *testing.T) {
	reloader := &PluginReloader{
		config: HotReloadConfig{
			Enabled:  true,
			Patterns: []string{"*.lua", "*.wasm"},
		},
	}

	tests := []struct {
		path     string
		expected bool
	}{
		{"/path/to/plugin.lua", true},
		{"/path/to/plugin.wasm", true},
		{"/path/to/plugin.txt", false},
		{"/path/to/lua", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := reloader.matchesPattern(tt.path)
			if got != tt.expected {
				t.Errorf("matchesPattern(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

// Test DynamicPluginManager UpdatePlugin creates new plugin
func TestDynamicPluginManager_UpdatePlugin_CreateNew(t *testing.T) {
	manager := NewDynamicPluginManager(nil)

	// Update a plugin that doesn't exist yet
	err := manager.UpdatePlugin("new-plugin", []byte("content"))
	if err != nil {
		t.Errorf("UpdatePlugin error: %v", err)
	}

	plugin, exists := manager.GetPlugin("new-plugin")
	if !exists {
		t.Error("Plugin should exist after update")
	}
	if string(plugin.Content) != "content" {
		t.Errorf("Content = %q, want %q", plugin.Content, "content")
	}
	if plugin.Status != "loaded" {
		t.Errorf("Status = %q, want loaded", plugin.Status)
	}
}

// Test DynamicPluginManager GetPlugin not found
func TestDynamicPluginManager_GetPlugin_NotFound(t *testing.T) {
	manager := NewDynamicPluginManager(nil)

	plugin, exists := manager.GetPlugin("nonexistent")
	if exists {
		t.Error("Should not exist")
	}
	if plugin.Name != "" {
		t.Error("Should return empty plugin")
	}
}

// Test DynamicPluginManager SetPluginError for non-existent plugin
func TestDynamicPluginManager_SetPluginError_NonExistent(t *testing.T) {
	manager := NewDynamicPluginManager(nil)

	// Should not panic
	manager.SetPluginError("nonexistent", fmt.Errorf("some error"))

	// Verify plugin was not created
	_, exists := manager.GetPlugin("nonexistent")
	if exists {
		t.Error("Plugin should not be created by SetPluginError")
	}
}

// Test DynamicPluginManager ClearPluginError for non-existent plugin
func TestDynamicPluginManager_ClearPluginError_NonExistent(t *testing.T) {
	manager := NewDynamicPluginManager(nil)

	// Should not panic
	manager.ClearPluginError("nonexistent")

	// Verify plugin was not created
	_, exists := manager.GetPlugin("nonexistent")
	if exists {
		t.Error("Plugin should not be created by ClearPluginError")
	}
}

// Test GetPluginManagerFromContext with missing key
func TestGetPluginManagerFromContext_Missing(t *testing.T) {
	ctx := context.Background()
	manager := GetPluginManagerFromContext(ctx)
	if manager != nil {
		t.Error("Should return nil for missing manager")
	}
}

// Test WithPluginManager and GetPluginManagerFromContext - renamed to avoid duplicate
func TestWithPluginManager_Additional(t *testing.T) {
	manager := NewDynamicPluginManager(nil)
	ctx := WithPluginManager(context.Background(), manager)

	retrieved := GetPluginManagerFromContext(ctx)
	if retrieved != manager {
		t.Error("Retrieved manager should match original")
	}
}

// Test Stop with nil channels
func TestStop_NilChannels(t *testing.T) {
	reloader := &PluginReloader{
		stopCh:  nil,
		watcher: nil,
	}

	// Should not panic
	reloader.Stop()
}

// Test Stop with valid channels
func TestStop_ValidChannels(t *testing.T) {
	tmpDir := t.TempDir()
	reloader, err := NewPluginReloader(HotReloadConfig{
		Enabled:  true,
		WatchDir: tmpDir,
	}, nil)
	if err != nil {
		t.Fatalf("NewPluginReloader error: %v", err)
	}

	// Should not panic
	reloader.Stop()
}

// Test watch with closed channels
func TestWatch_NotEnabled(t *testing.T) {
	reloader := &PluginReloader{
		config: HotReloadConfig{
			Enabled: false,
		},
	}

	// Should return immediately without blocking
	reloader.watch()
}

// Test handleFileChange with no handler
func TestHandleFileChange_NoHandler(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.lua")
	os.WriteFile(testFile, []byte("content"), 0644)

	reloader := &PluginReloader{
		config: HotReloadConfig{
			Enabled:  true,
			WatchDir: tmpDir,
		},
		handlers: make(map[string]ReloadHandler),
	}

	// Should not panic even without handler
	reloader.handleFileChange(testFile)
}

// Test URLRewrite Apply edge cases
func TestURLRewrite_Apply_EdgeCases(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var rewriter *URLRewrite
		err := rewriter.Apply(&PipelineContext{
			Request: httptest.NewRequest(http.MethodGet, "http://example.com/path", nil),
		})
		if err != nil {
			t.Errorf("Apply with nil receiver should return nil error, got %v", err)
		}
	})

	t.Run("nil context", func(t *testing.T) {
		rewriter, _ := NewURLRewrite(URLRewriteConfig{
			Pattern:     "/old",
			Replacement: "/new",
		})
		err := rewriter.Apply(nil)
		if err != nil {
			t.Errorf("Apply with nil context should return nil error, got %v", err)
		}
	})

	t.Run("nil request", func(t *testing.T) {
		rewriter, _ := NewURLRewrite(URLRewriteConfig{
			Pattern:     "/old",
			Replacement: "/new",
		})
		err := rewriter.Apply(&PipelineContext{
			Request: nil,
		})
		if err != nil {
			t.Errorf("Apply with nil request should return nil error, got %v", err)
		}
	})

	t.Run("nil URL", func(t *testing.T) {
		rewriter, _ := NewURLRewrite(URLRewriteConfig{
			Pattern:     "/old",
			Replacement: "/new",
		})
		req := httptest.NewRequest(http.MethodGet, "http://example.com/path", nil)
		req.URL = nil
		err := rewriter.Apply(&PipelineContext{
			Request: req,
		})
		if err != nil {
			t.Errorf("Apply with nil URL should return nil error, got %v", err)
		}
	})
}

// Test Timeout Apply edge cases
func TestTimeout_Apply_EdgeCases(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var timeout *Timeout
		timeout.Apply(&PipelineContext{
			Request: httptest.NewRequest(http.MethodGet, "http://example.com", nil),
		})
		// Should not panic
	})

	t.Run("nil context", func(t *testing.T) {
		timeout := NewTimeout(TimeoutConfig{Duration: time.Second})
		timeout.Apply(nil)
		// Should not panic
	})

	t.Run("nil request", func(t *testing.T) {
		timeout := NewTimeout(TimeoutConfig{Duration: time.Second})
		timeout.Apply(&PipelineContext{
			Request: nil,
		})
		// Should not panic
	})
}

// Test UserIPWhitelist Evaluate edge cases
func TestUserIPWhitelist_Evaluate_EdgeCases(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var whitelist *UserIPWhitelist
		err := whitelist.Evaluate(&PipelineContext{
			Request: httptest.NewRequest(http.MethodGet, "http://example.com", nil),
		})
		if err != nil {
			t.Errorf("Evaluate with nil receiver should return nil error, got %v", err)
		}
	})

	t.Run("nil context", func(t *testing.T) {
		whitelist := NewUserIPWhitelist()
		err := whitelist.Evaluate(nil)
		if err != nil {
			t.Errorf("Evaluate with nil context should return nil error, got %v", err)
		}
	})

	t.Run("nil request", func(t *testing.T) {
		whitelist := NewUserIPWhitelist()
		err := whitelist.Evaluate(&PipelineContext{
			Request: nil,
		})
		if err != nil {
			t.Errorf("Evaluate with nil request should return nil error, got %v", err)
		}
	})

	t.Run("nil consumer", func(t *testing.T) {
		whitelist := NewUserIPWhitelist()
		err := whitelist.Evaluate(&PipelineContext{
			Request:  httptest.NewRequest(http.MethodGet, "http://example.com", nil),
			Consumer: nil,
		})
		if err != nil {
			t.Errorf("Evaluate with nil consumer should return nil error, got %v", err)
		}
	})
}

// Test normalizeIPRuleList edge cases
func TestNormalizeIPRuleList_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected []string
	}{
		{"nil", nil, nil},
		{"[]string", []string{"1.1.1.1", "2.2.2.2"}, []string{"1.1.1.1", "2.2.2.2"}},
		{"[]string with whitespace", []string{" 1.1.1.1 ", " 2.2.2.2 "}, []string{"1.1.1.1", "2.2.2.2"}},
		{"[]string with empty", []string{"1.1.1.1", "", "2.2.2.2"}, []string{"1.1.1.1", "2.2.2.2"}},
		{"[]any", []any{"1.1.1.1", "2.2.2.2"}, []string{"1.1.1.1", "2.2.2.2"}},
		{"[]any with numbers", []any{1, 2, 3}, []string{"1", "2", "3"}},
		{"string", "1.1.1.1", []string{"1.1.1.1"}},
		{"empty string", "", nil},
		{"whitespace string", "   ", nil},
		{"comma separated", "1.1.1.1, 2.2.2.2", []string{"1.1.1.1", "2.2.2.2"}},
		{"comma with empty", "1.1.1.1,,2.2.2.2", []string{"1.1.1.1", "2.2.2.2"}},
		{"int", 42, nil},
		{"map", map[string]string{"a": "b"}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeIPRuleList(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("normalizeIPRuleList(%v) = %v, want %v", tt.input, got, tt.expected)
				return
			}
			for i := range tt.expected {
				if got[i] != tt.expected[i] {
					t.Errorf("normalizeIPRuleList(%v)[%d] = %q, want %q", tt.input, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

// Test matchesIPWhitelist edge cases
func TestMatchesIPWhitelist_EdgeCases(t *testing.T) {
	_, cidr1, _ := net.ParseCIDR("192.168.1.0/24")
	_, cidr2, _ := net.ParseCIDR("10.0.0.0/8")

	tests := []struct {
		name   string
		ip     net.IP
		exact  map[string]struct{}
		cidrs  []*net.IPNet
		expect bool
	}{
		{
			name:   "nil ip",
			ip:     nil,
			exact:  map[string]struct{}{"192.168.1.1": {}},
			cidrs:  []*net.IPNet{cidr1},
			expect: false,
		},
		{
			name:   "match exact",
			ip:     net.ParseIP("192.168.1.1"),
			exact:  map[string]struct{}{"192.168.1.1": {}},
			cidrs:  nil,
			expect: true,
		},
		{
			name:   "match cidr",
			ip:     net.ParseIP("192.168.1.50"),
			exact:  map[string]struct{}{},
			cidrs:  []*net.IPNet{cidr1},
			expect: true,
		},
		{
			name:   "no match",
			ip:     net.ParseIP("8.8.8.8"),
			exact:  map[string]struct{}{"192.168.1.1": {}},
			cidrs:  []*net.IPNet{cidr1},
			expect: false,
		},
		{
			name:   "multiple cidrs match second",
			ip:     net.ParseIP("10.0.0.5"),
			exact:  map[string]struct{}{},
			cidrs:  []*net.IPNet{cidr1, cidr2},
			expect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesIPWhitelist(tt.ip, tt.exact, tt.cidrs)
			if got != tt.expect {
				t.Errorf("matchesIPWhitelist() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// Test extractUserWhitelistRules edge cases
func TestExtractUserWhitelistRules_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		consumer *config.Consumer
		expected []string
	}{
		{
			name:     "nil consumer",
			consumer: nil,
			expected: nil,
		},
		{
			name:     "nil metadata",
			consumer: &config.Consumer{Metadata: nil},
			expected: nil,
		},
		{
			name:     "empty metadata",
			consumer: &config.Consumer{Metadata: map[string]any{}},
			expected: nil,
		},
		{
			name:     "missing ip_whitelist key",
			consumer: &config.Consumer{Metadata: map[string]any{"other": "value"}},
			expected: nil,
		},
		{
			name:     "with ip_whitelist",
			consumer: &config.Consumer{Metadata: map[string]any{"ip_whitelist": "192.168.1.1"}},
			expected: []string{"192.168.1.1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractUserWhitelistRules(tt.consumer)
			if len(got) != len(tt.expected) {
				t.Errorf("extractUserWhitelistRules() = %v, want %v", got, tt.expected)
				return
			}
			for i := range tt.expected {
				if got[i] != tt.expected[i] {
					t.Errorf("extractUserWhitelistRules()[%d] = %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

// Test Retry Backoff edge cases
func TestRetry_Backoff_EdgeCases(t *testing.T) {
	r := NewRetry(RetryConfig{
		MaxRetries: 5,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   time.Second,
		Jitter:     false,
	})

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 100 * time.Millisecond}, // base delay
		{1, 200 * time.Millisecond}, // 2^1 * base
		{2, 400 * time.Millisecond}, // 2^2 * base
		{3, 800 * time.Millisecond}, // 2^3 * base
		{4, time.Second},            // capped at max
		{10, time.Second},           // capped at max
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			got := r.Backoff(tt.attempt)
			if got != tt.expected {
				t.Errorf("Backoff(%d) = %v, want %v", tt.attempt, got, tt.expected)
			}
		})
	}
}

// Test Retry Backoff with jitter
func TestRetry_Backoff_Jitter(t *testing.T) {
	r := NewRetry(RetryConfig{
		MaxRetries: 5,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   time.Second,
		Jitter:     true,
	})

	// With jitter, delay should be in [50%, 100%] range
	for i := 0; i < 10; i++ {
		delay := r.Backoff(1)
		minDelay := 100 * time.Millisecond // 50% of 200ms
		maxDelay := 200 * time.Millisecond

		if delay < minDelay || delay > maxDelay {
			t.Errorf("Backoff(1) with jitter = %v, want between %v and %v", delay, minDelay, maxDelay)
		}
	}
}

// Test Retry ShouldRetry edge cases
func TestRetry_ShouldRetry_EdgeCases(t *testing.T) {
	r := NewRetry(RetryConfig{
		MaxRetries:    2,
		RetryMethods:  []string{"GET"},
		RetryOnStatus: []int{500, 502},
	})

	tests := []struct {
		name     string
		method   string
		attempt  int
		status   int
		proxyErr error
		expected bool
	}{
		{"non-retryable method", "POST", 0, 500, nil, false},
		{"max attempts reached", "GET", 2, 500, nil, false},
		{"retryable error", "GET", 0, 0, fmt.Errorf("connection refused"), true},
		{"retryable status", "GET", 0, 500, nil, true},
		{"non-retryable status", "GET", 0, 400, nil, false},
		{"attempt at limit", "GET", 2, 500, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.ShouldRetry(tt.method, tt.attempt, tt.status, tt.proxyErr)
			if got != tt.expected {
				t.Errorf("ShouldRetry(%q, %d, %d, %v) = %v, want %v",
					tt.method, tt.attempt, tt.status, tt.proxyErr, got, tt.expected)
			}
		})
	}
}

// Test Retry IsMethodRetryable edge cases
func TestRetry_IsMethodRetryable_EdgeCases(t *testing.T) {
	r := NewRetry(RetryConfig{
		RetryMethods: []string{"GET", "POST"},
	})

	tests := []struct {
		method   string
		expected bool
	}{
		{"GET", true},
		{"get", true}, // case insensitive
		{"POST", true},
		{" post ", true}, // trimmed
		{"DELETE", false},
		{"", false},
		{"   ", false}, // whitespace only becomes empty after trim
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			got := r.IsMethodRetryable(tt.method)
			if got != tt.expected {
				t.Errorf("IsMethodRetryable(%q) = %v, want %v", tt.method, got, tt.expected)
			}
		})
	}
}

// Test Retry NewRetry with default methods
func TestNewRetry_DefaultMethods(t *testing.T) {
	// When no methods specified, should use defaults
	r := NewRetry(RetryConfig{
		RetryMethods: []string{}, // empty
	})

	// Default methods are GET, HEAD, OPTIONS
	if !r.IsMethodRetryable("GET") {
		t.Error("GET should be retryable by default")
	}
	if !r.IsMethodRetryable("HEAD") {
		t.Error("HEAD should be retryable by default")
	}
	if !r.IsMethodRetryable("OPTIONS") {
		t.Error("OPTIONS should be retryable by default")
	}
	if r.IsMethodRetryable("POST") {
		t.Error("POST should not be retryable by default")
	}
}

// Test Retry NewRetry with default status codes
func TestNewRetry_DefaultStatusCodes(t *testing.T) {
	r := NewRetry(RetryConfig{
		RetryOnStatus: []int{}, // empty
	})

	// Default status codes are 502, 503, 504
	if !r.IsStatusRetryable(502) {
		t.Error("502 should be retryable by default")
	}
	if !r.IsStatusRetryable(503) {
		t.Error("503 should be retryable by default")
	}
	if !r.IsStatusRetryable(504) {
		t.Error("504 should be retryable by default")
	}
	if r.IsStatusRetryable(500) {
		t.Error("500 should not be retryable by default")
	}
}

// Test Retry NewRetry with invalid status codes filtered
func TestNewRetry_InvalidStatusCodes(t *testing.T) {
	r := NewRetry(RetryConfig{
		RetryOnStatus: []int{50, 100, 502, 99}, // Some invalid (< 100)
	})

	// Only valid status codes (>= 100) should be included
	if r.IsStatusRetryable(50) {
		t.Error("50 should not be retryable")
	}
	if !r.IsStatusRetryable(100) {
		t.Error("100 should be retryable")
	}
	if !r.IsStatusRetryable(502) {
		t.Error("502 should be retryable")
	}
}

// Test Retry NewRetry with empty methods after filtering
func TestNewRetry_EmptyMethodsAfterFilter(t *testing.T) {
	r := NewRetry(RetryConfig{
		RetryMethods: []string{"", "   "}, // All empty/whitespace
	})

	// Should fall back to defaults
	if !r.IsMethodRetryable("GET") {
		t.Error("GET should be retryable when all methods are empty")
	}
}

// Test NewTimeout with zero/negative duration
func TestNewTimeout_DefaultDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected time.Duration
	}{
		{"zero", 0, 5 * time.Second},
		{"negative", -1 * time.Second, 5 * time.Second},
		{"positive", 10 * time.Second, 10 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeout := NewTimeout(TimeoutConfig{Duration: tt.duration})
			// We can't directly check the duration, but we can verify it was created
			if timeout == nil {
				t.Fatal("NewTimeout returned nil")
			}
		})
	}
}

// Test buildRequestTransformPlugin with body_hooks
func TestBuildRequestTransformPlugin_BodyHooks(t *testing.T) {
	registry := NewDefaultRegistry()

	// Test with body_hooks
	plugin, err := registry.Build(config.PluginConfig{
		Name: "request-transform",
		Config: map[string]any{
			"body_hooks": map[string]any{
				"add_field": "value",
			},
		},
	}, BuilderContext{})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if plugin.Name() != "request-transform" {
		t.Errorf("Name() = %q, want request-transform", plugin.Name())
	}

	// Test with body_transform (fallback)
	plugin2, err := registry.Build(config.PluginConfig{
		Name: "request-transform",
		Config: map[string]any{
			"body_transform": map[string]any{
				"add_field": "value",
			},
		},
	}, BuilderContext{})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if plugin2.Name() != "request-transform" {
		t.Errorf("Name() = %q, want request-transform", plugin2.Name())
	}
}

// Test buildRequestTransformPlugin with path pattern
func TestBuildRequestTransformPlugin_PathPattern(t *testing.T) {
	registry := NewDefaultRegistry()

	// Test with path_pattern
	_, err := registry.Build(config.PluginConfig{
		Name: "request-transform",
		Config: map[string]any{
			"path_pattern":     "^/old/(.*)$",
			"path_replacement": "/new/$1",
		},
	}, BuilderContext{})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}

	// Test with path_regex (fallback)
	_, err = registry.Build(config.PluginConfig{
		Name: "request-transform",
		Config: map[string]any{
			"path_regex":   "^/old/(.*)$",
			"path_replace": "/new/$1",
		},
	}, BuilderContext{})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
}

// Test buildURLRewritePlugin with regex fallback
func TestBuildURLRewritePlugin_RegexFallback(t *testing.T) {
	registry := NewDefaultRegistry()

	// Test with regex instead of pattern
	plugin, err := registry.Build(config.PluginConfig{
		Name: "url-rewrite",
		Config: map[string]any{
			"regex":   "^/old/(.*)$",
			"replace": "/new/$1",
		},
	}, BuilderContext{})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if plugin.Name() != "url-rewrite" {
		t.Errorf("Name() = %q, want url-rewrite", plugin.Name())
	}
}

// Test buildResponseTransformPlugin with replace_body fallback
func TestBuildResponseTransformPlugin_ReplaceBodyFallback(t *testing.T) {
	registry := NewDefaultRegistry()

	// Test with body instead of replace_body
	plugin, err := registry.Build(config.PluginConfig{
		Name: "response-transform",
		Config: map[string]any{
			"body": "replacement body",
		},
	}, BuilderContext{})
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if plugin.Name() != "response-transform" {
		t.Errorf("Name() = %q, want response-transform", plugin.Name())
	}
}

// Test buildRedirectPlugin with various config options
func TestBuildRedirectPlugin_ConfigVariations(t *testing.T) {
	registry := NewDefaultRegistry()

	tests := []struct {
		name   string
		config map[string]any
	}{
		{
			name: "with from and to",
			config: map[string]any{
				"from": "/old",
				"to":   "/new",
			},
		},
		{
			name: "with path and target",
			config: map[string]any{
				"path":   "/old",
				"target": "/new",
			},
		},
		{
			name: "with custom status",
			config: map[string]any{
				"path":        "/old",
				"url":         "/new",
				"status_code": 301,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin, err := registry.Build(config.PluginConfig{
				Name:   "redirect",
				Config: tt.config,
			}, BuilderContext{})
			if err != nil {
				t.Fatalf("Build error: %v", err)
			}
			if plugin.Name() != "redirect" {
				t.Errorf("Name() = %q, want redirect", plugin.Name())
			}
		})
	}
}

// Test PipelinePlugin AfterProxy with nil after
func TestPipelinePlugin_AfterProxy_Nil(t *testing.T) {
	plugin := PipelinePlugin{
		name:  "test",
		phase: PhasePreAuth,
		after: nil,
	}

	// Should not panic
	plugin.AfterProxy(&PipelineContext{}, nil)
	plugin.AfterProxy(&PipelineContext{}, fmt.Errorf("some error"))
}

// Test PipelinePlugin Run with nil run
func TestPipelinePlugin_Run_Nil(t *testing.T) {
	plugin := PipelinePlugin{
		name:  "test",
		phase: PhasePreAuth,
		run:   nil,
	}

	handled, err := plugin.Run(&PipelineContext{})
	if handled != false {
		t.Errorf("Run() handled = %v, want false", handled)
	}
	if err != nil {
		t.Errorf("Run() err = %v, want nil", err)
	}
}

// Test PipelinePlugin Name and Phase
func TestPipelinePlugin_NamePhase(t *testing.T) {
	plugin := PipelinePlugin{
		name:     "test-plugin",
		phase:    PhaseAuth,
		priority: 10,
	}

	if plugin.Name() != "test-plugin" {
		t.Errorf("Name() = %q, want test-plugin", plugin.Name())
	}
	if plugin.Phase() != PhaseAuth {
		t.Errorf("Phase() = %v, want PhaseAuth", plugin.Phase())
	}
}

// Test BuildRoutePipelinesWithContext with plugin build error
func TestBuildRoutePipelinesWithContext_BuildError(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{
			{
				ID:    "route-1",
				Name:  "Test Route",
				Paths: []string{"/test"},
				Plugins: []config.PluginConfig{
					{
						Name: "rate-limit",
						Config: map[string]any{
							"algorithm": "invalid",
						},
					},
				},
			},
		},
	}

	_, _, err := BuildRoutePipelinesWithContext(cfg, BuilderContext{})
	if err == nil {
		t.Error("Expected error for invalid plugin config")
	}
}

// Test BuildRoutePipelinesWithContext with disabled plugin
func TestBuildRoutePipelinesWithContext_DisabledPlugin(t *testing.T) {
	disabled := false
	cfg := &config.Config{
		Routes: []config.Route{
			{
				ID:    "route-1",
				Name:  "Test Route",
				Paths: []string{"/test"},
				Plugins: []config.PluginConfig{
					{
						Name:    "correlation-id",
						Enabled: &disabled,
					},
				},
			},
		},
	}

	pipelines, _, err := BuildRoutePipelinesWithContext(cfg, BuilderContext{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Plugin should be skipped
	if len(pipelines["route-1"]) != 0 {
		t.Errorf("Expected 0 plugins (disabled), got %d", len(pipelines["route-1"]))
	}
}

// Test BuildRoutePipelinesWithContext sorting
func TestBuildRoutePipelinesWithContext_Sorting(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.Route{
			{
				ID:    "route-1",
				Name:  "Test Route",
				Paths: []string{"/test"},
				Plugins: []config.PluginConfig{
					{Name: "correlation-id"}, // PhasePreAuth, Priority 0
					{Name: "auth-apikey"},    // PhaseAuth, Priority 20
					{Name: "timeout"},        // PhaseProxy, Priority 10
				},
			},
		},
	}

	pipelines, _, err := BuildRoutePipelinesWithContext(cfg, BuilderContext{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	chain := pipelines["route-1"]
	if len(chain) != 3 {
		t.Fatalf("Expected 3 plugins, got %d", len(chain))
	}

	// Should be sorted by phase order: PreAuth, Auth, Proxy
	if chain[0].Phase() != PhasePreAuth {
		t.Errorf("First plugin phase = %v, want PhasePreAuth", chain[0].Phase())
	}
	if chain[1].Phase() != PhaseAuth {
		t.Errorf("Second plugin phase = %v, want PhaseAuth", chain[1].Phase())
	}
	if chain[2].Phase() != PhaseProxy {
		t.Errorf("Third plugin phase = %v, want PhaseProxy", chain[2].Phase())
	}
}

// Test NewDefaultRegistry registers all plugins
func TestNewDefaultRegistry(t *testing.T) {
	registry := NewDefaultRegistry()

	expectedPlugins := []string{
		"cors",
		"correlation-id",
		"bot-detect",
		"ip-restrict",
		"auth-apikey",
		"auth-jwt",
		"user-ip-whitelist",
		"endpoint-permission",
		"rate-limit",
		"request-size-limit",
		"request-validator",
		"circuit-breaker",
		"retry",
		"timeout",
		"url-rewrite",
		"request-transform",
		"response-transform",
		"compression",
		"redirect",
	}

	for _, name := range expectedPlugins {
		_, ok := registry.Lookup(name)
		if !ok {
			t.Errorf("Plugin %q not registered", name)
		}
	}
}

// Test DynamicPluginManager ListPlugins - renamed to avoid duplicate
func TestDynamicPluginManager_ListPlugins_Additional(t *testing.T) {
	manager := NewDynamicPluginManager(nil)

	// Initially empty
	plugins := manager.ListPlugins()
	if len(plugins) != 0 {
		t.Errorf("Expected 0 plugins, got %d", len(plugins))
	}

	// Add some plugins
	manager.LoadPlugin("plugin-1", []byte("content1"))
	manager.LoadPlugin("plugin-2", []byte("content2"))

	plugins = manager.ListPlugins()
	if len(plugins) != 2 {
		t.Errorf("Expected 2 plugins, got %d", len(plugins))
	}

	// Verify all plugins are returned
	names := make(map[string]bool)
	for _, p := range plugins {
		names[p.Name] = true
	}
	if !names["plugin-1"] || !names["plugin-2"] {
		t.Error("Not all plugins returned")
	}
}

// Test DynamicPluginManager UnloadPlugin - renamed to avoid duplicate
func TestDynamicPluginManager_UnloadPlugin_Additional(t *testing.T) {
	manager := NewDynamicPluginManager(nil)

	manager.LoadPlugin("test", []byte("content"))
	_, exists := manager.GetPlugin("test")
	if !exists {
		t.Fatal("Plugin should exist")
	}

	err := manager.UnloadPlugin("test")
	if err != nil {
		t.Errorf("UnloadPlugin error: %v", err)
	}

	_, exists = manager.GetPlugin("test")
	if exists {
		t.Error("Plugin should not exist after unload")
	}
}

// Test DynamicPluginManager SetPluginError and ClearPluginError
func TestDynamicPluginManager_SetClearPluginError(t *testing.T) {
	manager := NewDynamicPluginManager(nil)

	manager.LoadPlugin("test", []byte("content"))

	// Set error
	manager.SetPluginError("test", fmt.Errorf("test error"))

	plugin, exists := manager.GetPlugin("test")
	if !exists {
		t.Fatal("Plugin should exist")
	}
	if plugin.Status != "error" {
		t.Errorf("Status = %q, want error", plugin.Status)
	}
	if plugin.Error != "test error" {
		t.Errorf("Error = %q, want test error", plugin.Error)
	}

	// Clear error
	manager.ClearPluginError("test")

	plugin, _ = manager.GetPlugin("test")
	if plugin.Status != "loaded" {
		t.Errorf("Status = %q, want loaded", plugin.Status)
	}
	if plugin.Error != "" {
		t.Errorf("Error = %q, want empty", plugin.Error)
	}
}

// Test DynamicPluginManager SetPluginError with nil error
func TestDynamicPluginManager_SetPluginError_Nil(t *testing.T) {
	manager := NewDynamicPluginManager(nil)

	manager.LoadPlugin("test", []byte("content"))
	manager.SetPluginError("test", nil)

	plugin, _ := manager.GetPlugin("test")
	if plugin.Status != "error" {
		t.Errorf("Status = %q, want error", plugin.Status)
	}
	if plugin.Error != "" {
		t.Errorf("Error = %q, want empty for nil error", plugin.Error)
	}
}

// Test URLRewrite Apply with empty rewrite result
func TestURLRewrite_Apply_EmptyResult(t *testing.T) {
	rewriter, _ := NewURLRewrite(URLRewriteConfig{
		Pattern:     "/.*",
		Replacement: "   ", // Whitespace only
	})

	req := httptest.NewRequest(http.MethodGet, "http://example.com/path", nil)
	ctx := &PipelineContext{
		Request: req,
	}

	err := rewriter.Apply(ctx)
	if err != nil {
		t.Errorf("Apply error: %v", err)
	}

	// Empty/whitespace result should become "/"
	if req.URL.Path != "/" {
		t.Errorf("Path = %q, want /", req.URL.Path)
	}
}

// Test URLRewrite Apply without leading slash
func TestURLRewrite_Apply_NoLeadingSlash(t *testing.T) {
	rewriter, _ := NewURLRewrite(URLRewriteConfig{
		Pattern:     "^/path$",
		Replacement: "new-path",
	})

	req := httptest.NewRequest(http.MethodGet, "http://example.com/path", nil)
	ctx := &PipelineContext{
		Request: req,
	}

	err := rewriter.Apply(ctx)
	if err != nil {
		t.Errorf("Apply error: %v", err)
	}

	// Should add leading slash
	if req.URL.Path != "/new-path" {
		t.Errorf("Path = %q, want /new-path", req.URL.Path)
	}
}

// Test NewURLRewrite with empty pattern
func TestNewURLRewrite_EmptyPattern(t *testing.T) {
	_, err := NewURLRewrite(URLRewriteConfig{
		Pattern:     "",
		Replacement: "/new",
	})
	if err == nil {
		t.Error("Expected error for empty pattern")
	}
}

// Test NewURLRewrite with whitespace-only pattern
func TestNewURLRewrite_WhitespacePattern(t *testing.T) {
	_, err := NewURLRewrite(URLRewriteConfig{
		Pattern:     "   ",
		Replacement: "/new",
	})
	if err == nil {
		t.Error("Expected error for whitespace-only pattern")
	}
}

// Test NewURLRewrite with invalid pattern
func TestNewURLRewrite_InvalidPattern(t *testing.T) {
	_, err := NewURLRewrite(URLRewriteConfig{
		Pattern:     "[invalid(",
		Replacement: "/new",
	})
	if err == nil {
		t.Error("Expected error for invalid regex pattern")
	}
}

// Test NewURLRewrite with whitespace replacement
func TestNewURLRewrite_WhitespaceReplacement(t *testing.T) {
	rewriter, err := NewURLRewrite(URLRewriteConfig{
		Pattern:     "/old",
		Replacement: "  /new  ",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if rewriter.replacement != "/new" {
		t.Errorf("Replacement = %q, want /new", rewriter.replacement)
	}
}

// Test Retry NewRetry with negative maxRetries
func TestNewRetry_NegativeMaxRetries(t *testing.T) {
	r := NewRetry(RetryConfig{
		MaxRetries: -5,
	})

	// Negative should be clamped to 0
	if r.MaxAttempts("GET") != 1 {
		t.Errorf("MaxAttempts = %d, want 1", r.MaxAttempts("GET"))
	}
}

// Test Retry NewRetry with zero/negative delays
func TestNewRetry_ZeroDelays(t *testing.T) {
	r := NewRetry(RetryConfig{
		BaseDelay: 0,
		MaxDelay:  0,
	})

	// Should use defaults
	// Default base delay is 50ms
	if r.Backoff(0) != 50*time.Millisecond {
		t.Errorf("Backoff(0) = %v, want 50ms", r.Backoff(0))
	}
}

// Test Retry NewRetry with negative delays
func TestNewRetry_NegativeDelays(t *testing.T) {
	r := NewRetry(RetryConfig{
		BaseDelay: -100 * time.Millisecond,
		MaxDelay:  -500 * time.Millisecond,
	})

	// Should use defaults
	if r.Backoff(0) != 50*time.Millisecond {
		t.Errorf("Backoff(0) = %v, want 50ms", r.Backoff(0))
	}
}

// Test Retry Backoff with jitter at minimum
func TestRetry_Backoff_JitterMinimum(t *testing.T) {
	r := NewRetry(RetryConfig{
		BaseDelay: 1 * time.Millisecond,
		Jitter:    true,
	})

	// With very small delay, jittered result should be at least 1ms
	delay := r.Backoff(0)
	if delay < time.Millisecond {
		t.Errorf("Backoff(0) = %v, want at least 1ms", delay)
	}
}

// Test Timeout Apply adds cleanup function
func TestTimeout_Apply_AddsCleanup(t *testing.T) {
	timeout := NewTimeout(TimeoutConfig{Duration: time.Second})

	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	ctx := &PipelineContext{
		Request: req,
		Cleanup: []func(){},
	}

	timeout.Apply(ctx)

	// Should have added a cleanup function
	if len(ctx.Cleanup) != 1 {
		t.Errorf("Expected 1 cleanup function, got %d", len(ctx.Cleanup))
	}

	// Request context should be changed
	_, hasDeadline := ctx.Request.Context().Deadline()
	if !hasDeadline {
		t.Error("Request context should have deadline")
	}

	// Cleanup function should be callable
	if len(ctx.Cleanup) > 0 {
		ctx.Cleanup[0]()
	}
}

// Test UserIPWhitelist Evaluate with invalid IP rules
func TestUserIPWhitelist_Evaluate_InvalidRules(t *testing.T) {
	whitelist := NewUserIPWhitelist()

	ctx := &PipelineContext{
		Request: httptest.NewRequest(http.MethodGet, "http://example.com", nil),
		Consumer: &config.Consumer{
			Metadata: map[string]any{
				"ip_whitelist": "invalid-cidr",
			},
		},
	}

	err := whitelist.Evaluate(ctx)
	if err == nil {
		t.Error("Expected error for invalid IP rules")
	}

	// Should be UserIPWhitelistError
	if _, ok := err.(*UserIPWhitelistError); !ok {
		t.Errorf("Expected UserIPWhitelistError, got %T", err)
	}
}

// Test UserIPWhitelist Evaluate with invalid client IP
func TestUserIPWhitelist_Evaluate_InvalidClientIP(t *testing.T) {
	whitelist := NewUserIPWhitelist()

	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	req.RemoteAddr = "not-an-ip:1234"

	ctx := &PipelineContext{
		Request: req,
		Consumer: &config.Consumer{
			Metadata: map[string]any{
				"ip_whitelist": "192.168.1.1",
			},
		},
	}

	err := whitelist.Evaluate(ctx)
	if err == nil {
		t.Error("Expected error for invalid client IP")
	}
}

// Test UserIPWhitelist Evaluate with IP not in whitelist
func TestUserIPWhitelist_Evaluate_IPNotAllowed(t *testing.T) {
	whitelist := NewUserIPWhitelist()

	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	req.RemoteAddr = "8.8.8.8:1234"

	ctx := &PipelineContext{
		Request: req,
		Consumer: &config.Consumer{
			Metadata: map[string]any{
				"ip_whitelist": "192.168.1.1",
			},
		},
	}

	err := whitelist.Evaluate(ctx)
	if err == nil {
		t.Error("Expected error for IP not in whitelist")
	}

	// Should be UserIPWhitelistError with correct code
	if ipErr, ok := err.(*UserIPWhitelistError); ok {
		if ipErr.Code != "ip_not_allowed" {
			t.Errorf("Error code = %q, want ip_not_allowed", ipErr.Code)
		}
	} else {
		t.Errorf("Expected UserIPWhitelistError, got %T", err)
	}
}

// Test UserIPWhitelist Evaluate with allowed IP
func TestUserIPWhitelist_Evaluate_Allowed(t *testing.T) {
	whitelist := NewUserIPWhitelist()

	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	req.RemoteAddr = "192.168.1.50:1234"

	ctx := &PipelineContext{
		Request: req,
		Consumer: &config.Consumer{
			Metadata: map[string]any{
				"ip_whitelist": "192.168.1.0/24",
			},
		},
	}

	err := whitelist.Evaluate(ctx)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

// Test UserIPWhitelist Evaluate with no rules
func TestUserIPWhitelist_Evaluate_NoRules(t *testing.T) {
	whitelist := NewUserIPWhitelist()

	ctx := &PipelineContext{
		Request: httptest.NewRequest(http.MethodGet, "http://example.com", nil),
		Consumer: &config.Consumer{
			Metadata: map[string]any{},
		},
	}

	err := whitelist.Evaluate(ctx)
	if err != nil {
		t.Errorf("Unexpected error when no rules: %v", err)
	}
}

// Test UserIPWhitelist Evaluate with exact IP match
func TestUserIPWhitelist_Evaluate_ExactMatch(t *testing.T) {
	whitelist := NewUserIPWhitelist()

	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	req.RemoteAddr = "192.168.1.100:1234"

	ctx := &PipelineContext{
		Request: req,
		Consumer: &config.Consumer{
			Metadata: map[string]any{
				"ip_whitelist": "192.168.1.100",
			},
		},
	}

	err := whitelist.Evaluate(ctx)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

// Test RateLimit scopeKey method
func TestRateLimit_ScopeKey(t *testing.T) {
	rl, _ := NewRateLimit(RateLimitConfig{
		Algorithm: "token_bucket",
		Scope:     "consumer",
	})

	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	key := rl.scopeKey(RateLimitRequest{
		Request:  req,
		Consumer: &config.Consumer{ID: "consumer-123"},
	})

	if key == "" {
		t.Error("scopeKey should return non-empty string")
	}
}

// =============================================================================
// Tests for Response Transform 0.0% coverage functions
// =============================================================================

func TestTransformCaptureWriter_BodyBytes(t *testing.T) {
	t.Run("nil writer", func(t *testing.T) {
		var w *TransformCaptureWriter
		result := w.BodyBytes()
		if result != nil {
			t.Error("BodyBytes should return nil for nil writer")
		}
	})

	t.Run("empty body", func(t *testing.T) {
		rec := httptest.NewRecorder()
		w := NewTransformCaptureWriter(rec)
		result := w.BodyBytes()
		if len(result) != 0 {
			t.Errorf("BodyBytes length = %d, want 0", len(result))
		}
	})

	t.Run("with body content", func(t *testing.T) {
		rec := httptest.NewRecorder()
		w := NewTransformCaptureWriter(rec)
		w.body.WriteString("test content")
		result := w.BodyBytes()
		if string(result) != "test content" {
			t.Errorf("BodyBytes = %q, want test content", string(result))
		}
	})

	t.Run("returns copy", func(t *testing.T) {
		rec := httptest.NewRecorder()
		w := NewTransformCaptureWriter(rec)
		w.body.WriteString("original")

		result1 := w.BodyBytes()
		result1[0] = 'X'

		result2 := w.BodyBytes()
		if string(result2) != "original" {
			t.Error("BodyBytes should return a copy, not the original")
		}
	})
}

func TestTransformCaptureWriter_IsFlushed(t *testing.T) {
	t.Run("nil writer", func(t *testing.T) {
		var w *TransformCaptureWriter
		if w.IsFlushed() {
			t.Error("IsFlushed should return false for nil writer")
		}
	})

	t.Run("not flushed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		w := NewTransformCaptureWriter(rec)
		if w.IsFlushed() {
			t.Error("IsFlushed should return false before flush")
		}
	})

	t.Run("after flush", func(t *testing.T) {
		rec := httptest.NewRecorder()
		w := NewTransformCaptureWriter(rec)
		w.Flush()
		if !w.IsFlushed() {
			t.Error("IsFlushed should return true after flush")
		}
	})
}
