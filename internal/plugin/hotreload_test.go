package plugin

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestHotReloaderSetRegistry(t *testing.T) {
	t.Parallel()

	h := NewHotReloader(nil)
	if h.Registry() == nil {
		t.Fatal("expected default registry to be set")
	}

	newReg := NewRegistry()
	h.SetRegistry(newReg)

	if h.Registry() != newReg {
		t.Fatal("registry should be swapped")
	}
}

func TestHotReloaderRegisterUnregister(t *testing.T) {
	t.Parallel()

	h := NewHotReloader(nil)

	// Register a factory.
	err := h.Register("test-plugin", func(spec config.PluginConfig, ctx BuilderContext) (PipelinePlugin, error) {
		return PipelinePlugin{name: "test-plugin"}, nil
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Lookup should succeed.
	factory, ok := h.Lookup("test-plugin")
	if !ok {
		t.Fatal("expected factory to be found")
	}
	if factory == nil {
		t.Fatal("factory should not be nil")
	}

	// Unregister.
	h.Unregister("test-plugin")
	_, ok = h.Lookup("test-plugin")
	if ok {
		t.Fatal("expected factory to be removed")
	}
}

func TestHotReloaderConcurrentAccess(t *testing.T) {
	t.Parallel()

	h := NewHotReloader(nil)
	var wg sync.WaitGroup
	errCh := make(chan error, 100)

	for i := 0; i < 50; i++ {
		for j := 0; j < 2; j++ {
			idx := i*2 + j
			wg.Add(1)
			go func(n string) {
				defer wg.Done()
				if err := h.Register(n, func(spec config.PluginConfig, ctx BuilderContext) (PipelinePlugin, error) {
					return PipelinePlugin{name: n}, nil
				}); err != nil {
					errCh <- err
				}
			}(fmt.Sprintf("concurrent-plugin-%d", idx))
		}
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent error: %v", err)
	}
}

func TestHotReloaderWatchConfig(t *testing.T) {
	t.Parallel()

	h := NewHotReloader(nil)
	now := time.Now()
	h.WatchConfig("/path/to/config.yaml", now, "abc123")

	h.mu.RLock()
	w, ok := h.watchers["/path/to/config.yaml"]
	h.mu.RUnlock()

	if !ok {
		t.Fatal("expected watcher to be registered")
	}
	if w.ModTime != now {
		t.Fatalf("modtime mismatch")
	}
	if w.Hash != "abc123" {
		t.Fatalf("hash mismatch")
	}
}

func TestHotReloaderCheckChanges(t *testing.T) {
	t.Parallel()

	h := NewHotReloader(nil)
	now := time.Now()

	h.mu.Lock()
	h.watchers["/a/config.yaml"] = &ConfigWatcher{Path: "/a/config.yaml", ModTime: now, Hash: "hash-a"}
	h.watchers["/b/config.yaml"] = &ConfigWatcher{Path: "/b/config.yaml", ModTime: now, Hash: "hash-b"}
	h.mu.Unlock()

	// Case 1: No changes.
	current := map[string]ConfigState{
		"/a/config.yaml": {ModTime: now, Hash: "hash-a"},
		"/b/config.yaml": {ModTime: now, Hash: "hash-b"},
	}
	changed := h.CheckChanges(current)
	if len(changed) != 0 {
		t.Fatalf("expected no changes, got %v", changed)
	}

	// Case 2: One path changed.
	current = map[string]ConfigState{
		"/a/config.yaml": {ModTime: now, Hash: "hash-a"},
		"/b/config.yaml": {ModTime: now.Add(time.Hour), Hash: "new-hash"},
	}
	changed = h.CheckChanges(current)
	if len(changed) != 1 || changed[0] != "/b/config.yaml" {
		t.Fatalf("expected only /b/config.yaml changed, got %v", changed)
	}

	// Case 3: Path removed.
	current = map[string]ConfigState{
		"/a/config.yaml": {ModTime: now, Hash: "hash-a"},
	}
	changed = h.CheckChanges(current)
	if len(changed) != 1 || changed[0] != "/b/config.yaml" {
		t.Fatalf("expected /b/config.yaml (removed), got %v", changed)
	}
}

func TestHotReloaderReload(t *testing.T) {
	t.Parallel()

	var changedPaths []string
	h := NewHotReloader(func(path string) {
		changedPaths = append(changedPaths, path)
	})

	_ = h.Registry()
	newReg := NewRegistry()
	h.Reload([]string{"/a/config.yaml", "/b/config.yaml"}, newReg)

	if h.Registry() != newReg {
		t.Fatal("registry should be swapped")
	}
	if len(changedPaths) != 2 {
		t.Fatalf("expected 2 change callbacks, got %d", len(changedPaths))
	}
	if changedPaths[0] != "/a/config.yaml" || changedPaths[1] != "/b/config.yaml" {
		t.Fatalf("unexpected callback paths: %v", changedPaths)
	}

	// Calling reload with same registry should NOT fire callbacks
	// (only fires when registry object actually changes).
	prevLen := len(changedPaths)
	h.Reload([]string{"/c/config.yaml"}, newReg)
	if len(changedPaths) != prevLen {
		t.Fatalf("callback should NOT fire for same registry object")
	}

	// Reload with different registry object should fire callbacks.
	anotherReg := NewRegistry()
	h.Reload([]string{"/c/config.yaml"}, anotherReg)
	if len(changedPaths) != prevLen+1 {
		t.Fatalf("callback should fire when registry object changes")
	}
}

func TestHotReloaderSwapRegistry(t *testing.T) {
	t.Parallel()

	var callCount int
	h := NewHotReloader(func(_ string) {
		callCount++
	})

	h.SwapRegistry(nil) // Swap with nil should set registry to nil
	h.mu.RLock()
	if h.registry != nil {
		t.Fatal("expected nil after swap to nil")
	}
	h.mu.RUnlock()

	// Swap back to a real registry.
	newReg := NewRegistry()
	h.SwapRegistry(newReg)
	h.mu.RLock()
	if h.registry != newReg {
		t.Fatal("registry should be swapped to newReg")
	}
	h.mu.RUnlock()

	if callCount != 2 {
		t.Fatalf("expected 2 callbacks (nil + non-nil swap), got %d", callCount)
	}
}

func TestHotReloaderStop(t *testing.T) {
	t.Parallel()

	h := NewHotReloader(nil)

	// Stop should not panic.
	h.Stop()
	h.Stop() // Second call is no-op.

	select {
	case <-h.stopCh:
		// Expected: channel is closed.
	default:
		t.Fatal("stop channel should be closed")
	}
}

func TestHotReloaderBuildFromSpec(t *testing.T) {
	t.Parallel()

	h := NewHotReloader(nil)

	// Register a factory first.
	err := h.Register("build-test", func(spec config.PluginConfig, ctx BuilderContext) (PipelinePlugin, error) {
		return PipelinePlugin{name: "build-test", phase: PhasePreProxy}, nil
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Build should work with the current registry.
	plugin, err := h.BuildFromSpec(config.PluginConfig{Name: "build-test"}, BuilderContext{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if plugin.Name() != "build-test" {
		t.Fatalf("expected name 'build-test', got %q", plugin.Name())
	}
}

func TestHotReloaderBuildFromSpecNoRegistry(t *testing.T) {
	t.Parallel()

	h := NewHotReloader(nil)
	h.mu.Lock()
	h.registry = nil
	h.mu.Unlock()

	_, err := h.BuildFromSpec(config.PluginConfig{Name: "any"}, BuilderContext{})
	if err == nil {
		t.Fatal("expected error when registry is nil")
	}
}

func TestHotReloaderResetWatchers(t *testing.T) {
	t.Parallel()

	h := NewHotReloader(nil)
	h.WatchConfig("/path/a", time.Now(), "h1")
	h.WatchConfig("/path/b", time.Now(), "h2")

	h.ResetWatchers()

	h.mu.RLock()
	if len(h.watchers) != 0 {
		t.Fatalf("expected 0 watchers, got %d", len(h.watchers))
	}
	h.mu.RUnlock()
}

func TestHotReloaderConcurrentReload(t *testing.T) {
	t.Parallel()

	h := NewHotReloader(nil)
	var wg sync.WaitGroup
	errCh := make(chan error, 200)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Reload with a new registry each time.
			h.Reload([]string{"/path"}, NewRegistry())
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Register while reload is happening.
			_ = h.Register("concurrent-plugin", func(spec config.PluginConfig, ctx BuilderContext) (PipelinePlugin, error) {
				return PipelinePlugin{name: "concurrent-plugin"}, nil
			})
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Lookup while reload is happening.
			_, _ = h.Lookup("any")
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent error: %v", err)
	}
}