package plugin

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// HotReloadConfig holds hot-reload configuration
type HotReloadConfig struct {
	Enabled  bool     `yaml:"enabled" json:"enabled"`
	WatchDir string   `yaml:"watch_dir" json:"watch_dir"`
	Patterns []string `yaml:"patterns" json:"patterns"`
}

// PluginReloader handles dynamic plugin reloading
type PluginReloader struct {
	mu       sync.RWMutex
	config   HotReloadConfig
	watcher  *fsnotify.Watcher
	registry *Registry
	stopCh   chan struct{}
	handlers map[string]ReloadHandler
}

// ReloadHandler is called when a plugin is reloaded
type ReloadHandler func(name string, content []byte) error

// NewPluginReloader creates a new plugin reloader
func NewPluginReloader(config HotReloadConfig, registry *Registry) (*PluginReloader, error) {
	if !config.Enabled {
		return &PluginReloader{config: config, registry: registry}, nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	reloader := &PluginReloader{
		config:   config,
		watcher:  watcher,
		registry: registry,
		stopCh:   make(chan struct{}),
		handlers: make(map[string]ReloadHandler),
	}

	// Watch directory
	if config.WatchDir != "" {
		if err := watcher.Add(config.WatchDir); err != nil {
			if closeErr := watcher.Close(); closeErr != nil {
				log.Printf("[WARN] failed to close watcher: %v", closeErr)
			}
			return nil, fmt.Errorf("failed to watch directory: %w", err)
		}
	}

	// Start watching
	go reloader.watch()

	return reloader, nil
}

// RegisterHandler registers a reload handler for a plugin
func (r *PluginReloader) RegisterHandler(name string, handler ReloadHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[name] = handler
}

// UnregisterHandler removes a reload handler
func (r *PluginReloader) UnregisterHandler(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.handlers, name)
}

// watch monitors file changes
func (r *PluginReloader) watch() {
	if !r.config.Enabled {
		return
	}

	debounce := make(map[string]time.Time)
	debounceInterval := 500 * time.Millisecond

	for {
		select {
		case event, ok := <-r.watcher.Events:
			if !ok {
				return
			}

			// Check if file matches patterns
			if !r.matchesPattern(event.Name) {
				continue
			}

			// Debounce rapid changes
			if lastTime, exists := debounce[event.Name]; exists {
				if time.Since(lastTime) < debounceInterval {
					continue
				}
			}
			debounce[event.Name] = time.Now()

			// Handle event
			if event.Op&fsnotify.Write == fsnotify.Write {
				r.handleFileChange(event.Name)
			}
			if event.Op&fsnotify.Create == fsnotify.Create {
				r.handleFileCreate(event.Name)
			}
			if event.Op&fsnotify.Remove == fsnotify.Remove {
				r.handleFileRemove(event.Name)
			}

		case err, ok := <-r.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[ERROR] plugin reloader: %v", err)

		case <-r.stopCh:
			return
		}
	}
}

// matchesPattern checks if file matches watched patterns
func (r *PluginReloader) matchesPattern(path string) bool {
	if len(r.config.Patterns) == 0 {
		return true
	}

	for _, pattern := range r.config.Patterns {
		matched, _ := filepath.Match(pattern, filepath.Base(path))
		if matched {
			return true
		}
	}
	return false
}

// handleFileChange handles file modification
func (r *PluginReloader) handleFileChange(path string) {
	log.Printf("[INFO] plugin file modified: %s", path)

	// Extract plugin name
	name := filepath.Base(path)
	name = name[:len(name)-len(filepath.Ext(name))]

	// Read file
	// #nosec G304 -- path comes from the administrator-configured watch directory.
	content, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[ERROR] failed to read plugin file: %v", err)
		return
	}

	// Call handler
	r.mu.RLock()
	handler, exists := r.handlers[name]
	r.mu.RUnlock()

	if exists {
		if err := handler(name, content); err != nil {
			log.Printf("[ERROR] plugin reload failed: %v", err)
			return
		}
		log.Printf("[INFO] plugin reloaded: %s", name)
	}
}

// handleFileCreate handles new file
func (r *PluginReloader) handleFileCreate(path string) {
	log.Printf("[INFO] plugin file created: %s", path)
	r.handleFileChange(path)
}

// handleFileRemove handles file deletion
func (r *PluginReloader) handleFileRemove(path string) {
	log.Printf("[INFO] plugin file removed: %s", path)

	name := filepath.Base(path)
	name = name[:len(name)-len(filepath.Ext(name))]

	// Call handler with nil content
	r.mu.RLock()
	handler, exists := r.handlers[name]
	r.mu.RUnlock()

	if exists {
		if err := handler(name, nil); err != nil {
			log.Printf("[ERROR] plugin unload failed: %v", err)
		}
	}
}

// Stop stops the reloader
func (r *PluginReloader) Stop() {
	if r.stopCh != nil {
		close(r.stopCh)
	}
	if r.watcher != nil {
		if err := r.watcher.Close(); err != nil {
			log.Printf("[WARN] failed to close watcher: %v", err)
		}
	}
}

// WatchFile adds a file to watch
func (r *PluginReloader) WatchFile(path string) error {
	if !r.config.Enabled || r.watcher == nil {
		return nil
	}
	return r.watcher.Add(path)
}

// UnwatchFile removes a file from watch
func (r *PluginReloader) UnwatchFile(path string) error {
	if !r.config.Enabled || r.watcher == nil {
		return nil
	}
	return r.watcher.Remove(path)
}

// ReloadPlugin manually reloads a plugin
func (r *PluginReloader) ReloadPlugin(name string) error {
	r.mu.RLock()
	handler, exists := r.handlers[name]
	r.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no handler registered for plugin: %s", name)
	}

	// Find plugin file
	path := filepath.Join(r.config.WatchDir, name+".lua") // or .wasm, etc.

	// #nosec G304 -- path comes from the administrator-configured watch directory.
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read plugin file: %w", err)
	}

	return handler(name, content)
}

// DynamicPluginManager manages dynamically loaded plugins
type DynamicPluginManager struct {
	mu       sync.RWMutex
	plugins  map[string]DynamicPlugin
	reloader *PluginReloader
}

// DynamicPlugin represents a dynamically loaded plugin
type DynamicPlugin struct {
	Name      string
	Type      string
	Content   []byte
	LoadedAt  time.Time
	UpdatedAt time.Time
	Status    string
	Error     string
}

// NewDynamicPluginManager creates a new dynamic plugin manager
func NewDynamicPluginManager(reloader *PluginReloader) *DynamicPluginManager {
	return &DynamicPluginManager{
		plugins:  make(map[string]DynamicPlugin),
		reloader: reloader,
	}
}

// LoadPlugin loads a plugin
func (m *DynamicPluginManager) LoadPlugin(name string, content []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plugin := DynamicPlugin{
		Name:      name,
		Content:   content,
		LoadedAt:  time.Now(),
		UpdatedAt: time.Now(),
		Status:    "loaded",
	}

	m.plugins[name] = plugin
	log.Printf("[INFO] plugin loaded: %s", name)
	return nil
}

// UnloadPlugin unloads a plugin
func (m *DynamicPluginManager) UnloadPlugin(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.plugins, name)
	log.Printf("[INFO] plugin unloaded: %s", name)
	return nil
}

// UpdatePlugin updates a plugin
func (m *DynamicPluginManager) UpdatePlugin(name string, content []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plugin, exists := m.plugins[name]
	if !exists {
		// Create new plugin directly to avoid nested lock
		plugin = DynamicPlugin{
			Name:      name,
			Content:   content,
			LoadedAt:  time.Now(),
			UpdatedAt: time.Now(),
			Status:    "loaded",
		}
		m.plugins[name] = plugin
		log.Printf("[INFO] plugin loaded: %s", name)
		return nil
	}

	plugin.Content = content
	plugin.UpdatedAt = time.Now()
	plugin.Status = "updated"
	m.plugins[name] = plugin

	log.Printf("[INFO] plugin updated: %s", name)
	return nil
}

// GetPlugin gets a plugin
func (m *DynamicPluginManager) GetPlugin(name string) (DynamicPlugin, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	plugin, exists := m.plugins[name]
	return plugin, exists
}

// ListPlugins lists all loaded plugins
func (m *DynamicPluginManager) ListPlugins() []DynamicPlugin {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugins := make([]DynamicPlugin, 0, len(m.plugins))
	for _, plugin := range m.plugins {
		plugins = append(plugins, plugin)
	}
	return plugins
}

// SetPluginError sets an error for a plugin
func (m *DynamicPluginManager) SetPluginError(name string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if plugin, exists := m.plugins[name]; exists {
		plugin.Status = "error"
		if err != nil {
			plugin.Error = err.Error()
		}
		m.plugins[name] = plugin
	}
}

// ClearPluginError clears the error for a plugin
func (m *DynamicPluginManager) ClearPluginError(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if plugin, exists := m.plugins[name]; exists {
		plugin.Status = "loaded"
		plugin.Error = ""
		m.plugins[name] = plugin
	}
}

// Context key for plugin manager
type pluginManagerKey struct{}

// WithPluginManager adds plugin manager to context
func WithPluginManager(ctx context.Context, manager *DynamicPluginManager) context.Context {
	return context.WithValue(ctx, pluginManagerKey{}, manager)
}

// GetPluginManagerFromContext gets plugin manager from context
func GetPluginManagerFromContext(ctx context.Context) *DynamicPluginManager {
	if manager, ok := ctx.Value(pluginManagerKey{}).(*DynamicPluginManager); ok {
		return manager
	}
	return nil
}
