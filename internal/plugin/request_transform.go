package plugin

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// RequestTransformConfig configures request mutation before proxying.
type RequestTransformConfig struct {
	AddHeaders    map[string]string
	RemoveHeaders []string
	RenameHeaders map[string]string

	AddQuery    map[string]string
	RemoveQuery []string
	RenameQuery map[string]string

	Method string
	Path   string

	PathPattern     string
	PathReplacement string

	// BodyHooks holds body-transform directives for JSON body manipulation.
	BodyHooks map[string]any
}

// RequestTransform mutates incoming requests before upstream forwarding.
type RequestTransform struct {
	addHeaders    map[string]string
	removeHeaders []string
	renameHeaders map[string]string

	addQuery    map[string]string
	removeQuery []string
	renameQuery map[string]string

	method string
	path   string

	pathRegex       *regexp.Regexp
	pathReplacement string

	bodyHooks map[string]any
}

func NewRequestTransform(cfg RequestTransformConfig) (*RequestTransform, error) {
	method := strings.ToUpper(strings.TrimSpace(cfg.Method))
	if method != "" && !isValidHTTPMethodToken(method) {
		return nil, fmt.Errorf("invalid method override %q", cfg.Method)
	}

	path := strings.TrimSpace(cfg.Path)
	if path != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	pathPattern := strings.TrimSpace(cfg.PathPattern)
	pathReplacement := strings.TrimSpace(cfg.PathReplacement)
	var pathRegex *regexp.Regexp
	if pathPattern != "" {
		compiled, err := regexp.Compile(pathPattern)
		if err != nil {
			return nil, fmt.Errorf("invalid path_pattern %q: %w", pathPattern, err)
		}
		pathRegex = compiled
	}

	addHeaders := normalizeHeaderMap(cfg.AddHeaders)
	removeHeaders := normalizeHeaderList(cfg.RemoveHeaders)
	renameHeaders := normalizeRenameHeaderMap(cfg.RenameHeaders)

	addQuery := normalizeStringMap(cfg.AddQuery)
	removeQuery := normalizeStringList(cfg.RemoveQuery)
	renameQuery := normalizeRenameStringMap(cfg.RenameQuery)

	bodyHooks := normalizeAnyMap(cfg.BodyHooks)

	return &RequestTransform{
		addHeaders:      addHeaders,
		removeHeaders:   removeHeaders,
		renameHeaders:   renameHeaders,
		addQuery:        addQuery,
		removeQuery:     removeQuery,
		renameQuery:     renameQuery,
		method:          method,
		path:            path,
		pathRegex:       pathRegex,
		pathReplacement: pathReplacement,
		bodyHooks:       bodyHooks,
	}, nil
}

func (t *RequestTransform) Name() string  { return "request-transform" }
func (t *RequestTransform) Phase() Phase  { return PhasePreProxy }
func (t *RequestTransform) Priority() int { return 40 }

// Apply mutates request attributes in-place before proxy forwarding.
func (t *RequestTransform) Apply(in *PipelineContext) error {
	if t == nil || in == nil || in.Request == nil {
		return nil
	}

	req := in.Request

	t.applyHeaderTransforms(req)
	t.applyQueryTransforms(req)

	if t.method != "" {
		req.Method = t.method
	}
	if t.pathRegex != nil {
		rewritten := t.pathRegex.ReplaceAllString(req.URL.Path, t.pathReplacement)
		if strings.TrimSpace(rewritten) == "" {
			rewritten = "/"
		}
		if !strings.HasPrefix(rewritten, "/") {
			rewritten = "/" + rewritten
		}
		req.URL.Path = rewritten
		req.URL.RawPath = rewritten
	}
	if t.path != "" {
		req.URL.Path = t.path
		req.URL.RawPath = t.path
	}

	// Body transforms are loaded into t.bodyHooks but not yet applied.
	// TODO: implement JSON body read/rewrite in POST body phase.
	_ = t.bodyHooks

	in.Request = req
	return nil
}

func (t *RequestTransform) applyHeaderTransforms(req *http.Request) {
	if req == nil {
		return
	}
	for _, key := range t.removeHeaders {
		req.Header.Del(key)
	}
	for from, to := range t.renameHeaders {
		values := append([]string(nil), req.Header.Values(from)...)
		if len(values) == 0 {
			continue
		}
		req.Header.Del(from)
		for _, value := range values {
			req.Header.Add(to, value)
		}
	}
	for key, value := range t.addHeaders {
		req.Header.Set(key, value)
	}
}

func (t *RequestTransform) applyQueryTransforms(req *http.Request) {
	if req == nil || req.URL == nil {
		return
	}
	query := req.URL.Query()
	for _, key := range t.removeQuery {
		query.Del(key)
	}
	for from, to := range t.renameQuery {
		values, ok := query[from]
		if !ok || len(values) == 0 {
			continue
		}
		delete(query, from)
		for _, value := range values {
			query.Add(to, value)
		}
	}
	for key, value := range t.addQuery {
		query.Set(key, value)
	}
	req.URL.RawQuery = query.Encode()
}

func isValidHTTPMethodToken(method string) bool {
	if method == "" {
		return false
	}
	for i := 0; i < len(method); i++ {
		ch := method[i]
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			return false
		}
	}
	return true
}

func normalizeHeaderList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = http.CanonicalHeaderKey(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func normalizeHeaderMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		key = http.CanonicalHeaderKey(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if key == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func normalizeRenameHeaderMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for from, to := range values {
		from = http.CanonicalHeaderKey(strings.TrimSpace(from))
		to = http.CanonicalHeaderKey(strings.TrimSpace(to))
		if from == "" || to == "" {
			continue
		}
		out[from] = to
	}
	return out
}

func normalizeStringList(values []string) []string {
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

func normalizeStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func normalizeRenameStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for from, to := range values {
		from = strings.TrimSpace(from)
		to = strings.TrimSpace(to)
		if from == "" || to == "" {
			continue
		}
		out[from] = to
	}
	return out
}

func normalizeAnyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = value
	}
	return out
}
