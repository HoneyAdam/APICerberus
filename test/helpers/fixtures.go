// Package testhelpers provides utilities for testing APICerebrus.
package testhelpers

import (
	"fmt"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/store"
)

// FixtureBuilder helps build test fixtures with sensible defaults.
type FixtureBuilder struct {
	counter int
}

// NewFixtureBuilder creates a new fixture builder.
func NewFixtureBuilder() *FixtureBuilder {
	return &FixtureBuilder{counter: 0}
}

// NextID generates a unique ID for fixtures.
func (fb *FixtureBuilder) NextID(prefix string) string {
	fb.counter++
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().Unix(), fb.counter)
}

// FixtureUser creates a test user with sensible defaults.
// Options can be passed to override specific fields.
func FixtureUser(opts ...UserOption) *store.User {
	u := &store.User{
		ID:            generateID("user"),
		Email:         "test@example.com",
		Name:          "Test User",
		Company:       "Test Company",
		PasswordHash:  "$2a$10$N9qo8uLOickgx2ZMRZoMy.MqrqQzBZN0UfGNEsKYGsNQ1qQ1mKzIy", // "password"
		Role:          "user",
		Status:        "active",
		CreditBalance: 1000,
		RateLimits:    map[string]any{"default": 100},
		IPWhitelist:   []string{},
		Metadata:      map[string]any{"test": true},
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	for _, opt := range opts {
		opt(u)
	}

	return u
}

// FixtureAdmin creates a test admin user.
func FixtureAdmin(opts ...UserOption) *store.User {
	defaults := []UserOption{
		WithEmail("admin@example.com"),
		WithName("Admin User"),
		WithRole("admin"),
		WithCreditBalance(5000),
	}
	return FixtureUser(append(defaults, opts...)...)
}

// FixtureSuspendedUser creates a suspended test user.
func FixtureSuspendedUser(opts ...UserOption) *store.User {
	defaults := []UserOption{
		WithEmail("suspended@example.com"),
		WithName("Suspended User"),
		WithStatus("suspended"),
		WithCreditBalance(0),
	}
	return FixtureUser(append(defaults, opts...)...)
}

// UserOption is a function that modifies a User.
type UserOption func(*store.User)

// WithID sets the user ID.
func WithID(id string) UserOption {
	return func(u *store.User) {
		u.ID = id
	}
}

// WithEmail sets the user email.
func WithEmail(email string) UserOption {
	return func(u *store.User) {
		u.Email = email
	}
}

// WithName sets the user name.
func WithName(name string) UserOption {
	return func(u *store.User) {
		u.Name = name
	}
}

// WithCompany sets the user company.
func WithCompany(company string) UserOption {
	return func(u *store.User) {
		u.Company = company
	}
}

// WithPasswordHash sets the user password hash.
func WithPasswordHash(hash string) UserOption {
	return func(u *store.User) {
		u.PasswordHash = hash
	}
}

// WithRole sets the user role.
func WithRole(role string) UserOption {
	return func(u *store.User) {
		u.Role = role
	}
}

// WithStatus sets the user status.
func WithStatus(status string) UserOption {
	return func(u *store.User) {
		u.Status = status
	}
}

// WithCreditBalance sets the user credit balance.
func WithCreditBalance(balance int64) UserOption {
	return func(u *store.User) {
		u.CreditBalance = balance
	}
}

// WithRateLimits sets the user rate limits.
func WithRateLimits(limits map[string]any) UserOption {
	return func(u *store.User) {
		u.RateLimits = limits
	}
}

// WithIPWhitelist sets the user IP whitelist.
func WithIPWhitelist(ips []string) UserOption {
	return func(u *store.User) {
		u.IPWhitelist = ips
	}
}

// WithMetadata sets the user metadata.
func WithMetadata(metadata map[string]any) UserOption {
	return func(u *store.User) {
		u.Metadata = metadata
	}
}

// FixtureAPIKey creates a test API key struct (not persisted).
// Note: This creates the struct only. Use MockStore.MustCreateAPIKey to persist.
func FixtureAPIKey(opts ...APIKeyOption) *store.APIKey {
	now := time.Now().UTC()
	k := &store.APIKey{
		ID:         generateID("key"),
		UserID:     "user-test-001",
		KeyHash:    "abcd1234hash",
		KeyPrefix:  "ck_live_",
		Name:       "Test API Key",
		Status:     "active",
		LastUsedIP: "",
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	for _, opt := range opts {
		opt(k)
	}

	return k
}

// APIKeyOption is a function that modifies an APIKey.
type APIKeyOption func(*store.APIKey)

// WithKeyID sets the API key ID.
func WithKeyID(id string) APIKeyOption {
	return func(k *store.APIKey) {
		k.ID = id
	}
}

// WithUserID sets the API key user ID.
func WithUserID(userID string) APIKeyOption {
	return func(k *store.APIKey) {
		k.UserID = userID
	}
}

// WithKeyName sets the API key name.
func WithKeyName(name string) APIKeyOption {
	return func(k *store.APIKey) {
		k.Name = name
	}
}

// WithKeyStatus sets the API key status.
func WithKeyStatus(status string) APIKeyOption {
	return func(k *store.APIKey) {
		k.Status = status
	}
}

// WithKeyPrefix sets the API key prefix (e.g., "ck_live_" or "ck_test_").
func WithKeyPrefix(prefix string) APIKeyOption {
	return func(k *store.APIKey) {
		k.KeyPrefix = prefix
	}
}

// FixtureRoute creates a test route configuration.
func FixtureRoute(opts ...RouteOption) config.Route {
	r := config.Route{
		ID:           generateID("route"),
		Name:         "test-route",
		Service:      "test-service",
		Hosts:        []string{"example.com"},
		Paths:        []string{"/api/test"},
		Methods:      []string{"GET", "POST"},
		StripPath:    false,
		PreserveHost: false,
		Priority:     100,
		Plugins:      []config.PluginConfig{},
	}

	for _, opt := range opts {
		opt(&r)
	}

	return r
}

// RouteOption is a function that modifies a Route.
type RouteOption func(*config.Route)

// WithRouteID sets the route ID.
func WithRouteID(id string) RouteOption {
	return func(r *config.Route) {
		r.ID = id
	}
}

// WithRouteName sets the route name.
func WithRouteName(name string) RouteOption {
	return func(r *config.Route) {
		r.Name = name
	}
}

// WithRouteService sets the route service.
func WithRouteService(service string) RouteOption {
	return func(r *config.Route) {
		r.Service = service
	}
}

// WithRouteHosts sets the route hosts.
func WithRouteHosts(hosts []string) RouteOption {
	return func(r *config.Route) {
		r.Hosts = hosts
	}
}

// WithRoutePaths sets the route paths.
func WithRoutePaths(paths []string) RouteOption {
	return func(r *config.Route) {
		r.Paths = paths
	}
}

// WithRouteMethods sets the route methods.
func WithRouteMethods(methods []string) RouteOption {
	return func(r *config.Route) {
		r.Methods = methods
	}
}

// WithRouteStripPath sets whether to strip the path.
func WithRouteStripPath(strip bool) RouteOption {
	return func(r *config.Route) {
		r.StripPath = strip
	}
}

// WithRoutePreserveHost sets whether to preserve the host header.
func WithRoutePreserveHost(preserve bool) RouteOption {
	return func(r *config.Route) {
		r.PreserveHost = preserve
	}
}

// WithRoutePriority sets the route priority.
func WithRoutePriority(priority int) RouteOption {
	return func(r *config.Route) {
		r.Priority = priority
	}
}

// WithRoutePlugins sets the route plugins.
func WithRoutePlugins(plugins []config.PluginConfig) RouteOption {
	return func(r *config.Route) {
		r.Plugins = plugins
	}
}

// AddRoutePlugin adds a plugin to the route.
func AddRoutePlugin(plugin config.PluginConfig) RouteOption {
	return func(r *config.Route) {
		r.Plugins = append(r.Plugins, plugin)
	}
}

// FixtureService creates a test service configuration.
func FixtureService(opts ...ServiceOption) config.Service {
	s := config.Service{
		ID:             generateID("svc"),
		Name:           "test-service",
		Protocol:       "http",
		Upstream:       "test-upstream",
		ConnectTimeout: 5 * time.Second,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
	}

	for _, opt := range opts {
		opt(&s)
	}

	return s
}

// ServiceOption is a function that modifies a Service.
type ServiceOption func(*config.Service)

// WithServiceID sets the service ID.
func WithServiceID(id string) ServiceOption {
	return func(s *config.Service) {
		s.ID = id
	}
}

// WithServiceName sets the service name.
func WithServiceName(name string) ServiceOption {
	return func(s *config.Service) {
		s.Name = name
	}
}

// WithServiceProtocol sets the service protocol.
func WithServiceProtocol(protocol string) ServiceOption {
	return func(s *config.Service) {
		s.Protocol = protocol
	}
}

// WithServiceUpstream sets the service upstream.
func WithServiceUpstream(upstream string) ServiceOption {
	return func(s *config.Service) {
		s.Upstream = upstream
	}
}

// WithServiceTimeouts sets the service timeouts.
func WithServiceTimeouts(connect, read, write time.Duration) ServiceOption {
	return func(s *config.Service) {
		s.ConnectTimeout = connect
		s.ReadTimeout = read
		s.WriteTimeout = write
	}
}

// FixtureConsumer creates a test consumer configuration.
func FixtureConsumer(opts ...ConsumerOption) config.Consumer {
	c := config.Consumer{
		ID:   generateID("consumer"),
		Name: "test-consumer",
		APIKeys: []config.ConsumerAPIKey{
			{
				ID:        generateID("key"),
				Key:       "ck_live_testkey123",
				CreatedAt: time.Now().UTC().Format(time.RFC3339),
			},
		},
		RateLimit: config.ConsumerRateLimit{
			RequestsPerSecond: 100,
			Burst:             150,
		},
		ACLGroups: []string{"default"},
		Metadata:  map[string]any{"test": true},
	}

	for _, opt := range opts {
		opt(&c)
	}

	return c
}

// ConsumerOption is a function that modifies a Consumer.
type ConsumerOption func(*config.Consumer)

// WithConsumerID sets the consumer ID.
func WithConsumerID(id string) ConsumerOption {
	return func(c *config.Consumer) {
		c.ID = id
	}
}

// WithConsumerName sets the consumer name.
func WithConsumerName(name string) ConsumerOption {
	return func(c *config.Consumer) {
		c.Name = name
	}
}

// WithConsumerAPIKeys sets the consumer API keys.
func WithConsumerAPIKeys(keys []config.ConsumerAPIKey) ConsumerOption {
	return func(c *config.Consumer) {
		c.APIKeys = keys
	}
}

// AddConsumerAPIKey adds an API key to the consumer.
func AddConsumerAPIKey(key config.ConsumerAPIKey) ConsumerOption {
	return func(c *config.Consumer) {
		c.APIKeys = append(c.APIKeys, key)
	}
}

// WithConsumerRateLimit sets the consumer rate limit.
func WithConsumerRateLimit(rps, burst int) ConsumerOption {
	return func(c *config.Consumer) {
		c.RateLimit.RequestsPerSecond = rps
		c.RateLimit.Burst = burst
	}
}

// WithConsumerACLGroups sets the consumer ACL groups.
func WithConsumerACLGroups(groups []string) ConsumerOption {
	return func(c *config.Consumer) {
		c.ACLGroups = groups
	}
}

// WithConsumerMetadata sets the consumer metadata.
func WithConsumerMetadata(metadata map[string]any) ConsumerOption {
	return func(c *config.Consumer) {
		c.Metadata = metadata
	}
}

// FixtureUpstream creates a test upstream configuration.
func FixtureUpstream(opts ...UpstreamOption) config.Upstream {
	u := config.Upstream{
		ID:        generateID("upstream"),
		Name:      "test-upstream",
		Algorithm: "round_robin",
		Targets: []config.UpstreamTarget{
			{ID: generateID("target"), Address: "localhost:8081", Weight: 100},
			{ID: generateID("target"), Address: "localhost:8082", Weight: 100},
		},
		HealthCheck: config.HealthCheckConfig{
			Active: config.ActiveHealthCheckConfig{
				Path:               "/health",
				Interval:           10 * time.Second,
				Timeout:            5 * time.Second,
				HealthyThreshold:   2,
				UnhealthyThreshold: 3,
			},
		},
	}

	for _, opt := range opts {
		opt(&u)
	}

	return u
}

// UpstreamOption is a function that modifies an Upstream.
type UpstreamOption func(*config.Upstream)

// WithUpstreamID sets the upstream ID.
func WithUpstreamID(id string) UpstreamOption {
	return func(u *config.Upstream) {
		u.ID = id
	}
}

// WithUpstreamName sets the upstream name.
func WithUpstreamName(name string) UpstreamOption {
	return func(u *config.Upstream) {
		u.Name = name
	}
}

// WithUpstreamAlgorithm sets the upstream algorithm.
func WithUpstreamAlgorithm(algorithm string) UpstreamOption {
	return func(u *config.Upstream) {
		u.Algorithm = algorithm
	}
}

// WithUpstreamTargets sets the upstream targets.
func WithUpstreamTargets(targets []config.UpstreamTarget) UpstreamOption {
	return func(u *config.Upstream) {
		u.Targets = targets
	}
}

// AddUpstreamTarget adds a target to the upstream.
func AddUpstreamTarget(target config.UpstreamTarget) UpstreamOption {
	return func(u *config.Upstream) {
		u.Targets = append(u.Targets, target)
	}
}

// FixturePluginConfig creates a plugin configuration.
func FixturePluginConfig(name string, enabled bool, cfg map[string]any) config.PluginConfig {
	return config.PluginConfig{
		Name:    name,
		Enabled: &enabled,
		Config:  cfg,
	}
}

// FixtureCORSPlugin creates a CORS plugin configuration.
func FixtureCORSPlugin(origins, methods, headers []string) config.PluginConfig {
	enabled := true
	return config.PluginConfig{
		Name:    "cors",
		Enabled: &enabled,
		Config: map[string]any{
			"allowed_origins": origins,
			"allowed_methods": methods,
			"allowed_headers": headers,
			"credentials":     true,
			"max_age":         3600,
		},
	}
}

// FixtureRateLimitPlugin creates a rate limit plugin configuration.
func FixtureRateLimitPlugin(requestsPerSecond, burst int) config.PluginConfig {
	enabled := true
	return config.PluginConfig{
		Name:    "rate-limit",
		Enabled: &enabled,
		Config: map[string]any{
			"algorithm":           "token_bucket",
			"requests_per_second": requestsPerSecond,
			"burst":               burst,
			"scope":               "consumer",
		},
	}
}

// FixtureAuthAPIKeyPlugin creates an API key auth plugin configuration.
func FixtureAuthAPIKeyPlugin() config.PluginConfig {
	enabled := true
	return config.PluginConfig{
		Name:    "auth-apikey",
		Enabled: &enabled,
		Config: map[string]any{
			"key_names":    []string{"X-API-Key"},
			"query_names":  []string{"api_key"},
			"cookie_names": []string{"apikey"},
		},
	}
}

// FixtureRequestSizeLimitPlugin creates a request size limit plugin configuration.
func FixtureRequestSizeLimitPlugin(maxBytes int64) config.PluginConfig {
	enabled := true
	return config.PluginConfig{
		Name:    "request-size-limit",
		Enabled: &enabled,
		Config: map[string]any{
			"max_bytes": maxBytes,
		},
	}
}

// FixtureConfig creates a complete test configuration.
func FixtureConfig(opts ...ConfigOption) *config.Config {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			HTTPAddr:       "localhost:8080",
			ReadTimeout:    30 * time.Second,
			WriteTimeout:   30 * time.Second,
			IdleTimeout:    120 * time.Second,
			MaxHeaderBytes: 1 << 20,  // 1MB
			MaxBodyBytes:   10 << 20, // 10MB
		},
		Admin: config.AdminConfig{
			Addr:      "localhost:9876",
			APIKey:    "test-admin-key",
			UIEnabled: true,
		},
		Store: config.StoreConfig{
			Path:        ":memory:",
			BusyTimeout: 5 * time.Second,
			JournalMode: "MEMORY",
			ForeignKeys: true,
		},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
		Services: []config.Service{
			FixtureService(),
		},
		Routes: []config.Route{
			FixtureRoute(),
		},
		Upstreams: []config.Upstream{
			FixtureUpstream(),
		},
		Consumers: []config.Consumer{
			FixtureConsumer(),
		},
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return cfg
}

// ConfigOption is a function that modifies a Config.
type ConfigOption func(*config.Config)

// WithGatewayAddr sets the gateway HTTP address.
func WithGatewayAddr(addr string) ConfigOption {
	return func(c *config.Config) {
		c.Gateway.HTTPAddr = addr
	}
}

// WithAdminAddr sets the admin address.
func WithAdminAddr(addr string) ConfigOption {
	return func(c *config.Config) {
		c.Admin.Addr = addr
	}
}

// WithAdminAPIKey sets the admin API key.
func WithAdminAPIKey(key string) ConfigOption {
	return func(c *config.Config) {
		c.Admin.APIKey = key
	}
}

// WithServices sets the services.
func WithServices(services []config.Service) ConfigOption {
	return func(c *config.Config) {
		c.Services = services
	}
}

// WithRoutes sets the routes.
func WithRoutes(routes []config.Route) ConfigOption {
	return func(c *config.Config) {
		c.Routes = routes
	}
}

// WithUpstreams sets the upstreams.
func WithUpstreams(upstreams []config.Upstream) ConfigOption {
	return func(c *config.Config) {
		c.Upstreams = upstreams
	}
}

// WithConsumers sets the consumers.
func WithConsumers(consumers []config.Consumer) ConfigOption {
	return func(c *config.Config) {
		c.Consumers = consumers
	}
}

// WithGlobalPlugins sets the global plugins.
func WithGlobalPlugins(plugins []config.PluginConfig) ConfigOption {
	return func(c *config.Config) {
		c.GlobalPlugins = plugins
	}
}

// generateID creates a unique ID with the given prefix.
func generateID(prefix string) string {
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), time.Now().Unix())
}
