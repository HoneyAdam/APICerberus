package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	"github.com/redis/go-redis/v9"
)

// RedisLimiter provides distributed rate limiting using Redis.
type RedisLimiter struct {
	client *redis.Client
	config config.RedisConfig
	now    func() time.Time
	ctx    context.Context
	cancel context.CancelFunc
}

// NewRedisLimiter creates a new Redis-backed rate limiter.
func NewRedisLimiter(cfg config.RedisConfig) (*RedisLimiter, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("redis is disabled")
	}

	if cfg.Address == "" {
		cfg.Address = "localhost:6379"
	}

	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}

	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 5 * time.Second
	}

	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 3 * time.Second
	}

	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 3 * time.Second
	}

	if cfg.PoolSize == 0 {
		cfg.PoolSize = 10
	}

	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "ratelimit:"
	}

	ctx, cancel := context.WithCancel(context.Background())

	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Address,
		Password:     cfg.Password,
		DB:           cfg.Database,
		MaxRetries:   cfg.MaxRetries,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
	})

	// Test connection
	if err := client.Ping(ctx).Err(); err != nil {
		cancel()
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &RedisLimiter{
		client: client,
		config: cfg,
		now:    time.Now,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// Close closes the Redis connection.
func (rl *RedisLimiter) Close() error {
	rl.cancel()
	return rl.client.Close()
}

// IsAvailable returns true if Redis is available.
func (rl *RedisLimiter) IsAvailable() bool {
	return rl.client.Ping(rl.ctx).Err() == nil
}

// makeKey creates a Redis key with the configured prefix.
func (rl *RedisLimiter) makeKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		key = "_global"
	}
	return rl.config.KeyPrefix + key
}

// DistributedTokenBucket provides distributed token bucket rate limiting.
type DistributedTokenBucket struct {
	*RedisLimiter
	rate     float64
	capacity float64
	fallback *TokenBucket
}

// NewDistributedTokenBucket creates a new distributed token bucket limiter.
func NewDistributedTokenBucket(redisCfg config.RedisConfig, requestsPerSecond, burst int) (*DistributedTokenBucket, error) {
	capacity := float64(burst)
	if capacity <= 0 {
		capacity = 1
	}
	rate := float64(requestsPerSecond)
	if rate < 0 {
		rate = 0
	}

	redisLimiter, err := NewRedisLimiter(redisCfg)
	if err != nil {
		return nil, err
	}

	var fallback *TokenBucket
	if redisCfg.FallbackToLocal {
		fallback = NewTokenBucket(requestsPerSecond, burst)
	}

	return &DistributedTokenBucket{
		RedisLimiter: redisLimiter,
		rate:         rate,
		capacity:     capacity,
		fallback:     fallback,
	}, nil
}

// Allow consumes one token and returns decision, remaining tokens, and reset time.
func (dtb *DistributedTokenBucket) Allow(key string) (allowed bool, remaining int, resetAt time.Time) {
	if dtb == nil || dtb.RedisLimiter == nil {
		return false, 0, time.Time{}
	}

	redisKey := dtb.makeKey("tb:" + key)
	now := dtb.now()

	// Use Lua script for atomic operation
	result, err := dtb.evaluateTokenBucket(redisKey, now)
	if err != nil {
		// Fallback to local if configured
		if dtb.fallback != nil {
			return dtb.fallback.Allow(key)
		}
		return false, 0, now.Add(time.Second)
	}

	return result.allowed, result.remaining, result.resetAt
}

type tokenBucketResult struct {
	allowed   bool
	remaining int
	resetAt   time.Time
}

// evaluateTokenBucket executes the token bucket logic in Redis using Lua.
func (dtb *DistributedTokenBucket) evaluateTokenBucket(key string, now time.Time) (*tokenBucketResult, error) {
	script := `
		local key = KEYS[1]
		local rate = tonumber(ARGV[1])
		local capacity = tonumber(ARGV[2])
		local now = tonumber(ARGV[3])
		local window = tonumber(ARGV[4])

		local data = redis.call('HMGET', key, 'tokens', 'last')
		local tokens = tonumber(data[1]) or capacity
		local last = tonumber(data[2]) or now

		-- Calculate elapsed time and refill tokens
		local elapsed = math.max(0, now - last)
		local refill = elapsed * rate / window
		tokens = math.min(capacity, tokens + refill)

		-- Try to consume token
		local allowed = 0
		if tokens >= 1 then
			tokens = tokens - 1
			allowed = 1
		end

		-- Calculate reset time
		local need = 1 - tokens
		local resetAt = now
		if need > 0 and rate > 0 then
			resetAt = now + math.ceil(need * window / rate)
		end

		-- Update Redis
		redis.call('HMSET', key, 'tokens', tokens, 'last', now)
		redis.call('EXPIRE', key, math.ceil(capacity / rate * window) + 1)

		return {allowed, math.floor(tokens), resetAt}
	`

	window := 1.0 // 1 second window
	result, err := dtb.client.Eval(dtb.ctx, script, []string{key},
		dtb.rate, dtb.capacity, now.Unix(), window).Result()

	if err != nil {
		return nil, err
	}

	values := result.([]any)
	allowed := values[0].(int64) == 1
	remaining := int(values[1].(int64))
	resetAt := time.Unix(values[2].(int64), 0)

	return &tokenBucketResult{
		allowed:   allowed,
		remaining: remaining,
		resetAt:   resetAt,
	}, nil
}

// DistributedSlidingWindow provides distributed sliding window rate limiting.
type DistributedSlidingWindow struct {
	*RedisLimiter
	limit    int64
	window   time.Duration
	fallback *SlidingWindow
}

// NewDistributedSlidingWindow creates a new distributed sliding window limiter.
func NewDistributedSlidingWindow(redisCfg config.RedisConfig, limit int, window time.Duration) (*DistributedSlidingWindow, error) {
	if limit <= 0 {
		limit = 1
	}
	if window <= 0 {
		window = time.Second
	}

	redisLimiter, err := NewRedisLimiter(redisCfg)
	if err != nil {
		return nil, err
	}

	var fallback *SlidingWindow
	if redisCfg.FallbackToLocal {
		fallback = NewSlidingWindow(limit, window)
	}

	return &DistributedSlidingWindow{
		RedisLimiter: redisLimiter,
		limit:        int64(limit),
		window:       window,
		fallback:     fallback,
	}, nil
}

// Allow consumes one event and returns decision, remaining count, and reset time.
func (dsw *DistributedSlidingWindow) Allow(key string) (allowed bool, remaining int, resetAt time.Time) {
	if dsw == nil || dsw.RedisLimiter == nil {
		return false, 0, time.Time{}
	}

	redisKey := dsw.makeKey("sw:" + key)
	now := dsw.now()

	result, err := dsw.evaluateSlidingWindow(redisKey, now)
	if err != nil {
		if dsw.fallback != nil {
			return dsw.fallback.Allow(key)
		}
		return false, 0, now.Add(dsw.window)
	}

	return result.allowed, result.remaining, result.resetAt
}

type slidingWindowResult struct {
	allowed   bool
	remaining int
	resetAt   time.Time
}

// evaluateSlidingWindow executes the sliding window logic in Redis using Redis sorted sets.
func (dsw *DistributedSlidingWindow) evaluateSlidingWindow(key string, now time.Time) (*slidingWindowResult, error) {
	windowStart := now.Add(-dsw.window).UnixMilli()
	nowMilli := now.UnixMilli()

	pipe := dsw.client.Pipeline()

	// Remove old entries outside the window
	pipe.ZRemRangeByScore(dsw.ctx, key, "0", fmt.Sprintf("%d", windowStart))

	// Count current entries in window
	countCmd := pipe.ZCard(dsw.ctx, key)

	// Execute pipeline
	_, err := pipe.Exec(dsw.ctx)
	if err != nil {
		return nil, err
	}

	count := countCmd.Val()

	if count >= dsw.limit {
		// Get the oldest entry to calculate reset time
		rangeResult, err := dsw.client.ZRangeWithScores(dsw.ctx, key, 0, 0).Result()
		if err != nil {
			return nil, err
		}

		var resetTime time.Time
		if len(rangeResult) > 0 {
			oldest := int64(rangeResult[0].Score)
			resetTime = time.UnixMilli(oldest).Add(dsw.window)
		} else {
			resetTime = now.Add(dsw.window)
		}

		return &slidingWindowResult{
			allowed:   false,
			remaining: 0,
			resetAt:   resetTime,
		}, nil
	}

	// Add current request
	member := fmt.Sprintf("%d:%s", nowMilli, generateRequestID())
	err = dsw.client.ZAdd(dsw.ctx, key, redis.Z{
		Score:  float64(nowMilli),
		Member: member,
	}).Err()

	if err != nil {
		return nil, err
	}

	// Set expiration
	dsw.client.Expire(dsw.ctx, key, dsw.window*2)

	remaining := int(dsw.limit - count - 1)
	if remaining < 0 {
		remaining = 0
	}

	return &slidingWindowResult{
		allowed:   true,
		remaining: remaining,
		resetAt:   now.Add(dsw.window),
	}, nil
}

// generateRequestID creates a short unique identifier for request tracking.
func generateRequestID() string {
	timestamp := time.Now().UnixNano()
	hash := sha256.Sum256([]byte(fmt.Sprintf("%d", timestamp)))
	return hex.EncodeToString(hash[:])[:8]
}

// RateLimiterFactory creates rate limiters based on configuration.
type RateLimiterFactory struct {
	redisConfig config.RedisConfig
}

// NewRateLimiterFactory creates a new factory with the given Redis config.
func NewRateLimiterFactory(redisConfig config.RedisConfig) *RateLimiterFactory {
	return &RateLimiterFactory{
		redisConfig: redisConfig,
	}
}

// CreateTokenBucket creates either a distributed or local token bucket based on Redis availability.
func (f *RateLimiterFactory) CreateTokenBucket(requestsPerSecond, burst int) TokenLimiter {
	if !f.redisConfig.Enabled {
		return NewTokenBucket(requestsPerSecond, burst)
	}

	distributed, err := NewDistributedTokenBucket(f.redisConfig, requestsPerSecond, burst)
	if err != nil {
		// Fall back to local if Redis is not available
		return NewTokenBucket(requestsPerSecond, burst)
	}

	return distributed
}

// CreateSlidingWindow creates either a distributed or local sliding window based on Redis availability.
func (f *RateLimiterFactory) CreateSlidingWindow(limit int, window time.Duration) WindowLimiter {
	if !f.redisConfig.Enabled {
		return NewSlidingWindow(limit, window)
	}

	distributed, err := NewDistributedSlidingWindow(f.redisConfig, limit, window)
	if err != nil {
		return NewSlidingWindow(limit, window)
	}

	return distributed
}

// TokenLimiter defines the interface for token bucket rate limiters.
type TokenLimiter interface {
	Allow(key string) (allowed bool, remaining int, resetAt time.Time)
}

// WindowLimiter defines the interface for window-based rate limiters.
type WindowLimiter interface {
	Allow(key string) (allowed bool, remaining int, resetAt time.Time)
}
