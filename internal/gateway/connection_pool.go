package gateway

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"
)

// ConnectionPoolConfig holds connection pool settings
type ConnectionPoolConfig struct {
	MaxIdleConns        int           `yaml:"max_idle_conns" json:"max_idle_conns"`
	MaxIdleConnsPerHost int           `yaml:"max_idle_conns_per_host" json:"max_idle_conns_per_host"`
	MaxConnsPerHost     int           `yaml:"max_conns_per_host" json:"max_conns_per_host"`
	IdleConnTimeout     time.Duration `yaml:"idle_conn_timeout" json:"idle_conn_timeout"`
	TLSHandshakeTimeout time.Duration `yaml:"tls_handshake_timeout" json:"tls_handshake_timeout"`
	DisableKeepAlives   bool          `yaml:"disable_keep_alives" json:"disable_keep_alives"`
	DisableCompression  bool          `yaml:"disable_compression" json:"disable_compression"`
}

// DefaultConnectionPoolConfig returns sensible defaults
func DefaultConnectionPoolConfig() ConnectionPoolConfig {
	return ConnectionPoolConfig{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   false,
		DisableCompression:  false,
	}
}

// HTTPClientPool manages a pool of reusable HTTP clients
type HTTPClientPool struct {
	config ConnectionPoolConfig
	pool   sync.Pool
	mu     sync.RWMutex
	stats  PoolStats
}

// PoolStats holds pool statistics
type PoolStats struct {
	Gets      uint64
	Puts      uint64
	Misses    uint64
	Active    int64
	TotalIdle int64
}

// NewHTTPClientPool creates a new HTTP client pool
func NewHTTPClientPool(config ConnectionPoolConfig) *HTTPClientPool {
	pool := &HTTPClientPool{
		config: config,
	}

	pool.pool = sync.Pool{
		New: func() interface{} {
			pool.stats.Misses++
			return pool.createClient()
		},
	}

	return pool
}

// createClient creates a new HTTP client with optimized transport
func (p *HTTPClientPool) createClient() *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          p.config.MaxIdleConns,
		MaxIdleConnsPerHost:   p.config.MaxIdleConnsPerHost,
		MaxConnsPerHost:       p.config.MaxConnsPerHost,
		IdleConnTimeout:       p.config.IdleConnTimeout,
		TLSHandshakeTimeout:   p.config.TLSHandshakeTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     p.config.DisableKeepAlives,
		DisableCompression:    p.config.DisableCompression,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   0, // No timeout - handled by context
	}
}

// Get retrieves a client from the pool
func (p *HTTPClientPool) Get() *http.Client {
	p.stats.Gets++
	p.stats.Active++

	client := p.pool.Get().(*http.Client)
	return client
}

// Put returns a client to the pool
func (p *HTTPClientPool) Put(client *http.Client) {
	if client == nil {
		return
	}

	p.stats.Puts++
	p.stats.Active--
	p.stats.TotalIdle++

	// Reset client state before returning to pool
	client.Timeout = 0

	p.pool.Put(client)
}

// GetStats returns pool statistics
func (p *HTTPClientPool) GetStats() PoolStats {
	return p.stats
}

// Do executes an HTTP request using a pooled client
func (p *HTTPClientPool) Do(req *http.Request) (*http.Response, error) {
	client := p.Get()
	defer p.Put(client)

	return client.Do(req)
}

// DoWithTimeout executes a request with timeout
func (p *HTTPClientPool) DoWithTimeout(req *http.Request, timeout time.Duration) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(req.Context(), timeout)
	defer cancel()

	req = req.WithContext(ctx)
	return p.Do(req)
}

// Global pool instance
var globalPool = NewHTTPClientPool(DefaultConnectionPoolConfig())

// GetPooledClient gets a client from the global pool
func GetPooledClient() *http.Client {
	return globalPool.Get()
}

// PutPooledClient returns a client to the global pool
func PutPooledClient(client *http.Client) {
	globalPool.Put(client)
}

// PooledDo executes a request using the global pool
func PooledDo(req *http.Request) (*http.Response, error) {
	return globalPool.Do(req)
}

// ConnectionPoolManager manages multiple pools for different upstreams
type ConnectionPoolManager struct {
	pools map[string]*HTTPClientPool
	mu    sync.RWMutex
}

// NewConnectionPoolManager creates a new pool manager
func NewConnectionPoolManager() *ConnectionPoolManager {
	return &ConnectionPoolManager{
		pools: make(map[string]*HTTPClientPool),
	}
}

// GetPool gets or creates a pool for an upstream
func (m *ConnectionPoolManager) GetPool(upstreamID string, config ConnectionPoolConfig) *HTTPClientPool {
	m.mu.RLock()
	pool, exists := m.pools[upstreamID]
	m.mu.RUnlock()

	if exists {
		return pool
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check
	if pool, exists = m.pools[upstreamID]; exists {
		return pool
	}

	pool = NewHTTPClientPool(config)
	m.pools[upstreamID] = pool
	return pool
}

// GetAllStats returns stats for all pools
func (m *ConnectionPoolManager) GetAllStats() map[string]PoolStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]PoolStats)
	for id, pool := range m.pools {
		stats[id] = pool.GetStats()
	}
	return stats
}

// Close closes all pools
func (m *ConnectionPoolManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Clear all pools
	for id := range m.pools {
		delete(m.pools, id)
	}
}
