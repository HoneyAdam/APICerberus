package plugin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestWASMConfig_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     WASMConfig
		wantErr bool
	}{
		{"valid defaults", DefaultWASMConfig(), false},
		{"zero max_memory", WASMConfig{Enabled: true, MaxMemory: 0, MaxExecution: time.Second}, true},
		{"negative max_memory", WASMConfig{Enabled: true, MaxMemory: -1, MaxExecution: time.Second}, true},
		{"zero max_execution", WASMConfig{Enabled: true, MaxMemory: 1024, MaxExecution: 0}, true},
		{"negative max_execution", WASMConfig{Enabled: true, MaxMemory: 1024, MaxExecution: -1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultWASMConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultWASMConfig()
	if cfg.Enabled {
		t.Error("default should be disabled")
	}
	if cfg.MaxMemory <= 0 {
		t.Error("max memory should be positive")
	}
	if cfg.MaxExecution <= 0 {
		t.Error("max execution should be positive")
	}
	if cfg.AllowFilesystem {
		t.Error("filesystem should be disabled by default")
	}
}

func TestNewWASMRuntime_Disabled(t *testing.T) {
	t.Parallel()
	rt, err := NewWASMRuntime(WASMConfig{Enabled: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt != nil {
		t.Error("expected nil runtime when disabled")
	}
}

func TestNewWASMRuntime_InvalidConfig(t *testing.T) {
	t.Parallel()
	_, err := NewWASMRuntime(WASMConfig{Enabled: true, MaxMemory: 0, MaxExecution: time.Second})
	if err == nil {
		t.Error("expected error for invalid config")
	}
}

func TestNewWASMRuntime_Enabled(t *testing.T) {
	cfg := WASMConfig{
		Enabled:      true,
		MaxMemory:    10 * 1024 * 1024,
		MaxExecution: 5 * time.Second,
	}
	rt, err := NewWASMRuntime(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer rt.Close()

	if !rt.IsEnabled() {
		t.Error("should be enabled")
	}
}

func TestWASMRuntime_Close_Nil(t *testing.T) {
	t.Parallel()
	var rt *WASMRuntime
	if err := rt.Close(); err != nil {
		t.Errorf("nil Close should not error: %v", err)
	}
}

func TestValidateWASMModule_NotFound(t *testing.T) {
	t.Parallel()
	err := ValidateWASMModule("/nonexistent/path/test.wasm")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestValidateWASMModule_EmptyFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.wasm")
	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	err := ValidateWASMModule(path)
	if err == nil {
		t.Error("expected error for empty file")
	}
}

func TestValidateWASMModule_InvalidMagic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.wasm")
	if err := os.WriteFile(path, []byte("NotWASM"+strings.Repeat("x", 100)), 0644); err != nil {
		t.Fatal(err)
	}
	err := ValidateWASMModule(path)
	if err == nil {
		t.Error("expected error for invalid magic number")
	}
}

func TestValidateWASMModule_TooLarge(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "big.wasm")
	data := append([]byte("\x00asm"), make([]byte, 200)...)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	err := validateWASMModule(path, 100)
	if err == nil {
		t.Error("expected error for oversized file")
	}
}

func TestValidateWASMModule_ValidMagic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "valid.wasm")
	data := append([]byte("\x00asm"), make([]byte, 50)...)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	err := validateWASMModule(path, maxWASMModuleSize)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWASMRuntime_SafeResolvePath_Traversal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rt := &WASMRuntime{
		config: WASMConfig{
			ModuleDir: dir,
		},
	}
	_, err := rt.safeResolvePath("../../../etc/passwd")
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

func TestWASMRuntime_SafeResolvePath_Relative(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rt := &WASMRuntime{
		config: WASMConfig{
			ModuleDir: dir,
		},
	}
	resolved, err := rt.safeResolvePath("test.wasm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(dir, "test.wasm")
	if resolved != expected {
		t.Errorf("resolved = %q, want %q", resolved, expected)
	}
}

func TestToWASMContext_Nil(t *testing.T) {
	t.Parallel()
	wc := ToWASMContext(nil)
	if wc == nil {
		t.Error("expected non-nil WASMContext")
	}
	if wc.Method != "" {
		t.Error("expected empty method for nil context")
	}
}

func TestToWASMContext_ValidRequest(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/test?foo=bar", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Custom", "value")

	ctx := &PipelineContext{
		Request:        req,
		CorrelationID:  "corr-123",
		Consumer:       &config.Consumer{ID: "user-1", Name: "TestUser"},
		Route:          &config.Route{ID: "route-1", Name: "test-route"},
		Service:        &config.Service{Name: "test-svc"},
		Metadata:       map[string]any{"key": "value"},
	}

	wc := ToWASMContext(ctx)
	if wc.Method != http.MethodPost {
		t.Errorf("Method = %q, want %q", wc.Method, http.MethodPost)
	}
	if wc.Path != "/api/v1/test" {
		t.Errorf("Path = %q, want /api/v1/test", wc.Path)
	}
	if wc.Query != "foo=bar" {
		t.Errorf("Query = %q, want foo=bar", wc.Query)
	}
	if wc.ConsumerID != "user-1" {
		t.Errorf("ConsumerID = %q, want user-1", wc.ConsumerID)
	}
	if wc.RouteID != "route-1" {
		t.Errorf("RouteID = %q, want route-1", wc.RouteID)
	}
	if wc.ServiceName != "test-svc" {
		t.Errorf("ServiceName = %q, want test-svc", wc.ServiceName)
	}
	if wc.CorrelationID != "corr-123" {
		t.Errorf("CorrelationID = %q, want corr-123", wc.CorrelationID)
	}
	if wc.Headers["Content-Type"] != "application/json" {
		t.Error("missing Content-Type header")
	}
	if wc.Metadata["key"] != "value" {
		t.Error("missing metadata key")
	}
}

func TestWASMContext_ApplyToContext_Nil(t *testing.T) {
	t.Parallel()
	var wc *WASMContext
	wc.ApplyToContext(nil)

	wc = &WASMContext{}
	wc.ApplyToContext(nil)
}

func TestWASMContext_ApplyToContext(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := &PipelineContext{
		Request:       req,
		CorrelationID: "old-corr",
		Metadata:      map[string]any{},
	}

	wc := &WASMContext{
		Headers:       map[string]string{"X-New": "header"},
		CorrelationID: "new-corr",
		Metadata:      map[string]any{"added": true},
	}
	wc.ApplyToContext(ctx)

	if ctx.Request.Header.Get("X-New") != "header" {
		t.Error("header not applied")
	}
	if ctx.CorrelationID != "new-corr" {
		t.Errorf("CorrelationID = %q, want new-corr", ctx.CorrelationID)
	}
	if ctx.Metadata["added"] != true {
		t.Error("metadata not applied")
	}
}

func TestWASMContext_Serialize_Deserialize(t *testing.T) {
	t.Parallel()
	original := &WASMContext{
		Method:        "GET",
		Path:          "/api/test",
		ConsumerID:    "user-1",
		CorrelationID: "corr-123",
		Headers:       map[string]string{"X-Test": "value"},
		Metadata:      map[string]any{"key": "val"},
	}

	data, err := original.Serialize()
	if err != nil {
		t.Fatalf("Serialize error: %v", err)
	}

	var roundtrip WASMContext
	if err := roundtrip.Deserialize(data); err != nil {
		t.Fatalf("Deserialize error: %v", err)
	}

	if roundtrip.Method != original.Method {
		t.Errorf("Method = %q, want %q", roundtrip.Method, original.Method)
	}
	if roundtrip.Path != original.Path {
		t.Errorf("Path = %q, want %q", roundtrip.Path, original.Path)
	}
	if roundtrip.ConsumerID != original.ConsumerID {
		t.Errorf("ConsumerID = %q, want %q", roundtrip.ConsumerID, original.ConsumerID)
	}
}

func TestWASMGuestRequest_Response_Marshal(t *testing.T) {
	t.Parallel()
	req := WASMGuestRequest{
		Type: "request",
		Context: &WASMContext{
			Method: "GET",
			Path:   "/test",
		},
		Config: map[string]any{"key": "value"},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if !bytes.Contains(data, []byte(`"method":"GET"`)) {
		t.Error("expected method in JSON")
	}

	resp := WASMGuestResponse{
		Handled: true,
		Error:   "",
		Context: &WASMContext{Method: "POST"},
	}
	respData, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if !bytes.Contains(respData, []byte(`"handled":true`)) {
		t.Error("expected handled in JSON")
	}
}

func TestWASMModule_NilReceivers(t *testing.T) {
	t.Parallel()
	var m *WASMModule
	if m.ID() != "" {
		t.Error("nil ID should be empty")
	}
	if m.Name() != "" {
		t.Error("nil Name should be empty")
	}
	if m.Version() != "" {
		t.Error("nil Version should be empty")
	}
	if m.Phase() != PhasePreProxy {
		t.Error("nil Phase should be PhasePreProxy")
	}
	if m.Priority() != 100 {
		t.Error("nil Priority should be 100")
	}
	if m.Size() != 0 {
		t.Error("nil Size should be 0")
	}
	if err := m.Close(); err != nil {
		t.Errorf("nil Close should not error: %v", err)
	}
}

func TestWASMModule_Execute_NotLoaded(t *testing.T) {
	t.Parallel()
	m := &WASMModule{}
	_, err := m.Execute(nil)
	if err == nil {
		t.Error("expected error for not-loaded module")
	}
}

func TestWASMPluginManager_Disabled(t *testing.T) {
	t.Parallel()
	pm, err := NewWASMPluginManager(WASMConfig{Enabled: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pm.IsEnabled() {
		t.Error("disabled config should not be enabled")
	}
	if err := pm.LoadModule("test", "test.wasm", nil); err == nil {
		t.Error("expected error loading on disabled manager")
	}
}

func TestWASMPluginManager_Close_Nil(t *testing.T) {
	t.Parallel()
	var pm *WASMPluginManager
	if err := pm.Close(); err != nil {
		t.Errorf("nil Close should not error: %v", err)
	}
}

func TestWASMPluginManager_GetModule_NotFound(t *testing.T) {
	t.Parallel()
	pm, _ := NewWASMPluginManager(WASMConfig{Enabled: false})
	_, ok := pm.GetModule("nonexistent")
	if ok {
		t.Error("should not find module")
	}
}

func TestWASMPluginManager_ListModules_Empty(t *testing.T) {
	t.Parallel()
	pm, _ := NewWASMPluginManager(WASMConfig{Enabled: false})
	modules := pm.ListModules()
	if len(modules) != 0 {
		t.Error("expected empty list")
	}
}

func TestWASMPluginManager_UnloadModule_NotFound(t *testing.T) {
	t.Parallel()
	pm, _ := NewWASMPluginManager(WASMConfig{Enabled: false})
	err := pm.UnloadModule("nonexistent")
	if err == nil {
		t.Error("expected error for missing module")
	}
}

func TestWASMPluginManager_CreatePipelinePlugin_NotFound(t *testing.T) {
	t.Parallel()
	pm, _ := NewWASMPluginManager(WASMConfig{Enabled: false})
	_, err := pm.CreatePipelinePlugin("nonexistent")
	if err == nil {
		t.Error("expected error for missing module")
	}
}

func TestWASMRuntime_LoadModule_Disabled(t *testing.T) {
	t.Parallel()
	rt := &WASMRuntime{config: WASMConfig{Enabled: false}}
	_, err := rt.LoadModule("test", "test.wasm", nil)
	if err == nil {
		t.Error("expected error loading on disabled runtime")
	}
}

func TestWASMContext_EmptyCorrelationID_DoesNotOverwrite(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := &PipelineContext{
		Request:       req,
		CorrelationID: "existing-id",
		Metadata:      map[string]any{},
	}

	wc := &WASMContext{
		CorrelationID: "",
	}
	wc.ApplyToContext(ctx)
	if ctx.CorrelationID != "existing-id" {
		t.Errorf("empty CorrelationID should not overwrite, got %q", ctx.CorrelationID)
	}
}

// --- Tests with actual WASM binary ---

// minimalWASM is a pre-compiled WASM module exporting:
//   - "memory": 1 page of linear memory
//   - "handle_request": (i32,i32)->(i32,i32), returns (0, 0)
var minimalWASM = []byte{
	// \0asm magic + version 1
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
	// Type section: func (i32,i32)->(i32,i32)
	0x01, 0x08, 0x01, 0x60, 0x02, 0x7f, 0x7f, 0x02, 0x7f, 0x7f,
	// Function section: 1 func, type 0
	0x03, 0x02, 0x01, 0x00,
	// Memory section: 1 memory, 1 page, no max
	0x05, 0x03, 0x01, 0x00, 0x01,
	// Export section: "memory" + "handle_request"
	0x07, 0x1b, 0x02,
	0x06, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x02, 0x00,
	0x0e, 0x68, 0x61, 0x6e, 0x64, 0x6c, 0x65, 0x5f, 0x72, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x00, 0x00,
	// Code section: body returning (i32.const 0, i32.const 0)
	0x0a, 0x08, 0x01, 0x06, 0x00, 0x41, 0x00, 0x41, 0x00, 0x0b,
}

func writeMinimalWASM(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "test.wasm"), minimalWASM, 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func newTestWASMRuntime(t *testing.T) *WASMRuntime {
	t.Helper()
	dir := writeMinimalWASM(t)
	cfg := WASMConfig{
		Enabled:      true,
		ModuleDir:    dir,
		MaxMemory:    10 * 1024 * 1024,
		MaxExecution: 5 * time.Second,
	}
	rt, err := NewWASMRuntime(cfg)
	if err != nil {
		t.Fatalf("NewWASMRuntime: %v", err)
	}
	t.Cleanup(func() { rt.Close() })
	return rt
}

func newTestWASMManager(t *testing.T) *WASMPluginManager {
	t.Helper()
	dir := writeMinimalWASM(t)
	cfg := WASMConfig{
		Enabled:      true,
		ModuleDir:    dir,
		MaxMemory:    10 * 1024 * 1024,
		MaxExecution: 5 * time.Second,
	}
	pm, err := NewWASMPluginManager(cfg)
	if err != nil {
		t.Fatalf("NewWASMPluginManager: %v", err)
	}
	t.Cleanup(func() { pm.Close() })
	return pm
}

func TestWASMRuntime_LoadModule_Success(t *testing.T) {
	rt := newTestWASMRuntime(t)
	// SEC-WASM-001: PhaseAuth is intentionally rejected for WASM plugins,
	// so the happy-path success case uses PhasePreProxy. The rejection case
	// is covered by TestResolveWASMPhase.
	mod, err := rt.LoadModule("test-mod", "test.wasm", map[string]any{
		"name":    "TestPlugin",
		"version": "2.0.0",
		"phase":   "pre-proxy",
	})
	if err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	defer mod.Close()

	if mod.ID() != "test-mod" {
		t.Errorf("ID = %q, want test-mod", mod.ID())
	}
	if mod.Name() != "TestPlugin" {
		t.Errorf("Name = %q, want TestPlugin", mod.Name())
	}
	if mod.Version() != "2.0.0" {
		t.Errorf("Version = %q, want 2.0.0", mod.Version())
	}
	if mod.Phase() != PhasePreProxy {
		t.Errorf("Phase = %v, want pre-proxy", mod.Phase())
	}
	if mod.Size() == 0 {
		t.Error("Size should be > 0")
	}
}

func TestWASMRuntime_LoadModule_DefaultMetadata(t *testing.T) {
	rt := newTestWASMRuntime(t)
	mod, err := rt.LoadModule("mod2", "test.wasm", map[string]any{})
	if err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	defer mod.Close()

	if mod.Name() != "mod2" {
		t.Errorf("Name = %q, want mod2 (defaults to id)", mod.Name())
	}
	if mod.Version() != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", mod.Version())
	}
	if mod.Phase() != PhasePreProxy {
		t.Errorf("Phase = %v, want pre_proxy", mod.Phase())
	}
	if mod.Priority() != 100 {
		t.Errorf("Priority = %d, want 100", mod.Priority())
	}
}

func TestWASMRuntime_LoadModule_InvalidWASM(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.wasm"),
		append([]byte("\x00asm\x01\x00\x00\x00"), []byte("INVALID")...), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := WASMConfig{Enabled: true, ModuleDir: dir, MaxMemory: 10 * 1024 * 1024, MaxExecution: 5 * time.Second}
	rt, err := NewWASMRuntime(cfg)
	if err != nil {
		t.Fatalf("NewWASMRuntime: %v", err)
	}
	defer rt.Close()

	_, err = rt.LoadModule("bad", "bad.wasm", nil)
	if err == nil {
		t.Error("expected error for invalid WASM binary")
	}
}

func TestWASMModule_Execute_MinimalModule(t *testing.T) {
	rt := newTestWASMRuntime(t)
	mod, err := rt.LoadModule("exec-test", "test.wasm", nil)
	if err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	defer mod.Close()

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	_, err = mod.Execute(&PipelineContext{Request: req})
	if err == nil {
		t.Error("expected error from minimal module (empty response)")
	}
}

func TestWASMModule_Close_Loaded(t *testing.T) {
	rt := newTestWASMRuntime(t)
	mod, err := rt.LoadModule("close-test", "test.wasm", nil)
	if err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	if err := mod.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	_, err = mod.Execute(&PipelineContext{Request: req})
	if err == nil {
		t.Error("expected error after close")
	}
}

func TestWASMPluginManager_LoadModule_Success(t *testing.T) {
	pm := newTestWASMManager(t)
	if !pm.IsEnabled() {
		t.Error("should be enabled")
	}
	if err := pm.LoadModule("mod-1", "test.wasm", map[string]any{"name": "TestMod"}); err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	mod, ok := pm.GetModule("mod-1")
	if !ok {
		t.Fatal("module not found after load")
	}
	if mod.Name() != "TestMod" {
		t.Errorf("Name = %q, want TestMod", mod.Name())
	}
	if len(pm.ListModules()) != 1 {
		t.Errorf("ListModules count = %d, want 1", len(pm.ListModules()))
	}
}

func TestWASMPluginManager_UnloadModule_Success(t *testing.T) {
	pm := newTestWASMManager(t)
	pm.LoadModule("mod-1", "test.wasm", nil)
	if err := pm.UnloadModule("mod-1"); err != nil {
		t.Fatalf("UnloadModule: %v", err)
	}
	if _, ok := pm.GetModule("mod-1"); ok {
		t.Error("module should be gone after unload")
	}
}

func TestWASMPluginManager_CreatePipelinePlugin_Success(t *testing.T) {
	pm := newTestWASMManager(t)
	pm.LoadModule("mod-1", "test.wasm", map[string]any{
		"name": "WasmTest", "phase": "pre-proxy", "priority": 50,
	})
	plug, err := pm.CreatePipelinePlugin("mod-1")
	if err != nil {
		t.Fatalf("CreatePipelinePlugin: %v", err)
	}
	if plug.name != "wasm-WasmTest" {
		t.Errorf("name = %q, want wasm-WasmTest", plug.name)
	}
	if plug.phase != PhasePreProxy {
		t.Errorf("phase = %v, want pre-proxy", plug.phase)
	}
	if plug.priority != 50 {
		t.Errorf("priority = %d, want 50", plug.priority)
	}
}

func TestWASMPluginManager_CreatePipelinePlugin_Execute(t *testing.T) {
	pm := newTestWASMManager(t)
	pm.LoadModule("mod-1", "test.wasm", nil)
	plug, _ := pm.CreatePipelinePlugin("mod-1")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	_, err := plug.run(&PipelineContext{Request: req})
	if err == nil {
		t.Error("expected error from minimal module")
	}
}

func TestWASMPluginManager_ReloadModule(t *testing.T) {
	pm := newTestWASMManager(t)
	pm.LoadModule("mod-1", "test.wasm", map[string]any{"name": "v1"})
	mod1, _ := pm.GetModule("mod-1")
	if mod1.Name() != "v1" {
		t.Errorf("Name = %q, want v1", mod1.Name())
	}
	pm.LoadModule("mod-1", "test.wasm", map[string]any{"name": "v2"})
	mod2, _ := pm.GetModule("mod-1")
	if mod2.Name() != "v2" {
		t.Errorf("Name = %q, want v2", mod2.Name())
	}
}

func TestWASMRuntime_SafeResolvePath_Absolute(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rt := &WASMRuntime{config: WASMConfig{ModuleDir: dir}}
	absPath := filepath.Join(dir, "sub", "test.wasm")
	resolved, err := rt.safeResolvePath(absPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != absPath {
		t.Errorf("resolved = %q, want %q", resolved, absPath)
	}
}

func TestWASMRuntime_IsEnabled_Nil(t *testing.T) {
	t.Parallel()
	var rt *WASMRuntime
	if rt.IsEnabled() {
		t.Error("nil runtime should not be enabled")
	}
}

func TestWASMPluginManager_IsEnabled_Nil(t *testing.T) {
	t.Parallel()
	var pm *WASMPluginManager
	if pm.IsEnabled() {
		t.Error("nil manager should not be enabled")
	}
}

func TestToWASMContext_NilRequest(t *testing.T) {
	t.Parallel()
	ctx := &PipelineContext{}
	wc := ToWASMContext(ctx)
	if wc == nil {
		t.Fatal("expected non-nil WASMContext")
	}
	if wc.Method != "" {
		t.Error("expected empty method for nil request")
	}
}

func TestValidateWASMModule_PublicWrapper(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "valid.wasm")
	if err := os.WriteFile(path, append([]byte("\x00asm"), make([]byte, 50)...), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateWASMModule(path); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestResolveWASMPhase verifies the SEC-WASM-001 fix: the phase string from
// plugin config is validated against the known phase set, PhaseAuth is
// forbidden for WASM plugins, and anything unknown is rejected rather than
// silently accepted as a new Phase(p).
func TestResolveWASMPhase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		config     map[string]any
		wantPhase  Phase
		wantErrSub string // substring of expected error; "" means no error
	}{
		{
			name:      "empty_config_defaults_to_pre_proxy",
			config:    nil,
			wantPhase: PhasePreProxy,
		},
		{
			name:      "empty_phase_defaults_to_pre_proxy",
			config:    map[string]any{"phase": ""},
			wantPhase: PhasePreProxy,
		},
		{
			name:      "pre_auth_allowed",
			config:    map[string]any{"phase": "pre-auth"},
			wantPhase: PhasePreAuth,
		},
		{
			name:      "pre_proxy_allowed",
			config:    map[string]any{"phase": "pre-proxy"},
			wantPhase: PhasePreProxy,
		},
		{
			name:      "proxy_allowed",
			config:    map[string]any{"phase": "proxy"},
			wantPhase: PhaseProxy,
		},
		{
			name:       "post_proxy_rejected_for_wasm",
			config:     map[string]any{"phase": "post-proxy"},
			wantErrSub: `not permitted`,
		},
		{
			name:       "auth_phase_rejected_for_wasm",
			config:     map[string]any{"phase": "auth"},
			wantErrSub: `not permitted`,
		},
		{
			name:       "unknown_phase_rejected",
			config:     map[string]any{"phase": "custom"},
			wantErrSub: `invalid phase`,
		},
		{
			name:       "garbage_string_rejected",
			config:     map[string]any{"phase": "../etc/passwd"},
			wantErrSub: `invalid phase`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			phase, err := resolveWASMPhase("mod-1", tt.config)
			if tt.wantErrSub != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErrSub)
				}
				if !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if phase != tt.wantPhase {
				t.Fatalf("expected phase %q, got %q", tt.wantPhase, phase)
			}
		})
	}
}
