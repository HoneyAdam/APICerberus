package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// WASMConfig holds configuration for WASM plugins.
type WASMConfig struct {
	Enabled         bool              `yaml:"enabled" json:"enabled"`
	ModuleDir       string            `yaml:"module_dir" json:"module_dir"`
	MaxMemory       int64             `yaml:"max_memory" json:"max_memory"`
	MaxExecution    time.Duration     `yaml:"max_execution" json:"max_execution"`
	AllowFilesystem bool              `yaml:"allow_filesystem" json:"allow_filesystem"`
	AllowedPaths    map[string]string `yaml:"allowed_paths" json:"allowed_paths"` // guest path -> host path
	EnvVars         map[string]string `yaml:"env_vars" json:"env_vars"`
}

// DefaultWASMConfig returns default WASM configuration.
func DefaultWASMConfig() WASMConfig {
	return WASMConfig{
		Enabled:         false,
		ModuleDir:       "./plugins/wasm",
		MaxMemory:       128 * 1024 * 1024, // 128MB
		MaxExecution:    30 * time.Second,
		AllowFilesystem: false,
		AllowedPaths:    make(map[string]string),
		EnvVars:         make(map[string]string),
	}
}

// WASMModule represents a loaded WebAssembly module.
type WASMModule struct {
	id       string
	name     string
	version  string
	phase    Phase
	priority int
	path     string
	config   map[string]any

	// Runtime state
	runtime  *WASMRuntime
	mu       sync.RWMutex
	loaded   bool
	loadTime time.Time
}

// WASMRuntime is the interface for WASM runtime implementations.
// This is a placeholder for the actual runtime implementation.
type WASMRuntime struct {
	config WASMConfig
}

// NewWASMRuntime creates a new WASM runtime.
func NewWASMRuntime(cfg WASMConfig) (*WASMRuntime, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	return &WASMRuntime{
		config: cfg,
	}, nil
}

// IsEnabled returns true if WASM is enabled.
func (r *WASMRuntime) IsEnabled() bool {
	return r != nil && r.config.Enabled
}

// LoadModule loads a WASM module from file.
func (r *WASMRuntime) LoadModule(id, path string, pluginConfig map[string]any) (*WASMModule, error) {
	if !r.IsEnabled() {
		return nil, fmt.Errorf("wasm runtime is disabled")
	}

	// Validate file exists
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("wasm module not found: %w", err)
	}

	// Read module metadata from config
	name := id
	if n, ok := pluginConfig["name"].(string); ok && n != "" {
		name = n
	}

	version := "1.0.0"
	if v, ok := pluginConfig["version"].(string); ok && v != "" {
		version = v
	}

	phase := PhasePreProxy
	if p, ok := pluginConfig["phase"].(string); ok && p != "" {
		phase = Phase(p)
	}

	priority := 100
	if pr, ok := pluginConfig["priority"].(int); ok {
		priority = pr
	}

	module := &WASMModule{
		id:       id,
		name:     name,
		version:  version,
		phase:    phase,
		priority: priority,
		path:     path,
		config:   pluginConfig,
		runtime:  r,
		loaded:   true,
		loadTime: time.Now(),
	}

	return module, nil
}

// Execute runs the WASM module with the given context.
func (m *WASMModule) Execute(ctx *PipelineContext) (handled bool, err error) {
	if m == nil || !m.loaded {
		return false, fmt.Errorf("wasm module not loaded")
	}

	// In a real implementation, this would:
	// 1. Instantiate the WASM module
	// 2. Set up host functions
	// 3. Serialize the PipelineContext
	// 4. Call the WASM entry point
	// 5. Deserialize the result

	// Placeholder implementation
	return false, nil
}

// Close unloads the WASM module.
func (m *WASMModule) Close() error {
	if m == nil {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.loaded = false
	return nil
}

// ID returns the module ID.
func (m *WASMModule) ID() string {
	if m == nil {
		return ""
	}
	return m.id
}

// Name returns the module name.
func (m *WASMModule) Name() string {
	if m == nil {
		return ""
	}
	return m.name
}

// Version returns the module version.
func (m *WASMModule) Version() string {
	if m == nil {
		return ""
	}
	return m.version
}

// Phase returns the plugin phase.
func (m *WASMModule) Phase() Phase {
	if m == nil {
		return PhasePreProxy
	}
	return m.phase
}

// Priority returns the plugin priority.
func (m *WASMModule) Priority() int {
	if m == nil {
		return 100
	}
	return m.priority
}

// WASMPluginManager manages WASM plugins.
type WASMPluginManager struct {
	runtime *WASMRuntime
	modules map[string]*WASMModule
	mu      sync.RWMutex
	config  WASMConfig
}

// NewWASMPluginManager creates a new WASM plugin manager.
func NewWASMPluginManager(cfg WASMConfig) (*WASMPluginManager, error) {
	runtime, err := NewWASMRuntime(cfg)
	if err != nil {
		return nil, err
	}

	return &WASMPluginManager{
		runtime: runtime,
		modules: make(map[string]*WASMModule),
		config:  cfg,
	}, nil
}

// IsEnabled returns true if WASM plugins are enabled.
func (m *WASMPluginManager) IsEnabled() bool {
	return m != nil && m.runtime != nil && m.runtime.IsEnabled()
}

// LoadModule loads a WASM module.
func (m *WASMPluginManager) LoadModule(id, path string, config map[string]any) error {
	if !m.IsEnabled() {
		return fmt.Errorf("wasm plugin manager is disabled")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Unload existing module if present
	if existing, ok := m.modules[id]; ok {
		_ = existing.Close()
		delete(m.modules, id)
	}

	// Resolve full path
	if !filepath.IsAbs(path) {
		path = filepath.Join(m.config.ModuleDir, path)
	}

	module, err := m.runtime.LoadModule(id, path, config)
	if err != nil {
		return err
	}

	m.modules[id] = module
	return nil
}

// UnloadModule unloads a WASM module.
func (m *WASMPluginManager) UnloadModule(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	module, ok := m.modules[id]
	if !ok {
		return fmt.Errorf("module %s not found", id)
	}

	if err := module.Close(); err != nil {
		return err
	}

	delete(m.modules, id)
	return nil
}

// GetModule returns a loaded module.
func (m *WASMPluginManager) GetModule(id string) (*WASMModule, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	module, ok := m.modules[id]
	return module, ok
}

// ListModules returns all loaded modules.
func (m *WASMPluginManager) ListModules() []*WASMModule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	modules := make([]*WASMModule, 0, len(m.modules))
	for _, module := range m.modules {
		modules = append(modules, module)
	}
	return modules
}

// CreatePipelinePlugin creates a PipelinePlugin from a WASM module.
func (m *WASMPluginManager) CreatePipelinePlugin(moduleID string) (PipelinePlugin, error) {
	module, ok := m.GetModule(moduleID)
	if !ok {
		return PipelinePlugin{}, fmt.Errorf("wasm module %s not found", moduleID)
	}

	return PipelinePlugin{
		name:     fmt.Sprintf("wasm-%s", module.Name()),
		phase:    module.Phase(),
		priority: module.Priority(),
		run: func(ctx *PipelineContext) (bool, error) {
			return module.Execute(ctx)
		},
	}, nil
}

// Close shuts down the WASM plugin manager.
func (m *WASMPluginManager) Close() error {
	if m == nil {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, module := range m.modules {
		_ = module.Close()
	}
	m.modules = make(map[string]*WASMModule)

	return nil
}

// WASMGuestRequest represents a request to the WASM guest.
type WASMGuestRequest struct {
	Type    string                 `json:"type"`
	Context *WASMContext           `json:"context"`
	Config  map[string]any `json:"config"`
}

// WASMGuestResponse represents a response from the WASM guest.
type WASMGuestResponse struct {
	Handled bool         `json:"handled"`
	Error   string       `json:"error,omitempty"`
	Context *WASMContext `json:"context,omitempty"`
}

// WASMContext is a serializable subset of PipelineContext for WASM.
type WASMContext struct {
	Method        string                 `json:"method"`
	Path          string                 `json:"path"`
	Query         string                 `json:"query"`
	Headers       map[string]string      `json:"headers"`
	ConsumerID    string                 `json:"consumer_id"`
	ConsumerName  string                 `json:"consumer_name"`
	RouteID       string                 `json:"route_id"`
	RouteName     string                 `json:"route_name"`
	ServiceName   string                 `json:"service_name"`
	CorrelationID string                 `json:"correlation_id"`
	Metadata      map[string]any `json:"metadata"`
}

// ToWASMContext converts PipelineContext to WASMContext.
func ToWASMContext(ctx *PipelineContext) *WASMContext {
	if ctx == nil || ctx.Request == nil {
		return &WASMContext{}
	}

	wc := &WASMContext{
		Method:        ctx.Request.Method,
		Path:          ctx.Request.URL.Path,
		Query:         ctx.Request.URL.RawQuery,
		Headers:       make(map[string]string),
		CorrelationID: ctx.CorrelationID,
		Metadata:      make(map[string]any),
	}

	// Copy headers
	for k, v := range ctx.Request.Header {
		if len(v) > 0 {
			wc.Headers[k] = v[0]
		}
	}

	// Copy consumer info
	if ctx.Consumer != nil {
		wc.ConsumerID = ctx.Consumer.ID
		wc.ConsumerName = ctx.Consumer.Name
	}

	// Copy route info
	if ctx.Route != nil {
		wc.RouteID = ctx.Route.ID
		wc.RouteName = ctx.Route.Name
	}

	// Copy service info
	if ctx.Service != nil {
		wc.ServiceName = ctx.Service.Name
	}

	// Copy metadata
	for k, v := range ctx.Metadata {
		wc.Metadata[k] = v
	}

	return wc
}

// ApplyToContext applies WASMContext changes back to PipelineContext.
func (wc *WASMContext) ApplyToContext(ctx *PipelineContext) {
	if ctx == nil || ctx.Request == nil {
		return
	}

	// Apply header changes
	for k, v := range wc.Headers {
		ctx.Request.Header.Set(k, v)
	}

	// Apply metadata changes
	for k, v := range wc.Metadata {
		ctx.Metadata[k] = v
	}

	// Update correlation ID
	if wc.CorrelationID != "" {
		ctx.CorrelationID = wc.CorrelationID
	}
}

// Serialize serializes the context to JSON.
func (wc *WASMContext) Serialize() ([]byte, error) {
	return json.Marshal(wc)
}

// Deserialize deserializes the context from JSON.
func (wc *WASMContext) Deserialize(data []byte) error {
	return json.Unmarshal(data, wc)
}

// WASMHostFunctions provides functions exported to WASM guests.
type WASMHostFunctions struct {
	mu sync.RWMutex
}

// NewWASMHostFunctions creates a new host function provider.
func NewWASMHostFunctions() *WASMHostFunctions {
	return &WASMHostFunctions{}
}

// Log logs a message from the WASM guest.
func (h *WASMHostFunctions) Log(level, message string) {
	// In real implementation, this would use the gateway's logger
	fmt.Printf("[WASM:%s] %s\n", level, message)
}

// GetHeader gets a request header.
func (h *WASMHostFunctions) GetHeader(ctx *PipelineContext, name string) string {
	if ctx == nil || ctx.Request == nil {
		return ""
	}
	return ctx.Request.Header.Get(name)
}

// SetHeader sets a response header.
func (h *WASMHostFunctions) SetHeader(ctx *PipelineContext, name, value string) {
	if ctx == nil || ctx.ResponseWriter == nil {
		return
	}
	ctx.ResponseWriter.Header().Set(name, value)
}

// GetMetadata gets a metadata value.
func (h *WASMHostFunctions) GetMetadata(ctx *PipelineContext, key string) any {
	if ctx == nil || ctx.Metadata == nil {
		return nil
	}
	return ctx.Metadata[key]
}

// SetMetadata sets a metadata value.
func (h *WASMHostFunctions) SetMetadata(ctx *PipelineContext, key string, value any) {
	if ctx == nil {
		return
	}
	if ctx.Metadata == nil {
		ctx.Metadata = make(map[string]any)
	}
	ctx.Metadata[key] = value
}

// Abort aborts the request processing.
func (h *WASMHostFunctions) Abort(ctx *PipelineContext, reason string) {
	if ctx == nil {
		return
	}
	ctx.Abort(reason)
}

// ValidateWASMModule validates a WASM module file.
func ValidateWASMModule(path string) error {
	// Check file exists
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("wasm module not found: %w", err)
	}

	// Check file size (max 100MB)
	if info.Size() > 100*1024*1024 {
		return fmt.Errorf("wasm module too large: %d bytes", info.Size())
	}

	// Read and validate WASM magic number
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open wasm module: %w", err)
	}
	defer f.Close()

	magic := make([]byte, 4)
	if _, err := io.ReadFull(f, magic); err != nil {
		return fmt.Errorf("cannot read wasm magic: %w", err)
	}

	// WASM magic number: \0asm
	if !bytes.Equal(magic, []byte{0x00, 0x61, 0x73, 0x6d}) {
		return fmt.Errorf("invalid wasm magic number")
	}

	return nil
}

// buildWASMPlugin creates a PipelinePlugin from WASM configuration.
func buildWASMPlugin(spec config.PluginConfig, _ BuilderContext) (PipelinePlugin, error) {
	cfgMap := spec.Config

	moduleID := asString(cfgMap["module_id"])
	if moduleID == "" {
		return PipelinePlugin{}, fmt.Errorf("wasm module_id is required")
	}

	modulePath := asString(cfgMap["module_path"])
	if modulePath == "" {
		modulePath = moduleID + ".wasm"
	}

	phase := Phase(asString(cfgMap["phase"]))
	if phase == "" {
		phase = PhasePreProxy
	}

	priority := asInt(cfgMap["priority"], 100)

	// Validate module exists
	if err := ValidateWASMModule(modulePath); err != nil {
		// Don't fail if module doesn't exist yet - it might be loaded dynamically
		// Just log a warning
		fmt.Printf("Warning: WASM module validation failed: %v\n", err)
	}

	return PipelinePlugin{
		name:     fmt.Sprintf("wasm-%s", moduleID),
		phase:    phase,
		priority: priority,
		run: func(ctx *PipelineContext) (bool, error) {
			// In a real implementation, this would call the WASM runtime
			// For now, just return no handling
			return false, nil
		},
	}, nil
}
