package plugin

import (
	"container/list"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

func TestNewCache(t *testing.T) {
	tests := []struct {
		name    string
		cfg     CacheConfig
		wantErr bool
	}{
		{
			name: "default config",
			cfg:  CacheConfig{},
		},
		{
			name: "with exclude patterns",
			cfg: CacheConfig{
				ExcludePaths: []string{"^/admin/.*", "^/api/internal/.*"},
			},
		},
		{
			name: "with invalid exclude pattern",
			cfg: CacheConfig{
				ExcludePaths: []string{"[invalid"},
			},
			wantErr: true,
		},
		{
			name: "with custom TTL",
			cfg: CacheConfig{
				TTL: 10 * time.Minute,
			},
		},
		{
			name: "with custom limits",
			cfg: CacheConfig{
				MaxSize:        1000,
				MaxMemoryBytes: 1024 * 1024,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, err := NewCache(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cache == nil {
				t.Fatal("expected cache, got nil")
			}
			cache.Stop()
		})
	}
}

func TestCache_BasicOperations(t *testing.T) {
	cache, err := NewCache(CacheConfig{TTL: time.Hour})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	headers := http.Header{
		"Content-Type": []string{"application/json"},
	}
	body := []byte(`{"message": "hello"}`)

	// Test Set and Get
	t.Run("set and get", func(t *testing.T) {
		cache.Set("key1", http.StatusOK, headers, body, 0, nil)

		entry, found := cache.Get("key1")
		if !found {
			t.Fatal("expected to find entry")
		}
		if entry.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", entry.StatusCode)
		}
		if string(entry.Body) != string(body) {
			t.Errorf("expected body %s, got %s", body, entry.Body)
		}
	})

	// Test Get non-existent key
	t.Run("get non-existent", func(t *testing.T) {
		_, found := cache.Get("non-existent")
		if found {
			t.Error("expected not to find entry")
		}
	})

	// Test Delete
	t.Run("delete", func(t *testing.T) {
		cache.Set("key2", http.StatusOK, headers, body, 0, nil)
		if !cache.Delete("key2") {
			t.Error("expected delete to return true")
		}
		_, found := cache.Get("key2")
		if found {
			t.Error("expected entry to be deleted")
		}
	})

	// Test Delete non-existent
	t.Run("delete non-existent", func(t *testing.T) {
		if cache.Delete("non-existent") {
			t.Error("expected delete to return false")
		}
	})
}

func TestCache_TTL(t *testing.T) {
	now := time.Now()
	cache := &Cache{
		cfg: CacheConfig{
			TTL:                  time.Hour,
			CacheableStatusCodes: []int{200, 203, 204},
		},
		entries: make(map[string]*CacheEntry),
		lruList: list.New(),
		now:     func() time.Time { return now },
		warming: make(map[string]bool),
		stopCh:  make(chan struct{}),
	}

	headers := http.Header{}
	body := []byte("test")

	// Set entry with short TTL
	cache.Set("short", http.StatusOK, headers, body, time.Second, nil)
	// Set entry with default TTL
	cache.Set("default", http.StatusOK, headers, body, 0, nil)

	// Should find both initially
	if _, found := cache.Get("short"); !found {
		t.Error("expected to find short TTL entry")
	}
	if _, found := cache.Get("default"); !found {
		t.Error("expected to find default TTL entry")
	}

	// Advance time to expire the short TTL entry
	now = now.Add(2 * time.Second)

	// Short TTL should be expired now
	if _, found := cache.Get("short"); found {
		t.Error("expected short TTL entry to be expired")
	}

	// Default TTL should still exist
	if _, found := cache.Get("default"); !found {
		t.Error("expected default TTL entry to still exist")
	}
}

func TestCache_LRU_Eviction(t *testing.T) {
	cache, err := NewCache(CacheConfig{
		TTL:     time.Hour,
		MaxSize: 3,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	headers := http.Header{}
	body := []byte("test")

	// Add 3 entries
	cache.Set("key1", http.StatusOK, headers, body, 0, nil)
	cache.Set("key2", http.StatusOK, headers, body, 0, nil)
	cache.Set("key3", http.StatusOK, headers, body, 0, nil)

	// Access key1 to make it most recently used
	cache.Get("key1")

	// Add 4th entry - should evict key2 (least recently used)
	cache.Set("key4", http.StatusOK, headers, body, 0, nil)

	// key1 should exist
	if _, found := cache.Get("key1"); !found {
		t.Error("expected key1 to exist")
	}

	// key2 should be evicted
	if _, found := cache.Get("key2"); found {
		t.Error("expected key2 to be evicted")
	}

	// key3 and key4 should exist
	if _, found := cache.Get("key3"); !found {
		t.Error("expected key3 to exist")
	}
	if _, found := cache.Get("key4"); !found {
		t.Error("expected key4 to exist")
	}

	// Check eviction stats
	stats := cache.Stats()
	if stats.Evictions < 1 {
		t.Errorf("expected at least 1 eviction, got %d", stats.Evictions)
	}
}

func TestCache_MemoryLimit(t *testing.T) {
	cache, err := NewCache(CacheConfig{
		TTL:            time.Hour,
		MaxSize:        100,
		MaxMemoryBytes: 500, // Very small limit
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	headers := http.Header{}
	largeBody := make([]byte, 200) // 200 bytes each

	// Add entries that exceed memory limit
	cache.Set("key1", http.StatusOK, headers, largeBody, 0, nil)
	cache.Set("key2", http.StatusOK, headers, largeBody, 0, nil)
	cache.Set("key3", http.StatusOK, headers, largeBody, 0, nil)

	// Due to memory limit, some entries should be evicted
	count := 0
	for _, key := range []string{"key1", "key2", "key3"} {
		if _, found := cache.Get(key); found {
			count++
		}
	}

	// Should have at most 2 entries due to memory limit (500 / 200 = 2.5)
	if count > 2 {
		t.Errorf("expected at most 2 entries due to memory limit, got %d", count)
	}
}

func TestCache_Clear(t *testing.T) {
	cache, err := NewCache(CacheConfig{TTL: time.Hour})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	headers := http.Header{}
	body := []byte("test")

	cache.Set("key1", http.StatusOK, headers, body, 0, nil)
	cache.Set("key2", http.StatusOK, headers, body, 0, nil)

	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("expected cache to be empty, got %d entries", cache.Len())
	}

	if _, found := cache.Get("key1"); found {
		t.Error("expected key1 to be cleared")
	}
}

func TestCache_DeleteByPattern(t *testing.T) {
	cache, err := NewCache(CacheConfig{TTL: time.Hour})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	headers := http.Header{}
	body := []byte("test")

	cache.Set("/api/users/1", http.StatusOK, headers, body, 0, nil)
	cache.Set("/api/users/2", http.StatusOK, headers, body, 0, nil)
	cache.Set("/api/posts/1", http.StatusOK, headers, body, 0, nil)
	cache.Set("/other", http.StatusOK, headers, body, 0, nil)

	deleted, err := cache.DeleteByPattern(`^/api/users/.*`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted != 2 {
		t.Errorf("expected 2 deletions, got %d", deleted)
	}

	if _, found := cache.Get("/api/users/1"); found {
		t.Error("expected /api/users/1 to be deleted")
	}
	if _, found := cache.Get("/api/posts/1"); !found {
		t.Error("expected /api/posts/1 to exist")
	}

	// Test invalid pattern
	_, err = cache.DeleteByPattern("[invalid")
	if err == nil {
		t.Error("expected error for invalid pattern")
	}
}

func TestCache_DeleteByTag(t *testing.T) {
	cache, err := NewCache(CacheConfig{
		TTL:         time.Hour,
		TagsEnabled: true,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	headers := http.Header{}
	body := []byte("test")

	cache.Set("key1", http.StatusOK, headers, body, 0, []string{"users", "api"})
	cache.Set("key2", http.StatusOK, headers, body, 0, []string{"users"})
	cache.Set("key3", http.StatusOK, headers, body, 0, []string{"posts", "api"})
	cache.Set("key4", http.StatusOK, headers, body, 0, nil)

	deleted := cache.DeleteByTag("users")
	if deleted != 2 {
		t.Errorf("expected 2 deletions, got %d", deleted)
	}

	if _, found := cache.Get("key1"); found {
		t.Error("expected key1 to be deleted")
	}
	if _, found := cache.Get("key2"); found {
		t.Error("expected key2 to be deleted")
	}
	if _, found := cache.Get("key3"); !found {
		t.Error("expected key3 to exist")
	}
}

func TestCache_DeleteByTags(t *testing.T) {
	cache, err := NewCache(CacheConfig{
		TTL:         time.Hour,
		TagsEnabled: true,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	headers := http.Header{}
	body := []byte("test")

	cache.Set("key1", http.StatusOK, headers, body, 0, []string{"users", "api"})
	cache.Set("key2", http.StatusOK, headers, body, 0, []string{"users"})
	cache.Set("key3", http.StatusOK, headers, body, 0, []string{"posts", "api"})

	deleted := cache.DeleteByTags([]string{"users", "posts"})
	if deleted != 3 {
		t.Errorf("expected 3 deletions, got %d", deleted)
	}

	if cache.Len() != 0 {
		t.Errorf("expected cache to be empty, got %d entries", cache.Len())
	}
}

func TestCache_Stats(t *testing.T) {
	cache, err := NewCache(CacheConfig{TTL: time.Hour})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	headers := http.Header{}
	body := []byte("test")

	// Miss
	cache.Get("key1")

	// Hit
	cache.Set("key2", http.StatusOK, headers, body, 0, nil)
	cache.Get("key2")
	cache.Get("key2")

	stats := cache.Stats()

	if stats.Hits != 2 {
		t.Errorf("expected 2 hits, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}
	if stats.Count != 1 {
		t.Errorf("expected 1 entry, got %d", stats.Count)
	}

	hitRate := stats.HitRate()
	expectedRate := 2.0 / 3.0 * 100
	if hitRate < expectedRate-0.1 || hitRate > expectedRate+0.1 {
		t.Errorf("expected hit rate %.2f, got %.2f", expectedRate, hitRate)
	}
}

func TestCache_GenerateKey(t *testing.T) {
	cache, err := NewCache(CacheConfig{
		TTL:        time.Hour,
		KeyHeaders: []string{"Accept", "Authorization"},
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	tests := []struct {
		name     string
		method   string
		url      string
		headers  http.Header
		expected string
	}{
		{
			name:     "simple GET",
			method:   "GET",
			url:      "/api/users",
			headers:  http.Header{},
			expected: "GET|/api/users",
		},
		{
			name:   "with headers",
			method: "GET",
			url:    "/api/users",
			headers: http.Header{
				"Accept": []string{"application/json"},
			},
			expected: "GET|/api/users|Accept=application/json",
		},
		{
			name:   "with multiple header values",
			method: "GET",
			url:    "/api/users",
			headers: http.Header{
				"Accept": []string{"application/json", "text/html"},
			},
			expected: "GET|/api/users|Accept=application/json,text/html",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := cache.GenerateKey(tt.method, tt.url, tt.headers)
			if key != tt.expected {
				t.Errorf("expected key %q, got %q", tt.expected, key)
			}
		})
	}
}

func TestCache_GenerateKey_Hash(t *testing.T) {
	cache, err := NewCache(CacheConfig{
		TTL:        time.Hour,
		KeyHeaders: []string{"header1", "header2", "header3"},
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	// Create a very long key that should be hashed
	longURL := "/api/" + strings.Repeat("very/long/path/", 50)
	headers := http.Header{
		"Header1": []string{strings.Repeat("value1", 100)},
	}

	key := cache.GenerateKey("GET", longURL, headers)

	// Key should be a 64-character hex string (SHA-256)
	if len(key) != 64 {
		t.Errorf("expected hashed key of length 64, got %d", len(key))
	}

	// Should be valid hex
	matched, _ := regexp.MatchString("^[a-f0-9]+$", key)
	if !matched {
		t.Error("expected key to be hexadecimal")
	}
}

func TestCache_CanCacheRequest(t *testing.T) {
	cache, err := NewCache(CacheConfig{
		TTL:              time.Hour,
		CacheableMethods: []string{"GET", "HEAD"},
		ExcludePaths:     []string{"^/admin/.*", "^/api/internal/.*"},
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	tests := []struct {
		name     string
		method   string
		path     string
		headers  http.Header
		expected bool
	}{
		{
			name:     "GET request",
			method:   "GET",
			path:     "/api/users",
			expected: true,
		},
		{
			name:     "HEAD request",
			method:   "HEAD",
			path:     "/api/users",
			expected: true,
		},
		{
			name:     "POST request",
			method:   "POST",
			path:     "/api/users",
			expected: false,
		},
		{
			name:     "excluded path admin",
			method:   "GET",
			path:     "/admin/dashboard",
			expected: false,
		},
		{
			name:     "excluded path internal",
			method:   "GET",
			path:     "/api/internal/secrets",
			expected: false,
		},
		{
			name:   "no-cache header",
			method: "GET",
			path:   "/api/users",
			headers: http.Header{
				"Cache-Control": []string{"no-cache"},
			},
			expected: false,
		},
		{
			name:   "no-store header",
			method: "GET",
			path:   "/api/users",
			headers: http.Header{
				"Cache-Control": []string{"no-store"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			for key, values := range tt.headers {
				for _, v := range values {
					req.Header.Add(key, v)
				}
			}

			result := cache.CanCacheRequest(req)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestCache_CanCacheResponse(t *testing.T) {
	cache, err := NewCache(CacheConfig{
		TTL:                  time.Hour,
		CacheableStatusCodes: []int{200, 203, 204},
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	tests := []struct {
		name       string
		statusCode int
		headers    http.Header
		expected   bool
	}{
		{
			name:       "200 OK",
			statusCode: 200,
			headers:    http.Header{},
			expected:   true,
		},
		{
			name:       "404 Not Found",
			statusCode: 404,
			headers:    http.Header{},
			expected:   false,
		},
		{
			name:       "no-cache response",
			statusCode: 200,
			headers: http.Header{
				"Cache-Control": []string{"no-cache"},
			},
			expected: false,
		},
		{
			name:       "private response",
			statusCode: 200,
			headers: http.Header{
				"Cache-Control": []string{"private"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cache.CanCacheResponse(tt.statusCode, tt.headers)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestCache_BackgroundCleanup(t *testing.T) {
	now := time.Now()
	cache := &Cache{
		cfg: CacheConfig{
			TTL:                       time.Hour,
			BackgroundCleanupInterval: 50 * time.Millisecond,
			CacheableStatusCodes:      []int{200, 203, 204},
		},
		entries: make(map[string]*CacheEntry),
		lruList: list.New(),
		stopCh:  make(chan struct{}),
		now:     func() time.Time { return now },
		warming: make(map[string]bool),
	}

	headers := http.Header{}
	body := []byte("test")

	// Add entry with short TTL
	cache.Set("short", http.StatusOK, headers, body, time.Second, nil)
	// Add valid entry with long TTL
	cache.Set("valid", http.StatusOK, headers, body, time.Hour, nil)

	if cache.Len() != 2 {
		t.Fatalf("expected 2 entries, got %d", cache.Len())
	}

	// Advance time to expire the short TTL entry
	now = now.Add(2 * time.Second)

	// Run cleanup directly
	cache.cleanupExpired()

	// Expired entry should be removed
	if cache.Len() != 1 {
		t.Errorf("expected 1 entry after cleanup, got %d", cache.Len())
	}

	// Valid entry should still exist
	if _, found := cache.Get("valid"); !found {
		t.Error("expected valid entry to exist")
	}

	// Expired entry should not exist
	if _, found := cache.Get("short"); found {
		t.Error("expected short TTL entry to be removed")
	}
}

func TestCache_WarmURL(t *testing.T) {
	cache, err := NewCache(CacheConfig{TTL: time.Hour})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	headers := http.Header{
		"Content-Type": []string{"application/json"},
	}
	body := []byte(`{"message": "warmed"}`)

	err = cache.WarmURL("GET", "/api/users", headers, body, 0, []string{"users"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	key := cache.GenerateKey("GET", "/api/users", headers)
	entry, found := cache.Get(key)
	if !found {
		t.Fatal("expected to find warmed entry")
	}
	if string(entry.Body) != string(body) {
		t.Errorf("expected body %s, got %s", body, entry.Body)
	}
}

func TestCache_WarmURL_Duplicate(t *testing.T) {
	cache, err := NewCache(CacheConfig{TTL: time.Hour})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	headers := http.Header{}
	body := []byte("test")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cache.WarmURL("GET", "/api/users", headers, body, 0, nil)
		}()
	}
	wg.Wait()

	// Should only have 1 entry
	if cache.Len() != 1 {
		t.Errorf("expected 1 entry, got %d", cache.Len())
	}
}

func TestCache_WarmURLs(t *testing.T) {
	cache, err := NewCache(CacheConfig{TTL: time.Hour})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	specs := []WarmURLSpec{
		{Method: "GET", URL: "/api/users", Body: []byte("users")},
		{Method: "GET", URL: "/api/posts", Body: []byte("posts")},
		{Method: "GET", URL: "/api/comments", Body: []byte("comments")},
	}

	errs := cache.WarmURLs(specs)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}

	if cache.Len() != 3 {
		t.Errorf("expected 3 entries, got %d", cache.Len())
	}
}

func TestCache_Apply_ServeFromCache(t *testing.T) {
	cache, err := NewCache(CacheConfig{TTL: time.Hour})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	// Pre-populate cache
	headers := http.Header{
		"Content-Type": []string{"application/json"},
	}
	body := []byte(`{"message": "cached"}`)
	cache.Set("GET|/api/users", http.StatusOK, headers, body, 0, nil)

	// Create request
	req := httptest.NewRequest("GET", "/api/users", nil)
	rec := httptest.NewRecorder()

	ctx := &PipelineContext{
		Request:        req,
		ResponseWriter: rec,
	}

	cache.Apply(ctx)

	if !ctx.Aborted {
		t.Error("expected context to be aborted")
	}
	if ctx.AbortReason != "served_from_cache" {
		t.Errorf("expected abort reason 'served_from_cache', got %q", ctx.AbortReason)
	}

	// Check response
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if rec.Body.String() != string(body) {
		t.Errorf("expected body %s, got %s", body, rec.Body.String())
	}
	if rec.Header().Get("X-Cache") != "HIT" {
		t.Error("expected X-Cache: HIT header")
	}
}

func TestCache_Apply_CaptureResponse(t *testing.T) {
	cache, err := NewCache(CacheConfig{TTL: time.Hour})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	// Create request
	req := httptest.NewRequest("GET", "/api/users", nil)
	rec := httptest.NewRecorder()

	ctx := &PipelineContext{
		Request:        req,
		ResponseWriter: rec,
	}

	cache.Apply(ctx)

	if ctx.Aborted {
		t.Error("expected context not to be aborted for cache miss")
	}

	// ResponseWriter should be wrapped
	if _, ok := ctx.ResponseWriter.(*CaptureResponseWriter); !ok {
		t.Error("expected ResponseWriter to be wrapped in CaptureResponseWriter")
	}
}

func TestCache_AfterProxy(t *testing.T) {
	cache, err := NewCache(CacheConfig{
		TTL:                  time.Hour,
		CacheableStatusCodes: []int{200},
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	// Create request and response
	req := httptest.NewRequest("GET", "/api/users", nil)
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/json")
	rec.WriteHeader(http.StatusOK)
	rec.Write([]byte(`{"users": []}`))

	// Wrap in capture writer
	capture := NewCaptureResponseWriter(rec)
	capture.WriteHeader(http.StatusOK)
	capture.Write([]byte(`{"users": []}`))

	ctx := &PipelineContext{
		Request:        req,
		ResponseWriter: capture,
	}

	cache.AfterProxy(ctx, nil)

	// Check that response was cached
	key := cache.GenerateKey("GET", "/api/users", req.Header)
	if _, found := cache.Get(key); !found {
		t.Error("expected response to be cached")
	}
}

func TestCache_AfterProxy_NonCacheable(t *testing.T) {
	cache, err := NewCache(CacheConfig{
		TTL:                  time.Hour,
		CacheableStatusCodes: []int{200},
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	// Create request and response with non-cacheable status
	req := httptest.NewRequest("GET", "/api/users", nil)
	rec := httptest.NewRecorder()
	rec.WriteHeader(http.StatusNotFound)
	rec.Write([]byte(`{"error": "not found"}`))

	// Wrap in capture writer
	capture := NewCaptureResponseWriter(rec)
	capture.WriteHeader(http.StatusNotFound)
	capture.Write([]byte(`{"error": "not found"}`))

	ctx := &PipelineContext{
		Request:        req,
		ResponseWriter: capture,
	}

	cache.AfterProxy(ctx, nil)

	// Check that response was NOT cached
	key := cache.GenerateKey("GET", "/api/users", req.Header)
	if _, found := cache.Get(key); found {
		t.Error("expected response not to be cached")
	}
}

func TestCache_Entry_HitCount(t *testing.T) {
	entry := &CacheEntry{}

	if entry.HitCount() != 0 {
		t.Errorf("expected initial hit count 0, got %d", entry.HitCount())
	}

	entry.Hit()
	entry.Hit()
	entry.Hit()

	if entry.HitCount() != 3 {
		t.Errorf("expected hit count 3, got %d", entry.HitCount())
	}
}

func TestCache_Entry_IsExpired(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		expires  time.Time
		expected bool
	}{
		{
			name:     "not expired",
			expires:  now.Add(time.Hour),
			expected: false,
		},
		{
			name:     "expired",
			expires:  now.Add(-time.Hour),
			expected: true,
		},
		{
			name:     "nil entry",
			expires:  time.Time{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var entry *CacheEntry
			if tt.name != "nil entry" {
				entry = &CacheEntry{ExpiresAt: tt.expires}
			}
			if entry.IsExpired(now) != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, !tt.expected)
			}
		})
	}
}

func TestCache_StatsSnapshot_HitRate(t *testing.T) {
	tests := []struct {
		name     string
		hits     int64
		misses   int64
		expected float64
	}{
		{
			name:     "50% hit rate",
			hits:     50,
			misses:   50,
			expected: 50.0,
		},
		{
			name:     "no requests",
			hits:     0,
			misses:   0,
			expected: 0,
		},
		{
			name:     "100% hits",
			hits:     100,
			misses:   0,
			expected: 100.0,
		},
		{
			name:     "100% misses",
			hits:     0,
			misses:   100,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := CacheStatsSnapshot{
				Hits:   tt.hits,
				Misses: tt.misses,
			}
			if snapshot.HitRate() != tt.expected {
				t.Errorf("expected hit rate %f, got %f", tt.expected, snapshot.HitRate())
			}
		})
	}
}

func TestCache_Stop(t *testing.T) {
	cache, err := NewCache(CacheConfig{TTL: time.Hour})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	if cache.IsStopped() {
		t.Error("expected cache not to be stopped initially")
	}

	cache.Stop()

	if !cache.IsStopped() {
		t.Error("expected cache to be stopped")
	}

	// Second stop should be safe
	cache.Stop()
}

func TestCache_NilOperations(t *testing.T) {
	var cache *Cache

	// All operations should handle nil gracefully
	cache.Stop()
	cache.Clear()
	cache.Delete("key")
	cache.DeleteByPattern(".*")
	cache.DeleteByTag("tag")
	cache.DeleteByTags([]string{"tag"})
	cache.Get("key")
	cache.Set("key", 200, nil, nil, 0, nil)
	cache.WarmURL("GET", "/", nil, nil, 0, nil)
	cache.WarmURLs(nil)

	if cache.Len() != 0 {
		t.Error("expected Len to return 0 for nil cache")
	}
	if cache.MemoryUsed() != 0 {
		t.Error("expected MemoryUsed to return 0 for nil cache")
	}
	if !cache.IsStopped() {
		t.Error("expected IsStopped to return true for nil cache")
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	cache, err := NewCache(CacheConfig{
		TTL:     time.Hour,
		MaxSize: 100,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	headers := http.Header{}
	body := []byte("test data")

	var wg sync.WaitGroup
	numGoroutines := 100
	numOperations := 100

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				cache.Set(key, http.StatusOK, headers, body, 0, nil)
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				cache.Get(key)
			}
		}(i)
	}

	// Concurrent deletes
	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				cache.Delete(key)
			}
		}(i)
	}

	wg.Wait()

	// Cache should still be in a valid state
	stats := cache.Stats()
	if stats.Count != int64(cache.Len()) {
		t.Errorf("stats count %d doesn't match actual count %d", stats.Count, cache.Len())
	}
}

func TestCache_EntrySize(t *testing.T) {
	cache, err := NewCache(CacheConfig{TTL: time.Hour})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	headers := http.Header{
		"Content-Type":   []string{"application/json"},
		"Content-Length": []string{"100"},
	}
	body := []byte(`{"message": "test"}`)

	cache.Set("key1", http.StatusOK, headers, body, 0, nil)

	entry, found := cache.Get("key1")
	if !found {
		t.Fatal("expected to find entry")
	}

	// Size should be body + headers
	expectedSize := int64(len(body) + len("Content-Type") + len("application/json") +
		len("Content-Length") + len("100"))
	if entry.Size != expectedSize {
		t.Errorf("expected size %d, got %d", expectedSize, entry.Size)
	}
}

func TestBuildCachePlugin(t *testing.T) {
	tests := []struct {
		name    string
		config  config.PluginConfig
		wantErr bool
	}{
		{
			name: "default config",
			config: config.PluginConfig{
				Name:   "cache",
				Config: map[string]any{},
			},
		},
		{
			name: "full config",
			config: config.PluginConfig{
				Name: "cache",
				Config: map[string]any{
					"ttl":                    "5m",
					"max_size":               1000,
					"max_memory_mb":          50,
					"key_headers":            []string{"Accept"},
					"cacheable_methods":      []string{"GET", "HEAD"},
					"cacheable_status_codes": []int{200, 204},
					"exclude_paths":          []string{"^/admin/.*"},
					"cleanup_interval":       "1m",
					"tags_enabled":           true,
				},
			},
		},
		{
			name: "invalid exclude pattern",
			config: config.PluginConfig{
				Name: "cache",
				Config: map[string]any{
					"exclude_paths": []string{"[invalid"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin, err := buildCachePlugin(tt.config, BuilderContext{})
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if plugin.name != "cache" {
				t.Errorf("expected name 'cache', got %q", plugin.name)
			}
			if plugin.phase != PhasePostProxy {
				t.Errorf("expected phase PhasePostProxy, got %v", plugin.phase)
			}
		})
	}
}

func TestCache_TagsDisabled(t *testing.T) {
	cache, err := NewCache(CacheConfig{
		TTL:         time.Hour,
		TagsEnabled: false,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	headers := http.Header{}
	body := []byte("test")

	cache.Set("key1", http.StatusOK, headers, body, 0, []string{"users"})

	// DeleteByTag should return 0 when tags are disabled
	deleted := cache.DeleteByTag("users")
	if deleted != 0 {
		t.Errorf("expected 0 deletions when tags disabled, got %d", deleted)
	}

	// Entry should still exist
	if _, found := cache.Get("key1"); !found {
		t.Error("expected key1 to still exist")
	}
}

func TestCache_NormalizeTags(t *testing.T) {
	tests := []struct {
		input    []string
		expected []string
	}{
		{
			input:    []string{"Tag1", "TAG2", "  tag3  "},
			expected: []string{"tag1", "tag2", "tag3"},
		},
		{
			input:    []string{"", "  ", "tag", ""},
			expected: []string{"tag"},
		},
		{
			input:    []string{"dup", "DUP", "Dup"},
			expected: []string{"dup"},
		},
		{
			input:    nil,
			expected: nil,
		},
		{
			input:    []string{},
			expected: nil,
		},
	}

	for _, tt := range tests {
		result := normalizeTags(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("expected %v, got %v", tt.expected, result)
			continue
		}
		for i, v := range result {
			if v != tt.expected[i] {
				t.Errorf("expected %v, got %v", tt.expected, result)
				break
			}
		}
	}
}

func TestParseMaxAge(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"max-age=3600", 3600 * time.Second},
		{"public, max-age=300", 300 * time.Second},
		{"max-age=0", 0},
		{"no-cache", 0},
		{"", 0},
		{"MAX-AGE=600", 600 * time.Second}, // case insensitive
	}

	for _, tt := range tests {
		result := parseMaxAge(tt.input)
		if result != tt.expected {
			t.Errorf("parseMaxAge(%q) = %v, expected %v", tt.input, result, tt.expected)
		}
	}
}

func TestCacheEntry_Hit_Nil(t *testing.T) {
	var entry *CacheEntry

	// Should not panic
	entry.Hit()
	if entry.HitCount() != 0 {
		t.Error("expected hit count 0 for nil entry")
	}
}

func TestCache_Stats_Nil(t *testing.T) {
	var stats *CacheStats

	// Should not panic
	snapshot := stats.Snapshot()
	if snapshot.Hits != 0 {
		t.Error("expected 0 hits for nil stats")
	}
}

func TestCache_CaptureResponseWriter_Flush(t *testing.T) {
	out := httptest.NewRecorder()
	capture := NewCaptureResponseWriter(out)

	capture.Header().Set("X-Test", "yes")
	capture.WriteHeader(http.StatusCreated)
	_, _ = capture.Write([]byte("captured"))

	if err := capture.Flush(); err != nil {
		t.Fatalf("Flush error: %v", err)
	}

	if out.Code != http.StatusCreated {
		t.Fatalf("expected status 201 got %d", out.Code)
	}
	if out.Body.String() != "captured" {
		t.Fatalf("unexpected body %q", out.Body.String())
	}
	if out.Header().Get("X-Test") != "yes" {
		t.Fatalf("expected captured header to flush")
	}
	if !capture.IsFlushed() {
		t.Error("expected IsFlushed to return true")
	}
}

func TestCache_CaptureResponseWriter_SetBody(t *testing.T) {
	out := httptest.NewRecorder()
	capture := NewCaptureResponseWriter(out)

	capture.WriteHeader(http.StatusOK)
	_, _ = capture.Write([]byte("original"))
	capture.SetBody([]byte("replaced"))

	if string(capture.BodyBytes()) != "replaced" {
		t.Errorf("expected body to be replaced, got %s", capture.BodyBytes())
	}
}

func TestCache_CaptureResponseWriter_ReadBody(t *testing.T) {
	out := httptest.NewRecorder()
	capture := NewCaptureResponseWriter(out)

	capture.WriteHeader(http.StatusOK)
	_, _ = capture.Write([]byte("test data"))

	reader := capture.ReadBody()
	data, _ := io.ReadAll(reader)
	if string(data) != "test data" {
		t.Errorf("expected body from ReadBody, got %s", data)
	}
}

func TestCache_CaptureResponseWriter_Write(t *testing.T) {
	out := httptest.NewRecorder()
	capture := NewCaptureResponseWriter(out)

	// Write without WriteHeader should auto-set StatusOK
	_, _ = capture.Write([]byte("data"))

	if capture.StatusCode() != http.StatusOK {
		t.Errorf("expected status OK, got %d", capture.StatusCode())
	}
	if !capture.HasCaptured() {
		t.Error("expected HasCaptured to be true")
	}
}

func TestCache_AfterProxy_WithCacheControl(t *testing.T) {
	cache, err := NewCache(CacheConfig{
		TTL:                  time.Hour,
		CacheableStatusCodes: []int{200},
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	// Create request and response with Cache-Control max-age
	req := httptest.NewRequest("GET", "/api/users", nil)
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/json")
	rec.Header().Set("Cache-Control", "max-age=60")
	rec.WriteHeader(http.StatusOK)
	rec.Write([]byte(`{"users": []}`))

	// Wrap in capture writer
	capture := NewCaptureResponseWriter(rec)
	capture.WriteHeader(http.StatusOK)
	capture.Write([]byte(`{"users": []}`))

	ctx := &PipelineContext{
		Request:        req,
		ResponseWriter: capture,
	}

	cache.AfterProxy(ctx, nil)

	// Check that response was cached with 60 second TTL
	key := cache.GenerateKey("GET", "/api/users", req.Header)
	if entry, found := cache.Get(key); found {
		expectedExpiry := time.Now().Add(60 * time.Second)
		if entry.ExpiresAt.Before(expectedExpiry.Add(-5*time.Second)) || entry.ExpiresAt.After(expectedExpiry.Add(5*time.Second)) {
			t.Errorf("expected TTL around 60s, got expiry at %v", entry.ExpiresAt)
		}
	} else {
		t.Error("expected response to be cached")
	}
}

func TestCache_AfterProxy_WithTags(t *testing.T) {
	cache, err := NewCache(CacheConfig{
		TTL:                  time.Hour,
		CacheableStatusCodes: []int{200},
		TagsEnabled:          true,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	// Create request and response with cache tags
	req := httptest.NewRequest("GET", "/api/users", nil)
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/json")
	rec.Header().Set("X-Cache-Tags", "users,api")
	rec.WriteHeader(http.StatusOK)
	rec.Write([]byte(`{"users": []}`))

	// Wrap in capture writer
	capture := NewCaptureResponseWriter(rec)
	capture.WriteHeader(http.StatusOK)
	capture.Write([]byte(`{"users": []}`))

	ctx := &PipelineContext{
		Request:        req,
		ResponseWriter: capture,
	}

	cache.AfterProxy(ctx, nil)

	// Check that response was cached with tags
	key := cache.GenerateKey("GET", "/api/users", req.Header)
	if entry, found := cache.Get(key); found {
		if len(entry.Tags) != 2 {
			t.Errorf("expected 2 tags, got %d", len(entry.Tags))
		}
	} else {
		t.Error("expected response to be cached")
	}
}

func TestCache_AfterProxy_NonCacheableStatus(t *testing.T) {
	cache, err := NewCache(CacheConfig{
		TTL:                  time.Hour,
		CacheableStatusCodes: []int{200},
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	// Create request and response with non-cacheable status
	req := httptest.NewRequest("GET", "/api/users", nil)
	rec := httptest.NewRecorder()
	rec.WriteHeader(http.StatusInternalServerError)
	rec.Write([]byte(`{"error": "server error"}`))

	// Wrap in capture writer
	capture := NewCaptureResponseWriter(rec)
	capture.WriteHeader(http.StatusInternalServerError)
	capture.Write([]byte(`{"error": "server error"}`))

	ctx := &PipelineContext{
		Request:        req,
		ResponseWriter: capture,
	}

	cache.AfterProxy(ctx, nil)

	// Check that response was NOT cached
	key := cache.GenerateKey("GET", "/api/users", req.Header)
	if _, found := cache.Get(key); found {
		t.Error("expected response not to be cached for 500 status")
	}
}

func TestCache_AfterProxy_AlreadyServedFromCache(t *testing.T) {
	cache, err := NewCache(CacheConfig{
		TTL:                  time.Hour,
		CacheableStatusCodes: []int{200},
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	ctx := &PipelineContext{
		Aborted:     true,
		AbortReason: "served_from_cache",
	}

	// Should not panic or error
	cache.AfterProxy(ctx, nil)
}

func TestCache_AfterProxy_ProxyError(t *testing.T) {
	cache, err := NewCache(CacheConfig{
		TTL:                  time.Hour,
		CacheableStatusCodes: []int{200},
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	ctx := &PipelineContext{}

	// Should not cache when there's a proxy error
	cache.AfterProxy(ctx, fmt.Errorf("proxy error"))
}

func TestCache_Apply_WithStoppedCache(t *testing.T) {
	cache, err := NewCache(CacheConfig{
		TTL:                  time.Hour,
		CacheableStatusCodes: []int{200},
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	cache.Stop()

	req := httptest.NewRequest("GET", "/api/users", nil)
	rec := httptest.NewRecorder()

	ctx := &PipelineContext{
		Request:        req,
		ResponseWriter: rec,
	}

	// Should not panic when cache is stopped
	cache.Apply(ctx)

	if ctx.Aborted {
		t.Error("expected context not to be aborted when cache is stopped")
	}
}

func TestCache_WarmURL_Stopped(t *testing.T) {
	cache, err := NewCache(CacheConfig{TTL: time.Hour})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	cache.Stop()

	// Should return nil (no error) when cache is stopped, but not warm anything
	err = cache.WarmURL("GET", "/api/users", nil, []byte("test"), 0, nil)
	if err != nil {
		t.Errorf("expected no error when warming URL on stopped cache, got %v", err)
	}
}

func TestCache_Set_NonCacheableStatus(t *testing.T) {
	cache, err := NewCache(CacheConfig{
		TTL:                  time.Hour,
		CacheableStatusCodes: []int{200},
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	headers := http.Header{}
	body := []byte("test")

	// Try to set non-cacheable status
	cache.Set("key1", http.StatusNotFound, headers, body, 0, nil)

	// Should not be cached
	if _, found := cache.Get("key1"); found {
		t.Error("expected 404 response not to be cached")
	}
}

func TestCache_DeleteByTag_EmptyTag(t *testing.T) {
	cache, err := NewCache(CacheConfig{
		TTL:         time.Hour,
		TagsEnabled: true,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	// Should return 0 for empty tag
	deleted := cache.DeleteByTag("")
	if deleted != 0 {
		t.Errorf("expected 0 deletions for empty tag, got %d", deleted)
	}
}

func TestCache_evictLRU_WithBadElement(t *testing.T) {
	cache, err := NewCache(CacheConfig{
		TTL:     time.Hour,
		MaxSize: 2,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	headers := http.Header{}
	body := []byte("test")

	// Add entries
	cache.Set("key1", http.StatusOK, headers, body, 0, nil)
	cache.Set("key2", http.StatusOK, headers, body, 0, nil)

	// Manually add a bad element to the LRU list
	badElem := cache.lruList.PushBack(123) // non-string value

	// This should handle the bad element gracefully
	cache.mu.Lock()
	cache.evictLRULocked()
	cache.mu.Unlock()

	// Remove the bad element we added
	cache.mu.Lock()
	if badElem != nil {
		cache.lruList.Remove(badElem)
	}
	cache.mu.Unlock()
}

func TestCache_deleteEntry_NotExists(t *testing.T) {
	cache, err := NewCache(CacheConfig{TTL: time.Hour})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	// Should not panic when deleting non-existent key
	cache.deleteEntry("non-existent")
}

func TestCache_CanCacheRequest_NilRequest(t *testing.T) {
	cache, err := NewCache(CacheConfig{TTL: time.Hour})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	if cache.CanCacheRequest(nil) {
		t.Error("expected CanCacheRequest to return false for nil request")
	}
}

func TestCache_CanCacheResponse_NilCache(t *testing.T) {
	var cache *Cache
	if cache.CanCacheResponse(200, nil) {
		t.Error("expected CanCacheResponse to return false for nil cache")
	}
}

func TestCache_ServeFromCache(t *testing.T) {
	cache, err := NewCache(CacheConfig{TTL: time.Hour})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cache.Stop()

	headers := http.Header{
		"Content-Type": []string{"application/json"},
	}
	body := []byte(`{"message": "hello"}`)
	cache.Set("key1", http.StatusOK, headers, body, 0, []string{"test-tag"})

	entry, _ := cache.Get("key1")
	rec := httptest.NewRecorder()

	cache.serveFromCache(rec, entry)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if rec.Body.String() != string(body) {
		t.Errorf("expected body %s, got %s", body, rec.Body.String())
	}
	if rec.Header().Get("X-Cache") != "HIT" {
		t.Error("expected X-Cache: HIT header")
	}
	if rec.Header().Get("X-Cache-Hits") != "1" {
		t.Errorf("expected X-Cache-Hits: 1, got %s", rec.Header().Get("X-Cache-Hits"))
	}
}

func TestCache_buildCachePlugin_InvalidConfig(t *testing.T) {
	cfg := config.PluginConfig{
		Name: "cache",
		Config: map[string]any{
			"exclude_paths": []string{"[invalid"},
		},
	}

	_, err := buildCachePlugin(cfg, BuilderContext{})
	if err == nil {
		t.Error("expected error for invalid exclude pattern")
	}
}

func TestCache_WarmURLs_WithNilCache(t *testing.T) {
	var cache *Cache

	specs := []WarmURLSpec{
		{Method: "GET", URL: "/api/users", Body: []byte("users")},
	}

	// Should not panic with nil cache
	errs := cache.WarmURLs(specs)
	if len(errs) != 0 {
		t.Errorf("expected no errors with nil cache, got %v", errs)
	}
}
