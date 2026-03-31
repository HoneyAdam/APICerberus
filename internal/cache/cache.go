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

// Cache represents an in-memory HTTP response cache.
type Cache struct {
	mu         sync.RWMutex
	entries    map[string]*CacheEntry
	maxSize    int64
	currentSize int64
	ttl        time.Duration
}

// CacheEntry represents a cached HTTP response.
type CacheEntry struct {
	Key        string
	StatusCode int
	Headers    http.Header
	Body       []byte
	CreatedAt  time.Time
	Expiration time.Time
	Size       int64
}

// Config holds cache configuration.
type Config struct {
	Enabled      bool          `yaml:"enabled"`
	MaxSize      int64         `yaml:"max_size"`      // Max cache size in bytes
	TTL          time.Duration `yaml:"ttl"`           // Default TTL
	MaxItemSize  int64         `yaml:"max_item_size"` // Max size for single item
	KeyPrefix    string        `yaml:"key_prefix"`
}

// DefaultConfig returns default cache configuration.
func DefaultConfig() *Config {
	return &Config{
		Enabled:     true,
		MaxSize:     100 * 1024 * 1024, // 100MB
		TTL:         5 * time.Minute,
		MaxItemSize: 10 * 1024 * 1024, // 10MB
		KeyPrefix:   "apicerberus",
	}
}

// New creates a new cache with the given configuration.
func New(config *Config) *Cache {
	if config == nil {
		config = DefaultConfig()
	}

	return &Cache{
		entries: make(map[string]*CacheEntry),
		maxSize: config.MaxSize,
		ttl:     config.TTL,
	}
}

// Get retrieves a cached response for the given request.
func (c *Cache) Get(req *http.Request) (*CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := c.generateKey(req)
	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}

	// Check if expired
	if time.Now().After(entry.Expiration) {
		return nil, false
	}

	return entry, true
}

// Set stores a response in the cache.
func (c *Cache) Set(req *http.Request, statusCode int, headers http.Header, body []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if item is too large
	itemSize := int64(len(body))
	if itemSize > c.maxSize/10 { // Max 10% of total cache
		return
	}

	// Evict expired entries first
	c.evictExpired()

	// Evict oldest entries if necessary
	for c.currentSize+itemSize > c.maxSize && len(c.entries) > 0 {
		c.evictOldest()
	}

	key := c.generateKey(req)

	// Remove old entry if exists
	if old, ok := c.entries[key]; ok {
		c.currentSize -= old.Size
	}

	entry := &CacheEntry{
		Key:        key,
		StatusCode: statusCode,
		Headers:    cloneHeaders(headers),
		Body:       body,
		CreatedAt:  time.Now(),
		Expiration:  time.Now().Add(ttl),
		Size:       itemSize,
	}

	c.entries[key] = entry
	c.currentSize += itemSize
}

// Delete removes a cached entry.
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.entries[key]; ok {
		c.currentSize -= entry.Size
		delete(c.entries, key)
	}
}

// Clear clears all cached entries.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*CacheEntry)
	c.currentSize = 0
}

// Stats returns cache statistics.
func (c *Cache) Stats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]interface{}{
		"entries":      len(c.entries),
		"current_size": c.currentSize,
		"max_size":     c.maxSize,
		"utilization":  float64(c.currentSize) / float64(c.maxSize) * 100,
	}
}

// generateKey generates a cache key from the request.
func (c *Cache) generateKey(req *http.Request) string {
	// Include method, path, query string, and vary headers
	keyParts := []string{
		req.Method,
		req.URL.Path,
		req.URL.RawQuery,
	}

	// Add vary headers if present
	varyHeaders := []string{"Accept", "Accept-Encoding", "Accept-Language", "Authorization"}
	for _, header := range varyHeaders {
		if value := req.Header.Get(header); value != "" {
			keyParts = append(keyParts, fmt.Sprintf("%s=%s", header, value))
		}
	}

	hasher := sha256.New()
	hasher.Write([]byte(strings.Join(keyParts, "|")))
	return hex.EncodeToString(hasher.Sum(nil))
}

// evictExpired removes expired entries.
func (c *Cache) evictExpired() {
	now := time.Now()
	for key, entry := range c.entries {
		if now.After(entry.Expiration) {
			c.currentSize -= entry.Size
			delete(c.entries, key)
		}
	}
}

// evictOldest removes the oldest entry.
func (c *Cache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.entries {
		if oldestKey == "" || entry.CreatedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.CreatedAt
		}
	}

	if oldestKey != "" {
		c.currentSize -= c.entries[oldestKey].Size
		delete(c.entries, oldestKey)
	}
}

// cloneHeaders creates a deep copy of HTTP headers.
func cloneHeaders(headers http.Header) http.Header {
	clone := make(http.Header)
	for key, values := range headers {
		clone[key] = append([]string(nil), values...)
	}
	return clone
}

// CanCache determines if a request/response can be cached based on HTTP cache headers.
type CanCache struct {
	Request  *http.Request
	Response *http.Response
}

// ShouldCache returns true if the response should be cached.
func (cc *CanCache) ShouldCache() bool {
	if cc.Request == nil || cc.Response == nil {
		return false
	}

	// Check request method
	if cc.Request.Method != http.MethodGet && cc.Request.Method != http.MethodHead {
		return false
	}

	// Check Cache-Control headers on request
	if cc.Request.Header.Get("Cache-Control") == "no-store" {
		return false
	}

	// Check Cache-Control headers on response
	cacheControl := cc.Response.Header.Get("Cache-Control")
	if strings.Contains(cacheControl, "no-store") || strings.Contains(cacheControl, "no-cache") {
		return false
	}

	// Check for Authorization header (unless public)
	if cc.Request.Header.Get("Authorization") != "" && !strings.Contains(cacheControl, "public") {
		return false
	}

	// Check response status code
	switch cc.Response.StatusCode {
	case http.StatusOK, http.StatusNonAuthoritativeInfo, http.StatusNoContent,
		http.StatusPartialContent, http.StatusMultipleChoices,
		http.StatusMovedPermanently, http.StatusFound,
		http.StatusNotFound, http.StatusGone:
		return true
	default:
		return false
	}
}

// GetTTL returns the TTL based on Cache-Control headers.
func (cc *CanCache) GetTTL(defaultTTL time.Duration) time.Duration {
	if cc.Response == nil {
		return defaultTTL
	}

	cacheControl := cc.Response.Header.Get("Cache-Control")

	// Parse max-age
	if idx := strings.Index(cacheControl, "max-age="); idx != -1 {
		end := idx + len("max-age=")
		for end < len(cacheControl) && cacheControl[end] >= '0' && cacheControl[end] <= '9' {
			end++
		}
		if seconds, err := parseInt(cacheControl[idx+len("max-age="):end]); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}

	// Check Expires header
	if expires := cc.Response.Header.Get("Expires"); expires != "" {
		if expTime, err := http.ParseTime(expires); err == nil {
			return time.Until(expTime)
		}
	}

	return defaultTTL
}

// parseInt parses an integer from a string.
func parseInt(s string) (int64, error) {
	var result int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			break
		}
		result = result*10 + int64(ch-'0')
	}
	if result == 0 && len(s) > 0 && s[0] != '0' {
		return 0, fmt.Errorf("invalid number")
	}
	return result, nil
}

// CacheMiddleware wraps an HTTP handler with caching.
type CacheMiddleware struct {
	cache  *Cache
	next   http.Handler
	config *Config
}

// NewCacheMiddleware creates a new cache middleware.
func NewCacheMiddleware(cache *Cache, next http.Handler, config *Config) *CacheMiddleware {
	return &CacheMiddleware{
		cache:  cache,
		next:   next,
		config: config,
	}
}

// ServeHTTP implements the http.Handler interface.
func (cm *CacheMiddleware) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if !cm.config.Enabled {
		cm.next.ServeHTTP(w, req)
		return
	}

	// Try to get from cache
	if entry, ok := cm.cache.Get(req); ok {
		// Write cached response
		for key, values := range entry.Headers {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.Header().Set("X-Cache", "HIT")
		w.WriteHeader(entry.StatusCode)
		w.Write(entry.Body)
		return
	}

	// Serve and capture response
	w.Header().Set("X-Cache", "MISS")

	// Wrap response writer to capture
	recorder := &responseRecorder{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		headers:        make(http.Header),
	}

	cm.next.ServeHTTP(recorder, req)

	// Cache the response if cacheable
	cc := &CanCache{
		Request:  req,
		Response: &http.Response{
			StatusCode: recorder.statusCode,
			Header:     recorder.headers,
		},
	}

	if cc.ShouldCache() {
		ttl := cc.GetTTL(cm.config.TTL)
		cm.cache.Set(req, recorder.statusCode, recorder.headers, recorder.body, ttl)
	}
}

// responseRecorder records HTTP responses for caching.
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	headers    http.Header
	body       []byte
	written    bool
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.statusCode = code
	rr.ResponseWriter.WriteHeader(code)
}

func (rr *responseRecorder) Write(p []byte) (int, error) {
	if !rr.written {
		rr.body = append(rr.body, p...)
		rr.written = true
	}
	return rr.ResponseWriter.Write(p)
}

func (rr *responseRecorder) Header() http.Header {
	return rr.ResponseWriter.Header()
}
