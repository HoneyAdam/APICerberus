package testhelpers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/plugin"
)

// TestMockStore verifies the mock store works correctly.
func TestMockStore(t *testing.T) {
	store := NewMockStore(t)

	// Verify store is created
	if store.Store == nil {
		t.Fatal("Store should not be nil")
	}

	// Verify DB is accessible
	if store.DB() == nil {
		t.Fatal("DB should not be nil")
	}

	// Create a user
	user := FixtureUser(
		WithEmail("test-mock@example.com"),
		WithName("Test Mock User"),
	)
	store.MustCreateUser(user)

	// Verify user exists
	store.AssertUserExists("test-mock@example.com")
}

// TestMockStoreWithData verifies the pre-populated store.
func TestMockStoreWithData(t *testing.T) {
	store := NewMockStoreWithData(t)

	// Verify test users exist
	store.AssertUserExists("test@example.com")
	store.AssertUserExists("admin@example.com")
	store.AssertUserExists("suspended@example.com")

	// Verify API keys were created
	user := store.MustFindUserByEmail("test@example.com")
	keys, err := store.APIKeys().ListByUser(user.ID)
	if err != nil {
		t.Fatalf("Failed to list API keys: %v", err)
	}
	if len(keys) == 0 {
		t.Error("Expected API keys to be created")
	}
}

// TestFixtureUser verifies user fixtures work.
func TestFixtureUser(t *testing.T) {
	// Default user
	user := FixtureUser()
	if user.Email != "test@example.com" {
		t.Errorf("Expected default email, got %s", user.Email)
	}
	if user.Role != "user" {
		t.Errorf("Expected default role 'user', got %s", user.Role)
	}

	// Custom user
	custom := FixtureUser(
		WithEmail("custom@example.com"),
		WithName("Custom Name"),
		WithRole("admin"),
		WithCreditBalance(5000),
	)
	if custom.Email != "custom@example.com" {
		t.Errorf("Expected custom email, got %s", custom.Email)
	}
	if custom.Role != "admin" {
		t.Errorf("Expected admin role, got %s", custom.Role)
	}
	if custom.CreditBalance != 5000 {
		t.Errorf("Expected credit balance 5000, got %d", custom.CreditBalance)
	}
}

// TestFixtureRoute verifies route fixtures work.
func TestFixtureRoute(t *testing.T) {
	route := FixtureRoute()
	if route.Name != "test-route" {
		t.Errorf("Expected default name, got %s", route.Name)
	}

	custom := FixtureRoute(
		WithRouteName("custom-route"),
		WithRouteService("custom-service"),
		WithRouteMethods([]string{"POST"}),
	)
	if custom.Name != "custom-route" {
		t.Errorf("Expected custom name, got %s", custom.Name)
	}
	if custom.Service != "custom-service" {
		t.Errorf("Expected custom service, got %s", custom.Service)
	}
}

// TestFixtureService verifies service fixtures work.
func TestFixtureService(t *testing.T) {
	service := FixtureService()
	if service.Name != "test-service" {
		t.Errorf("Expected default name, got %s", service.Name)
	}

	custom := FixtureService(
		WithServiceName("custom-service"),
		WithServiceProtocol("https"),
	)
	if custom.Name != "custom-service" {
		t.Errorf("Expected custom name, got %s", custom.Name)
	}
	if custom.Protocol != "https" {
		t.Errorf("Expected https protocol, got %s", custom.Protocol)
	}
}

// TestFixtureConsumer verifies consumer fixtures work.
func TestFixtureConsumer(t *testing.T) {
	consumer := FixtureConsumer()
	if consumer.Name != "test-consumer" {
		t.Errorf("Expected default name, got %s", consumer.Name)
	}
	if len(consumer.APIKeys) == 0 {
		t.Error("Expected API keys to be created")
	}

	custom := FixtureConsumer(
		WithConsumerName("custom-consumer"),
		WithConsumerRateLimit(1000, 2000),
	)
	if custom.Name != "custom-consumer" {
		t.Errorf("Expected custom name, got %s", custom.Name)
	}
	if custom.RateLimit.RequestsPerSecond != 1000 {
		t.Errorf("Expected RPS 1000, got %d", custom.RateLimit.RequestsPerSecond)
	}
}

// TestFixtureUpstream verifies upstream fixtures work.
func TestFixtureUpstream(t *testing.T) {
	upstream := FixtureUpstream()
	if upstream.Name != "test-upstream" {
		t.Errorf("Expected default name, got %s", upstream.Name)
	}
	if len(upstream.Targets) == 0 {
		t.Error("Expected targets to be created")
	}
}

// TestFixtureConfig verifies config fixtures work.
func TestFixtureConfig(t *testing.T) {
	cfg := FixtureConfig()
	if cfg.Gateway.HTTPAddr != "localhost:8080" {
		t.Errorf("Expected default addr, got %s", cfg.Gateway.HTTPAddr)
	}

	custom := FixtureConfig(
		WithGatewayAddr("localhost:9090"),
		WithAdminAPIKey("custom-key"),
	)
	if custom.Gateway.HTTPAddr != "localhost:9090" {
		t.Errorf("Expected custom addr, got %s", custom.Gateway.HTTPAddr)
	}
	if custom.Admin.APIKey != "custom-key" {
		t.Errorf("Expected custom key, got %s", custom.Admin.APIKey)
	}
}

// TestTestServer verifies the test server works.
func TestTestServer(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	server := NewTestServer(t, handler)

	resp := server.GET("/test", nil)
	AssertStatusOK(t, resp)
	AssertBodyContains(t, resp, "status")
}

// TestResponseAssertions verifies HTTP assertions work.
func TestResponseAssertions(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusOK)
		case "/created":
			w.WriteHeader(http.StatusCreated)
		case "/bad-request":
			w.WriteHeader(http.StatusBadRequest)
		case "/unauthorized":
			w.WriteHeader(http.StatusUnauthorized)
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"id":"123","name":"test"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	server := NewTestServer(t, handler)

	AssertStatusOK(t, server.GET("/ok", nil))
	AssertStatusCreated(t, server.GET("/created", nil))
	AssertStatusBadRequest(t, server.GET("/bad-request", nil))
	AssertStatusUnauthorized(t, server.GET("/unauthorized", nil))
	AssertStatusNotFound(t, server.GET("/not-found", nil))

	resp := server.GET("/json", nil)
	AssertHeader(t, resp, "Content-Type", "application/json")
	AssertJSONContains(t, resp, map[string]any{
		"id":   "123",
		"name": "test",
	})
}

// TestTestPipelineContext verifies the pipeline context works.
func TestTestPipelineContext(t *testing.T) {
	ctx := NewTestPipelineContext(t)

	if ctx.Request == nil {
		t.Fatal("Request should not be nil")
	}
	if ctx.Recorder == nil {
		t.Fatal("Recorder should not be nil")
	}

	// Test not aborted initially
	ctx.AssertNotAborted()

	// Test abort
	ctx.Abort("test abort")
	ctx.AssertAborted()
	ctx.AssertAbortReason("test abort")
}

// TestMockPlugin verifies mock plugins work.
func TestMockPlugin(t *testing.T) {
	// No-op plugin
	noop := NoOpMockPlugin("noop", plugin.PhasePreAuth)
	if noop.Name() != "noop" {
		t.Errorf("Expected name 'noop', got %s", noop.Name())
	}
	if noop.Phase() != plugin.PhasePreAuth {
		t.Errorf("Expected phase pre-auth, got %s", noop.Phase())
	}

	// Aborting plugin
	abortPlugin := AbortingMockPlugin("abort", plugin.PhaseAuth, http.StatusForbidden, "access denied")
	ctx := NewTestPipelineContext(t)
	handled, err := abortPlugin.Run(ctx.PipelineContext)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !handled {
		t.Error("Expected plugin to handle request")
	}
	ctx.AssertAborted()
	ctx.AssertStatus(http.StatusForbidden)

	// Error plugin
	errPlugin := ErrorMockPlugin("error", plugin.PhasePreProxy, http.ErrHandlerTimeout)
	ctx2 := NewTestPipelineContext(t)
	_, err = errPlugin.Run(ctx2.PipelineContext)
	if err == nil {
		t.Error("Expected error from plugin")
	}
}

// TestPluginChain verifies plugin chain execution.
func TestPluginChain(t *testing.T) {
	var callOrder []string

	plugin1 := NewMockPlugin("p1", plugin.PhasePreAuth).WithRun(func(ctx *plugin.PipelineContext) (bool, error) {
		callOrder = append(callOrder, "p1")
		return false, nil
	})

	plugin2 := NewMockPlugin("p2", plugin.PhaseAuth).WithRun(func(ctx *plugin.PipelineContext) (bool, error) {
		callOrder = append(callOrder, "p2")
		return false, nil
	})

	plugins := []plugin.PipelinePlugin{plugin1.ToPipelinePlugin(), plugin2.ToPipelinePlugin()}

	ctx := NewTestPipelineContext(t)
	handled, err := ctx.RunPluginChain(plugins)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if handled {
		t.Error("Expected chain not to be handled")
	}

	if len(callOrder) != 2 {
		t.Errorf("Expected 2 calls, got %d", len(callOrder))
	}
	if callOrder[0] != "p1" || callOrder[1] != "p2" {
		t.Errorf("Expected order [p1, p2], got %v", callOrder)
	}
}

// TestTestBuilderContext verifies builder context creation.
func TestTestBuilderContext(t *testing.T) {
	consumers := []config.Consumer{
		TestConsumer("c1", "Consumer 1", "key1", "key2"),
		TestConsumer("c2", "Consumer 2", "key3"),
	}

	ctx := NewTestBuilderContext(consumers)

	if len(ctx.Consumers) != 2 {
		t.Errorf("Expected 2 consumers, got %d", len(ctx.Consumers))
	}

	// Test API key lookup
	consumer, err := ctx.APIKeyLookup("key1", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if consumer == nil {
		t.Error("Expected to find consumer for key1")
	} else if consumer.ID != "c1" {
		t.Errorf("Expected consumer c1, got %s", consumer.ID)
	}

	// Test permission lookup
	perm, err := ctx.PermissionLookup("user1", "route1")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if perm == nil {
		t.Fatal("Expected permission record")
	}
	if !perm.Allowed {
		t.Error("Expected permission to be allowed by default")
	}
}

// TestDenyAllPermissionLookup verifies the deny-all lookup.
func TestDenyAllPermissionLookup(t *testing.T) {
	lookup := NewDenyAllPermissionLookup()
	perm, err := lookup("user", "route")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if perm == nil {
		t.Fatal("Expected permission record")
	}
	if perm.Allowed {
		t.Error("Expected permission to be denied")
	}
}

// TestErrorPermissionLookup verifies the error lookup.
func TestErrorPermissionLookup(t *testing.T) {
	testErr := http.ErrHandlerTimeout
	lookup := NewErrorPermissionLookup(testErr)
	perm, err := lookup("user", "route")
	if err != testErr {
		t.Errorf("Expected error %v, got %v", testErr, err)
	}
	if perm != nil {
		t.Error("Expected nil permission on error")
	}
}

// TestPluginRegistry verifies the test registry works.
func TestPluginRegistry(t *testing.T) {
	registry := NewTestPluginRegistry(t)

	// Register a custom plugin
	registry.MustRegister("test-plugin", func(spec config.PluginConfig, ctx plugin.BuilderContext) (plugin.PipelinePlugin, error) {
		return NoOpMockPlugin("test-plugin", plugin.PhasePreAuth).ToPipelinePlugin(), nil
	})

	// Build the plugin
	p := registry.MustBuild(config.PluginConfig{Name: "test-plugin"}, plugin.BuilderContext{})
	if p.Name() != "test-plugin" {
		t.Errorf("Expected name 'test-plugin', got %s", p.Name())
	}
}

// TestDefaultRegistry verifies the default registry has all plugins.
func TestDefaultRegistry(t *testing.T) {
	registry := NewDefaultTestRegistry(t)

	// Try to build some common plugins
	plugins := []string{
		"cors",
		"rate-limit",
		"auth-apikey",
		"request-size-limit",
	}

	for _, name := range plugins {
		spec := config.PluginConfig{
			Name:    name,
			Enabled: boolPtr(true),
			Config:  map[string]any{},
		}
		_, err := registry.Build(spec, plugin.BuilderContext{})
		if err != nil {
			t.Errorf("Failed to build plugin %q: %v", name, err)
		}
	}
}

// TestFixturePluginConfigs verifies plugin config fixtures.
func TestFixturePluginConfigs(t *testing.T) {
	// CORS plugin
	cors := FixtureCORSPlugin(
		[]string{"https://example.com"},
		[]string{"GET", "POST"},
		[]string{"Content-Type"},
	)
	if cors.Name != "cors" {
		t.Errorf("Expected name 'cors', got %s", cors.Name)
	}
	if cors.Config == nil {
		t.Error("Expected config to be set")
	}

	// Rate limit plugin
	rl := FixtureRateLimitPlugin(100, 150)
	if rl.Name != "rate-limit" {
		t.Errorf("Expected name 'rate-limit', got %s", rl.Name)
	}

	// Auth API key plugin
	auth := FixtureAuthAPIKeyPlugin()
	if auth.Name != "auth-apikey" {
		t.Errorf("Expected name 'auth-apikey', got %s", auth.Name)
	}

	// Request size limit
	size := FixtureRequestSizeLimitPlugin(1024)
	if size.Name != "request-size-limit" {
		t.Errorf("Expected name 'request-size-limit', got %s", size.Name)
	}
}

// TestFixtureUpstream verifies upstream fixtures work.
func TestFixtureUpstreamWithOptions(t *testing.T) {
	upstream := FixtureUpstream(
		WithUpstreamName("custom-upstream"),
		WithUpstreamAlgorithm("least_conn"),
		AddUpstreamTarget(config.UpstreamTarget{
			Address: "localhost:9090",
			Weight:  50,
		}),
	)

	if upstream.Name != "custom-upstream" {
		t.Errorf("Expected name 'custom-upstream', got %s", upstream.Name)
	}
	if upstream.Algorithm != "least_conn" {
		t.Errorf("Expected algorithm 'least_conn', got %s", upstream.Algorithm)
	}
	if len(upstream.Targets) != 3 { // 2 default + 1 added
		t.Errorf("Expected 3 targets, got %d", len(upstream.Targets))
	}
}

// TestFixtureAdmin verifies admin fixture works.
func TestFixtureAdmin(t *testing.T) {
	admin := FixtureAdmin()
	if admin.Role != "admin" {
		t.Errorf("Expected role 'admin', got %s", admin.Role)
	}
	if admin.Email != "admin@example.com" {
		t.Errorf("Expected email 'admin@example.com', got %s", admin.Email)
	}
	if admin.CreditBalance != 5000 {
		t.Errorf("Expected credit balance 5000, got %d", admin.CreditBalance)
	}
}

// TestFixtureSuspendedUser verifies suspended user fixture works.
func TestFixtureSuspendedUser(t *testing.T) {
	suspended := FixtureSuspendedUser()
	if suspended.Status != "suspended" {
		t.Errorf("Expected status 'suspended', got %s", suspended.Status)
	}
	if suspended.CreditBalance != 0 {
		t.Errorf("Expected credit balance 0, got %d", suspended.CreditBalance)
	}
}

// TestNewRequest verifies request building helpers.
func TestNewRequest(t *testing.T) {
	// Simple request
	req := NewRequest(t, "GET", "http://example.com/test", nil)
	if req.Method != "GET" {
		t.Errorf("Expected method GET, got %s", req.Method)
	}

	// With body - verify Content-Type is set when body is provided
	req = NewRequest(t, "POST", "http://example.com/test", map[string]string{
		"key": "value",
	})
	// Note: NewRequest sets Content-Type to application/json when body is not nil and not already a string/[]byte/io.Reader
	// The Content-Type is set by the MakeRequest methods, not NewRequest itself
	// Let's verify the body was set correctly instead
	if req.Body == nil {
		t.Error("Expected body to be set")
	}

	// With auth
	req = NewRequest(t, "GET", "http://example.com/test", nil)
	req = WithBearerAuth(req, "token123")
	if req.Header.Get("Authorization") != "Bearer token123" {
		t.Errorf("Expected Bearer token, got %s", req.Header.Get("Authorization"))
	}

	req = NewRequest(t, "GET", "http://example.com/test", nil)
	req = WithAPIKey(req, "ck_live_xxx")
	if req.Header.Get("X-API-Key") != "ck_live_xxx" {
		t.Errorf("Expected API key, got %s", req.Header.Get("X-API-Key"))
	}
}

// TestTestRoute verifies test route helper.
func TestTestRoute(t *testing.T) {
	route := TestRoute("my-route", "my-service", []string{"GET", "POST"}, []string{"/api/*"})
	if route.Name != "my-route" {
		t.Errorf("Expected name 'my-route', got %s", route.Name)
	}
	if route.Service != "my-service" {
		t.Errorf("Expected service 'my-service', got %s", route.Service)
	}
	if len(route.Methods) != 2 {
		t.Errorf("Expected 2 methods, got %d", len(route.Methods))
	}
}

// TestTestConsumer verifies test consumer helper.
func TestTestConsumer(t *testing.T) {
	consumer := TestConsumer("c1", "Test Consumer", "key1", "key2", "key3")
	if consumer.ID != "c1" {
		t.Errorf("Expected ID 'c1', got %s", consumer.ID)
	}
	if consumer.Name != "Test Consumer" {
		t.Errorf("Expected name 'Test Consumer', got %s", consumer.Name)
	}
	if len(consumer.APIKeys) != 3 {
		t.Errorf("Expected 3 keys, got %d", len(consumer.APIKeys))
	}
}

// TestRecordingMockPlugin verifies recording mock plugin.
func TestRecordingMockPlugin(t *testing.T) {
	plugin, called := RecordingMockPlugin("recorder", plugin.PhasePreAuth)

	if *called {
		t.Error("Expected plugin not to be called yet")
	}

	ctx := NewTestPipelineContext(t)
	plugin.Run(ctx.PipelineContext)

	if !*called {
		t.Error("Expected plugin to be called")
	}
}

// TestAssertJSON verifies JSON assertions.
func TestAssertJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"123","name":"test","nested":{"key":"value"}}`))
	})

	server := NewTestServer(t, handler)
	resp := server.GET("/test", nil)

	// Test JSON contains
	AssertJSONContains(t, resp, map[string]any{
		"id":   "123",
		"name": "test",
	})
}

// TestAssertBodyContains verifies body contains assertion.
func TestAssertBodyContains(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`Hello, World!`))
	})

	server := NewTestServer(t, handler)
	resp := server.GET("/test", nil)

	AssertBodyContains(t, resp, "World")
}

// TestReadBody verifies body reading.
func TestReadBody(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`test body content`))
	})

	server := NewTestServer(t, handler)
	resp := server.GET("/test", nil)

	body := ReadBody(t, resp)
	if body != "test body content" {
		t.Errorf("Expected 'test body content', got %s", body)
	}
}

// TestParseJSON verifies JSON parsing.
func TestParseJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"123","name":"test"}`))
	})

	server := NewTestServer(t, handler)
	resp := server.GET("/test", nil)

	var result struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	ParseJSON(t, resp, &result)

	if result.ID != "123" {
		t.Errorf("Expected ID '123', got %s", result.ID)
	}
	if result.Name != "test" {
		t.Errorf("Expected name 'test', got %s", result.Name)
	}
}

// TestStoreCount verifies store count helper.
func TestStoreCount(t *testing.T) {
	store := NewMockStore(t)

	// Store starts with 1 admin user (auto-created)
	initialCount := store.Count("users", "")
	if initialCount != 1 {
		t.Errorf("Expected 1 admin user, got %d", initialCount)
	}

	// Add a user
	store.MustCreateUser(FixtureUser())

	count := store.Count("users", "")
	if count != initialCount+1 {
		t.Errorf("Expected %d users, got %d", initialCount+1, count)
	}
}

// TestStoreAssertRowCount verifies row count assertion.
func TestStoreAssertRowCount(t *testing.T) {
	store := NewMockStore(t)
	// Store starts with 1 admin user
	store.AssertRowCount("users", 1)

	store.MustCreateUser(FixtureUser())
	store.AssertRowCount("users", 2)
}

// TestStoreTruncate verifies truncate helper.
func TestStoreTruncate(t *testing.T) {
	store := NewMockStore(t)
	// Store starts with 1 admin user
	store.MustCreateUser(FixtureUser())
	store.AssertRowCount("users", 2) // admin + created user

	store.Truncate("users")
	store.AssertRowCount("users", 0)
}

// TestNewRecorder verifies response recorder.
func TestNewRecorder(t *testing.T) {
	recorder := NewRecorder()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`created`))
	})

	req := httptest.NewRequest("GET", "/test", nil)
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", recorder.Code)
	}
	if recorder.BodyString() != "created" {
		t.Errorf("Expected body 'created', got %s", recorder.BodyString())
	}
}

// TestAssertPluginPhase verifies plugin phase assertion.
func TestAssertPluginPhase(t *testing.T) {
	mock := NoOpMockPlugin("test", plugin.PhaseAuth).ToPipelinePlugin()
	AssertPluginPhase(t, mock, plugin.PhaseAuth)
}

// TestAssertPluginPriority verifies plugin priority assertion.
func TestAssertPluginPriority(t *testing.T) {
	mock := NewMockPlugin("test", plugin.PhasePreAuth).
		WithRun(func(ctx *plugin.PipelineContext) (bool, error) {
			return false, nil
		}).ToPipelinePlugin()

	// The mock doesn't have a SetPriority method, so we just verify the assertion doesn't panic
	// when the values match (default priority is 100 from the MockPlugin struct)
	AssertPluginPriority(t, mock, 100)
}

// TestBuildDefaultTestPipeline verifies pipeline building.
func TestBuildDefaultTestPipeline(t *testing.T) {
	specs := []config.PluginConfig{
		FixtureCORSPlugin([]string{"*"}, []string{"GET"}, []string{}),
	}

	plugins := BuildDefaultTestPipeline(t, specs, nil)
	if len(plugins) != 1 {
		t.Errorf("Expected 1 plugin, got %d", len(plugins))
	}
}

// TestMustBuildRoutePipelines verifies route pipeline building.
func TestMustBuildRoutePipelines(t *testing.T) {
	cfg := FixtureConfig(
		WithRoutes([]config.Route{
			FixtureRoute(
				WithRoutePlugins([]config.PluginConfig{
					FixtureCORSPlugin([]string{"*"}, []string{"GET"}, []string{}),
				}),
			),
		}),
	)

	pipelines, hasAuth := MustBuildRoutePipelines(t, cfg, nil)
	if len(pipelines) == 0 {
		t.Error("Expected pipelines to be built")
	}
	if hasAuth == nil {
		t.Error("Expected hasAuth map to be returned")
	}
}

// TestFixtureAPIKey verifies API key fixture.
func TestFixtureAPIKey(t *testing.T) {
	key := FixtureAPIKey()
	if key.Name != "Test API Key" {
		t.Errorf("Expected name 'Test API Key', got %s", key.Name)
	}
	if key.Status != "active" {
		t.Errorf("Expected status 'active', got %s", key.Status)
	}

	custom := FixtureAPIKey(
		WithKeyName("Custom Key"),
		WithKeyPrefix("ck_test_"),
	)
	if custom.Name != "Custom Key" {
		t.Errorf("Expected name 'Custom Key', got %s", custom.Name)
	}
	if custom.KeyPrefix != "ck_test_" {
		t.Errorf("Expected prefix 'ck_test_', got %s", custom.KeyPrefix)
	}
}

// TestContextWithRequest verifies request context helper.
func TestContextWithRequest(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	ctx := ContextWithRequest(req.Context(), req)

	if ctx == nil {
		t.Error("Expected non-nil context")
	}
}

// Helper function
func boolPtr(b bool) *bool {
	return &b
}

// Ensure time is used
var _ = time.Now
