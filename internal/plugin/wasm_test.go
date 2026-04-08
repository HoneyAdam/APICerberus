package plugin

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestDefaultWASMConfig(t *testing.T) {
	cfg := DefaultWASMConfig()

	if cfg.Enabled {
		t.Error("Expected WASM to be disabled by default")
	}

	if cfg.ModuleDir != "./plugins/wasm" {
		t.Errorf("Expected module dir './plugins/wasm', got %s", cfg.ModuleDir)
	}

	if cfg.MaxMemory != 128*1024*1024 {
		t.Errorf("Expected max memory 128MB, got %d", cfg.MaxMemory)
	}

	if cfg.MaxExecution != 30*time.Second {
		t.Errorf("Expected max execution 30s, got %v", cfg.MaxExecution)
	}

	if cfg.AllowFilesystem {
		t.Error("Expected filesystem access to be disabled by default")
	}
}

func TestNewWASMRuntime_Disabled(t *testing.T) {
	cfg := WASMConfig{Enabled: false}

	runtime, err := NewWASMRuntime(cfg)
	if err != nil {
		t.Errorf("NewWASMRuntime() error = %v", err)
	}
	if runtime != nil {
		t.Error("Expected nil runtime when disabled")
	}
}

func TestNewWASMRuntime_Enabled(t *testing.T) {
	cfg := DefaultWASMConfig()
	cfg.Enabled = true

	runtime, err := NewWASMRuntime(cfg)
	if err != nil {
		t.Fatalf("NewWASMRuntime() error = %v", err)
	}
	if runtime == nil {
		t.Fatal("Expected non-nil runtime")
	}

	if !runtime.IsEnabled() {
		t.Error("Expected runtime to be enabled")
	}
}

func TestWASMRuntime_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		runtime  *WASMRuntime
		expected bool
	}{
		{
			name:     "nil runtime",
			runtime:  nil,
			expected: false,
		},
		{
			name: "disabled runtime",
			runtime: &WASMRuntime{
				config: WASMConfig{Enabled: false},
			},
			expected: false,
		},
		{
			name: "enabled runtime",
			runtime: &WASMRuntime{
				config: WASMConfig{Enabled: true},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.runtime.IsEnabled(); got != tt.expected {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestWASMRuntime_LoadModule_NotFound(t *testing.T) {
	cfg := DefaultWASMConfig()
	cfg.Enabled = true

	runtime, _ := NewWASMRuntime(cfg)

	_, err := runtime.LoadModule("test", "/nonexistent/module.wasm", nil)
	if err == nil {
		t.Error("Expected error when loading non-existent module")
	}
}

func TestWASMModule_Accessors(t *testing.T) {
	module := &WASMModule{
		id:       "test-module",
		name:     "Test Module",
		version:  "1.2.3",
		phase:    PhasePreAuth,
		priority: 50,
		loaded:   true,
	}

	if module.ID() != "test-module" {
		t.Errorf("ID() = %s, want test-module", module.ID())
	}

	if module.Name() != "Test Module" {
		t.Errorf("Name() = %s, want 'Test Module'", module.Name())
	}

	if module.Version() != "1.2.3" {
		t.Errorf("Version() = %s, want 1.2.3", module.Version())
	}

	if module.Phase() != PhasePreAuth {
		t.Errorf("Phase() = %v, want PhasePreAuth", module.Phase())
	}

	if module.Priority() != 50 {
		t.Errorf("Priority() = %d, want 50", module.Priority())
	}
}

func TestWASMModule_Nil(t *testing.T) {
	var module *WASMModule

	if module.ID() != "" {
		t.Error("Expected empty ID for nil module")
	}

	if module.Name() != "" {
		t.Error("Expected empty Name for nil module")
	}

	if module.Version() != "" {
		t.Error("Expected empty Version for nil module")
	}

	if module.Phase() != PhasePreProxy {
		t.Error("Expected PhasePreProxy for nil module")
	}

	if module.Priority() != 100 {
		t.Error("Expected priority 100 for nil module")
	}
}

func TestWASMModule_Execute_NotLoaded(t *testing.T) {
	module := &WASMModule{
		id:     "test",
		loaded: false,
	}

	_, err := module.Execute(nil)
	if err == nil {
		t.Error("Expected error when executing unloaded module")
	}
}

func TestWASMPluginManager(t *testing.T) {
	cfg := DefaultWASMConfig()
	cfg.Enabled = true

	manager, err := NewWASMPluginManager(cfg)
	if err != nil {
		t.Fatalf("NewWASMPluginManager() error = %v", err)
	}
	defer manager.Close()

	if !manager.IsEnabled() {
		t.Error("Expected manager to be enabled")
	}

	modules := manager.ListModules()
	if len(modules) != 0 {
		t.Errorf("Expected 0 modules, got %d", len(modules))
	}

	_, ok := manager.GetModule("nonexistent")
	if ok {
		t.Error("Expected false for non-existent module")
	}
}

func TestWASMPluginManager_LoadUnload(t *testing.T) {
	cfg := DefaultWASMConfig()
	cfg.Enabled = true

	manager, _ := NewWASMPluginManager(cfg)
	defer manager.Close()

	tmpDir := t.TempDir()
	wasmPath := filepath.Join(tmpDir, "test.wasm")
	wasmMagic := []byte{0x00, 0x61, 0x73, 0x6d}
	if err := os.WriteFile(wasmPath, wasmMagic, 0644); err != nil {
		t.Fatalf("Failed to create test wasm file: %v", err)
	}

	pluginConfig := map[string]interface{}{
		"name":     "Test Plugin",
		"version":  "1.0.0",
		"phase":    "pre-proxy",
		"priority": 50,
	}

	err := manager.LoadModule("test", wasmPath, pluginConfig)
	if err != nil {
		t.Errorf("LoadModule() error = %v", err)
	}

	module, ok := manager.GetModule("test")
	if !ok {
		t.Fatal("Expected module to be found")
	}

	if module.Name() != "Test Plugin" {
		t.Errorf("Expected name 'Test Plugin', got %s", module.Name())
	}

	err = manager.UnloadModule("test")
	if err != nil {
		t.Errorf("UnloadModule() error = %v", err)
	}

	_, ok = manager.GetModule("test")
	if ok {
		t.Error("Expected module to be unloaded")
	}
}

func TestWASMHostFunctions(t *testing.T) {
	host := NewWASMHostFunctions()
	if host == nil {
		t.Fatal("Expected non-nil host functions")
	}

	host.Log("info", "test message")

	host.GetHeader(nil, "X-Test")
	host.SetHeader(nil, "X-Test", "value")
	host.GetMetadata(nil, "key")
	host.SetMetadata(nil, "key", "value")
	host.Abort(nil, "reason")
}

func TestValidateWASMModule_NotFound(t *testing.T) {
	err := ValidateWASMModule("/nonexistent/module.wasm")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

func TestValidateWASMModule_InvalidMagic(t *testing.T) {
	tmpDir := t.TempDir()
	wasmPath := filepath.Join(tmpDir, "invalid.wasm")

	if err := os.WriteFile(wasmPath, []byte("NOTWASM"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	err := ValidateWASMModule(wasmPath)
	if err == nil {
		t.Error("Expected error for invalid magic")
	}
}

func TestValidateWASMModule_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	wasmPath := filepath.Join(tmpDir, "valid.wasm")

	wasmMagic := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	if err := os.WriteFile(wasmPath, wasmMagic, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	err := ValidateWASMModule(wasmPath)
	if err != nil {
		t.Errorf("ValidateWASMModule() error = %v", err)
	}
}

func TestValidateWASMModule_TooLarge(t *testing.T) {
	tmpDir := t.TempDir()
	wasmPath := filepath.Join(tmpDir, "large.wasm")

	data := make([]byte, 101*1024*1024)
	data[0] = 0x00
	data[1] = 0x61
	data[2] = 0x73
	data[3] = 0x6d

	if err := os.WriteFile(wasmPath, data, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	err := ValidateWASMModule(wasmPath)
	if err == nil {
		t.Error("Expected error for large file")
	}
}

func TestBuildWASMPlugin(t *testing.T) {
	spec := config.PluginConfig{
		Name: "wasm-test",
		Config: map[string]any{
			"module_id":   "test",
			"module_path": "/test/module.wasm",
			"phase":       "pre-auth",
			"priority":    50,
		},
	}

	plugin, err := buildWASMPlugin(spec, BuilderContext{})
	if err != nil {
		t.Fatalf("buildWASMPlugin() error = %v", err)
	}

	if plugin.Name() != "wasm-test" {
		t.Errorf("Name() = %s, want wasm-test", plugin.Name())
	}

	if plugin.Phase() != PhasePreAuth {
		t.Errorf("Phase() = %v, want PhasePreAuth", plugin.Phase())
	}

	if plugin.Priority() != 50 {
		t.Errorf("Priority() = %d, want 50", plugin.Priority())
	}
}

func TestBuildWASMPlugin_NoModuleID(t *testing.T) {
	spec := config.PluginConfig{
		Name:   "wasm-test",
		Config: map[string]any{},
	}

	_, err := buildWASMPlugin(spec, BuilderContext{})
	if err == nil {
		t.Error("Expected error when module_id is missing")
	}
}

func TestBuildWASMPlugin_Defaults(t *testing.T) {
	spec := config.PluginConfig{
		Name: "wasm-test",
		Config: map[string]any{
			"module_id": "test",
		},
	}

	plugin, err := buildWASMPlugin(spec, BuilderContext{})
	if err != nil {
		t.Fatalf("buildWASMPlugin() error = %v", err)
	}

	if plugin.Phase() != PhasePreProxy {
		t.Errorf("Phase() = %v, want PhasePreProxy", plugin.Phase())
	}

	if plugin.Priority() != 100 {
		t.Errorf("Priority() = %d, want 100", plugin.Priority())
	}
}
