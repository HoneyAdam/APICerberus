package config

import "time"

// Config is the runtime gateway configuration snapshot.
type Config struct {
	Gateway       GatewayConfig  `yaml:"gateway" json:"gateway"`
	Admin         AdminConfig    `yaml:"admin" json:"admin"`
	Portal        PortalConfig   `yaml:"portal" json:"portal"`
	Logging       LoggingConfig  `yaml:"logging" json:"logging"`
	Store         StoreConfig    `yaml:"store" json:"store"`
	Billing       BillingConfig  `yaml:"billing" json:"billing"`
	Audit         AuditConfig    `yaml:"audit" json:"audit"`
	Services      []Service      `yaml:"services" json:"services"`
	Routes        []Route        `yaml:"routes" json:"routes"`
	Upstreams     []Upstream     `yaml:"upstreams" json:"upstreams"`
	Consumers     []Consumer     `yaml:"consumers" json:"consumers"`
	Auth          AuthConfig     `yaml:"auth" json:"auth"`
	GlobalPlugins []PluginConfig `yaml:"global_plugins" json:"global_plugins"`
}

type BillingConfig struct {
	Enabled           bool               `yaml:"enabled" json:"enabled"`
	DefaultCost       int64              `yaml:"default_cost" json:"default_cost"`
	RouteCosts        map[string]int64   `yaml:"route_costs" json:"route_costs"`
	MethodMultipliers map[string]float64 `yaml:"method_multipliers" json:"method_multipliers"`
	TestModeEnabled   bool               `yaml:"test_mode_enabled" json:"test_mode_enabled"`
	ZeroBalanceAction string             `yaml:"zero_balance_action" json:"zero_balance_action"`
}

type AuditConfig struct {
	Enabled              bool           `yaml:"enabled" json:"enabled"`
	BufferSize           int            `yaml:"buffer_size" json:"buffer_size"`
	BatchSize            int            `yaml:"batch_size" json:"batch_size"`
	FlushInterval        time.Duration  `yaml:"flush_interval" json:"flush_interval"`
	RetentionDays        int            `yaml:"retention_days" json:"retention_days"`
	RouteRetentionDays   map[string]int `yaml:"route_retention_days" json:"route_retention_days"`
	ArchiveEnabled       bool           `yaml:"archive_enabled" json:"archive_enabled"`
	ArchiveDir           string         `yaml:"archive_dir" json:"archive_dir"`
	ArchiveCompress      bool           `yaml:"archive_compress" json:"archive_compress"`
	CleanupInterval      time.Duration  `yaml:"cleanup_interval" json:"cleanup_interval"`
	CleanupBatchSize     int            `yaml:"cleanup_batch_size" json:"cleanup_batch_size"`
	MaxRequestBodyBytes  int64          `yaml:"max_request_body_bytes" json:"max_request_body_bytes"`
	MaxResponseBodyBytes int64          `yaml:"max_response_body_bytes" json:"max_response_body_bytes"`
	MaskHeaders          []string       `yaml:"mask_headers" json:"mask_headers"`
	MaskBodyFields       []string       `yaml:"mask_body_fields" json:"mask_body_fields"`
	MaskReplacement      string         `yaml:"mask_replacement" json:"mask_replacement"`
}

type AuthConfig struct {
	APIKey APIKeyAuthConfig `yaml:"api_key" json:"api_key"`
}

type APIKeyAuthConfig struct {
	KeyNames    []string `yaml:"key_names" json:"key_names"`
	QueryNames  []string `yaml:"query_names" json:"query_names"`
	CookieNames []string `yaml:"cookie_names" json:"cookie_names"`
}

type GatewayConfig struct {
	HTTPAddr       string        `yaml:"http_addr" json:"http_addr"`
	HTTPSAddr      string        `yaml:"https_addr" json:"https_addr"`
	TLS            TLSConfig     `yaml:"tls" json:"tls"`
	ReadTimeout    time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout   time.Duration `yaml:"write_timeout" json:"write_timeout"`
	IdleTimeout    time.Duration `yaml:"idle_timeout" json:"idle_timeout"`
	MaxHeaderBytes int           `yaml:"max_header_bytes" json:"max_header_bytes"`
	MaxBodyBytes   int64         `yaml:"max_body_bytes" json:"max_body_bytes"`
}

type TLSConfig struct {
	Auto      bool   `yaml:"auto" json:"auto"`
	ACMEEmail string `yaml:"acme_email" json:"acme_email"`
	ACMEDir   string `yaml:"acme_dir" json:"acme_dir"`
	CertFile  string `yaml:"cert_file" json:"cert_file"`
	KeyFile   string `yaml:"key_file" json:"key_file"`
}

type AdminConfig struct {
	Addr      string `yaml:"addr" json:"addr"`
	APIKey    string `yaml:"api_key" json:"api_key"`
	UIEnabled bool   `yaml:"ui_enabled" json:"ui_enabled"`
	UIPath    string `yaml:"ui_path" json:"ui_path"`
}

type PortalConfig struct {
	Enabled    bool                `yaml:"enabled" json:"enabled"`
	Addr       string              `yaml:"addr" json:"addr"`
	PathPrefix string              `yaml:"path_prefix" json:"path_prefix"`
	Session    PortalSessionConfig `yaml:"session" json:"session"`
}

type PortalSessionConfig struct {
	Secret     string        `yaml:"secret" json:"secret"`
	CookieName string        `yaml:"cookie_name" json:"cookie_name"`
	MaxAge     time.Duration `yaml:"max_age" json:"max_age"`
	Secure     bool          `yaml:"secure" json:"secure"`
}

type LoggingConfig struct {
	Level    string            `yaml:"level" json:"level"`
	Format   string            `yaml:"format" json:"format"`
	Output   string            `yaml:"output" json:"output"`
	File     string            `yaml:"file" json:"file"`
	Rotation LogRotationConfig `yaml:"rotation" json:"rotation"`
}

type StoreConfig struct {
	Path        string        `yaml:"path" json:"path"`
	BusyTimeout time.Duration `yaml:"busy_timeout" json:"busy_timeout"`
	JournalMode string        `yaml:"journal_mode" json:"journal_mode"`
	ForeignKeys bool          `yaml:"foreign_keys" json:"foreign_keys"`
}

type LogRotationConfig struct {
	MaxSizeMB  int  `yaml:"max_size_mb" json:"max_size_mb"`
	MaxBackups int  `yaml:"max_backups" json:"max_backups"`
	Compress   bool `yaml:"compress" json:"compress"`
}

type Service struct {
	ID             string        `yaml:"id" json:"id"`
	Name           string        `yaml:"name" json:"name"`
	Protocol       string        `yaml:"protocol" json:"protocol"`
	Upstream       string        `yaml:"upstream" json:"upstream"`
	ConnectTimeout time.Duration `yaml:"connect_timeout" json:"connect_timeout"`
	ReadTimeout    time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout   time.Duration `yaml:"write_timeout" json:"write_timeout"`
}

type Route struct {
	ID           string         `yaml:"id" json:"id"`
	Name         string         `yaml:"name" json:"name"`
	Service      string         `yaml:"service" json:"service"`
	Hosts        []string       `yaml:"hosts" json:"hosts"`
	Paths        []string       `yaml:"paths" json:"paths"`
	Methods      []string       `yaml:"methods" json:"methods"`
	StripPath    bool           `yaml:"strip_path" json:"strip_path"`
	PreserveHost bool           `yaml:"preserve_host" json:"preserve_host"`
	Priority     int            `yaml:"priority" json:"priority"`
	Plugins      []PluginConfig `yaml:"plugins" json:"plugins"`
}

type PluginConfig struct {
	Name    string         `yaml:"name" json:"name"`
	Enabled *bool          `yaml:"enabled" json:"enabled"`
	Config  map[string]any `yaml:"config" json:"config"`
}

type Upstream struct {
	ID          string            `yaml:"id" json:"id"`
	Name        string            `yaml:"name" json:"name"`
	Algorithm   string            `yaml:"algorithm" json:"algorithm"`
	Targets     []UpstreamTarget  `yaml:"targets" json:"targets"`
	HealthCheck HealthCheckConfig `yaml:"health_check" json:"health_check"`
}

type UpstreamTarget struct {
	ID      string `yaml:"id" json:"id"`
	Address string `yaml:"address" json:"address"`
	Weight  int    `yaml:"weight" json:"weight"`
}

type HealthCheckConfig struct {
	Active ActiveHealthCheckConfig `yaml:"active" json:"active"`
}

type ActiveHealthCheckConfig struct {
	Path               string        `yaml:"path" json:"path"`
	Interval           time.Duration `yaml:"interval" json:"interval"`
	Timeout            time.Duration `yaml:"timeout" json:"timeout"`
	HealthyThreshold   int           `yaml:"healthy_threshold" json:"healthy_threshold"`
	UnhealthyThreshold int           `yaml:"unhealthy_threshold" json:"unhealthy_threshold"`
}

type Consumer struct {
	ID        string            `yaml:"id" json:"id"`
	Name      string            `yaml:"name" json:"name"`
	APIKeys   []ConsumerAPIKey  `yaml:"api_keys" json:"api_keys"`
	RateLimit ConsumerRateLimit `yaml:"rate_limit" json:"rate_limit"`
	ACLGroups []string          `yaml:"acl_groups" json:"acl_groups"`
	Metadata  map[string]any    `yaml:"metadata" json:"metadata"`
}

type ConsumerAPIKey struct {
	ID        string `yaml:"id" json:"id"`
	Key       string `yaml:"key" json:"key"`
	CreatedAt string `yaml:"created_at" json:"created_at"`
	ExpiresAt string `yaml:"expires_at" json:"expires_at"`
}

type ConsumerRateLimit struct {
	RequestsPerSecond int `yaml:"requests_per_second" json:"requests_per_second"`
	Burst             int `yaml:"burst" json:"burst"`
}
