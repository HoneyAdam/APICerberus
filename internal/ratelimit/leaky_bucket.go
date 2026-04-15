package ratelimit

import (
	"math"
	"strings"
	"sync"
	"time"
)

type leakyBucketState struct {
	mu        sync.Mutex
	queue     float64
	updatedAt time.Time
}

// LeakyBucket smooths throughput by draining queue at a fixed rate.
type LeakyBucket struct {
	capacity int
	leakRate float64 // requests per second
	buckets  sync.Map
	now      func() time.Time
}

func NewLeakyBucket(capacity int, leakPerSecond int) *LeakyBucket {
	if capacity <= 0 {
		capacity = 1
	}
	rate := float64(leakPerSecond)
	if rate <= 0 {
		rate = 1
	}
	return &LeakyBucket{
		capacity: capacity,
		leakRate: rate,
		now:      time.Now,
	}
}

// Allow enqueues one request if capacity allows and returns decision/remaining/resetAt.
func (lb *LeakyBucket) Allow(key string) (allowed bool, remaining int, resetAt time.Time) {
	if lb == nil {
		return false, 0, time.Time{}
	}
	key = strings.TrimSpace(key)
	if key == "" {
		key = "_global"
	}
	now := lb.now()
	raw, _ := lb.buckets.LoadOrStore(key, &leakyBucketState{updatedAt: now})
	state := raw.(*leakyBucketState)

	state.mu.Lock()
	defer state.mu.Unlock()

	lb.drainLocked(state, now)
	// M-006 FIX: Check integer queue against capacity to prevent fractional overage.
	// Before: state.queue+1 > capacity (allows queue=capacity to pass, then increments to capacity+1)
	// After: int(state.queue)+1 > capacity (only allows if integer queue < capacity)
	if float64(int(state.queue))+1 > float64(lb.capacity) {
		remaining = 0
		resetAt = lb.nextReset(now, state.queue)
		return false, remaining, resetAt
	}

	state.queue++
	remaining = lb.capacity - int(math.Ceil(state.queue))
	if remaining < 0 {
		remaining = 0
	}
	resetAt = lb.nextReset(now, state.queue)
	return true, remaining, resetAt
}

func (lb *LeakyBucket) drainLocked(state *leakyBucketState, now time.Time) {
	if state == nil {
		return
	}
	elapsed := now.Sub(state.updatedAt).Seconds()
	if elapsed <= 0 {
		return
	}
	drained := elapsed * lb.leakRate
	state.queue -= drained
	if state.queue < 0 {
		state.queue = 0
	}
	state.updatedAt = now
}

func (lb *LeakyBucket) nextReset(now time.Time, queue float64) time.Time {
	if lb.leakRate <= 0 || queue <= 0 {
		return now
	}
	seconds := queue / lb.leakRate
	return now.Add(time.Duration(seconds * float64(time.Second)))
}

// PurgeStale removes keys whose bucket was last updated before cutoff.
func (lb *LeakyBucket) PurgeStale(cutoff time.Time) {
	if lb == nil {
		return
	}
	lb.buckets.Range(func(key, value any) bool {
		state := value.(*leakyBucketState)
		state.mu.Lock()
		stale := state.updatedAt.Before(cutoff)
		state.mu.Unlock()
		if stale {
			lb.buckets.Delete(key)
		}
		return true
	})
}
