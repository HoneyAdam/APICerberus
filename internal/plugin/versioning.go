package plugin

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// VersioningConfig controls API version extraction and header injection.
type VersioningConfig struct {
	DefaultVersion string
	Versions       []string
	StripPrefix    bool
	HeaderName     string
	Deprecation    map[string]DeprecationInfo
}

// DeprecationInfo holds deprecation metadata for an API version.
type DeprecationInfo struct {
	Sunset   time.Time
	Message  string
	Link     string
	Disabled bool
}

// Versioning extracts the API version from the URL path, injects version
// headers for upstream services, and handles version deprecation notices.
//
// URL convention: /v{N}/... where N is a positive integer.
// Example: /v1/users → version "1", path /users.
type Versioning struct {
	defaultVersion string
	versions       map[string]struct{}
	stripPrefix    bool
	headerName     string
	deprecation    map[string]DeprecationInfo
}

func NewVersioning(cfg VersioningConfig) *Versioning {
	versions := make(map[string]struct{}, len(cfg.Versions))
	for _, v := range cfg.Versions {
		v = strings.TrimSpace(v)
		if v != "" {
			versions[v] = struct{}{}
		}
	}
	headerName := cfg.HeaderName
	if headerName == "" {
		headerName = "X-API-Version"
	}
	deprecation := make(map[string]DeprecationInfo, len(cfg.Deprecation))
	for k, v := range cfg.Deprecation {
		deprecation[k] = v
	}
	return &Versioning{
		defaultVersion: strings.TrimSpace(cfg.DefaultVersion),
		versions:       versions,
		stripPrefix:    cfg.StripPrefix,
		headerName:     headerName,
		deprecation:    deprecation,
	}
}

func (v *Versioning) Name() string  { return "versioning" }
func (v *Versioning) Phase() Phase  { return PhasePreProxy }
func (v *Versioning) Priority() int { return 8 }

// Apply extracts the version from the URL, validates it, rewrites the path if
// configured, and adds version headers. Returns true to stop the pipeline only
// when the version is deprecated and disabled.
func (v *Versioning) Apply(ctx *PipelineContext) (bool, error) {
	if v == nil || ctx == nil || ctx.Request == nil {
		return false, nil
	}

	req := ctx.Request
	path := req.URL.Path
	version, remaining := extractVersion(path)

	if version == "" {
		if v.defaultVersion == "" {
			return false, nil
		}
		version = v.defaultVersion
	} else if v.stripPrefix && remaining != path {
		req.URL.Path = remaining
	}

	// Validate version when an allowlist is configured.
	if len(v.versions) > 0 {
		if _, ok := v.versions[version]; !ok {
			http.Error(ctx.ResponseWriter,
				fmt.Sprintf(`{"error":"unsupported API version: %s","message":"Supported versions: %s"}`, version, v.supportedList()),
				http.StatusNotFound)
			return true, nil
		}
	}

	// Inject version header for upstream.
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Header.Set(v.headerName, version)

	// Add version to pipeline metadata for downstream plugins.
	if ctx.Metadata == nil {
		ctx.Metadata = make(map[string]any)
	}
	ctx.Metadata["api_version"] = version

	// Handle deprecation.
	if info, ok := v.deprecation[version]; ok {
		if info.Disabled {
			http.Error(ctx.ResponseWriter,
				fmt.Sprintf(`{"error":"API version %s has been removed","message":"%s"}`, version, info.Message),
				http.StatusGone)
			return true, nil
		}
		w := ctx.ResponseWriter
		if w != nil {
			w.Header().Set("Deprecation", "true")
			if !info.Sunset.IsZero() {
				w.Header().Set("Sunset", info.Sunset.Format(time.RFC1123))
			}
			if info.Message != "" {
				w.Header().Set("X-Deprecation-Notice", info.Message)
			}
			if info.Link != "" {
				w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="deprecation"`, info.Link))
			}
		}
		log.Printf("[INFO] versioning: request to deprecated version %s from %s", version, req.RemoteAddr)
	}

	return false, nil
}

func (v *Versioning) supportedList() string {
	versions := make([]string, 0, len(v.versions))
	for v := range v.versions {
		versions = append(versions, v)
	}
	return strings.Join(versions, ", ")
}

// extractVersion parses /v{N}/... paths and returns the version string and
// the remaining path. If no version prefix is found, it returns ("", path).
func extractVersion(path string) (version, remaining string) {
	if len(path) < 3 {
		return "", path
	}
	// Look for /v{digits}/ at the start.
	if path[0] != '/' {
		return "", path
	}
	// Find the next slash after /v
	idx := strings.IndexByte(path[1:], '/')
	if idx < 0 {
		// Path is like /v1 (no trailing slash)
		candidate := path[1:]
		if len(candidate) > 1 && candidate[0] == 'v' {
			numStr := candidate[1:]
			if isDigits(numStr) {
				return numStr, "/"
			}
		}
		return "", path
	}
	candidate := path[1 : 1+idx]
	if len(candidate) > 1 && candidate[0] == 'v' {
		numStr := candidate[1:]
		if isDigits(numStr) {
			return numStr, path[1+idx:]
		}
	}
	return "", path
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
