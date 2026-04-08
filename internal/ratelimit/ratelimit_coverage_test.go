package ratelimit

import (
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
)

// TestTokenBucket_EdgeCases tests edge cases for token bucket
func TestTokenBucket_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("nil token bucket", func(t *testing.T) {
		var tb *TokenBucket
		allowed, remaining, resetAt := tb.Allow("key")
		if allowed {
			t.Error("nil token bucket should deny all requests")
		}
		if remaining != 0 {
			t.Errorf("expected remaining 0, got %d", remaining)
		}
		if !resetAt.IsZero() {
			t.Error("expected zero reset time for nil bucket")
		}
	})

	t.Run("empty key uses global", func(t *testing.T) {
		tb := NewTokenBucket(1, 1)
		tb.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

		// Empty key should use "_global"
		allowed1, _, _ := tb.Allow("")
		if !allowed1 {
			t.Error("first request with empty key should be allowed")
		}

		// Second request should be denied
		allowed2, _, _ := tb.Allow("")
		if allowed2 {
			t.Error("second request with empty key should be denied")
		}
	})

	t.Run("whitespace key uses global", func(t *testing.T) {
		tb := NewTokenBucket(1, 1)
		tb.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

		// Whitespace-only key should use "_global"
		allowed1, _, _ := tb.Allow("   ")
		if !allowed1 {
			t.Error("first request with whitespace key should be allowed")
		}
	})

	t.Run("zero rate", func(t *testing.T) {
		tb := NewTokenBucket(0, 10)
		tb.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

		// With zero rate, tokens should not refill
		allowed, remaining, _ := tb.Allow("client")
		if !allowed {
			t.Error("first request should be allowed with initial burst")
		}
		if remaining != 9 {
			t.Errorf("expected remaining 9, got %d", remaining)
		}
	})

	t.Run("negative rate becomes zero", func(t *testing.T) {
		tb := NewTokenBucket(-5, 10)
		if tb.rate != 0 {
			t.Errorf("expected rate to be 0, got %f", tb.rate)
		}
	})

	t.Run("zero burst becomes one", func(t *testing.T) {
		tb := NewTokenBucket(1, 0)
		if tb.capacity != 1 {
			t.Errorf("expected capacity to be 1, got %f", tb.capacity)
		}
	})

	t.Run("negative burst becomes one", func(t *testing.T) {
		tb := NewTokenBucket(1, -5)
		if tb.capacity != 1 {
			t.Errorf("expected capacity to be 1, got %f", tb.capacity)
		}
	})
}

// TestTokenBucket_RefillScenarios tests various refill scenarios
func TestTokenBucket_RefillScenarios(t *testing.T) {
	t.Parallel()

	t.Run("refill adds tokens correctly", func(t *testing.T) {
		start := time.Unix(1_700_000_000, 0)
		tb := NewTokenBucket(10, 5) // 10 tokens/sec, burst of 5
		tb.now = func() time.Time { return start }

		// Use all tokens
		for i := 0; i < 5; i++ {
			tb.Allow("client")
		}

		// Move forward 0.5 seconds (should refill 5 tokens, but capped at burst)
		tb.now = func() time.Time { return start.Add(500 * time.Millisecond) }

		allowed, remaining, _ := tb.Allow("client")
		if !allowed {
			t.Error("request should be allowed after refill")
		}
		// After 0.5s at 10 tokens/sec = 5 tokens refilled, but capped at burst (5)
		// We consume 1, so remaining = 5 - 1 = 4
		if remaining != 4 {
			t.Errorf("expected remaining 4, got %d", remaining)
		}
	})

	t.Run("refill does not exceed capacity", func(t *testing.T) {
		start := time.Unix(1_700_000_000, 0)
		tb := NewTokenBucket(100, 5) // High rate, low burst
		tb.now = func() time.Time { return start }

		// Use one token
		tb.Allow("client")

		// Move forward 1 second (would refill 100 tokens, but capped at 5)
		tb.now = func() time.Time { return start.Add(time.Second) }

		allowed, remaining, _ := tb.Allow("client")
		if !allowed {
			t.Error("request should be allowed")
		}
		// Should have 4 remaining (5 capacity - 1 used)
		if remaining != 4 {
			t.Errorf("expected remaining 4, got %d", remaining)
		}
	})

	t.Run("no refill when no time elapsed", func(t *testing.T) {
		start := time.Unix(1_700_000_000, 0)
		tb := NewTokenBucket(10, 5)
		tb.now = func() time.Time { return start }

		// Use one token
		tb.Allow("client")

		// Same timestamp
		allowed, remaining, _ := tb.Allow("client")
		if !allowed {
			t.Error("second request should be allowed")
		}
		if remaining != 3 {
			t.Errorf("expected remaining 3, got %d", remaining)
		}
	})
}

// TestTokenBucket_ResetAt tests reset time calculations
func TestTokenBucket_ResetAt(t *testing.T) {
	t.Parallel()

	t.Run("reset at when tokens available", func(t *testing.T) {
		start := time.Unix(1_700_000_000, 0)
		tb := NewTokenBucket(1, 5)
		tb.now = func() time.Time { return start }

		_, _, resetAt := tb.Allow("client")
		// When tokens are available, resetAt should be now
		if !resetAt.Equal(start) {
			t.Errorf("expected resetAt to be now when tokens available, got %v", resetAt)
		}
	})

	t.Run("reset at when tokens depleted", func(t *testing.T) {
		start := time.Unix(1_700_000_000, 0)
		tb := NewTokenBucket(1, 1) // 1 token/sec, burst 1
		tb.now = func() time.Time { return start }

		tb.Allow("client") // Use the only token

		_, _, resetAt := tb.Allow("client")
		// Should be 1 second in the future
		expectedReset := start.Add(time.Second)
		if !resetAt.Equal(expectedReset) {
			t.Errorf("expected resetAt %v, got %v", expectedReset, resetAt)
		}
	})

	t.Run("reset at with partial tokens", func(t *testing.T) {
		start := time.Unix(1_700_000_000, 0)
		tb := NewTokenBucket(2, 2) // 2 tokens/sec
		tb.now = func() time.Time { return start }

		tb.Allow("client") // Use 1 token, 1 remaining
		tb.Allow("client") // Use last token, 0 remaining

		// At 0 tokens, need 1 token at 2/sec = 0.5 seconds
		_, _, resetAt := tb.Allow("client")
		expectedReset := start.Add(500 * time.Millisecond)
		if !resetAt.Equal(expectedReset) {
			t.Errorf("expected resetAt %v, got %v", expectedReset, resetAt)
		}
	})
}

// TestFixedWindow_EdgeCases tests edge cases for fixed window
func TestFixedWindow_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("nil fixed window", func(t *testing.T) {
		var fw *FixedWindow
		allowed, remaining, resetAt := fw.Allow("key")
		if allowed {
			t.Error("nil fixed window should deny all requests")
		}
		if remaining != 0 {
			t.Errorf("expected remaining 0, got %d", remaining)
		}
		if !resetAt.IsZero() {
			t.Error("expected zero reset time for nil window")
		}
	})

	t.Run("empty key uses global", func(t *testing.T) {
		start := time.Unix(1_700_000_000, 0)
		fw := NewFixedWindow(1, time.Minute)
		fw.now = func() time.Time { return start }

		allowed1, _, _ := fw.Allow("")
		if !allowed1 {
			t.Error("first request with empty key should be allowed")
		}

		allowed2, _, _ := fw.Allow("")
		if allowed2 {
			t.Error("second request with empty key should be denied")
		}
	})

	t.Run("zero limit becomes one", func(t *testing.T) {
		fw := NewFixedWindow(0, time.Minute)
		if fw.limit != 1 {
			t.Errorf("expected limit to be 1, got %d", fw.limit)
		}
	})

	t.Run("negative limit becomes one", func(t *testing.T) {
		fw := NewFixedWindow(-5, time.Minute)
		if fw.limit != 1 {
			t.Errorf("expected limit to be 1, got %d", fw.limit)
		}
	})

	t.Run("zero window becomes one second", func(t *testing.T) {
		fw := NewFixedWindow(10, 0)
		if fw.windowSeconds != 1 {
			t.Errorf("expected windowSeconds to be 1, got %d", fw.windowSeconds)
		}
	})

	t.Run("negative window becomes one second", func(t *testing.T) {
		fw := NewFixedWindow(10, -time.Second)
		if fw.windowSeconds != 1 {
			t.Errorf("expected windowSeconds to be 1, got %d", fw.windowSeconds)
		}
	})
}

// TestFixedWindow_WindowTransitions tests window transition behavior
func TestFixedWindow_WindowTransitions(t *testing.T) {
	t.Parallel()

	t.Run("window reset after duration", func(t *testing.T) {
		start := time.Unix(1_700_000_000, 0)
		fw := NewFixedWindow(2, time.Minute)
		fw.now = func() time.Time { return start }

		// Use all tokens
		fw.Allow("client")
		fw.Allow("client")

		// Third request denied
		allowed, _, _ := fw.Allow("client")
		if allowed {
			t.Error("third request should be denied")
		}

		// Move to next window
		fw.now = func() time.Time { return start.Add(time.Minute) }

		// Should be allowed again
		allowed, remaining, _ := fw.Allow("client")
		if !allowed {
			t.Error("request in new window should be allowed")
		}
		if remaining != 1 {
			t.Errorf("expected remaining 1, got %d", remaining)
		}
	})

	t.Run("reset at is end of current window", func(t *testing.T) {
		start := time.Unix(1_700_000_000, 0) // Check the actual window boundary
		fw := NewFixedWindow(1, time.Minute)
		fw.now = func() time.Time { return start }

		_, _, resetAt := fw.Allow("client")

		// The reset time should be at the end of the current window
		// which is calculated as (windowID+1)*windowSeconds
		windowID := start.Unix() / 60
		expectedReset := time.Unix((windowID+1)*60, 0)
		if !resetAt.Equal(expectedReset) {
			t.Errorf("expected resetAt %v, got %v", expectedReset, resetAt)
		}
	})
}

// TestSlidingWindow_EdgeCases tests edge cases for sliding window
func TestSlidingWindow_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("nil sliding window", func(t *testing.T) {
		var sw *SlidingWindow
		allowed, remaining, resetAt := sw.Allow("key")
		if allowed {
			t.Error("nil sliding window should deny all requests")
		}
		if remaining != 0 {
			t.Errorf("expected remaining 0, got %d", remaining)
		}
		if !resetAt.IsZero() {
			t.Error("expected zero reset time for nil window")
		}
	})

	t.Run("empty key uses global", func(t *testing.T) {
		start := time.Unix(1_700_000_000, 0)
		sw := NewSlidingWindow(1, time.Minute)
		sw.now = func() time.Time { return start }

		allowed1, _, _ := sw.Allow("")
		if !allowed1 {
			t.Error("first request with empty key should be allowed")
		}

		allowed2, _, _ := sw.Allow("")
		if allowed2 {
			t.Error("second request with empty key should be denied")
		}
	})

	t.Run("zero limit becomes one", func(t *testing.T) {
		sw := NewSlidingWindow(0, time.Minute)
		if sw.limit != 1 {
			t.Errorf("expected limit to be 1, got %d", sw.limit)
		}
	})

	t.Run("negative limit becomes one", func(t *testing.T) {
		sw := NewSlidingWindow(-5, time.Minute)
		if sw.limit != 1 {
			t.Errorf("expected limit to be 1, got %d", sw.limit)
		}
	})

	t.Run("zero window becomes one second", func(t *testing.T) {
		sw := NewSlidingWindow(10, 0)
		if sw.window != time.Second {
			t.Errorf("expected window to be 1s, got %v", sw.window)
		}
	})

	t.Run("negative window becomes one second", func(t *testing.T) {
		sw := NewSlidingWindow(10, -time.Second)
		if sw.window != time.Second {
			t.Errorf("expected window to be 1s, got %v", sw.window)
		}
	})
}

// TestSlidingWindow_Pruning tests the pruning behavior
func TestSlidingWindow_Pruning(t *testing.T) {
	t.Parallel()

	t.Run("old slots are pruned", func(t *testing.T) {
		start := time.Unix(1_700_000_000, 0)
		sw := NewSlidingWindow(10, time.Second) // 1 second window, 100ms sub-windows
		sw.now = func() time.Time { return start }

		// Add requests in current window
		sw.Allow("client")

		// Move forward past the window
		sw.now = func() time.Time { return start.Add(2 * time.Second) }

		// Old slots should be pruned, allowing new requests
		allowed, remaining, _ := sw.Allow("client")
		if !allowed {
			t.Error("request should be allowed after old slots pruned")
		}
		if remaining != 9 {
			t.Errorf("expected remaining 9, got %d", remaining)
		}
	})

	t.Run("reset at considers oldest slot", func(t *testing.T) {
		start := time.Unix(1_700_000_000, 0)
		sw := NewSlidingWindow(1, time.Second)
		sw.now = func() time.Time { return start }

		sw.Allow("client") // Use the only allowed request

		// Next request should be denied with reset time in the future
		allowed, _, resetAt := sw.Allow("client")
		if allowed {
			t.Error("second request should be denied")
		}
		if !resetAt.After(start) {
			t.Error("resetAt should be in the future")
		}
	})
}

// TestSlidingWindow_MultipleRequests tests multiple request scenarios
func TestSlidingWindow_MultipleRequests(t *testing.T) {
	t.Parallel()

	t.Run("requests across different keys are independent", func(t *testing.T) {
		start := time.Unix(1_700_000_000, 0)
		sw := NewSlidingWindow(1, time.Minute)
		sw.now = func() time.Time { return start }

		// Use limit for key-a
		sw.Allow("key-a")

		// key-b should still be allowed
		allowed, _, _ := sw.Allow("key-b")
		if !allowed {
			t.Error("key-b should be allowed independently")
		}

		// key-a should be denied
		allowed, _, _ = sw.Allow("key-a")
		if allowed {
			t.Error("key-a should be denied after limit reached")
		}
	})

	t.Run("remaining count decreases correctly", func(t *testing.T) {
		start := time.Unix(1_700_000_000, 0)
		sw := NewSlidingWindow(5, time.Minute)
		sw.now = func() time.Time { return start }

		expectedRemaining := []int{4, 3, 2, 1, 0}
		for i, expected := range expectedRemaining {
			_, remaining, _ := sw.Allow("client")
			if remaining != expected {
				t.Errorf("request %d: expected remaining %d, got %d", i+1, expected, remaining)
			}
		}
	})
}

// TestLeakyBucket_EdgeCases tests edge cases for leaky bucket
func TestLeakyBucket_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("nil leaky bucket", func(t *testing.T) {
		var lb *LeakyBucket
		allowed, remaining, resetAt := lb.Allow("key")
		if allowed {
			t.Error("nil leaky bucket should deny all requests")
		}
		if remaining != 0 {
			t.Errorf("expected remaining 0, got %d", remaining)
		}
		if !resetAt.IsZero() {
			t.Error("expected zero reset time for nil bucket")
		}
	})

	t.Run("zero leak rate becomes one", func(t *testing.T) {
		lb := NewLeakyBucket(10, 0)
		if lb.leakRate != 1 {
			t.Errorf("expected leakRate 1, got %f", lb.leakRate)
		}
	})

	t.Run("negative leak rate becomes one", func(t *testing.T) {
		lb := NewLeakyBucket(10, -5)
		if lb.leakRate != 1 {
			t.Errorf("expected leakRate 1, got %f", lb.leakRate)
		}
	})

	t.Run("zero capacity becomes one", func(t *testing.T) {
		lb := NewLeakyBucket(0, 1)
		if lb.capacity != 1 {
			t.Errorf("expected capacity 1, got %d", lb.capacity)
		}
	})

	t.Run("negative capacity becomes one", func(t *testing.T) {
		lb := NewLeakyBucket(-5, 1)
		if lb.capacity != 1 {
			t.Errorf("expected capacity 1, got %d", lb.capacity)
		}
	})
}

// TestLeakyBucket_Behavior tests leaky bucket behavior
func TestLeakyBucket_Behavior(t *testing.T) {
	t.Parallel()

	t.Run("requests fill bucket up to capacity", func(t *testing.T) {
		start := time.Unix(1_700_000_000, 0)
		lb := NewLeakyBucket(3, 1) // capacity 3, 1 req/sec leak rate
		lb.now = func() time.Time { return start }

		// First 3 requests should be allowed (fill bucket)
		for i := 0; i < 3; i++ {
			allowed, _, _ := lb.Allow("client")
			if !allowed {
				t.Errorf("request %d should be allowed", i+1)
			}
		}

		// Fourth request should be denied (bucket full)
		allowed, _, _ := lb.Allow("client")
		if allowed {
			t.Error("fourth request should be denied")
		}
	})

	t.Run("bucket leaks over time allowing new requests", func(t *testing.T) {
		start := time.Unix(1_700_000_000, 0)
		lb := NewLeakyBucket(1, 1) // capacity 1, 1 req/sec
		lb.now = func() time.Time { return start }

		// Fill bucket
		lb.Allow("client")

		// Wait for leak
		lb.now = func() time.Time { return start.Add(2 * time.Second) }

		// Should be allowed again
		allowed, _, _ := lb.Allow("client")
		if !allowed {
			t.Error("request should be allowed after leak")
		}
	})

	t.Run("empty key uses global", func(t *testing.T) {
		start := time.Unix(1_700_000_000, 0)
		lb := NewLeakyBucket(1, 1)
		lb.now = func() time.Time { return start }

		allowed, _, _ := lb.Allow("")
		if !allowed {
			t.Error("first request with empty key should be allowed")
		}
	})
}

// TestLeakyBucket_ResetAt tests reset time calculations for leaky bucket
func TestLeakyBucket_ResetAt(t *testing.T) {
	t.Parallel()

	t.Run("reset at when bucket has space", func(t *testing.T) {
		start := time.Unix(1_700_000_000, 0)
		lb := NewLeakyBucket(5, 1) // capacity 5, 1 req/sec leak rate
		lb.now = func() time.Time { return start }

		_, _, resetAt := lb.Allow("client")
		// When bucket has items (queue > 0), resetAt is calculated based on queue
		// After first request, queue = 1, so resetAt = now + (1/1) seconds
		expectedReset := start.Add(time.Second)
		if !resetAt.Equal(expectedReset) {
			t.Errorf("expected resetAt %v, got %v", expectedReset, resetAt)
		}
	})

	t.Run("reset at when bucket is full", func(t *testing.T) {
		start := time.Unix(1_700_000_000, 0)
		lb := NewLeakyBucket(1, 1) // capacity 1, 1 req/sec leak rate
		lb.now = func() time.Time { return start }

		lb.Allow("client") // Fill bucket

		_, _, resetAt := lb.Allow("client")
		// Should be 1 second in the future (time to leak one request)
		expectedReset := start.Add(time.Second)
		if !resetAt.Equal(expectedReset) {
			t.Errorf("expected resetAt %v, got %v", expectedReset, resetAt)
		}
	})
}

// TestRateLimiterFactory tests the factory pattern
func TestRateLimiterFactory(t *testing.T) {
	t.Parallel()

	t.Run("factory with disabled redis creates local token bucket", func(t *testing.T) {
		factory := NewRateLimiterFactory(config.RedisConfig{Enabled: false})
		limiter := factory.CreateTokenBucket(10, 20)

		_, ok := limiter.(*TokenBucket)
		if !ok {
			t.Error("expected TokenBucket when Redis is disabled")
		}
	})

	t.Run("factory with disabled redis creates local sliding window", func(t *testing.T) {
		factory := NewRateLimiterFactory(config.RedisConfig{Enabled: false})
		limiter := factory.CreateSlidingWindow(10, time.Minute)

		_, ok := limiter.(*SlidingWindow)
		if !ok {
			t.Error("expected SlidingWindow when Redis is disabled")
		}
	})
}
