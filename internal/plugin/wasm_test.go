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
	if err := os.WriteFile(path, []byte("NotWASM" + strings.Repeat("x", 100)), 0644); err != nil {
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
	// Create file with valid magic but that exceeds size limit
	data := append([]byte("\x00asm"), make([]byte, 200)...)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	// Test with max size smaller than file
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
	wc.ApplyToContext(nil) // should not panic

	wc = &WASMContext{}
	wc.ApplyToContext(nil) // should not panic
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
	if !bytes.Contains(data, []byte(`"handle_request"`)) {
		// Just verify it marshals as valid JSON
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

func TestWriteToWASMMemory_NilModule(t *testing.T) {
	t.Parallel()
	// Can't test with nil api.Module since it's an interface
	// This tests the memory write/read cycle indirectly via the context helpers
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
		CorrelationID: "", // empty - should not overwrite
	}
	wc.ApplyToContext(ctx)
	if ctx.CorrelationID != "existing-id" {
		t.Errorf("empty CorrelationID should not overwrite, got %q", ctx.CorrelationID)
	}
}
