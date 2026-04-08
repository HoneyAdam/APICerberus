package plugin

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestRateLimitGlobalTokenBucket(t *testing.T) {
	t.Parallel()

	plugin, err := NewRateLimit(RateLimitConfig{
		Algorithm:         "token_bucket",
		Scope:             "global",
		RequestsPerSecond: 2,
		Burst:             2,
	})
	if err != nil {
		t.Fatalf("NewRateLimit error: %v", err)
	}
	plugin.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/users", nil)

	rr1 := httptest.NewRecorder()
	if ok := plugin.Enforce(rr1, RateLimitRequest{Request: req}); !ok {
		t.Fatalf("first request should be allowed")
	}
	rr2 := httptest.NewRecorder()
	if ok := plugin.Enforce(rr2, RateLimitRequest{Request: req}); !ok {
		t.Fatalf("second request should be allowed")
	}
	rr3 := httptest.NewRecorder()
	if ok := plugin.Enforce(rr3, RateLimitRequest{Request: req}); ok {
		t.Fatalf("third request should be limited")
	}
	if rr3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 got %d", rr3.Code)
	}
	if rr3.Header().Get("Retry-After") == "" {
		t.Fatalf("expected Retry-After header")
	}
	if rr1.Header().Get("X-RateLimit-Limit") != "2" {
		t.Fatalf("unexpected X-RateLimit-Limit header: %q", rr1.Header().Get("X-RateLimit-Limit"))
	}
}

func TestRateLimitConsumerScope(t *testing.T) {
	t.Parallel()

	plugin, err := NewRateLimit(RateLimitConfig{
		Algorithm: "fixed_window",
		Scope:     "consumer",
		Limit:     1,
		Window:    time.Second,
	})
	if err != nil {
		t.Fatalf("NewRateLimit error: %v", err)
	}

	now := time.Unix(1_700_000_000, 0)
	plugin.now = func() time.Time { return now }

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/users", nil)
	consumerA := &config.Consumer{ID: "consumer-a"}
	consumerB := &config.Consumer{ID: "consumer-b"}

	if ok := plugin.Enforce(httptest.NewRecorder(), RateLimitRequest{Request: req, Consumer: consumerA}); !ok {
		t.Fatalf("consumer A first request should be allowed")
	}
	if ok := plugin.Enforce(httptest.NewRecorder(), RateLimitRequest{Request: req, Consumer: consumerB}); !ok {
		t.Fatalf("consumer B first request should be allowed")
	}

	rr := httptest.NewRecorder()
	if ok := plugin.Enforce(rr, RateLimitRequest{Request: req, Consumer: consumerA}); ok {
		t.Fatalf("consumer A second request should be limited")
	}
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 got %d", rr.Code)
	}
}

func TestRateLimitIPScope(t *testing.T) {
	t.Parallel()

	plugin, err := NewRateLimit(RateLimitConfig{
		Algorithm: "fixed_window",
		Scope:     "ip",
		Limit:     1,
		Window:    time.Second,
	})
	if err != nil {
		t.Fatalf("NewRateLimit error: %v", err)
	}

	now := time.Unix(1_700_000_000, 0)
	plugin.now = func() time.Time { return now }

	reqA := httptest.NewRequest(http.MethodGet, "http://gateway.local/users", nil)
	reqA.RemoteAddr = "10.0.0.1:1234"
	reqB := httptest.NewRequest(http.MethodGet, "http://gateway.local/users", nil)
	reqB.RemoteAddr = "10.0.0.2:1234"

	if ok := plugin.Enforce(httptest.NewRecorder(), RateLimitRequest{Request: reqA}); !ok {
		t.Fatalf("ip A should be allowed")
	}
	if ok := plugin.Enforce(httptest.NewRecorder(), RateLimitRequest{Request: reqB}); !ok {
		t.Fatalf("ip B should be allowed")
	}
	if ok := plugin.Enforce(httptest.NewRecorder(), RateLimitRequest{Request: reqA}); ok {
		t.Fatalf("ip A second request should be limited")
	}
}

func TestRateLimitCompositeScope(t *testing.T) {
	t.Parallel()

	plugin, err := NewRateLimit(RateLimitConfig{
		Algorithm:       "fixed_window",
		Scope:           "composite",
		Limit:           1,
		Window:          time.Second,
		CompositeScopes: []string{"consumer", "route"},
	})
	if err != nil {
		t.Fatalf("NewRateLimit error: %v", err)
	}

	now := time.Unix(1_700_000_000, 0)
	plugin.now = func() time.Time { return now }

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/users", nil)
	consumer := &config.Consumer{ID: "consumer-a"}
	routeA := &config.Route{ID: "route-a"}
	routeB := &config.Route{ID: "route-b"}

	if ok := plugin.Enforce(httptest.NewRecorder(), RateLimitRequest{Request: req, Consumer: consumer, Route: routeA}); !ok {
		t.Fatalf("first request for route A should be allowed")
	}
	if ok := plugin.Enforce(httptest.NewRecorder(), RateLimitRequest{Request: req, Consumer: consumer, Route: routeB}); !ok {
		t.Fatalf("first request for route B should be allowed (different composite key)")
	}
	if ok := plugin.Enforce(httptest.NewRecorder(), RateLimitRequest{Request: req, Consumer: consumer, Route: routeA}); ok {
		t.Fatalf("second request for route A should be limited")
	}
}

func TestRateLimitAlgorithmFactoryAndComparativeBehavior(t *testing.T) {
	t.Parallel()

	// Keep test far from fixed-window boundary (window=5s) for stable assertions.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if time.Now().Unix()%5 <= 2 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	makeRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "http://gateway.local/users", nil)
		req.RemoteAddr = "203.0.113.10:1234"
		return req
	}

	token, err := NewRateLimit(RateLimitConfig{
		Algorithm:         "token_bucket",
		Scope:             "global",
		RequestsPerSecond: 2,
		Burst:             2,
	})
	if err != nil {
		t.Fatalf("token bucket init error: %v", err)
	}
	fixed, err := NewRateLimit(RateLimitConfig{
		Algorithm: "fixed_window",
		Scope:     "global",
		Limit:     2,
		Window:    5 * time.Second,
	})
	if err != nil {
		t.Fatalf("fixed window init error: %v", err)
	}
	sliding, err := NewRateLimit(RateLimitConfig{
		Algorithm: "sliding_window",
		Scope:     "global",
		Limit:     2,
		Window:    5 * time.Second,
	})
	if err != nil {
		t.Fatalf("sliding window init error: %v", err)
	}
	leaky, err := NewRateLimit(RateLimitConfig{
		Algorithm:         "leaky_bucket",
		Scope:             "global",
		Burst:             2,
		RequestsPerSecond: 2,
	})
	if err != nil {
		t.Fatalf("leaky bucket init error: %v", err)
	}

	// Same burst load for all algorithms.
	for _, limiter := range []*RateLimit{token, fixed, sliding, leaky} {
		if ok := limiter.Enforce(httptest.NewRecorder(), RateLimitRequest{Request: makeRequest()}); !ok {
			t.Fatalf("first request should pass for %s", limiter.algorithm)
		}
		if ok := limiter.Enforce(httptest.NewRecorder(), RateLimitRequest{Request: makeRequest()}); !ok {
			t.Fatalf("second request should pass for %s", limiter.algorithm)
		}
		if ok := limiter.Enforce(httptest.NewRecorder(), RateLimitRequest{Request: makeRequest()}); ok {
			t.Fatalf("third request should be limited for %s", limiter.algorithm)
		}
	}

	time.Sleep(600 * time.Millisecond)
	if ok := token.Enforce(httptest.NewRecorder(), RateLimitRequest{Request: makeRequest()}); !ok {
		t.Fatalf("token bucket should recover capacity after partial refill")
	}
	if ok := leaky.Enforce(httptest.NewRecorder(), RateLimitRequest{Request: makeRequest()}); !ok {
		t.Fatalf("leaky bucket should allow after drain")
	}
	if ok := fixed.Enforce(httptest.NewRecorder(), RateLimitRequest{Request: makeRequest()}); ok {
		t.Fatalf("fixed window should still block until full window reset")
	}
	if ok := sliding.Enforce(httptest.NewRecorder(), RateLimitRequest{Request: makeRequest()}); ok {
		t.Fatalf("sliding window should still block in early window interval")
	}
}

func TestRateLimitPermissionOverride(t *testing.T) {
	t.Parallel()

	plugin, err := NewRateLimit(RateLimitConfig{
		Algorithm: "fixed_window",
		Scope:     "global",
		Limit:     100,
		Window:    time.Second,
	})
	if err != nil {
		t.Fatalf("NewRateLimit error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/users", nil)
	override := map[string]any{
		"algorithm": "fixed_window",
		"scope":     "global",
		"limit":     1,
		"window":    "1s",
	}

	if ok := plugin.Enforce(httptest.NewRecorder(), RateLimitRequest{
		Request:  req,
		Metadata: map[string]any{metadataPermissionRateLimitOverride: override},
	}); !ok {
		t.Fatalf("first override request should be allowed")
	}

	rr := httptest.NewRecorder()
	if ok := plugin.Enforce(rr, RateLimitRequest{
		Request:  req,
		Metadata: map[string]any{metadataPermissionRateLimitOverride: override},
	}); ok {
		t.Fatalf("second override request should be limited")
	}
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 got %d", rr.Code)
	}
}

func TestRateLimitUserOverride(t *testing.T) {
	t.Parallel()

	plugin, err := NewRateLimit(RateLimitConfig{
		Algorithm: "fixed_window",
		Scope:     "global",
		Limit:     100,
		Window:    time.Second,
	})
	if err != nil {
		t.Fatalf("NewRateLimit error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/users", nil)
	consumer := &config.Consumer{
		ID: "user-1",
		Metadata: map[string]any{
			"rate_limits": map[string]any{
				"algorithm": "fixed_window",
				"scope":     "consumer",
				"limit":     1,
				"window":    "1s",
			},
		},
	}

	if ok := plugin.Enforce(httptest.NewRecorder(), RateLimitRequest{Request: req, Consumer: consumer}); !ok {
		t.Fatalf("first user override request should be allowed")
	}
	rr := httptest.NewRecorder()
	if ok := plugin.Enforce(rr, RateLimitRequest{Request: req, Consumer: consumer}); ok {
		t.Fatalf("second user override request should be limited")
	}
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 got %d", rr.Code)
	}
}

// TestNormalizeAny tests the normalizeAny function with various types
func TestNormalizeAny(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{"nil", nil, "nil"},
		{"string", "Hello", "hello"},
		{"string with spaces", "  World  ", "world"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"int", 42, "42"},
		{"int64", int64(9223372036854775807), "9223372036854775807"},
		{"float64", 3.14159, "3.14159"},
		{"float32", float32(2.5), "2.5"},
		{"[]any", []any{"a", "b", "c"}, "[a,b,c]"},
		{"[]string", []string{"x", "y", "z"}, "[x,y,z]"},
		{"map[string]any", map[string]any{"key": "value"}, "key=value"},
		{"unknown type", struct{ Name string }{"test"}, "{test}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeAny(tt.value)
			// For struct types, just check it's not empty
			if tt.name == "unknown type" {
				if result == "" {
					t.Errorf("normalizeAny(%v) returned empty string", tt.value)
				}
				return
			}
			if result != tt.expected {
				t.Errorf("normalizeAny(%v) = %q, want %q", tt.value, result, tt.expected)
			}
		})
	}
}
