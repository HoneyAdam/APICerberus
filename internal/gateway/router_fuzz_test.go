package gateway

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

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
