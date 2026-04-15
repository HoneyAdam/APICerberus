package shutdown

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	if m.HookCount() != 0 {
		t.Errorf("expected 0 hooks, got %d", m.HookCount())
	}
}

func TestManager_Register(t *testing.T) {
	m := NewManager()

	var called atomic.Bool
	m.Register("test-hook", func(ctx context.Context) error {
		called.Store(true)
		return nil
	})

	if m.HookCount() != 1 {
		t.Errorf("expected 1 hook, got %d", m.HookCount())
	}

	// Execute shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := m.Shutdown(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !called.Load() {
		t.Error("expected hook to be called")
	}
}

func TestManager_RegisterFunc(t *testing.T) {
	m := NewManager()

	var called atomic.Bool
	m.RegisterFunc("test-func", func() {
		called.Store(true)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := m.Shutdown(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !called.Load() {
		t.Error("expected func to be called")
	}
}

func TestManager_ShutdownOrder(t *testing.T) {
	m := NewManager()

	var order []string
	var mu sync.Mutex

	m.Register("first", func(ctx context.Context) error {
		mu.Lock()
		order = append(order, "first")
		mu.Unlock()
		return nil
	})

	m.Register("second", func(ctx context.Context) error {
		mu.Lock()
		order = append(order, "second")
		mu.Unlock()
		return nil
	})

	m.Register("third", func(ctx context.Context) error {
		mu.Lock()
		order = append(order, "third")
		mu.Unlock()
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := m.Shutdown(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// Hooks should execute in reverse order (LIFO)
	expected := []string{"third", "second", "first"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d hooks, got %d", len(expected), len(order))
	}

	for i, exp := range expected {
		if order[i] != exp {
			t.Errorf("hook %d: expected %q, got %q", i, exp, order[i])
		}
	}
}

func TestManager_ShutdownWithError(t *testing.T) {
	m := NewManager()

	m.Register("failing-hook", func(ctx context.Context) error {
		return errors.New("hook failed")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := m.Shutdown(ctx)
	if err == nil {
		t.Error("expected error from failing hook")
	}

	if !contains(err.Error(), "hook failed") {
		t.Errorf("expected error to contain 'hook failed', got: %v", err)
	}
}

func TestManager_ShutdownTimeout(t *testing.T) {
	m := NewManager()

	m.Register("slow-hook", func(ctx context.Context) error {
		time.Sleep(500 * time.Millisecond)
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := m.Shutdown(ctx)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestManager_StartStop(t *testing.T) {
	m := NewManager()

	// Start should not block
	m.Start()

	// Stop should complete without panic
	m.Stop()
}

func TestManager_MultipleStart(t *testing.T) {
	m := NewManager()

	// Multiple starts should be safe
	m.Start()
	m.Start()
	m.Start()

	m.Stop()
}

func TestManager_StopWithoutStart(t *testing.T) {
	m := NewManager()

	// Stop without start should be safe
	m.Stop()
}

func TestDefaultManager(t *testing.T) {
	// Reset default manager for testing
	Default = NewManager()

	var called atomic.Bool
	Register("default-hook", func(ctx context.Context) error {
		called.Store(true)
		return nil
	})

	if HookCount() != 1 {
		t.Errorf("expected 1 hook on default, got %d", HookCount())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := Shutdown(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !called.Load() {
		t.Error("expected default hook to be called")
	}
}

func TestShutdownWithTimeout(t *testing.T) {
	m := NewManager()

	var called atomic.Bool
	m.Register("quick-hook", func(ctx context.Context) error {
		called.Store(true)
		return nil
	})

	err := m.executeShutdown(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !called.Load() {
		t.Error("expected hook to be called")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// --- Package-level convenience functions ---

func TestPackageLevel_RegisterFunc(t *testing.T) {
	called := false
	RegisterFunc("test-pkg-hook", func() {
		called = true
	})
	if HookCount() < 1 {
		t.Error("expected at least one hook")
	}
	// Trigger shutdown to test hook execution
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	Shutdown(ctx)
	if !called {
		t.Error("expected hook to be called")
	}
}

func TestPackageLevel_ShutdownWithTimeout(t *testing.T) {
	// Create a fresh default manager for this test
	old := Default
	Default = NewManager()
	defer func() { Default = old }()

	Default.RegisterFunc("timeout-test", func() {})
	err := ShutdownWithTimeout(2 * time.Second)
	if err != nil {
		t.Errorf("ShutdownWithTimeout: %v", err)
	}
}
