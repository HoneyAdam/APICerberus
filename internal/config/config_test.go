package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadAppliesDefaultsAndGeneratesIDs(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
gateway:
  http_addr: ":8080"
services:
  - name: "svc-users"
    upstream: "up-users"
routes:
  - name: "users-route"
    service: "svc-users"
    paths:
      - "/users"
upstreams:
  - name: "up-users"
    targets:
      - address: "127.0.0.1:9000"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if cfg.Gateway.ReadTimeout != 30*time.Second {
		t.Fatalf("expected default gateway read timeout 30s, got %v", cfg.Gateway.ReadTimeout)
	}
	if cfg.Admin.Addr != ":9876" {
		t.Fatalf("expected default admin addr :9876, got %q", cfg.Admin.Addr)
	}
	if cfg.Admin.UIPath != "/dashboard" {
		t.Fatalf("expected default admin ui_path /dashboard, got %q", cfg.Admin.UIPath)
	}
	if cfg.Logging.Level != "info" || cfg.Logging.Format != "json" || cfg.Logging.Output != "stdout" {
		t.Fatalf("unexpected logging defaults: %#v", cfg.Logging)
	}
	if cfg.Store.Path != "apicerberus.db" {
		t.Fatalf("expected default store path apicerberus.db, got %q", cfg.Store.Path)
	}
	if cfg.Store.BusyTimeout != 5*time.Second {
		t.Fatalf("expected default store busy timeout 5s, got %v", cfg.Store.BusyTimeout)
	}
	if cfg.Store.JournalMode != "WAL" {
		t.Fatalf("expected default store journal mode WAL, got %q", cfg.Store.JournalMode)
	}
	if !cfg.Store.ForeignKeys {
		t.Fatalf("expected default store foreign_keys=true")
	}
	if cfg.Billing.DefaultCost != 1 {
		t.Fatalf("expected default billing.default_cost=1, got %d", cfg.Billing.DefaultCost)
	}
	if cfg.Billing.ZeroBalanceAction != "reject" {
		t.Fatalf("expected default billing.zero_balance_action=reject, got %q", cfg.Billing.ZeroBalanceAction)
	}
	if !cfg.Billing.TestModeEnabled {
		t.Fatalf("expected default billing.test_mode_enabled=true")
	}

	if len(cfg.Services) != 1 || cfg.Services[0].ID == "" {
		t.Fatalf("service id should be generated")
	}
	if len(cfg.Routes) != 1 || cfg.Routes[0].ID == "" {
		t.Fatalf("route id should be generated")
	}
	if len(cfg.Upstreams) != 1 || cfg.Upstreams[0].ID == "" {
		t.Fatalf("upstream id should be generated")
	}
	if len(cfg.Upstreams[0].Targets) != 1 || cfg.Upstreams[0].Targets[0].ID == "" {
		t.Fatalf("upstream target id should be generated")
	}
	if cfg.Upstreams[0].Targets[0].Weight != 100 {
		t.Fatalf("expected default target weight 100, got %d", cfg.Upstreams[0].Targets[0].Weight)
	}
	if len(cfg.Routes[0].Methods) != 1 || cfg.Routes[0].Methods[0] != "GET" {
		t.Fatalf("expected default route method GET, got %#v", cfg.Routes[0].Methods)
	}
}

func TestLoadValidation(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
gateway:
  http_addr: ":8080"
services:
  - name: "svc-users"
    upstream: "missing-upstream"
routes:
  - name: "users-route"
    service: "missing-service"
    paths:
      - "/users"
upstreams:
  - name: "up-users"
    targets:
      - address: ""
`)

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "references unknown upstream") ||
		!strings.Contains(msg, "references unknown service") {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestLoadValidationHTTPSRequiresTLS(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
gateway:
  http_addr: ""
  https_addr: ":8443"
services:
  - name: "svc-users"
    upstream: "up-users"
routes:
  - name: "users-route"
    service: "svc-users"
    paths:
      - "/users"
upstreams:
  - name: "up-users"
    targets:
      - address: "127.0.0.1:9000"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected validation error for https without tls settings")
	}
	if !strings.Contains(err.Error(), "gateway.https_addr requires gateway.tls.auto=true or gateway.tls.cert_file+key_file") {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestLoadValidationHTTPSWithAutoTLS(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
gateway:
  http_addr: ""
  https_addr: ":8443"
  tls:
    auto: true
    acme_email: "admin@example.com"
    acme_dir: "acme-certs"
services:
  - name: "svc-users"
    upstream: "up-users"
routes:
  - name: "users-route"
    service: "svc-users"
    paths:
      - "/users"
upstreams:
  - name: "up-users"
    targets:
      - address: "127.0.0.1:9000"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.Gateway.HTTPSAddr != ":8443" {
		t.Fatalf("expected https_addr :8443, got %q", cfg.Gateway.HTTPSAddr)
	}
	if !cfg.Gateway.TLS.Auto {
		t.Fatalf("expected gateway.tls.auto=true")
	}
}

func TestEnvOverrides(t *testing.T) {
	path := writeTempConfig(t, `
gateway:
  http_addr: ":8080"
services:
  - name: "svc-users"
    protocol: "http"
    upstream: "up-users"
routes:
  - name: "users-route"
    service: "svc-users"
    paths:
      - "/users"
upstreams:
  - name: "up-users"
    targets:
      - address: "127.0.0.1:9000"
`)

	t.Setenv("APICERBERUS_GATEWAY_HTTP_ADDR", ":9090")
	t.Setenv("APICERBERUS_GATEWAY_READ_TIMEOUT", "45s")
	t.Setenv("APICERBERUS_LOGGING_LEVEL", "debug")
	t.Setenv("APICERBERUS_LOGGING_ROTATION_MAX_SIZE_MB", "12")
	t.Setenv("APICERBERUS_STORE_PATH", "custom.db")
	t.Setenv("APICERBERUS_STORE_BUSY_TIMEOUT", "7s")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if cfg.Gateway.HTTPAddr != ":9090" {
		t.Fatalf("gateway.http_addr env override failed: %q", cfg.Gateway.HTTPAddr)
	}
	if cfg.Gateway.ReadTimeout != 45*time.Second {
		t.Fatalf("gateway.read_timeout env override failed: %v", cfg.Gateway.ReadTimeout)
	}
	if cfg.Logging.Level != "debug" {
		t.Fatalf("logging.level env override failed: %q", cfg.Logging.Level)
	}
	if cfg.Logging.Rotation.MaxSizeMB != 12 {
		t.Fatalf("logging.rotation.max_size_mb env override failed: %d", cfg.Logging.Rotation.MaxSizeMB)
	}
	if cfg.Store.Path != "custom.db" {
		t.Fatalf("store.path env override failed: %q", cfg.Store.Path)
	}
	if cfg.Store.BusyTimeout != 7*time.Second {
		t.Fatalf("store.busy_timeout env override failed: %v", cfg.Store.BusyTimeout)
	}
}

func TestLoadConsumersSection(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
gateway:
  http_addr: ":8080"
services:
  - name: "svc-users"
    upstream: "up-users"
routes:
  - name: "users-route"
    service: "svc-users"
    paths:
      - "/users"
upstreams:
  - name: "up-users"
    targets:
      - address: "127.0.0.1:9000"
consumers:
  - name: "mobile-app"
    api_keys:
      - key: "ck_live_mobile_123"
    rate_limit:
      requests_per_second: 100
      burst: 150
    acl_groups:
      - "mobile-tier"
    metadata:
      app_name: "Mobile App"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if len(cfg.Consumers) != 1 {
		t.Fatalf("expected 1 consumer, got %d", len(cfg.Consumers))
	}
	c := cfg.Consumers[0]
	if c.Name != "mobile-app" {
		t.Fatalf("unexpected consumer name: %q", c.Name)
	}
	if c.ID == "" {
		t.Fatalf("expected generated consumer ID")
	}
	if len(c.APIKeys) != 1 || c.APIKeys[0].Key != "ck_live_mobile_123" {
		t.Fatalf("unexpected api_keys: %#v", c.APIKeys)
	}
	if c.APIKeys[0].ID == "" {
		t.Fatalf("expected generated api key ID")
	}
	if c.RateLimit.RequestsPerSecond != 100 || c.RateLimit.Burst != 150 {
		t.Fatalf("unexpected rate limit: %#v", c.RateLimit)
	}
	if len(c.ACLGroups) != 1 || c.ACLGroups[0] != "mobile-tier" {
		t.Fatalf("unexpected acl groups: %#v", c.ACLGroups)
	}
	if c.Metadata["app_name"] != "Mobile App" {
		t.Fatalf("unexpected metadata: %#v", c.Metadata)
	}
}

func TestLoadConsumerValidationDuplicateKey(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
gateway:
  http_addr: ":8080"
services:
  - name: "svc-users"
    upstream: "up-users"
routes:
  - name: "users-route"
    service: "svc-users"
    paths:
      - "/users"
upstreams:
  - name: "up-users"
    targets:
      - address: "127.0.0.1:9000"
consumers:
  - name: "a"
    api_keys:
      - key: "same-key"
  - name: "b"
    api_keys:
      - key: "same-key"
`)

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected duplicate api key validation error")
	}
	if !strings.Contains(err.Error(), "api key is duplicated across consumers") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadAuthAPIKeyNames(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
gateway:
  http_addr: ":8080"
services:
  - name: "svc-users"
    upstream: "up-users"
routes:
  - name: "users-route"
    service: "svc-users"
    paths:
      - "/users"
upstreams:
  - name: "up-users"
    targets:
      - address: "127.0.0.1:9000"
auth:
  api_key:
    key_names:
      - "X-App-Key"
      - " "
    query_names:
      - "token"
    cookie_names:
      - "session_key"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if len(cfg.Auth.APIKey.KeyNames) != 1 || cfg.Auth.APIKey.KeyNames[0] != "X-App-Key" {
		t.Fatalf("unexpected key_names: %#v", cfg.Auth.APIKey.KeyNames)
	}
	if len(cfg.Auth.APIKey.QueryNames) != 1 || cfg.Auth.APIKey.QueryNames[0] != "token" {
		t.Fatalf("unexpected query_names: %#v", cfg.Auth.APIKey.QueryNames)
	}
	if len(cfg.Auth.APIKey.CookieNames) != 1 || cfg.Auth.APIKey.CookieNames[0] != "session_key" {
		t.Fatalf("unexpected cookie_names: %#v", cfg.Auth.APIKey.CookieNames)
	}
}

func TestLoadPluginConfigs(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
gateway:
  http_addr: ":8080"
services:
  - name: "svc-users"
    upstream: "up-users"
routes:
  - name: "users-route"
    service: "svc-users"
    paths:
      - "/users"
    plugins:
      - name: "rate-limit"
        config:
          algorithm: "fixed_window"
          scope: "route"
          limit: 100
          window: "1s"
upstreams:
  - name: "up-users"
    targets:
      - address: "127.0.0.1:9000"
global_plugins:
  - name: "cors"
    config:
      allowed_origins:
        - "*"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(cfg.GlobalPlugins) != 1 || cfg.GlobalPlugins[0].Name != "cors" {
		t.Fatalf("unexpected global plugins: %#v", cfg.GlobalPlugins)
	}
	if len(cfg.Routes) != 1 || len(cfg.Routes[0].Plugins) != 1 {
		t.Fatalf("unexpected route plugins: %#v", cfg.Routes)
	}
	if cfg.Routes[0].Plugins[0].Name != "rate-limit" {
		t.Fatalf("unexpected route plugin name: %#v", cfg.Routes[0].Plugins[0])
	}
}

func TestLoadBillingConfig(t *testing.T) {
	t.Parallel()

	path := writeTempConfig(t, `
gateway:
  http_addr: ":8080"
services:
  - name: "svc-users"
    upstream: "up-users"
routes:
  - name: "users-route"
    service: "svc-users"
    paths:
      - "/users"
upstreams:
  - name: "up-users"
    targets:
      - address: "127.0.0.1:9000"
billing:
  enabled: true
  default_cost: 2
  route_costs:
    "users-route": 5
  method_multipliers:
    GET: 1.0
    POST: 2.0
  zero_balance_action: "allow_with_flag"
  test_mode_enabled: false
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if !cfg.Billing.Enabled {
		t.Fatalf("expected billing.enabled=true")
	}
	if cfg.Billing.DefaultCost != 2 {
		t.Fatalf("unexpected billing.default_cost: %d", cfg.Billing.DefaultCost)
	}
	if cfg.Billing.RouteCosts["users-route"] != 5 {
		t.Fatalf("unexpected billing.route_costs: %#v", cfg.Billing.RouteCosts)
	}
	if cfg.Billing.MethodMultipliers["POST"] != 2.0 {
		t.Fatalf("unexpected billing.method_multipliers: %#v", cfg.Billing.MethodMultipliers)
	}
	if cfg.Billing.ZeroBalanceAction != "allow_with_flag" {
		t.Fatalf("unexpected billing.zero_balance_action: %q", cfg.Billing.ZeroBalanceAction)
	}
	if cfg.Billing.TestModeEnabled {
		t.Fatalf("expected billing.test_mode_enabled=false to be preserved")
	}
}

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "apicerberus.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(body)+"\n"), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}
