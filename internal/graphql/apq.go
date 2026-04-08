package graphql

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// APQConfig configures the Automatic Persisted Queries feature.
type APQConfig struct {
	// Enabled enables APQ support
	Enabled bool `json:"enabled"`

	// MaxCacheSize is the maximum number of persisted queries to store
	MaxCacheSize int `json:"max_cache_size"`

	// MaxQuerySize is the maximum query size that can be persisted (bytes)
	MaxQuerySize int `json:"max_query_size"`

	// TTL is how long to keep persisted queries in cache
	TTL time.Duration `json:"ttl"`

	// AllowAutomaticPersistence allows clients to persist queries automatically
	// If false, queries must be pre-registered via admin API
	AllowAutomaticPersistence bool `json:"allow_automatic_persistence"`

	// HashAlgorithm specifies the hash algorithm (currently only "sha256")
	HashAlgorithm string `json:"hash_algorithm"`
}

// DefaultAPQConfig returns the default APQ configuration.
func DefaultAPQConfig() APQConfig {
	return APQConfig{
		Enabled:                   true,
		MaxCacheSize:              10000,
		MaxQuerySize:              100 * 1024, // 100KB
		TTL:                       24 * time.Hour,
		AllowAutomaticPersistence: true,
		HashAlgorithm:             "sha256",
	}
}

// PersistedQuery represents an automatic persisted query entry.
type PersistedQuery struct {
	Query     string    `json:"query"`
	Hash      string    `json:"hash"`
	CreatedAt time.Time `json:"created_at"`
	LastUsed  time.Time `json:"last_used"`
	UseCount  int64     `json:"use_count"`
}

// APQExtensions represents the extensions.persistedQuery field in GraphQL requests.
type APQExtensions struct {
	Version    int    `json:"version"`
	Sha256Hash string `json:"sha256Hash"`
}

// APQCache is the interface for APQ storage backends.
type APQCache interface {
	// Get retrieves a persisted query by hash
	Get(hash string) (*PersistedQuery, bool)

	// Set stores a persisted query
	Set(query string, hash string) error

	// Delete removes a persisted query
	Delete(hash string) bool

	// Len returns the number of cached queries
	Len() int

	// Clear removes all persisted queries
	Clear()

	// Stats returns cache statistics
	Stats() APQStats
}

// APQStats holds cache statistics.
type APQStats struct {
	Size      int   `json:"size"`
	Hits      int64 `json:"hits"`
	Misses    int64 `json:"misses"`
	Evictions int64 `json:"evictions"`
}

// InMemoryAPQCache implements an in-memory APQ cache with LRU eviction.
type InMemoryAPQCache struct {
	mu      sync.RWMutex
	entries map[string]*PersistedQuery
	lruList []string // Simple LRU: most recent at end
	maxSize int
	ttl     time.Duration
	stats   APQStats
	stopCh  chan struct{}
}

// NewInMemoryAPQCache creates a new in-memory APQ cache.
func NewInMemoryAPQCache(maxSize int, ttl time.Duration) *InMemoryAPQCache {
	if maxSize <= 0 {
		maxSize = 10000
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	cache := &InMemoryAPQCache{
		entries: make(map[string]*PersistedQuery),
		lruList: make([]string, 0, maxSize),
		maxSize: maxSize,
		ttl:     ttl,
		stopCh:  make(chan struct{}),
	}

	// Start cleanup goroutine
	go cache.cleanupLoop()

	return cache
}

// Get retrieves a persisted query by hash.
func (c *InMemoryAPQCache) Get(hash string) (*PersistedQuery, bool) {
	if c == nil {
		return nil, false
	}

	c.mu.RLock()
	entry, ok := c.entries[hash]
	c.mu.RUnlock()

	if !ok {
		c.mu.Lock()
		c.stats.Misses++
		c.mu.Unlock()
		return nil, false
	}

	// Check TTL
	if time.Since(entry.LastUsed) > c.ttl {
		c.mu.Lock()
		delete(c.entries, hash)
		c.removeFromLRU(hash)
		c.stats.Misses++
		c.mu.Unlock()
		return nil, false
	}

	// Update stats and LRU
	c.mu.Lock()
	entry.LastUsed = time.Now()
	entry.UseCount++
	c.stats.Hits++
	c.moveToEnd(hash)
	c.mu.Unlock()

	return entry, true
}

// Set stores a persisted query.
func (c *InMemoryAPQCache) Set(query string, hash string) error {
	if c == nil {
		return errors.New("cache is nil")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already exists
	if existing, ok := c.entries[hash]; ok {
		existing.LastUsed = time.Now()
		existing.UseCount++
		return nil
	}

	// Evict oldest if at capacity
	for len(c.entries) >= c.maxSize && len(c.lruList) > 0 {
		oldest := c.lruList[0]
		c.lruList = c.lruList[1:]
		delete(c.entries, oldest)
		c.stats.Evictions++
	}

	now := time.Now()
	entry := &PersistedQuery{
		Query:     query,
		Hash:      hash,
		CreatedAt: now,
		LastUsed:  now,
		UseCount:  1,
	}

	c.entries[hash] = entry
	c.lruList = append(c.lruList, hash)

	return nil
}

// Delete removes a persisted query.
func (c *InMemoryAPQCache) Delete(hash string) bool {
	if c == nil {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.entries[hash]; !ok {
		return false
	}

	delete(c.entries, hash)
	c.removeFromLRU(hash)
	return true
}

// Len returns the number of cached queries.
func (c *InMemoryAPQCache) Len() int {
	if c == nil {
		return 0
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Clear removes all persisted queries.
func (c *InMemoryAPQCache) Clear() {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*PersistedQuery)
	c.lruList = c.lruList[:0]
}

// Stats returns cache statistics.
func (c *InMemoryAPQCache) Stats() APQStats {
	if c == nil {
		return APQStats{}
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

// Stop stops the cleanup goroutine.
func (c *InMemoryAPQCache) Stop() {
	if c == nil {
		return
	}
	close(c.stopCh)
}

func (c *InMemoryAPQCache) removeFromLRU(hash string) {
	for i, h := range c.lruList {
		if h == hash {
			c.lruList = append(c.lruList[:i], c.lruList[i+1:]...)
			return
		}
	}
}

func (c *InMemoryAPQCache) moveToEnd(hash string) {
	c.removeFromLRU(hash)
	c.lruList = append(c.lruList, hash)
}

func (c *InMemoryAPQCache) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCh:
			return
		}
	}
}

func (c *InMemoryAPQCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for hash, entry := range c.entries {
		if now.Sub(entry.LastUsed) > c.ttl {
			delete(c.entries, hash)
			c.removeFromLRU(hash)
		}
	}
}

// APQMiddleware handles Automatic Persisted Queries.
type APQMiddleware struct {
	config APQConfig
	cache  APQCache
}

// NewAPQMiddleware creates a new APQ middleware.
func NewAPQMiddleware(config APQConfig, cache APQCache) *APQMiddleware {
	if cache == nil {
		cache = NewInMemoryAPQCache(config.MaxCacheSize, config.TTL)
	}
	return &APQMiddleware{
		config: config,
		cache:  cache,
	}
}

// APQResult represents the result of APQ processing.
type APQResult struct {
	// Query is the full query text (either from request or cache)
	Query string

	// Hash is the query hash
	Hash string

	// Found indicates if the query was found in cache
	Found bool

	// Persisted indicates if the query was persisted in this request
	Persisted bool

	// Error is set if there was an APQ error
	Error *APQError
}

// APQError represents an APQ-specific error.
type APQError struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

// Error implements the error interface.
func (e *APQError) Error() string {
	return e.Message
}

// ProcessRequest processes a GraphQL request for APQ.
func (m *APQMiddleware) ProcessRequest(req *Request) (*APQResult, error) {
	if !m.config.Enabled {
		return &APQResult{Query: req.Query}, nil
	}

	// Parse extensions
	apqExt, err := m.parseExtensions(req.Extensions)
	if err != nil {
		return nil, err
	}

	// No APQ extension, process normally
	if apqExt == nil {
		return &APQResult{Query: req.Query}, nil
	}

	// Validate version
	if apqExt.Version != 1 {
		return nil, &APQError{
			Message: fmt.Sprintf("unsupported APQ version: %d", apqExt.Version),
			Code:    "APQ_VERSION_ERROR",
		}
	}

	hash := apqExt.Sha256Hash

	// Client sent query text - validate and potentially persist
	if req.Query != "" {
		// Validate hash matches query
		computedHash := ComputeQueryHash(req.Query)
		if computedHash != hash {
			return &APQResult{
				Error: &APQError{
					Message: "provided sha256Hash does not match query",
					Code:    "APQ_HASH_MISMATCH",
				},
			}, nil
		}

		// Persist the query if allowed
		if m.config.AllowAutomaticPersistence {
			if len(req.Query) > m.config.MaxQuerySize {
				return &APQResult{
					Error: &APQError{
						Message: fmt.Sprintf("query exceeds maximum size of %d bytes", m.config.MaxQuerySize),
						Code:    "APQ_QUERY_TOO_LARGE",
					},
				}, nil
			}

			if err := m.cache.Set(req.Query, hash); err != nil {
				return nil, err
			}
		}

		return &APQResult{
			Query:     req.Query,
			Hash:      hash,
			Found:     true,
			Persisted: m.config.AllowAutomaticPersistence,
		}, nil
	}

	// Client only sent hash, look up in cache
	entry, found := m.cache.Get(hash)
	if !found {
		return &APQResult{
			Hash:  hash,
			Found: false,
			Error: &APQError{
				Message: "PersistedQueryNotFound",
				Code:    "APQ_QUERY_NOT_FOUND",
			},
		}, nil
	}

	return &APQResult{
		Query: entry.Query,
		Hash:  hash,
		Found: true,
	}, nil
}

// parseExtensions extracts APQ extensions from the request.
func (m *APQMiddleware) parseExtensions(extensions map[string]any) (*APQExtensions, error) {
	if extensions == nil {
		return nil, nil
	}

	pqRaw, ok := extensions["persistedQuery"]
	if !ok {
		return nil, nil
	}

	// Convert map[string]any to APQExtensions
	pqMap, ok := pqRaw.(map[string]any)
	if !ok {
		return nil, &APQError{
			Message: "invalid persistedQuery format",
			Code:    "APQ_FORMAT_ERROR",
		}
	}

	var apqExt APQExtensions

	if v, ok := pqMap["version"].(float64); ok {
		apqExt.Version = int(v)
	}
	if v, ok := pqMap["sha256Hash"].(string); ok {
		apqExt.Sha256Hash = v
	}

	return &apqExt, nil
}

// ComputeQueryHash computes the SHA256 hash of a query.
func ComputeQueryHash(query string) string {
	hash := sha256.Sum256([]byte(strings.TrimSpace(query)))
	return hex.EncodeToString(hash[:])
}

// APQHTTPMiddleware wraps an HTTP handler with APQ support.
func (m *APQMiddleware) APQHTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.config.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Only process POST requests with JSON body
		if r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}

		// Read and parse request body
		body, err := readBody(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var req Request
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Process APQ
		result, err := m.ProcessRequest(&req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Handle APQ errors
		if result.Error != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK) // GraphQL returns 200 even for errors

			response := map[string]any{
				"errors": []map[string]any{
					{
						"message": result.Error.Message,
						"extensions": map[string]string{
							"code": result.Error.Code,
						},
					},
				},
			}

			_ = json.NewEncoder(w).Encode(response) // #nosec G104
			return
		}

		// Update request with resolved query
		req.Query = result.Query

		// Remove APQ extension to prevent downstream processing
		if req.Extensions != nil {
			delete(req.Extensions, "persistedQuery")
		}

		// Re-serialize body
		newBody, err := json.Marshal(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Create new request with updated body
		r = setBody(r, newBody)

		next.ServeHTTP(w, r)
	})
}

// Helper functions for body manipulation
func readBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return []byte{}, nil
	}
	defer r.Body.Close()

	return io.ReadAll(r.Body)
}

func setBody(r *http.Request, body []byte) *http.Request {
	r.Body = &bodyReader{data: body}
	r.ContentLength = int64(len(body))
	return r
}

type bodyReader struct {
	data []byte
	pos  int
}

func (br *bodyReader) Read(p []byte) (n int, err error) {
	if br.pos >= len(br.data) {
		return 0, io.EOF
	}
	n = copy(p, br.data[br.pos:])
	br.pos += n
	return n, nil
}

func (br *bodyReader) Close() error {
	return nil
}

// Admin API handlers for APQ management

// ListPersistedQueries returns a list of all persisted queries.
func (m *APQMiddleware) ListPersistedQueries(limit, offset int) []*PersistedQuery {
	if m.cache == nil {
		return nil
	}

	// This is a simplified implementation
	// In production, you'd want to iterate through the cache
	return nil
}

// GetPersistedQuery retrieves a specific persisted query.
func (m *APQMiddleware) GetPersistedQuery(hash string) (*PersistedQuery, bool) {
	return m.cache.Get(hash)
}

// DeletePersistedQuery removes a persisted query.
func (m *APQMiddleware) DeletePersistedQuery(hash string) bool {
	return m.cache.Delete(hash)
}

// GetStats returns APQ statistics.
func (m *APQMiddleware) GetStats() APQStats {
	return m.cache.Stats()
}

// RegisterQuery manually registers a query with APQ.
func (m *APQMiddleware) RegisterQuery(query string) (string, error) {
	hash := ComputeQueryHash(query)
	if err := m.cache.Set(query, hash); err != nil {
		return "", err
	}
	return hash, nil
}
