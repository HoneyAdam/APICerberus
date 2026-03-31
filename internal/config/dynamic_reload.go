package config

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// ConfigReloader handles dynamic configuration reloading
type ConfigReloader struct {
	mu           sync.RWMutex
	path         string
	watcher      *fsnotify.Watcher
	current      *Config
	reloader     func(*Config) error
	stopCh       chan struct{}
	lastModified time.Time
	debounceTime time.Duration
	onChange     func(old, new *Config)
}

// NewConfigReloader creates a new config reloader
func NewConfigReloader(path string, reloader func(*Config) error) (*ConfigReloader, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	// Watch config file
	if err := watcher.Add(path); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("failed to watch config file: %w", err)
	}

	// Also watch parent directory for file recreation
	dir := filepath.Dir(path)
	if err := watcher.Add(dir); err != nil {
		log.Printf("[WARN] failed to watch config directory: %v", err)
	}

	return &ConfigReloader{
		path:         path,
		watcher:      watcher,
		reloader:     reloader,
		stopCh:       make(chan struct{}),
		debounceTime: 1 * time.Second,
	}, nil
}

// SetDebounceTime sets the debounce time
func (r *ConfigReloader) SetDebounceTime(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.debounceTime = d
}

// SetOnChange sets the change callback
func (r *ConfigReloader) SetOnChange(fn func(old, new *Config)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onChange = fn
}

// Start starts watching for config changes
func (r *ConfigReloader) Start() {
	go r.watch()
}

// watch monitors config file changes
func (r *ConfigReloader) watch() {
	for {
		select {
		case event, ok := <-r.watcher.Events:
			if !ok {
				return
			}

			// Only handle events for our config file
			if event.Name != r.path {
				continue
			}

			if event.Op&fsnotify.Write == fsnotify.Write ||
				event.Op&fsnotify.Create == fsnotify.Create {
				r.handleChange()
			}

		case err, ok := <-r.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[ERROR] config watcher: %v", err)

		case <-r.stopCh:
			return
		}
	}
}

// handleChange handles config file change
func (r *ConfigReloader) handleChange() {
	// Debounce
	r.mu.Lock()
	if time.Since(r.lastModified) < r.debounceTime {
		r.mu.Unlock()
		return
	}
	r.lastModified = time.Now()
	r.mu.Unlock()

	log.Printf("[INFO] config file changed, reloading...")

	// Small delay to ensure file is fully written
	time.Sleep(100 * time.Millisecond)

	// Reload config
	newConfig, err := Load(r.path)
	if err != nil {
		log.Printf("[ERROR] failed to reload config: %v", err)
		return
	}

	// Validate new config
	if err := validate(newConfig); err != nil {
		log.Printf("[ERROR] config validation failed: %v", err)
		return
	}

	// Store old config
	r.mu.RLock()
	oldConfig := r.current
	r.mu.RUnlock()

	// Apply new config
	if err := r.reloader(newConfig); err != nil {
		log.Printf("[ERROR] failed to apply config: %v", err)
		return
	}

	// Update current config
	r.mu.Lock()
	r.current = newConfig
	onChange := r.onChange
	r.mu.Unlock()

	log.Printf("[INFO] config reloaded successfully")

	// Call change callback
	if onChange != nil {
		onChange(oldConfig, newConfig)
	}
}

// Stop stops watching
func (r *ConfigReloader) Stop() {
	if r.stopCh != nil {
		close(r.stopCh)
	}
	if r.watcher != nil {
		r.watcher.Close()
	}
}

// GetCurrent returns current config
func (r *ConfigReloader) GetCurrent() *Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.current
}

// TriggerManualReload manually triggers a reload
func (r *ConfigReloader) TriggerManualReload() error {
	r.handleChange()
	return nil
}

// ConfigDiff represents differences between two configs
type ConfigDiff struct {
	ChangedRoutes    []string
	ChangedServices  []string
	ChangedUpstreams []string
	ChangedPlugins   []string
	ChangedGlobal    bool
}

// CompareConfigs compares two configs and returns differences
func CompareConfigs(old, new *Config) ConfigDiff {
	diff := ConfigDiff{}

	if old == nil || new == nil {
		diff.ChangedGlobal = true
		return diff
	}

	// Compare routes
	oldRoutes := make(map[string]Route)
	for _, r := range old.Routes {
		oldRoutes[r.ID] = r
	}
	for _, r := range new.Routes {
		if oldR, exists := oldRoutes[r.ID]; !exists || !routesEqual(oldR, r) {
			diff.ChangedRoutes = append(diff.ChangedRoutes, r.ID)
		}
	}
	for _, r := range old.Routes {
		found := false
		for _, nr := range new.Routes {
			if nr.ID == r.ID {
				found = true
				break
			}
		}
		if !found {
			diff.ChangedRoutes = append(diff.ChangedRoutes, r.ID)
		}
	}

	// Compare services
	oldServices := make(map[string]Service)
	for _, s := range old.Services {
		oldServices[s.ID] = s
	}
	for _, s := range new.Services {
		if oldS, exists := oldServices[s.ID]; !exists || !servicesEqual(oldS, s) {
			diff.ChangedServices = append(diff.ChangedServices, s.ID)
		}
	}

	// Compare upstreams
	oldUpstreams := make(map[string]Upstream)
	for _, u := range old.Upstreams {
		oldUpstreams[u.ID] = u
	}
	for _, u := range new.Upstreams {
		if oldU, exists := oldUpstreams[u.ID]; !exists || !upstreamsEqual(oldU, u) {
			diff.ChangedUpstreams = append(diff.ChangedUpstreams, u.ID)
		}
	}

	return diff
}

// routesEqual checks if two routes are equal
func routesEqual(a, b Route) bool {
	// Simplified comparison
	return a.ID == b.ID &&
		a.Service == b.Service &&
		stringSlicesEqual(a.Paths, b.Paths) &&
		stringSlicesEqual(a.Methods, b.Methods)
}

// servicesEqual checks if two services are equal
func servicesEqual(a, b Service) bool {
	return a.ID == b.ID &&
		a.Upstream == b.Upstream
}

// upstreamsEqual checks if two upstreams are equal
func upstreamsEqual(a, b Upstream) bool {
	return a.ID == b.ID
}

// stringSlicesEqual checks if two string slices are equal
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// DynamicConfigManager manages dynamic configuration
type DynamicConfigManager struct {
	mu       sync.RWMutex
	config   *Config
	reloader *ConfigReloader
	history  []ConfigVersion
	maxHistory int
}

// ConfigVersion represents a config version
type ConfigVersion struct {
	Config    *Config
	AppliedAt time.Time
	AppliedBy string
}

// NewDynamicConfigManager creates a dynamic config manager
func NewDynamicConfigManager(config *Config, reloader func(*Config) error) (*DynamicConfigManager, error) {
	manager := &DynamicConfigManager{
		config:     config,
		maxHistory: 10,
		history:    make([]ConfigVersion, 0, 10),
	}

	// Save initial version
	manager.saveVersion(config, "system")

	return manager, nil
}

// saveVersion saves a config version
func (m *DynamicConfigManager) saveVersion(config *Config, appliedBy string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	version := ConfigVersion{
		Config:    config,
		AppliedAt: time.Now(),
		AppliedBy: appliedBy,
	}

	m.history = append(m.history, version)

	// Trim history
	if len(m.history) > m.maxHistory {
		m.history = m.history[len(m.history)-m.maxHistory:]
	}
}

// UpdateConfig updates the current config
func (m *DynamicConfigManager) UpdateConfig(config *Config, appliedBy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.config = config
	m.saveVersion(config, appliedBy)

	return nil
}

// GetCurrentConfig returns current config
func (m *DynamicConfigManager) GetCurrentConfig() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// GetHistory returns config history
func (m *DynamicConfigManager) GetHistory() []ConfigVersion {
	m.mu.RLock()
	defer m.mu.RUnlock()

	history := make([]ConfigVersion, len(m.history))
	copy(history, m.history)
	return history
}

// Rollback rolls back to a previous version
func (m *DynamicConfigManager) Rollback(index int) (*Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if index < 0 || index >= len(m.history) {
		return nil, fmt.Errorf("invalid rollback index")
	}

	// Get version from end (most recent first)
	version := m.history[len(m.history)-1-index]
	m.config = version.Config

	return version.Config, nil
}

// ValidateConfig validates a config
func (m *DynamicConfigManager) ValidateConfig(config *Config) error {
	return validate(config)
}

// Context key for config manager
type configManagerKey struct{}

// WithDynamicConfigManager adds config manager to context
func WithDynamicConfigManager(ctx context.Context, manager *DynamicConfigManager) context.Context {
	return context.WithValue(ctx, configManagerKey{}, manager)
}

// GetDynamicConfigManagerFromContext gets config manager from context
func GetDynamicConfigManagerFromContext(ctx context.Context) *DynamicConfigManager {
	if manager, ok := ctx.Value(configManagerKey{}).(*DynamicConfigManager); ok {
		return manager
	}
	return nil
}
