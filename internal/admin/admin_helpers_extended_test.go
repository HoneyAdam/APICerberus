package admin

import (
	"testing"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// TestCloneConfig tests the deep copy function with populated config
func TestCloneConfig(t *testing.T) {
	t.Parallel()

	enabled := true

	t.Run("clone populated config", func(t *testing.T) {
		src := &config.Config{
			Services: []config.Service{{ID: "svc-1", Name: "Test Service", Protocol: "http"}},
			Routes: []config.Route{
				{
					ID: "route-1", Name: "Test Route", Service: "svc-1",
					Paths: []string{"/api/*"}, Methods: []string{"GET"},
					Plugins: []config.PluginConfig{{Name: "rate-limit", Enabled: &enabled}},
				},
			},
			Upstreams: []config.Upstream{
				{
					ID: "up-1", Name: "Test Upstream", Algorithm: "round_robin",
					Targets: []config.UpstreamTarget{{ID: "t-1", Address: "localhost:8080"}},
				},
			},
			Consumers: []config.Consumer{
				{
					ID: "consumer-1", Name: "Test Consumer",
					APIKeys:   []config.ConsumerAPIKey{{Key: "ck_live_test"}},
					ACLGroups: []string{"admin"},
					Metadata:  map[string]any{"env": "test"},
				},
			},
			Audit: config.AuditConfig{
				RouteRetentionDays: map[string]int{"route-1": 90},
			},
			Billing: config.BillingConfig{
				DefaultCost:       10,
				RouteCosts:        map[string]int64{"route-1": 5},
				MethodMultipliers: map[string]float64{"GET": 1.0},
				ZeroBalanceAction: "reject",
			},
			Auth: config.AuthConfig{
				APIKey: config.APIKeyAuthConfig{
					KeyNames:    []string{"X-API-Key"},
					QueryNames:  []string{"api_key"},
					CookieNames: []string{"auth_token"},
				},
			},
		}
		cloned := config.CloneConfig(src)

		if cloned == src {
			t.Error("cloned config should be a different pointer")
		}
		if len(cloned.Services) != 1 {
			t.Errorf("expected 1 service, got %d", len(cloned.Services))
		}
		if cloned.Services[0].ID != "svc-1" {
			t.Errorf("expected service ID svc-1, got %s", cloned.Services[0].ID)
		}
		if len(cloned.Routes) != 1 {
			t.Errorf("expected 1 route, got %d", len(cloned.Routes))
		}
		if len(cloned.Routes[0].Plugins) != 1 {
			t.Errorf("expected 1 route plugin, got %d", len(cloned.Routes[0].Plugins))
		}
		if len(cloned.Upstreams) != 1 {
			t.Errorf("expected 1 upstream, got %d", len(cloned.Upstreams))
		}
		if len(cloned.Upstreams[0].Targets) != 1 {
			t.Errorf("expected 1 upstream target, got %d", len(cloned.Upstreams[0].Targets))
		}
		if len(cloned.Consumers) != 1 {
			t.Errorf("expected 1 consumer, got %d", len(cloned.Consumers))
		}
		if cloned.Consumers[0].Metadata["env"] != "test" {
			t.Errorf("expected consumer metadata env=test, got %v", cloned.Consumers[0].Metadata)
		}
		if len(cloned.Auth.APIKey.KeyNames) != 1 {
			t.Errorf("expected 1 key name, got %d", len(cloned.Auth.APIKey.KeyNames))
		}
	})

	t.Run("clone empty config", func(t *testing.T) {
		src := &config.Config{}
		cloned := config.CloneConfig(src)
		if cloned == nil {
			t.Error("expected non-nil cloned config")
		}
	})

	t.Run("clone config with nil consumer metadata", func(t *testing.T) {
		src := &config.Config{
			Consumers: []config.Consumer{{ID: "c-1", Metadata: nil}},
		}
		cloned := config.CloneConfig(src)
		if cloned.Consumers[0].Metadata != nil {
			t.Error("expected nil metadata for nil source")
		}
	})

	t.Run("clone config with empty route retention", func(t *testing.T) {
		src := &config.Config{
			Audit: config.AuditConfig{RouteRetentionDays: nil},
		}
		cloned := config.CloneConfig(src)
		if cloned.Audit.RouteRetentionDays != nil {
			t.Error("expected nil route retention map")
		}
	})
}

// TestServiceAndRouteLookup tests service/route/upstream lookup helpers
func TestServiceAndRouteLookup(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Services:  []config.Service{{ID: "svc-1", Name: "MyService"}},
		Routes:    []config.Route{{ID: "route-1", Name: "MyRoute"}},
		Upstreams: []config.Upstream{{ID: "up-1", Name: "MyUpstream"}},
	}

	t.Run("service by ID found", func(t *testing.T) {
		svc := serviceByID(cfg, "svc-1")
		if svc == nil {
			t.Fatal("expected non-nil service")
		}
		if svc.Name != "MyService" {
			t.Errorf("expected MyService, got %s", svc.Name)
		}
	})

	t.Run("service by ID not found", func(t *testing.T) {
		svc := serviceByID(cfg, "nonexistent")
		if svc != nil {
			t.Errorf("expected nil service, got %+v", svc)
		}
	})

	t.Run("service by name found case insensitive", func(t *testing.T) {
		svc := serviceByName(cfg, "myservice")
		if svc == nil {
			t.Fatal("expected non-nil service")
		}
		if svc.Name != "MyService" {
			t.Errorf("expected MyService, got %s", svc.Name)
		}
	})

	t.Run("service by name not found", func(t *testing.T) {
		svc := serviceByName(cfg, "nonexistent")
		if svc != nil {
			t.Errorf("expected nil service, got %+v", svc)
		}
	})

	t.Run("route by ID found", func(t *testing.T) {
		route := routeByID(cfg, "route-1")
		if route == nil {
			t.Fatal("expected non-nil route")
		}
		if route.Name != "MyRoute" {
			t.Errorf("expected MyRoute, got %s", route.Name)
		}
	})

	t.Run("route by ID not found", func(t *testing.T) {
		route := routeByID(cfg, "nonexistent")
		if route != nil {
			t.Errorf("expected nil route, got %+v", route)
		}
	})

	t.Run("route by name found case insensitive", func(t *testing.T) {
		route := routeByName(cfg, "myroute")
		if route == nil {
			t.Fatal("expected non-nil route")
		}
		if route.Name != "MyRoute" {
			t.Errorf("expected MyRoute, got %s", route.Name)
		}
	})

	t.Run("route by name not found", func(t *testing.T) {
		route := routeByName(cfg, "nonexistent")
		if route != nil {
			t.Errorf("expected nil route, got %+v", route)
		}
	})

	t.Run("upstream by ID found", func(t *testing.T) {
		up := upstreamByID(cfg, "up-1")
		if up == nil {
			t.Fatal("expected non-nil upstream")
		}
		if up.Name != "MyUpstream" {
			t.Errorf("expected MyUpstream, got %s", up.Name)
		}
	})

	t.Run("upstream by ID not found", func(t *testing.T) {
		up := upstreamByID(cfg, "nonexistent")
		if up != nil {
			t.Errorf("expected nil upstream, got %+v", up)
		}
	})

	t.Run("upstream by name found case insensitive", func(t *testing.T) {
		up := upstreamByName(cfg, "myupstream")
		if up == nil {
			t.Fatal("expected non-nil upstream")
		}
		if up.Name != "MyUpstream" {
			t.Errorf("expected MyUpstream, got %s", up.Name)
		}
	})

	t.Run("upstream by name not found", func(t *testing.T) {
		up := upstreamByName(cfg, "nonexistent")
		if up != nil {
			t.Errorf("expected nil upstream, got %+v", up)
		}
	})

	t.Run("service index by ID found", func(t *testing.T) {
		idx := serviceIndexByID(cfg, "svc-1")
		if idx != 0 {
			t.Errorf("expected index 0, got %d", idx)
		}
	})

	t.Run("service index by ID not found", func(t *testing.T) {
		idx := serviceIndexByID(cfg, "nonexistent")
		if idx != -1 {
			t.Errorf("expected index -1, got %d", idx)
		}
	})

	t.Run("route index by ID found", func(t *testing.T) {
		idx := routeIndexByID(cfg, "route-1")
		if idx != 0 {
			t.Errorf("expected index 0, got %d", idx)
		}
	})

	t.Run("route index by ID not found", func(t *testing.T) {
		idx := routeIndexByID(cfg, "nonexistent")
		if idx != -1 {
			t.Errorf("expected index -1, got %d", idx)
		}
	})
}

// TestHelperBranches tests additional helper function branches not covered by existing tests
func TestHelperBranches(t *testing.T) {
	t.Parallel()

	t.Run("asFloat64 float32", func(t *testing.T) {
		got, ok := asFloat64(float32(3.14))
		if !ok {
			t.Fatal("expected ok=true for float32")
		}
		if got < 3.13 || got > 3.15 {
			t.Errorf("got %f, want ~3.14", got)
		}
	})

	t.Run("asFloat64 int32", func(t *testing.T) {
		got, ok := asFloat64(int32(42))
		if !ok {
			t.Fatal("expected ok=true for int32")
		}
		if got != 42.0 {
			t.Errorf("got %f, want 42", got)
		}
	})

	t.Run("asFloat64 empty string", func(t *testing.T) {
		_, ok := asFloat64("  ")
		if ok {
			t.Error("expected ok=false for empty string")
		}
	})

	t.Run("asIntSlice direct int", func(t *testing.T) {
		got := asIntSlice([]int{1, 2, 3})
		if len(got) != 3 {
			t.Errorf("got %d items, want 3", len(got))
		}
	})

	t.Run("asIntSlice any", func(t *testing.T) {
		got := asIntSlice([]any{1, 2.5, "3"})
		if len(got) != 3 {
			t.Errorf("got %d items, want 3", len(got))
		}
	})

	t.Run("asIntSlice invalid type", func(t *testing.T) {
		got := asIntSlice("not a slice")
		if got != nil {
			t.Errorf("expected nil for invalid type, got %v", got)
		}
	})

	t.Run("asBool string yes", func(t *testing.T) {
		if !asBool("yes", false) {
			t.Error("expected true for yes")
		}
	})

	t.Run("asBool string on", func(t *testing.T) {
		if !asBool("on", false) {
			t.Error("expected true for on")
		}
	})

	t.Run("asBool string empty falls back", func(t *testing.T) {
		if asBool("  ", true) != true {
			t.Error("expected fallback true for empty string")
		}
	})

	t.Run("asBool string empty falls back false", func(t *testing.T) {
		if asBool("  ", false) != false {
			t.Error("expected fallback false for empty string")
		}
	})

	t.Run("asAnyMap nil input", func(t *testing.T) {
		got := asAnyMap(nil)
		if got == nil {
			t.Error("expected non-nil empty map")
		}
		if len(got) != 0 {
			t.Errorf("expected empty map, got %v", got)
		}
	})

	t.Run("asAnyMap with empty keys filtered", func(t *testing.T) {
		got := asAnyMap(map[string]any{"  ": "val", "key": "value"})
		if _, exists := got["  "]; exists {
			t.Error("expected empty key to be filtered")
		}
		if got["key"] != "value" {
			t.Errorf("expected key=value, got %v", got)
		}
	})

	t.Run("asStringSlice comma separated with spaces", func(t *testing.T) {
		got := asStringSlice("a, b ,  c")
		if len(got) != 3 {
			t.Errorf("got %d items, want 3: %v", len(got), got)
		}
	})

	t.Run("asInt64 float32", func(t *testing.T) {
		got := asInt64(float32(42), 0)
		if got != 42 {
			t.Errorf("got %d, want 42", got)
		}
	})

	t.Run("asInt64 int32", func(t *testing.T) {
		got := asInt64(int32(42), 0)
		if got != 42 {
			t.Errorf("got %d, want 42", got)
		}
	})
}
