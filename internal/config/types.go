package config

import "time"

// Config is the runtime gateway configuration snapshot.
type Config struct {
	Gateway           GatewayConfig           `yaml:"gateway" json:"gateway"`
	Admin             AdminConfig             `yaml:"admin" json:"admin"`
	Portal            PortalConfig            `yaml:"portal" json:"portal"`
	Logging           LoggingConfig           `yaml:"logging" json:"logging"`
	Store             StoreConfig             `yaml:"store" json:"store"`
	Billing           BillingConfig           `yaml:"billing" json:"billing"`
	Audit             AuditConfig             `yaml:"audit" json:"audit"`
	Cluster           ClusterConfig           `yaml:"cluster" json:"cluster"`
	Federation        FederationConfig        `yaml:"federation" json:"federation"`
	Branding          BrandingConfig          `yaml:"branding" json:"branding"`
	ACME              ACMEConfig              `yaml:"acme" json:"acme"`
	Services          []Service               `yaml:"services" json:"services"`
	Routes            []Route                 `yaml:"routes" json:"routes"`
	Upstreams         []Upstream              `yaml:"upstreams" json:"upstreams"`
	Consumers         []Consumer              `yaml:"consumers" json:"consumers"`
	Auth              AuthConfig              `yaml:"auth" json:"auth"`
	GlobalPlugins     []PluginConfig          `yaml:"global_plugins" json:"global_plugins"`
	Tracing           TracingConfig           `yaml:"tracing" json:"tracing"`
	Redis             RedisConfig             `yaml:"redis" json:"redis"`
	Kafka             KafkaConfig             `yaml:"kafka" json:"kafka"`
	PluginMarketplace PluginMarketplaceConfig `yaml:"plugin_marketplace" json:"plugin_marketplace"`
}

// PluginMarketplaceConfig holds marketplace plugin configuration.
type PluginMarketplaceConfig struct {
	Enabled           bool              `yaml:"enabled" json:"enabled"`
	DataDir           string            `yaml:"data_dir" json:"data_dir"`
	RegistryURL       string            `yaml:"registry_url" json:"registry_url"`
	TrustedSigners    []string          `yaml:"trusted_signers" json:"trusted_signers"`
	TrustedSignerKeys map[string]string `yaml:"trusted_signer_keys" json:"trusted_signer_keys"`
	AutoUpdate        bool              `yaml:"auto_update" json:"auto_update"`
	UpdateInterval    time.Duration     `yaml:"update_interval" json:"update_interval"`
	VerifySignatures  bool              `yaml:"verify_signatures" json:"verify_signatures"`
	MaxPluginSize     int64             `yaml:"max_plugin_size" json:"max_plugin_size"`
	AllowedPhases     []string          `yaml:"allowed_phases" json:"allowed_phases"`
}

// ACMEConfig holds ACME/Let's Encrypt configuration.
type ACMEConfig struct {
	Enabled      bool   `yaml:"enabled" json:"enabled"`
	Email        string `yaml:"email" json:"email"`
	DirectoryURL string `yaml:"directory_url" json:"directory_url"`
	StoragePath  string `yaml:"storage_path" json:"storage_path"`
	AcceptedTOS  bool   `yaml:"accepted_tos" json:"accepted_tos"`
}

// FederationConfig holds GraphQL Federation settings.
type FederationConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
}

// ClusterConfig holds Raft cluster configuration.
type ClusterConfig struct {
	Enabled            bool                  `yaml:"enabled" json:"enabled"`
	NodeID             string                `yaml:"node_id" json:"node_id"`
	BindAddress        string                `yaml:"bind_address" json:"bind_address"`
	Peers              []ClusterPeer         `yaml:"peers" json:"peers"`
	ElectionTimeoutMin time.Duration         `yaml:"election_timeout_min" json:"election_timeout_min"`
	ElectionTimeoutMax time.Duration         `yaml:"election_timeout_max" json:"election_timeout_max"`
	HeartbeatInterval  time.Duration         `yaml:"heartbeat_interval" json:"heartbeat_interval"`
	CertificateSync    CertificateSyncConfig `yaml:"certificate_sync" json:"certificate_sync"`
	MTLS               ClusterMTLSConfig     `yaml:"mtls" json:"mtls"`
	RPCSecret          string                `yaml:"rpc_secret" json:"rpc_secret"`
}

// ClusterMTLSConfig holds Raft inter-node mTLS configuration.
type ClusterMTLSConfig struct {
	Enabled      bool   `yaml:"enabled" json:"enabled"`
	CACertPath   string `yaml:"ca_cert_path" json:"ca_cert_path"`
	NodeCertPath string `yaml:"node_cert_path" json:"node_cert_path"`
	NodeKeyPath  string `yaml:"node_key_path" json:"node_key_path"`
	AutoGenerate bool   `yaml:"auto_generate" json:"auto_generate"`
	AutoCertDir  string `yaml:"auto_cert_dir" json:"auto_cert_dir"`
}

// CertificateSyncConfig holds certificate synchronization settings.
type CertificateSyncConfig struct {
	Enabled         bool   `yaml:"enabled" json:"enabled"`
	StoragePath     string `yaml:"storage_path" json:"storage_path"`
	RaftReplication bool   `yaml:"raft_replication" json:"raft_replication"`
}

// ClusterPeer represents a peer node in the Raft cluster.
type ClusterPeer struct {
	ID      string `yaml:"id" json:"id"`
	Address string `yaml:"address" json:"address"`
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
	StoreRequestBody     bool           `yaml:"store_request_body" json:"store_request_body"`
	StoreResponseBody    bool           `yaml:"store_response_body" json:"store_response_body"`
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
	HTTPAddr             string        `yaml:"http_addr" json:"http_addr"`
	HTTPSAddr            string        `yaml:"https_addr" json:"https_addr"`
	TLS                  TLSConfig     `yaml:"tls" json:"tls"`
	GRPC                 GRPCConfig    `yaml:"grpc" json:"grpc"`
	ReadTimeout          time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout         time.Duration `yaml:"write_timeout" json:"write_timeout"`
	IdleTimeout          time.Duration `yaml:"idle_timeout" json:"idle_timeout"`
	MaxHeaderBytes       int           `yaml:"max_header_bytes" json:"max_header_bytes"`
	MaxBodyBytes         int64         `yaml:"max_body_bytes" json:"max_body_bytes"`
	TrustedProxies       []string      `yaml:"trusted_proxies" json:"trusted_proxies"`
	HTMLErrors           bool          `yaml:"html_errors" json:"html_errors"`                       // Global HTML error page toggle
	DenyPrivateUpstreams bool          `yaml:"deny_private_upstreams" json:"deny_private_upstreams"` // Reject private/loopback upstream IPs in production
	ConnectionPool       PoolConfig    `yaml:"connection_pool" json:"connection_pool"`
}

// PoolConfig controls the HTTP connection pool for upstream proxying.
type PoolConfig struct {
	MaxIdleConns        int           `yaml:"max_idle_conns" json:"max_idle_conns"`
	MaxIdleConnsPerHost int           `yaml:"max_idle_conns_per_host" json:"max_idle_conns_per_host"`
	IdleConnTimeout     time.Duration `yaml:"idle_conn_timeout" json:"idle_conn_timeout"`
}

type GRPCConfig struct {
	Enabled           bool   `yaml:"enabled" json:"enabled"`
	Addr              string `yaml:"addr" json:"addr"`
	EnableWeb         bool   `yaml:"enable_web" json:"enable_web"`
	EnableTranscoding bool   `yaml:"enable_transcoding" json:"enable_transcoding"`
}

type TLSConfig struct {
	Enabled      bool     `yaml:"enabled" json:"enabled"`
	Auto         bool     `yaml:"auto" json:"auto"`
	ACMEEmail    string   `yaml:"acme_email" json:"acme_email"`
	ACMEDir      string   `yaml:"acme_dir" json:"acme_dir"`
	CertFile     string   `yaml:"cert_file" json:"cert_file"`
	KeyFile      string   `yaml:"key_file" json:"key_file"`
	MinVersion   string   `yaml:"min_version" json:"min_version"`
	CipherSuites []string `yaml:"cipher_suites" json:"cipher_suites"`
	SkipVerify   bool     `yaml:"skip_verify" json:"skip_verify"`
	ServerName   string   `yaml:"server_name" json:"server_name"`
}

type AdminConfig struct {
	Addr           string        `yaml:"addr" json:"addr"`
	APIKey         string        `yaml:"api_key" json:"api_key"`
	AllowedIPs     []string      `yaml:"allowed_ips" json:"allowed_ips"`
	AllowedOrigins []string      `yaml:"allowed_origins" json:"allowed_origins"`
	TokenSecret    string        `yaml:"token_secret" json:"token_secret"`
	TokenTTL       time.Duration `yaml:"token_ttl" json:"token_ttl"`
	UIEnabled      bool          `yaml:"ui_enabled" json:"ui_enabled"`
	UIPath         string        `yaml:"ui_path" json:"ui_path"`
	OIDC           OIDCConfig    `yaml:"oidc" json:"oidc"`
}

// OIDCConfig holds OpenID Connect SSO configuration.
type OIDCConfig struct {
	Enabled       bool              `yaml:"enabled" json:"enabled"`
	IssuerURL     string            `yaml:"issuer_url" json:"issuer_url"`
	ClientID      string            `yaml:"client_id" json:"client_id"`
	ClientSecret  string            `yaml:"client_secret" json:"client_secret"`
	RedirectURL   string            `yaml:"redirect_url" json:"redirect_url"`
	Scopes        []string          `yaml:"scopes" json:"scopes"`
	ClaimMapping  map[string]string `yaml:"claim_mapping" json:"claim_mapping"`
	AutoProvision bool              `yaml:"auto_provision" json:"auto_provision"`
	DefaultRole   string            `yaml:"default_role" json:"default_role"`
	Provider      OIDCProviderConfig `yaml:"provider" json:"provider"`
}

// OIDCProviderConfig enables APICerberus to act as an OIDC Authorization Server.
type OIDCProviderConfig struct {
	Enabled          bool          `yaml:"enabled" json:"enabled"`
	Issuer           string        `yaml:"issuer" json:"issuer"`
	KeyType          string        `yaml:"key_type" json:"key_type"` // "rsa" or "ec"
	KeyID            string        `yaml:"key_id" json:"key_id"`
	RSAPrivateKeyFile string       `yaml:"rsa_private_key_file" json:"rsa_private_key_file"`
	ECPrivateKeyFile  string       `yaml:"ec_private_key_file" json:"ec_private_key_file"`
	AccessTokenTTL   time.Duration `yaml:"access_token_ttl" json:"access_token_ttl"`
	IDTokenTTL       time.Duration `yaml:"id_token_ttl" json:"id_token_ttl"`
	AuthCodeTTL      time.Duration `yaml:"auth_code_ttl" json:"auth_code_ttl"`
	Clients          []OIDCClient  `yaml:"clients" json:"clients"`
}

// OIDCClient represents a registered OAuth2/OIDC client for the provider.
type OIDCClient struct {
	ClientID     string   `yaml:"client_id" json:"client_id"`
	ClientSecret string   `yaml:"client_secret" json:"client_secret"` // bcrypt hash
	RedirectURIs []string `yaml:"redirect_uris" json:"redirect_uris"`
	Scopes       []string `yaml:"scopes" json:"scopes"`
	GrantTypes   []string `yaml:"grant_types" json:"grant_types"`
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
	Driver     string          `yaml:"driver" json:"driver"` // "sqlite" or "postgres"
	Path       string          `yaml:"path" json:"path"`
	BusyTimeout time.Duration  `yaml:"busy_timeout" json:"busy_timeout"`
	JournalMode string         `yaml:"journal_mode" json:"journal_mode"`
	ForeignKeys bool           `yaml:"foreign_keys" json:"foreign_keys"`
	MaxOpenConns int           `yaml:"max_open_conns" json:"max_open_conns"`
	Synchronous string         `yaml:"synchronous" json:"synchronous"`
	WALAutoCheckpoint int      `yaml:"wal_autocheckpoint" json:"wal_autocheckpoint"`
	CacheSize   int            `yaml:"cache_size" json:"cache_size"`
	Postgres    PostgresConfig `yaml:"postgres" json:"postgres"`
}

// PostgresConfig holds PostgreSQL connection configuration.
type PostgresConfig struct {
	Host     string `yaml:"host" json:"host"`
	Port     int    `yaml:"port" json:"port"`
	User     string `yaml:"user" json:"user"`
	Password string `yaml:"password" json:"password"`
	Database string `yaml:"database" json:"database"`
	SSLMode  string `yaml:"ssl_mode" json:"ssl_mode"`
	MaxConns int    `yaml:"max_conns" json:"max_conns"`
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
	HTMLErrors   bool           `yaml:"html_errors" json:"html_errors"` // Use HTML error pages instead of JSON
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

// KafkaConfig holds Kafka configuration for audit log streaming.
type KafkaConfig struct {
	Enabled       bool          `yaml:"enabled" json:"enabled"`
	Brokers       []string      `yaml:"brokers" json:"brokers"`
	Topic         string        `yaml:"topic" json:"topic"`
	ClientID      string        `yaml:"client_id" json:"client_id"`
	GatewayID     string        `yaml:"gateway_id" json:"gateway_id"`
	Region        string        `yaml:"region" json:"region"`
	Datacenter    string        `yaml:"datacenter" json:"datacenter"`
	TLS           TLSConfig     `yaml:"tls" json:"tls"`
	SASL          SASLConfig    `yaml:"sasl" json:"sasl"`
	BatchSize     int           `yaml:"batch_size" json:"batch_size"`
	BufferSize    int           `yaml:"buffer_size" json:"buffer_size"`
	FlushInterval time.Duration `yaml:"flush_interval" json:"flush_interval"`
	WriteTimeout  time.Duration `yaml:"write_timeout" json:"write_timeout"`
	DialTimeout   time.Duration `yaml:"dial_timeout" json:"dial_timeout"`
	Workers       int           `yaml:"workers" json:"workers"`
	BlockOnFull   bool          `yaml:"block_on_full" json:"block_on_full"`
	AsyncConnect  bool          `yaml:"async_connect" json:"async_connect"`
}

// SASLConfig holds SASL authentication configuration.
type SASLConfig struct {
	Enabled   bool   `yaml:"enabled" json:"enabled"`
	Mechanism string `yaml:"mechanism" json:"mechanism"` // "plain", "scram-sha-256", "scram-sha-512"
	Username  string `yaml:"username" json:"username"`
	Password  string `yaml:"password" json:"password"`
}

// TracingConfig holds OpenTelemetry tracing configuration.
type TracingConfig struct {
	Enabled            bool              `yaml:"enabled" json:"enabled"`
	ServiceName        string            `yaml:"service_name" json:"service_name"`
	ServiceVersion     string            `yaml:"service_version" json:"service_version"`
	Exporter           string            `yaml:"exporter" json:"exporter"` // "otlp", "jaeger", "stdout"
	OTLPEndpoint       string            `yaml:"otlp_endpoint" json:"otlp_endpoint"`
	OTLPHeaders        map[string]string `yaml:"otlp_headers" json:"otlp_headers"`
	SamplingRate       float64           `yaml:"sampling_rate" json:"sampling_rate"`
	BatchTimeout       time.Duration     `yaml:"batch_timeout" json:"batch_timeout"`
	MaxQueueSize       int               `yaml:"max_queue_size" json:"max_queue_size"`
	MaxExportBatchSize int               `yaml:"max_export_batch_size" json:"max_export_batch_size"`
	Attributes         map[string]string `yaml:"attributes" json:"attributes"`
}

// RedisConfig holds Redis configuration for distributed rate limiting.
type RedisConfig struct {
	Enabled         bool          `yaml:"enabled" json:"enabled"`
	Address         string        `yaml:"address" json:"address"`
	Password        string        `yaml:"password" json:"password"`
	Database        int           `yaml:"database" json:"database"`
	MaxRetries      int           `yaml:"max_retries" json:"max_retries"`
	DialTimeout     time.Duration `yaml:"dial_timeout" json:"dial_timeout"`
	ReadTimeout     time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout" json:"write_timeout"`
	PoolSize        int           `yaml:"pool_size" json:"pool_size"`
	MinIdleConns    int           `yaml:"min_idle_conns" json:"min_idle_conns"`
	KeyPrefix       string        `yaml:"key_prefix" json:"key_prefix"`
	FallbackToLocal bool          `yaml:"fallback_to_local" json:"fallback_to_local"`
	SyncLocalOnMiss bool          `yaml:"sync_local_on_miss" json:"sync_local_on_miss"`
}

// BrandingConfig holds white-label branding customization settings.
type BrandingConfig struct {
	AppName      string `yaml:"app_name" json:"app_name"`
	LogoURL      string `yaml:"logo_url" json:"logo_url"`
	FaviconURL   string `yaml:"favicon_url" json:"favicon_url"`
	PrimaryColor string `yaml:"primary_color" json:"primary_color"`
	AccentColor  string `yaml:"accent_color" json:"accent_color"`
	ThemeMode    string `yaml:"theme_mode" json:"theme_mode"` // "light", "dark", "system"
}
