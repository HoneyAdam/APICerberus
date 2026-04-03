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

// Test ConnectionPool GetStats
func TestConnectionPool_GetStats(t *testing.T) {
	t.Parallel()

	pool := NewHTTPClientPool(DefaultConnectionPoolConfig())

	// Get initial stats
	stats := pool.GetStats()

	// Verify stats has expected fields
	if stats.Active < 0 {
		t.Error("Active should be non-negative")
	}
	if stats.TotalIdle < 0 {
		t.Error("TotalIdle should be non-negative")
	}
	if stats.Gets < 0 {
		t.Error("Gets should be non-negative")
	}
	if stats.Puts < 0 {
		t.Error("Puts should be non-negative")
	}
	if stats.Misses < 0 {
		t.Error("Misses should be non-negative")
	}

	// Get a client to increment stats
	client := pool.Get()
	stats = pool.GetStats()
	if stats.Gets != 1 {
		t.Errorf("Gets = %d, want 1", stats.Gets)
	}
	if stats.Active != 1 {
		t.Errorf("Active = %d, want 1", stats.Active)
	}

	// Put it back
	pool.Put(client)
	stats = pool.GetStats()
	if stats.Puts != 1 {
		t.Errorf("Puts = %d, want 1", stats.Puts)
	}
	if stats.Active != 0 {
		t.Errorf("Active = %d, want 0", stats.Active)
	}
	if stats.TotalIdle != 1 {
		t.Errorf("TotalIdle = %d, want 1", stats.TotalIdle)
	}
}

// Test ConnectionPool Do
func TestConnectionPool_Do(t *testing.T) {
	pool := NewHTTPClientPool(DefaultConnectionPoolConfig())

	// Create a request
	req, err := http.NewRequest("GET", "http://127.0.0.1:1/test", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	// Do should attempt the request (will fail since no server is running)
	_, err = pool.Do(req)
	// We expect an error since there's no server
	if err == nil {
		t.Error("Do() should return error when request fails")
	}
}

// Test ConnectionPool DoWithTimeout
func TestConnectionPool_DoWithTimeout(t *testing.T) {
	pool := NewHTTPClientPool(DefaultConnectionPoolConfig())

	// Create a request
	req, err := http.NewRequest("GET", "http://127.0.0.1:1/test", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	// DoWithTimeout should attempt the request with timeout
	_, err = pool.DoWithTimeout(req, 100*time.Millisecond)
	// We expect an error since there's no server
	if err == nil {
		t.Error("DoWithTimeout() should return error when request fails")
	}
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
	// Create a request
	req, err := http.NewRequest("GET", "http://127.0.0.1:1/test", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	// PooledDo should attempt the request
	_, err = PooledDo(req)
	// We expect an error since there's no server
	if err == nil {
		t.Error("PooledDo() should return error when request fails")
	}
}

// Test ConnectionPool Put with nil client
func TestConnectionPool_PutNil(t *testing.T) {
	t.Parallel()

	pool := NewHTTPClientPool(DefaultConnectionPoolConfig())

	// Put nil should not panic
	pool.Put(nil)

	// Stats should not change
	stats := pool.GetStats()
	if stats.Puts != 0 {
		t.Errorf("Puts = %d, want 0", stats.Puts)
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
