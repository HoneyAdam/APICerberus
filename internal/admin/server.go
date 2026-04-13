package admin

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/APICerberus/APICerebrus/internal/analytics"
	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/federation"
	"github.com/APICerberus/APICerebrus/internal/gateway"
	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/pkg/uuid"
	yamlpkg "github.com/APICerberus/APICerebrus/internal/pkg/yaml"
	"github.com/APICerberus/APICerebrus/internal/store"
	"github.com/APICerberus/APICerebrus/internal/version"
)

// Sentinel errors for admin operations — use errors.Is() to match.
var (
	errServiceNotFound  = errors.New("service not found")
	errServiceInUse     = errors.New("service is referenced by route")
	errRouteNotFound    = errors.New("route not found")
	errUpstreamNotFound = errors.New("upstream not found")
	errUpstreamInUse    = errors.New("upstream is referenced by service")
	errTargetNotFound   = errors.New("target not found")
)

type Server struct {
	mu             sync.RWMutex
	cfg            *config.Config
	gateway        *gateway.Gateway
	alertEngine    *analytics.AlertEngine
	webhookManager *WebhookManager
	mux            *http.ServeMux
	dashboardFS    fs.FS

	startedAt time.Time

	// Rate limiting for admin API authentication
	rlMu            sync.RWMutex
	rlAttempts      map[string]*adminAuthAttempts
	rlCleanupTicker *time.Ticker
	rlStopCh        chan struct{}

	// Lifecycle
	closeOnce sync.Once
	closed    bool

}

type adminAuthAttempts struct {
	count     int
	firstSeen time.Time
	lastSeen  time.Time
	blocked   bool
}

const emptyMapImportSentinel = "__apicerberus_empty_map__"

func NewServer(cfg *config.Config, gw *gateway.Gateway) (*Server, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}
	if gw == nil {
		return nil, errors.New("gateway is nil")
	}

	s := &Server{
		cfg:         cfg,
		gateway:     gw,
		alertEngine: analytics.NewAlertEngine(analytics.AlertEngineOptions{}),
		mux:         http.NewServeMux(),
		startedAt:   time.Now(),
		rlAttempts:  make(map[string]*adminAuthAttempts),
		rlStopCh:    make(chan struct{}),
	}
	s.startRateLimitCleanup()
	SetTrustedProxies(cfg.Gateway.TrustedProxies)

	if gw.Store() != nil {
		s.webhookManager = NewWebhookManager(gw.Store())
		s.webhookManager.Start()
	}

	if cfg.Admin.UIEnabled {
		dashboardFS, err := embeddedDashboardFS()
		if err != nil {
			return nil, fmt.Errorf("load embedded dashboard assets: %w", err)
		}
		s.dashboardFS = dashboardFS
	}
	s.registerRoutes()
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	s.mux.ServeHTTP(w, r)
}

// Close stops background goroutines and releases resources.
func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		s.closed = true
		if s.rlCleanupTicker != nil {
			s.rlCleanupTicker.Stop()
		}
		if s.webhookManager != nil {
			s.webhookManager.Stop()
		}
		close(s.rlStopCh)
	})
	return nil
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("POST /admin/api/v1/auth/token", s.withAdminStaticAuth(s.handleTokenIssue))
	s.mux.HandleFunc("POST /admin/api/v1/auth/logout", s.withAdminBearerAuth(s.handleTokenLogout))
	s.mux.HandleFunc("GET /admin/api/v1/auth/sso/login", s.handleOIDCLogin)
	s.mux.HandleFunc("GET /admin/api/v1/auth/sso/callback", s.handleOIDCCallback)
	s.mux.HandleFunc("POST /admin/api/v1/auth/sso/logout", s.withAdminBearerAuth(s.handleOIDCLogout))
	s.mux.HandleFunc("GET /admin/api/v1/auth/sso/status", s.withAdminBearerAuth(s.handleOIDCStatus))
	s.mux.HandleFunc("POST /admin/login", s.handleFormLogin)
	s.mux.HandleFunc("POST /admin/logout", s.handleFormLogout)
	s.handle("GET /admin/api/v1/status", s.handleStatus)
	s.mux.HandleFunc("GET /admin/api/v1/info", s.withAdminBearerAuth(s.handleInfo))
	s.handle("GET /admin/api/v1/branding", s.handleBranding)
	s.mux.HandleFunc("GET /admin/api/v1/branding/public", s.handleBrandingPublic)
	s.handle("GET /admin/api/v1/config/export", s.handleConfigExport)
	s.handle("POST /admin/api/v1/config/import", s.handleConfigImport)
	s.handle("POST /admin/api/v1/config/reload", s.handleConfigReload)

	s.handle("GET /admin/api/v1/services", s.listServices)
	s.handle("POST /admin/api/v1/services", s.createService)
	s.handle("GET /admin/api/v1/services/{id}", s.getService)
	s.handle("PUT /admin/api/v1/services/{id}", s.updateService)
	s.handle("DELETE /admin/api/v1/services/{id}", s.deleteService)

	s.handle("GET /admin/api/v1/routes", s.listRoutes)
	s.handle("POST /admin/api/v1/routes", s.createRoute)
	s.handle("GET /admin/api/v1/routes/{id}", s.getRoute)
	s.handle("PUT /admin/api/v1/routes/{id}", s.updateRoute)
	s.handle("DELETE /admin/api/v1/routes/{id}", s.deleteRoute)

	s.handle("GET /admin/api/v1/upstreams", s.listUpstreams)
	s.handle("POST /admin/api/v1/upstreams", s.createUpstream)
	s.handle("GET /admin/api/v1/upstreams/{id}", s.getUpstream)
	s.handle("PUT /admin/api/v1/upstreams/{id}", s.updateUpstream)
	s.handle("DELETE /admin/api/v1/upstreams/{id}", s.deleteUpstream)
	s.handle("POST /admin/api/v1/upstreams/{id}/targets", s.addUpstreamTarget)
	s.handle("DELETE /admin/api/v1/upstreams/{id}/targets/{tid}", s.deleteUpstreamTarget)
	s.handle("GET /admin/api/v1/upstreams/{id}/health", s.getUpstreamHealth)

	s.handle("GET /admin/api/v1/users", s.listUsers)
	s.handle("POST /admin/api/v1/users", s.createUser)
	s.handle("GET /admin/api/v1/users/{id}", s.getUser)
	s.handle("PUT /admin/api/v1/users/{id}", s.updateUser)
	s.handle("DELETE /admin/api/v1/users/{id}", s.deleteUser)
	s.handle("POST /admin/api/v1/users/{id}/suspend", s.suspendUser)
	s.handle("POST /admin/api/v1/users/{id}/activate", s.activateUser)
	s.handle("PUT /admin/api/v1/users/{id}/status", s.updateUserStatusUnified)
	s.handle("PUT /admin/api/v1/users/{id}/role", s.updateUserRole)
	s.handle("POST /admin/api/v1/users/{id}/reset-password", s.resetUserPassword)

	s.handle("GET /admin/api/v1/users/{id}/api-keys", s.listUserAPIKeys)
	s.handle("POST /admin/api/v1/users/{id}/api-keys", s.createUserAPIKey)
	s.handle("DELETE /admin/api/v1/users/{id}/api-keys/{keyId}", s.revokeUserAPIKey)

	s.handle("GET /admin/api/v1/users/{id}/permissions", s.listUserPermissions)
	s.handle("POST /admin/api/v1/users/{id}/permissions", s.createUserPermission)
	s.handle("PUT /admin/api/v1/users/{id}/permissions/{pid}", s.updateUserPermission)
	s.handle("DELETE /admin/api/v1/users/{id}/permissions/{pid}", s.deleteUserPermission)
	s.handle("POST /admin/api/v1/users/{id}/permissions/bulk", s.bulkAssignUserPermissions)

	s.handle("GET /admin/api/v1/users/{id}/ip-whitelist", s.listUserIPWhitelist)
	s.handle("POST /admin/api/v1/users/{id}/ip-whitelist", s.addUserIPWhitelist)
	s.handle("DELETE /admin/api/v1/users/{id}/ip-whitelist/{ip}", s.deleteUserIPWhitelist)

	s.handle("GET /admin/api/v1/credits/overview", s.creditOverview)
	s.handle("POST /admin/api/v1/users/{id}/credits", s.adjustCreditsUnified)
	s.handle("POST /admin/api/v1/users/{id}/credits/topup", s.topupCredits)
	s.handle("POST /admin/api/v1/users/{id}/credits/deduct", s.deductCredits)
	s.handle("GET /admin/api/v1/users/{id}/credits", s.userCreditOverview)
	s.handle("GET /admin/api/v1/users/{id}/credits/balance", s.userCreditBalance)
	s.handle("GET /admin/api/v1/users/{id}/credits/transactions", s.listCreditTransactions)
	s.handle("GET /admin/api/v1/audit-logs", s.searchAuditLogs)
	s.handle("GET /admin/api/v1/audit-logs/{id}", s.getAuditLog)
	s.handle("GET /admin/api/v1/audit-logs/export", s.exportAuditLogs)
	s.handle("GET /admin/api/v1/audit-logs/stats", s.auditLogStats)
	s.handle("DELETE /admin/api/v1/audit-logs/cleanup", s.cleanupAuditLogs)
	s.handle("GET /admin/api/v1/users/{id}/audit-logs", s.searchUserAuditLogs)
	s.handle("GET /admin/api/v1/analytics/overview", s.analyticsOverview)
	s.handle("GET /admin/api/v1/analytics/timeseries", s.analyticsTimeSeries)
	s.handle("GET /admin/api/v1/analytics/top-routes", s.analyticsTopRoutes)
	s.handle("GET /admin/api/v1/analytics/top-consumers", s.analyticsTopConsumers)
	s.handle("GET /admin/api/v1/analytics/errors", s.analyticsErrors)
	s.handle("GET /admin/api/v1/analytics/latency", s.analyticsLatency)
	s.handle("GET /admin/api/v1/analytics/throughput", s.analyticsThroughput)
	s.handle("GET /admin/api/v1/analytics/status-codes", s.analyticsStatusCodes)
	s.handle("GET /admin/api/v1/alerts", s.listAlerts)
	s.handle("POST /admin/api/v1/alerts", s.createAlert)
	s.handle("PUT /admin/api/v1/alerts/{id}", s.updateAlert)
	s.handle("DELETE /admin/api/v1/alerts/{id}", s.deleteAlert)

	s.handle("GET /admin/api/v1/billing/config", s.getBillingConfig)
	s.handle("PUT /admin/api/v1/billing/config", s.updateBillingConfig)
	s.handle("GET /admin/api/v1/billing/route-costs", s.getBillingRouteCosts)
	s.handle("PUT /admin/api/v1/billing/route-costs", s.updateBillingRouteCosts)

	s.handle("GET /admin/api/v1/subgraphs", s.listSubgraphs)
	s.handle("POST /admin/api/v1/subgraphs", s.addSubgraph)
	s.handle("GET /admin/api/v1/subgraphs/{id}", s.getSubgraph)
	s.handle("DELETE /admin/api/v1/subgraphs/{id}", s.removeSubgraph)
	s.handle("POST /admin/api/v1/subgraphs/compose", s.composeSubgraphs)

	s.mux.HandleFunc("GET /admin/api/v1/ws", s.handleRealtimeWebSocket)

	// Register advanced analytics routes
	s.RegisterAdvancedAnalyticsRoutes()

	// Register bulk operation routes
	s.RegisterBulkRoutes()
	s.RegisterBulkImportRoute()

	// Register GraphQL routes
	s.RegisterGraphQLRoutes()

	// Register webhook routes
	s.RegisterWebhookRoutes()

	if s.dashboardFS != nil {
		s.mux.Handle("/", s.newDashboardHandler())
	}
}

func (s *Server) handle(pattern string, handler http.HandlerFunc) {
	s.mux.HandleFunc(pattern, s.withAdminBearerAuth(handler))
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	storeMetrics := map[string]any{}
	if st := s.gateway.Store(); st != nil {
		if db := st.DB(); db != nil {
			stats := db.Stats()
			storeMetrics = map[string]any{
				"open_connections":    stats.OpenConnections,
				"in_use":              stats.InUse,
				"idle":                stats.Idle,
				"wait_count":          stats.WaitCount,
				"wait_duration_ms":    stats.WaitDuration.Milliseconds(),
				"max_open_conns":      stats.MaxOpenConnections,
			}
		}
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"store":  storeMetrics,
	})
}

func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	cfg := s.cfg
	startedAt := s.startedAt
	s.mu.RUnlock()

	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"version":    version.Version,
		"commit":     version.Commit,
		"build_time": version.BuildTime,
		"uptime_sec": int(time.Since(startedAt).Seconds()),
		"summary": map[string]any{
			"services":  len(cfg.Services),
			"routes":    len(cfg.Routes),
			"upstreams": len(cfg.Upstreams),
		},
	})
}

func (s *Server) handleBranding(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	b := s.cfg.Branding
	s.mu.RUnlock()

	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"app_name":      orDefault(b.AppName, "API Cerberus"),
		"logo_url":      b.LogoURL,
		"favicon_url":   b.FaviconURL,
		"primary_color": orDefault(b.PrimaryColor, "rgb(109 40 217)"),
		"accent_color":  b.AccentColor,
		"theme_mode":    orDefault(b.ThemeMode, "system"),
	})
}

// handleBrandingPublic returns branding config without authentication (for login page).
func (s *Server) handleBrandingPublic(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	b := s.cfg.Branding
	s.mu.RUnlock()

	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"app_name":      orDefault(b.AppName, "API Cerberus"),
		"logo_url":      b.LogoURL,
		"favicon_url":   b.FaviconURL,
		"primary_color": orDefault(b.PrimaryColor, "rgb(109 40 217)"),
		"accent_color":  b.AccentColor,
		"theme_mode":    orDefault(b.ThemeMode, "system"),
	})
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func (s *Server) handleConfigReload(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	next := config.CloneConfig(s.cfg)
	s.mu.RUnlock()

	if err := s.gateway.Reload(next); err != nil {
		writeError(w, http.StatusBadRequest, "config_reload_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{"reloaded": true})
}

func (s *Server) handleConfigExport(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	cfg := config.CloneConfig(s.cfg)
	s.mu.RUnlock()

	// Redact all secrets from the config before exporting
	redactSecrets(cfg)

	raw, err := yamlpkg.Marshal(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_export_failed", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/x-yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

// redactSecrets replaces all secret/key/password fields in the config with
// a masked placeholder before export.
func redactSecrets(cfg *config.Config) {
	if cfg == nil {
		return
	}
	// Admin secrets
	cfg.Admin.APIKey = "***redacted***"
	cfg.Admin.TokenSecret = "***redacted***"

	// Portal session secret
	cfg.Portal.Session.Secret = "***redacted***"

	// Redis password
	cfg.Redis.Password = "***redacted***"

	// Kafka SASL credentials
	cfg.Kafka.SASL.Password = "***redacted***"

	// Tracing OTLP headers (may contain auth tokens)
	for k := range cfg.Tracing.OTLPHeaders {
		cfg.Tracing.OTLPHeaders[k] = "***redacted***"
	}

	// Consumer API keys — redact the actual key values
	for i := range cfg.Consumers {
		for j := range cfg.Consumers[i].APIKeys {
			cfg.Consumers[i].APIKeys[j].Key = "***redacted***"
		}
	}

	// TLS key file paths are not secrets themselves, but SkipVerify is a
	// security-sensitive flag — leave it visible for audit purposes.
}

func (s *Server) handleConfigImport(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	if strings.TrimSpace(string(raw)) == "" {
		writeError(w, http.StatusBadRequest, "invalid_payload", "empty config payload")
		return
	}
	normalized := normalizeYAMLEmptyMaps(raw, emptyMapImportSentinel)

	// Create temp file in a restricted directory to prevent other users from reading imported config (CWE-377)
	importDir := os.TempDir()
	if dir := os.Getenv("APICERBERUS_TMPDIR"); dir != "" {
		importDir = dir
	}
	file, err := os.CreateTemp(importDir, "apicerberus-config-import-*.yaml")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "config_import_failed", err.Error())
		return
	}
	path := file.Name()
	_ = file.Close()
	defer os.Remove(path)

	// Safe: path is from os.CreateTemp, not user-controlled.
	//nolint:gosec // G703: path is sanitized via CreateTemp
	if err := os.WriteFile(path, normalized, 0o600); err != nil {
		writeError(w, http.StatusInternalServerError, "config_import_failed", err.Error())
		return
	}
	loaded, err := config.Load(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_config", err.Error())
		return
	}
	cleanupImportedConfigSentinel(loaded, emptyMapImportSentinel)
	next := config.CloneConfig(loaded)
	if err := s.mutateConfig(func(cfg *config.Config) error {
		*cfg = *next
		return nil
	}); err != nil {
		writeError(w, http.StatusBadRequest, "config_import_failed", err.Error())
		return
	}

	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"imported": true,
		"summary": map[string]any{
			"services":  len(next.Services),
			"routes":    len(next.Routes),
			"upstreams": len(next.Upstreams),
		},
	})
}

func normalizeYAMLEmptyMaps(raw []byte, sentinel string) []byte {
	if len(raw) == 0 {
		return nil
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "{}" {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		lines[i] = indent + sentinel + ": 0"
	}
	return []byte(strings.Join(lines, "\n"))
}

func cleanupImportedConfigSentinel(cfg *config.Config, sentinel string) {
	if cfg == nil {
		return
	}
	if cfg.Billing.RouteCosts != nil {
		delete(cfg.Billing.RouteCosts, sentinel)
	}
	if cfg.Billing.MethodMultipliers != nil {
		delete(cfg.Billing.MethodMultipliers, sentinel)
	}
	if cfg.Audit.RouteRetentionDays != nil {
		delete(cfg.Audit.RouteRetentionDays, sentinel)
	}
	for i := range cfg.Consumers {
		if cfg.Consumers[i].Metadata != nil {
			delete(cfg.Consumers[i].Metadata, sentinel)
		}
	}
	for i := range cfg.GlobalPlugins {
		if cfg.GlobalPlugins[i].Config != nil {
			delete(cfg.GlobalPlugins[i].Config, sentinel)
		}
	}
	for i := range cfg.Routes {
		for j := range cfg.Routes[i].Plugins {
			if cfg.Routes[i].Plugins[j].Config != nil {
				delete(cfg.Routes[i].Plugins[j].Config, sentinel)
			}
		}
	}
}

func (s *Server) mutateConfig(mutator func(*config.Config) error) error {
	s.mu.RLock()
	next := config.CloneConfig(s.cfg)
	s.mu.RUnlock()

	if err := mutator(next); err != nil {
		return err
	}
	if err := s.gateway.Reload(next); err != nil {
		return err
	}

	s.mu.Lock()
	s.cfg = next
	s.mu.Unlock()
	return nil
}

func (s *Server) openStore() (*store.Store, error) {
	s.mu.RLock()
	cfg := config.CloneConfig(s.cfg)
	s.mu.RUnlock()
	return store.Open(cfg)
}

func (s *Server) listSubgraphs(w http.ResponseWriter, _ *http.Request) {
	mgr := s.gateway.Subgraphs()
	if mgr == nil {
		writeError(w, http.StatusBadRequest, "federation_disabled", "Federation is not enabled")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, mgr.ListSubgraphs())
}

func (s *Server) addSubgraph(w http.ResponseWriter, r *http.Request) {
	mgr := s.gateway.Subgraphs()
	if mgr == nil {
		writeError(w, http.StatusBadRequest, "federation_disabled", "Federation is not enabled")
		return
	}
	var in struct {
		ID      string            `json:"id"`
		Name    string            `json:"name"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	}
	if err := jsonutil.ReadJSON(r, &in, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	if strings.TrimSpace(in.ID) == "" {
		id, err := uuid.NewString()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "id_generation_failed", err.Error())
			return
		}
		in.ID = id
	}
	if strings.TrimSpace(in.URL) == "" {
		writeError(w, http.StatusBadRequest, "invalid_subgraph", "url is required")
		return
	}
	sg := &federation.Subgraph{
		ID:      in.ID,
		Name:    in.Name,
		URL:     in.URL,
		Headers: in.Headers,
	}
	if err := mgr.AddSubgraph(sg); err != nil {
		writeError(w, http.StatusBadRequest, "add_subgraph_failed", err.Error())
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusCreated, sg)
}

func (s *Server) getSubgraph(w http.ResponseWriter, r *http.Request) {
	mgr := s.gateway.Subgraphs()
	if mgr == nil {
		writeError(w, http.StatusBadRequest, "federation_disabled", "Federation is not enabled")
		return
	}
	id := r.PathValue("id")
	sg, ok := mgr.GetSubgraph(id)
	if !ok {
		writeError(w, http.StatusNotFound, "subgraph_not_found", "Subgraph not found")
		return
	}
	_ = jsonutil.WriteJSON(w, http.StatusOK, sg)
}

func (s *Server) removeSubgraph(w http.ResponseWriter, r *http.Request) {
	mgr := s.gateway.Subgraphs()
	if mgr == nil {
		writeError(w, http.StatusBadRequest, "federation_disabled", "Federation is not enabled")
		return
	}
	id := r.PathValue("id")
	if _, ok := mgr.GetSubgraph(id); !ok {
		writeError(w, http.StatusNotFound, "subgraph_not_found", "Subgraph not found")
		return
	}
	mgr.RemoveSubgraph(id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) composeSubgraphs(w http.ResponseWriter, _ *http.Request) {
	mgr := s.gateway.Subgraphs()
	composer := s.gateway.FederationComposer()
	if mgr == nil || composer == nil {
		writeError(w, http.StatusBadRequest, "federation_disabled", "Federation is not enabled")
		return
	}
	subgraphs := mgr.ListSubgraphs()
	if len(subgraphs) == 0 {
		writeError(w, http.StatusBadRequest, "no_subgraphs", "No subgraphs registered")
		return
	}
	supergraph, err := composer.Compose(subgraphs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "compose_failed", err.Error())
		return
	}
	// Rebuild the planner with the newly composed schema.
	s.gateway.RebuildFederationPlanner()
	_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
		"composed": true,
		"types":    len(supergraph.Types),
		"sdl":      supergraph.SDL,
	})
}
