package plugin

import (
	"fmt"
	"net/http"
	"strings"
)

// CORSConfig configures CORS behavior.
type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	MaxAge           int
	AllowCredentials bool
}

// CORS plugin applies cross-origin policies.
type CORS struct {
	origins          []string
	allowAllOrigins  bool
	methods          []string
	headers          []string
	maxAge           int
	allowCredentials bool
}

func NewCORS(cfg CORSConfig) *CORS {
	origins := normalizeList(cfg.AllowedOrigins)
	allowAllOrigins := false
	for _, origin := range origins {
		if origin == "*" {
			allowAllOrigins = true
			break
		}
	}

	// Security: reject wildcard origins entirely.
	// Even with AllowCredentials=false, returning Access-Control-Allow-Origin: *
	// on every response is a security misconfiguration (CWE-942). It allows any
	// site to read responses, enabling data theft. Additionally, the Vary: Origin
	// header must be set when origin is dynamic, which the current implementation
	// does not do for non-preflight responses.
	if allowAllOrigins {
		allowAllOrigins = false
		cfg.AllowCredentials = false
	}

	methods := normalizeList(cfg.AllowedMethods)
	if len(methods) == 0 {
		methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	}

	headers := normalizeList(cfg.AllowedHeaders)
	if len(headers) == 0 {
		headers = []string{"Authorization", "Content-Type"}
	}

	maxAge := cfg.MaxAge
	if maxAge < 0 {
		maxAge = 0
	}

	return &CORS{
		origins:          origins,
		allowAllOrigins:  allowAllOrigins,
		methods:          methods,
		headers:          headers,
		maxAge:           maxAge,
		allowCredentials: cfg.AllowCredentials,
	}
}

func (c *CORS) Name() string  { return "cors" }
func (c *CORS) Phase() Phase  { return PhasePreAuth }
func (c *CORS) Priority() int { return 1 }

// Handle applies CORS logic. Returns true when response is fully handled.
func (c *CORS) Handle(w http.ResponseWriter, r *http.Request) bool {
	if c == nil || w == nil || r == nil {
		return false
	}

	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return false
	}
	if !c.isOriginAllowed(origin) {
		http.Error(w, "Origin not allowed", http.StatusForbidden)
		return true
	}

	allowOrigin := c.allowOriginValue(origin)
	w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
	w.Header().Set("Vary", "Origin")
	if c.allowCredentials {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}

	if isCORSPreflight(r) {
		w.Header().Set("Access-Control-Allow-Methods", strings.Join(c.methods, ", "))
		w.Header().Set("Access-Control-Allow-Headers", strings.Join(c.headers, ", "))
		if c.maxAge > 0 {
			w.Header().Set("Access-Control-Max-Age", fmt.Sprintf("%d", c.maxAge))
		}
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

func (c *CORS) isOriginAllowed(origin string) bool {
	if c.allowAllOrigins {
		return true
	}
	for _, allowed := range c.origins {
		if strings.EqualFold(allowed, origin) {
			return true
		}
	}
	return false
}

func (c *CORS) allowOriginValue(requestOrigin string) string {
	if c.allowAllOrigins && !c.allowCredentials {
		return "*"
	}
	return requestOrigin
}

func isCORSPreflight(r *http.Request) bool {
	if r.Method != http.MethodOptions {
		return false
	}
	return strings.TrimSpace(r.Header.Get("Access-Control-Request-Method")) != ""
}

func normalizeList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}
