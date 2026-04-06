package plugin

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// CacheConfig configures the cache plugin behavior.
type CacheConfig struct {
	// TTL is the default time-to-live for cache entries
	TTL time.Duration

	// MaxSize is the maximum number of entries in the cache (0 = unlimited)
	MaxSize int

	// MaxMemoryBytes is the maximum memory usage in bytes (0 = unlimited)
	MaxMemoryBytes int64

	// KeyHeaders are headers to include in cache key generation
	KeyHeaders []string

	// VaryByQuery are query parameters to vary cache by
	VaryByQuery []string

	// CacheableMethods are HTTP methods that can be cached
	CacheableMethods []string

	// CacheableStatusCodes are status codes that can be cached
	CacheableStatusCodes []int

	// ExcludePaths are path patterns that should not be cached
	ExcludePaths []string

	// WarmURLs are URLs to preload on startup
	WarmURLs []string

	// BackgroundCleanupInterval is how often to clean expired entries
	BackgroundCleanupInterval time.Duration

	// TagsEnabled enables tag-based cache invalidation
	TagsEnabled bool
}

// CacheEntry represents a cached response.
type CacheEntry struct {
	Key          string
	StatusCode   int
	Headers      http.Header
	Body         []byte
	CreatedAt    time.Time
	ExpiresAt    time.Time
	Tags         []string
	Size         int64
	hitCount     atomic.Int64
	listElement  *list.Element
}

// IsExpired returns true if the entry has expired.
func (e *CacheEntry) IsExpired(now time.Time) bool {
	if e == nil {
		return true
	}
	return now.After(e.ExpiresAt)
}

// Hit increments the hit counter and returns the new count.
func (e *CacheEntry) Hit() int64 {
	if e == nil {
		return 0
	}
	return e.hitCount.Add(1)
}

// HitCount returns the number of cache hits for this entry.
func (e *CacheEntry) HitCount() int64 {
	if e == nil {
		return 0
	}
	return e.hitCount.Load()
}

// CacheStats holds cache performance statistics.
type CacheStats struct {
	Hits       atomic.Int64
	Misses     atomic.Int64
	Evictions  atomic.Int64
	Expirations atomic.Int64
	Size       atomic.Int64
	Count      atomic.Int64
	MemoryUsed atomic.Int64
}

// Snapshot returns a copy of the current statistics.
func (s *CacheStats) Snapshot() CacheStatsSnapshot {
	if s == nil {
		return CacheStatsSnapshot{}
	}
	return CacheStatsSnapshot{
		Hits:        s.Hits.Load(),
		Misses:      s.Misses.Load(),
		Evictions:   s.Evictions.Load(),
		Expirations: s.Expirations.Load(),
		Size:        s.Size.Load(),
		Count:       s.Count.Load(),
		MemoryUsed:  s.MemoryUsed.Load(),
	}
}

// CacheStatsSnapshot is a point-in-time copy of cache statistics.
type CacheStatsSnapshot struct {
	Hits        int64
	Misses      int64
	Evictions   int64
	Expirations int64
	Size        int64
	Count       int64
	MemoryUsed  int64
}

// HitRate returns the cache hit rate as a percentage.
func (s CacheStatsSnapshot) HitRate() float64 {
	total := s.Hits + s.Misses
	if total == 0 {
		return 0
	}
	return float64(s.Hits) / float64(total) * 100
}

// Cache implements a full-featured HTTP response cache with LRU eviction,
// TTL support, tag-based invalidation, and background cleanup.
type Cache struct {
	cfg CacheConfig

	mu       sync.RWMutex
	entries  map[string]*CacheEntry
	lruList  *list.List
	stats    CacheStats
	stopCh   chan struct{}
	stopped  atomic.Bool
	excludes []*regexp.Regexp

	// warming tracks URLs currently being warmed to prevent duplicates
	warmingMu sync.Mutex
	warming   map[string]bool

	// now is overridable for testing
	now func() time.Time
}

// NewCache creates a new cache instance with the given configuration.
func NewCache(cfg CacheConfig) (*Cache, error) {
	c := &Cache{
		cfg:       cfg,
		entries:   make(map[string]*CacheEntry),
		lruList:   list.New(),
		stopCh:    make(chan struct{}),
		warming:   make(map[string]bool),
		now:       time.Now,
	}

	// Compile exclude patterns
	for _, pattern := range cfg.ExcludePaths {
		if pattern == "" {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid exclude pattern %q: %w", pattern, err)
		}
		c.excludes = append(c.excludes, re)
	}

	// Set defaults
	if c.cfg.TTL <= 0 {
		c.cfg.TTL = 5 * time.Minute
	}
	if len(c.cfg.CacheableMethods) == 0 {
		c.cfg.CacheableMethods = []string{http.MethodGet, http.MethodHead}
	}
	if len(c.cfg.CacheableStatusCodes) == 0 {
		c.cfg.CacheableStatusCodes = []int{http.StatusOK, http.StatusNonAuthoritativeInfo, http.StatusNoContent}
	}
	if c.cfg.BackgroundCleanupInterval <= 0 {
		c.cfg.BackgroundCleanupInterval = 30 * time.Second
	}

	// Normalize methods to uppercase
	for i, m := range c.cfg.CacheableMethods {
		c.cfg.CacheableMethods[i] = strings.ToUpper(strings.TrimSpace(m))
	}

	// Start background cleanup
	go c.backgroundCleanup()

	return c, nil
}

// Name returns the plugin name.
func (c *Cache) Name() string { return "cache" }

// Phase returns the plugin phase.
func (c *Cache) Phase() Phase { return PhasePostProxy }

// Priority returns the plugin priority.
func (c *Cache) Priority() int { return 40 }

// Stop stops the background cleanup goroutine.
func (c *Cache) Stop() {
	if c == nil {
		return
	}
	if c.stopped.CompareAndSwap(false, true) {
		close(c.stopCh)
	}
}

// IsStopped returns true if the cache has been stopped.
func (c *Cache) IsStopped() bool {
	if c == nil {
		return true
	}
	return c.stopped.Load()
}

// Get retrieves a cache entry by key.
func (c *Cache) Get(key string) (*CacheEntry, bool) {
	if c == nil {
		return nil, false
	}

	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists {
		c.stats.Misses.Add(1)
		return nil, false
	}

	if entry.IsExpired(c.now()) {
		c.stats.Expirations.Add(1)
		c.stats.Misses.Add(1)
		c.deleteEntry(key)
		return nil, false
	}

	c.stats.Hits.Add(1)
	entry.Hit()

	// Move to front of LRU list
	c.mu.Lock()
	if entry.listElement != nil {
		c.lruList.MoveToFront(entry.listElement)
	}
	c.mu.Unlock()

	return entry, true
}

// Set stores a response in the cache.
func (c *Cache) Set(key string, statusCode int, headers http.Header, body []byte, ttl time.Duration, tags []string) {
	if c == nil {
		return
	}

	// Check if status code is cacheable
	if !c.isCacheableStatusCode(statusCode) {
		return
	}

	// Use default TTL if not specified
	if ttl <= 0 {
		ttl = c.cfg.TTL
	}

	now := c.now()
	entry := &CacheEntry{
		Key:       key,
		StatusCode: statusCode,
		Headers:   headers.Clone(),
		Body:      body,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
		Tags:      normalizeTags(tags),
		Size:      int64(len(body) + estimateHeaderSize(headers)),
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check memory limit before adding
	if c.cfg.MaxMemoryBytes > 0 {
		currentMemory := c.stats.MemoryUsed.Load()
		if currentMemory+entry.Size > c.cfg.MaxMemoryBytes {
			// Evict entries until we have room
			c.evictUntilSpaceLocked(entry.Size)
		}
	}

	// Check count limit
	if c.cfg.MaxSize > 0 && len(c.entries) >= c.cfg.MaxSize {
		c.evictLRULocked()
	}

	// Remove old entry if exists
	if oldEntry, exists := c.entries[key]; exists {
		c.removeEntryLocked(oldEntry)
	}

	// Add to cache
	c.entries[key] = entry
	entry.listElement = c.lruList.PushFront(key)
	c.stats.Count.Add(1)
	c.stats.Size.Add(1)
	c.stats.MemoryUsed.Add(entry.Size)
}

// Delete removes a cache entry by key.
func (c *Cache) Delete(key string) bool {
	if c == nil {
		return false
	}

	c.mu.Lock()
	entry, exists := c.entries[key]
	if !exists {
		c.mu.Unlock()
		return false
	}
	c.removeEntryLocked(entry)
	c.mu.Unlock()

	return true
}

// DeleteByPattern removes all entries whose keys match the given pattern.
func (c *Cache) DeleteByPattern(pattern string) (int, error) {
	if c == nil {
		return 0, nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return 0, fmt.Errorf("invalid pattern: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	var keysToDelete []string
	for key := range c.entries {
		if re.MatchString(key) {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		if entry, exists := c.entries[key]; exists {
			c.removeEntryLocked(entry)
		}
	}

	return len(keysToDelete), nil
}

// DeleteByTag removes all entries with the given tag.
func (c *Cache) DeleteByTag(tag string) int {
	if c == nil || !c.cfg.TagsEnabled {
		return 0
	}

	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	var keysToDelete []string
	for key, entry := range c.entries {
		for _, t := range entry.Tags {
			if t == tag {
				keysToDelete = append(keysToDelete, key)
				break
			}
		}
	}

	for _, key := range keysToDelete {
		if entry, exists := c.entries[key]; exists {
			c.removeEntryLocked(entry)
		}
	}

	return len(keysToDelete)
}

// DeleteByTags removes all entries matching any of the given tags.
func (c *Cache) DeleteByTags(tags []string) int {
	if c == nil || !c.cfg.TagsEnabled || len(tags) == 0 {
		return 0
	}

	tagSet := make(map[string]bool)
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" {
			tagSet[t] = true
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	var keysToDelete []string
	for key, entry := range c.entries {
		for _, t := range entry.Tags {
			if tagSet[t] {
				keysToDelete = append(keysToDelete, key)
				break
			}
		}
	}

	for _, key := range keysToDelete {
		if entry, exists := c.entries[key]; exists {
			c.removeEntryLocked(entry)
		}
	}

	return len(keysToDelete)
}

// Clear removes all entries from the cache.
func (c *Cache) Clear() {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, entry := range c.entries {
		if entry.listElement != nil {
			c.lruList.Remove(entry.listElement)
		}
		c.stats.MemoryUsed.Add(-entry.Size)
	}

	c.entries = make(map[string]*CacheEntry)
	c.lruList.Init()
	c.stats.Count.Store(0)
	c.stats.Size.Store(0)
}

// Stats returns a snapshot of current cache statistics.
func (c *Cache) Stats() CacheStatsSnapshot {
	if c == nil {
		return CacheStatsSnapshot{}
	}
	return c.stats.Snapshot()
}

// Len returns the number of entries in the cache.
func (c *Cache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// MemoryUsed returns the approximate memory usage in bytes.
func (c *Cache) MemoryUsed() int64 {
	if c == nil {
		return 0
	}
	return c.stats.MemoryUsed.Load()
}

// WarmURL preloads a URL into the cache.
func (c *Cache) WarmURL(method, url string, headers http.Header, body []byte, ttl time.Duration, tags []string) error {
	if c == nil {
		return fmt.Errorf("cache is nil")
	}

	key := c.GenerateKey(method, url, headers)

	c.warmingMu.Lock()
	if c.warming[key] {
		c.warmingMu.Unlock()
		return nil // Already warming
	}
	c.warming[key] = true
	c.warmingMu.Unlock()

	defer func() {
		c.warmingMu.Lock()
		delete(c.warming, key)
		c.warmingMu.Unlock()
	}()

	// For warming, we simulate a 200 OK response
	c.Set(key, http.StatusOK, headers, body, ttl, tags)
	return nil
}

// WarmURLs preloads multiple URLs into the cache.
func (c *Cache) WarmURLs(urls []WarmURLSpec) []error {
	if c == nil {
		return nil
	}

	var errs []error
	for _, spec := range urls {
		if err := c.WarmURL(spec.Method, spec.URL, spec.Headers, spec.Body, spec.TTL, spec.Tags); err != nil {
			errs = append(errs, fmt.Errorf("failed to warm %s: %w", spec.URL, err))
		}
	}
	return errs
}

// WarmURLSpec specifies a URL to warm.
type WarmURLSpec struct {
	Method  string
	URL     string
	Headers http.Header
	Body    []byte
	TTL     time.Duration
	Tags    []string
}

// GenerateKey creates a cache key from request components.
func (c *Cache) GenerateKey(method, url string, headers http.Header) string {
	if c == nil {
		return ""
	}

	var parts []string
	parts = append(parts, strings.ToUpper(method))
	parts = append(parts, url)

	// Add vary-by headers
	for _, header := range c.cfg.KeyHeaders {
		header = http.CanonicalHeaderKey(strings.TrimSpace(header))
		if values, ok := headers[header]; ok && len(values) > 0 {
			sorted := make([]string, len(values))
			copy(sorted, values)
			sort.Strings(sorted)
			parts = append(parts, header+"="+strings.Join(sorted, ","))
		}
	}

	key := strings.Join(parts, "|")

	// Hash long keys
	if len(key) > 256 {
		hash := sha256.Sum256([]byte(key))
		key = hex.EncodeToString(hash[:])
	}

	return key
}

// CanCacheRequest checks if a request can be cached.
func (c *Cache) CanCacheRequest(req *http.Request) bool {
	if c == nil || req == nil {
		return false
	}

	// Check method
	method := strings.ToUpper(req.Method)
	found := false
	for _, m := range c.cfg.CacheableMethods {
		if m == method {
			found = true
			break
		}
	}
	if !found {
		return false
	}

	// Check exclude patterns
	path := req.URL.Path
	for _, re := range c.excludes {
		if re.MatchString(path) {
			return false
		}
	}

	// Check Cache-Control headers
	cacheControl := req.Header.Get("Cache-Control")
	if strings.Contains(cacheControl, "no-cache") || strings.Contains(cacheControl, "no-store") {
		return false
	}

	return true
}

// CanCacheResponse checks if a response can be cached.
func (c *Cache) CanCacheResponse(statusCode int, headers http.Header) bool {
	if c == nil {
		return false
	}

	// Check status code
	if !c.isCacheableStatusCode(statusCode) {
		return false
	}

	// Check Cache-Control headers
	cacheControl := headers.Get("Cache-Control")
	if strings.Contains(cacheControl, "no-cache") ||
		strings.Contains(cacheControl, "no-store") ||
		strings.Contains(cacheControl, "private") {
		return false
	}

	return true
}

// Apply sets up response capture for caching.
func (c *Cache) Apply(ctx *PipelineContext) {
	if c == nil || ctx == nil || ctx.Request == nil {
		return
	}

	if c.stopped.Load() {
		return
	}

	if !c.CanCacheRequest(ctx.Request) {
		return
	}

	// Check if we have a cached response
	key := c.GenerateKey(ctx.Request.Method, ctx.Request.URL.String(), ctx.Request.Header)
	if entry, found := c.Get(key); found {
		// Serve from cache
		c.serveFromCache(ctx.ResponseWriter, entry)
		ctx.Abort("served_from_cache")
		return
	}

	// Set up response capture
	if _, ok := ctx.ResponseWriter.(*CaptureResponseWriter); !ok {
		ctx.ResponseWriter = NewCaptureResponseWriter(ctx.ResponseWriter)
	}
}

// AfterProxy processes the captured response and stores it in cache.
func (c *Cache) AfterProxy(ctx *PipelineContext, proxyErr error) {
	if c == nil || ctx == nil || proxyErr != nil {
		return
	}

	if c.stopped.Load() {
		return
	}

	if ctx.Aborted && ctx.AbortReason == "served_from_cache" {
		return
	}

	capture, ok := ctx.ResponseWriter.(*CaptureResponseWriter)
	if !ok || !capture.HasCaptured() {
		return
	}

	statusCode := capture.StatusCode()
	headers := capture.Header()
	body := capture.BodyBytes()

	if !c.CanCacheResponse(statusCode, headers) {
		return
	}

	// Extract TTL from Cache-Control if present
	ttl := c.cfg.TTL
	if cc := headers.Get("Cache-Control"); cc != "" {
		if maxAge := parseMaxAge(cc); maxAge > 0 {
			ttl = maxAge
		}
	}

	key := c.GenerateKey(ctx.Request.Method, ctx.Request.URL.String(), ctx.Request.Header)

	// Extract tags from response headers if enabled
	var tags []string
	if c.cfg.TagsEnabled {
		if tagHeader := headers.Get("X-Cache-Tags"); tagHeader != "" {
			tags = strings.Split(tagHeader, ",")
		}
	}

	c.Set(key, statusCode, headers, body, ttl, tags)
}

// serveFromCache writes a cached response to the client.
func (c *Cache) serveFromCache(w http.ResponseWriter, entry *CacheEntry) {
	// Copy headers
	for key, values := range entry.Headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Add cache hit header
	w.Header().Set("X-Cache", "HIT")
	w.Header().Set("X-Cache-Hits", fmt.Sprintf("%d", entry.HitCount()))

	w.WriteHeader(entry.StatusCode)
	if len(entry.Body) > 0 {
		_, _ = w.Write(entry.Body)
	}
}

// isCacheableStatusCode checks if a status code can be cached.
func (c *Cache) isCacheableStatusCode(code int) bool {
	for _, sc := range c.cfg.CacheableStatusCodes {
		if sc == code {
			return true
		}
	}
	return false
}

// deleteEntry removes an entry from the cache (must be called with lock held or after acquiring lock).
func (c *Cache) deleteEntry(key string) {
	c.mu.Lock()
	entry, exists := c.entries[key]
	if !exists {
		c.mu.Unlock()
		return
	}
	c.removeEntryLocked(entry)
	c.mu.Unlock()
}

// removeEntryLocked removes an entry from cache data structures (must hold lock).
func (c *Cache) removeEntryLocked(entry *CacheEntry) {
	if entry.listElement != nil {
		c.lruList.Remove(entry.listElement)
	}
	delete(c.entries, entry.Key)
	c.stats.Count.Add(-1)
	c.stats.Size.Add(-1)
	c.stats.MemoryUsed.Add(-entry.Size)
}

// evictLRULocked removes the least recently used entry (must hold lock).
func (c *Cache) evictLRULocked() {
	elem := c.lruList.Back()
	if elem == nil {
		return
	}

	key, ok := elem.Value.(string)
	if !ok {
		c.lruList.Remove(elem)
		return
	}

	if entry, exists := c.entries[key]; exists {
		c.removeEntryLocked(entry)
		c.stats.Evictions.Add(1)
	}
}

// evictUntilSpaceLocked evicts entries until there's enough space (must hold lock).
func (c *Cache) evictUntilSpaceLocked(needed int64) {
	for c.stats.MemoryUsed.Load()+needed > c.cfg.MaxMemoryBytes && c.lruList.Len() > 0 {
		c.evictLRULocked()
	}
}

// backgroundCleanup periodically removes expired entries.
func (c *Cache) backgroundCleanup() {
	ticker := time.NewTicker(c.cfg.BackgroundCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanupExpired()
		case <-c.stopCh:
			return
		}
	}
}

// cleanupExpired removes all expired entries.
func (c *Cache) cleanupExpired() {
	now := c.now()

	c.mu.Lock()
	defer c.mu.Unlock()

	var expired []string
	for key, entry := range c.entries {
		if entry.IsExpired(now) {
			expired = append(expired, key)
		}
	}

	for _, key := range expired {
		if entry, exists := c.entries[key]; exists {
			c.removeEntryLocked(entry)
			c.stats.Expirations.Add(1)
		}
	}
}

// estimateHeaderSize estimates the memory size of HTTP headers.
func estimateHeaderSize(headers http.Header) int {
	size := 0
	for key, values := range headers {
		size += len(key)
		for _, v := range values {
			size += len(v)
		}
	}
	return size
}

// normalizeTags normalizes cache tags.
func normalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var result []string
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" && !seen[t] {
			seen[t] = true
			result = append(result, t)
		}
	}
	sort.Strings(result)
	return result
}

// parseMaxAge extracts max-age from Cache-Control header.
func parseMaxAge(cacheControl string) time.Duration {
	parts := strings.Split(cacheControl, ",")
	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(part))
		if strings.HasPrefix(part, "max-age=") {
			ageStr := strings.TrimPrefix(part, "max-age=")
			var age int
			if _, err := fmt.Sscanf(ageStr, "%d", &age); err == nil && age > 0 {
				return time.Duration(age) * time.Second
			}
		}
	}
	return 0
}

// CaptureResponseWriter captures response data for caching.
type CaptureResponseWriter struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
	wrote      bool
	flushed    bool
}

// NewCaptureResponseWriter creates a new capture response writer.
func NewCaptureResponseWriter(w http.ResponseWriter) *CaptureResponseWriter {
	return &CaptureResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

// WriteHeader captures the status code.
func (c *CaptureResponseWriter) WriteHeader(code int) {
	if !c.wrote {
		c.statusCode = code
		c.wrote = true
		c.ResponseWriter.WriteHeader(code)
	}
}

// Write captures the body data.
func (c *CaptureResponseWriter) Write(data []byte) (int, error) {
	if !c.wrote {
		c.WriteHeader(http.StatusOK)
	}
	c.body.Write(data)
	return c.ResponseWriter.Write(data)
}

// StatusCode returns the captured status code.
func (c *CaptureResponseWriter) StatusCode() int {
	return c.statusCode
}

// BodyBytes returns the captured body.
func (c *CaptureResponseWriter) BodyBytes() []byte {
	return c.body.Bytes()
}

// HasCaptured returns true if any data was captured.
func (c *CaptureResponseWriter) HasCaptured() bool {
	return c.wrote
}

// SetBody sets the body (used for compression/cache plugins).
func (c *CaptureResponseWriter) SetBody(body []byte) {
	c.body.Reset()
	c.body.Write(body)
}

// ReadBody reads the captured body (for testing).
func (c *CaptureResponseWriter) ReadBody() io.Reader {
	return &c.body
}

// Flush implements http.Flusher to flush the response writer.
func (c *CaptureResponseWriter) Flush() error {
	c.flushed = true
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

// IsFlushed returns true if Flush has been called.
func (c *CaptureResponseWriter) IsFlushed() bool {
	return c.flushed
}

// buildCachePlugin builds the cache plugin for the pipeline.
func buildCachePlugin(spec config.PluginConfig, _ BuilderContext) (PipelinePlugin, error) {
	cfgMap := spec.Config

	cfg := CacheConfig{
		TTL:                       asDuration(cfgMap["ttl"], 5*time.Minute),
		MaxSize:                   asInt(cfgMap["max_size"], 10000),
		MaxMemoryBytes:            int64(asInt(cfgMap["max_memory_mb"], 100)) * 1024 * 1024,
		KeyHeaders:                asStringSlice(cfgMap["key_headers"]),
		VaryByQuery:               asStringSlice(cfgMap["vary_by_query"]),
		CacheableMethods:          asStringSlice(cfgMap["cacheable_methods"]),
		CacheableStatusCodes:      asIntSlice(cfgMap["cacheable_status_codes"], []int{200, 203, 204}),
		ExcludePaths:              asStringSlice(cfgMap["exclude_paths"]),
		BackgroundCleanupInterval: asDuration(cfgMap["cleanup_interval"], 30*time.Second),
		TagsEnabled:               asBool(cfgMap["tags_enabled"], false),
	}

	cache, err := NewCache(cfg)
	if err != nil {
		return PipelinePlugin{}, err
	}

	return PipelinePlugin{
		name:     cache.Name(),
		phase:    cache.Phase(),
		priority: cache.Priority(),
		run: func(ctx *PipelineContext) (bool, error) {
			cache.Apply(ctx)
			return ctx.Aborted, nil
		},
		after: func(ctx *PipelineContext, proxyErr error) {
			cache.AfterProxy(ctx, proxyErr)
		},
	}, nil
}
