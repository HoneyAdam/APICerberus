package plugin

import (
	"net/http"
	"net/url"
	"strings"
)

// RedirectRule maps one path to a redirect target and status code.
type RedirectRule struct {
	Path       string
	TargetURL  string
	StatusCode int
}

// RedirectConfig configures redirect plugin behavior.
type RedirectConfig struct {
	Rules []RedirectRule
}

// Redirect performs early redirect response based on request path.
type Redirect struct {
	rules []RedirectRule
}

func NewRedirect(cfg RedirectConfig) *Redirect {
	rules := make([]RedirectRule, 0, len(cfg.Rules))
	for _, rule := range cfg.Rules {
		path := strings.TrimSpace(rule.Path)
		target := strings.TrimSpace(rule.TargetURL)
		if path == "" || target == "" {
			continue
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		status := normalizeRedirectStatus(rule.StatusCode)
		rules = append(rules, RedirectRule{
			Path:       path,
			TargetURL:  target,
			StatusCode: status,
		})
	}
	return &Redirect{rules: rules}
}

func (r *Redirect) Name() string  { return "redirect" }
func (r *Redirect) Phase() Phase  { return PhasePreProxy }
func (r *Redirect) Priority() int { return 15 }

func (r *Redirect) Handle(w http.ResponseWriter, req *http.Request) bool {
	if r == nil || w == nil || req == nil || req.URL == nil {
		return false
	}
	for _, rule := range r.rules {
		if req.URL.Path != rule.Path {
			continue
		}
		// Do not append original query parameters to redirect target.
		// Original query may contain tokens, session IDs, or other
		// sensitive data that should not be leaked to external domains.
		http.Redirect(w, req, rule.TargetURL, rule.StatusCode)
		return true
	}
	return false
}

func normalizeRedirectStatus(code int) int {
	switch code {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return code
	default:
		return http.StatusFound
	}
}

//lint:ignore U1000 test-only helper for redirect URL query string handling
func appendQueryIfMissing(target, rawQuery string) string {
	if strings.TrimSpace(target) == "" || strings.TrimSpace(rawQuery) == "" {
		return target
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return target
	}
	if parsed.RawQuery != "" {
		return target
	}
	parsed.RawQuery = rawQuery
	return parsed.String()
}
