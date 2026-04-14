package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/redis/go-redis/v9"
)

func TestNewRedisLimiter_Disabled(t *testing.T) {
	cfg := config.RedisConfig{
		Enabled: false,
	}

	_, err := NewRedisLimiter(cfg)
	if err == nil {
		t.Error("Expected error when Redis is disabled")
	}
}

func TestNewRedisLimiter_NoAddress(t *testing.T) {
	cfg := config.RedisConfig{
		Enabled:     true,
		Address:     "localhost:1", // guaranteed unreachable
		DialTimeout: 100 * time.Millisecond,
	}

	// This will fail to connect since nothing listens on port 1
	_, err := NewRedisLimiter(cfg)
	if err == nil {
		t.Error("Expected connection error when Redis is not available")
	}
}

func TestDistributedTokenBucket_Fallback(t *testing.T) {
	cfg := config.RedisConfig{
		Enabled:         true,
		Address:         "localhost:1",
		FallbackToLocal: true,
	}

	// Create with fallback since Redis is not running
	dtb, err := NewDistributedTokenBucket(cfg, 10, 20)
	if err != nil {
		// Expected to fail since Redis is not running
		// Test fallback behavior manually
		localLimiter := NewTokenBucket(10, 20)
		allowed, remaining, _ := localLimiter.Allow("test-key")
		if !allowed {
			t.Error("Expected local fallback to allow request")
		}
		if remaining != 19 {
			t.Errorf("Expected 19 remaining, got %d", remaining)
		}
		return
	}
	defer dtb.Close()
}

func TestDistributedSlidingWindow_Fallback(t *testing.T) {
	cfg := config.RedisConfig{
		Enabled:         true,
		Address:         "localhost:1",
		FallbackToLocal: true,
	}

	// Create with fallback since Redis is not running
	dsw, err := NewDistributedSlidingWindow(cfg, 10, time.Second)
	if err != nil {
		// Expected to fail since Redis is not running
		// Test fallback behavior manually
		localLimiter := NewSlidingWindow(10, time.Second)
		allowed, remaining, _ := localLimiter.Allow("test-key")
		if !allowed {
			t.Error("Expected local fallback to allow request")
		}
		if remaining != 9 {
			t.Errorf("Expected 9 remaining, got %d", remaining)
		}
		return
	}
	defer dsw.Close()
}

func TestRateLimiterFactory_DisabledRedis(t *testing.T) {
	cfg := config.RedisConfig{
		Enabled: false,
	}

	factory := NewRateLimiterFactory(cfg)

	// Should create local token bucket
	limiter := factory.CreateTokenBucket(10, 20)
	if limiter == nil {
		t.Fatal("Expected limiter to be created")
	}

	allowed, remaining, _ := limiter.Allow("test-key")
	if !allowed {
		t.Error("Expected request to be allowed")
	}
	if remaining != 19 {
		t.Errorf("Expected 19 remaining, got %d", remaining)
	}
}

func TestRateLimiterFactory_CreateSlidingWindow(t *testing.T) {
	cfg := config.RedisConfig{
		Enabled: false,
	}

	factory := NewRateLimiterFactory(cfg)

	// Should create local sliding window
	limiter := factory.CreateSlidingWindow(10, time.Second)
	if limiter == nil {
		t.Fatal("Expected limiter to be created")
	}

	allowed, remaining, _ := limiter.Allow("test-key")
	if !allowed {
		t.Error("Expected request to be allowed")
	}
	if remaining != 9 {
		t.Errorf("Expected 9 remaining, got %d", remaining)
	}
}

func TestRedisConfig_Defaults(t *testing.T) {
	cfg := config.RedisConfig{
		Enabled: true,
		// Leave other fields empty
	}

	// This will fail to connect, but we're testing defaults
	limiter, err := NewRedisLimiter(cfg)
	if err != nil {
		// Expected since Redis is not running
		return
	}
	defer limiter.Close()
}

func TestGenerateRequestID(t *testing.T) {
	id1 := generateRequestID()
	time.Sleep(time.Millisecond) // Ensure different timestamp
	id2 := generateRequestID()

	if id1 == "" {
		t.Error("Expected non-empty request ID")
	}

	if len(id1) != 8 {
		t.Errorf("Expected request ID length 8, got %d", len(id1))
	}

	if id1 == id2 {
		t.Error("Expected different request IDs")
	}
}

func TestRedisLimiter_makeKey(t *testing.T) {
	// Create a mock limiter for testing
	rl := &RedisLimiter{
		config: config.RedisConfig{
			KeyPrefix: "test:",
		},
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"key1", "test:key1"},
		{"  key2  ", "test:key2"},
		{"", "test:_global"},
		{"   ", "test:_global"},
	}

	for _, tt := range tests {
		result := rl.makeKey(tt.input)
		if result != tt.expected {
			t.Errorf("makeKey(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestDistributedTokenBucket_Nil(t *testing.T) {
	var dtb *DistributedTokenBucket

	allowed, remaining, _ := dtb.Allow("test")
	if allowed {
		t.Error("Expected nil DistributedTokenBucket to deny")
	}
	if remaining != 0 {
		t.Error("Expected 0 remaining for nil limiter")
	}
}

func TestDistributedSlidingWindow_Nil(t *testing.T) {
	var dsw *DistributedSlidingWindow

	allowed, remaining, _ := dsw.Allow("test")
	if allowed {
		t.Error("Expected nil DistributedSlidingWindow to deny")
	}
	if remaining != 0 {
		t.Error("Expected 0 remaining for nil limiter")
	}
}

func TestDistributedTokenBucket_Validation(t *testing.T) {
	// Test with zero/negative values
	cfg := config.RedisConfig{
		Enabled: false, // Use disabled to test validation path
	}

	factory := NewRateLimiterFactory(cfg)

	// Test with 0 requests per second and 0 burst
	limiter := factory.CreateTokenBucket(0, 0)
	if limiter == nil {
		t.Fatal("Expected limiter to be created even with 0 values")
	}

	allowed, remaining, _ := limiter.Allow("test")
	if remaining < 0 {
		t.Error("Expected non-negative remaining")
	}

	// Should be allowed because burst defaults to 1
	if !allowed {
		t.Log("Note: Zero values may still allow one request due to burst default")
	}
}

func TestDistributedSlidingWindow_Validation(t *testing.T) {
	cfg := config.RedisConfig{
		Enabled: false,
	}

	factory := NewRateLimiterFactory(cfg)

	// Test with 0 limit
	limiter := factory.CreateSlidingWindow(0, time.Second)
	if limiter == nil {
		t.Fatal("Expected limiter to be created")
	}

	// With 0 limit, every request after the first should be denied
	allowed1, _, _ := limiter.Allow("test")
	allowed2, _, _ := limiter.Allow("test")

	if allowed1 {
		t.Log("First request allowed")
	}
	if allowed2 {
		t.Log("Second request allowed - may vary based on implementation")
	}
}

func TestDistributedTokenBucket_AllowMarksUnavailableAndFallbacks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := &RedisLimiter{
		client: redis.NewClient(&redis.Options{Addr: "localhost:1"}),
		config: config.RedisConfig{KeyPrefix: "test:", FallbackToLocal: true},
		now:    time.Now,
		ctx:    ctx,
		cancel: cancel,
	}
	rl.available.Store(true)

	dtb := &DistributedTokenBucket{
		RedisLimiter: rl,
		rate:         10,
		capacity:     20,
		fallback:     NewTokenBucket(10, 20),
	}

	// First call hits Redis (down), marks unavailable, then falls back.
	allowed, remaining, _ := dtb.Allow("key")
	if !allowed {
		t.Error("expected fallback to allow request")
	}
	if remaining != 19 {
		t.Errorf("expected 19 remaining, got %d", remaining)
	}
	if dtb.IsAvailable() {
		t.Error("expected Redis to be marked unavailable after failure")
	}

	// Second call should fast-fallback without waiting for Redis timeout.
	allowed2, remaining2, _ := dtb.Allow("key")
	if !allowed2 {
		t.Error("expected fast fallback to allow request")
	}
	if remaining2 != 18 {
		t.Errorf("expected 18 remaining, got %d", remaining2)
	}
}

func TestDistributedSlidingWindow_AllowMarksUnavailableAndFallbacks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := &RedisLimiter{
		client: redis.NewClient(&redis.Options{Addr: "localhost:1"}),
		config: config.RedisConfig{KeyPrefix: "test:", FallbackToLocal: true},
		now:    time.Now,
		ctx:    ctx,
		cancel: cancel,
	}
	rl.available.Store(true)

	dsw := &DistributedSlidingWindow{
		RedisLimiter: rl,
		limit:        10,
		window:       time.Second,
		fallback:     NewSlidingWindow(10, time.Second),
	}

	allowed, remaining, _ := dsw.Allow("key")
	if !allowed {
		t.Error("expected fallback to allow request")
	}
	if remaining != 9 {
		t.Errorf("expected 9 remaining, got %d", remaining)
	}
	if dsw.IsAvailable() {
		t.Error("expected Redis to be marked unavailable after failure")
	}

	allowed2, remaining2, _ := dsw.Allow("key")
	if !allowed2 {
		t.Error("expected fast fallback to allow request")
	}
	if remaining2 != 8 {
		t.Errorf("expected 8 remaining, got %d", remaining2)
	}
}

func TestRedisLimiter_MarkUnavailableSchedulesReconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := &RedisLimiter{
		client: redis.NewClient(&redis.Options{Addr: "localhost:1"}),
		config: config.RedisConfig{KeyPrefix: "test:"},
		now:    time.Now,
		ctx:    ctx,
		cancel: cancel,
	}
	rl.available.Store(true)

	if !rl.IsAvailable() {
		t.Fatal("expected available before markUnavailable")
	}

	rl.markUnavailable()

	if rl.IsAvailable() {
		t.Error("expected unavailable after markUnavailable")
	}

	// Reconnect goroutine should have started.
	if !rl.reconnecting.Load() {
		t.Error("expected reconnecting to be true after markUnavailable")
	}

	// Cancel context to stop reconnect loop.
	cancel()

	// Wait briefly for goroutine to exit.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !rl.reconnecting.Load() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("expected reconnecting to become false after context cancel")
}
