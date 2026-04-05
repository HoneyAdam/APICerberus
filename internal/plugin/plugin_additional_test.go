package plugin

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net/http"
	"net/http/httptest"
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
		{"nil", nil, "<nil>", true},  // nil falls through to default case which uses fmt.Sprint
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
		name string
		input any
		want bool
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
				"sub": "X-User-ID",
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
		{"25:00", 0, true},     // Hour out of range
		{"12:60", 0, true},     // Minute out of range
		{"-1:00", 0, true},     // Negative hour
		{"12:-1", 0, true},     // Negative minute
		{"abc", 0, true},       // Invalid
		{"", 0, true},          // Empty
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
		name     string
		input    string
		wantErr  bool
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
		{"short token", "Bearer x", "x"}, // Just needs 8+ chars
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
		Window:       time.Minute,
		ErrorThreshold: 0.5,
		SleepWindow:  time.Second,
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
		Window:       time.Minute,
		ErrorThreshold: 0.5,
		SleepWindow:  time.Second,
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
		Window:       time.Minute,
		ErrorThreshold: 0.5,
		SleepWindow:  time.Second,
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
		{"[]int", []int{1, 2, 3}, []int{0}, []int{1, 2, 3}}, // []int returns as-is
		{"[]any with ints >= 100", []any{100, 200}, []int{0}, []int{100, 200}}, // filtered by >= 100
		{"[]any with ints < 100", []any{1, 2}, []int{99}, []int{99}}, // falls back because all < 100
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
