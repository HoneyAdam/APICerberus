package plugin

import (
	"fmt"
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

// --- BotDetect metadata ---

func TestBotDetect_Name(t *testing.T) {
	t.Parallel()
	b := NewBotDetect(BotDetectConfig{})
	if b.Name() != "bot-detect" {
		t.Errorf("Name() = %q, want %q", b.Name(), "bot-detect")
	}
}

func TestBotDetect_Phase(t *testing.T) {
	t.Parallel()
	b := NewBotDetect(BotDetectConfig{})
	if b.Phase() != PhasePreAuth {
		t.Errorf("Phase() = %v, want %v", b.Phase(), PhasePreAuth)
	}
}

func TestBotDetect_Priority(t *testing.T) {
	t.Parallel()
	b := NewBotDetect(BotDetectConfig{})
	if b.Priority() != 3 {
		t.Errorf("Priority() = %d, want 3", b.Priority())
	}
}

// --- CircuitBreaker plugin metadata ---

func TestCircuitBreakerPlugin_Name(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(CircuitBreakerConfig{})
	if cb.Name() != "circuit-breaker" {
		t.Errorf("Name() = %q, want %q", cb.Name(), "circuit-breaker")
	}
}

func TestCircuitBreakerPlugin_Phase(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(CircuitBreakerConfig{})
	if cb.Phase() != PhaseProxy {
		t.Errorf("Phase() = %v, want %v", cb.Phase(), PhaseProxy)
	}
}

func TestCircuitBreakerPlugin_Priority(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker(CircuitBreakerConfig{})
	if cb.Priority() != 30 {
		t.Errorf("Priority() = %d, want 30", cb.Priority())
	}
}

// --- Compression metadata ---

func TestCompression_Metadata(t *testing.T) {
	t.Parallel()
	c := NewCompression(CompressionConfig{})
	if c.Name() != "compression" {
		t.Errorf("Name() = %q", c.Name())
	}
	if c.Phase() != PhasePostProxy {
		t.Errorf("Phase() = %v", c.Phase())
	}
	if c.Priority() != 50 {
		t.Errorf("Priority() = %d", c.Priority())
	}
}

// --- CorrelationID metadata ---

func TestCorrelationID_Metadata(t *testing.T) {
	t.Parallel()
	c := NewCorrelationID()
	if c.Name() != "correlation-id" {
		t.Errorf("Name() = %q", c.Name())
	}
	if c.Phase() != PhasePreAuth {
		t.Errorf("Phase() = %v", c.Phase())
	}
	if c.Priority() != 0 {
		t.Errorf("Priority() = %d", c.Priority())
	}
}

// --- IPRestrict metadata ---

func TestIPRestrict_Metadata(t *testing.T) {
	t.Parallel()
	p, err := NewIPRestrict(IPRestrictConfig{})
	if err != nil {
		t.Fatalf("NewIPRestrict: %v", err)
	}
	if p.Name() != "ip-restrict" {
		t.Errorf("Name() = %q", p.Name())
	}
	if p.Phase() != PhasePreAuth {
		t.Errorf("Phase() = %v", p.Phase())
	}
	if p.Priority() != 5 {
		t.Errorf("Priority() = %d", p.Priority())
	}
}

// --- Timeout metadata ---

func TestTimeout_Metadata(t *testing.T) {
	t.Parallel()
	to := NewTimeout(TimeoutConfig{Duration: time.Second})
	if to.Name() != "timeout" {
		t.Errorf("Name() = %q", to.Name())
	}
	if to.Phase() != PhaseProxy {
		t.Errorf("Phase() = %v", to.Phase())
	}
	if to.Priority() != 10 {
		t.Errorf("Priority() = %d", to.Priority())
	}
}

// --- Retry metadata ---

func TestRetry_Metadata(t *testing.T) {
	t.Parallel()
	r := NewRetry(RetryConfig{MaxRetries: 3})
	if r.Name() != "retry" {
		t.Errorf("Name() = %q", r.Name())
	}
	if r.Phase() != PhaseProxy {
		t.Errorf("Phase() = %v", r.Phase())
	}
	if r.Priority() != 20 {
		t.Errorf("Priority() = %d", r.Priority())
	}
	if r.MaxAttempts("GET") != 4 { // 3 retries + 1 initial
		t.Errorf("MaxAttempts = %d, want 4", r.MaxAttempts("GET"))
	}
}

// --- RequestValidator metadata ---

func TestRequestValidator_Metadata(t *testing.T) {
	t.Parallel()
	v, err := NewRequestValidator(RequestValidatorConfig{})
	if err != nil {
		t.Fatalf("NewRequestValidator: %v", err)
	}
	if v.Name() != "request-validator" {
		t.Errorf("Name() = %q", v.Name())
	}
	if v.Phase() != PhasePreProxy {
		t.Errorf("Phase() = %v", v.Phase())
	}
	if v.Priority() != 30 {
		t.Errorf("Priority() = %d", v.Priority())
	}
}

// --- Redirect metadata ---

func TestRedirect_Metadata(t *testing.T) {
	t.Parallel()
	r := NewRedirect(RedirectConfig{})
	if r.Name() != "redirect" {
		t.Errorf("Name() = %q", r.Name())
	}
	if r.Phase() != PhasePreProxy {
		t.Errorf("Phase() = %v", r.Phase())
	}
	if r.Priority() != 15 {
		t.Errorf("Priority() = %d", r.Priority())
	}
}

// --- PluginError Error/As ---

func TestPluginError_Error(t *testing.T) {
	t.Parallel()
	e := &PluginError{Code: "test", Message: "something failed", Status: 400}
	if e.Error() != "something failed" {
		t.Errorf("Error() = %q", e.Error())
	}
}

func TestPluginError_As(t *testing.T) {
	t.Parallel()
	e := &PluginError{Code: "test", Message: "msg"}
	var target *PluginError
	if !e.As(&target) {
		t.Error("expected As to match **PluginError")
	}
	if target != e {
		t.Error("expected same pointer")
	}
}

func TestPluginError_As_WrongType(t *testing.T) {
	t.Parallel()
	e := &PluginError{Code: "test"}
	var target *string
	if e.As(&target) {
		t.Error("expected As to return false for wrong type")
	}
}

// --- TransformCaptureWriter BodyBytes/IsFlushed ---

func TestTransformCaptureWriter_BodyBytes_Nil(t *testing.T) {
	t.Parallel()
	var w *TransformCaptureWriter
	if w.BodyBytes() != nil {
		t.Error("expected nil for nil receiver")
	}
}

func TestTransformCaptureWriter_IsFlushed_Nil(t *testing.T) {
	t.Parallel()
	var w *TransformCaptureWriter
	if w.IsFlushed() {
		t.Error("expected false for nil receiver")
	}
}

// --- Pipeline Plugins() ---

func TestPipeline_Plugins_Nil(t *testing.T) {
	t.Parallel()
	var p *Pipeline
	if p.Plugins() != nil {
		t.Error("expected nil for nil pipeline")
	}
}

func TestPipeline_Plugins_Copy(t *testing.T) {
	t.Parallel()
	p := &Pipeline{plugins: []PipelinePlugin{
		{name: "a", phase: PhaseAuth, priority: 10},
		{name: "b", phase: PhaseProxy, priority: 20},
	}}
	plugs := p.Plugins()
	if len(plugs) != 2 {
		t.Fatalf("len = %d, want 2", len(plugs))
	}
	if plugs[0].Name() != "a" || plugs[1].Name() != "b" {
		t.Error("unexpected plugin names")
	}
}

// --- PipelinePlugin metadata ---

func TestPipelinePlugin_Metadata(t *testing.T) {
	t.Parallel()
	p := PipelinePlugin{name: "test", phase: PhaseAuth, priority: 42}
	if p.Name() != "test" {
		t.Errorf("Name() = %q", p.Name())
	}
	if p.Phase() != PhaseAuth {
		t.Errorf("Phase() = %v", p.Phase())
	}
	if p.Priority() != 42 {
		t.Errorf("Priority() = %d", p.Priority())
	}
}

func TestPipelinePlugin_Run_Nil(t *testing.T) {
	t.Parallel()
	p := PipelinePlugin{name: "test"}
	handled, err := p.Run(&PipelineContext{})
	if handled || err != nil {
		t.Errorf("handled=%v err=%v, want false nil", handled, err)
	}
}

func TestPipelinePlugin_AfterProxy_Nil(t *testing.T) {
	t.Parallel()
	p := PipelinePlugin{name: "test"}
	p.AfterProxy(&PipelineContext{}, nil) // should not panic
}

func TestPipelinePlugin_AfterProxy_WithFunc(t *testing.T) {
	t.Parallel()
	called := false
	p := PipelinePlugin{
		name:  "test",
		after: func(ctx *PipelineContext, err error) { called = true },
	}
	p.AfterProxy(&PipelineContext{}, nil)
	if !called {
		t.Error("expected after to be called")
	}
}

// --- NewPipelinePlugin ---

func TestNewPipelinePlugin(t *testing.T) {
	t.Parallel()
	run := func(ctx *PipelineContext) (bool, error) { return true, nil }
	p := NewPipelinePlugin("my-plugin", PhaseProxy, 15, run, nil)
	if p.Name() != "my-plugin" {
		t.Errorf("Name() = %q", p.Name())
	}
	if p.Phase() != PhaseProxy {
		t.Errorf("Phase() = %v", p.Phase())
	}
	if p.Priority() != 15 {
		t.Errorf("Priority() = %d", p.Priority())
	}
	handled, err := p.Run(&PipelineContext{})
	if !handled || err != nil {
		t.Errorf("Run: handled=%v err=%v", handled, err)
	}
}

// --- build*Plugin factory functions ---

func TestBuildCorrelationIDPlugin(t *testing.T) {
	t.Parallel()
	p, err := buildCorrelationIDPlugin(config.PluginConfig{}, BuilderContext{})
	if err != nil {
		t.Fatalf("buildCorrelationIDPlugin: %v", err)
	}
	if p.Name() != "correlation-id" {
		t.Errorf("Name() = %q", p.Name())
	}
}

func TestBuildBotDetectPlugin(t *testing.T) {
	t.Parallel()
	p, err := buildBotDetectPlugin(config.PluginConfig{Config: map[string]any{}}, BuilderContext{})
	if err != nil {
		t.Fatalf("buildBotDetectPlugin: %v", err)
	}
	if p.Name() != "bot-detect" {
		t.Errorf("Name() = %q", p.Name())
	}
}

func TestBuildIPRestrictPlugin(t *testing.T) {
	t.Parallel()
	p, err := buildIPRestrictPlugin(config.PluginConfig{Config: map[string]any{}}, BuilderContext{})
	if err != nil {
		t.Fatalf("buildIPRestrictPlugin: %v", err)
	}
	if p.Name() != "ip-restrict" {
		t.Errorf("Name() = %q", p.Name())
	}
}

func TestBuildAuthJWTPlugin(t *testing.T) {
	t.Parallel()
	p, err := buildAuthJWTPlugin(config.PluginConfig{Config: map[string]any{
		"secret": "test-secret-key-at-least-32-characters",
	}}, BuilderContext{})
	if err != nil {
		t.Fatalf("buildAuthJWTPlugin: %v", err)
	}
	if p.Name() != "auth-jwt" {
		t.Errorf("Name() = %q", p.Name())
	}
}

func TestBuildCircuitBreakerPlugin(t *testing.T) {
	t.Parallel()
	p, err := buildCircuitBreakerPlugin(config.PluginConfig{Config: map[string]any{}}, BuilderContext{})
	if err != nil {
		t.Fatalf("buildCircuitBreakerPlugin: %v", err)
	}
	if p.Name() != "circuit-breaker" {
		t.Errorf("Name() = %q", p.Name())
	}
}

func TestBuildRetryPlugin(t *testing.T) {
	t.Parallel()
	p, err := buildRetryPlugin(config.PluginConfig{Config: map[string]any{
		"max_retries": 3,
	}}, BuilderContext{})
	if err != nil {
		t.Fatalf("buildRetryPlugin: %v", err)
	}
	if p.Name() != "retry" {
		t.Errorf("Name() = %q", p.Name())
	}
}

func TestBuildTimeoutPlugin(t *testing.T) {
	t.Parallel()
	p, err := buildTimeoutPlugin(config.PluginConfig{Config: map[string]any{}}, BuilderContext{})
	if err != nil {
		t.Fatalf("buildTimeoutPlugin: %v", err)
	}
	if p.Name() != "timeout" {
		t.Errorf("Name() = %q", p.Name())
	}
}

func TestBuildCompressionPlugin(t *testing.T) {
	t.Parallel()
	p, err := buildCompressionPlugin(config.PluginConfig{Config: map[string]any{}}, BuilderContext{})
	if err != nil {
		t.Fatalf("buildCompressionPlugin: %v", err)
	}
	if p.Name() != "compression" {
		t.Errorf("Name() = %q", p.Name())
	}
}

func TestBuildRedirectPlugin(t *testing.T) {
	t.Parallel()
	p, err := buildRedirectPlugin(config.PluginConfig{Config: map[string]any{}}, BuilderContext{})
	if err != nil {
		t.Fatalf("buildRedirectPlugin: %v", err)
	}
	if p.Name() != "redirect" {
		t.Errorf("Name() = %q", p.Name())
	}
}

func TestBuildRequestSizeLimitPlugin(t *testing.T) {
	t.Parallel()
	p, err := buildRequestSizeLimitPlugin(config.PluginConfig{Config: map[string]any{
		"max_bytes": float64(1048576),
	}}, BuilderContext{})
	if err != nil {
		t.Fatalf("buildRequestSizeLimitPlugin: %v", err)
	}
	if p.Name() != "request-size-limit" {
		t.Errorf("Name() = %q", p.Name())
	}
}

func TestBuildRequestValidatorPlugin(t *testing.T) {
	t.Parallel()
	p, err := buildRequestValidatorPlugin(config.PluginConfig{Config: map[string]any{}}, BuilderContext{})
	if err != nil {
		t.Fatalf("buildRequestValidatorPlugin: %v", err)
	}
	if p.Name() != "request-validator" {
		t.Errorf("Name() = %q", p.Name())
	}
}

func TestBuildUserIPWhitelistPlugin(t *testing.T) {
	t.Parallel()
	p, err := buildUserIPWhitelistPlugin(config.PluginConfig{}, BuilderContext{})
	if err != nil {
		t.Fatalf("buildUserIPWhitelistPlugin: %v", err)
	}
	if p.Name() != "user-ip-whitelist" {
		t.Errorf("Name() = %q", p.Name())
	}
}

// --- Registry ---

func TestNewRegistry(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
}

func TestRegistry_RegisterAndBuild(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	err := r.Register("test-plugin", func(spec config.PluginConfig, ctx BuilderContext) (PipelinePlugin, error) {
		return PipelinePlugin{name: "test-plugin", phase: PhaseAuth, priority: 10}, nil
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	p, err := r.Build(config.PluginConfig{Name: "test-plugin"}, BuilderContext{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if p.Name() != "test-plugin" {
		t.Errorf("Name() = %q", p.Name())
	}
}

func TestRegistry_Build_NotFound(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_, err := r.Build(config.PluginConfig{Name: "nonexistent"}, BuilderContext{})
	if err == nil {
		t.Error("expected error for nonexistent plugin")
	}
}

func TestRegistry_Register_NilRegistry(t *testing.T) {
	t.Parallel()
	var r *Registry
	err := r.Register("test", nil)
	if err == nil {
		t.Error("expected error for nil registry")
	}
}

func TestRegistry_Lookup(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_ = r.Register("cors", buildCORSPlugin)
	factory, ok := r.Lookup("cors")
	if !ok {
		t.Error("expected to find cors")
	}
	if factory == nil {
		t.Error("expected non-nil factory")
	}
}

func TestRegistry_Lookup_NotFound(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_, ok := r.Lookup("nonexistent")
	if ok {
		t.Error("expected not found")
	}
}

func TestRegistry_Lookup_NilRegistry(t *testing.T) {
	t.Parallel()
	var r *Registry
	_, ok := r.Lookup("anything")
	if ok {
		t.Error("expected false for nil registry")
	}
}

func TestNewDefaultRegistry(t *testing.T) {
	t.Parallel()
	r := NewDefaultRegistry()
	if r == nil {
		t.Fatal("expected non-nil default registry")
	}
	// Should have standard plugins registered
	_, ok := r.Lookup("cors")
	if !ok {
		t.Error("expected cors plugin")
	}
	_, ok = r.Lookup("bot-detect")
	if !ok {
		t.Error("expected bot-detect plugin")
	}
}

// --- asRedirectRules ---

func TestAsRedirectRules_Nil(t *testing.T) {
	t.Parallel()
	if asRedirectRules(nil) != nil {
		t.Error("expected nil for nil input")
	}
}

func TestAsRedirectRules_EmptySlice(t *testing.T) {
	t.Parallel()
	if len(asRedirectRules([]any{})) != 0 {
		t.Error("expected empty for empty slice")
	}
}

func TestAsRedirectRules_ValidRules(t *testing.T) {
	t.Parallel()
	rules := asRedirectRules([]any{
		map[string]any{"path": "/old", "url": "https://example.com/new"},
		map[string]any{"from": "/a", "to": "https://example.com/b"},
	})
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].Path != "/old" {
		t.Errorf("path = %q", rules[0].Path)
	}
	if rules[1].Path != "/a" {
		t.Errorf("path = %q", rules[1].Path)
	}
}

func TestAsRedirectRules_InvalidType(t *testing.T) {
	t.Parallel()
	if asRedirectRules("not a slice") != nil {
		t.Error("expected nil for string")
	}
}

func TestAsRedirectRules_MissingFields(t *testing.T) {
	t.Parallel()
	rules := asRedirectRules([]any{
		map[string]any{"path": "/only-path"},             // no target
		map[string]any{"url": "https://example.com/only-target"}, // no path
	})
	if len(rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(rules))
	}
}

// --- asIntSlice ---

func TestAsIntSlice_Nil(t *testing.T) {
	t.Parallel()
	result := asIntSlice(nil, []int{200})
	if len(result) != 1 || result[0] != 200 {
		t.Errorf("expected [200], got %v", result)
	}
}

func TestAsIntSlice_IntSlice(t *testing.T) {
	t.Parallel()
	result := asIntSlice([]int{200, 301, 404}, nil)
	if len(result) != 3 {
		t.Fatalf("expected 3, got %d", len(result))
	}
	if result[0] != 200 || result[1] != 301 || result[2] != 404 {
		t.Errorf("unexpected: %v", result)
	}
}

func TestAsIntSlice_AnySlice(t *testing.T) {
	t.Parallel()
	result := asIntSlice([]any{float64(200), float64(301)}, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
	if result[0] != 200 || result[1] != 301 {
		t.Errorf("unexpected: %v", result)
	}
}

func TestAsIntSlice_CommaString(t *testing.T) {
	t.Parallel()
	result := asIntSlice("200,301,404", nil)
	if len(result) != 3 {
		t.Fatalf("expected 3, got %d", len(result))
	}
}

func TestAsIntSlice_EmptyString(t *testing.T) {
	t.Parallel()
	result := asIntSlice("  ", []int{500})
	if len(result) != 1 || result[0] != 500 {
		t.Errorf("expected fallback [500], got %v", result)
	}
}

func TestAsIntSlice_EmptyIntSlice(t *testing.T) {
	t.Parallel()
	result := asIntSlice([]int{}, []int{200})
	if len(result) != 1 || result[0] != 200 {
		t.Errorf("expected fallback, got %v", result)
	}
}

// --- pickFirstString ---

func TestPickFirstString(t *testing.T) {
	t.Parallel()
	if s := pickFirstString(nil, "hello"); s != "hello" {
		t.Errorf("expected hello, got %q", s)
	}
	if s := pickFirstString("", "fallback"); s != "fallback" {
		t.Errorf("expected fallback, got %q", s)
	}
	if s := pickFirstString(); s != "" {
		t.Errorf("expected empty, got %q", s)
	}
}

// --- consumerKey / routeKey ---

func TestConsumerKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		consumer *config.Consumer
		want     string
	}{
		{"nil", nil, "anonymous"},
		{"empty", &config.Consumer{}, "anonymous"},
		{"by_id", &config.Consumer{ID: "c-1", Name: "test"}, "c-1"},
		{"by_name", &config.Consumer{Name: "my-consumer"}, "my-consumer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := consumerKey(tt.consumer)
			if got != tt.want {
				t.Errorf("consumerKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRouteKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		route *config.Route
		req   *http.Request
		want  string
	}{
		{"nil_both", nil, nil, "unknown"},
		{"by_id", &config.Route{ID: "r-1", Name: "test"}, nil, "r-1"},
		{"by_name", &config.Route{Name: "my-route"}, nil, "my-route"},
		{"by_path", nil, httptest.NewRequest(http.MethodGet, "/api/v1/users", nil), "/api/v1/users"},
		{"route_over_req", &config.Route{ID: "r-1"}, httptest.NewRequest(http.MethodGet, "/api/v1/users", nil), "r-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := routeKey(tt.route, tt.req)
			if got != tt.want {
				t.Errorf("routeKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- valueMatchesType ---

func TestValueMatchesType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value    any
		expected string
		want     bool
	}{
		{"hello", "string", true},
		{123, "string", false},
		{3.14, "number", true},
		{"str", "number", false},
		{3.0, "integer", true},
		{3.5, "integer", false},
		{true, "boolean", true},
		{"x", "boolean", false},
		{map[string]any{"a": 1}, "object", true},
		{[]any{1, 2}, "array", true},
		{"any", "", true},
		{"any", "unknown", true},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v/%s", tt.value, tt.expected), func(t *testing.T) {
			t.Parallel()
			got := valueMatchesType(tt.value, tt.expected)
			if got != tt.want {
				t.Errorf("valueMatchesType(%v, %q) = %v, want %v", tt.value, tt.expected, got, tt.want)
			}
		})
	}
}

// --- routePipelineKey ---

func TestRoutePipelineKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		route *config.Route
		idx   int
		want  string
	}{
		{"nil", nil, 3, "route-3"},
		{"by_id", &config.Route{ID: "r-1"}, 0, "r-1"},
		{"by_name", &config.Route{Name: "my-route"}, 5, "my-route"},
		{"empty", &config.Route{}, 7, "route-7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := routePipelineKey(tt.route, tt.idx)
			if got != tt.want {
				t.Errorf("routePipelineKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- isPluginEnabled ---

func TestIsPluginEnabled(t *testing.T) {
	t.Parallel()
	t.Run("nil_enabled", func(t *testing.T) {
		t.Parallel()
		if !isPluginEnabled(config.PluginConfig{}) {
			t.Error("nil Enabled should default to true")
		}
	})
	t.Run("true", func(t *testing.T) {
		v := true
		if !isPluginEnabled(config.PluginConfig{Enabled: &v}) {
			t.Error("should be enabled")
		}
	})
	t.Run("false", func(t *testing.T) {
		v := false
		if isPluginEnabled(config.PluginConfig{Enabled: &v}) {
			t.Error("should be disabled")
		}
	})
}

// --- mergePluginSpecs ---

func TestMergePluginSpecs(t *testing.T) {
	t.Parallel()
	t.Run("both_empty", func(t *testing.T) {
		t.Parallel()
		if mergePluginSpecs(nil, nil) != nil {
			t.Error("expected nil")
		}
	})
	t.Run("global_only", func(t *testing.T) {
		t.Parallel()
		r := mergePluginSpecs([]config.PluginConfig{{Name: "cors"}}, nil)
		if len(r) != 1 || r[0].Name != "cors" {
			t.Errorf("got %v", r)
		}
	})
	t.Run("override", func(t *testing.T) {
		t.Parallel()
		r := mergePluginSpecs(
			[]config.PluginConfig{{Name: "cors", Config: map[string]any{"origins": "*"}}},
			[]config.PluginConfig{{Name: "cors", Config: map[string]any{"origins": "http://local"}}},
		)
		if len(r) != 1 {
			t.Fatalf("got %d", len(r))
		}
		if r[0].Config["origins"] != "http://local" {
			t.Error("route should override")
		}
	})
}
