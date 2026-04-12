// Package testhelpers provides utilities for testing APICerebrus.
package testhelpers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/APICerberus/APICerebrus/internal/plugin"
)

// PluginTestRegistry wraps the plugin registry with test utilities.
type PluginTestRegistry struct {
	*plugin.Registry
	t testing.TB
}

// NewTestPluginRegistry creates a new plugin registry for testing.
func NewTestPluginRegistry(t testing.TB) *PluginTestRegistry {
	return &PluginTestRegistry{
		Registry: plugin.NewRegistry(),
		t:        t,
	}
}

// NewDefaultTestRegistry creates a registry with all default plugins registered.
func NewDefaultTestRegistry(t testing.TB) *PluginTestRegistry {
	return &PluginTestRegistry{
		Registry: plugin.NewDefaultRegistry(),
		t:        t,
	}
}

// MustRegister registers a plugin factory and fails the test if registration fails.
func (r *PluginTestRegistry) MustRegister(name string, factory plugin.PluginFactory) {
	r.t.Helper()
	if err := r.Register(name, factory); err != nil {
		r.t.Fatalf("Failed to register plugin %q: %v", name, err)
	}
}

// MustBuild builds a plugin and fails the test if building fails.
func (r *PluginTestRegistry) MustBuild(spec config.PluginConfig, ctx plugin.BuilderContext) plugin.PipelinePlugin {
	r.t.Helper()
	p, err := r.Build(spec, ctx)
	if err != nil {
		r.t.Fatalf("Failed to build plugin %q: %v", spec.Name, err)
	}
	return p
}

// TestPipelineContext provides a test context for running plugin chains.
type TestPipelineContext struct {
	*plugin.PipelineContext
	Recorder *httptest.ResponseRecorder
	t        testing.TB
}

// NewTestPipelineContext creates a new test pipeline context.
func NewTestPipelineContext(t testing.TB) *TestPipelineContext {
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)

	return &TestPipelineContext{
		PipelineContext: &plugin.PipelineContext{
			Request:        req,
			ResponseWriter: recorder,
			Response:       nil,
			Route:          nil,
			Service:        nil,
			Consumer:       nil,
			CorrelationID:  "test-correlation-id",
			Metadata:       make(map[string]any),
			Aborted:        false,
			AbortReason:    "",
			Retry:          nil,
			Cleanup:        nil,
		},
		Recorder: recorder,
		t:        t,
	}
}

// NewTestPipelineContextWithRoute creates a test context with a route configured.
func NewTestPipelineContextWithRoute(t testing.TB, route config.Route) *TestPipelineContext {
	ctx := NewTestPipelineContext(t)
	ctx.Route = &route
	return ctx
}

// NewTestPipelineContextWithConsumer creates a test context with a consumer configured.
func NewTestPipelineContextWithConsumer(t testing.TB, consumer config.Consumer) *TestPipelineContext {
	ctx := NewTestPipelineContext(t)
	ctx.Consumer = &consumer
	return ctx
}

// WithRequest sets the request in the context.
func (ctx *TestPipelineContext) WithRequest(req *http.Request) *TestPipelineContext {
	ctx.Request = req
	return ctx
}

// WithRoute sets the route in the context.
func (ctx *TestPipelineContext) WithRoute(route *config.Route) *TestPipelineContext {
	ctx.Route = route
	return ctx
}

// WithService sets the service in the context.
func (ctx *TestPipelineContext) WithService(service *config.Service) *TestPipelineContext {
	ctx.Service = service
	return ctx
}

// WithConsumer sets the consumer in the context.
func (ctx *TestPipelineContext) WithConsumer(consumer *config.Consumer) *TestPipelineContext {
	ctx.Consumer = consumer
	return ctx
}

// WithMetadata sets metadata in the context.
func (ctx *TestPipelineContext) WithMetadata(key string, value any) *TestPipelineContext {
	if ctx.Metadata == nil {
		ctx.Metadata = make(map[string]any)
	}
	ctx.Metadata[key] = value
	return ctx
}

// RunPlugin runs a single plugin and returns whether it handled the request.
func (ctx *TestPipelineContext) RunPlugin(p plugin.PipelinePlugin) (handled bool, err error) {
	ctx.t.Helper()
	return p.Run(ctx.PipelineContext)
}

// RunPluginChain runs a chain of plugins in order.
// Returns true if any plugin handled the request (aborted the chain).
func (ctx *TestPipelineContext) RunPluginChain(plugins []plugin.PipelinePlugin) (handled bool, err error) {
	ctx.t.Helper()

	for _, p := range plugins {
		handled, err = p.Run(ctx.PipelineContext)
		if err != nil {
			return false, err
		}
		if handled || ctx.Aborted {
			return true, nil
		}
	}

	return false, nil
}

// RunPostProxy runs the post-proxy hooks for all plugins.
func (ctx *TestPipelineContext) RunPostProxy(plugins []plugin.PipelinePlugin, proxyErr error) {
	ctx.t.Helper()

	for _, p := range plugins {
		p.AfterProxy(ctx.PipelineContext, proxyErr)
	}
}

// AssertAborted asserts that the context was aborted.
func (ctx *TestPipelineContext) AssertAborted() {
	ctx.t.Helper()
	if !ctx.Aborted {
		ctx.t.Error("Expected context to be aborted, but it was not")
	}
}

// AssertNotAborted asserts that the context was not aborted.
func (ctx *TestPipelineContext) AssertNotAborted() {
	ctx.t.Helper()
	if ctx.Aborted {
		ctx.t.Errorf("Expected context not to be aborted, but it was: %s", ctx.AbortReason)
	}
}

// AssertAbortReason asserts that the abort reason matches the expected value.
func (ctx *TestPipelineContext) AssertAbortReason(expected string) {
	ctx.t.Helper()
	if ctx.AbortReason != expected {
		ctx.t.Errorf("Expected abort reason %q, got %q", expected, ctx.AbortReason)
	}
}

// AssertStatus asserts that the response has the expected status code.
func (ctx *TestPipelineContext) AssertStatus(expected int) {
	ctx.t.Helper()
	if ctx.Recorder.Code != expected {
		ctx.t.Errorf("Expected status %d, got %d", expected, ctx.Recorder.Code)
	}
}

// AssertHeader asserts that the response has the expected header.
func (ctx *TestPipelineContext) AssertHeader(key, expected string) {
	ctx.t.Helper()
	actual := ctx.Recorder.Header().Get(key)
	if actual != expected {
		ctx.t.Errorf("Expected header %q to be %q, got %q", key, expected, actual)
	}
}

// AssertHeaderContains asserts that the response header contains the expected value.
func (ctx *TestPipelineContext) AssertHeaderContains(key, expected string) {
	ctx.t.Helper()
	actual := ctx.Recorder.Header().Get(key)
	if !contains(actual, expected) {
		ctx.t.Errorf("Expected header %q to contain %q, got %q", key, expected, actual)
	}
}

// AssertBodyContains asserts that the response body contains the expected string.
func (ctx *TestPipelineContext) AssertBodyContains(expected string) {
	ctx.t.Helper()
	body := ctx.Recorder.Body.String()
	if !contains(body, expected) {
		ctx.t.Errorf("Expected body to contain %q, got:\n%s", expected, body)
	}
}

// Body returns the response body as a string.
func (ctx *TestPipelineContext) Body() string {
	return ctx.Recorder.Body.String()
}

// Status returns the response status code.
func (ctx *TestPipelineContext) Status() int {
	return ctx.Recorder.Code
}

// Headers returns the response headers.
func (ctx *TestPipelineContext) Headers() http.Header {
	return ctx.Recorder.Header()
}

// MockPlugin is a mock plugin for testing.
type MockPlugin struct {
	NameVal     string
	PhaseVal    plugin.Phase
	PriorityVal int
	RunFunc     func(*plugin.PipelineContext) (bool, error)
	AfterFunc   func(*plugin.PipelineContext, error)
}

// Name returns the plugin name.
func (m *MockPlugin) Name() string { return m.NameVal }

// Phase returns the plugin phase.
func (m *MockPlugin) Phase() plugin.Phase { return m.PhaseVal }

// Priority returns the plugin priority.
func (m *MockPlugin) Priority() int { return m.PriorityVal }

// Run executes the plugin.
func (m *MockPlugin) Run(ctx *plugin.PipelineContext) (bool, error) {
	if m.RunFunc != nil {
		return m.RunFunc(ctx)
	}
	return false, nil
}

// AfterProxy runs the post-proxy hook.
func (m *MockPlugin) AfterProxy(ctx *plugin.PipelineContext, proxyErr error) {
	if m.AfterFunc != nil {
		m.AfterFunc(ctx, proxyErr)
	}
}

// ToPipelinePlugin converts the mock to a PipelinePlugin.
func (m *MockPlugin) ToPipelinePlugin() plugin.PipelinePlugin {
	return plugin.NewPipelinePlugin(
		m.NameVal,
		m.PhaseVal,
		m.PriorityVal,
		m.RunFunc,
		m.AfterFunc,
	)
}

// NewMockPlugin creates a new mock plugin.
func NewMockPlugin(name string, phase plugin.Phase) *MockPlugin {
	return &MockPlugin{
		NameVal:     name,
		PhaseVal:    phase,
		PriorityVal: 100,
	}
}

// WithRun sets the run function for the mock plugin.
func (m *MockPlugin) WithRun(fn func(*plugin.PipelineContext) (bool, error)) *MockPlugin {
	m.RunFunc = fn
	return m
}

// WithAfter sets the after function for the mock plugin.
func (m *MockPlugin) WithAfter(fn func(*plugin.PipelineContext, error)) *MockPlugin {
	m.AfterFunc = fn
	return m
}

// AbortingMockPlugin creates a mock plugin that aborts with the given reason.
func AbortingMockPlugin(name string, phase plugin.Phase, status int, reason string) *MockPlugin {
	return NewMockPlugin(name, phase).WithRun(func(ctx *plugin.PipelineContext) (bool, error) {
		ctx.ResponseWriter.WriteHeader(status)
		_, _ = ctx.ResponseWriter.Write([]byte(reason))
		ctx.Abort(reason)
		return true, nil
	})
}

// ErrorMockPlugin creates a mock plugin that returns an error.
func ErrorMockPlugin(name string, phase plugin.Phase, err error) *MockPlugin {
	return NewMockPlugin(name, phase).WithRun(func(ctx *plugin.PipelineContext) (bool, error) {
		return false, err
	})
}

// NoOpMockPlugin creates a mock plugin that does nothing.
func NoOpMockPlugin(name string, phase plugin.Phase) *MockPlugin {
	return NewMockPlugin(name, phase)
}

// RecordingMockPlugin creates a mock plugin that records whether it was called.
func RecordingMockPlugin(name string, phase plugin.Phase) (*MockPlugin, *bool) {
	var called bool
	return NewMockPlugin(name, phase).WithRun(func(ctx *plugin.PipelineContext) (bool, error) {
		called = true
		return false, nil
	}), &called
}

// RunPluginChain executes a plugin pipeline in tests.
// This is a standalone function for convenience.
func RunPluginChain(t testing.TB, ctx *plugin.PipelineContext, plugins []plugin.PipelinePlugin) (handled bool, err error) {
	t.Helper()

	for _, p := range plugins {
		handled, err = p.Run(ctx)
		if err != nil {
			return false, err
		}
		if handled || ctx.Aborted {
			return true, nil
		}
	}

	return false, nil
}

// BuildTestPipeline builds a pipeline from plugin configs for testing.
func BuildTestPipeline(t testing.TB, registry *plugin.Registry, specs []config.PluginConfig, ctx plugin.BuilderContext) []plugin.PipelinePlugin {
	t.Helper()

	plugins := make([]plugin.PipelinePlugin, 0, len(specs))
	for _, spec := range specs {
		if !isPluginEnabled(spec) {
			continue
		}
		p, err := registry.Build(spec, ctx)
		if err != nil {
			t.Fatalf("Failed to build plugin %q: %v", spec.Name, err)
		}
		plugins = append(plugins, p)
	}

	return plugins
}

// BuildDefaultTestPipeline builds a pipeline using the default registry.
func BuildDefaultTestPipeline(t testing.TB, specs []config.PluginConfig, consumers []config.Consumer) []plugin.PipelinePlugin {
	t.Helper()

	registry := plugin.NewDefaultRegistry()
	ctx := plugin.BuilderContext{
		Consumers: consumers,
	}

	return BuildTestPipeline(t, registry, specs, ctx)
}

// MustBuildRoutePipelines builds route pipelines and fails the test on error.
func MustBuildRoutePipelines(t testing.TB, cfg *config.Config, consumers []config.Consumer) (map[string][]plugin.PipelinePlugin, map[string]bool) {
	t.Helper()

	pipelines, hasAuth, err := plugin.BuildRoutePipelines(cfg, consumers)
	if err != nil {
		t.Fatalf("Failed to build route pipelines: %v", err)
	}

	return pipelines, hasAuth
}

// AssertPluginPhase asserts that a plugin has the expected phase.
func AssertPluginPhase(t testing.TB, p plugin.PipelinePlugin, expected plugin.Phase) {
	t.Helper()
	if p.Phase() != expected {
		t.Errorf("Expected plugin %q to have phase %q, got %q", p.Name(), expected, p.Phase())
	}
}

// AssertPluginPriority asserts that a plugin has the expected priority.
func AssertPluginPriority(t testing.TB, p plugin.PipelinePlugin, expected int) {
	t.Helper()
	if p.Priority() != expected {
		t.Errorf("Expected plugin %q to have priority %d, got %d", p.Name(), expected, p.Priority())
	}
}

// NewTestBuilderContext creates a builder context for testing.
func NewTestBuilderContext(consumers []config.Consumer) plugin.BuilderContext {
	return plugin.BuilderContext{
		Consumers: consumers,
		APIKeyLookup: func(rawKey string, req *http.Request) (*config.Consumer, error) {
			for _, c := range consumers {
				for _, k := range c.APIKeys {
					if k.Key == rawKey {
						return &c, nil
					}
				}
			}
			return nil, nil
		},
		PermissionLookup: func(userID, routeID string) (*plugin.EndpointPermissionRecord, error) {
			// Default: allow all for testing
			return &plugin.EndpointPermissionRecord{
				Allowed: true,
			}, nil
		},
	}
}

// NewDenyAllPermissionLookup creates a permission lookup that denies all requests.
func NewDenyAllPermissionLookup() plugin.EndpointPermissionLookupFunc {
	return func(userID, routeID string) (*plugin.EndpointPermissionRecord, error) {
		return &plugin.EndpointPermissionRecord{
			Allowed: false,
		}, nil
	}
}

// NewErrorPermissionLookup creates a permission lookup that returns an error.
func NewErrorPermissionLookup(err error) plugin.EndpointPermissionLookupFunc {
	return func(userID, routeID string) (*plugin.EndpointPermissionRecord, error) {
		return nil, err
	}
}

// isPluginEnabled checks if a plugin is enabled.
func isPluginEnabled(spec config.PluginConfig) bool {
	if spec.Enabled == nil {
		return true
	}
	return *spec.Enabled
}

// contains checks if a string contains a substring.
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

// Common test errors.
var (
	ErrTestPluginError = errors.New("test plugin error")
)

// TestConsumer creates a test consumer for plugin testing.
func TestConsumer(id, name string, apiKeys ...string) config.Consumer {
	keys := make([]config.ConsumerAPIKey, len(apiKeys))
	for i, k := range apiKeys {
		keys[i] = config.ConsumerAPIKey{
			ID:  generateID("key"),
			Key: k,
		}
	}

	return config.Consumer{
		ID:      id,
		Name:    name,
		APIKeys: keys,
		RateLimit: config.ConsumerRateLimit{
			RequestsPerSecond: 100,
			Burst:             150,
		},
	}
}

// TestRoute creates a test route for plugin testing.
func TestRoute(name, service string, methods, paths []string) config.Route {
	return config.Route{
		ID:      generateID("route"),
		Name:    name,
		Service: service,
		Methods: methods,
		Paths:   paths,
		Hosts:   []string{"example.com"},
	}
}

// ContextWithRequest creates a context with a specific request.
func ContextWithRequest(ctx context.Context, req *http.Request) context.Context {
	return req.WithContext(ctx).Context()
}
