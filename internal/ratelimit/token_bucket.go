package ratelimit

import (
	"math"
	"strings"
	"sync"
	"time"
)

type tokenBucketState struct {
	mu     sync.Mutex
	tokens float64
	last   time.Time
}

// TokenBucket is an in-memory token bucket limiter keyed by scope key.
type TokenBucket struct {
	rate     float64
	capacity float64
	buckets  sync.Map // map[string]*tokenBucketState
	now      func() time.Time
}

// NewTokenBucket creates limiter with refill rate (tokens/sec) and burst capacity.
func NewTokenBucket(requestsPerSecond, burst int) *TokenBucket {
	capacity := float64(burst)
	if capacity <= 0 {
		capacity = 1
	}
	rate := float64(requestsPerSecond)
	if rate < 0 {
		rate = 0
	}
	return &TokenBucket{
		rate:     rate,
		capacity: capacity,
		now:      time.Now,
	}
}

// Allow consumes one token for key and returns allow decision, remaining tokens and reset time.
func (tb *TokenBucket) Allow(key string) (allowed bool, remaining int, resetAt time.Time) {
	if tb == nil {
		return false, 0, time.Time{}
	}
	key = strings.TrimSpace(key)
	if key == "" {
		key = "_global"
	}

	now := tb.now()
	// M-005: Load inside lock to prevent race between LoadOrStore and Lock.
	// Previous code used LoadOrStore outside lock, which could cause concurrent
	// goroutines to retrieve the same state entry before acquiring the mutex,
	// leading to potential capacity violations during refill.
	raw, _ := tb.buckets.LoadOrStore(key, &tokenBucketState{
		tokens: tb.capacity,
		last:   now,
	})
	state := raw.(*tokenBucketState)

	state.mu.Lock()
	defer state.mu.Unlock()

	tb.refill(state, now)
	if state.tokens >= 1 {
		state.tokens -= 1
		allowed = true
	}
	if state.tokens < 0 {
		state.tokens = 0
	}

	remaining = int(math.Floor(state.tokens))
	if remaining < 0 {
		remaining = 0
	}

	resetAt = tb.nextRefillAt(state.tokens, now)
	state.last = now
	return allowed, remaining, resetAt
}

func (tb *TokenBucket) refill(state *tokenBucketState, now time.Time) {
	if state == nil || tb.rate <= 0 {
		return
	}
	elapsed := now.Sub(state.last).Seconds()
	if elapsed <= 0 {
		return
	}
	state.tokens += elapsed * tb.rate
	if state.tokens > tb.capacity {
		state.tokens = tb.capacity
	}
}

func (tb *TokenBucket) nextRefillAt(tokens float64, now time.Time) time.Time {
	if tb.rate <= 0 || tokens >= 1 {
		return now
	}
	need := 1 - tokens
	seconds := need / tb.rate
	if seconds <= 0 {
		return now
	}
	return now.Add(time.Duration(seconds * float64(time.Second)))
}

// PurgeStale removes keys whose bucket was last accessed before cutoff.
func (tb *TokenBucket) PurgeStale(cutoff time.Time) {
	if tb == nil {
		return
	}
	tb.buckets.Range(func(key, value any) bool {
		state := value.(*tokenBucketState)
		state.mu.Lock()
		stale := state.last.Before(cutoff)
		state.mu.Unlock()
		if stale {
			tb.buckets.Delete(key)
		}
		return true
	})
}
