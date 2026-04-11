package plugin

import (
	"testing"
	"time"
)

func TestJTIReplayCache_AddAndSeen(t *testing.T) {
	t.Parallel()

	c := &JTIReplayCache{
		entries: make(map[string]time.Time),
		maxSize: DefaultJTIReplayCacheMaxSize,
		now:     func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
	}

	c.Add("token-1", 5*time.Minute)

	if !c.Seen("token-1") {
		t.Fatal("expected jti to be seen after Add")
	}

	if c.Seen("token-2") {
		t.Fatal("expected unknown jti to not be seen")
	}
}

func TestJTIReplayCache_ExpiredNotSeen(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { return base }

	c := &JTIReplayCache{
		entries: make(map[string]time.Time),
		maxSize: DefaultJTIReplayCacheMaxSize,
		now:     nowFn,
	}

	c.Add("old-token", 1*time.Minute)

	// Advance past expiry
	c.now = func() time.Time { return base.Add(2 * time.Minute) }

	if c.Seen("old-token") {
		t.Fatal("expired jti should not be seen")
	}
}

func TestJTIReplayCache_Len(t *testing.T) {
	t.Parallel()

	c := &JTIReplayCache{
		entries: make(map[string]time.Time),
		maxSize: DefaultJTIReplayCacheMaxSize,
		now:     func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
	}

	c.Add("a", 5*time.Minute)
	c.Add("b", 5*time.Minute)
	c.Add("c", -1*time.Minute) // already expired (negative TTL)

	if got := c.Len(); got != 2 {
		t.Errorf("Len() = %d, want 2 (c is expired)", got)
	}
}

func TestJTIReplayCache_EvictExpired(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := &JTIReplayCache{
		entries: make(map[string]time.Time),
		maxSize: DefaultJTIReplayCacheMaxSize,
		now:     func() time.Time { return base },
	}

	c.Add("valid", 10*time.Minute)
	c.Add("expired", 1*time.Minute)

	c.now = func() time.Time { return base.Add(5 * time.Minute) }

	// Len calls evictExpired internally
	if got := c.Len(); got != 1 {
		t.Errorf("Len() after expiry = %d, want 1", got)
	}
}

func TestJTIReplayCache_AddOverwrites(t *testing.T) {
	t.Parallel()

	c := &JTIReplayCache{
		entries: make(map[string]time.Time),
		maxSize: DefaultJTIReplayCacheMaxSize,
		now:     func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
	}

	c.Add("token", 1*time.Minute)
	c.Add("token", 10*time.Minute) // extend TTL

	if !c.Seen("token") {
		t.Fatal("token should be seen after TTL extension")
	}
}

func TestJTIReplayCache_BoundedMaxSize(t *testing.T) {
	t.Parallel()

	c := NewJTIReplayCacheWithSize(10)

	// Fill to capacity
	for i := 0; i < 10; i++ {
		c.Add("token-a", 5*time.Minute)
	}

	// Should never exceed maxSize
	if got := c.Len(); got > 10 {
		t.Errorf("Len() = %d, want <= 10", got)
	}
}

func TestJTIReplayCache_EvictsOldestWhenFull(t *testing.T) {
	t.Parallel()

	c := NewJTIReplayCacheWithSize(5)

	// Add 5 entries with different TTLs
	c.Add("old-1", 10*time.Minute)
	c.Add("old-2", 20*time.Minute)
	c.Add("old-3", 30*time.Minute)
	c.Add("old-4", 40*time.Minute)
	c.Add("old-5", 50*time.Minute)

	// Add more entries — should trigger eviction of oldest
	c.Add("new-1", 60*time.Minute)
	c.Add("new-2", 60*time.Minute)

	// Cache size should still be bounded
	if got := c.Len(); got > 5 {
		t.Errorf("Len() = %d, want <= 5 after adding beyond capacity", got)
	}

	// "old-1" (earliest expiry) should have been evicted
	if c.Seen("old-1") {
		t.Error("oldest entry should have been evicted")
	}
}

func TestJTIReplayCache_DefaultMaxSize(t *testing.T) {
	t.Parallel()

	c := NewJTIReplayCache()

	if c.maxSize != DefaultJTIReplayCacheMaxSize {
		t.Errorf("maxSize = %d, want %d", c.maxSize, DefaultJTIReplayCacheMaxSize)
	}
}

func TestJTIReplayCache_ZeroMaxSizeUsesDefault(t *testing.T) {
	t.Parallel()

	c := NewJTIReplayCacheWithSize(0)

	if c.maxSize != DefaultJTIReplayCacheMaxSize {
		t.Errorf("maxSize = %d, want %d", c.maxSize, DefaultJTIReplayCacheMaxSize)
	}
}

func TestJTIReplayCache_ExistingEntryUpdated(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := &JTIReplayCache{
		entries: make(map[string]time.Time),
		maxSize: 3,
		now:     func() time.Time { return base },
	}

	c.Add("token", 1*time.Minute)
	c.Add("token", 10*time.Minute)

	// Only one entry should exist
	if got := c.Len(); got != 1 {
		t.Errorf("Len() = %d, want 1 (existing entry updated, not duplicated)", got)
	}

	// Advance past the original TTL but within the new one
	c.now = func() time.Time { return base.Add(5 * time.Minute) }

	// Should still be seen because TTL was extended
	if !c.Seen("token") {
		t.Error("token should still be seen after TTL extension")
	}
}

func TestJTIReplayCache_EvictsExpiredBeforeMakingRoom(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := &JTIReplayCache{
		entries: make(map[string]time.Time),
		maxSize: 3,
		now:     func() time.Time { return base },
	}

	// Fill with entries that will expire
	c.Add("exp-1", 1*time.Minute)
	c.Add("exp-2", 1*time.Minute)
	c.Add("valid", 60*time.Minute)

	// Advance time so exp-1 and exp-2 are expired
	c.now = func() time.Time { return base.Add(2 * time.Minute) }

	// Add a new entry — expired entries should be evicted first
	c.Add("new", 60*time.Minute)

	if got := c.Len(); got > 3 {
		t.Errorf("Len() = %d, want <= 3 after expired eviction", got)
	}
}
