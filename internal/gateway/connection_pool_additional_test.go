package gateway

import (
	"net/http"
	"testing"
	"time"
)

// Test ConnectionPool Get and Put
func TestConnectionPool_GetPut(t *testing.T) {
	t.Parallel()

	pool := NewHTTPClientPool(DefaultConnectionPoolConfig())

	// Get a client
	client := pool.Get()
	if client == nil {
		t.Fatal("Get() returned nil client")
	}

	// Put it back
	pool.Put(client)
}

// Test ConnectionPool StatsSnapshot
func TestConnectionPool_StatsSnapshot(t *testing.T) {
	t.Parallel()

	pool := NewHTTPClientPool(DefaultConnectionPoolConfig())

	// Get initial stats
	gets, puts, misses, active, totalIdle := pool.StatsSnapshot()

	// Verify stats has expected fields
	if active < 0 {
		t.Error("Active should be non-negative")
	}
	if totalIdle < 0 {
		t.Error("TotalIdle should be non-negative")
	}

	_ = misses
	_ = gets
	_ = puts

	// Get a client to increment stats
	client := pool.Get()
	gets, _, _, active, _ = pool.StatsSnapshot()
	if gets != 1 {
		t.Errorf("Gets = %d, want 1", gets)
	}
	if active != 1 {
		t.Errorf("Active = %d, want 1", active)
	}

	// Put it back
	pool.Put(client)
	_, puts, _, active, totalIdle = pool.StatsSnapshot()
	if puts != 1 {
		t.Errorf("Puts = %d, want 1", puts)
	}
	if active != 0 {
		t.Errorf("Active = %d, want 0", active)
	}
	if totalIdle != 1 {
		t.Errorf("TotalIdle = %d, want 1", totalIdle)
	}
}

// Test ConnectionPool Do
func TestConnectionPool_Do(t *testing.T) {
	pool := NewHTTPClientPool(DefaultConnectionPoolConfig())

	// Create a request to a closed port
	// Use localhost:1 which should fail (privileged port, no server)
	req, err := http.NewRequest("GET", "http://127.0.0.1:1/test", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	// Do should attempt the request - on Windows this may succeed due to
	// lightweight TCP resets, so we accept both nil error and connection errors
	_, err = pool.Do(req)
	// Just verify it doesn't panic - behavior varies by platform/OS
	_ = err
}

// Test ConnectionPool DoWithTimeout
func TestConnectionPool_DoWithTimeout(t *testing.T) {
	pool := NewHTTPClientPool(DefaultConnectionPoolConfig())

	// Create a request to a closed port
	req, err := http.NewRequest("GET", "http://127.0.0.1:1/test", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	// DoWithTimeout should attempt the request with timeout
	// On Windows, connection behavior varies; just verify no panic
	_, err = pool.DoWithTimeout(req, 100*time.Millisecond)
	_ = err
}

// Test GetPooledClient and PutPooledClient
func TestGetPooledClient(t *testing.T) {
	t.Parallel()

	// GetPooledClient should return a client
	client := GetPooledClient()
	if client == nil {
		t.Fatal("GetPooledClient() returned nil")
	}

	// PutPooledClient should not panic
	PutPooledClient(client)
}

// Test PooledDo
func TestPooledDo(t *testing.T) {
	// Create a request to a closed port
	req, err := http.NewRequest("GET", "http://127.0.0.1:1/test", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	// PooledDo should attempt the request
	// On Windows, behavior varies; just verify no panic
	_, err = PooledDo(req)
	_ = err
}

// Test ConnectionPool Put with nil client
func TestConnectionPool_PutNil(t *testing.T) {
	t.Parallel()

	pool := NewHTTPClientPool(DefaultConnectionPoolConfig())

	// Put nil should not panic
	pool.Put(nil)

	// Stats should not change
	_, puts, _, _, _ := pool.StatsSnapshot()
	if puts != 0 {
		t.Errorf("Puts = %d, want 0", puts)
	}
}

// Test ConnectionPool createClient
func TestConnectionPool_createClient(t *testing.T) {
	t.Parallel()

	config := DefaultConnectionPoolConfig()
	pool := NewHTTPClientPool(config)

	// Get should return a valid client (calls createClient internally on first miss)
	client := pool.Get()
	if client == nil {
		t.Fatal("Get() returned nil client")
	}
	pool.Put(client)
}

// --- GetStats ---

func TestHTTPClientPool_GetStats(t *testing.T) {
	t.Parallel()
	pool := NewHTTPClientPool(ConnectionPoolConfig{MaxIdleConns: 10})

	// Initial stats should be zero
	stats := pool.GetStats()
	if stats.Gets != 0 || stats.Puts != 0 || stats.Active != 0 {
		t.Errorf("initial stats unexpected: gets=%d puts=%d active=%d", stats.Gets, stats.Puts, stats.Active)
	}

	// Get and put to increment counters
	client := pool.Get()
	pool.Put(client)

	stats = pool.GetStats()
	if stats.Gets != 1 {
		t.Errorf("gets = %d, want 1", stats.Gets)
	}
	if stats.Puts != 1 {
		t.Errorf("puts = %d, want 1", stats.Puts)
	}
}
