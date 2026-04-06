package graphql

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDefaultAPQConfig(t *testing.T) {
	cfg := DefaultAPQConfig()

	if !cfg.Enabled {
		t.Error("expected Enabled to be true")
	}
	if cfg.MaxCacheSize != 10000 {
		t.Errorf("expected MaxCacheSize 10000, got %d", cfg.MaxCacheSize)
	}
	if cfg.MaxQuerySize != 100*1024 {
		t.Errorf("expected MaxQuerySize %d, got %d", 100*1024, cfg.MaxQuerySize)
	}
	if cfg.TTL != 24*time.Hour {
		t.Errorf("expected TTL 24h, got %v", cfg.TTL)
	}
	if !cfg.AllowAutomaticPersistence {
		t.Error("expected AllowAutomaticPersistence to be true")
	}
	if cfg.HashAlgorithm != "sha256" {
		t.Errorf("expected HashAlgorithm sha256, got %s", cfg.HashAlgorithm)
	}
}

func TestComputeQueryHash(t *testing.T) {
	query := "{ users { id name } }"
	hash := ComputeQueryHash(query)

	if hash == "" {
		t.Error("expected non-empty hash")
	}

	// Hash should be consistent
	hash2 := ComputeQueryHash(query)
	if hash != hash2 {
		t.Error("expected hash to be consistent")
	}

	// Different queries should have different hashes
	differentQuery := "{ users { id } }"
	differentHash := ComputeQueryHash(differentQuery)
	if hash == differentHash {
		t.Error("expected different hashes for different queries")
	}

	// Whitespace should be trimmed
	queryWithSpace := "  { users { id name } }  "
	hashWithSpace := ComputeQueryHash(queryWithSpace)
	if hash != hashWithSpace {
		t.Error("expected hash to trim whitespace")
	}
}

func TestInMemoryAPQCache(t *testing.T) {
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()

	query := "{ users { id } }"
	hash := ComputeQueryHash(query)

	// Test Set and Get
	err := cache.Set(query, hash)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	entry, found := cache.Get(hash)
	if !found {
		t.Error("expected to find entry")
	}
	if entry.Query != query {
		t.Errorf("expected query %q, got %q", query, entry.Query)
	}
	if entry.Hash != hash {
		t.Errorf("expected hash %q, got %q", hash, entry.Hash)
	}
	if entry.UseCount != 1 {
		t.Errorf("expected UseCount 1, got %d", entry.UseCount)
	}

	// Test Get updates LastUsed and UseCount
	time.Sleep(10 * time.Millisecond)
	entry2, found := cache.Get(hash)
	if !found {
		t.Error("expected to find entry on second get")
	}
	if entry2.UseCount != 2 {
		t.Errorf("expected UseCount 2, got %d", entry2.UseCount)
	}
	if !entry2.LastUsed.After(entry.LastUsed) {
		t.Error("expected LastUsed to be updated")
	}

	// Test Len
	if cache.Len() != 1 {
		t.Errorf("expected Len 1, got %d", cache.Len())
	}

	// Test Delete
	if !cache.Delete(hash) {
		t.Error("expected Delete to return true")
	}
	if cache.Len() != 0 {
		t.Errorf("expected Len 0 after delete, got %d", cache.Len())
	}

	// Test Get after delete
	_, found = cache.Get(hash)
	if found {
		t.Error("expected entry to be deleted")
	}

	// Test Delete non-existent
	if cache.Delete("nonexistent") {
		t.Error("expected Delete to return false for non-existent hash")
	}
}

func TestInMemoryAPQCache_Stats(t *testing.T) {
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()

	// Initial stats
	stats := cache.Stats()
	if stats.Size != 0 {
		t.Errorf("expected initial Size 0, got %d", stats.Size)
	}

	// Add entry and get it
	query := "{ users { id } }"
	hash := ComputeQueryHash(query)
	cache.Set(query, hash)
	cache.Get(hash)
	cache.Get(hash)

	stats = cache.Stats()
	if stats.Hits != 2 {
		t.Errorf("expected 2 hits, got %d", stats.Hits)
	}

	// Miss
	cache.Get("nonexistent")
	stats = cache.Stats()
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}
}

func TestInMemoryAPQCache_Eviction(t *testing.T) {
	cache := NewInMemoryAPQCache(2, time.Hour)
	defer cache.Stop()

	// Add 3 entries (max is 2)
	for i := 0; i < 3; i++ {
		query := strings.Repeat("a", i+1)
		hash := ComputeQueryHash(query)
		cache.Set(query, hash)
	}

	if cache.Len() != 2 {
		t.Errorf("expected Len 2 after eviction, got %d", cache.Len())
	}

	stats := cache.Stats()
	if stats.Evictions != 1 {
		t.Errorf("expected 1 eviction, got %d", stats.Evictions)
	}
}

func TestInMemoryAPQCache_TTL(t *testing.T) {
	cache := NewInMemoryAPQCache(100, 50*time.Millisecond)
	defer cache.Stop()

	query := "{ users { id } }"
	hash := ComputeQueryHash(query)
	cache.Set(query, hash)

	// Should find immediately
	_, found := cache.Get(hash)
	if !found {
		t.Error("expected to find entry immediately")
	}

	// Wait for TTL to expire
	time.Sleep(100 * time.Millisecond)

	// Should not find after TTL
	_, found = cache.Get(hash)
	if found {
		t.Error("expected entry to expire after TTL")
	}
}

func TestInMemoryAPQCache_Clear(t *testing.T) {
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()

	// Add entries
	for i := 0; i < 5; i++ {
		query := strings.Repeat("a", i+1)
		hash := ComputeQueryHash(query)
		cache.Set(query, hash)
	}

	if cache.Len() != 5 {
		t.Errorf("expected Len 5, got %d", cache.Len())
	}

	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("expected Len 0 after clear, got %d", cache.Len())
	}
}

func TestAPQMiddleware_ProcessRequest_NoAPQ(t *testing.T) {
	config := DefaultAPQConfig()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()

	middleware := NewAPQMiddleware(config, cache)

	// Request without APQ extension
	req := &Request{
		Query: "{ users { id } }",
	}

	result, err := middleware.ProcessRequest(req)
	if err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	if result.Query != req.Query {
		t.Errorf("expected query %q, got %q", req.Query, result.Query)
	}
	if result.Found {
		t.Error("expected Found to be false")
	}
	if result.Error != nil {
		t.Errorf("expected no error, got %v", result.Error)
	}
}

func TestAPQMiddleware_ProcessRequest_WithQueryAndHash(t *testing.T) {
	config := DefaultAPQConfig()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()

	middleware := NewAPQMiddleware(config, cache)

	query := "{ users { id } }"
	hash := ComputeQueryHash(query)

	req := &Request{
		Query: query,
		Extensions: map[string]interface{}{
			"persistedQuery": map[string]interface{}{
				"version":    float64(1),
				"sha256Hash": hash,
			},
		},
	}

	result, err := middleware.ProcessRequest(req)
	if err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}

	if result.Query != query {
		t.Errorf("expected query %q, got %q", query, result.Query)
	}
	if result.Hash != hash {
		t.Errorf("expected hash %q, got %q", hash, result.Hash)
	}
	if !result.Found {
		t.Error("expected Found to be true")
	}
	if !result.Persisted {
		t.Error("expected Persisted to be true")
	}

	// Verify it was cached
	_, found := cache.Get(hash)
	if !found {
		t.Error("expected query to be cached")
	}
}

func TestAPQMiddleware_ProcessRequest_HashOnly(t *testing.T) {
	config := DefaultAPQConfig()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()

	middleware := NewAPQMiddleware(config, cache)

	// First, persist the query
	query := "{ users { id } }"
	hash := ComputeQueryHash(query)
	cache.Set(query, hash)

	// Now request with hash only
	req := &Request{
		Query: "", // No query
		Extensions: map[string]interface{}{
			"persistedQuery": map[string]interface{}{
				"version":    float64(1),
				"sha256Hash": hash,
			},
		},
	}

	result, err := middleware.ProcessRequest(req)
	if err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}

	if result.Query != query {
		t.Errorf("expected query %q, got %q", query, result.Query)
	}
	if !result.Found {
		t.Error("expected Found to be true")
	}
	if result.Persisted {
		t.Error("expected Persisted to be false (already existed)")
	}
}

func TestAPQMiddleware_ProcessRequest_QueryNotFound(t *testing.T) {
	config := DefaultAPQConfig()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()

	middleware := NewAPQMiddleware(config, cache)

	// Request with hash only but not persisted
	req := &Request{
		Query: "",
		Extensions: map[string]interface{}{
			"persistedQuery": map[string]interface{}{
				"version":    float64(1),
				"sha256Hash": "abc123",
			},
		},
	}

	result, err := middleware.ProcessRequest(req)
	if err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	if result.Error == nil {
		t.Fatal("expected error for non-existent query")
	}

	if result.Error.Code != "APQ_QUERY_NOT_FOUND" {
		t.Errorf("expected error code APQ_QUERY_NOT_FOUND, got %s", result.Error.Code)
	}
}

func TestAPQMiddleware_ProcessRequest_HashMismatch(t *testing.T) {
	config := DefaultAPQConfig()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()

	middleware := NewAPQMiddleware(config, cache)

	query := "{ users { id } }"
	wrongHash := "wronghash123"

	req := &Request{
		Query: query,
		Extensions: map[string]interface{}{
			"persistedQuery": map[string]interface{}{
				"version":    float64(1),
				"sha256Hash": wrongHash,
			},
		},
	}

	result, err := middleware.ProcessRequest(req)
	if err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	if result.Error == nil {
		t.Fatal("expected error for hash mismatch")
	}

	if result.Error.Code != "APQ_HASH_MISMATCH" {
		t.Errorf("expected error code APQ_HASH_MISMATCH, got %s", result.Error.Code)
	}
}

func TestAPQMiddleware_ProcessRequest_UnsupportedVersion(t *testing.T) {
	config := DefaultAPQConfig()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()

	middleware := NewAPQMiddleware(config, cache)

	req := &Request{
		Extensions: map[string]interface{}{
			"persistedQuery": map[string]interface{}{
				"version":    float64(2), // Unsupported version
				"sha256Hash": "abc123",
			},
		},
	}

	_, err := middleware.ProcessRequest(req)
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}

	apqErr, ok := err.(*APQError)
	if !ok {
		t.Fatalf("expected APQError, got %T", err)
	}

	if apqErr.Code != "APQ_VERSION_ERROR" {
		t.Errorf("expected error code APQ_VERSION_ERROR, got %s", apqErr.Code)
	}
}

func TestAPQMiddleware_ProcessRequest_QueryTooLarge(t *testing.T) {
	config := DefaultAPQConfig()
	config.MaxQuerySize = 10 // Very small
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()

	middleware := NewAPQMiddleware(config, cache)

	query := "{ users { id name email } }" // Longer than 10 bytes
	hash := ComputeQueryHash(query)

	req := &Request{
		Query: query,
		Extensions: map[string]interface{}{
			"persistedQuery": map[string]interface{}{
				"version":    float64(1),
				"sha256Hash": hash,
			},
		},
	}

	result, err := middleware.ProcessRequest(req)
	if err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	if result.Error == nil {
		t.Fatal("expected error for query too large")
	}

	if result.Error.Code != "APQ_QUERY_TOO_LARGE" {
		t.Errorf("expected error code APQ_QUERY_TOO_LARGE, got %s", result.Error.Code)
	}
}

func TestAPQMiddleware_ProcessRequest_Disabled(t *testing.T) {
	config := DefaultAPQConfig()
	config.Enabled = false
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()

	middleware := NewAPQMiddleware(config, cache)

	req := &Request{
		Query: "{ users { id } }",
	}

	result, err := middleware.ProcessRequest(req)
	if err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	if result.Query != req.Query {
		t.Errorf("expected query %q, got %q", req.Query, result.Query)
	}
}

func TestAPQMiddleware_ProcessRequest_NoAutomaticPersistence(t *testing.T) {
	config := DefaultAPQConfig()
	config.AllowAutomaticPersistence = false
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()

	middleware := NewAPQMiddleware(config, cache)

	query := "{ users { id } }"
	hash := ComputeQueryHash(query)

	req := &Request{
		Query: query,
		Extensions: map[string]interface{}{
			"persistedQuery": map[string]interface{}{
				"version":    float64(1),
				"sha256Hash": hash,
			},
		},
	}

	result, err := middleware.ProcessRequest(req)
	if err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}

	if result.Persisted {
		t.Error("expected Persisted to be false when automatic persistence is disabled")
	}

	// Verify it was NOT cached
	_, found := cache.Get(hash)
	if found {
		t.Error("expected query NOT to be cached when automatic persistence is disabled")
	}
}

func TestAPQMiddleware_APQHTTPMiddleware(t *testing.T) {
	config := DefaultAPQConfig()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()

	middleware := NewAPQMiddleware(config, cache)

	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read body and verify query was resolved
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)

		var req Request
		json.Unmarshal(body, &req)

		if req.Query == "" {
			t.Error("expected query to be resolved")
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":{"users":[]}}`))
	})

	handler := middleware.APQHTTPMiddleware(testHandler)

	// Test with hash only (after persisting)
	query := "{ users { id } }"
	hash := ComputeQueryHash(query)
	cache.Set(query, hash)

	reqBody := map[string]interface{}{
		"query": "",
		"extensions": map[string]interface{}{
			"persistedQuery": map[string]interface{}{
				"version":    1,
				"sha256Hash": hash,
			},
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestAPQMiddleware_APQHTTPMiddleware_APQError(t *testing.T) {
	config := DefaultAPQConfig()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()

	middleware := NewAPQMiddleware(config, cache)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called when APQ has error")
	})

	handler := middleware.APQHTTPMiddleware(testHandler)

	// Request with hash only but not persisted
	reqBody := map[string]interface{}{
		"query": "",
		"extensions": map[string]interface{}{
			"persistedQuery": map[string]interface{}{
				"version":    1,
				"sha256Hash": "nonexistent",
			},
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200 (GraphQL returns 200 even for errors), got %d", rr.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	errors, ok := response["errors"].([]interface{})
	if !ok || len(errors) == 0 {
		t.Fatal("expected errors in response")
	}
}

func TestAPQMiddleware_APQHTTPMiddleware_NonPost(t *testing.T) {
	config := DefaultAPQConfig()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()

	middleware := NewAPQMiddleware(config, cache)

	called := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.APQHTTPMiddleware(testHandler)

	req := httptest.NewRequest(http.MethodGet, "/graphql", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected handler to be called for GET request")
	}
}

func TestAPQMiddleware_APQHTTPMiddleware_Disabled(t *testing.T) {
	config := DefaultAPQConfig()
	config.Enabled = false
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()

	middleware := NewAPQMiddleware(config, cache)

	called := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.APQHTTPMiddleware(testHandler)

	reqBody := map[string]interface{}{
		"query": "{ users { id } }",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected handler to be called when APQ is disabled")
	}
}

func TestAPQMiddleware_RegisterQuery(t *testing.T) {
	config := DefaultAPQConfig()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()

	middleware := NewAPQMiddleware(config, cache)

	query := "{ users { id } }"
	hash, err := middleware.RegisterQuery(query)
	if err != nil {
		t.Fatalf("RegisterQuery failed: %v", err)
	}

	expectedHash := ComputeQueryHash(query)
	if hash != expectedHash {
		t.Errorf("expected hash %q, got %q", expectedHash, hash)
	}

	// Verify it was cached
	_, found := cache.Get(hash)
	if !found {
		t.Error("expected query to be cached after registration")
	}
}

func TestAPQMiddleware_DeletePersistedQuery(t *testing.T) {
	config := DefaultAPQConfig()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()

	middleware := NewAPQMiddleware(config, cache)

	query := "{ users { id } }"
	hash := ComputeQueryHash(query)
	cache.Set(query, hash)

	if !middleware.DeletePersistedQuery(hash) {
		t.Error("expected DeletePersistedQuery to return true")
	}

	_, found := cache.Get(hash)
	if found {
		t.Error("expected query to be deleted")
	}
}

func TestAPQMiddleware_GetPersistedQuery(t *testing.T) {
	config := DefaultAPQConfig()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()

	middleware := NewAPQMiddleware(config, cache)

	query := "{ users { id } }"
	hash := ComputeQueryHash(query)
	cache.Set(query, hash)

	entry, found := middleware.GetPersistedQuery(hash)
	if !found {
		t.Fatal("expected to find query")
	}

	if entry.Query != query {
		t.Errorf("expected query %q, got %q", query, entry.Query)
	}

	_, found = middleware.GetPersistedQuery("nonexistent")
	if found {
		t.Error("expected not to find non-existent query")
	}
}

func TestAPQMiddleware_GetStats(t *testing.T) {
	config := DefaultAPQConfig()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()

	middleware := NewAPQMiddleware(config, cache)

	// Add and retrieve entry
	query := "{ users { id } }"
	hash := ComputeQueryHash(query)
	cache.Set(query, hash)
	cache.Get(hash)

	stats := middleware.GetStats()
	if stats.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", stats.Hits)
	}
}

func TestInMemoryAPQCache_Nil(t *testing.T) {
	var cache *InMemoryAPQCache

	// These should not panic
	_, found := cache.Get("hash")
	if found {
		t.Error("expected Get on nil cache to return false")
	}

	err := cache.Set("query", "hash")
	if err == nil {
		t.Error("expected Set on nil cache to return error")
	}

	if cache.Delete("hash") {
		t.Error("expected Delete on nil cache to return false")
	}

	if cache.Len() != 0 {
		t.Error("expected Len on nil cache to return 0")
	}

	cache.Clear() // Should not panic

	stats := cache.Stats()
	if stats.Size != 0 {
		t.Error("expected Stats on nil cache to return empty stats")
	}

	cache.Stop() // Should not panic
}

func TestAPQMiddleware_NilCache(t *testing.T) {
	config := DefaultAPQConfig()

	// Should create default cache
	middleware := NewAPQMiddleware(config, nil)
	if middleware.cache == nil {
		t.Error("expected middleware to create default cache")
	}
}
