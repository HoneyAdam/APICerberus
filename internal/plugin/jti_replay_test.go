package plugin

import (
	"testing"
	"time"
)

func TestJTIReplayCache_AddAndSeen(t *testing.T) {
	t.Parallel()

	c := &JTIReplayCache{
		entries: make(map[string]time.Time),
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
		now:     func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
	}

	c.Add("token", 1*time.Minute)
	c.Add("token", 10*time.Minute) // extend TTL

	if !c.Seen("token") {
		t.Fatal("token should be seen after TTL extension")
	}
}
