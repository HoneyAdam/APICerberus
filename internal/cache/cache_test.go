package cache

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewCache(t *testing.T) {
	config := DefaultConfig()
	cache := New(config)

	if cache == nil {
		t.Fatal("New() returned nil")
	}

	if cache.maxSize != config.MaxSize {
		t.Errorf("maxSize = %v, want %v", cache.maxSize, config.MaxSize)
	}
}

func TestCacheGetSet(t *testing.T) {
	config := DefaultConfig()
	cache := New(config)

	// Create a request
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	// Set a cache entry
	statusCode := http.StatusOK
	headers := http.Header{"Content-Type": []string{"application/json"}}
	body := []byte(`{"message": "hello"}`)

	cache.Set(req, statusCode, headers, body, time.Minute)

	// Get the cache entry
	entry, ok := cache.Get(req)
	if !ok {
		t.Fatal("Expected cache entry to exist")
	}

	if entry.StatusCode != statusCode {
		t.Errorf("StatusCode = %v, want %v", entry.StatusCode, statusCode)
	}

	if string(entry.Body) != string(body) {
		t.Errorf("Body = %v, want %v", string(entry.Body), string(body))
	}
}

func TestCacheExpiration(t *testing.T) {
	config := DefaultConfig()
	cache := New(config)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	statusCode := http.StatusOK
	headers := http.Header{}
	body := []byte(`{"message": "hello"}`)

	// Set with very short TTL
	cache.Set(req, statusCode, headers, body, 1*time.Millisecond)

	// Should exist immediately
	_, ok := cache.Get(req)
	if !ok {
		t.Error("Expected cache entry to exist immediately")
	}

	// Wait for expiration
	time.Sleep(10 * time.Millisecond)

	// Should not exist after expiration
	_, ok = cache.Get(req)
	if ok {
		t.Error("Expected cache entry to be expired")
	}
}

func TestCacheEviction(t *testing.T) {
	config := &Config{
		Enabled:     true,
		MaxSize:     100,
		TTL:         time.Hour,
		MaxItemSize: 50,
	}
	cache := New(config)

	// Set entries that exceed max size
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/test/%d", i), nil)
		cache.Set(req, http.StatusOK, http.Header{}, []byte("test data"), time.Hour)
	}

	// Should have evicted some entries
	stats := cache.Stats()
	if stats["current_size"].(int64) > config.MaxSize {
		t.Errorf("Current size %v exceeds max size %v", stats["current_size"], config.MaxSize)
	}
}

func TestCacheClear(t *testing.T) {
	config := DefaultConfig()
	cache := New(config)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	cache.Set(req, http.StatusOK, http.Header{}, []byte("data"), time.Hour)

	// Clear cache
	cache.Clear()

	// Should be empty
	_, ok := cache.Get(req)
	if ok {
		t.Error("Expected cache to be empty after Clear()")
	}

	stats := cache.Stats()
	if stats["entries"].(int) != 0 {
		t.Errorf("Expected 0 entries, got %v", stats["entries"])
	}
}

func TestCacheStats(t *testing.T) {
	config := DefaultConfig()
	cache := New(config)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	cache.Set(req, http.StatusOK, http.Header{}, []byte("data"), time.Hour)

	stats := cache.Stats()

	if stats["entries"].(int) != 1 {
		t.Errorf("Expected 1 entry, got %v", stats["entries"])
	}

	if stats["max_size"].(int64) != config.MaxSize {
		t.Errorf("Expected max_size %v, got %v", config.MaxSize, stats["max_size"])
	}
}

func TestCanCacheShouldCache(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		cc       string
		expected bool
	}{
		{"GET request", http.MethodGet, "", true},
		{"HEAD request", http.MethodHead, "", true},
		{"POST request", http.MethodPost, "", false},
		{"PUT request", http.MethodPut, "", false},
		{"DELETE request", http.MethodDelete, "", false},
		{"no-store header", http.MethodGet, "no-store", false},
		{"no-cache header", http.MethodGet, "no-cache", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/test", nil)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
			}

			if tt.cc != "" {
				resp.Header.Set("Cache-Control", tt.cc)
			}

			cc := &CanCache{Request: req, Response: resp}
			got := cc.ShouldCache()
			if got != tt.expected {
				t.Errorf("ShouldCache() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCanCacheGetTTL(t *testing.T) {
	tests := []struct {
		name       string
		cc         string
		expires    string
		defaultTTL time.Duration
		expected   time.Duration
	}{
		{
			name:       "max-age",
			cc:         "max-age=60",
			defaultTTL: time.Minute,
			expected:   60 * time.Second,
		},
		{
			name:       "no max-age",
			cc:         "",
			defaultTTL: time.Minute,
			expected:   time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
			}

			if tt.cc != "" {
				resp.Header.Set("Cache-Control", tt.cc)
			}
			if tt.expires != "" {
				resp.Header.Set("Expires", tt.expires)
			}

			cc := &CanCache{Response: resp}
			got := cc.GetTTL(tt.defaultTTL)

			// Allow small delta for timing differences
			diff := got - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > time.Second {
				t.Errorf("GetTTL() = %v, want ~%v", got, tt.expected)
			}
		})
	}
}

func TestCacheMiddleware(t *testing.T) {
	config := DefaultConfig()
	cache := New(config)

	// Create a handler that returns a fixed response
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "hello"}`))
	})

	middleware := NewCacheMiddleware(cache, handler, config)

	// First request - cache miss
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec1 := httptest.NewRecorder()
	middleware.ServeHTTP(rec1, req1)

	if rec1.Header().Get("X-Cache") != "MISS" {
		t.Errorf("Expected X-Cache: MISS, got %v", rec1.Header().Get("X-Cache"))
	}

	// Second request - should be cache hit
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec2 := httptest.NewRecorder()
	middleware.ServeHTTP(rec2, req2)

	if rec2.Header().Get("X-Cache") != "HIT" {
		t.Errorf("Expected X-Cache: HIT, got %v", rec2.Header().Get("X-Cache"))
	}

	// Response should be the same
	if rec1.Body.String() != rec2.Body.String() {
		t.Errorf("Bodies don't match: %v vs %v", rec1.Body.String(), rec2.Body.String())
	}
}

func TestCacheMiddlewareDisabled(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = false
	cache := New(config)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response"))
	})

	middleware := NewCacheMiddleware(cache, handler, config)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	middleware.ServeHTTP(rec, req)

	// Should not have X-Cache header when disabled
	if rec.Header().Get("X-Cache") != "" {
		t.Errorf("Expected no X-Cache header when disabled, got %v", rec.Header().Get("X-Cache"))
	}
}

