package ratelimit

import (
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// TestDistributedTokenBucket_ErrorPaths tests error paths in distributed token bucket
func TestDistributedTokenBucket_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("Allow with nil limiter", func(t *testing.T) {
		var dtb *DistributedTokenBucket
		allowed, remaining, resetAt := dtb.Allow("test-key")
		if allowed {
			t.Error("expected nil limiter to deny")
		}
		if remaining != 0 {
			t.Errorf("expected 0 remaining, got %d", remaining)
		}
		if !resetAt.IsZero() {
			t.Error("expected zero reset time")
		}
	})

	t.Run("Allow with nil RedisLimiter inside", func(t *testing.T) {
		dtb := &DistributedTokenBucket{
			RedisLimiter: nil,
			rate:         10,
			capacity:     20,
		}
		allowed, remaining, _ := dtb.Allow("test-key")
		if allowed {
			t.Error("expected limiter with nil RedisLimiter to deny")
		}
		if remaining != 0 {
			t.Errorf("expected 0 remaining, got %d", remaining)
		}
	})

	t.Run("Allow without fallback returns false when Redis unavailable", func(t *testing.T) {
		dtb := &DistributedTokenBucket{
			RedisLimiter: nil, // Simulate Redis failure
			rate:         10,
			capacity:     20,
			fallback:     nil, // No fallback
		}

		allowed, remaining, resetAt := dtb.Allow("test-key")
		if allowed {
			t.Error("expected request to be denied without Redis")
		}
		if remaining != 0 {
			t.Errorf("expected 0 remaining, got %d", remaining)
		}
		// When RedisLimiter is nil, resetAt is zero time
		if !resetAt.IsZero() {
			t.Error("expected zero reset time when Redis unavailable")
		}
	})
}

// TestDistributedSlidingWindow_ErrorPaths tests error paths in distributed sliding window
func TestDistributedSlidingWindow_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("Allow with nil limiter", func(t *testing.T) {
		var dsw *DistributedSlidingWindow
		allowed, remaining, resetAt := dsw.Allow("test-key")
		if allowed {
			t.Error("expected nil limiter to deny")
		}
		if remaining != 0 {
			t.Errorf("expected 0 remaining, got %d", remaining)
		}
		if !resetAt.IsZero() {
			t.Error("expected zero reset time")
		}
	})

	t.Run("Allow with nil RedisLimiter inside", func(t *testing.T) {
		dsw := &DistributedSlidingWindow{
			RedisLimiter: nil,
			limit:        10,
			window:       time.Minute,
		}
		allowed, remaining, _ := dsw.Allow("test-key")
		if allowed {
			t.Error("expected limiter with nil RedisLimiter to deny")
		}
		if remaining != 0 {
			t.Errorf("expected 0 remaining, got %d", remaining)
		}
	})

	t.Run("Allow without fallback returns false when Redis unavailable", func(t *testing.T) {
		dsw := &DistributedSlidingWindow{
			RedisLimiter: nil, // Simulate Redis failure
			limit:        10,
			window:       time.Minute,
			fallback:     nil, // No fallback
		}

		allowed, remaining, resetAt := dsw.Allow("test-key")
		if allowed {
			t.Error("expected request to be denied without Redis")
		}
		if remaining != 0 {
			t.Errorf("expected 0 remaining, got %d", remaining)
		}
		// When RedisLimiter is nil, resetAt is zero time
		if !resetAt.IsZero() {
			t.Error("expected zero reset time when Redis unavailable")
		}
	})
}

// TestDistributedLimiters_EdgeCases tests edge cases for distributed limiters
func TestDistributedLimiters_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("distributed token bucket with zero values", func(t *testing.T) {
		cfg := config.RedisConfig{Enabled: false}
		factory := NewRateLimiterFactory(cfg)
		limiter := factory.CreateTokenBucket(0, 0)

		// Should create local token bucket as fallback
		_, ok := limiter.(*TokenBucket)
		if !ok {
			t.Error("expected TokenBucket fallback")
		}
	})

	t.Run("distributed sliding window with zero values", func(t *testing.T) {
		cfg := config.RedisConfig{Enabled: false}
		factory := NewRateLimiterFactory(cfg)
		limiter := factory.CreateSlidingWindow(0, 0)

		// Should create local sliding window as fallback
		_, ok := limiter.(*SlidingWindow)
		if !ok {
			t.Error("expected SlidingWindow fallback")
		}
	})
}

// TestRateLimiterFactory_WithUnavailableRedis tests factory behavior when Redis is unavailable
func TestRateLimiterFactory_WithUnavailableRedis(t *testing.T) {
	t.Parallel()

	// Use a guaranteed-unreachable address (port 1 is reserved/never listening)
	cfg := config.RedisConfig{
		Enabled:         true,
		Address:         "localhost:1",
		DialTimeout:     100 * time.Millisecond,
		FallbackToLocal: true,
	}

	factory := NewRateLimiterFactory(cfg)

	t.Run("CreateTokenBucket falls back to local", func(t *testing.T) {
		limiter := factory.CreateTokenBucket(10, 20)

		// Should fall back to local token bucket
		_, ok := limiter.(*TokenBucket)
		if !ok {
			t.Error("expected TokenBucket when Redis unavailable")
		}

		// Verify it works
		allowed, remaining, _ := limiter.Allow("test")
		if !allowed {
			t.Error("expected fallback limiter to allow")
		}
		if remaining != 19 {
			t.Errorf("expected 19 remaining, got %d", remaining)
		}
	})

	t.Run("CreateSlidingWindow falls back to local", func(t *testing.T) {
		limiter := factory.CreateSlidingWindow(10, time.Minute)

		// Should fall back to local sliding window
		_, ok := limiter.(*SlidingWindow)
		if !ok {
			t.Error("expected SlidingWindow when Redis unavailable")
		}

		// Verify it works
		allowed, remaining, _ := limiter.Allow("test")
		if !allowed {
			t.Error("expected fallback limiter to allow")
		}
		if remaining != 9 {
			t.Errorf("expected 9 remaining, got %d", remaining)
		}
	})
}

// TestGenerateRequestID_Coverage tests the request ID generation
func TestGenerateRequestID_Coverage(t *testing.T) {
	t.Parallel()

	id1 := generateRequestID()
	time.Sleep(1 * time.Millisecond) // Ensure different timestamp
	id2 := generateRequestID()

	if id1 == "" {
		t.Error("generateRequestID should return non-empty ID")
	}
	if len(id1) != 8 {
		t.Errorf("generateRequestID should return 8 char ID, got %d", len(id1))
	}
	if id1 == id2 {
		t.Error("generateRequestID should return unique IDs")
	}
}
