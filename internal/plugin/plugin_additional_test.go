package plugin

import (
	"strings"
	"testing"
	"time"
)

// Test AuthError Error method
func TestAuthError_Error(t *testing.T) {
	err := &AuthError{
		Code:    "test_code",
		Message: "test message",
		Status:  401,
	}

	if err.Error() != "test message" {
		t.Errorf("Error() = %q, want %q", err.Error(), "test message")
	}
}

// Test AuthAPIKey Name, Phase, Priority methods
func TestAuthAPIKey_Methods(t *testing.T) {
	auth := NewAuthAPIKey(nil, AuthAPIKeyOptions{})

	if auth.Name() != "auth-apikey" {
		t.Errorf("Name() = %q, want %q", auth.Name(), "auth-apikey")
	}

	if auth.Phase() != PhaseAuth {
		t.Errorf("Phase() = %v, want %v", auth.Phase(), PhaseAuth)
	}

	if auth.Priority() != 10 {
		t.Errorf("Priority() = %d, want %d", auth.Priority(), 10)
	}
}

// Test AuthJWT Name, Phase, Priority methods
func TestAuthJWT_Methods(t *testing.T) {
	auth := NewAuthJWT(AuthJWTOptions{})

	if auth.Name() != "auth-jwt" {
		t.Errorf("Name() = %q, want %q", auth.Name(), "auth-jwt")
	}

	if auth.Phase() != PhaseAuth {
		t.Errorf("Phase() = %v, want %v", auth.Phase(), PhaseAuth)
	}

	if auth.Priority() != 20 {
		t.Errorf("Priority() = %d, want %d", auth.Priority(), 20)
	}
}

// Test JWTAuthError Error method
func TestJWTAuthError_Error(t *testing.T) {
	err := &JWTAuthError{
		Code:    "test_code",
		Message: "test message",
		Status:  401,
	}

	if err.Error() != "test message" {
		t.Errorf("Error() = %q, want %q", err.Error(), "test message")
	}
}

// Test BotDetect Name, Phase, Priority methods
func TestBotDetect_Methods(t *testing.T) {
	bd := NewBotDetect(BotDetectConfig{})

	if bd.Name() != "bot-detect" {
		t.Errorf("Name() = %q, want %q", bd.Name(), "bot-detect")
	}

	if bd.Phase() != PhasePreAuth {
		t.Errorf("Phase() = %v, want %v", bd.Phase(), PhasePreAuth)
	}

	if bd.Priority() != 3 {
		t.Errorf("Priority() = %d, want %d", bd.Priority(), 3)
	}
}

// Test BotDetectError Error method
func TestBotDetectError_Error(t *testing.T) {
	err := &BotDetectError{
		Code:    "bot_detected",
		Message: "bot detected",
		Status:  403,
	}

	if err.Error() != "bot detected" {
		t.Errorf("Error() = %q, want %q", err.Error(), "bot detected")
	}
}

// Test CircuitBreaker Name, Phase, Priority methods
func TestCircuitBreaker_Methods(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		ErrorThreshold:  0.5,
		VolumeThreshold: 10,
	})

	if cb.Name() != "circuit-breaker" {
		t.Errorf("Name() = %q, want %q", cb.Name(), "circuit-breaker")
	}

	if cb.Phase() != PhaseProxy {
		t.Errorf("Phase() = %v, want %v", cb.Phase(), PhaseProxy)
	}

	if cb.Priority() != 30 {
		t.Errorf("Priority() = %d, want %d", cb.Priority(), 30)
	}
}

// Test CircuitBreakerError Error method
func TestCircuitBreakerError_Error(t *testing.T) {
	err := &CircuitBreakerError{
		Code:    "circuit_open",
		Message: "circuit breaker is open",
		Status:  503,
	}

	if err.Error() != "circuit breaker is open" {
		t.Errorf("Error() = %q, want %q", err.Error(), "circuit breaker is open")
	}
}

// Test Compression Name, Phase, Priority methods
func TestCompression_Methods(t *testing.T) {
	c := NewCompression(CompressionConfig{})

	if c.Name() != "compression" {
		t.Errorf("Name() = %q, want %q", c.Name(), "compression")
	}

	if c.Phase() != PhasePostProxy {
		t.Errorf("Phase() = %v, want %v", c.Phase(), PhasePostProxy)
	}

	if c.Priority() != 50 {
		t.Errorf("Priority() = %d, want %d", c.Priority(), 50)
	}
}

// Test CorrelationID Name, Phase, Priority methods
func TestCorrelationID_Methods(t *testing.T) {
	cid := NewCorrelationID()

	if cid.Name() != "correlation-id" {
		t.Errorf("Name() = %q, want %q", cid.Name(), "correlation-id")
	}

	if cid.Phase() != PhasePreAuth {
		t.Errorf("Phase() = %v, want %v", cid.Phase(), PhasePreAuth)
	}

	if cid.Priority() != 0 {
		t.Errorf("Priority() = %d, want %d", cid.Priority(), 0)
	}
}

// Test IPRestriction Name, Phase, Priority methods
func TestIPRestrict_Methods(t *testing.T) {
	ipr, err := NewIPRestrict(IPRestrictConfig{
		Blacklist: []string{"192.168.1.1"},
	})
	if err != nil {
		t.Fatalf("NewIPRestrict error: %v", err)
	}

	if ipr.Name() != "ip-restrict" {
		t.Errorf("Name() = %q, want %q", ipr.Name(), "ip-restrict")
	}

	if ipr.Phase() != PhasePreAuth {
		t.Errorf("Phase() = %v, want %v", ipr.Phase(), PhasePreAuth)
	}

	if ipr.Priority() != 5 {
		t.Errorf("Priority() = %d, want %d", ipr.Priority(), 5)
	}
}

// Test IPRestrictError Error method
func TestIPRestrictError_Error(t *testing.T) {
	err := &IPRestrictError{
		Code:    "ip_blocked",
		Message: "IP is blocked",
		Status:  403,
	}

	if err.Error() != "IP is blocked" {
		t.Errorf("Error() = %q, want %q", err.Error(), "IP is blocked")
	}
}

// Test RateLimit Name, Phase, Priority methods
func TestRateLimit_Methods(t *testing.T) {
	rl, err := NewRateLimit(RateLimitConfig{
		Algorithm:         "token_bucket",
		RequestsPerSecond: 10,
		Burst:             20,
	})
	if err != nil {
		t.Fatalf("NewRateLimit error: %v", err)
	}

	if rl.Name() != "rate-limit" {
		t.Errorf("Name() = %q, want %q", rl.Name(), "rate-limit")
	}

	if rl.Phase() != PhasePreProxy {
		t.Errorf("Phase() = %v, want %v", rl.Phase(), PhasePreProxy)
	}

	if rl.Priority() != 20 {
		t.Errorf("Priority() = %d, want %d", rl.Priority(), 20)
	}
}

// Test RateLimitError Error method
func TestRateLimitError_Error(t *testing.T) {
	err := &RateLimitError{
		Code:       "rate_limited",
		Message:    "rate limit exceeded",
		Status:     429,
		RetryAfter: 60,
	}

	if err.Error() != "rate limit exceeded" {
		t.Errorf("Error() = %q, want %q", err.Error(), "rate limit exceeded")
	}
}

// Test RequestSizeLimit Name, Phase, Priority methods
func TestRequestSizeLimit_Methods(t *testing.T) {
	rsl := NewRequestSizeLimit(RequestSizeLimitConfig{
		MaxBytes: 1024 * 1024,
	})

	if rsl.Name() != "request-size-limit" {
		t.Errorf("Name() = %q, want %q", rsl.Name(), "request-size-limit")
	}

	if rsl.Phase() != PhasePreProxy {
		t.Errorf("Phase() = %v, want %v", rsl.Phase(), PhasePreProxy)
	}

	if rsl.Priority() != 25 {
		t.Errorf("Priority() = %d, want %d", rsl.Priority(), 25)
	}
}

// Test RequestValidator Name, Phase, Priority methods
func TestRequestValidator_Methods(t *testing.T) {
	rv, err := NewRequestValidator(RequestValidatorConfig{})
	if err != nil {
		t.Fatalf("NewRequestValidator error: %v", err)
	}

	if rv.Name() != "request-validator" {
		t.Errorf("Name() = %q, want %q", rv.Name(), "request-validator")
	}

	if rv.Phase() != PhasePreProxy {
		t.Errorf("Phase() = %v, want %v", rv.Phase(), PhasePreProxy)
	}

	if rv.Priority() != 30 {
		t.Errorf("Priority() = %d, want %d", rv.Priority(), 30)
	}
}

// Test Retry Name, Phase, Priority methods
func TestRetry_Methods(t *testing.T) {
	r := NewRetry(RetryConfig{
		MaxRetries: 3,
	})

	if r.Name() != "retry" {
		t.Errorf("Name() = %q, want %q", r.Name(), "retry")
	}

	if r.Phase() != PhaseProxy {
		t.Errorf("Phase() = %v, want %v", r.Phase(), PhaseProxy)
	}

	if r.Priority() != 20 {
		t.Errorf("Priority() = %d, want %d", r.Priority(), 20)
	}
}

// Test Timeout Name, Phase, Priority methods
func TestTimeout_Methods(t *testing.T) {
	to := NewTimeout(TimeoutConfig{
		Duration: 30 * time.Second,
	})

	if to.Name() != "timeout" {
		t.Errorf("Name() = %q, want %q", to.Name(), "timeout")
	}

	if to.Phase() != PhaseProxy {
		t.Errorf("Phase() = %v, want %v", to.Phase(), PhaseProxy)
	}

	if to.Priority() != 10 {
		t.Errorf("Priority() = %d, want %d", to.Priority(), 10)
	}
}

// Test URLRewrite Name, Phase, Priority methods
func TestURLRewrite_Methods(t *testing.T) {
	ur, err := NewURLRewrite(URLRewriteConfig{
		Pattern:     "/api/(.*)",
		Replacement: "/$1",
	})
	if err != nil {
		t.Fatalf("NewURLRewrite error: %v", err)
	}

	if ur.Name() != "url-rewrite" {
		t.Errorf("Name() = %q, want %q", ur.Name(), "url-rewrite")
	}

	if ur.Phase() != PhasePreProxy {
		t.Errorf("Phase() = %v, want %v", ur.Phase(), PhasePreProxy)
	}

	if ur.Priority() != 35 {
		t.Errorf("Priority() = %d, want %d", ur.Priority(), 35)
	}
}

// Test Redirect Name, Phase, Priority methods
func TestRedirect_Methods(t *testing.T) {
	r := NewRedirect(RedirectConfig{
		Rules: []RedirectRule{
			{Path: "/old", TargetURL: "/new", StatusCode: 301},
		},
	})

	if r.Name() != "redirect" {
		t.Errorf("Name() = %q, want %q", r.Name(), "redirect")
	}

	if r.Phase() != PhasePreProxy {
		t.Errorf("Phase() = %v, want %v", r.Phase(), PhasePreProxy)
	}

	if r.Priority() != 15 {
		t.Errorf("Priority() = %d, want %d", r.Priority(), 15)
	}
}

// Test UserIPWhitelist Name, Phase, Priority methods
func TestUserIPWhitelist_Methods(t *testing.T) {
	uiw := NewUserIPWhitelist()

	if uiw.Name() != "user-ip-whitelist" {
		t.Errorf("Name() = %q, want %q", uiw.Name(), "user-ip-whitelist")
	}

	if uiw.Phase() != PhasePreProxy {
		t.Errorf("Phase() = %v, want %v", uiw.Phase(), PhasePreProxy)
	}

	if uiw.Priority() != 12 {
		t.Errorf("Priority() = %d, want %d", uiw.Priority(), 12)
	}
}

// Test ResponseTransform Name, Phase, Priority methods
func TestResponseTransform_Methods(t *testing.T) {
	rt := NewResponseTransform(ResponseTransformConfig{
		RemoveHeaders: []string{"X-Internal"},
	})

	if rt.Name() != "response-transform" {
		t.Errorf("Name() = %q, want %q", rt.Name(), "response-transform")
	}

	if rt.Phase() != PhasePostProxy {
		t.Errorf("Phase() = %v, want %v", rt.Phase(), PhasePostProxy)
	}

	if rt.Priority() != 40 {
		t.Errorf("Priority() = %d, want %d", rt.Priority(), 40)
	}
}

// Test CaptureResponseWriter IsFlushed method
func TestCaptureResponseWriter_IsFlushed(t *testing.T) {
	w := &CaptureResponseWriter{}

	if w.IsFlushed() {
		t.Error("IsFlushed() should return false initially")
	}

	w.flushed = true
	if !w.IsFlushed() {
		t.Error("IsFlushed() should return true after flush")
	}
}

// Test EndpointPermission Name, Phase, Priority methods
func TestEndpointPermission_Methods(t *testing.T) {
	ep := NewEndpointPermission(func(userID, routeID string) (*EndpointPermissionRecord, error) {
		return nil, nil
	})

	if ep.Name() != "endpoint-permission" {
		t.Errorf("Name() = %q, want %q", ep.Name(), "endpoint-permission")
	}

	if ep.Phase() != PhasePreProxy {
		t.Errorf("Phase() = %v, want %v", ep.Phase(), PhasePreProxy)
	}

	if ep.Priority() != 15 {
		t.Errorf("Priority() = %d, want %d", ep.Priority(), 15)
	}
}

// Test CORS Name, Phase, Priority methods
func TestCORS_Methods(t *testing.T) {
	cors := NewCORS(CORSConfig{
		AllowedOrigins: []string{"*"},
	})

	if cors.Name() != "cors" {
		t.Errorf("Name() = %q, want %q", cors.Name(), "cors")
	}

	if cors.Phase() != PhasePreAuth {
		t.Errorf("Phase() = %v, want %v", cors.Phase(), PhasePreAuth)
	}

	if cors.Priority() != 1 {
		t.Errorf("Priority() = %d, want %d", cors.Priority(), 1)
	}
}

// Test Pipeline Plugins method
func TestPipeline_Plugins(t *testing.T) {
	plugins := []PipelinePlugin{
		{name: "cors", phase: PhasePreAuth, priority: 1},
		{name: "rate-limit", phase: PhasePreProxy, priority: 50},
	}

	p := NewPipeline(plugins)

	got := p.Plugins()
	if len(got) != len(plugins) {
		t.Errorf("Plugins() returned %d plugins, want %d", len(got), len(plugins))
	}
}

// Test generic plugin error interface compliance
func TestErrorInterface(t *testing.T) {
	// Test that all error types implement error interface
	errors := []error{
		&AuthError{Message: "test"},
		&JWTAuthError{Message: "test"},
		&BotDetectError{Message: "test"},
		&CircuitBreakerError{Message: "test"},
		&IPRestrictError{Message: "test"},
		&RateLimitError{Message: "test"},
	}

	for _, err := range errors {
		if err.Error() != "test" {
			t.Errorf("Error() returned unexpected value: %q", err.Error())
		}
	}
}

// Test claimValueToHeader function
func TestClaimValueToHeader(t *testing.T) {
	tests := []struct {
		name      string
		claim     any
		wantValue string
		wantOK    bool
	}{
		{
			name:      "string claim",
			claim:     "test-value",
			wantValue: "test-value",
			wantOK:    true,
		},
		{
			name:      "empty string claim",
			claim:     "",
			wantValue: "",
			wantOK:    false,
		},
		{
			name:      "int claim",
			claim:     42,
			wantValue: "42",
			wantOK:    true,
		},
		{
			name:      "bool claim true",
			claim:     true,
			wantValue: "true",
			wantOK:    true,
		},
		{
			name:      "bool claim false",
			claim:     false,
			wantValue: "false",
			wantOK:    true,
		},
		{
			name:      "float64 claim",
			claim:     3.14,
			wantValue: "3",
			wantOK:    true,
		},
		{
			name:      "nil claim",
			claim:     nil,
			wantValue: "<nil>",
			wantOK:    true,
		},
		{
			name:      "slice claim",
			claim:     []string{"a", "b"},
			wantValue: "[a b]",
			wantOK:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotValue, gotOK := claimValueToHeader(tt.claim)
			if gotValue != tt.wantValue {
				t.Errorf("claimValueToHeader() value = %q, want %q", gotValue, tt.wantValue)
			}
			if gotOK != tt.wantOK {
				t.Errorf("claimValueToHeader() ok = %v, want %v", gotOK, tt.wantOK)
			}
		})
	}
}

// Test hasClaimValue function
func TestHasClaimValue(t *testing.T) {
	tests := []struct {
		name     string
		claim    any
		expected bool
	}{
		{
			name:     "non-empty string",
			claim:    "value",
			expected: true,
		},
		{
			name:     "empty string",
			claim:    "",
			expected: false,
		},
		{
			name:     "whitespace only string",
			claim:    "   ",
			expected: false,
		},
		{
			name:     "nil",
			claim:    nil,
			expected: false,
		},
		{
			name:     "int",
			claim:    42,
			expected: true,
		},
		{
			name:     "bool true",
			claim:    true,
			expected: true,
		},
		{
			name:     "bool false",
			claim:    false,
			expected: true,
		},
		{
			name:     "empty slice",
			claim:    []string{},
			expected: false,
		},
		{
			name:     "non-empty slice",
			claim:    []string{"a"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasClaimValue(tt.claim)
			if got != tt.expected {
				t.Errorf("hasClaimValue() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// Test Retry MaxAttempts function
func TestRetry_MaxAttempts(t *testing.T) {
	r := NewRetry(RetryConfig{
		MaxRetries: 5,
	})

	// MaxAttempts returns maxRetries + 1
	if r.MaxAttempts("GET") != 6 {
		t.Errorf("MaxAttempts() = %d, want 6", r.MaxAttempts("GET"))
	}
}

// Test TimeoutError Error method
func TestTimeoutError_Error(t *testing.T) {
	err := &TimeoutError{Message: "timeout occurred"}
	if err.Error() != "timeout occurred" {
		t.Errorf("Error() = %q, want %q", err.Error(), "timeout occurred")
	}
}

// Test URLRewriteError Error method
func TestURLRewriteError_Error(t *testing.T) {
	err := &URLRewriteError{Message: "rewrite failed"}
	if err.Error() != "rewrite failed" {
		t.Errorf("Error() = %q, want %q", err.Error(), "rewrite failed")
	}
}

// Test UserIPWhitelistError Error method
func TestUserIPWhitelistError_Error(t *testing.T) {
	err := &UserIPWhitelistError{Message: "IP not whitelisted"}
	if err.Error() != "IP not whitelisted" {
		t.Errorf("Error() = %q, want %q", err.Error(), "IP not whitelisted")
	}
}

// Test RequestValidatorError Error method
func TestRequestValidatorError_Error(t *testing.T) {
	err := &RequestValidatorError{Message: "validation failed"}
	if err.Error() != "validation failed" {
		t.Errorf("Error() = %q, want %q", err.Error(), "validation failed")
	}
}

// Test normalizeIPRuleList function
func TestNormalizeIPRuleList(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "empty list",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "single IP",
			input:    []string{"192.168.1.1"},
			expected: []string{"192.168.1.1"},
		},
		{
			name:     "multiple IPs",
			input:    []string{"192.168.1.1", "10.0.0.0/8"},
			expected: []string{"192.168.1.1", "10.0.0.0/8"},
		},
		{
			name:     "with whitespace",
			input:    []string{" 192.168.1.1 ", " 10.0.0.0/8 "},
			expected: []string{"192.168.1.1", "10.0.0.0/8"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeIPRuleList(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("normalizeIPRuleList() returned %d items, want %d", len(got), len(tt.expected))
				return
			}
			for i, v := range got {
				if v != tt.expected[i] {
					t.Errorf("normalizeIPRuleList()[%d] = %q, want %q", i, v, tt.expected[i])
				}
			}
		})
	}
}

// Test AuthAPIKey Lookup method
func TestAuthAPIKey_Lookup(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{
			name:    "empty key",
			key:     "",
			wantErr: true,
		},
		{
			name:    "invalid key",
			key:     "invalid-key",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := NewAuthAPIKey(nil, AuthAPIKeyOptions{})

			_, err := auth.Lookup(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("Lookup() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Test AuthAPIKey DebugSummary method
func TestAuthAPIKey_DebugSummary(t *testing.T) {
	auth := NewAuthAPIKey(nil, AuthAPIKeyOptions{})

	summary := auth.DebugSummary()
	if summary == "" {
		t.Error("DebugSummary() returned empty string")
	}

	// Should contain expected format
	if !strings.Contains(summary, "consumers=") {
		t.Error("DebugSummary() should contain 'consumers='")
	}
	if !strings.Contains(summary, "keys=") {
		t.Error("DebugSummary() should contain 'keys='")
	}
}

// Test claimValueToHeader with additional types
func TestClaimValueToHeader_AdditionalTypes(t *testing.T) {
	tests := []struct {
		name      string
		claim     any
		wantValue string
		wantOK    bool
	}{
		{
			name:      "float32 value",
			claim:     float32(3.14),
			wantValue: "3",
			wantOK:    true,
		},
		{
			name:      "int value",
			claim:     int(42),
			wantValue: "42",
			wantOK:    true,
		},
		{
			name:      "[]any with values",
			claim:     []any{"a", "b", "c"},
			wantValue: "a,b,c",
			wantOK:    true,
		},
		{
			name:      "[]any with nil item",
			claim:     []any{"a", nil, "c"},
			wantValue: "a,c",
			wantOK:    true,
		},
		{
			name:      "[]any empty",
			claim:     []any{},
			wantValue: "",
			wantOK:    false,
		},
		{
			name:      "[]any all nil",
			claim:     []any{nil, nil},
			wantValue: "",
			wantOK:    false,
		},
		{
			name:      "default case with whitespace",
			claim:     struct{ Name string }{Name: "test"},
			wantValue: "{test}",
			wantOK:    true,
		},
		{
			name:      "default case empty",
			claim:     "",
			wantValue: "",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotValue, gotOK := claimValueToHeader(tt.claim)
			if gotValue != tt.wantValue {
				t.Errorf("claimValueToHeader() value = %q, want %q", gotValue, tt.wantValue)
			}
			if gotOK != tt.wantOK {
				t.Errorf("claimValueToHeader() ok = %v, want %v", gotOK, tt.wantOK)
			}
		})
	}
}
