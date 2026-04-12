// Package shutdown provides graceful shutdown management for the gateway.
package shutdown

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Hook is a function that will be called during shutdown.
type Hook func(ctx context.Context) error

// Manager manages shutdown hooks and coordinates graceful shutdown.
type Manager struct {
	hooks   []Hook
	names   []string
	mu      sync.RWMutex
	started bool
	stopCh  chan struct{}
	signals chan os.Signal
}

// NewManager creates a new shutdown manager.
func NewManager() *Manager {
	return &Manager{
		hooks:   make([]Hook, 0),
		names:   make([]string, 0),
		stopCh:  make(chan struct{}),
		signals: make(chan os.Signal, 1),
	}
}

// Register registers a shutdown hook with a name.
// Hooks are executed in reverse order of registration (LIFO).
func (m *Manager) Register(name string, hook Hook) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.hooks = append(m.hooks, hook)
	m.names = append(m.names, name)
}

// RegisterFunc registers a simple function as a shutdown hook.
func (m *Manager) RegisterFunc(name string, fn func()) {
	m.Register(name, func(ctx context.Context) error {
		fn()
		return nil
	})
}

// Start begins listening for shutdown signals.
// When a signal is received, it triggers the shutdown sequence.
func (m *Manager) Start() {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	signal.Notify(m.signals, os.Interrupt, syscall.SIGTERM)
	m.mu.Unlock()

	go func() {
		select {
		case sig := <-m.signals:
			log.Printf("[shutdown] Received signal: %v", sig)
			_ = m.executeShutdown(context.Background()) // #nosec G104 -- runs in goroutine, errors logged internally
		case <-m.stopCh:
			// Manual stop requested
		}
	}()
}

// Stop stops the signal listener without executing hooks.
func (m *Manager) Stop() {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return
	}
	m.started = false
	signal.Stop(m.signals)
	m.mu.Unlock()

	close(m.stopCh)
}

// Shutdown triggers the shutdown sequence with the given context.
// Hooks are executed in reverse order of registration.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.Stop() // Stop signal listener
	return m.executeShutdown(ctx)
}

// executeShutdown executes all registered hooks.
func (m *Manager) executeShutdown(ctx context.Context) error {
	m.mu.RLock()
	hooks := make([]Hook, len(m.hooks))
	names := make([]string, len(m.names))
	copy(hooks, m.hooks)
	copy(names, m.names)
	m.mu.RUnlock()

	var errs []error

	// Execute hooks in reverse order (LIFO)
	for i := len(hooks) - 1; i >= 0; i-- {
		hook := hooks[i]
		name := names[i]

		done := make(chan error, 1)
		go func() {
			done <- hook(ctx)
		}()

		select {
		case err := <-done:
			if err != nil {
				log.Printf("[shutdown] Hook %q failed: %v", name, err)
				errs = append(errs, fmt.Errorf("hook %q: %w", name, err))
			} else {
				log.Printf("[shutdown] Hook %q completed successfully", name)
			}
		case <-ctx.Done():
			log.Printf("[shutdown] Hook %q timed out: %v", name, ctx.Err())
			errs = append(errs, fmt.Errorf("hook %q timeout: %w", name, ctx.Err()))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// HookCount returns the number of registered hooks.
func (m *Manager) HookCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.hooks)
}

// WaitForSignal blocks until a shutdown signal is received or Stop is called.
// This is useful for main goroutine blocking.
func (m *Manager) WaitForSignal() {
	<-m.stopCh
}

// Default is the global default shutdown manager.
var Default = NewManager()

// Register registers a hook on the default manager.
func Register(name string, hook Hook) {
	Default.Register(name, hook)
}

// RegisterFunc registers a simple function on the default manager.
func RegisterFunc(name string, fn func()) {
	Default.RegisterFunc(name, fn)
}

// Start starts the default manager.
func Start() {
	Default.Start()
}

// Stop stops the default manager.
func Stop() {
	Default.Stop()
}

// Shutdown triggers shutdown on the default manager.
func Shutdown(ctx context.Context) error {
	return Default.Shutdown(ctx)
}

// ShutdownWithTimeout triggers shutdown with a timeout.
func ShutdownWithTimeout(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return Default.Shutdown(ctx)
}

// WaitForSignal blocks on the default manager.
func WaitForSignal() {
	Default.WaitForSignal()
}

// HookCount returns the number of hooks on the default manager.
func HookCount() int {
	return Default.HookCount()
}
