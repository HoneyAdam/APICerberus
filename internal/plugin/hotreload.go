package plugin

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// HotReloader monitors plugin configuration changes and updates the registry atomically.
type HotReloader struct {
	mu       sync.RWMutex
	registry *Registry
	watchers map[string]*ConfigWatcher
	stopCh   chan struct{}
	onChange func(string)
}

// ConfigWatcher watches a config path for changes.
type ConfigWatcher struct {
	Path    string
	ModTime time.Time
	Hash    string
}

// NewHotReloader creates a hot reloader with the given base registry.
// The registry can be nil and set later via SetRegistry.
func NewHotReloader(onChange func(string)) *HotReloader {
	return &HotReloader{
		registry: NewDefaultRegistry(),
		watchers: make(map[string]*ConfigWatcher),
		stopCh:   make(chan struct{}),
		onChange: onChange,
	}
}

// SetRegistry atomically replaces the active registry.
func (h *HotReloader) SetRegistry(r *Registry) {
	h.mu.Lock()
	h.registry = r
	h.mu.Unlock()
}

// Registry returns the current registry (read-only).
func (h *HotReloader) Registry() *Registry {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.registry
}

// BuildFromSpec wraps Registry.Build with the current registry.
func (h *HotReloader) BuildFromSpec(spec config.PluginConfig, ctx BuilderContext) (PipelinePlugin, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.registry == nil {
		return PipelinePlugin{}, fmt.Errorf("no registry configured")
	}
	return h.registry.Build(spec, ctx)
}

// Lookup returns a factory from the current registry.
func (h *HotReloader) Lookup(name string) (PluginFactory, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.registry == nil {
		return nil, false
	}
	return h.registry.Lookup(name)
}

// Register adds a factory to the current registry.
func (h *HotReloader) Register(name string, factory PluginFactory) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.registry == nil {
		h.registry = NewRegistry()
	}
	return h.registry.Register(name, factory)
}

// Unregister removes a factory from the current registry.
func (h *HotReloader) Unregister(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.registry == nil {
		return
	}
	h.registry.mu.Lock()
	delete(h.registry.factories, name)
	h.registry.mu.Unlock()
}

// WatchConfig records a config path for change detection.
func (h *HotReloader) WatchConfig(path string, modTime time.Time, hash string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.watchers[path] = &ConfigWatcher{
		Path:    path,
		ModTime: modTime,
		Hash:    hash,
	}
}

// CheckChanges compares recorded config state against current filesystem state.
// Returns the list of changed paths. An empty returned slice means no changes.
func (h *HotReloader) CheckChanges(current map[string]ConfigState) (changed []string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for path, cached := range h.watchers {
		cur, ok := current[path]
		if !ok {
			// Path removed or not found — consider it changed.
			changed = append(changed, path)
			continue
		}
		if cached.Hash != cur.Hash || cached.ModTime != cur.ModTime {
			changed = append(changed, path)
		}
	}
	return changed
}

// ConfigState holds the current filesystem metadata for a watched path.
type ConfigState struct {
	ModTime time.Time
	Hash    string
}

// Stop signals all watchers to stop.
func (h *HotReloader) Stop() {
	select {
	case <-h.stopCh:
		return
	default:
		close(h.stopCh)
	}
}

// Reload triggers a registry reload from the current configuration.
// It fires the onChange callback for each changed path.
func (h *HotReloader) Reload(paths []string, newRegistry *Registry) {
	h.mu.Lock()
	oldRegistry := h.registry
	h.registry = newRegistry
	// Update watchers with new mod times.
	now := time.Now()
	for _, p := range paths {
		if w, ok := h.watchers[p]; ok {
			w.ModTime = now
		}
	}
	h.mu.Unlock()

	if oldRegistry != newRegistry && h.onChange != nil {
		for _, p := range paths {
			h.onChange(p)
		}
	}
	log.Printf("[INFO] hotreload: reloaded registry (watched %d paths)", len(paths))
}

// ReloadPlugins rebuilds route plugin chains for all routes in the given config
// and returns the new pipeline map and auth status map.
func (h *HotReloader) ReloadPlugins(cfg *config.Config, consumers []config.Consumer) (map[string][]PipelinePlugin, map[string]bool, error) {
	return BuildRoutePipelinesWithContext(cfg, BuilderContext{
		Consumers: consumers,
	})
}

// ResetWatchers clears all watched paths.
func (h *HotReloader) ResetWatchers() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.watchers = make(map[string]*ConfigWatcher)
}

// SwapRegistry atomically swaps the entire registry.
// This allows a full registry replacement (e.g., for plugin bundle loads).
func (h *HotReloader) SwapRegistry(newReg *Registry) {
	h.mu.Lock()
	old := h.registry
	h.registry = newReg
	h.mu.Unlock()
	if old != newReg && h.onChange != nil {
		h.onChange("registry-swap")
	}
}

// HotReloadConfig represents a hot reload operation result.
type HotReloadConfig struct {
	ChangedPaths []string
	PluginCount int
	Error       error
}