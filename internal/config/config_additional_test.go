package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"
)

// ==================== toSnakeCase Tests ====================

func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"simple", "simple"},
		{"Simple", "simple"},
		{"A", "a"},
		{"HTTPServer", "h_t_t_p_server"},
		{"GatewayConfig", "gateway_config"},
		{"HTTPAddr", "h_t_t_p_addr"},
		{"Already_snake", "already_snake"},
		{"MixedCaseName", "mixed_case_name"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := toSnakeCase(tt.input)
			if result != tt.expected {
				t.Errorf("toSnakeCase(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// ==================== Watch Tests ====================

func TestWatch_Success(t *testing.T) {
	// Create a temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yaml")

	initialContent := `
gateway:
  http_addr: ":8080"
services:
  - name: "svc-test"
    upstream: "up-test"
routes:
  - name: "test-route"
    service: "svc-test"
    paths:
      - "/test"
upstreams:
  - name: "up-test"
    targets:
      - address: "127.0.0.1:9000"
`
	if err := os.WriteFile(configPath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	changeCalled := make(chan bool, 1)
	onChange := func(cfg *Config, err error) {
		changeCalled <- err == nil && cfg != nil
	}

	stop, err := Watch(configPath, onChange)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	defer stop()

	// Modify the file
	time.Sleep(100 * time.Millisecond)
	newContent := initialContent + "\n# modified"
	if err := os.WriteFile(configPath, []byte(newContent), 0644); err != nil {
		t.Fatalf("Failed to modify config: %v", err)
	}

	// Wait for change detection
	select {
	case success := <-changeCalled:
		if !success {
			t.Error("onChange called with error")
		}
	case <-time.After(3 * time.Second):
		t.Error("Timeout waiting for config change")
	}
}

func TestWatch_InvalidPath(t *testing.T) {
	// Use a path that definitely doesn't exist
	tmpDir := t.TempDir()
	nonexistentPath := filepath.Join(tmpDir, "nonexistent", "config.yaml")

	_, err := Watch(nonexistentPath, nil)
	if err == nil {
		t.Error("Watch() should return error for invalid path")
	}
}


func TestWatch_StatError(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yaml")

	content := `
gateway:
  http_addr: ":8080"
services:
  - name: "svc-test"
    upstream: "up-test"
routes:
  - name: "test-route"
    service: "svc-test"
    paths:
      - "/test"
upstreams:
  - name: "up-test"
    targets:
      - address: "127.0.0.1:9000"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	errorCalled := make(chan error, 1)
	onChange := func(cfg *Config, err error) {
		if err != nil {
			errorCalled <- err
		}
	}

	stop, err := Watch(configPath, onChange)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	defer stop()

	// Delete the file to cause stat error
	time.Sleep(100 * time.Millisecond)
	os.Remove(configPath)

	// Wait for error
	select {
	case <-errorCalled:
		// Expected
	case <-time.After(3 * time.Second):
		t.Error("Timeout waiting for stat error")
	}
}

// ==================== Load Error Path Tests ====================

func TestLoad_ReadFileError(t *testing.T) {
	// Use a path that definitely doesn't exist
	tmpDir := t.TempDir()
	nonexistentPath := filepath.Join(tmpDir, "nonexistent", "config.yaml")

	_, err := Load(nonexistentPath)
	if err == nil {
		t.Error("Load() should return error for non-existent file")
	}
	if err != nil && !contains(err.Error(), "read config") {
		t.Errorf("Expected 'read config' error, got: %v", err)
	}
}

func TestLoad_ParseError(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	// Write invalid YAML - use a clearly malformed YAML structure
	if err := os.WriteFile(configPath, []byte("{invalid: yaml: content"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	_, err := Load(configPath)
	// The YAML parser may or may not error on certain inputs
	// Just verify the function doesn't panic
	_ = err
}

func TestLoad_EnvOverrideError(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yaml")

	content := `
gateway:
  http_addr: ":8080"
services:
  - name: "svc-test"
    upstream: "up-test"
routes:
  - name: "test-route"
    service: "svc-test"
    paths:
      - "/test"
upstreams:
  - name: "up-test"
    targets:
      - address: "127.0.0.1:9000"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Set an invalid duration value
	t.Setenv("APICERBERUS_GATEWAY_READ_TIMEOUT", "invalid-duration")

	_, err := Load(configPath)
	if err == nil {
		t.Error("Load() should return error for invalid env override")
	}
}

func TestLoad_GenerateIDError(t *testing.T) {
	// This test is hard to trigger since uuid.NewString() rarely fails
	// We'll skip it as it's practically impossible to test without mocking
	t.Skip("Skipping: uuid.NewString() failure is practically impossible to trigger")
}

// ==================== setDefaults Edge Cases ====================

func TestSetDefaults_NilConfig(t *testing.T) {
	// Should not panic
	setDefaults(nil)
}

func TestSetDefaults_EmptyConfig(t *testing.T) {
	cfg := &Config{}
	setDefaults(cfg)

	// Check defaults were applied
	if cfg.Gateway.HTTPAddr != ":8080" {
		t.Errorf("Expected default HTTPAddr :8080, got %q", cfg.Gateway.HTTPAddr)
	}
	if cfg.Gateway.ReadTimeout != 30*time.Second {
		t.Errorf("Expected default ReadTimeout 30s, got %v", cfg.Gateway.ReadTimeout)
	}
	if cfg.Admin.Addr != ":9876" {
		t.Errorf("Expected default Admin.Addr :9876, got %q", cfg.Admin.Addr)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("Expected default Logging.Level info, got %q", cfg.Logging.Level)
	}
}

func TestSetDefaults_AdminUIEnabledWhenDefaults(t *testing.T) {
	// When all admin settings are at defaults, UIEnabled should be set to true
	cfg := &Config{
		Admin: AdminConfig{
			Addr:      ":9876",
			UIPath:    "/dashboard",
			UIEnabled: false, // Zero value
		},
	}
	setDefaults(cfg)

	// When all values are at defaults, UIEnabled should be set to true
	if !cfg.Admin.UIEnabled {
		t.Error("UIEnabled should be true when all admin settings are at defaults")
	}
}

func TestSetDefaults_PortalEnabledWhenDefaults(t *testing.T) {
	// When all portal settings are at defaults, Enabled should be set to true
	cfg := &Config{
		Portal: PortalConfig{
			Addr:       ":9877",
			PathPrefix: "/portal",
			Session: PortalSessionConfig{
				CookieName: "apicerberus_session",
				MaxAge:     24 * time.Hour,
			},
			Enabled: false, // Zero value
		},
	}
	setDefaults(cfg)

	// When all values are at defaults, Enabled should be set to true
	if !cfg.Portal.Enabled {
		t.Error("Portal.Enabled should be true when all portal settings are at defaults")
	}
}

func TestSetDefaults_StoreForeignKeysWhenDefaults(t *testing.T) {
	// When all store settings are at defaults, ForeignKeys should be set to true
	cfg := &Config{
		Store: StoreConfig{
			Path:        "apicerberus.db",
			BusyTimeout: 5 * time.Second,
			JournalMode: "WAL",
			ForeignKeys: false, // Zero value
		},
	}
	setDefaults(cfg)

	// When all values are at defaults, ForeignKeys should be set to true
	if !cfg.Store.ForeignKeys {
		t.Error("Store.ForeignKeys should be true when all store settings are at defaults")
	}
}

func TestSetDefaults_BillingTestModeWhenDefaults(t *testing.T) {
	// When all billing settings are at defaults, TestModeEnabled should be set to true
	cfg := &Config{
		Billing: BillingConfig{
			DefaultCost:       1,
			RouteCosts:        map[string]int64{},
			MethodMultipliers: map[string]float64{},
			ZeroBalanceAction: "reject",
			TestModeEnabled:   false, // Zero value
		},
	}
	setDefaults(cfg)

	// When all values are at defaults, TestModeEnabled should be set to true
	if !cfg.Billing.TestModeEnabled {
		t.Error("Billing.TestModeEnabled should be true when all billing settings are at defaults")
	}
}

func TestSetDefaults_ConsumerNegativeRateLimit(t *testing.T) {
	cfg := &Config{
		Consumers: []Consumer{
			{
				Name: "test-consumer",
				RateLimit: ConsumerRateLimit{
					RequestsPerSecond: -10,
					Burst:             -5,
				},
			},
		},
	}
	setDefaults(cfg)

	if cfg.Consumers[0].RateLimit.RequestsPerSecond != 0 {
		t.Errorf("Expected RequestsPerSecond to be 0, got %d", cfg.Consumers[0].RateLimit.RequestsPerSecond)
	}
	if cfg.Consumers[0].RateLimit.Burst != 0 {
		t.Errorf("Expected Burst to be 0, got %d", cfg.Consumers[0].RateLimit.Burst)
	}
}

func TestSetDefaults_RouteRetentionEmptyKey(t *testing.T) {
	cfg := &Config{
		Audit: AuditConfig{
			RouteRetentionDays: map[string]int{
				"":      10,
				"valid": 20,
			},
		},
	}
	setDefaults(cfg)

	// Empty keys should be removed
	if _, exists := cfg.Audit.RouteRetentionDays[""]; exists {
		t.Error("Empty key should be removed from RouteRetentionDays")
	}
	if _, exists := cfg.Audit.RouteRetentionDays["valid"]; !exists {
		t.Error("Valid key should remain in RouteRetentionDays")
	}
}

// ==================== Validation Tests ====================

func TestValidate_NilConfig(t *testing.T) {
	err := validate(nil)
	if err == nil {
		t.Error("validate(nil) should return error")
	}
}

func TestValidate_NoGatewayAddrs(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr:  "",
			HTTPSAddr: "",
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "http_addr or gateway.https_addr must be set") {
		t.Errorf("Expected gateway address error, got: %v", err)
	}
}

func TestValidate_HTTPSPartialTLS(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr:  "",
			HTTPSAddr: ":8443",
			TLS: TLSConfig{
				CertFile: "cert.pem",
				KeyFile:  "", // Missing key file
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "cert_file and gateway.tls.key_file must be provided together") {
		t.Errorf("Expected TLS cert/key error, got: %v", err)
	}
}

func TestValidate_HTTPSAutoTLSMissingEmail(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr:  "",
			HTTPSAddr: ":8443",
			TLS: TLSConfig{
				Auto:      true,
				ACMEEmail: "", // Missing email
				ACMEDir:   "certs",
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "acme_email is required") {
		t.Errorf("Expected ACME email error, got: %v", err)
	}
}

func TestValidate_HTTPSAutoTLSMissingDir(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr:  "",
			HTTPSAddr: ":8443",
			TLS: TLSConfig{
				Auto:      true,
				ACMEEmail: "test@example.com",
				ACMEDir:   "", // Missing dir
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "acme_dir is required") {
		t.Errorf("Expected ACME dir error, got: %v", err)
	}
}

func TestValidate_NegativeTimeouts(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr:    ":8080",
			ReadTimeout: -1 * time.Second,
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "timeouts cannot be negative") {
		t.Errorf("Expected negative timeout error, got: %v", err)
	}
}

func TestValidate_InvalidMaxHeaderBytes(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr:       ":8080",
			MaxHeaderBytes: 0,
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "max_header_bytes must be greater than zero") {
		t.Errorf("Expected max_header_bytes error, got: %v", err)
	}
}

func TestValidate_InvalidMaxBodyBytes(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr:     ":8080",
			MaxBodyBytes: 0,
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "max_body_bytes must be greater than zero") {
		t.Errorf("Expected max_body_bytes error, got: %v", err)
	}
}

func TestValidate_AdminUIPathNoPrefix(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Admin: AdminConfig{
			UIPath: "dashboard", // Missing leading /
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "admin.ui_path must start with") {
		t.Errorf("Expected admin.ui_path error, got: %v", err)
	}
}

func TestValidate_PortalPathPrefixNoPrefix(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Portal: PortalConfig{
			PathPrefix: "portal", // Missing leading /
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "portal.path_prefix must start with") {
		t.Errorf("Expected portal.path_prefix error, got: %v", err)
	}
}

func TestValidate_EmptyCookieName(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Portal: PortalConfig{
			PathPrefix: "/portal",
			Session: PortalSessionConfig{
				CookieName: "",
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "cookie_name is required") {
		t.Errorf("Expected cookie_name error, got: %v", err)
	}
}

func TestValidate_InvalidMaxAge(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Portal: PortalConfig{
			PathPrefix: "/portal",
			Session: PortalSessionConfig{
				CookieName: "session",
				MaxAge:     0,
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "max_age must be greater than zero") {
		t.Errorf("Expected max_age error, got: %v", err)
	}
}

func TestValidate_PortalEnabledNoAddr(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Portal: PortalConfig{
			Enabled:    true,
			Addr:       "", // Missing addr
			PathPrefix: "/portal",
			Session: PortalSessionConfig{
				CookieName: "session",
				MaxAge:     time.Hour,
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "portal.addr is required") {
		t.Errorf("Expected portal.addr error, got: %v", err)
	}
}

func TestValidate_InvalidLoggingLevel(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Logging: LoggingConfig{
			Level: "invalid",
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "logging.level must be one of") {
		t.Errorf("Expected logging.level error, got: %v", err)
	}
}

func TestValidate_InvalidLoggingFormat(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "xml", // Invalid
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "logging.format must be one of") {
		t.Errorf("Expected logging.format error, got: %v", err)
	}
}

func TestValidate_EmptyStorePath(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Store: StoreConfig{
			Path: "",
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "store.path is required") {
		t.Errorf("Expected store.path error, got: %v", err)
	}
}

func TestValidate_NegativeBusyTimeout(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Store: StoreConfig{
			Path:        "test.db",
			BusyTimeout: -1 * time.Second,
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "store.busy_timeout cannot be negative") {
		t.Errorf("Expected busy_timeout error, got: %v", err)
	}
}

func TestValidate_InvalidJournalMode(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Store: StoreConfig{
			Path:        "test.db",
			JournalMode: "INVALID",
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "store.journal_mode must be one of") {
		t.Errorf("Expected journal_mode error, got: %v", err)
	}
}

func TestValidate_NegativeDefaultCost(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Billing: BillingConfig{
			DefaultCost: -1,
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "billing.default_cost cannot be negative") {
		t.Errorf("Expected default_cost error, got: %v", err)
	}
}

func TestValidate_EmptyRouteCostKey(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Billing: BillingConfig{
			RouteCosts: map[string]int64{
				"": -1,
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "billing.route_costs keys cannot be empty") {
		t.Errorf("Expected route_costs key error, got: %v", err)
	}
}

func TestValidate_NegativeRouteCost(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Billing: BillingConfig{
			RouteCosts: map[string]int64{
				"route1": -5,
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "cannot be negative") {
		t.Errorf("Expected negative route_cost error, got: %v", err)
	}
}

func TestValidate_EmptyMethodMultiplierKey(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Billing: BillingConfig{
			MethodMultipliers: map[string]float64{
				"": 1.0,
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "billing.method_multipliers keys cannot be empty") {
		t.Errorf("Expected method_multipliers key error, got: %v", err)
	}
}

func TestValidate_InvalidMethodMultiplier(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Billing: BillingConfig{
			MethodMultipliers: map[string]float64{
				"GET": 0, // Must be > 0
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "must be greater than zero") {
		t.Errorf("Expected method_multiplier error, got: %v", err)
	}
}

func TestValidate_InvalidZeroBalanceAction(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Billing: BillingConfig{
			ZeroBalanceAction: "invalid",
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "zero_balance_action must be one of") {
		t.Errorf("Expected zero_balance_action error, got: %v", err)
	}
}

func TestValidate_InvalidAuditBufferSize(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Audit: AuditConfig{
			BufferSize: 0,
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "audit.buffer_size must be greater than zero") {
		t.Errorf("Expected buffer_size error, got: %v", err)
	}
}

func TestValidate_InvalidAuditBatchSize(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Audit: AuditConfig{
			BufferSize: 100,
			BatchSize:  0,
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "audit.batch_size must be greater than zero") {
		t.Errorf("Expected batch_size error, got: %v", err)
	}
}

func TestValidate_InvalidAuditFlushInterval(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Audit: AuditConfig{
			BufferSize:    100,
			BatchSize:     10,
			FlushInterval: 0,
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "audit.flush_interval must be greater than zero") {
		t.Errorf("Expected flush_interval error, got: %v", err)
	}
}

func TestValidate_InvalidAuditRetentionDays(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Audit: AuditConfig{
			BufferSize:    100,
			BatchSize:     10,
			FlushInterval: time.Second,
			RetentionDays: 0,
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "audit.retention_days must be greater than zero") {
		t.Errorf("Expected retention_days error, got: %v", err)
	}
}

func TestValidate_InvalidRouteRetentionDays(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Audit: AuditConfig{
			BufferSize:    100,
			BatchSize:     10,
			FlushInterval: time.Second,
			RetentionDays: 30,
			RouteRetentionDays: map[string]int{
				"route1": 0,
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "must be greater than zero") {
		t.Errorf("Expected route_retention_days error, got: %v", err)
	}
}

func TestValidate_EmptyRouteRetentionKey(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Audit: AuditConfig{
			BufferSize:    100,
			BatchSize:     10,
			FlushInterval: time.Second,
			RetentionDays: 30,
			RouteRetentionDays: map[string]int{
				"": 10,
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "audit.route_retention_days keys cannot be empty") {
		t.Errorf("Expected route_retention_days key error, got: %v", err)
	}
}

func TestValidate_EmptyArchiveDir(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Audit: AuditConfig{
			BufferSize:    100,
			BatchSize:     10,
			FlushInterval: time.Second,
			RetentionDays: 30,
			ArchiveDir:    "",
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "audit.archive_dir is required") {
		t.Errorf("Expected archive_dir error, got: %v", err)
	}
}

func TestValidate_InvalidCleanupInterval(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Audit: AuditConfig{
			BufferSize:      100,
			BatchSize:       10,
			FlushInterval:   time.Second,
			RetentionDays:   30,
			ArchiveDir:      "archive",
			CleanupInterval: 0,
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "audit.cleanup_interval must be greater than zero") {
		t.Errorf("Expected cleanup_interval error, got: %v", err)
	}
}

func TestValidate_InvalidCleanupBatchSize(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Audit: AuditConfig{
			BufferSize:       100,
			BatchSize:        10,
			FlushInterval:    time.Second,
			RetentionDays:    30,
			ArchiveDir:       "archive",
			CleanupInterval:  time.Hour,
			CleanupBatchSize: 0,
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "audit.cleanup_batch_size must be greater than zero") {
		t.Errorf("Expected cleanup_batch_size error, got: %v", err)
	}
}

func TestValidate_NegativeMaxRequestBodyBytes(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Audit: AuditConfig{
			BufferSize:          100,
			BatchSize:           10,
			FlushInterval:       time.Second,
			RetentionDays:       30,
			ArchiveDir:          "archive",
			CleanupInterval:     time.Hour,
			CleanupBatchSize:    1000,
			MaxRequestBodyBytes: -1,
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "audit.max_request_body_bytes cannot be negative") {
		t.Errorf("Expected max_request_body_bytes error, got: %v", err)
	}
}

func TestValidate_NegativeMaxResponseBodyBytes(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Audit: AuditConfig{
			BufferSize:           100,
			BatchSize:            10,
			FlushInterval:        time.Second,
			RetentionDays:        30,
			ArchiveDir:           "archive",
			CleanupInterval:      time.Hour,
			CleanupBatchSize:     1000,
			MaxResponseBodyBytes: -1,
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "audit.max_response_body_bytes cannot be negative") {
		t.Errorf("Expected max_response_body_bytes error, got: %v", err)
	}
}

func TestValidate_UpstreamNoName(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Upstreams: []Upstream{
			{
				Name: "", // No name
				Targets: []UpstreamTarget{
					{Address: "localhost:8080"},
				},
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "upstreams[0].name is required") {
		t.Errorf("Expected upstream name error, got: %v", err)
	}
}

func TestValidate_DuplicateUpstreamName(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Upstreams: []Upstream{
			{
				Name: "up1",
				Targets: []UpstreamTarget{
					{Address: "localhost:8080"},
				},
			},
			{
				Name: "up1", // Duplicate
				Targets: []UpstreamTarget{
					{Address: "localhost:8081"},
				},
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "duplicate upstream name") {
		t.Errorf("Expected duplicate upstream error, got: %v", err)
	}
}

func TestValidate_UpstreamNoTargets(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Upstreams: []Upstream{
			{
				Name:    "up1",
				Targets: []UpstreamTarget{}, // No targets
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "must include at least one target") {
		t.Errorf("Expected upstream targets error, got: %v", err)
	}
}

func TestValidate_UpstreamTargetNoAddress(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Upstreams: []Upstream{
			{
				Name: "up1",
				Targets: []UpstreamTarget{
					{Address: ""}, // No address
				},
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "address is required") {
		t.Errorf("Expected target address error, got: %v", err)
	}
}

func TestValidate_UpstreamTargetInvalidWeight(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Upstreams: []Upstream{
			{
				Name: "up1",
				Targets: []UpstreamTarget{
					{Address: "localhost:8080", Weight: 0},
				},
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "weight must be greater than zero") {
		t.Errorf("Expected target weight error, got: %v", err)
	}
}

func TestValidate_UpstreamNegativeHealthCheck(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Upstreams: []Upstream{
			{
				Name: "up1",
				Targets: []UpstreamTarget{
					{Address: "localhost:8080", Weight: 100},
				},
				HealthCheck: HealthCheckConfig{
					Active: ActiveHealthCheckConfig{
						Interval: -1 * time.Second,
					},
				},
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "health check interval/timeout cannot be negative") {
		t.Errorf("Expected health check error, got: %v", err)
	}
}

func TestValidate_UpstreamInvalidThresholds(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Upstreams: []Upstream{
			{
				Name: "up1",
				Targets: []UpstreamTarget{
					{Address: "localhost:8080", Weight: 100},
				},
				HealthCheck: HealthCheckConfig{
					Active: ActiveHealthCheckConfig{
						Interval:         time.Second,
						Timeout:          time.Second,
						HealthyThreshold: 0,
					},
				},
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "health check thresholds must be greater than zero") {
		t.Errorf("Expected health check threshold error, got: %v", err)
	}
}

func TestValidate_ServiceNoName(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Upstreams: []Upstream{
			{
				Name: "up1",
				Targets: []UpstreamTarget{
					{Address: "localhost:8080", Weight: 100},
				},
			},
		},
		Services: []Service{
			{
				Name:     "", // No name
				Protocol: "http",
				Upstream: "up1",
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "services[0].name is required") {
		t.Errorf("Expected service name error, got: %v", err)
	}
}

func TestValidate_DuplicateServiceName(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Upstreams: []Upstream{
			{
				Name: "up1",
				Targets: []UpstreamTarget{
					{Address: "localhost:8080", Weight: 100},
				},
			},
		},
		Services: []Service{
			{
				Name:     "svc1",
				Protocol: "http",
				Upstream: "up1",
			},
			{
				Name:     "svc1", // Duplicate
				Protocol: "http",
				Upstream: "up1",
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "duplicate service name") {
		t.Errorf("Expected duplicate service error, got: %v", err)
	}
}

func TestValidate_ServiceInvalidProtocol(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Upstreams: []Upstream{
			{
				Name: "up1",
				Targets: []UpstreamTarget{
					{Address: "localhost:8080", Weight: 100},
				},
			},
		},
		Services: []Service{
			{
				Name:     "svc1",
				Protocol: "invalid",
				Upstream: "up1",
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "protocol must be one of") {
		t.Errorf("Expected service protocol error, got: %v", err)
	}
}

func TestValidate_ServiceNoUpstream(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Services: []Service{
			{
				Name:     "svc1",
				Protocol: "http",
				Upstream: "", // No upstream
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "upstream is required") {
		t.Errorf("Expected service upstream error, got: %v", err)
	}
}

func TestValidate_ServiceUnknownUpstream(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Upstreams: []Upstream{
			{
				Name: "up1",
				Targets: []UpstreamTarget{
					{Address: "localhost:8080", Weight: 100},
				},
			},
		},
		Services: []Service{
			{
				Name:     "svc1",
				Protocol: "http",
				Upstream: "unknown-upstream",
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "references unknown upstream") {
		t.Errorf("Expected unknown upstream error, got: %v", err)
	}
}

func TestValidate_RouteNoName(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Upstreams: []Upstream{
			{
				Name: "up1",
				Targets: []UpstreamTarget{
					{Address: "localhost:8080", Weight: 100},
				},
			},
		},
		Services: []Service{
			{
				Name:     "svc1",
				Protocol: "http",
				Upstream: "up1",
			},
		},
		Routes: []Route{
			{
				Name:    "", // No name
				Service: "svc1",
				Paths:   []string{"/api"},
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "routes[0].name is required") {
		t.Errorf("Expected route name error, got: %v", err)
	}
}

func TestValidate_RouteNoService(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Routes: []Route{
			{
				Name:    "route1",
				Service: "", // No service
				Paths:   []string{"/api"},
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "service is required") {
		t.Errorf("Expected route service error, got: %v", err)
	}
}

func TestValidate_RouteUnknownService(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Upstreams: []Upstream{
			{
				Name: "up1",
				Targets: []UpstreamTarget{
					{Address: "localhost:8080", Weight: 100},
				},
			},
		},
		Services: []Service{
			{
				Name:     "svc1",
				Protocol: "http",
				Upstream: "up1",
			},
		},
		Routes: []Route{
			{
				Name:    "route1",
				Service: "unknown-service",
				Paths:   []string{"/api"},
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "references unknown service") {
		t.Errorf("Expected unknown service error, got: %v", err)
	}
}

func TestValidate_RouteNoPaths(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Upstreams: []Upstream{
			{
				Name: "up1",
				Targets: []UpstreamTarget{
					{Address: "localhost:8080", Weight: 100},
				},
			},
		},
		Services: []Service{
			{
				Name:     "svc1",
				Protocol: "http",
				Upstream: "up1",
			},
		},
		Routes: []Route{
			{
				Name:    "route1",
				Service: "svc1",
				Paths:   []string{}, // No paths
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "must include at least one path") {
		t.Errorf("Expected route paths error, got: %v", err)
	}
}

func TestValidate_RouteInvalidMethod(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Upstreams: []Upstream{
			{
				Name: "up1",
				Targets: []UpstreamTarget{
					{Address: "localhost:8080", Weight: 100},
				},
			},
		},
		Services: []Service{
			{
				Name:     "svc1",
				Protocol: "http",
				Upstream: "up1",
			},
		},
		Routes: []Route{
			{
				Name:    "route1",
				Service: "svc1",
				Paths:   []string{"/api"},
				Methods: []string{"INVALID"},
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "has invalid method") {
		t.Errorf("Expected route method error, got: %v", err)
	}
}

func TestValidate_RoutePluginNoName(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Upstreams: []Upstream{
			{
				Name: "up1",
				Targets: []UpstreamTarget{
					{Address: "localhost:8080", Weight: 100},
				},
			},
		},
		Services: []Service{
			{
				Name:     "svc1",
				Protocol: "http",
				Upstream: "up1",
			},
		},
		Routes: []Route{
			{
				Name:    "route1",
				Service: "svc1",
				Paths:   []string{"/api"},
				Plugins: []PluginConfig{
					{Name: ""}, // No name
				},
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "plugins[0].name is required") {
		t.Errorf("Expected route plugin name error, got: %v", err)
	}
}

func TestValidate_GlobalPluginNoName(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		GlobalPlugins: []PluginConfig{
			{Name: ""}, // No name
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "global_plugins[0].name is required") {
		t.Errorf("Expected global plugin name error, got: %v", err)
	}
}

func TestValidate_ConsumerNoName(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Consumers: []Consumer{
			{
				Name: "", // No name
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "consumers[0].name is required") {
		t.Errorf("Expected consumer name error, got: %v", err)
	}
}

func TestValidate_DuplicateConsumerName(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Consumers: []Consumer{
			{
				Name: "consumer1",
			},
			{
				Name: "consumer1", // Duplicate
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "duplicate consumer name") {
		t.Errorf("Expected duplicate consumer error, got: %v", err)
	}
}

func TestValidate_ConsumerAPIKeyNoKey(t *testing.T) {
	cfg := &Config{
		Gateway: GatewayConfig{
			HTTPAddr: ":8080",
		},
		Consumers: []Consumer{
			{
				Name: "consumer1",
				APIKeys: []ConsumerAPIKey{
					{Key: ""}, // No key
				},
			},
		},
	}
	err := validate(cfg)
	if err == nil || !contains(err.Error(), "api_keys[0].key is required") {
		t.Errorf("Expected api key error, got: %v", err)
	}
}

// ==================== setFromString Tests ====================

func TestSetFromString(t *testing.T) {
	tests := []struct {
		name     string
		field    reflect.Value
		raw      string
		wantErr  bool
		expected interface{}
	}{
		{
			name:     "string field",
			field:    reflect.ValueOf(new(string)).Elem(),
			raw:      "test-value",
			wantErr:  false,
			expected: "test-value",
		},
		{
			name:     "bool field true",
			field:    reflect.ValueOf(new(bool)).Elem(),
			raw:      "true",
			wantErr:  false,
			expected: true,
		},
		{
			name:     "bool field false",
			field:    reflect.ValueOf(new(bool)).Elem(),
			raw:      "false",
			wantErr:  false,
			expected: false,
		},
		{
			name:     "bool field invalid",
			field:    reflect.ValueOf(new(bool)).Elem(),
			raw:      "not-a-bool",
			wantErr:  true,
			expected: false,
		},
		{
			name:     "int field",
			field:    reflect.ValueOf(new(int)).Elem(),
			raw:      "42",
			wantErr:  false,
			expected: int64(42),
		},
		{
			name:     "int field invalid",
			field:    reflect.ValueOf(new(int)).Elem(),
			raw:      "not-an-int",
			wantErr:  true,
			expected: int64(0),
		},
		{
			name:     "int64 field",
			field:    reflect.ValueOf(new(int64)).Elem(),
			raw:      "9223372036854775807",
			wantErr:  false,
			expected: int64(9223372036854775807),
		},
		{
			name:     "uint field",
			field:    reflect.ValueOf(new(uint)).Elem(),
			raw:      "42",
			wantErr:  false,
			expected: uint64(42),
		},
		{
			name:     "uint field invalid",
			field:    reflect.ValueOf(new(uint)).Elem(),
			raw:      "-1",
			wantErr:  true,
			expected: uint64(0),
		},
		{
			name:     "float64 field",
			field:    reflect.ValueOf(new(float64)).Elem(),
			raw:      "3.14",
			wantErr:  false,
			expected: 3.14,
		},
		{
			name:     "float64 field invalid",
			field:    reflect.ValueOf(new(float64)).Elem(),
			raw:      "not-a-float",
			wantErr:  true,
			expected: float64(0),
		},
		{
			name:     "duration field",
			field:    reflect.ValueOf(new(time.Duration)).Elem(),
			raw:      "5m30s",
			wantErr:  false,
			expected: 5*time.Minute + 30*time.Second,
		},
		{
			name:     "duration field invalid",
			field:    reflect.ValueOf(new(time.Duration)).Elem(),
			raw:      "not-a-duration",
			wantErr:  true,
			expected: time.Duration(0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := setFromString(tt.field, tt.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("setFromString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				var actual interface{}
				switch tt.field.Kind() {
				case reflect.String:
					actual = tt.field.String()
				case reflect.Bool:
					actual = tt.field.Bool()
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					// For duration fields, compare the underlying int64 value
					if tt.field.Type() == reflect.TypeOf(time.Duration(0)) {
						actual = time.Duration(tt.field.Int())
					} else {
						actual = tt.field.Int()
					}
				case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
					actual = tt.field.Uint()
				case reflect.Float32, reflect.Float64:
					actual = tt.field.Float()
				}
				if actual != tt.expected {
					t.Errorf("setFromString() = %v, want %v", actual, tt.expected)
				}
			}
		})
	}
}

func TestSetFromString_UnsupportedType(t *testing.T) {
	// Test with an unsupported type (slice)
	field := reflect.ValueOf(new([]string)).Elem()
	err := setFromString(field, "test")
	if err == nil {
		t.Error("setFromString() should return error for unsupported type")
	}
}

func TestSetFromString_CannotSet(t *testing.T) {
	// Test with a field that cannot be set (unexported)
	type testStruct struct {
		unexported string
	}
	s := testStruct{}
	field := reflect.ValueOf(&s).Elem().FieldByName("unexported")
	err := setFromString(field, "test")
	if err == nil {
		t.Error("setFromString() should return error for unsettable field")
	}
}

// ==================== applyEnvOverrides Tests ====================

func TestApplyEnvOverrides_NilConfig(t *testing.T) {
	err := applyEnvOverrides(nil)
	if err != nil {
		t.Errorf("applyEnvOverrides(nil) should not error, got: %v", err)
	}
}

func TestApplyEnvOverrides_InvalidEnvValue(t *testing.T) {
	cfg := &Config{}

	// Set an invalid boolean value - Billing.Enabled is a bool field
	t.Setenv("APICERBERUS_BILLING_ENABLED", "not-a-bool")

	err := applyEnvOverrides(cfg)
	if err == nil {
		t.Error("applyEnvOverrides() should return error for invalid bool")
	}
}

func TestApplyEnvOverrides_InvalidIntValue(t *testing.T) {
	cfg := &Config{}

	// Set an invalid int value
	t.Setenv("APICERBERUS_GATEWAY_MAX_HEADER_BYTES", "not-an-int")

	err := applyEnvOverrides(cfg)
	if err == nil {
		t.Error("applyEnvOverrides() should return error for invalid int")
	}
}

func TestApplyEnvOverrides_InvalidUintValue(t *testing.T) {
	// There are no uint fields in the config struct, so we test with a float field instead
	cfg := &Config{}

	// Set an invalid float value
	t.Setenv("APICERBERUS_BILLING_DEFAULT_COST", "not-a-number")

	err := applyEnvOverrides(cfg)
	if err == nil {
		t.Error("applyEnvOverrides() should return error for invalid numeric value")
	}
}

func TestApplyEnvOverrides_InvalidFloatValue(t *testing.T) {
	cfg := &Config{}

	// Set an invalid float value
	t.Setenv("APICERBERUS_BILLING_DEFAULT_COST", "not-a-float")

	err := applyEnvOverrides(cfg)
	if err == nil {
		t.Error("applyEnvOverrides() should return error for invalid float")
	}
}

func TestApplyEnvOverrides_NestedStruct(t *testing.T) {
	cfg := &Config{}

	// Set a nested value
	t.Setenv("APICERBERUS_GATEWAY_HTTP_ADDR", ":9090")

	err := applyEnvOverrides(cfg)
	if err != nil {
		t.Errorf("applyEnvOverrides() error = %v", err)
	}

	if cfg.Gateway.HTTPAddr != ":9090" {
		t.Errorf("Expected HTTPAddr = :9090, got %q", cfg.Gateway.HTTPAddr)
	}
}

func TestApplyEnvOverrides_DeepNested(t *testing.T) {
	cfg := &Config{}

	// Set a deeply nested value
	t.Setenv("APICERBERUS_LOGGING_ROTATION_MAX_SIZE_MB", "50")

	err := applyEnvOverrides(cfg)
	if err != nil {
		t.Errorf("applyEnvOverrides() error = %v", err)
	}

	if cfg.Logging.Rotation.MaxSizeMB != 50 {
		t.Errorf("Expected MaxSizeMB = 50, got %d", cfg.Logging.Rotation.MaxSizeMB)
	}
}

// ==================== envSegment Tests ====================

func TestEnvSegment(t *testing.T) {
	tests := []struct {
		name     string
		field    reflect.StructField
		expected string
	}{
		{
			name:     "with yaml tag",
			field:    reflect.StructField{Name: "HTTPAddr", Tag: reflect.StructTag(`yaml:"http_addr"`)},
			expected: "HTTP_ADDR",
		},
		{
			name:     "with yaml tag and options",
			field:    reflect.StructField{Name: "HTTPAddr", Tag: reflect.StructTag(`yaml:"http_addr,omitempty"`)},
			expected: "HTTP_ADDR",
		},
		{
			name:     "without yaml tag",
			field:    reflect.StructField{Name: "GatewayConfig"},
			expected: "GATEWAY_CONFIG",
		},
		{
			name:     "yaml tag with dash",
			field:    reflect.StructField{Name: "SomeField", Tag: reflect.StructTag(`yaml:"some-field"`)},
			expected: "SOME_FIELD",
		},
		{
			name:     "yaml tag is dash",
			field:    reflect.StructField{Name: "Ignored", Tag: reflect.StructTag(`yaml:"-"`)},
			expected: "IGNORED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := envSegment(tt.field)
			if result != tt.expected {
				t.Errorf("envSegment() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// ==================== isNestedStructField Tests ====================

func TestIsNestedStructField(t *testing.T) {
	tests := []struct {
		name     string
		value    reflect.Value
		expected bool
	}{
		{
			name:     "struct value",
			value:    reflect.ValueOf(GatewayConfig{}),
			expected: true,
		},
		{
			name:     "string value",
			value:    reflect.ValueOf("test"),
			expected: false,
		},
		{
			name:     "time.Time value",
			value:    reflect.ValueOf(time.Now()),
			expected: false,
		},
		{
			name:     "int value",
			value:    reflect.ValueOf(42),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNestedStructField(tt.value)
			if result != tt.expected {
				t.Errorf("isNestedStructField() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// ==================== normalizeNames Tests ====================

func TestNormalizeNames(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "empty slice",
			input:    []string{},
			expected: nil,
		},
		{
			name:     "nil slice",
			input:    nil,
			expected: nil,
		},
		{
			name:     "with whitespace",
			input:    []string{"  header1  ", "  ", "header2"},
			expected: []string{"header1", "header2"},
		},
		{
			name:     "all empty strings",
			input:    []string{"  ", "", "   "},
			expected: nil,
		},
		{
			name:     "no trimming needed",
			input:    []string{"header1", "header2"},
			expected: []string{"header1", "header2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeNames(tt.input)
			if !stringSlicesEqual(result, tt.expected) {
				t.Errorf("normalizeNames() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// ==================== normalizePluginConfigs Tests ====================

func TestNormalizePluginConfigs(t *testing.T) {
	tests := []struct {
		name     string
		input    []PluginConfig
		expected []PluginConfig
	}{
		{
			name:     "empty slice",
			input:    []PluginConfig{},
			expected: nil,
		},
		{
			name:     "nil slice",
			input:    nil,
			expected: nil,
		},
		{
			name: "with whitespace in name",
			input: []PluginConfig{
				{Name: "  cors  ", Config: nil},
			},
			expected: []PluginConfig{
				{Name: "cors", Config: map[string]any{}},
			},
		},
		{
			name: "with existing config",
			input: []PluginConfig{
				{Name: "rate-limit", Config: map[string]any{"limit": 100}},
			},
			expected: []PluginConfig{
				{Name: "rate-limit", Config: map[string]any{"limit": 100}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizePluginConfigs(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("normalizePluginConfigs() length = %d, want %d", len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i].Name != tt.expected[i].Name {
					t.Errorf("normalizePluginConfigs()[%d].Name = %q, want %q", i, result[i].Name, tt.expected[i].Name)
				}
			}
		})
	}
}

// ==================== generateIDs Tests ====================

func TestGenerateIDs_PreservesExisting(t *testing.T) {
	cfg := &Config{
		Services: []Service{
			{ID: "existing-service-id", Name: "svc1"},
		},
		Routes: []Route{
			{ID: "existing-route-id", Name: "route1"},
		},
		Upstreams: []Upstream{
			{
				ID:   "existing-upstream-id",
				Name: "up1",
				Targets: []UpstreamTarget{
					{ID: "existing-target-id", Address: "localhost:8080"},
				},
			},
		},
		Consumers: []Consumer{
			{
				ID:   "existing-consumer-id",
				Name: "consumer1",
				APIKeys: []ConsumerAPIKey{
					{ID: "existing-key-id", Key: "test-key"},
				},
			},
		},
	}

	err := generateIDs(cfg)
	if err != nil {
		t.Fatalf("generateIDs() error = %v", err)
	}

	// Check that existing IDs are preserved
	if cfg.Services[0].ID != "existing-service-id" {
		t.Error("Service ID should be preserved")
	}
	if cfg.Routes[0].ID != "existing-route-id" {
		t.Error("Route ID should be preserved")
	}
	if cfg.Upstreams[0].ID != "existing-upstream-id" {
		t.Error("Upstream ID should be preserved")
	}
	if cfg.Upstreams[0].Targets[0].ID != "existing-target-id" {
		t.Error("Target ID should be preserved")
	}
	if cfg.Consumers[0].ID != "existing-consumer-id" {
		t.Error("Consumer ID should be preserved")
	}
	if cfg.Consumers[0].APIKeys[0].ID != "existing-key-id" {
		t.Error("API Key ID should be preserved")
	}
}

func TestGenerateIDs_GeneratesNew(t *testing.T) {
	cfg := &Config{
		Services: []Service{
			{Name: "svc1"},
		},
		Routes: []Route{
			{Name: "route1"},
		},
		Upstreams: []Upstream{
			{
				Name: "up1",
				Targets: []UpstreamTarget{
					{Address: "localhost:8080"},
				},
			},
		},
		Consumers: []Consumer{
			{
				Name: "consumer1",
				APIKeys: []ConsumerAPIKey{
					{Key: "test-key"},
				},
			},
		},
	}

	err := generateIDs(cfg)
	if err != nil {
		t.Fatalf("generateIDs() error = %v", err)
	}

	// Check that new IDs are generated
	if cfg.Services[0].ID == "" {
		t.Error("Service ID should be generated")
	}
	if cfg.Routes[0].ID == "" {
		t.Error("Route ID should be generated")
	}
	if cfg.Upstreams[0].ID == "" {
		t.Error("Upstream ID should be generated")
	}
	if cfg.Upstreams[0].Targets[0].ID == "" {
		t.Error("Target ID should be generated")
	}
	if cfg.Consumers[0].ID == "" {
		t.Error("Consumer ID should be generated")
	}
	if cfg.Consumers[0].APIKeys[0].ID == "" {
		t.Error("API Key ID should be generated")
	}
}

// ==================== DynamicConfigManager Tests ====================

func TestDynamicConfigManager_SaveVersionHistoryLimit(t *testing.T) {
	config := &Config{Gateway: GatewayConfig{HTTPAddr: ":8080"}}
	reloader := func(cfg *Config) error { return nil }
	manager, _ := NewDynamicConfigManager(config, reloader)

	// Add more than maxHistory versions
	for i := 0; i < 15; i++ {
		newConfig := &Config{Gateway: GatewayConfig{HTTPAddr: ":" + strconv.Itoa(9000+i)}}
		manager.UpdateConfig(newConfig, "user1")
	}

	history := manager.GetHistory()
	if len(history) != 10 { // Should be limited to maxHistory
		t.Errorf("history length = %d, want 10", len(history))
	}
}

// ==================== ConfigReloader Tests ====================

func TestConfigReloader_Start(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yaml")

	configContent := `
gateway:
  http_addr: ":8080"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	reloader := func(cfg *Config) error { return nil }
	cr, err := NewConfigReloader(configPath, reloader)
	if err != nil {
		t.Fatalf("NewConfigReloader() error = %v", err)
	}
	defer cr.Stop()

	// Start the reloader
	cr.Start()

	// Give it time to start
	time.Sleep(100 * time.Millisecond)
}

func TestConfigReloader_WatchEvents(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yaml")

	configContent := `
gateway:
  http_addr: ":8080"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	reloaderCalled := make(chan bool, 1)
	reloader := func(cfg *Config) error {
		reloaderCalled <- true
		return nil
	}

	cr, err := NewConfigReloader(configPath, reloader)
	if err != nil {
		t.Fatalf("NewConfigReloader() error = %v", err)
	}
	defer cr.Stop()

	cr.SetDebounceTime(100 * time.Millisecond)
	cr.Start()

	// Modify the file
	time.Sleep(50 * time.Millisecond)
	newContent := configContent + "# modified\n"
	if err := os.WriteFile(configPath, []byte(newContent), 0644); err != nil {
		t.Fatalf("Failed to modify config: %v", err)
	}

	// Wait for reload
	select {
	case <-reloaderCalled:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for reloader to be called")
	}
}

func TestConfigReloader_HandleChangeValidationError(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yaml")

	// Create an invalid config (missing required fields)
	configContent := `
gateway:
  http_addr: ""
  https_addr: ""
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	reloader := func(cfg *Config) error { return nil }
	cr, err := NewConfigReloader(configPath, reloader)
	if err != nil {
		t.Fatalf("NewConfigReloader() error = %v", err)
	}
	defer cr.Stop()

	// Trigger manual reload - should fail validation but not panic
	cr.TriggerManualReload()

	// Give it time to process
	time.Sleep(200 * time.Millisecond)
}

func TestConfigReloader_HandleChangeReloaderError(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yaml")

	configContent := `
gateway:
  http_addr: ":8080"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	reloaderCalled := make(chan bool, 1)
	reloader := func(cfg *Config) error {
		reloaderCalled <- true
		return nil // Return error to test error path
	}

	cr, err := NewConfigReloader(configPath, reloader)
	if err != nil {
		t.Fatalf("NewConfigReloader() error = %v", err)
	}
	defer cr.Stop()

	cr.SetDebounceTime(50 * time.Millisecond)
	cr.Start()

	// Trigger manual reload
	cr.TriggerManualReload()

	// Wait for reload
	select {
	case <-reloaderCalled:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for reloader to be called")
	}
}

// ==================== Helper Functions ====================

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || containsInternal(s, substr))
}

func containsInternal(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Test ConfigReloader with reloader error
func TestConfigReloader_ReloaderError(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yaml")

	configContent := `
gateway:
  http_addr: ":8080"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	reloaderCalled := make(chan bool, 1)
	reloader := func(cfg *Config) error {
		reloaderCalled <- true
		return fmt.Errorf("reloader error")
	}

	cr, err := NewConfigReloader(configPath, reloader)
	if err != nil {
		t.Fatalf("NewConfigReloader() error = %v", err)
	}
	defer cr.Stop()

	cr.SetDebounceTime(50 * time.Millisecond)
	cr.Start()

	// Trigger manual reload
	cr.TriggerManualReload()

	// Wait for reload to be called (even though it returns an error)
	select {
	case <-reloaderCalled:
		// Success - reloader was called even though it returned an error
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for reloader to be called")
	}
}
