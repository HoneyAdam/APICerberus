package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	coerce "github.com/APICerberus/APICerebrus/internal/pkg/coerce"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

const (
	maxWASMModuleSize = 100 * 1024 * 1024 // 100MB hard cap
	wasmMagicHeader   = "\x00asm"
	wasmExportName    = "handle_request"  // expected WASM export for request handling
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

// Validate checks that the WASM config has sane limits.
func (c WASMConfig) Validate() error {
	if c.MaxMemory <= 0 {
		return fmt.Errorf("wasm max_memory must be positive")
	}
	if c.MaxExecution <= 0 {
		return fmt.Errorf("wasm max_execution must be positive")
	}
	return nil
}

// WASMModule represents a loaded WebAssembly module.
type WASMModule struct {
	id       string
	name     string
	version  string
	phase    Phase
	priority int
	path     string
	size     int64
	config   map[string]any

	// wazero runtime state
	runtime   *WASMRuntime
	mu        sync.RWMutex
	compiled  wazero.CompiledModule
	module    api.Module
	loaded    bool
	loadTime  time.Time
}

// WASMRuntime is the interface for WASM runtime implementations.
type WASMRuntime struct {
	config  WASMConfig
	runtime wazero.Runtime
	mu      sync.Mutex
}

// NewWASMRuntime creates a new WASM runtime.
func NewWASMRuntime(cfg WASMConfig) (*WASMRuntime, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid wasm config: %w", err)
	}

	ctx := context.Background()
	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().
		WithCoreFeatures(api.CoreFeaturesV2).
		WithMemoryLimitPages(uint32(cfg.MaxMemory)/65536))

	// Instantiate WASI (required by most WASM compilers)
	wasi_snapshot_preview1.MustInstantiate(ctx, rt)

	return &WASMRuntime{
		config:  cfg,
		runtime: rt,
	}, nil
}

// IsEnabled returns true if WASM is enabled.
func (r *WASMRuntime) IsEnabled() bool {
	return r != nil && r.config.Enabled
}

// Close closes the wazero runtime and all compiled modules.
func (r *WASMRuntime) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runtime.Close(context.Background())
}

// validateWASMModule checks that a WASM file exists, is within bounds,
// and has the correct magic number.
func validateWASMModule(path string, maxSize int64) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("wasm module not found: %w", err)
	}

	if info.Size() <= 0 {
		return fmt.Errorf("wasm module is empty")
	}
	if info.Size() > maxSize {
		return fmt.Errorf("wasm module size %d exceeds limit %d", info.Size(), maxSize)
	}
	if info.Size() > maxWASMModuleSize {
		return fmt.Errorf("wasm module size %d exceeds hard cap %d", info.Size(), maxWASMModuleSize)
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open wasm module: %w", err)
	}
	defer f.Close()

	magic := make([]byte, 4)
	if _, err := io.ReadFull(f, magic); err != nil {
		return fmt.Errorf("cannot read wasm magic: %w", err)
	}
	if !bytes.Equal(magic, []byte(wasmMagicHeader)) {
		return fmt.Errorf("invalid wasm magic number")
	}
	return nil
}

// safeResolvePath resolves a module path and ensures it's within the module directory.
func (r *WASMRuntime) safeResolvePath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		base, err := filepath.Abs(r.config.ModuleDir)
		if err != nil {
			return "", fmt.Errorf("cannot resolve wasm module dir: %w", err)
		}
		path = filepath.Join(base, path)
	}

	// Ensure the resolved path is within the module directory (prevent traversal)
	moduleDir, err := filepath.Abs(r.config.ModuleDir)
	if err != nil {
		return "", fmt.Errorf("cannot resolve wasm module dir: %w", err)
	}
	rel, err := filepath.Rel(moduleDir, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("wasm module path %q is outside module dir %q", path, moduleDir)
	}

	return path, nil
}

// LoadModule compiles and loads a WASM module from file.
func (r *WASMRuntime) LoadModule(id, path string, pluginConfig map[string]any) (*WASMModule, error) {
	if !r.IsEnabled() {
		return nil, fmt.Errorf("wasm runtime is disabled")
	}

	// Resolve and validate path
	resolved, err := r.safeResolvePath(path)
	if err != nil {
		return nil, err
	}

	// Validate module file (existence, size, magic)
	if err := validateWASMModule(resolved, r.config.MaxMemory); err != nil {
		return nil, err
	}

	info, _ := os.Stat(resolved)
	wasmBytes, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("cannot read wasm module: %w", err)
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

	// Compile the module (AOT compilation, done once)
	ctx := context.Background()
	compiled, err := r.runtime.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("wasm compile failed: %w", err)
	}

	// Instantiate the module (creates memory, globals, etc.)
	// Memory is limited by the runtime's WithMemoryLimitPages config.
	inst, err := r.runtime.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().
		WithName(id).
		WithStartFunctions("_start"))
	if err != nil {
		compiled.Close(ctx)
		return nil, fmt.Errorf("wasm instantiate failed: %w", err)
	}

	module := &WASMModule{
		id:        id,
		name:      name,
		version:   version,
		phase:     phase,
		priority:  priority,
		path:      resolved,
		size:      info.Size(),
		config:    pluginConfig,
		runtime:   r,
		compiled:  compiled,
		module:    inst,
		loaded:    true,
		loadTime:  time.Now(),
	}

	return module, nil
}

// Execute runs the WASM module with the given pipeline context.
// It serializes the context to JSON, calls the WASM export, and deserializes the result.
// Enforces MaxExecution timeout and MaxMemory limits via wazero.
func (m *WASMModule) Execute(ctx *PipelineContext) (handled bool, err error) {
	if m == nil || !m.loaded {
		return false, fmt.Errorf("wasm module not loaded")
	}

	m.mu.RLock()
	timeout := m.runtime.config.MaxExecution
	mod := m.module
	m.mu.RUnlock()

	// Create a context with the configured timeout
	execCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Serialize pipeline context to JSON
	wasmCtx := ToWASMContext(ctx)
	reqBytes, err := json.Marshal(WASMGuestRequest{
		Type:    "request",
		Context: wasmCtx,
		Config:  m.config,
	})
	if err != nil {
		return false, fmt.Errorf("wasm context marshal failed: %w", err)
	}

	// Find the exported handle_request function
	fn := mod.ExportedFunction(wasmExportName)
	if fn == nil {
		// Fallback: try _start (some modules only run initialization)
		fn = mod.ExportedFunction("_start")
		if fn == nil {
			return false, fmt.Errorf("wasm module exports neither %q nor _start", wasmExportName)
		}
	}

	// Allocate memory in the WASM module for the input
	ptr, size, err := writeToWASMMemory(mod, reqBytes)
	if err != nil {
		return false, fmt.Errorf("wasm memory write failed: %w", err)
	}

	// Call the WASM function with (ptr, len) arguments
	// The WASM module should return (result_ptr, result_len)
	results, err := fn.Call(execCtx, uint64(ptr), uint64(size))
	if err != nil {
		return false, fmt.Errorf("wasm execution failed: %w", err)
	}

	// If the function doesn't return values, treat as no-op
	if len(results) < 2 {
		return false, nil
	}

	// Read the result from WASM memory
	resultPtr := uint32(results[0])
	resultLen := uint32(results[1])
	resultBytes, err := readFromWASMMemory(mod, resultPtr, resultLen)
	if err != nil {
		return false, fmt.Errorf("wasm memory read failed: %w", err)
	}

	// Parse the WASM response
	var resp WASMGuestResponse
	if err := json.Unmarshal(resultBytes, &resp); err != nil {
		return false, fmt.Errorf("wasm response parse failed: %w", err)
	}

	// Apply any changes the WASM module made back to the pipeline context
	if resp.Context != nil {
		resp.Context.ApplyToContext(ctx)
	}

	if resp.Error != "" {
		return resp.Handled, fmt.Errorf("wasm plugin error: %s", resp.Error)
	}

	return resp.Handled, nil
}

// writeToWASMMemory allocates memory in the WASM module and writes data.
func writeToWASMMemory(mod api.Module, data []byte) (uint32, uint32, error) {
	mem := mod.Memory()
	if mem == nil {
		return 0, 0, fmt.Errorf("wasm module has no memory")
	}

	// Use the module's alloc function if available, otherwise use a simple approach
	allocFn := mod.ExportedFunction("alloc")
	if allocFn != nil {
		results, err := allocFn.Call(context.Background(), uint64(len(data)))
		if err != nil {
			return 0, 0, fmt.Errorf("wasm alloc failed: %w", err)
		}
		ptr := uint32(results[0])
		if !mem.Write(ptr, data) {
			return 0, 0, fmt.Errorf("wasm memory write out of bounds")
		}
		return ptr, uint32(len(data)), nil
	}

	// Fallback: use a fixed offset (the module should have enough memory)
	// This is only for simple test modules
	freePtr := mod.ExportedGlobal("__data_end")
	var offset uint32
	if freePtr != nil {
		offset = uint32(freePtr.Get())
	} else {
		offset = 0 // Start at beginning if no __data_end
	}
	// Align to 16 bytes
	offset = (offset + 15) &^ 15
	if !mem.Write(offset, data) {
		return 0, 0, fmt.Errorf("wasm memory write out of bounds at offset %d", offset)
	}
	return offset, uint32(len(data)), nil
}

// readFromWASMMemory reads data from the WASM module's memory.
func readFromWASMMemory(mod api.Module, ptr, length uint32) ([]byte, error) {
	mem := mod.Memory()
	if mem == nil {
		return nil, fmt.Errorf("wasm module has no memory")
	}

	data, ok := mem.Read(ptr, length)
	if !ok {
		return nil, fmt.Errorf("wasm memory read out of bounds at ptr=%d len=%d", ptr, length)
	}

	result := make([]byte, length)
	copy(result, data)
	return result, nil
}

// Close unloads the WASM module.
func (m *WASMModule) Close() error {
	if m == nil {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	ctx := context.Background()
	if m.compiled != nil {
		_ = m.compiled.Close(ctx)
	}
	if m.module != nil {
		_ = m.module.Close(ctx)
	}
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

// Size returns the module file size in bytes.
func (m *WASMModule) Size() int64 {
	if m == nil {
		return 0
	}
	return m.size
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
func (m *WASMPluginManager) LoadModule(id, path string, pluginConfig map[string]any) error {
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

	module, err := m.runtime.LoadModule(id, path, pluginConfig)
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

	if m.runtime != nil {
		_ = m.runtime.Close()
	}

	return nil
}

// WASMGuestRequest represents a request to the WASM guest.
type WASMGuestRequest struct {
	Type    string         `json:"type"`
	Context *WASMContext   `json:"context"`
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
	Method        string            `json:"method"`
	Path          string            `json:"path"`
	Query         string            `json:"query"`
	Headers       map[string]string `json:"headers"`
	ConsumerID    string            `json:"consumer_id"`
	ConsumerName  string            `json:"consumer_name"`
	RouteID       string            `json:"route_id"`
	RouteName     string            `json:"route_name"`
	ServiceName   string            `json:"service_name"`
	CorrelationID string            `json:"correlation_id"`
	Metadata      map[string]any    `json:"metadata"`
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

// WASMHostFunctions provides capability-restricted functions exported to WASM guests.
type WASMHostFunctions struct {
	mu sync.RWMutex
	// Capabilities define which host functions the WASM guest may call.
	capabilities map[string]bool
}

// NewWASMHostFunctions creates a new host function provider with default capabilities.
func NewWASMHostFunctions(capabilities map[string]bool) *WASMHostFunctions {
	if capabilities == nil {
		// Default: only allow logging and metadata access
		capabilities = map[string]bool{
			"log":         true,
			"get_metadata": true,
			"set_metadata": true,
		}
	}
	return &WASMHostFunctions{
		capabilities: capabilities,
	}
}

// HasCapability checks if a capability is granted.
func (h *WASMHostFunctions) HasCapability(name string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.capabilities[name]
}

// Log logs a message from the WASM guest.
func (h *WASMHostFunctions) Log(level, message string) {
	if !h.HasCapability("log") {
		return
	}
	fmt.Printf("[WASM:%s] %s\n", level, message)
}

// GetHeader gets a request header.
func (h *WASMHostFunctions) GetHeader(ctx *PipelineContext, name string) string {
	if !h.HasCapability("get_header") {
		return ""
	}
	if ctx == nil || ctx.Request == nil {
		return ""
	}
	return ctx.Request.Header.Get(name)
}

// SetHeader sets a response header.
func (h *WASMHostFunctions) SetHeader(ctx *PipelineContext, name, value string) {
	if !h.HasCapability("set_header") {
		return
	}
	if ctx == nil || ctx.ResponseWriter == nil {
		return
	}
	ctx.ResponseWriter.Header().Set(name, value)
}

// GetMetadata gets a metadata value.
func (h *WASMHostFunctions) GetMetadata(ctx *PipelineContext, key string) any {
	if !h.HasCapability("get_metadata") {
		return nil
	}
	if ctx == nil || ctx.Metadata == nil {
		return nil
	}
	return ctx.Metadata[key]
}

// SetMetadata sets a metadata value.
func (h *WASMHostFunctions) SetMetadata(ctx *PipelineContext, key string, value any) {
	if !h.HasCapability("set_metadata") {
		return
	}
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
	if !h.HasCapability("abort") {
		return
	}
	if ctx == nil {
		return
	}
	ctx.Abort(reason)
}

// ValidateWASMModule validates a WASM module file.
func ValidateWASMModule(path string) error {
	return validateWASMModule(path, maxWASMModuleSize)
}

// buildWASMPlugin creates a PipelinePlugin from WASM configuration.
//lint:ignore U1000 test-only WASM plugin builder for plugin testing
func buildWASMPlugin(spec config.PluginConfig, _ BuilderContext) (PipelinePlugin, error) {
	cfgMap := spec.Config

	moduleID := coerce.AsString(cfgMap["module_id"])
	if moduleID == "" {
		return PipelinePlugin{}, fmt.Errorf("wasm module_id is required")
	}

	modulePath := coerce.AsString(cfgMap["module_path"])
	if modulePath == "" {
		modulePath = moduleID + ".wasm"
	}

	phase := Phase(coerce.AsString(cfgMap["phase"]))
	if phase == "" {
		phase = PhasePreProxy
	}

	priority := coerce.AsInt(cfgMap["priority"], 100)

	// Validate module exists
	if err := ValidateWASMModule(modulePath); err != nil {
		// Don't fail if module doesn't exist yet - it might be loaded dynamically
		fmt.Printf("Warning: WASM module validation failed: %v\n", err)
	}

	return PipelinePlugin{
		name:     fmt.Sprintf("wasm-%s", moduleID),
		phase:    phase,
		priority: priority,
		run: func(ctx *PipelineContext) (bool, error) {
			return false, nil
		},
	}, nil
}
