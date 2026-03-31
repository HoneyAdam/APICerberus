package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// APIVersion represents an API version
type APIVersion struct {
	Version     string
	Prefix      string
	Deprecated  bool
	SunsetDate  string
	Handler     http.Handler
	Middlewares []func(http.Handler) http.Handler
}

// VersionRouter handles API versioning
type VersionRouter struct {
	mu       sync.RWMutex
	versions map[string]*APIVersion
	defaultV string
	strategy VersionStrategy
}

// VersionStrategy determines how versions are selected
type VersionStrategy string

const (
	// StrategyPath uses URL path (e.g., /v1/users)
	StrategyPath VersionStrategy = "path"

	// StrategyHeader uses Accept-Version header
	StrategyHeader VersionStrategy = "header"

	// StrategyQuery uses query parameter (e.g., ?version=v1)
	StrategyQuery VersionStrategy = "query"

	// StrategyHost uses subdomain (e.g., v1.api.example.com)
	StrategyHost VersionStrategy = "host"
)

// NewVersionRouter creates a version router
func NewVersionRouter(strategy VersionStrategy) *VersionRouter {
	return &VersionRouter{
		versions: make(map[string]*APIVersion),
		strategy: strategy,
	}
}

// RegisterVersion registers an API version
func (vr *VersionRouter) RegisterVersion(v *APIVersion) error {
	vr.mu.Lock()
	defer vr.mu.Unlock()

	if v.Version == "" {
		return fmt.Errorf("version is required")
	}

	vr.versions[v.Version] = v

	// Set as default if first version
	if vr.defaultV == "" {
		vr.defaultV = v.Version
	}

	return nil
}

// SetDefault sets the default version
func (vr *VersionRouter) SetDefault(version string) error {
	vr.mu.Lock()
	defer vr.mu.Unlock()

	if _, exists := vr.versions[version]; !exists {
		return fmt.Errorf("version not found: %s", version)
	}

	vr.defaultV = version
	return nil
}

// GetVersion gets a specific version
func (vr *VersionRouter) GetVersion(version string) (*APIVersion, bool) {
	vr.mu.RLock()
	defer vr.mu.RUnlock()
	v, exists := vr.versions[version]
	return v, exists
}

// ServeHTTP implements http.Handler
func (vr *VersionRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	version := vr.extractVersion(r)

	vr.mu.RLock()
	v, exists := vr.versions[version]
	if !exists {
		// Fall back to default
		v = vr.versions[vr.defaultV]
	}
	vr.mu.RUnlock()

	if v == nil {
		http.Error(w, "API version not found", http.StatusNotFound)
		return
	}

	// Add deprecation warning if applicable
	if v.Deprecated {
		w.Header().Set("Deprecation", "true")
		if v.SunsetDate != "" {
			w.Header().Set("Sunset", v.SunsetDate)
		}
	}

	// Add version info
	w.Header().Set("X-API-Version", v.Version)

	// Apply middlewares
	handler := v.Handler
	for i := len(v.Middlewares) - 1; i >= 0; i-- {
		handler = v.Middlewares[i](handler)
	}

	handler.ServeHTTP(w, r)
}

// extractVersion extracts version from request based on strategy
func (vr *VersionRouter) extractVersion(r *http.Request) string {
	switch vr.strategy {
	case StrategyPath:
		return extractFromPath(r.URL.Path)
	case StrategyHeader:
		return r.Header.Get("Accept-Version")
	case StrategyQuery:
		return r.URL.Query().Get("version")
	case StrategyHost:
		return extractFromHost(r.Host)
	default:
		return vr.defaultV
	}
}

// extractFromPath extracts version from URL path
func extractFromPath(path string) string {
	// Path format: /v1/users, /v2/users, etc.
	parts := strings.Split(path, "/")
	if len(parts) >= 2 && strings.HasPrefix(parts[1], "v") {
		return parts[1]
	}
	return ""
}

// extractFromHost extracts version from host
func extractFromHost(host string) string {
	// Host format: v1.api.example.com, v2.api.example.com
	parts := strings.Split(host, ".")
	if len(parts) >= 3 && strings.HasPrefix(parts[0], "v") {
		return parts[0]
	}
	return ""
}

// VersionMiddleware adds version info to response
func VersionMiddleware(version string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-API-Version", version)
			next.ServeHTTP(w, r)
		})
	}
}

// DeprecationMiddleware adds deprecation headers
func DeprecationMiddleware(sunsetDate string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Deprecation", "true")
			if sunsetDate != "" {
				w.Header().Set("Sunset", sunsetDate)
			}
			w.Header().Set("Link", fmt.Sprintf("</v2%s>; rel=successor-version", r.URL.Path))
			next.ServeHTTP(w, r)
		})
	}
}

// APIVersionManager manages multiple API versions
type APIVersionManager struct {
	mu        sync.RWMutex
	router    *VersionRouter
	versions  map[string]*VersionConfig
}

// VersionConfig holds version configuration
type VersionConfig struct {
	Version     string
	Stable      bool
	Deprecated  bool
	SunsetDate  string
	Description string
}

// NewAPIVersionManager creates a version manager
func NewAPIVersionManager(strategy VersionStrategy) *APIVersionManager {
	return &APIVersionManager{
		router:   NewVersionRouter(strategy),
		versions: make(map[string]*VersionConfig),
	}
}

// RegisterVersion registers a version
func (m *APIVersionManager) RegisterVersion(config VersionConfig, handler http.Handler) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.versions[config.Version] = &config

	v := &APIVersion{
		Version:    config.Version,
		Prefix:     "/" + config.Version,
		Deprecated: config.Deprecated,
		SunsetDate: config.SunsetDate,
		Handler:    handler,
	}

	return m.router.RegisterVersion(v)
}

// GetVersions returns all registered versions
func (m *APIVersionManager) GetVersions() []VersionConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	versions := make([]VersionConfig, 0, len(m.versions))
	for _, v := range m.versions {
		versions = append(versions, *v)
	}
	return versions
}

// GetVersionInfo returns info about a version
func (m *APIVersionManager) GetVersionInfo(version string) (VersionConfig, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	v, exists := m.versions[version]
	if !exists {
		return VersionConfig{}, false
	}
	return *v, true
}

// Router returns the version router
func (m *APIVersionManager) Router() *VersionRouter {
	return m.router
}

// DiscoveryHandler returns version discovery info
func (m *APIVersionManager) DiscoveryHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m.mu.RLock()
		versions := make([]VersionConfig, 0, len(m.versions))
		for _, v := range m.versions {
			versions = append(versions, *v)
		}
		m.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"versions": versions,
		})
	}
}
