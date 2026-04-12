package plugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestNewPluginReloader_Disabled(t *testing.T) {
	config := HotReloadConfig{
		Enabled: false,
	}
	registry := NewRegistry()

	reloader, err := NewPluginReloader(config, registry)
	if err != nil {
		t.Fatalf("NewPluginReloader() error = %v", err)
	}
	if reloader == nil {
		t.Fatal("NewPluginReloader() returned nil")
	}
	if reloader.config.Enabled {
		t.Error("Enabled should be false")
	}
	if reloader.watcher != nil {
		t.Error("watcher should be nil when disabled")
	}
}

func TestNewPluginReloader_Enabled(t *testing.T) {
	tmpDir := t.TempDir()
	config := HotReloadConfig{
		Enabled:  true,
		WatchDir: tmpDir,
		Patterns: []string{"*.lua"},
	}
	registry := NewRegistry()

	reloader, err := NewPluginReloader(config, registry)
	if err != nil {
		t.Fatalf("NewPluginReloader() error = %v", err)
	}
	if reloader == nil {
		t.Fatal("NewPluginReloader() returned nil")
	}
	if !reloader.config.Enabled {
		t.Error("Enabled should be true")
	}
	if reloader.watcher == nil {
		t.Error("watcher should be initialized")
	}
	if reloader.handlers == nil {
		t.Error("handlers map should be initialized")
	}

	reloader.Stop()
}

func TestNewPluginReloader_InvalidDir(t *testing.T) {
	config := HotReloadConfig{
		Enabled:  true,
		WatchDir: "/nonexistent/directory/that/does/not/exist",
	}
	registry := NewRegistry()

	_, err := NewPluginReloader(config, registry)
	if err == nil {
		t.Error("NewPluginReloader() should return error for invalid directory")
	}
}

func TestPluginReloader_RegisterHandler(t *testing.T) {
	config := HotReloadConfig{Enabled: false}
	registry := NewRegistry()
	reloader, _ := NewPluginReloader(config, registry)

	// Initialize handlers map if nil (happens when disabled)
	if reloader.handlers == nil {
		reloader.handlers = make(map[string]ReloadHandler)
	}

	handler := func(name string, content []byte) error {
		return nil
	}

	reloader.RegisterHandler("test-plugin", handler)

	reloader.mu.RLock()
	_, exists := reloader.handlers["test-plugin"]
	reloader.mu.RUnlock()

	if !exists {
		t.Error("Handler should be registered")
	}
}

func TestPluginReloader_UnregisterHandler(t *testing.T) {
	config := HotReloadConfig{Enabled: false}
	registry := NewRegistry()
	reloader, _ := NewPluginReloader(config, registry)

	// Initialize handlers map if nil
	if reloader.handlers == nil {
		reloader.handlers = make(map[string]ReloadHandler)
	}

	handler := func(name string, content []byte) error {
		return nil
	}

	reloader.RegisterHandler("test-plugin", handler)
	reloader.UnregisterHandler("test-plugin")

	reloader.mu.RLock()
	_, exists := reloader.handlers["test-plugin"]
	reloader.mu.RUnlock()

	if exists {
		t.Error("Handler should be unregistered")
	}
}

func TestPluginReloader_matchesPattern(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		path     string
		want     bool
	}{
		{
			name:     "no patterns",
			patterns: []string{},
			path:     "test.lua",
			want:     true,
		},
		{
			name:     "matches lua",
			patterns: []string{"*.lua"},
			path:     "test.lua",
			want:     true,
		},
		{
			name:     "does not match",
			patterns: []string{"*.lua"},
			path:     "test.txt",
			want:     false,
		},
		{
			name:     "multiple patterns match",
			patterns: []string{"*.lua", "*.wasm"},
			path:     "test.wasm",
			want:     true,
		},
		{
			name:     "multiple patterns no match",
			patterns: []string{"*.lua", "*.wasm"},
			path:     "test.txt",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := HotReloadConfig{
				Enabled:  false,
				Patterns: tt.patterns,
			}
			registry := NewRegistry()
			reloader, _ := NewPluginReloader(config, registry)

			got := reloader.matchesPattern(tt.path)
			if got != tt.want {
				t.Errorf("matchesPattern() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPluginReloader_Stop(t *testing.T) {
	tmpDir := t.TempDir()
	config := HotReloadConfig{
		Enabled:  true,
		WatchDir: tmpDir,
	}
	registry := NewRegistry()

	reloader, _ := NewPluginReloader(config, registry)

	// Stop should not panic
	reloader.Stop()
}

func TestPluginReloader_WatchFile(t *testing.T) {
	tmpDir := t.TempDir()
	config := HotReloadConfig{
		Enabled:  true,
		WatchDir: tmpDir,
	}
	registry := NewRegistry()

	reloader, _ := NewPluginReloader(config, registry)
	defer reloader.Stop()

	testFile := filepath.Join(tmpDir, "test.lua")
	_ = os.WriteFile(testFile, []byte("test content"), 0644)

	err := reloader.WatchFile(testFile)
	if err != nil {
		t.Errorf("WatchFile() error = %v", err)
	}
}

func TestPluginReloader_WatchFile_Disabled(t *testing.T) {
	config := HotReloadConfig{Enabled: false}
	registry := NewRegistry()

	reloader, _ := NewPluginReloader(config, registry)

	err := reloader.WatchFile("/some/path")
	if err != nil {
		t.Error("WatchFile should return nil when disabled")
	}
}

func TestPluginReloader_UnwatchFile(t *testing.T) {
	tmpDir := t.TempDir()
	config := HotReloadConfig{
		Enabled:  true,
		WatchDir: tmpDir,
	}
	registry := NewRegistry()

	reloader, _ := NewPluginReloader(config, registry)
	defer reloader.Stop()

	testFile := filepath.Join(tmpDir, "test.lua")
	_ = os.WriteFile(testFile, []byte("test content"), 0644)
	_ = reloader.WatchFile(testFile)

	err := reloader.UnwatchFile(testFile)
	if err != nil {
		t.Errorf("UnwatchFile() error = %v", err)
	}
}

func TestPluginReloader_ReloadPlugin(t *testing.T) {
	tmpDir := t.TempDir()
	config := HotReloadConfig{
		Enabled:  true,
		WatchDir: tmpDir,
	}
	registry := NewRegistry()

	reloader, _ := NewPluginReloader(config, registry)
	defer reloader.Stop()

	// Create plugin file
	pluginFile := filepath.Join(tmpDir, "myplugin.lua")
	_ = os.WriteFile(pluginFile, []byte("plugin content"), 0644)

	// Register handler
	handlerCalled := false
	handler := func(name string, content []byte) error {
		handlerCalled = true
		return nil
	}
	reloader.RegisterHandler("myplugin", handler)

	err := reloader.ReloadPlugin("myplugin")
	if err != nil {
		t.Errorf("ReloadPlugin() error = %v", err)
	}
	if !handlerCalled {
		t.Error("Handler should be called")
	}
}

func TestPluginReloader_ReloadPlugin_NoHandler(t *testing.T) {
	config := HotReloadConfig{Enabled: false}
	registry := NewRegistry()

	reloader, _ := NewPluginReloader(config, registry)

	err := reloader.ReloadPlugin("unknown")
	if err == nil {
		t.Error("ReloadPlugin should return error for unknown plugin")
	}
}

func TestNewDynamicPluginManager(t *testing.T) {
	reloader := &PluginReloader{config: HotReloadConfig{Enabled: false}}
	manager := NewDynamicPluginManager(reloader)

	if manager == nil {
		t.Fatal("NewDynamicPluginManager() returned nil")
	}
	if manager.plugins == nil {
		t.Error("plugins map not initialized")
	}
	if manager.reloader != reloader {
		t.Error("reloader not set correctly")
	}
}

func TestDynamicPluginManager_LoadPlugin(t *testing.T) {
	manager := NewDynamicPluginManager(nil)

	err := manager.LoadPlugin("test-plugin", []byte("plugin content"))
	if err != nil {
		t.Errorf("LoadPlugin() error = %v", err)
	}

	plugin, exists := manager.GetPlugin("test-plugin")
	if !exists {
		t.Error("Plugin should exist after loading")
	}
	if plugin.Name != "test-plugin" {
		t.Errorf("Name = %v, want test-plugin", plugin.Name)
	}
	if string(plugin.Content) != "plugin content" {
		t.Errorf("Content = %v, want plugin content", string(plugin.Content))
	}
	if plugin.Status != "loaded" {
		t.Errorf("Status = %v, want loaded", plugin.Status)
	}
	if plugin.LoadedAt.IsZero() {
		t.Error("LoadedAt should be set")
	}
}

func TestDynamicPluginManager_UnloadPlugin(t *testing.T) {
	manager := NewDynamicPluginManager(nil)

	_ = manager.LoadPlugin("test-plugin", []byte("content"))
	err := manager.UnloadPlugin("test-plugin")
	if err != nil {
		t.Errorf("UnloadPlugin() error = %v", err)
	}

	_, exists := manager.GetPlugin("test-plugin")
	if exists {
		t.Error("Plugin should not exist after unloading")
	}
}

func TestDynamicPluginManager_UpdatePlugin(t *testing.T) {
	manager := NewDynamicPluginManager(nil)

	// Load initial
	_ = manager.LoadPlugin("test-plugin", []byte("old content"))

	// Update
	err := manager.UpdatePlugin("test-plugin", []byte("new content"))
	if err != nil {
		t.Errorf("UpdatePlugin() error = %v", err)
	}

	plugin, _ := manager.GetPlugin("test-plugin")
	if string(plugin.Content) != "new content" {
		t.Errorf("Content = %v, want new content", string(plugin.Content))
	}
	if plugin.Status != "updated" {
		t.Errorf("Status = %v, want updated", plugin.Status)
	}
}

func TestDynamicPluginManager_UpdatePlugin_NewPlugin(t *testing.T) {
	manager := NewDynamicPluginManager(nil)

	// Update non-existent plugin should load it
	err := manager.UpdatePlugin("new-plugin", []byte("content"))
	if err != nil {
		t.Errorf("UpdatePlugin() error = %v", err)
	}

	_, exists := manager.GetPlugin("new-plugin")
	if !exists {
		t.Error("Plugin should exist after update")
	}
}

func TestDynamicPluginManager_ListPlugins(t *testing.T) {
	manager := NewDynamicPluginManager(nil)

	_ = manager.LoadPlugin("plugin1", []byte("content1"))
	_ = manager.LoadPlugin("plugin2", []byte("content2"))

	plugins := manager.ListPlugins()
	if len(plugins) != 2 {
		t.Errorf("len(plugins) = %v, want 2", len(plugins))
	}
}

func TestDynamicPluginManager_SetPluginError(t *testing.T) {
	manager := NewDynamicPluginManager(nil)

	_ = manager.LoadPlugin("test-plugin", []byte("content"))
	manager.SetPluginError("test-plugin", fmt.Errorf("test error"))

	plugin, _ := manager.GetPlugin("test-plugin")
	if plugin.Status != "error" {
		t.Errorf("Status = %v, want error", plugin.Status)
	}
	if plugin.Error != "test error" {
		t.Errorf("Error = %v, want test error", plugin.Error)
	}
}

func TestDynamicPluginManager_ClearPluginError(t *testing.T) {
	manager := NewDynamicPluginManager(nil)

	_ = manager.LoadPlugin("test-plugin", []byte("content"))
	manager.SetPluginError("test-plugin", fmt.Errorf("test error"))
	manager.ClearPluginError("test-plugin")

	plugin, _ := manager.GetPlugin("test-plugin")
	if plugin.Status != "loaded" {
		t.Errorf("Status = %v, want loaded", plugin.Status)
	}
	if plugin.Error != "" {
		t.Errorf("Error = %v, want empty", plugin.Error)
	}
}

func TestWithPluginManager(t *testing.T) {
	manager := NewDynamicPluginManager(nil)

	ctx := WithPluginManager(context.Background(), manager)
	retrieved := GetPluginManagerFromContext(ctx)

	if retrieved != manager {
		t.Error("GetPluginManagerFromContext() returned different manager")
	}
}

func TestGetPluginManagerFromContext_NotFound(t *testing.T) {
	retrieved := GetPluginManagerFromContext(context.Background())
	if retrieved != nil {
		t.Error("GetPluginManagerFromContext() should return nil when not found")
	}
}

func TestHotReloadConfig(t *testing.T) {
	config := HotReloadConfig{
		Enabled:  true,
		WatchDir: "/plugins",
		Patterns: []string{"*.lua", "*.wasm"},
	}

	if !config.Enabled {
		t.Error("Enabled should be true")
	}
	if config.WatchDir != "/plugins" {
		t.Errorf("WatchDir = %v, want /plugins", config.WatchDir)
	}
	if len(config.Patterns) != 2 {
		t.Errorf("len(Patterns) = %v, want 2", len(config.Patterns))
	}
}
