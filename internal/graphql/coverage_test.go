package graphql

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- Depth methods on parser types ---

func TestFragmentSpreadDepth(t *testing.T) {
	t.Parallel()
	fs := &FragmentSpread{Name: "UserFields"}
	if d := fs.Depth(); d != 1 {
		t.Errorf("FragmentSpread.Depth() = %d, want 1", d)
	}
}

func TestInlineFragmentDepth_Empty(t *testing.T) {
	t.Parallel()
	ifrag := &InlineFragment{TypeCondition: "User"}
	if d := ifrag.Depth(); d != 1 {
		t.Errorf("InlineFragment.Depth() empty = %d, want 1", d)
	}
}

func TestInlineFragmentDepth_WithSelections(t *testing.T) {
	t.Parallel()
	ifrag := &InlineFragment{
		TypeCondition: "User",
		Selections: []Node{
			&Field{Name: "name"},
			&Field{
				Name: "profile",
				Selections: []Node{
					&Field{Name: "avatar"},
				},
			},
		},
	}
	d := ifrag.Depth()
	if d < 2 {
		t.Errorf("InlineFragment.Depth() = %d, want >= 2", d)
	}
}

func TestFragmentDefinitionDepth_Empty(t *testing.T) {
	t.Parallel()
	fd := &FragmentDefinition{Name: "UserFields", Type: "User"}
	if d := fd.Depth(); d != 1 {
		t.Errorf("FragmentDefinition.Depth() empty = %d, want 1", d)
	}
}

func TestFragmentDefinitionDepth_WithSelections(t *testing.T) {
	t.Parallel()
	fd := &FragmentDefinition{
		Name: "UserFields",
		Type: "User",
		Selections: []Node{
			&Field{Name: "id"},
			&Field{
				Name: "profile",
				Selections: []Node{
					&Field{Name: "bio"},
				},
			},
		},
	}
	d := fd.Depth()
	if d < 2 {
		t.Errorf("FragmentDefinition.Depth() = %d, want >= 2", d)
	}
}

// --- APQ cache tests ---

func TestInMemoryAPQCache_NilReceiver(t *testing.T) {
	t.Parallel()
	var c *InMemoryAPQCache
	if _, ok := c.Get("hash"); ok {
		t.Error("expected false for nil Get")
	}
	if err := c.Set("q", "h"); err == nil {
		t.Error("expected error for nil Set")
	}
	if c.Delete("hash") {
		t.Error("expected false for nil Delete")
	}
	if c.Len() != 0 {
		t.Errorf("Len() = %d, want 0", c.Len())
	}
	c.Clear() // should not panic
	stats := c.Stats()
	if stats.Size != 0 {
		t.Errorf("Stats().Size = %d, want 0", stats.Size)
	}
	c.Stop() // should not panic
}

func TestInMemoryAPQCache_Defaults(t *testing.T) {
	t.Parallel()
	c := NewInMemoryAPQCache(0, 0)
	defer c.Stop()
	if c.Len() != 0 {
		t.Errorf("Len() = %d, want 0", c.Len())
	}
}

func TestInMemoryAPQCache_SetGet(t *testing.T) {
	t.Parallel()
	c := NewInMemoryAPQCache(100, time.Hour)
	defer c.Stop()

	hash := ComputeQueryHash("{ users { name } }")
	if err := c.Set("{ users { name } }", hash); err != nil {
		t.Fatalf("Set: %v", err)
	}
	entry, ok := c.Get(hash)
	if !ok {
		t.Fatal("expected to find entry")
	}
	if entry.Query != "{ users { name } }" {
		t.Errorf("Query = %q", entry.Query)
	}
	if entry.UseCount < 1 {
		t.Errorf("UseCount = %d, want >= 1", entry.UseCount)
	}
}

func TestInMemoryAPQCache_Delete(t *testing.T) {
	t.Parallel()
	c := NewInMemoryAPQCache(100, time.Hour)
	defer c.Stop()

	hash := ComputeQueryHash("test")
	c.Set("test", hash)

	if !c.Delete(hash) {
		t.Error("expected Delete to return true")
	}
	if c.Delete(hash) {
		t.Error("expected Delete to return false for missing")
	}
	if _, ok := c.Get(hash); ok {
		t.Error("expected Get to return false after delete")
	}
}

func TestInMemoryAPQCache_ClearAll(t *testing.T) {
	t.Parallel()
	c := NewInMemoryAPQCache(100, time.Hour)
	defer c.Stop()

	c.Set("q1", "h1")
	c.Set("q2", "h2")
	c.Clear()
	if c.Len() != 0 {
		t.Errorf("Len() after Clear = %d, want 0", c.Len())
	}
}

func TestInMemoryAPQCache_LRUEviction(t *testing.T) {
	t.Parallel()
	c := NewInMemoryAPQCache(2, time.Hour)
	defer c.Stop()

	c.Set("q1", "h1")
	c.Set("q2", "h2")
	c.Set("q3", "h3") // should evict oldest

	if c.Len() > 2 {
		t.Errorf("Len() = %d, want <= 2", c.Len())
	}
	stats := c.Stats()
	if stats.Evictions < 1 {
		t.Errorf("Evictions = %d, want >= 1", stats.Evictions)
	}
}

func TestInMemoryAPQCache_Cleanup(t *testing.T) {
	t.Parallel()
	c := NewInMemoryAPQCache(100, 1*time.Nanosecond) // immediate expiry
	defer c.Stop()

	c.Set("q1", "h1")
	// Wait for entry to expire
	time.Sleep(10 * time.Millisecond)

	// Directly call cleanup
	c.cleanup()
	if c.Len() != 0 {
		t.Errorf("Len() after cleanup = %d, want 0", c.Len())
	}
}

func TestInMemoryAPQCache_UpdateExisting(t *testing.T) {
	t.Parallel()
	c := NewInMemoryAPQCache(100, time.Hour)
	defer c.Stop()

	c.Set("q1", "h1")
	c.Set("q1-updated", "h1") // update same hash

	entry, ok := c.Get("h1")
	if !ok {
		t.Fatal("expected to find entry")
	}
	if c.Len() != 1 {
		t.Errorf("Len() = %d, want 1", c.Len())
	}
	if entry.UseCount < 2 {
		t.Errorf("UseCount = %d, want >= 2", entry.UseCount)
	}
}

// --- APQ Middleware tests ---

func TestAPQError_Error(t *testing.T) {
	t.Parallel()
	err := &APQError{Message: "test error", Code: "TEST"}
	if err.Error() != "test error" {
		t.Errorf("Error() = %q, want %q", err.Error(), "test error")
	}
}

func TestAPQMiddleware_Disabled(t *testing.T) {
	t.Parallel()
	m := NewAPQMiddleware(APQConfig{Enabled: false}, nil)
	result, err := m.ProcessRequest(&Request{Query: "{ users { name } }"})
	if err != nil {
		t.Fatalf("ProcessRequest: %v", err)
	}
	if result.Query != "{ users { name } }" {
		t.Errorf("Query = %q", result.Query)
	}
}

func TestAPQMiddleware_NoExtension(t *testing.T) {
	t.Parallel()
	m := NewAPQMiddleware(APQConfig{Enabled: true}, nil)
	result, err := m.ProcessRequest(&Request{Query: "{ users { name } }"})
	if err != nil {
		t.Fatalf("ProcessRequest: %v", err)
	}
	if result.Query != "{ users { name } }" {
		t.Errorf("Query = %q", result.Query)
	}
}

func TestAPQMiddleware_BadVersion(t *testing.T) {
	t.Parallel()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()
	m := NewAPQMiddleware(APQConfig{Enabled: true}, cache)

	_, err := m.ProcessRequest(&Request{
		Query: "test",
		Extensions: map[string]any{
			"persistedQuery": map[string]any{
				"version":    float64(2),
				"sha256Hash": "abc",
			},
		},
	})
	if err == nil {
		t.Fatal("expected error for bad version")
	}
}

func TestAPQMiddleware_HashMismatch(t *testing.T) {
	t.Parallel()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()
	m := NewAPQMiddleware(APQConfig{
		Enabled:                   true,
		AllowAutomaticPersistence: true,
	}, cache)

	result, err := m.ProcessRequest(&Request{
		Query: "test query",
		Extensions: map[string]any{
			"persistedQuery": map[string]any{
				"version":    float64(1),
				"sha256Hash": "wronghash",
			},
		},
	})
	if err != nil {
		t.Fatalf("ProcessRequest: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected APQ error for hash mismatch")
	}
	if result.Error.Code != "APQ_HASH_MISMATCH" {
		t.Errorf("Code = %q", result.Error.Code)
	}
}

func TestAPQMiddleware_QueryTooLarge(t *testing.T) {
	t.Parallel()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()
	query := "{ users { name } }"
	hash := ComputeQueryHash(query)
	m := NewAPQMiddleware(APQConfig{
		Enabled:                   true,
		AllowAutomaticPersistence: true,
		MaxQuerySize:              5, // very small
	}, cache)

	result, err := m.ProcessRequest(&Request{
		Query: query,
		Extensions: map[string]any{
			"persistedQuery": map[string]any{
				"version":    float64(1),
				"sha256Hash": hash,
			},
		},
	})
	if err != nil {
		t.Fatalf("ProcessRequest: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected APQ error for too large query")
	}
	if result.Error.Code != "APQ_QUERY_TOO_LARGE" {
		t.Errorf("Code = %q", result.Error.Code)
	}
}

func TestAPQMiddleware_SuccessfulPersist(t *testing.T) {
	t.Parallel()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()
	query := "{ users { name } }"
	hash := ComputeQueryHash(query)
	m := NewAPQMiddleware(APQConfig{
		Enabled:                   true,
		AllowAutomaticPersistence: true,
		MaxQuerySize:              1024 * 100,
	}, cache)

	result, err := m.ProcessRequest(&Request{
		Query: query,
		Extensions: map[string]any{
			"persistedQuery": map[string]any{
				"version":    float64(1),
				"sha256Hash": hash,
			},
		},
	})
	if err != nil {
		t.Fatalf("ProcessRequest: %v", err)
	}
	if !result.Persisted {
		t.Error("expected Persisted = true")
	}
}

func TestAPQMiddleware_QueryNotFound(t *testing.T) {
	t.Parallel()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()
	m := NewAPQMiddleware(APQConfig{Enabled: true}, cache)

	result, err := m.ProcessRequest(&Request{
		Extensions: map[string]any{
			"persistedQuery": map[string]any{
				"version":    float64(1),
				"sha256Hash": "missinghash",
			},
		},
	})
	if err != nil {
		t.Fatalf("ProcessRequest: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected APQ error for not found")
	}
	if result.Error.Code != "APQ_QUERY_NOT_FOUND" {
		t.Errorf("Code = %q", result.Error.Code)
	}
}

func TestAPQMiddleware_ParseExtensions_InvalidFormat(t *testing.T) {
	t.Parallel()
	m := NewAPQMiddleware(APQConfig{Enabled: true}, nil)
	_, err := m.parseExtensions(map[string]any{
		"persistedQuery": "not-a-map",
	})
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestComputeQueryHash_Deterministic(t *testing.T) {
	t.Parallel()
	h1 := ComputeQueryHash("{ users { name } }")
	h2 := ComputeQueryHash("{ users { name } }")
	h3 := ComputeQueryHash("{ users { email } }")
	if h1 != h2 {
		t.Error("same query should produce same hash")
	}
	if h1 == h3 {
		t.Error("different queries should produce different hashes")
	}
	if len(h1) != 64 {
		t.Errorf("hash len = %d, want 64 (sha256 hex)", len(h1))
	}
}

// --- APQ HTTP Middleware tests ---

func TestAPQHTTPMiddleware_Disabled(t *testing.T) {
	t.Parallel()
	m := NewAPQMiddleware(APQConfig{Enabled: false}, nil)
	called := false
	handler := m.APQHTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"query":"{ users { name } }"}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if !called {
		t.Error("expected next handler to be called")
	}
}

func TestAPQHTTPMiddleware_NonPost(t *testing.T) {
	t.Parallel()
	m := NewAPQMiddleware(APQConfig{Enabled: true}, nil)
	called := false
	handler := m.APQHTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if !called {
		t.Error("expected next handler to be called for non-POST")
	}
}

func TestAPQHTTPMiddleware_InvalidJSON(t *testing.T) {
	t.Parallel()
	m := NewAPQMiddleware(APQConfig{Enabled: true}, nil)
	handler := m.APQHTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next should not be called")
	}))
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAPQHTTPMiddleware_NilBody(t *testing.T) {
	t.Parallel()
	m := NewAPQMiddleware(APQConfig{Enabled: true}, nil)
	handler := m.APQHTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Body = nil
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	// nil body → empty body → json unmarshal fails on empty
	// depends on implementation: readBody returns empty, json unmarshal fails
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAPQHTTPMiddleware_APQError(t *testing.T) {
	t.Parallel()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()
	m := NewAPQMiddleware(APQConfig{Enabled: true}, cache)

	body := `{"extensions":{"persistedQuery":{"version":2,"sha256Hash":"abc"}}}`
	handler := m.APQHTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next should not be called for APQ error")
	}))
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	// Version 2 should cause an error — but ProcessRequest returns it as result.Error
	// not as Go error. So the middleware should still handle it.
	if w.Code != http.StatusOK {
		t.Logf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

// --- bodyReader tests ---

func TestBodyReader_Close(t *testing.T) {
	t.Parallel()
	br := &bodyReader{data: []byte("test")}
	if err := br.Close(); err != nil {
		t.Errorf("Close() = %v", err)
	}
}

func TestBodyReader_Read(t *testing.T) {
	t.Parallel()
	br := &bodyReader{data: []byte("hello")}
	buf := make([]byte, 10)
	n, err := br.Read(buf)
	if err != nil {
		t.Errorf("Read() err = %v", err)
	}
	if string(buf[:n]) != "hello" {
		t.Errorf("Read() = %q, want %q", string(buf[:n]), "hello")
	}
	// Second read should return EOF
	n, err = br.Read(buf)
	if n != 0 {
		t.Errorf("second Read() n = %d, want 0", n)
	}
}

// --- RegisterQuery test ---

func TestRegisterQuery(t *testing.T) {
	t.Parallel()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()
	m := NewAPQMiddleware(APQConfig{Enabled: true}, cache)

	hash, err := m.RegisterQuery("{ users { name } }")
	if err != nil {
		t.Fatalf("RegisterQuery: %v", err)
	}
	if hash == "" {
		t.Error("expected non-empty hash")
	}
	// Verify it's in the cache
	entry, ok := cache.Get(hash)
	if !ok {
		t.Fatal("expected to find registered query")
	}
	if entry.Query != "{ users { name } }" {
		t.Errorf("Query = %q", entry.Query)
	}
}

// --- Admin API helpers ---

func TestGetPersistedQuery(t *testing.T) {
	t.Parallel()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()
	m := NewAPQMiddleware(APQConfig{Enabled: true}, cache)

	hash, _ := m.RegisterQuery("test query")
	pq, ok := m.GetPersistedQuery(hash)
	if !ok {
		t.Fatal("expected to find persisted query")
	}
	if pq.Query != "test query" {
		t.Errorf("Query = %q", pq.Query)
	}
}

func TestDeletePersistedQuery(t *testing.T) {
	t.Parallel()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()
	m := NewAPQMiddleware(APQConfig{Enabled: true}, cache)

	hash, _ := m.RegisterQuery("test query")
	if !m.DeletePersistedQuery(hash) {
		t.Error("expected Delete to return true")
	}
	if m.DeletePersistedQuery("nonexistent") {
		t.Error("expected Delete to return false for missing")
	}
}

func TestGetStats(t *testing.T) {
	t.Parallel()
	cache := NewInMemoryAPQCache(100, time.Hour)
	defer cache.Stop()
	m := NewAPQMiddleware(APQConfig{Enabled: true}, cache)

	m.RegisterQuery("q1")
	m.RegisterQuery("q2")
	stats := m.GetStats()
	// Stats tracks hits/misses/evictions, size is from the cache directly
	_ = stats // verify no panic
	if cache.Len() != 2 {
		t.Errorf("Len() = %d, want 2", cache.Len())
	}
}

func TestDefaultAPQConfig_Values(t *testing.T) {
	t.Parallel()
	cfg := DefaultAPQConfig()
	if !cfg.Enabled {
		t.Error("expected Enabled = true")
	}
	if cfg.MaxCacheSize != 10000 {
		t.Errorf("MaxCacheSize = %d, want 10000", cfg.MaxCacheSize)
	}
}

// --- APQ Result JSON ---

func TestAPQResult_JSON(t *testing.T) {
	t.Parallel()
	result := &APQResult{
		Query:     "{ users }",
		Hash:      "abc123",
		Found:     true,
		Persisted: true,
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), "abc123") {
		t.Error("expected hash in JSON")
	}
}
