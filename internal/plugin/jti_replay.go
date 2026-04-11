package plugin

import (
	"sort"
	"sync"
	"time"
)

// JTIReplayCache tracks JWT IDs (jti) to prevent token replay attacks.
// It stores seen JTIs with per-entry TTLs based on the token's remaining
// lifetime. Entries are automatically evicted on expiry or when the cache
// exceeds its maximum size (oldest entries evicted first).
type JTIReplayCache struct {
	mu      sync.Mutex
	entries map[string]time.Time // jti -> expiry
	maxSize int
	now     func() time.Time
}

// DefaultJTIReplayCacheMaxSize is the default maximum number of JTIs stored.
const DefaultJTIReplayCacheMaxSize = 10000

// NewJTIReplayCache creates a replay cache with periodic cleanup.
func NewJTIReplayCache() *JTIReplayCache {
	return NewJTIReplayCacheWithSize(DefaultJTIReplayCacheMaxSize)
}

// NewJTIReplayCacheWithSize creates a replay cache with a maximum entry count.
func NewJTIReplayCacheWithSize(maxSize int) *JTIReplayCache {
	if maxSize <= 0 {
		maxSize = DefaultJTIReplayCacheMaxSize
	}
	c := &JTIReplayCache{
		entries: make(map[string]time.Time),
		maxSize: maxSize,
		now:     time.Now,
	}
	go c.cleanupLoop(5 * time.Minute)
	return c
}

// Seen returns true if the jti has been seen and is not yet expired.
func (c *JTIReplayCache) Seen(jti string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	exp, ok := c.entries[jti]
	if !ok {
		return false
	}
	if c.now().After(exp) {
		delete(c.entries, jti)
		return false
	}
	return true
}

// Add registers a jti with the given TTL.
// If the cache is at capacity, the oldest entries are evicted.
func (c *JTIReplayCache) Add(jti string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If entry already exists, just update the expiry.
	if _, exists := c.entries[jti]; exists {
		c.entries[jti] = c.now().Add(ttl)
		return
	}

	// Evict expired entries first to free space.
	c.evictExpiredLocked()

	// If still at capacity, evict oldest entries to make room.
	if len(c.entries) >= c.maxSize {
		c.evictOldestLocked(c.maxSize / 4) // Evict 25% to reduce churn
	}

	c.entries[jti] = c.now().Add(ttl)
}

// Len returns the current number of entries (for testing).
func (c *JTIReplayCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictExpiredLocked()
	return len(c.entries)
}

func (c *JTIReplayCache) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		c.evictExpiredLocked()
		c.mu.Unlock()
	}
}

// evictExpiredLocked removes all expired entries. Caller must hold c.mu.
func (c *JTIReplayCache) evictExpiredLocked() {
	now := c.now()
	for jti, exp := range c.entries {
		if now.After(exp) {
			delete(c.entries, jti)
		}
	}
}

// evictOldestLocked removes the n entries with the earliest expiry times.
// Caller must hold c.mu.
func (c *JTIReplayCache) evictOldestLocked(n int) {
	if n <= 0 || len(c.entries) == 0 {
		return
	}

	type entry struct {
		jti string
		exp time.Time
	}

	entries := make([]entry, 0, len(c.entries))
	for jti, exp := range c.entries {
		entries = append(entries, entry{jti, exp})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].exp.Before(entries[j].exp)
	})

	toRemove := n
	if toRemove > len(entries) {
		toRemove = len(entries)
	}

	for i := 0; i < toRemove; i++ {
		delete(c.entries, entries[i].jti)
	}
}
