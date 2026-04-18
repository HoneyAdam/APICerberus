package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/pkg/uuid"
	customyaml "github.com/APICerberus/APICerebrus/internal/pkg/yaml"
)

// Load reads, parses, normalizes and validates configuration from disk.
func Load(path string) (*Config, error) {
	// #nosec G304 -- path is the administrator-supplied config file path.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{}
	if err := customyaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	setDefaults(cfg)
	if err := applyEnvOverrides(cfg); err != nil {
		return nil, fmt.Errorf("apply env overrides: %w", err)
	}
	if err := generateIDs(cfg); err != nil {
		return nil, fmt.Errorf("generate ids: %w", err)
	}
	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

func setDefaults(cfg *Config) {
	if cfg == nil {
		return
	}

	if strings.TrimSpace(cfg.Gateway.HTTPAddr) == "" && strings.TrimSpace(cfg.Gateway.HTTPSAddr) == "" {
		cfg.Gateway.HTTPAddr = ":8080"
	}
	cfg.Gateway.HTTPSAddr = strings.TrimSpace(cfg.Gateway.HTTPSAddr)
	cfg.Gateway.TLS.ACMEEmail = strings.TrimSpace(cfg.Gateway.TLS.ACMEEmail)
	cfg.Gateway.TLS.ACMEDir = strings.TrimSpace(cfg.Gateway.TLS.ACMEDir)
	cfg.Gateway.TLS.CertFile = strings.TrimSpace(cfg.Gateway.TLS.CertFile)
	cfg.Gateway.TLS.KeyFile = strings.TrimSpace(cfg.Gateway.TLS.KeyFile)
	if cfg.Gateway.TLS.Auto && cfg.Gateway.TLS.ACMEDir == "" {
		cfg.Gateway.TLS.ACMEDir = "acme-certs"
	}
	if cfg.Gateway.ReadTimeout == 0 {
		cfg.Gateway.ReadTimeout = 30 * time.Second
	}
	if cfg.Gateway.WriteTimeout == 0 {
		cfg.Gateway.WriteTimeout = 30 * time.Second
	}
	if cfg.Gateway.IdleTimeout == 0 {
		cfg.Gateway.IdleTimeout = 120 * time.Second
	}
	if cfg.Gateway.MaxHeaderBytes == 0 {
		cfg.Gateway.MaxHeaderBytes = 1 << 20 // 1MB
	}
	if cfg.Gateway.MaxBodyBytes == 0 {
		cfg.Gateway.MaxBodyBytes = 10 << 20 // 10MB
	}

	if cfg.Admin.Addr == "" {
		cfg.Admin.Addr = ":9876"
	}
	if cfg.Admin.UIPath == "" {
		cfg.Admin.UIPath = "/dashboard"
	}
	if !cfg.Admin.UIEnabled {
		// Keep explicit false if user sets it; assume true only when no other admin settings are present.
		if cfg.Admin.APIKey == "" && cfg.Admin.Addr == ":9876" && cfg.Admin.UIPath == "/dashboard" {
			cfg.Admin.UIEnabled = true
		}
	}

	if cfg.Portal.Addr == "" {
		cfg.Portal.Addr = ":9877"
	}
	if cfg.Portal.PathPrefix == "" {
		cfg.Portal.PathPrefix = "/portal"
	}
	if cfg.Portal.Session.CookieName == "" {
		cfg.Portal.Session.CookieName = "apicerberus_session"
	}
	if cfg.Portal.Session.MaxAge <= 0 {
		cfg.Portal.Session.MaxAge = 24 * time.Hour
	}
	// Portal is disabled by default unless explicitly enabled.
	// This prevents unexpected port binding issues when users intend to keep it disabled.
	// If cfg.Portal.Enabled is false, the portal server is simply not started.

	// Cluster defaults
	if cfg.Cluster.Enabled {
		if cfg.Cluster.ElectionTimeoutMin == 0 {
			cfg.Cluster.ElectionTimeoutMin = 150 * time.Millisecond
		}
		if cfg.Cluster.ElectionTimeoutMax == 0 {
			cfg.Cluster.ElectionTimeoutMax = 300 * time.Millisecond
		}
		if cfg.Cluster.HeartbeatInterval == 0 {
			cfg.Cluster.HeartbeatInterval = 50 * time.Millisecond
		}
	}

	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}
	if cfg.Logging.Output == "" {
		cfg.Logging.Output = "stdout"
	}
	if cfg.Logging.Rotation.MaxSizeMB == 0 {
		cfg.Logging.Rotation.MaxSizeMB = 100
	}
	if cfg.Logging.Rotation.MaxBackups == 0 {
		cfg.Logging.Rotation.MaxBackups = 7
	}

	if strings.TrimSpace(cfg.Store.Path) == "" {
		cfg.Store.Path = "apicerberus.db"
	}
	if cfg.Store.BusyTimeout == 0 {
		cfg.Store.BusyTimeout = 5 * time.Second
	}
	if strings.TrimSpace(cfg.Store.JournalMode) == "" {
		cfg.Store.JournalMode = "WAL"
	}
	if !cfg.Store.ForeignKeys {
		// Preserve explicit false as much as possible; default to true when store section is omitted.
		if cfg.Store.Path == "apicerberus.db" && cfg.Store.BusyTimeout == 5*time.Second && strings.EqualFold(cfg.Store.JournalMode, "WAL") {
			cfg.Store.ForeignKeys = true
		}
	}

	if cfg.Billing.DefaultCost == 0 {
		cfg.Billing.DefaultCost = 1
	}
	if cfg.Billing.RouteCosts == nil {
		cfg.Billing.RouteCosts = map[string]int64{}
	}
	if cfg.Billing.MethodMultipliers == nil {
		cfg.Billing.MethodMultipliers = map[string]float64{}
	}
	cfg.Billing.ZeroBalanceAction = strings.ToLower(strings.TrimSpace(cfg.Billing.ZeroBalanceAction))
	if cfg.Billing.ZeroBalanceAction == "" {
		cfg.Billing.ZeroBalanceAction = "reject"
	}
	// TestModeEnabled must be explicitly set in config — never auto-enabled
	// based on heuristics, to prevent unintended credit bypass in production.

	if cfg.Audit.BufferSize <= 0 {
		cfg.Audit.BufferSize = 10_000
	}
	if cfg.Audit.BatchSize <= 0 {
		cfg.Audit.BatchSize = 100
	}
	if cfg.Audit.FlushInterval <= 0 {
		cfg.Audit.FlushInterval = time.Second
	}
	if cfg.Audit.RetentionDays <= 0 {
		cfg.Audit.RetentionDays = 30
	}
	if cfg.Audit.RouteRetentionDays == nil {
		cfg.Audit.RouteRetentionDays = map[string]int{}
	}
	normalizedRouteRetention := make(map[string]int, len(cfg.Audit.RouteRetentionDays))
	for key, days := range cfg.Audit.RouteRetentionDays {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		normalizedRouteRetention[trimmedKey] = days
	}
	cfg.Audit.RouteRetentionDays = normalizedRouteRetention
	if strings.TrimSpace(cfg.Audit.ArchiveDir) == "" {
		cfg.Audit.ArchiveDir = "audit-archive"
	}
	if cfg.Audit.CleanupInterval <= 0 {
		cfg.Audit.CleanupInterval = time.Hour
	}
	if cfg.Audit.CleanupBatchSize <= 0 {
		cfg.Audit.CleanupBatchSize = 1000
	}
	if cfg.Audit.MaxRequestBodyBytes <= 0 {
		cfg.Audit.MaxRequestBodyBytes = 64 << 10 // 64KB
	}
	if cfg.Audit.MaxResponseBodyBytes <= 0 {
		cfg.Audit.MaxResponseBodyBytes = 64 << 10 // 64KB
	}
	if strings.TrimSpace(cfg.Audit.MaskReplacement) == "" {
		cfg.Audit.MaskReplacement = "***REDACTED***"
	}
	cfg.Audit.MaskHeaders = normalizeNames(cfg.Audit.MaskHeaders)
	cfg.Audit.MaskBodyFields = normalizeNames(cfg.Audit.MaskBodyFields)

	for i := range cfg.Services {
		if cfg.Services[i].Protocol == "" {
			cfg.Services[i].Protocol = "http"
		}
		if cfg.Services[i].ConnectTimeout == 0 {
			cfg.Services[i].ConnectTimeout = 5 * time.Second
		}
		if cfg.Services[i].ReadTimeout == 0 {
			cfg.Services[i].ReadTimeout = 30 * time.Second
		}
		if cfg.Services[i].WriteTimeout == 0 {
			cfg.Services[i].WriteTimeout = 30 * time.Second
		}
	}

	for i := range cfg.Routes {
		if len(cfg.Routes[i].Methods) == 0 {
			cfg.Routes[i].Methods = []string{"GET"}
		}
		cfg.Routes[i].Plugins = normalizePluginConfigs(cfg.Routes[i].Plugins)
	}

	cfg.GlobalPlugins = normalizePluginConfigs(cfg.GlobalPlugins)

	for i := range cfg.Upstreams {
		if cfg.Upstreams[i].Algorithm == "" {
			cfg.Upstreams[i].Algorithm = "round_robin"
		}
		if cfg.Upstreams[i].HealthCheck.Active.Path == "" {
			cfg.Upstreams[i].HealthCheck.Active.Path = "/health"
		}
		if cfg.Upstreams[i].HealthCheck.Active.Interval == 0 {
			cfg.Upstreams[i].HealthCheck.Active.Interval = 10 * time.Second
		}
		if cfg.Upstreams[i].HealthCheck.Active.Timeout == 0 {
			cfg.Upstreams[i].HealthCheck.Active.Timeout = 2 * time.Second
		}
		if cfg.Upstreams[i].HealthCheck.Active.HealthyThreshold == 0 {
			cfg.Upstreams[i].HealthCheck.Active.HealthyThreshold = 2
		}
		if cfg.Upstreams[i].HealthCheck.Active.UnhealthyThreshold == 0 {
			cfg.Upstreams[i].HealthCheck.Active.UnhealthyThreshold = 3
		}

		for j := range cfg.Upstreams[i].Targets {
			if cfg.Upstreams[i].Targets[j].Weight == 0 {
				cfg.Upstreams[i].Targets[j].Weight = 100
			}
		}
	}

	for i := range cfg.Consumers {
		if cfg.Consumers[i].RateLimit.RequestsPerSecond < 0 {
			cfg.Consumers[i].RateLimit.RequestsPerSecond = 0
		}
		if cfg.Consumers[i].RateLimit.Burst < 0 {
			cfg.Consumers[i].RateLimit.Burst = 0
		}
	}

	cfg.Auth.APIKey.KeyNames = normalizeNames(cfg.Auth.APIKey.KeyNames)
	cfg.Auth.APIKey.QueryNames = normalizeNames(cfg.Auth.APIKey.QueryNames)
	cfg.Auth.APIKey.CookieNames = normalizeNames(cfg.Auth.APIKey.CookieNames)
}

func validate(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	errs := make([]string, 0)
	addErr := func(msg string) {
		errs = append(errs, msg)
	}

	if cfg.Gateway.HTTPAddr == "" && cfg.Gateway.HTTPSAddr == "" {
		addErr("gateway.http_addr or gateway.https_addr must be set")
	}
	certFile := strings.TrimSpace(cfg.Gateway.TLS.CertFile)
	keyFile := strings.TrimSpace(cfg.Gateway.TLS.KeyFile)
	if cfg.Gateway.HTTPSAddr != "" {
		if (certFile == "") != (keyFile == "") {
			addErr("gateway.tls.cert_file and gateway.tls.key_file must be provided together")
		}
		if certFile == "" && keyFile == "" && !cfg.Gateway.TLS.Auto {
			addErr("gateway.https_addr requires gateway.tls.auto=true or gateway.tls.cert_file+key_file")
		}
		if cfg.Gateway.TLS.Auto {
			if strings.TrimSpace(cfg.Gateway.TLS.ACMEEmail) == "" {
				addErr("gateway.tls.acme_email is required when gateway.tls.auto is true")
			}
			if strings.TrimSpace(cfg.Gateway.TLS.ACMEDir) == "" {
				addErr("gateway.tls.acme_dir is required when gateway.tls.auto is true")
			}
		}
	}
	if cfg.Gateway.ReadTimeout < 0 || cfg.Gateway.WriteTimeout < 0 || cfg.Gateway.IdleTimeout < 0 {
		addErr("gateway timeouts cannot be negative")
	}
	if cfg.Gateway.MaxHeaderBytes <= 0 {
		addErr("gateway.max_header_bytes must be greater than zero")
	}
	if cfg.Gateway.MaxBodyBytes <= 0 {
		addErr("gateway.max_body_bytes must be greater than zero")
	}

	apiKey := strings.TrimSpace(cfg.Admin.APIKey)
	if apiKey == "" {
		addErr("admin.api_key is required")
	} else {
		if len(apiKey) < 32 {
			addErr("admin.api_key must be at least 32 characters")
		}
		lowerKey := strings.ToLower(apiKey)
		if strings.Contains(lowerKey, "change") || strings.Contains(lowerKey, "secret") || strings.Contains(lowerKey, "password") || strings.Contains(lowerKey, "123") {
			addErr("admin.api_key appears to be a placeholder or weak value")
		}
	}
	tokenSecret := strings.TrimSpace(cfg.Admin.TokenSecret)
	if len(tokenSecret) < 32 {
		addErr("admin.token_secret must be at least 32 characters")
	}
	lowerTokenSecret := strings.ToLower(tokenSecret)
	if strings.Contains(lowerTokenSecret, "change") || strings.Contains(lowerTokenSecret, "secret") || strings.Contains(lowerTokenSecret, "password") {
		addErr("admin.token_secret appears to be a placeholder or weak value")
	}
	if !strings.HasPrefix(cfg.Admin.UIPath, "/") {
		addErr("admin.ui_path must start with '/'")
	}
	if !strings.HasPrefix(cfg.Portal.PathPrefix, "/") {
		addErr("portal.path_prefix must start with '/'")
	}
	secret := strings.TrimSpace(cfg.Portal.Session.Secret)
	// SECURITY: Always validate portal secret regardless of current enabled state.
	// A hot-reload enabling the portal with an empty/weak secret would be catastrophic.
	if len(secret) < 32 {
		addErr("portal.session.secret must be at least 32 characters")
	}
	lowerSecret := strings.ToLower(secret)
	if strings.Contains(lowerSecret, "change") || strings.Contains(lowerSecret, "secret") || strings.Contains(lowerSecret, "password") {
		addErr("portal.session.secret appears to be a placeholder value")
	}
	if cfg.Portal.Enabled {
		if cfg.Gateway.HTTPSAddr != "" && !cfg.Portal.Session.Secure {
			addErr("portal.session.secure must be true when gateway.https_addr is configured")
		}
	}
	if strings.TrimSpace(cfg.Portal.Session.CookieName) == "" {
		addErr("portal.session.cookie_name is required")
	}
	if cfg.Portal.Session.MaxAge <= 0 {
		addErr("portal.session.max_age must be greater than zero")
	}
	if cfg.Portal.Enabled && strings.TrimSpace(cfg.Portal.Addr) == "" {
		addErr("portal.addr is required when portal.enabled is true")
	}

	level := strings.ToLower(cfg.Logging.Level)
	if level != "debug" && level != "info" && level != "warn" && level != "error" {
		addErr("logging.level must be one of: debug, info, warn, error")
	}
	format := strings.ToLower(cfg.Logging.Format)
	if format != "json" && format != "text" {
		addErr("logging.format must be one of: json, text")
	}

	if strings.TrimSpace(cfg.Store.Path) == "" {
		addErr("store.path is required")
	}
	if cfg.Store.BusyTimeout < 0 {
		addErr("store.busy_timeout cannot be negative")
	}
	journalMode := strings.ToUpper(strings.TrimSpace(cfg.Store.JournalMode))
	switch journalMode {
	case "WAL", "DELETE", "TRUNCATE", "PERSIST", "MEMORY", "OFF":
	default:
		addErr("store.journal_mode must be one of: WAL, DELETE, TRUNCATE, PERSIST, MEMORY, OFF")
	}
	if cfg.Billing.DefaultCost < 0 {
		addErr("billing.default_cost cannot be negative")
	}
	for routeID, cost := range cfg.Billing.RouteCosts {
		if strings.TrimSpace(routeID) == "" {
			addErr("billing.route_costs keys cannot be empty")
			continue
		}
		if cost < 0 {
			addErr(fmt.Sprintf("billing.route_costs[%q] cannot be negative", routeID))
		}
	}
	for method, multiplier := range cfg.Billing.MethodMultipliers {
		if strings.TrimSpace(method) == "" {
			addErr("billing.method_multipliers keys cannot be empty")
			continue
		}
		if multiplier <= 0 {
			addErr(fmt.Sprintf("billing.method_multipliers[%q] must be greater than zero", method))
		}
	}
	switch cfg.Billing.ZeroBalanceAction {
	case "reject", "allow_with_flag":
	default:
		addErr("billing.zero_balance_action must be one of: reject, allow_with_flag")
	}
	// H-004 FIX: Reject test_mode_enabled to prevent credit bypass in production.
	if cfg.Billing.TestModeEnabled {
		addErr("billing.test_mode_enabled is not permitted — use test API keys (ck_test_*) in test environments instead")
	}
	if cfg.Audit.BufferSize <= 0 {
		addErr("audit.buffer_size must be greater than zero")
	}
	if cfg.Audit.BatchSize <= 0 {
		addErr("audit.batch_size must be greater than zero")
	}
	if cfg.Audit.FlushInterval <= 0 {
		addErr("audit.flush_interval must be greater than zero")
	}
	if cfg.Audit.RetentionDays <= 0 {
		addErr("audit.retention_days must be greater than zero")
	}
	for route, days := range cfg.Audit.RouteRetentionDays {
		if strings.TrimSpace(route) == "" {
			addErr("audit.route_retention_days keys cannot be empty")
			continue
		}
		if days <= 0 {
			addErr(fmt.Sprintf("audit.route_retention_days[%q] must be greater than zero", route))
		}
	}
	if strings.TrimSpace(cfg.Audit.ArchiveDir) == "" {
		addErr("audit.archive_dir is required")
	}
	if cfg.Audit.CleanupInterval <= 0 {
		addErr("audit.cleanup_interval must be greater than zero")
	}
	if cfg.Audit.CleanupBatchSize <= 0 {
		addErr("audit.cleanup_batch_size must be greater than zero")
	}
	if cfg.Audit.MaxRequestBodyBytes < 0 {
		addErr("audit.max_request_body_bytes cannot be negative")
	}
	if cfg.Audit.MaxResponseBodyBytes < 0 {
		addErr("audit.max_response_body_bytes cannot be negative")
	}
	// Kafka TLS: reject insecure skip-verify in production
	if cfg.Kafka.Enabled && cfg.Kafka.TLS.Enabled && cfg.Kafka.TLS.SkipVerify {
		addErr("kafka.tls.skip_verify is insecure and must not be used in production")
	}

	// SEC-RAFT-001: cluster.mtls.enabled was historically not wired through
	// run.go, so operators who toggled cluster.enabled: true on its own shipped
	// clusters where Raft RPCs travelled in cleartext and accepted unauthenticated
	// AppendEntries from anyone with L2/L3 reach to the Raft port — a direct
	// FSM-command injection path (credits, routes, certs). Full mTLS wiring is
	// tracked as a follow-up fix; until then, refuse to start when clustering
	// is enabled without explicitly opting into mTLS so the foot-gun cannot
	// fire by default.
	if cfg.Cluster.Enabled && !cfg.Cluster.MTLS.Enabled {
		addErr("cluster.enabled=true requires cluster.mtls.enabled=true (see SECURITY-REPORT.md CRIT-1)")
	}

	upstreamByName := make(map[string]struct{}, len(cfg.Upstreams))
	for i, up := range cfg.Upstreams {
		if up.Name == "" {
			addErr(fmt.Sprintf("upstreams[%d].name is required", i))
			continue
		}
		if _, exists := upstreamByName[up.Name]; exists {
			addErr(fmt.Sprintf("duplicate upstream name: %s", up.Name))
		}
		upstreamByName[up.Name] = struct{}{}

		if len(up.Targets) == 0 {
			addErr(fmt.Sprintf("upstream %q must include at least one target", up.Name))
		}
		for j, t := range up.Targets {
			if strings.TrimSpace(t.Address) == "" {
				addErr(fmt.Sprintf("upstream %q targets[%d].address is required", up.Name, j))
			}
			if t.Weight <= 0 {
				addErr(fmt.Sprintf("upstream %q targets[%d].weight must be greater than zero", up.Name, j))
			}
		}
		if up.HealthCheck.Active.Interval < 0 || up.HealthCheck.Active.Timeout < 0 {
			addErr(fmt.Sprintf("upstream %q health check interval/timeout cannot be negative", up.Name))
		}
		if up.HealthCheck.Active.HealthyThreshold <= 0 || up.HealthCheck.Active.UnhealthyThreshold <= 0 {
			addErr(fmt.Sprintf("upstream %q health check thresholds must be greater than zero", up.Name))
		}
	}

	serviceByName := make(map[string]struct{}, len(cfg.Services))
	for i, svc := range cfg.Services {
		if svc.Name == "" {
			addErr(fmt.Sprintf("services[%d].name is required", i))
			continue
		}
		if _, exists := serviceByName[svc.Name]; exists {
			addErr(fmt.Sprintf("duplicate service name: %s", svc.Name))
		}
		serviceByName[svc.Name] = struct{}{}

		protocol := strings.ToLower(strings.TrimSpace(svc.Protocol))
		switch protocol {
		case "http", "grpc", "graphql":
		default:
			addErr(fmt.Sprintf("service %q protocol must be one of: http, grpc, graphql", svc.Name))
		}
		if strings.TrimSpace(svc.Upstream) == "" {
			addErr(fmt.Sprintf("service %q upstream is required", svc.Name))
		} else if _, ok := upstreamByName[svc.Upstream]; !ok {
			addErr(fmt.Sprintf("service %q references unknown upstream %q", svc.Name, svc.Upstream))
		}
	}

	allowedMethods := map[string]struct{}{
		"*": {}, "GET": {}, "POST": {}, "PUT": {}, "PATCH": {}, "DELETE": {}, "HEAD": {}, "OPTIONS": {},
	}
	for i, rt := range cfg.Routes {
		if rt.Name == "" {
			addErr(fmt.Sprintf("routes[%d].name is required", i))
			continue
		}
		if strings.TrimSpace(rt.Service) == "" {
			addErr(fmt.Sprintf("route %q service is required", rt.Name))
		} else if _, ok := serviceByName[rt.Service]; !ok {
			addErr(fmt.Sprintf("route %q references unknown service %q", rt.Name, rt.Service))
		}
		if len(rt.Paths) == 0 {
			addErr(fmt.Sprintf("route %q must include at least one path", rt.Name))
		}
		for _, m := range rt.Methods {
			method := strings.ToUpper(strings.TrimSpace(m))
			if _, ok := allowedMethods[method]; !ok {
				addErr(fmt.Sprintf("route %q has invalid method %q", rt.Name, m))
			}
		}
		for j, p := range rt.Plugins {
			if strings.TrimSpace(p.Name) == "" {
				addErr(fmt.Sprintf("route %q plugins[%d].name is required", rt.Name, j))
			}
		}
	}

	for i, p := range cfg.GlobalPlugins {
		if strings.TrimSpace(p.Name) == "" {
			addErr(fmt.Sprintf("global_plugins[%d].name is required", i))
		}
	}

	consumerByName := make(map[string]struct{}, len(cfg.Consumers))
	apiKeyOwners := make(map[string]string)
	for i, consumer := range cfg.Consumers {
		if strings.TrimSpace(consumer.Name) == "" {
			addErr(fmt.Sprintf("consumers[%d].name is required", i))
			continue
		}
		if _, exists := consumerByName[consumer.Name]; exists {
			addErr(fmt.Sprintf("duplicate consumer name: %s", consumer.Name))
		}
		consumerByName[consumer.Name] = struct{}{}

		for j, key := range consumer.APIKeys {
			if strings.TrimSpace(key.Key) == "" {
				addErr(fmt.Sprintf("consumer %q api_keys[%d].key is required", consumer.Name, j))
				continue
			}
			// M-015: Enforce minimum key length for entropy. Weak keys (< 16 chars) can be
			// brute-forced. Require at least 32 chars for live keys, 16 for test keys.
			keyLen := len(key.Key)
			if strings.HasPrefix(key.Key, "ck_live_") && keyLen < 32 {
				addErr(fmt.Sprintf("consumer %q api_keys[%d].key is too short (live key requires >= 32 chars, got %d)", consumer.Name, j, keyLen))
			}
			if strings.HasPrefix(key.Key, "ck_test_") && keyLen < 16 {
				addErr(fmt.Sprintf("consumer %q api_keys[%d].key is too short (test key requires >= 16 chars, got %d)", consumer.Name, j, keyLen))
			}
			if owner, exists := apiKeyOwners[key.Key]; exists && owner != consumer.Name {
				addErr(fmt.Sprintf("api key is duplicated across consumers: %s", key.Key))
			}
			apiKeyOwners[key.Key] = consumer.Name
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func generateIDs(cfg *Config) error {
	for i := range cfg.Services {
		if strings.TrimSpace(cfg.Services[i].ID) == "" {
			id, err := uuid.NewString()
			if err != nil {
				return err
			}
			cfg.Services[i].ID = id
		}
	}
	for i := range cfg.Routes {
		if strings.TrimSpace(cfg.Routes[i].ID) == "" {
			id, err := uuid.NewString()
			if err != nil {
				return err
			}
			cfg.Routes[i].ID = id
		}
	}
	for i := range cfg.Upstreams {
		if strings.TrimSpace(cfg.Upstreams[i].ID) == "" {
			id, err := uuid.NewString()
			if err != nil {
				return err
			}
			cfg.Upstreams[i].ID = id
		}
		for j := range cfg.Upstreams[i].Targets {
			if strings.TrimSpace(cfg.Upstreams[i].Targets[j].ID) == "" {
				id, err := uuid.NewString()
				if err != nil {
					return err
				}
				cfg.Upstreams[i].Targets[j].ID = id
			}
		}
	}
	for i := range cfg.Consumers {
		if strings.TrimSpace(cfg.Consumers[i].ID) == "" {
			id, err := uuid.NewString()
			if err != nil {
				return err
			}
			cfg.Consumers[i].ID = id
		}
		for j := range cfg.Consumers[i].APIKeys {
			if strings.TrimSpace(cfg.Consumers[i].APIKeys[j].ID) == "" {
				id, err := uuid.NewString()
				if err != nil {
					return err
				}
				cfg.Consumers[i].APIKeys[j].ID = id
			}
		}
	}
	return nil
}

func normalizeNames(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizePluginConfigs(in []PluginConfig) []PluginConfig {
	if len(in) == 0 {
		return nil
	}
	out := make([]PluginConfig, 0, len(in))
	for _, plugin := range in {
		plugin.Name = strings.TrimSpace(plugin.Name)
		if plugin.Config == nil {
			plugin.Config = map[string]any{}
		}
		out = append(out, plugin)
	}
	return out
}
