package plugin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestRegistryRegisterLookup(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	err := registry.Register("dummy", func(spec config.PluginConfig, ctx BuilderContext) (PipelinePlugin, error) {
		return PipelinePlugin{name: "dummy", phase: PhasePreAuth, priority: 1}, nil
	})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}

	if _, ok := registry.Lookup("dummy"); !ok {
		t.Fatalf("expected dummy plugin to be registered")
	}
	if _, ok := registry.Lookup("missing"); ok {
		t.Fatalf("expected missing plugin to be absent")
	}
}

func TestBuildRoutePipelinesSortAndMerge(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Routes: []config.Route{
			{
				ID:      "route-1",
				Name:    "route-1",
				Service: "svc",
				Paths:   []string{"/x"},
				Methods: []string{http.MethodGet},
				Plugins: []config.PluginConfig{
					{
						Name: "auth-apikey",
					},
					{
						Name: "rate-limit",
						Config: map[string]any{
							"algorithm": "fixed_window",
							"scope":     "global",
							"limit":     1,
							"window":    "1s",
						},
					},
				},
			},
		},
		GlobalPlugins: []config.PluginConfig{
			{
				Name: "cors",
				Config: map[string]any{
					"allowed_origins": []any{"*"},
				},
			},
		},
		Consumers: []config.Consumer{
			{
				ID:   "consumer-1",
				Name: "consumer-1",
				APIKeys: []config.ConsumerAPIKey{
					{Key: "k1"},
				},
			},
		},
	}

	pipelines, hasAuth, err := BuildRoutePipelines(cfg, cfg.Consumers)
	if err != nil {
		t.Fatalf("BuildRoutePipelines error: %v", err)
	}
	chain := pipelines["route-1"]
	if len(chain) != 3 {
		t.Fatalf("expected 3 plugins in chain, got %d", len(chain))
	}
	if chain[0].Phase() != PhasePreAuth || chain[0].Name() != "cors" {
		t.Fatalf("expected first plugin cors pre-auth, got %s/%s", chain[0].Name(), chain[0].Phase())
	}
	if chain[1].Phase() != PhaseAuth || chain[1].Name() != "auth-apikey" {
		t.Fatalf("expected second plugin auth-apikey auth, got %s/%s", chain[1].Name(), chain[1].Phase())
	}
	if chain[2].Phase() != PhasePreProxy || chain[2].Name() != "rate-limit" {
		t.Fatalf("expected third plugin rate-limit pre-proxy, got %s/%s", chain[2].Name(), chain[2].Phase())
	}
	if !hasAuth["route-1"] {
		t.Fatalf("expected route to be marked as having auth plugin")
	}
}

func TestBuildRoutePipelinesRoutePluginOverridesGlobalByName(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Routes: []config.Route{
			{
				ID:      "route-override",
				Name:    "route-override",
				Service: "svc",
				Paths:   []string{"/x"},
				Methods: []string{http.MethodGet},
				Plugins: []config.PluginConfig{
					{
						Name: "rate-limit",
						Config: map[string]any{
							"algorithm": "fixed_window",
							"scope":     "global",
							"limit":     2,
							"window":    "1s",
						},
					},
				},
			},
		},
		GlobalPlugins: []config.PluginConfig{
			{
				Name: "rate-limit",
				Config: map[string]any{
					"algorithm": "fixed_window",
					"scope":     "global",
					"limit":     1,
					"window":    "1s",
				},
			},
		},
	}

	pipelines, _, err := BuildRoutePipelines(cfg, nil)
	if err != nil {
		t.Fatalf("BuildRoutePipelines error: %v", err)
	}
	chain := pipelines["route-override"]
	if len(chain) != 1 {
		t.Fatalf("expected only one merged rate-limit plugin, got %d", len(chain))
	}
	if chain[0].Name() != "rate-limit" {
		t.Fatalf("expected rate-limit plugin, got %q", chain[0].Name())
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/x", nil)
	ctx := &PipelineContext{
		Request:        req,
		ResponseWriter: rr,
		Route:          &cfg.Routes[0],
	}

	handled, runErr := chain[0].Run(ctx)
	if runErr != nil {
		t.Fatalf("plugin run error: %v", runErr)
	}
	if handled {
		t.Fatalf("first request should not be rate limited")
	}

	rr2 := httptest.NewRecorder()
	ctx.ResponseWriter = rr2
	handled, runErr = chain[0].Run(ctx)
	if runErr != nil {
		t.Fatalf("plugin run error on second request: %v", runErr)
	}
	if handled {
		t.Fatalf("second request should still pass with route override limit=2")
	}
}

func TestBuildRoutePipelinesAutoAddsEndpointPermissionForStoreUsers(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Routes: []config.Route{
			{
				ID:      "route-1",
				Name:    "route-1",
				Service: "svc",
				Paths:   []string{"/x"},
				Methods: []string{http.MethodGet},
				Plugins: []config.PluginConfig{
					{Name: "rate-limit", Config: map[string]any{"algorithm": "fixed_window", "scope": "global", "limit": 1, "window": "1s"}},
				},
			},
		},
	}

	pipelines, _, err := BuildRoutePipelinesWithContext(cfg, BuilderContext{
		PermissionLookup: func(userID, routeID string) (*EndpointPermissionRecord, error) {
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("BuildRoutePipelinesWithContext error: %v", err)
	}
	chain := pipelines["route-1"]
	if len(chain) != 3 {
		t.Fatalf("expected user-ip-whitelist + endpoint-permission + rate-limit plugins, got %d", len(chain))
	}
	if chain[0].Name() != "user-ip-whitelist" || chain[0].Phase() != PhasePreProxy {
		t.Fatalf("expected first plugin user-ip-whitelist pre-proxy, got %s/%s", chain[0].Name(), chain[0].Phase())
	}
	if chain[1].Name() != "endpoint-permission" || chain[1].Phase() != PhasePreProxy {
		t.Fatalf("expected second plugin endpoint-permission pre-proxy, got %s/%s", chain[1].Name(), chain[1].Phase())
	}
}
