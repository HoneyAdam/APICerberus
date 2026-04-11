package gateway

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// FuzzRouterMatch tests the radix tree router against malformed and
// adversarial inputs: path traversal, null bytes, Unicode,超长 strings,
// deeply nested paths, and malformed query strings.
func FuzzRouterMatch(f *testing.F) {
	// Seed corpus with common attack patterns
	seeds := []string{
		"/api/v1/users",
		"/api/v1/users/123",
		"/../../../etc/passwd",
		"/%00",
		"/%2e%2e/%2e%2e/etc/passwd",
		"/api/v1/users?id=1' OR '1'='1",
		"/<script>alert(1)</script>",
		"/api/v1/users/" + string(make([]byte, 10000)),
		"/" + string(make([]byte, 65536)),
		"/a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s/t/u/v/w/x/y/z",
		"/api/健康检查",
		"/api/\u0000test",
		"/api/v1/users/..%2F..%2Fetc%2Fpasswd",
		"/api/v1/users/;%20cat%20/etc/passwd",
		"/api/v1/users/{id}/profile",
		"/",
		"//",
		"///",
	}

	services := []config.Service{
		{
			ID:       "svc-api",
			Name:     "svc-api",
			Protocol: "http",
			Upstream: "up-api",
		},
	}
	routes := []config.Route{
		{
			ID:      "route-users",
			Name:    "route-users",
			Service: "svc-api",
			Paths:   []string{"/api/v1/users/*id"},
			Methods: []string{"GET", "POST", "PUT", "DELETE"},
		},
		{
			ID:      "route-health",
			Name:    "route-health",
			Service: "svc-api",
			Paths:   []string{"/health", "/healthz"},
			Methods: []string{"GET"},
		},
		{
			ID:      "route-wildcard",
			Name:    "route-wildcard",
			Service: "svc-api",
			Paths:   []string{"/*path"},
			Methods: []string{"*"},
		},
	}

	router, err := NewRouter(routes, services)
	if err != nil {
		f.Fatalf("failed to create router: %v", err)
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, path string) {
		// Cap path length to prevent stack overflow during tree traversal
		if len(path) > 8192 {
			path = path[:8192]
		}
		// Strip null bytes — url.Parse rejects them
		path = strings.ReplaceAll(path, "\x00", "")
		if path == "" {
			return
		}

		// Test with various methods
		methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD", "", "GET\n", "GET\r\n"}
		for _, method := range methods {
			u, err := url.Parse("http://example.com" + path)
			if err != nil {
				return // skip malformed URLs
			}
			req := &http.Request{
				Method: method,
				URL:    u,
				Host:   "example.com",
				Header: make(http.Header),
			}

			// Router.Match must never panic
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("Match panicked with path=%q method=%q: %v", path, method, r)
					}
				}()
				_, _, _ = router.Match(req)
			}()
		}
	})
}

// FuzzCompileRegex tests the compileRegex function against adversarial regex
// patterns including ReDoS (catastrophic backtracking) patterns, malformed
// regex, and pathological quantifiers.
func FuzzCompileRegex(f *testing.F) {
	seeds := []string{
		"/api/*",
		"/api/v[0-9]+/*id",
		"^/api/(v[0-9]+)/users/[0-9]+$",
		"(a+)+",
		"(a+)*b",
		"(a|)+",
		"([a-zA-Z]+)*$",
		"(.*.*)*$",
		"^(a|a?)+$",
		"^(a|aa)+$",
		"^(a|aaa)+$",
		"(a|b|ab)*$",
		"^(.*){1,100}$",
		"^.{1000}$",
		"^(a{1,100}){1,100}$",
		`[a-z]+@[a-z]+\.[a-z]+`,
		"^/healthz?$",
		"",
		"^",
		"$",
		`\\`,
		"[",
		"]",
		"(",
		")",
		"{",
		"}",
		"?",
		"+",
		"*",
		"|",
		"^$^$^$",
		`\d+\w+\s+`,
		"/api/{id}",
		"/api/{slug:[a-z-]+}",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, pattern string) {
		if len(pattern) > 2048 {
			pattern = pattern[:2048]
		}

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("compileRegex panicked on pattern %q: %v", pattern, r)
			}
		}()

		re, err := compileRegex(pattern)
		if err != nil {
			return
		}

		if re != nil {
			testInput := strings.Repeat("a", 25) + "!"
			done := make(chan bool, 1)
			go func() {
				_ = re.MatchString(testInput)
				done <- true
			}()
			select {
			case <-done:
			case <-time.After(500 * time.Millisecond):
				t.Errorf("compileRegex(%q) produced regex that took >500ms (possible ReDoS)", pattern)
			}
		}
	})
}

// TestCompileRegex_ReDosPatterns verifies that known ReDoS patterns are either
// rejected or complete quickly.
func TestCompileRegex_ReDosPatterns(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
	}{
		{"nested_quantifier_1", "(a+)+"},
		{"nested_quantifier_2", "(a+)*b"},
		{"alternation_explosion", "(a|)+"},
		{"nested_groups", "([a-zA-Z]+)*$"},
		{"double_wildcard", "(.*.*)*$"},
		{"large_quantifier", "^(.*){1,100}$"},
		{"exponential_backtrack", "^(a|a?)+$"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			re, err := compileRegex(tt.pattern)
			if err != nil {
				return
			}
			done := make(chan bool, 1)
			go func() {
				_ = re.MatchString(strings.Repeat("a", 30) + "!")
				done <- true
			}()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("regex match took >1s — likely ReDoS vulnerability")
			}
		})
	}
}

// TestCompileRegex_BoundsValidation tests length and anchoring constraints.
func TestCompileRegex_BoundsValidation(t *testing.T) {
	longPattern := strings.Repeat("a", maxRegexLength+1)
	_, err := compileRegex(longPattern)
	if err == nil {
		t.Error("expected error for over-length pattern")
	}

	re, err := compileRegex("/api/*")
	if err != nil {
		t.Fatalf("unexpected error for valid pattern: %v", err)
	}
	if re == nil {
		t.Error("expected non-nil regex")
	}

	re2, err := compileRegex("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if re2.MatchString("test_extra") {
		t.Error("auto-anchored regex should not match prefix")
	}
	if !re2.MatchString("test") {
		t.Error("auto-anchored regex should match exact string")
	}
}

// TestRouterRegexRoutes tests that routes with regex patterns compile and match.
func TestRouterRegexRoutes(t *testing.T) {
	routes := []config.Route{
		{
			ID:      "regex-versioned",
			Name:    "regex-versioned",
			Service: "svc-api",
			Paths:   []string{"/api/v[0-9]+/.*"},
			Methods: []string{"GET"},
		},
		{
			ID:      "regex-slug",
			Name:    "regex-slug",
			Service: "svc-api",
			Paths:   []string{"/blog/[a-z0-9-]+"},
			Methods: []string{"GET"},
		},
	}
	services := []config.Service{
		{ID: "svc-api", Name: "svc-api", Protocol: "http", Upstream: "up1"},
	}

	router, err := NewRouter(routes, services)
	if err != nil {
		t.Fatalf("failed to create router: %v", err)
	}

	tests := []struct {
		path   string
		method string
		match  bool
	}{
		{"/api/v1/users", "GET", true},
		{"/api/v2/posts", "GET", true},
		{"/api/abc/posts", "GET", false},
		{"/blog/my-post-title", "GET", true},
		{"/blog/INVALID_POST", "GET", false},
		{"/api/v1", "GET", false}, // trailing slash required by pattern
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			u, _ := url.Parse("http://example.com" + tt.path)
			req := &http.Request{Method: tt.method, URL: u, Host: "example.com", Header: make(http.Header)}
			route, _, err := router.Match(req)
			if tt.match {
				if err != nil || route == nil {
					t.Errorf("expected match for %s, got err=%v", tt.path, err)
				}
			} else {
				if route != nil {
					t.Errorf("expected no match for %s, got route=%s", tt.path, route.ID)
				}
			}
		})
	}
}

var _ = regexp.Compile // ensure regexp import is used
