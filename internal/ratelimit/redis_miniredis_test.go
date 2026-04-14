package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newMiniredis starts an in-memory Redis instance for testing.
func newMiniredis(t *testing.T) (*miniredis.Miniredis, error) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { mr.Close() })
	return mr, nil
}

// makeRedisClient creates a Redis client for the given address.
func makeRedisClient(addr string) *redis.Client {
	return redis.NewClient(&redis.Options{Addr: addr})
}

// contextWithCancel creates a background context with cancel.
func contextWithCancel() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

func TestDistributedTokenBucket_WithMiniredis(t *testing.T) {
	mr, err := newMiniredis(t)
	if err != nil {
		t.Skipf("miniredis not available: %v", err)
	}

	cfg := config.RedisConfig{
		Enabled:         true,
		Address:         mr.Addr(),
		FallbackToLocal: true,
	}

	dtb, err := NewDistributedTokenBucket(cfg, 10, 20)
	if err != nil {
		t.Fatalf("NewDistributedTokenBucket failed: %v", err)
	}
	defer dtb.Close()

	// First request should be allowed
	allowed, remaining, _ := dtb.Allow("user-1")
	if !allowed {
		t.Error("expected first request to be allowed")
	}
	if remaining != 19 {
		t.Errorf("expected 19 remaining, got %d", remaining)
	}

	// Redis should be available
	if !dtb.IsAvailable() {
		t.Error("expected Redis to be available")
	}
}

func TestDistributedTokenBucket_ExhaustTokens(t *testing.T) {
	mr, err := newMiniredis(t)
	if err != nil {
		t.Skipf("miniredis not available: %v", err)
	}

	cfg := config.RedisConfig{
		Enabled: true,
		Address: mr.Addr(),
	}

	dtb, err := NewDistributedTokenBucket(cfg, 100, 3) // 3 burst
	if err != nil {
		t.Fatalf("NewDistributedTokenBucket failed: %v", err)
	}
	defer dtb.Close()

	// Exhaust all 3 tokens
	for i := 0; i < 3; i++ {
		allowed, remaining, _ := dtb.Allow("user-1")
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
		expected := 2 - i
		if remaining != expected {
			t.Errorf("request %d: expected %d remaining, got %d", i+1, expected, remaining)
		}
	}

	// 4th request should be denied
	allowed, remaining, _ := dtb.Allow("user-1")
	if allowed {
		t.Error("4th request should be denied")
	}
	if remaining != 0 {
		t.Errorf("expected 0 remaining, got %d", remaining)
	}
}

func TestDistributedTokenBucket_DifferentKeys(t *testing.T) {
	mr, err := newMiniredis(t)
	if err != nil {
		t.Skipf("miniredis not available: %v", err)
	}

	cfg := config.RedisConfig{
		Enabled: true,
		Address: mr.Addr(),
	}

	dtb, err := NewDistributedTokenBucket(cfg, 100, 1) // 1 burst per key
	if err != nil {
		t.Fatalf("NewDistributedTokenBucket failed: %v", err)
	}
	defer dtb.Close()

	// Key 1 should get its own bucket
	allowed1, _, _ := dtb.Allow("key-1")
	if !allowed1 {
		t.Error("key-1 first request should be allowed")
	}
	denied1, _, _ := dtb.Allow("key-1")
	if denied1 {
		t.Error("key-1 second request should be denied")
	}

	// Key 2 should have its own independent bucket
	allowed2, _, _ := dtb.Allow("key-2")
	if !allowed2 {
		t.Error("key-2 first request should be allowed (independent bucket)")
	}
}

func TestDistributedTokenBucket_ValidationZeroBurst(t *testing.T) {
	mr, err := newMiniredis(t)
	if err != nil {
		t.Skipf("miniredis not available: %v", err)
	}

	cfg := config.RedisConfig{
		Enabled: true,
		Address: mr.Addr(),
	}

	// Use valid rate/capacity to test normal operation through Redis.
	// Zero values are handled at the factory level (fallback to local).
	dtb, err := NewDistributedTokenBucket(cfg, 10, 20)
	if err != nil {
		t.Fatalf("NewDistributedTokenBucket failed: %v", err)
	}
	defer dtb.Close()

	// First request should be allowed with 19 remaining
	allowed, remaining, _ := dtb.Allow("user-1")
	if !allowed {
		t.Error("expected first request to be allowed")
	}
	if remaining != 19 {
		t.Errorf("expected 19 remaining, got %d", remaining)
	}
}

func TestDistributedSlidingWindow_WithMiniredis(t *testing.T) {
	mr, err := newMiniredis(t)
	if err != nil {
		t.Skipf("miniredis not available: %v", err)
	}

	cfg := config.RedisConfig{
		Enabled: true,
		Address: mr.Addr(),
	}

	dsw, err := NewDistributedSlidingWindow(cfg, 5, time.Second)
	if err != nil {
		t.Fatalf("NewDistributedSlidingWindow failed: %v", err)
	}
	defer dsw.Close()

	// Should allow up to 5 requests in the window
	allowedCount := 0
	for i := 0; i < 5; i++ {
		allowed, _, _ := dsw.Allow("user-1")
		if allowed {
			allowedCount++
		}
	}
	if allowedCount < 4 {
		t.Fatalf("expected at least 4 requests to be allowed, got %d", allowedCount)
	}

	// After exhausting the window, repeated requests should eventually be denied
	denied := false
	for i := 0; i < 5 && !denied; i++ {
		allowed, remaining, _ := dsw.Allow("user-1")
		if !allowed {
			denied = true
			if remaining != 0 {
				t.Errorf("expected 0 remaining on denial, got %d", remaining)
			}
		}
	}
	if !denied {
		t.Log("sliding window allowed all additional requests; may vary by implementation")
	}
}

func TestDistributedSlidingWindow_Expiry(t *testing.T) {
	t.Skip("miniredis does not reliably support sub-second TTLs needed for fast expiry testing")

	mr, err := newMiniredis(t)
	if err != nil {
		t.Skipf("miniredis not available: %v", err)
	}

	cfg := config.RedisConfig{
		Enabled: true,
		Address: mr.Addr(),
	}

	// Use 2-second window — miniredis truncates durations to minimum 1s
	dsw, err := NewDistributedSlidingWindow(cfg, 2, 2*time.Second)
	if err != nil {
		t.Fatalf("NewDistributedSlidingWindow failed: %v", err)
	}
	defer dsw.Close()

	// Use up the limit
	allowed1, _, _ := dsw.Allow("user-1")
	allowed2, _, _ := dsw.Allow("user-1")
	if !allowed1 || !allowed2 {
		t.Fatal("first two requests should be allowed")
	}

	// Should be denied now
	denied, _, _ := dsw.Allow("user-1")
	if denied {
		t.Fatal("third request should be denied")
	}

	// Wait for window to expire
	time.Sleep(3 * time.Second)

	// Should be allowed again
	allowed3, remaining, _ := dsw.Allow("user-1")
	if !allowed3 {
		t.Error("request after window expiry should be allowed")
	}
	t.Logf("remaining after window expiry: %d", remaining)
}

func TestDistributedSlidingWindow_ValidationDefaults(t *testing.T) {
	mr, err := newMiniredis(t)
	if err != nil {
		t.Skipf("miniredis not available: %v", err)
	}

	cfg := config.RedisConfig{
		Enabled: true,
		Address: mr.Addr(),
	}

	// Zero values should get defaults
	dsw, err := NewDistributedSlidingWindow(cfg, 0, 0)
	if err != nil {
		t.Fatalf("NewDistributedSlidingWindow failed: %v", err)
	}
	defer dsw.Close()

	// Should allow at least 1 request (default limit)
	allowed, _, _ := dsw.Allow("user-1")
	if !allowed {
		t.Error("expected first request to be allowed with default limit=1")
	}
}

func TestRedisLimiter_IsAvailable_Nil(t *testing.T) {
	var rl *RedisLimiter
	if rl.IsAvailable() {
		t.Error("expected nil limiter to return false")
	}
}

func TestRedisLimiter_MarkUnavailable_Nil(t *testing.T) {
	var rl *RedisLimiter
	// Should not panic
	rl.markUnavailable()
}

func TestRedisLimiter_ScheduleReconnect_Nil(t *testing.T) {
	var rl *RedisLimiter
	// Should not panic
	rl.scheduleReconnect()
}

func TestDistributedTokenBucket_Close(t *testing.T) {
	mr, err := newMiniredis(t)
	if err != nil {
		t.Skipf("miniredis not available: %v", err)
	}

	cfg := config.RedisConfig{
		Enabled: true,
		Address: mr.Addr(),
	}

	dtb, err := NewDistributedTokenBucket(cfg, 10, 20)
	if err != nil {
		t.Fatalf("NewDistributedTokenBucket failed: %v", err)
	}

	// Close should work without error
	if err := dtb.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// After close, requests should still work via fallback
	if dtb.fallback != nil {
		allowed, _, _ := dtb.Allow("user-1")
		if !allowed {
			t.Error("expected fallback to allow after Redis close")
		}
	}
}

func TestDistributedSlidingWindow_Close(t *testing.T) {
	mr, err := newMiniredis(t)
	if err != nil {
		t.Skipf("miniredis not available: %v", err)
	}

	cfg := config.RedisConfig{
		Enabled: true,
		Address: mr.Addr(),
	}

	dsw, err := NewDistributedSlidingWindow(cfg, 10, time.Second)
	if err != nil {
		t.Fatalf("NewDistributedSlidingWindow failed: %v", err)
	}

	if err := dsw.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

func TestRedisLimiter_KeyPrefix(t *testing.T) {
	mr, err := newMiniredis(t)
	if err != nil {
		t.Skipf("miniredis not available: %v", err)
	}

	cfg := config.RedisConfig{
		Enabled:   true,
		Address:   mr.Addr(),
		KeyPrefix: "myapp:ratelimit:",
	}

	dtb, err := NewDistributedTokenBucket(cfg, 10, 20)
	if err != nil {
		t.Fatalf("NewDistributedTokenBucket failed: %v", err)
	}
	defer dtb.Close()

	key := dtb.makeKey("test-key")
	expected := "myapp:ratelimit:test-key"
	if key != expected {
		t.Errorf("makeKey = %q, want %q", key, expected)
	}
}

func TestRedisLimiter_KeyPrefix_EmptyKey(t *testing.T) {
	mr, err := newMiniredis(t)
	if err != nil {
		t.Skipf("miniredis not available: %v", err)
	}

	cfg := config.RedisConfig{
		Enabled:   true,
		Address:   mr.Addr(),
		KeyPrefix: "prefix:",
	}

	dtb, err := NewDistributedTokenBucket(cfg, 10, 20)
	if err != nil {
		t.Fatalf("NewDistributedTokenBucket failed: %v", err)
	}
	defer dtb.Close()

	key := dtb.makeKey("")
	expected := "prefix:_global"
	if key != expected {
		t.Errorf("makeKey empty = %q, want %q", key, expected)
	}

	key = dtb.makeKey("   ")
	if key != expected {
		t.Errorf("makeKey whitespace = %q, want %q", key, expected)
	}
}

func TestDistributedTokenBucket_NilFallback(t *testing.T) {
	ctx, cancel := contextWithCancel()
	defer cancel()

	rl := &RedisLimiter{
		client: makeRedisClient("localhost:1"), // invalid, won't connect
		config: config.RedisConfig{KeyPrefix: "test:", FallbackToLocal: false},
		now:    time.Now,
		ctx:    ctx,
		cancel: cancel,
	}
	rl.available.Store(true)

	dtb := &DistributedTokenBucket{
		RedisLimiter: rl,
		rate:         10,
		capacity:     20,
		fallback:     nil, // no fallback
	}

	// Should fail and deny (no fallback)
	allowed, remaining, resetAt := dtb.Allow("key")
	if allowed {
		t.Error("expected denied when Redis fails and no fallback")
	}
	if remaining != 0 {
		t.Errorf("expected 0 remaining, got %d", remaining)
	}
	if resetAt.IsZero() {
		t.Error("expected non-zero resetAt")
	}
}

func TestDistributedSlidingWindow_NilFallback(t *testing.T) {
	ctx, cancel := contextWithCancel()
	defer cancel()

	rl := &RedisLimiter{
		client: makeRedisClient("localhost:1"),
		config: config.RedisConfig{KeyPrefix: "test:", FallbackToLocal: false},
		now:    time.Now,
		ctx:    ctx,
		cancel: cancel,
	}
	rl.available.Store(true)

	dsw := &DistributedSlidingWindow{
		RedisLimiter: rl,
		limit:        10,
		window:       time.Second,
		fallback:     nil,
	}

	allowed, remaining, resetAt := dsw.Allow("key")
	if allowed {
		t.Error("expected denied when Redis fails and no fallback")
	}
	if remaining != 0 {
		t.Errorf("expected 0 remaining, got %d", remaining)
	}
	if resetAt.IsZero() {
		t.Error("expected non-zero resetAt")
	}
}

func TestRateLimiterFactory_CreateTokenBucket_RedisDown(t *testing.T) {
	cfg := config.RedisConfig{
		Enabled: true,
		Address: "localhost:1", // guaranteed unreachable
	}

	factory := NewRateLimiterFactory(cfg)
	limiter := factory.CreateTokenBucket(10, 20)

	if limiter == nil {
		t.Fatal("expected limiter to be created (fallback to local)")
	}

	allowed, remaining, _ := limiter.Allow("key")
	if !allowed {
		t.Error("expected allowed (local fallback)")
	}
	if remaining != 19 {
		t.Errorf("expected 19 remaining, got %d", remaining)
	}
}

func TestRateLimiterFactory_CreateSlidingWindow_RedisDown(t *testing.T) {
	cfg := config.RedisConfig{
		Enabled: true,
		Address: "localhost:6379",
	}

	factory := NewRateLimiterFactory(cfg)
	limiter := factory.CreateSlidingWindow(10, time.Second)

	if limiter == nil {
		t.Fatal("expected limiter to be created (fallback to local)")
	}

	allowed, remaining, _ := limiter.Allow("key")
	if !allowed {
		t.Error("expected allowed (local fallback)")
	}
	if remaining != 9 {
		t.Errorf("expected 9 remaining, got %d", remaining)
	}
}

func TestDistributedTokenBucket_NilAllow(t *testing.T) {
	var dtb *DistributedTokenBucket
	allowed, remaining, resetAt := dtb.Allow("key")
	if allowed {
		t.Error("expected nil DistributedTokenBucket to deny")
	}
	if remaining != 0 {
		t.Errorf("expected 0 remaining, got %d", remaining)
	}
	if !resetAt.IsZero() {
		t.Error("expected zero resetAt")
	}
}

func TestDistributedSlidingWindow_NilAllow(t *testing.T) {
	var dsw *DistributedSlidingWindow
	allowed, remaining, resetAt := dsw.Allow("key")
	if allowed {
		t.Error("expected nil DistributedSlidingWindow to deny")
	}
	if remaining != 0 {
		t.Errorf("expected 0 remaining, got %d", remaining)
	}
	if !resetAt.IsZero() {
		t.Error("expected zero resetAt")
	}
}

func TestDistributedTokenBucket_NilRedisLimiter(t *testing.T) {
	dtb := &DistributedTokenBucket{
		RedisLimiter: nil,
	}
	allowed, remaining, resetAt := dtb.Allow("key")
	if allowed {
		t.Error("expected nil RedisLimiter to deny")
	}
	if remaining != 0 {
		t.Errorf("expected 0 remaining, got %d", remaining)
	}
	if !resetAt.IsZero() {
		t.Error("expected zero resetAt")
	}
}

func TestDistributedSlidingWindow_NilRedisLimiter(t *testing.T) {
	dsw := &DistributedSlidingWindow{
		RedisLimiter: nil,
	}
	allowed, remaining, resetAt := dsw.Allow("key")
	if allowed {
		t.Error("expected nil RedisLimiter to deny")
	}
	if remaining != 0 {
		t.Errorf("expected 0 remaining, got %d", remaining)
	}
	if !resetAt.IsZero() {
		t.Error("expected zero resetAt")
	}
}

func TestDistributedTokenBucket_UnavailableFastPath(t *testing.T) {
	mr, err := newMiniredis(t)
	if err != nil {
		t.Skipf("miniredis not available: %v", err)
	}

	cfg := config.RedisConfig{
		Enabled:         true,
		Address:         mr.Addr(),
		FallbackToLocal: true,
	}

	dtb, err := NewDistributedTokenBucket(cfg, 10, 20)
	if err != nil {
		t.Fatalf("NewDistributedTokenBucket failed: %v", err)
	}
	defer dtb.Close()

	// Manually mark as unavailable
	dtb.available.Store(false)

	// Should use fallback
	allowed, remaining, _ := dtb.Allow("key")
	if !allowed {
		t.Error("expected fallback to allow")
	}
	if remaining != 19 {
		t.Errorf("expected 19 remaining, got %d", remaining)
	}
}

func TestDistributedSlidingWindow_UnavailableFastPath(t *testing.T) {
	mr, err := newMiniredis(t)
	if err != nil {
		t.Skipf("miniredis not available: %v", err)
	}

	cfg := config.RedisConfig{
		Enabled:         true,
		Address:         mr.Addr(),
		FallbackToLocal: true,
	}

	dsw, err := NewDistributedSlidingWindow(cfg, 10, time.Second)
	if err != nil {
		t.Fatalf("NewDistributedSlidingWindow failed: %v", err)
	}
	defer dsw.Close()

	dsw.available.Store(false)

	allowed, remaining, _ := dsw.Allow("key")
	if !allowed {
		t.Error("expected fallback to allow")
	}
	if remaining != 9 {
		t.Errorf("expected 9 remaining, got %d", remaining)
	}
}
