package plugin

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// URLRewriteConfig configures path rewrite behavior.
type URLRewriteConfig struct {
	Pattern     string
	Replacement string
}

// URLRewrite rewrites request URL path using regex replacement.
type URLRewrite struct {
	pattern     *regexp.Regexp
	replacement string
}

func NewURLRewrite(cfg URLRewriteConfig) (*URLRewrite, error) {
	pattern := strings.TrimSpace(cfg.Pattern)
	if pattern == "" {
		return nil, fmt.Errorf("url rewrite pattern is required")
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid url rewrite pattern %q: %w", pattern, err)
	}
	return &URLRewrite{
		pattern:     compiled,
		replacement: strings.TrimSpace(cfg.Replacement),
	}, nil
}

func (u *URLRewrite) Name() string  { return "url-rewrite" }
func (u *URLRewrite) Phase() Phase  { return PhasePreProxy }
func (u *URLRewrite) Priority() int { return 35 }

// Apply rewrites request path in-place and preserves query string.
func (u *URLRewrite) Apply(in *PipelineContext) error {
	if u == nil || in == nil || in.Request == nil || in.Request.URL == nil {
		return nil
	}
	rewritten := u.pattern.ReplaceAllString(in.Request.URL.Path, u.replacement)
	if strings.TrimSpace(rewritten) == "" {
		rewritten = "/"
	}
	if !strings.HasPrefix(rewritten, "/") {
		rewritten = "/" + rewritten
	}
	in.Request.URL.Path = rewritten
	in.Request.URL.RawPath = rewritten
	return nil
}

// URLRewriteError indicates invalid plugin setup.
type URLRewriteError struct {
	PluginError
}

var ErrURLRewriteInvalid = &URLRewriteError{
	PluginError: PluginError{
		Code:    "invalid_url_rewrite",
		Message: "URL rewrite configuration is invalid",
		Status:  http.StatusBadRequest,
	},
}
