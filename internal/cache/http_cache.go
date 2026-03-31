package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HTTPCache provides response caching with TTL
type HTTPCache struct {
	mu       sync.RWMutex
	entries  map[string]*CacheEntry
	maxSize  int
	maxItems int
	stats    CacheStats
}

// CacheStats holds cache statistics
type CacheStats struct {
	Hits       uint64
	Misses     uint64
	Evictions  uint64
	TotalItems uint64
	BytesSaved uint64
}

// CacheConfig holds cache configuration
type CacheConfig struct {
	Enabled       bool          `yaml:"enabled" json:"enabled"`
	MaxSize       int           `yaml:"max_size" json:"max_size"` // bytes
	MaxItems      int           `yaml:"max_items" json:"max_items"`
	DefaultTTL    time.Duration `yaml:"default_ttl" json:"default_ttl"`
	VaryByHeaders []string      `yaml:"vary_by_headers" json:"vary_by_headers"`
}

// DefaultCacheConfig returns default cache config
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		Enabled:       true,
		MaxSize:       100 * 1024 * 1024, // 100MB
		MaxItems:      10000,
		DefaultTTL:    5 * time.Minute,
		VaryByHeaders: []string{"Accept", "Accept-Encoding", "Accept-Language"},
	}
}

// NewHTTPCache creates a new HTTP cache
func NewHTTPCache(config CacheConfig) *HTTPCache {
	cache := &HTTPCache{
		entries:  make(map[string]*CacheEntry),
		maxSize:  config.MaxSize,
		maxItems: config.MaxItems,
	}

	// Start cleanup goroutine
	go cache.cleanup()

	return cache
}

// Get retrieves a cached response
func (c *HTTPCache) Get(key string) (*CacheEntry, bool) {
	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists {
		c.stats.Misses++
		return nil, false
	}

	// Check expiration
	if time.Now().After(entry.Expiration) {
		c.mu.Lock()
		delete(c.entries, key)
		c.stats.Evictions++
		c.mu.Unlock()
		c.stats.Misses++
		return nil, false
	}

	c.stats.Hits++
	return entry, true
}

// Set stores a response in cache
func (c *HTTPCache) Set(key string, entry *CacheEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict
	if len(c.entries) >= c.maxItems {
		c.evictOldest()
	}

	// Check size limit
	entrySize := len(entry.Body)
	if entrySize > c.maxSize/10 { // Don't cache items > 10% of max
		return fmt.Errorf("entry too large: %d bytes", entrySize)
	}

	c.entries[key] = entry
	c.stats.TotalItems++
	c.stats.BytesSaved += uint64(entrySize)

	return nil
}

// Delete removes an entry from cache
func (c *HTTPCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.entries[key]; exists {
		c.stats.BytesSaved -= uint64(len(entry.Body))
		delete(c.entries, key)
	}
}

// Clear clears all cache entries
func (c *HTTPCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*CacheEntry)
	c.stats.TotalItems = 0
	c.stats.BytesSaved = 0
}

// GetStats returns cache statistics
func (c *HTTPCache) GetStats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

// evictOldest removes oldest entries
func (c *HTTPCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.entries {
		if oldestKey == "" || entry.Expiration.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.Expiration
		}
	}

	if oldestKey != "" {
		if entry := c.entries[oldestKey]; entry != nil {
			c.stats.BytesSaved -= uint64(len(entry.Body))
		}
		delete(c.entries, oldestKey)
		c.stats.Evictions++
	}
}

// cleanup periodically removes expired entries
func (c *HTTPCache) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, entry := range c.entries {
			if now.After(entry.Expiration) {
				c.stats.BytesSaved -= uint64(len(entry.Body))
				delete(c.entries, key)
				c.stats.Evictions++
			}
		}
		c.mu.Unlock()
	}
}

// GenerateKey generates a cache key from request
func GenerateKey(r *http.Request, varyByHeaders []string) string {
	var parts []string

	// Method and URL
	parts = append(parts, r.Method)
	parts = append(parts, r.URL.String())

	// Vary by headers
	for _, header := range varyByHeaders {
		if value := r.Header.Get(header); value != "" {
			parts = append(parts, header+"="+value)
		}
	}

	key := strings.Join(parts, "|")
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// IsCacheable checks if a request/response is cacheable
func IsCacheable(req *http.Request, resp *http.Response) bool {
	// Only cache GET and HEAD
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		return false
	}

	// Don't cache if Cache-Control: no-store
	if strings.Contains(resp.Header.Get("Cache-Control"), "no-store") {
		return false
	}

	// Don't cache if Authorization header present
	if req.Header.Get("Authorization") != "" {
		return false
	}

	// Only cache successful responses
	if resp.StatusCode != http.StatusOK {
		return false
	}

	return true
}

// ParseCacheControl parses Cache-Control header
func ParseCacheControl(header string) map[string]string {
	directives := make(map[string]string)
	parts := strings.Split(header, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if idx := strings.Index(part, "="); idx != -1 {
			directives[part[:idx]] = part[idx+1:]
		} else {
			directives[part] = ""
		}
	}

	return directives
}

// GetTTLFromHeader extracts TTL from Cache-Control
func GetTTLFromHeader(resp *http.Response, defaultTTL time.Duration) time.Duration {
	cacheControl := resp.Header.Get("Cache-Control")
	directives := ParseCacheControl(cacheControl)

	// Check max-age
	if maxAge, ok := directives["max-age"]; ok {
		if seconds, err := time.ParseDuration(maxAge + "s"); err == nil {
			return seconds
		}
	}

	// Check s-maxage (shared cache)
	if sMaxAge, ok := directives["s-maxage"]; ok {
		if seconds, err := time.ParseDuration(sMaxAge + "s"); err == nil {
			return seconds
		}
	}

	// Check Expires header
	if expires := resp.Header.Get("Expires"); expires != "" {
		if expTime, err := http.ParseTime(expires); err == nil {
			return time.Until(expTime)
		}
	}

	return defaultTTL
}

// Global cache instance
var globalCache = NewHTTPCache(DefaultCacheConfig())

// GetGlobalCache returns the global cache instance
func GetGlobalCache() *HTTPCache {
	return globalCache
}

// SetGlobalCache sets the global cache instance
func SetGlobalCache(cache *HTTPCache) {
	globalCache = cache
}
