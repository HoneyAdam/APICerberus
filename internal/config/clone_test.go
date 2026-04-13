package config

import "testing"

func TestCloneConfig(t *testing.T) {
	t.Parallel()

	t.Run("nil input returns empty config", func(t *testing.T) {
		got := CloneConfig(nil)
		if got == nil {
			t.Fatal("CloneConfig(nil) should return non-nil")
		}
	})

	t.Run("deep copy with populated config", func(t *testing.T) {
		enabled := true
		src := &Config{
			Services: []Service{{ID: "svc-1", Name: "Test", Protocol: "http"}},
			Routes: []Route{
				{
					ID: "route-1", Name: "Test Route", Service: "svc-1",
					Paths: []string{"/api/*"}, Methods: []string{"GET"},
					Plugins: []PluginConfig{{Name: "rate-limit", Enabled: &enabled}},
				},
			},
			Upstreams: []Upstream{
				{
					ID: "up-1", Name: "Test Upstream", Algorithm: "round_robin",
					Targets: []UpstreamTarget{{ID: "t-1", Address: "localhost:8080"}},
				},
			},
			Consumers: []Consumer{
				{
					ID: "consumer-1", Name: "Test",
					APIKeys:   []ConsumerAPIKey{{Key: "ck_live_test"}},
					ACLGroups: []string{"admin"},
					Metadata:  map[string]any{"env": "test"},
				},
			},
			Audit: AuditConfig{
				RouteRetentionDays: map[string]int{"route-1": 90},
			},
			Billing: BillingConfig{
				DefaultCost:       10,
				RouteCosts:        map[string]int64{"route-1": 5},
				MethodMultipliers: map[string]float64{"GET": 1.0},
			},
			Auth: AuthConfig{
				APIKey: APIKeyAuthConfig{
					KeyNames:    []string{"X-API-Key"},
					QueryNames:  []string{"api_key"},
					CookieNames: []string{"auth_token"},
				},
			},
		}
		got := CloneConfig(src)

		if got == src {
			t.Error("CloneConfig should return a different pointer")
		}
		if len(got.Services) != 1 || got.Services[0].ID != "svc-1" {
			t.Errorf("unexpected services: %+v", got.Services)
		}
		if len(got.Routes) != 1 || len(got.Routes[0].Plugins) != 1 {
			t.Errorf("unexpected routes/plugins: %+v", got.Routes)
		}
		if len(got.Upstreams) != 1 || len(got.Upstreams[0].Targets) != 1 {
			t.Errorf("unexpected upstreams: %+v", got.Upstreams)
		}
		if len(got.Consumers) != 1 || got.Consumers[0].Metadata["env"] != "test" {
			t.Errorf("unexpected consumers: %+v", got.Consumers)
		}
		if got.Billing.RouteCosts["route-1"] != 5 {
			t.Errorf("unexpected billing: %+v", got.Billing)
		}
		if len(got.Auth.APIKey.KeyNames) != 1 {
			t.Errorf("unexpected auth: %+v", got.Auth.APIKey)
		}

		// Verify mutation isolation
		got.Services[0].ID = "mutated"
		if src.Services[0].ID == "mutated" {
			t.Error("CloneConfig should be a deep copy, mutations should not affect source")
		}
	})
}

func TestClonePluginConfigs(t *testing.T) {
	t.Parallel()

	t.Run("nil returns nil", func(t *testing.T) {
		if got := ClonePluginConfigs(nil); got != nil {
			t.Error("expected nil for nil input")
		}
	})

	t.Run("empty returns nil", func(t *testing.T) {
		if got := ClonePluginConfigs([]PluginConfig{}); got != nil {
			t.Error("expected nil for empty input")
		}
	})

	t.Run("deep copy", func(t *testing.T) {
		enabled := true
		src := []PluginConfig{{
			Name:    "test",
			Enabled: &enabled,
			Config:  map[string]any{"key": "value"},
		}}
		got := ClonePluginConfigs(src)
		if len(got) != 1 {
			t.Fatalf("expected 1 element, got %d", len(got))
		}
		if got[0].Enabled == src[0].Enabled {
			t.Error("Enabled pointer should be different")
		}
		if got[0].Config["key"] != "value" {
			t.Error("Config value should be copied")
		}
		got[0].Config["key"] = "mutated"
		if src[0].Config["key"] == "mutated" {
			t.Error("mutation should not affect source")
		}
	})
}

func TestCloneBillingConfig(t *testing.T) {
	t.Parallel()

	src := BillingConfig{
		Enabled:           true,
		DefaultCost:       10,
		RouteCosts:        map[string]int64{"r1": 5},
		MethodMultipliers: map[string]float64{"GET": 1.5},
		TestModeEnabled:   true,
	}
	got := CloneBillingConfig(src)

	if got.RouteCosts["r1"] != 5 {
		t.Errorf("unexpected RouteCosts: %+v", got.RouteCosts)
	}
	if got.MethodMultipliers["GET"] != 1.5 {
		t.Errorf("unexpected MethodMultipliers: %+v", got.MethodMultipliers)
	}

	// Mutation isolation
	got.RouteCosts["r1"] = 99
	if src.RouteCosts["r1"] == 99 {
		t.Error("mutation should not affect source")
	}
}

func TestCloneInt64Map(t *testing.T) {
	t.Parallel()

	t.Run("empty returns non-nil", func(t *testing.T) {
		got := CloneInt64Map(map[string]int64{})
		if got == nil {
			t.Error("expected non-nil empty map")
		}
	})

	t.Run("nil returns non-nil empty", func(t *testing.T) {
		got := CloneInt64Map(nil)
		if got == nil || len(got) != 0 {
			t.Error("expected non-nil empty map for nil input")
		}
	})

	t.Run("deep copy", func(t *testing.T) {
		src := map[string]int64{"a": 1, "b": 2}
		got := CloneInt64Map(src)
		if got["a"] != 1 || got["b"] != 2 {
			t.Errorf("unexpected values: %+v", got)
		}
		got["a"] = 99
		if src["a"] == 99 {
			t.Error("mutation should not affect source")
		}
	})
}

func TestCloneFloat64Map(t *testing.T) {
	t.Parallel()

	t.Run("empty returns non-nil", func(t *testing.T) {
		got := CloneFloat64Map(map[string]float64{})
		if got == nil {
			t.Error("expected non-nil empty map")
		}
	})

	t.Run("nil returns non-nil empty", func(t *testing.T) {
		got := CloneFloat64Map(nil)
		if got == nil || len(got) != 0 {
			t.Error("expected non-nil empty map for nil input")
		}
	})

	t.Run("deep copy", func(t *testing.T) {
		src := map[string]float64{"a": 1.5, "b": 2.5}
		got := CloneFloat64Map(src)
		if got["a"] != 1.5 || got["b"] != 2.5 {
			t.Errorf("unexpected values: %+v", got)
		}
		got["a"] = 99
		if src["a"] == 99 {
			t.Error("mutation should not affect source")
		}
	})
}

func TestCloneAnyMap(t *testing.T) {
	t.Parallel()

	t.Run("empty returns non-nil", func(t *testing.T) {
		got := CloneAnyMap(map[string]any{})
		if got == nil {
			t.Error("expected non-nil empty map")
		}
	})

	t.Run("nil returns non-nil empty", func(t *testing.T) {
		got := CloneAnyMap(nil)
		if got == nil || len(got) != 0 {
			t.Error("expected non-nil empty map for nil input")
		}
	})

	t.Run("deep copy", func(t *testing.T) {
		src := map[string]any{"str": "hello", "num": 42}
		got := CloneAnyMap(src)
		if got["str"] != "hello" || got["num"] != 42 {
			t.Errorf("unexpected values: %+v", got)
		}
		got["str"] = "mutated"
		if src["str"] == "mutated" {
			t.Error("mutation should not affect source")
		}
	})
}
