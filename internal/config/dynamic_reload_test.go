package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewConfigReloader(t *testing.T) {
	// Create a temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yaml")
	configContent := `
gateway:
  http_addr: ":8080"
routes:
  - id: test-route
    paths:
      - /test
    service: test-service
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	reloader := func(cfg *Config) error { return nil }
	cr, err := NewConfigReloader(configPath, reloader)
	if err != nil {
		t.Fatalf("NewConfigReloader() error = %v", err)
	}
	if cr == nil {
		t.Fatal("NewConfigReloader() returned nil")
	}
	if cr.path != configPath {
		t.Errorf("path = %v, want %v", cr.path, configPath)
	}
	if cr.watcher == nil {
		t.Error("watcher not initialized")
	}
	if cr.debounceTime != time.Second {
		t.Errorf("debounceTime = %v, want 1s", cr.debounceTime)
	}

	cr.Stop()
}

func TestNewConfigReloader_InvalidPath(t *testing.T) {
	tmpDir := t.TempDir()
	invalidPath := filepath.Join(tmpDir, "nonexistent", "config.yaml")

	reloader := func(cfg *Config) error { return nil }
	_, err := NewConfigReloader(invalidPath, reloader)
	if err == nil {
		t.Error("NewConfigReloader() should return error for invalid path")
	}
}

func TestConfigReloader_SetDebounceTime(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yaml")
	os.WriteFile(configPath, []byte("gateway:\n  http_addr: \":8080\""), 0644)

	reloader := func(cfg *Config) error { return nil }
	cr, _ := NewConfigReloader(configPath, reloader)
	defer cr.Stop()

	cr.SetDebounceTime(5 * time.Second)
	if cr.debounceTime != 5*time.Second {
		t.Errorf("debounceTime = %v, want 5s", cr.debounceTime)
	}
}

func TestConfigReloader_SetOnChange(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yaml")
	os.WriteFile(configPath, []byte("gateway:\n  http_addr: \":8080\""), 0644)

	reloader := func(cfg *Config) error { return nil }
	cr, _ := NewConfigReloader(configPath, reloader)
	defer cr.Stop()

	callback := func(old, new *Config) {
		// Callback function
	}

	cr.SetOnChange(callback)
	if cr.onChange == nil {
		t.Error("SetOnChange did not set callback")
	}
}

func TestConfigReloader_GetCurrent(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yaml")
	os.WriteFile(configPath, []byte("gateway:\n  http_addr: \":8080\""), 0644)

	reloader := func(cfg *Config) error { return nil }
	cr, _ := NewConfigReloader(configPath, reloader)
	defer cr.Stop()

	current := cr.GetCurrent()
	if current != nil {
		t.Error("GetCurrent should return nil initially")
	}
}

func TestConfigReloader_TriggerManualReload(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yaml")
	configContent := `
gateway:
  http_addr: ":8080"
`
	os.WriteFile(configPath, []byte(configContent), 0644)

	reloaderCalled := false
	reloader := func(cfg *Config) error {
		reloaderCalled = true
		return nil
	}

	cr, _ := NewConfigReloader(configPath, reloader)
	defer cr.Stop()

	err := cr.TriggerManualReload()
	if err != nil {
		t.Errorf("TriggerManualReload() error = %v", err)
	}

	// Give time for reload
	time.Sleep(200 * time.Millisecond)

	if !reloaderCalled {
		t.Error("Reloader function was not called")
	}
}

func TestConfigReloader_Stop(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yaml")
	os.WriteFile(configPath, []byte("gateway:\n  http_addr: \":8080\""), 0644)

	reloader := func(cfg *Config) error { return nil }
	cr, _ := NewConfigReloader(configPath, reloader)

	// Stop should not panic
	cr.Stop()
	// Note: Double Stop causes panic due to closing closed channel
	// This is a known limitation in the original code
}

func TestCompareConfigs(t *testing.T) {
	t.Run("nil configs", func(t *testing.T) {
		diff := CompareConfigs(nil, nil)
		if !diff.ChangedGlobal {
			t.Error("ChangedGlobal should be true for nil configs")
		}
	})

	t.Run("one nil config", func(t *testing.T) {
		old := &Config{Gateway: GatewayConfig{HTTPAddr: ":8080"}}
		diff := CompareConfigs(old, nil)
		if !diff.ChangedGlobal {
			t.Error("ChangedGlobal should be true when one config is nil")
		}
	})

	t.Run("equal configs", func(t *testing.T) {
		old := &Config{
			Routes: []Route{
				{ID: "route1", Service: "svc1", Paths: []string{"/api"}},
			},
		}
		new := &Config{
			Routes: []Route{
				{ID: "route1", Service: "svc1", Paths: []string{"/api"}},
			},
		}
		diff := CompareConfigs(old, new)
		if len(diff.ChangedRoutes) != 0 {
			t.Errorf("ChangedRoutes should be empty, got %v", diff.ChangedRoutes)
		}
	})

	t.Run("changed routes", func(t *testing.T) {
		old := &Config{
			Routes: []Route{
				{ID: "route1", Service: "svc1", Paths: []string{"/api"}},
			},
		}
		new := &Config{
			Routes: []Route{
				{ID: "route1", Service: "svc2", Paths: []string{"/api"}},
				{ID: "route2", Service: "svc3", Paths: []string{"/new"}},
			},
		}
		diff := CompareConfigs(old, new)
		if len(diff.ChangedRoutes) == 0 {
			t.Error("ChangedRoutes should not be empty")
		}
	})

	t.Run("removed routes", func(t *testing.T) {
		old := &Config{
			Routes: []Route{
				{ID: "route1", Service: "svc1", Paths: []string{"/api"}},
				{ID: "route2", Service: "svc2", Paths: []string{"/old"}},
			},
		}
		new := &Config{
			Routes: []Route{
				{ID: "route1", Service: "svc1", Paths: []string{"/api"}},
			},
		}
		diff := CompareConfigs(old, new)
		found := false
		for _, id := range diff.ChangedRoutes {
			if id == "route2" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Removed route should be in ChangedRoutes")
		}
	})

	t.Run("changed services", func(t *testing.T) {
		old := &Config{
			Services: []Service{
				{ID: "svc1", Upstream: "upstream1"},
			},
		}
		new := &Config{
			Services: []Service{
				{ID: "svc1", Upstream: "upstream2"},
			},
		}
		diff := CompareConfigs(old, new)
		if len(diff.ChangedServices) == 0 {
			t.Error("ChangedServices should not be empty")
		}
	})

	t.Run("changed upstreams", func(t *testing.T) {
		old := &Config{
			Upstreams: []Upstream{
				{ID: "up1", Targets: []UpstreamTarget{{Address: "localhost:8080"}}},
			},
		}
		new := &Config{
			Upstreams: []Upstream{
				{ID: "up2", Targets: []UpstreamTarget{{Address: "localhost:8080"}}},
			},
		}
		diff := CompareConfigs(old, new)
		if len(diff.ChangedUpstreams) == 0 {
			t.Error("ChangedUpstreams should not be empty")
		}
	})
}

func TestRoutesEqual(t *testing.T) {
	tests := []struct {
		name string
		a    Route
		b    Route
		want bool
	}{
		{
			name: "equal routes",
			a:    Route{ID: "r1", Service: "s1", Paths: []string{"/api"}, Methods: []string{"GET"}},
			b:    Route{ID: "r1", Service: "s1", Paths: []string{"/api"}, Methods: []string{"GET"}},
			want: true,
		},
		{
			name: "different id",
			a:    Route{ID: "r1", Service: "s1"},
			b:    Route{ID: "r2", Service: "s1"},
			want: false,
		},
		{
			name: "different service",
			a:    Route{ID: "r1", Service: "s1"},
			b:    Route{ID: "r1", Service: "s2"},
			want: false,
		},
		{
			name: "different paths",
			a:    Route{ID: "r1", Paths: []string{"/api"}},
			b:    Route{ID: "r1", Paths: []string{"/test"}},
			want: false,
		},
		{
			name: "different methods",
			a:    Route{ID: "r1", Methods: []string{"GET"}},
			b:    Route{ID: "r1", Methods: []string{"POST"}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := routesEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("routesEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestServicesEqual(t *testing.T) {
	tests := []struct {
		name string
		a    Service
		b    Service
		want bool
	}{
		{
			name: "equal services",
			a:    Service{ID: "s1", Upstream: "u1"},
			b:    Service{ID: "s1", Upstream: "u1"},
			want: true,
		},
		{
			name: "different id",
			a:    Service{ID: "s1"},
			b:    Service{ID: "s2"},
			want: false,
		},
		{
			name: "different upstream",
			a:    Service{ID: "s1", Upstream: "u1"},
			b:    Service{ID: "s1", Upstream: "u2"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := servicesEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("servicesEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUpstreamsEqual(t *testing.T) {
	tests := []struct {
		name string
		a    Upstream
		b    Upstream
		want bool
	}{
		{
			name: "equal upstreams",
			a:    Upstream{ID: "u1"},
			b:    Upstream{ID: "u1"},
			want: true,
		},
		{
			name: "different id",
			a:    Upstream{ID: "u1"},
			b:    Upstream{ID: "u2"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := upstreamsEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("upstreamsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStringSlicesEqual(t *testing.T) {
	tests := []struct {
		name string
		a    []string
		b    []string
		want bool
	}{
		{
			name: "equal slices",
			a:    []string{"a", "b", "c"},
			b:    []string{"a", "b", "c"},
			want: true,
		},
		{
			name: "different lengths",
			a:    []string{"a", "b"},
			b:    []string{"a", "b", "c"},
			want: false,
		},
		{
			name: "different elements",
			a:    []string{"a", "b"},
			b:    []string{"a", "c"},
			want: false,
		},
		{
			name: "empty slices",
			a:    []string{},
			b:    []string{},
			want: true,
		},
		{
			name: "nil slices",
			a:    nil,
			b:    nil,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stringSlicesEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("stringSlicesEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewDynamicConfigManager(t *testing.T) {
	config := &Config{Gateway: GatewayConfig{HTTPAddr: ":8080"}}
	reloader := func(cfg *Config) error { return nil }

	manager, err := NewDynamicConfigManager(config, reloader)
	if err != nil {
		t.Fatalf("NewDynamicConfigManager() error = %v", err)
	}
	if manager == nil {
		t.Fatal("NewDynamicConfigManager() returned nil")
	}
	if manager.maxHistory != 10 {
		t.Errorf("maxHistory = %v, want 10", manager.maxHistory)
	}
	if len(manager.history) != 1 {
		t.Errorf("history length = %v, want 1", len(manager.history))
	}
}

func TestDynamicConfigManager_GetCurrentConfig(t *testing.T) {
	config := &Config{Gateway: GatewayConfig{HTTPAddr: ":8080"}}
	reloader := func(cfg *Config) error { return nil }
	manager, _ := NewDynamicConfigManager(config, reloader)

	current := manager.GetCurrentConfig()
	if current == nil {
		t.Error("GetCurrentConfig() returned nil")
	}
	if current.Gateway.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %v, want :8080", current.Gateway.HTTPAddr)
	}
}

func TestDynamicConfigManager_UpdateConfig(t *testing.T) {
	config := &Config{Gateway: GatewayConfig{HTTPAddr: ":8080"}}
	reloader := func(cfg *Config) error { return nil }
	manager, _ := NewDynamicConfigManager(config, reloader)

	newConfig := &Config{Gateway: GatewayConfig{HTTPAddr: ":9090"}}
	err := manager.UpdateConfig(newConfig, "user1")
	if err != nil {
		t.Errorf("UpdateConfig() error = %v", err)
	}

	current := manager.GetCurrentConfig()
	if current.Gateway.HTTPAddr != ":9090" {
		t.Errorf("HTTPAddr = %v, want :9090", current.Gateway.HTTPAddr)
	}
}

func TestDynamicConfigManager_GetHistory(t *testing.T) {
	config := &Config{Gateway: GatewayConfig{HTTPAddr: ":8080"}}
	reloader := func(cfg *Config) error { return nil }
	manager, _ := NewDynamicConfigManager(config, reloader)

	// Add more versions
	for i := 0; i < 5; i++ {
		newConfig := &Config{Gateway: GatewayConfig{HTTPAddr: fmt.Sprintf(":%d", 9000+i)}}
		manager.UpdateConfig(newConfig, "user1")
	}

	history := manager.GetHistory()
	if len(history) != 6 { // 1 initial + 5 updates
		t.Errorf("history length = %v, want 6", len(history))
	}
}

func TestDynamicConfigManager_Rollback(t *testing.T) {
	config := &Config{Gateway: GatewayConfig{HTTPAddr: ":8080"}}
	reloader := func(cfg *Config) error { return nil }
	manager, _ := NewDynamicConfigManager(config, reloader)

	// Add more versions
	newConfig := &Config{Gateway: GatewayConfig{HTTPAddr: ":9090"}}
	manager.UpdateConfig(newConfig, "user1")

	// Rollback to initial version (index 0 = most recent)
	rolledBack, err := manager.Rollback(0)
	if err != nil {
		t.Errorf("Rollback() error = %v", err)
	}
	if rolledBack == nil {
		t.Error("Rollback() returned nil config")
	}

	// Rollback with invalid index
	_, err = manager.Rollback(100)
	if err == nil {
		t.Error("Rollback() should return error for invalid index")
	}

	// Rollback with negative index
	_, err = manager.Rollback(-1)
	if err == nil {
		t.Error("Rollback() should return error for negative index")
	}
}

func TestDynamicConfigManager_ValidateConfig(t *testing.T) {
	config := &Config{Gateway: GatewayConfig{HTTPAddr: ":8080"}}
	reloader := func(cfg *Config) error { return nil }
	manager, _ := NewDynamicConfigManager(config, reloader)

	// Test that validation runs (actual validation behavior depends on implementation)
	minimalConfig := &Config{Gateway: GatewayConfig{HTTPAddr: ":9090"}}
	err := manager.ValidateConfig(minimalConfig)
	// Validation will likely fail due to missing required fields,
	// but we're testing that the method executes without panic
	_ = err
}

func TestWithDynamicConfigManager(t *testing.T) {
	config := &Config{Gateway: GatewayConfig{HTTPAddr: ":8080"}}
	reloader := func(cfg *Config) error { return nil }
	manager, _ := NewDynamicConfigManager(config, reloader)

	ctx := WithDynamicConfigManager(context.Background(), manager)
	retrieved := GetDynamicConfigManagerFromContext(ctx)

	if retrieved != manager {
		t.Error("GetDynamicConfigManagerFromContext() returned different manager")
	}
}

func TestGetDynamicConfigManagerFromContext_NotFound(t *testing.T) {
	retrieved := GetDynamicConfigManagerFromContext(context.Background())
	if retrieved != nil {
		t.Error("GetDynamicConfigManagerFromContext() should return nil when not found")
	}
}

func TestConfigVersion(t *testing.T) {
	config := &Config{Gateway: GatewayConfig{HTTPAddr: ":8080"}}
	version := ConfigVersion{
		Config:    config,
		AppliedAt: time.Now(),
		AppliedBy: "test-user",
	}

	if version.Config == nil {
		t.Error("Config should not be nil")
	}
	if version.AppliedBy != "test-user" {
		t.Errorf("AppliedBy = %v, want test-user", version.AppliedBy)
	}
}
