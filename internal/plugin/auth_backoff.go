package plugin

import (
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/APICerberus/APICerebrus/internal/pkg/netutil"
)

// AuthBackoff implements per-IP exponential backoff for failed authentication attempts.
// After each failed attempt from the same IP, the delay doubles (starting at 100ms),
// up to a maximum of 30 seconds. The counter resets after a successful auth or after
// the max delay period has elapsed.
type AuthBackoff struct {
	mu          sync.Mutex
	failures    map[string]*ipBackoffState
	initialDelay time.Duration
	maxDelay     time.Duration
	maxFailures  int
}

type ipBackoffState struct {
	count      int
	lastFail   time.Time
	currentDelay time.Duration
}

// NewAuthBackoff creates a new auth backoff tracker.
func NewAuthBackoff() *AuthBackoff {
	return &AuthBackoff{
		failures:     make(map[string]*ipBackoffState),
		initialDelay: 100 * time.Millisecond,
		maxDelay:     30 * time.Second,
		maxFailures:  100, // after this many failures, IP is permanently blocked until cleanup
	}
}

// Check returns a delay duration if the IP should be backoff-delayed, or 0 if allowed.
// It also cleans up stale entries whose backoff period has fully elapsed.
func (ab *AuthBackoff) Check(req *http.Request) time.Duration {
	ip := extractIPForBackoff(req)
	if ip == "" {
		return 0
	}

	ab.mu.Lock()
	defer ab.mu.Unlock()

	state, ok := ab.failures[ip]
	if !ok {
		return 0
	}

	// If the backoff period has fully elapsed since the last failure, reset.
	if time.Since(state.lastFail) > state.currentDelay {
		delete(ab.failures, ip)
		return 0
	}

	// Calculate remaining delay time.
	remaining := state.currentDelay - time.Since(state.lastFail)
	if remaining <= 0 {
		delete(ab.failures, ip)
		return 0
	}

	return remaining
}

// RecordFailure records a failed auth attempt and returns the new backoff delay.
func (ab *AuthBackoff) RecordFailure(req *http.Request) time.Duration {
	ip := extractIPForBackoff(req)
	if ip == "" {
		return 0
	}

	ab.mu.Lock()
	defer ab.mu.Unlock()

	state, ok := ab.failures[ip]
	if !ok {
		state = &ipBackoffState{
			count:        0,
			currentDelay: ab.initialDelay,
		}
		ab.failures[ip] = state
	}

	state.count++
	state.lastFail = time.Now()
	// Exponential backoff: delay doubles each failure
	state.currentDelay = ab.initialDelay * time.Duration(math.Pow(2, float64(state.count-1)))
	if state.currentDelay > ab.maxDelay {
		state.currentDelay = ab.maxDelay
	}

	return state.currentDelay
}

// RecordSuccess clears the backoff state for the IP after successful auth.
func (ab *AuthBackoff) RecordSuccess(req *http.Request) {
	ip := extractIPForBackoff(req)
	if ip == "" {
		return
	}

	ab.mu.Lock()
	delete(ab.failures, ip)
	ab.mu.Unlock()
}

// Len returns the number of tracked IPs (for testing/monitoring).
func (ab *AuthBackoff) Len() int {
	ab.mu.Lock()
	defer ab.mu.Unlock()
	return len(ab.failures)
}

func extractIPForBackoff(req *http.Request) string {
	if req == nil {
		return ""
	}
	return netutil.ExtractClientIP(req)
}
